package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/revans/skill-eval/pkg/skilleval"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
// Falls back to "dev" for local builds.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-version":
			fmt.Printf("skill-eval %s\n", version)
			os.Exit(0)
		case "compile-summary":
			os.Exit(runCompileSummary(os.Args[2:]))
		case "init":
			os.Exit(skilleval.RunInit(os.Args[2:]))
		case "scan":
			os.Exit(skilleval.RunScan(os.Args[2:]))
		}
	}
	os.Exit(dispatch())
}

// isTargetedMode reports whether the given flags request targeted mode.
func isTargetedMode(f cliFlags) bool {
	return f.promptFile != "" || f.evalID != ""
}

// validateFlags checks flag mutual exclusions.
// Returns ("", 0) when valid. Returns (errMsg, 2) on conflict.
func validateFlags(f cliFlags) (string, int) {
	// Suite-mode rejection of targeted-only flags.
	if !isTargetedMode(f) {
		if f.model != "" {
			return "--model is only available in targeted mode (--prompt-file and/or --eval).\n\nSuite mode always uses the default model from .skill-eval.yml.\nFor multi-model runs in suite mode, use shell loops:\n\n  for m in claude-sonnet-4-6 claude-haiku-4-5 claude-opus-4-7; do\n    skill-eval --compare\n  done\n\nEach iteration's results land in their own summary file with a different\ntimestamp.", 2
		}
		if f.allModels {
			return "--all-models is only available in targeted mode (--prompt-file and/or --eval).\n\nSuite mode always uses the default model from .skill-eval.yml.\nFor multi-model runs in suite mode, use shell loops:\n\n  for m in claude-sonnet-4-6 claude-haiku-4-5 claude-opus-4-7; do\n    skill-eval --compare\n  done\n\nEach iteration's results land in their own summary file with a different\ntimestamp.", 2
		}
		return "", 0
	}
	if f.filter != "" {
		return "--filter cannot be used with --prompt-file or --eval", 2
	}
	if f.compare {
		return "--compare cannot be used with --prompt-file or --eval (targeted mode is always compare)", 2
	}
	if f.model != "" && f.allModels {
		return "--model and --all-models are mutually exclusive", 2
	}
	return "", 0
}

// dispatch parses flags, validates mutual exclusions, loads config and evals,
// then routes to suite mode or targeted mode.
func dispatch() int {
	f := parseFlags()

	if f.concurrency == 0 {
		fmt.Fprintf(os.Stderr, "error: --concurrency must be >= 1\n")
		return 2
	}

	if msg, code := validateFlags(f); code != 0 {
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
		return code
	}

	cfg, err := skilleval.LoadConfig(f.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	evals, err := skilleval.LoadEvals(cfg.EvalsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	if isTargetedMode(f) {
		return runTargetedMode(f, cfg, evals)
	}
	return runSuiteMode(f, cfg, evals)
}

// --- Targeted mode ---

func runTargetedMode(f cliFlags, cfg skilleval.Config, allEvals []skilleval.Eval) int {
	var evals []skilleval.Eval

	if f.evalID != "" {
		e, ok := skilleval.ResolveByID(allEvals, f.evalID)
		if !ok {
			fmt.Fprintf(os.Stderr, "error: no eval with ID %q\n", f.evalID)
			return 2
		}
		evals = []skilleval.Eval{e}
	} else {
		evals = skilleval.ResolveByPromptFile(allEvals, f.promptFile)
		if len(evals) == 0 {
			fmt.Fprintf(os.Stderr, "error: no evals with prompt_file %q\n", f.promptFile)
			return 2
		}
	}

	// Resolve the model list using the four-tier fallback.
	modelFlag := skilleval.ParseModelList(f.model)
	models, err := skilleval.ResolveModels(evals[0], modelFlag, f.allModels, cfg.DefaultModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	concurrency := cfg.Concurrency
	if f.concurrency > 0 {
		concurrency = f.concurrency
	}
	if concurrency < 1 {
		concurrency = 1
	}

	return skilleval.RunTargeted(evals, cfg, models, concurrency, os.Stdout)
}

// --- Suite mode ---

func runSuiteMode(f cliFlags, cfg skilleval.Config, evals []skilleval.Eval) int {
	if f.filter != "" {
		evals = applyFilter(evals, f.filter)
		if len(evals) == 0 {
			fmt.Fprintf(os.Stderr, "error: no evals matched filter %q\n", f.filter)
			return 2
		}
	}

	concurrency := cfg.Concurrency
	if f.concurrency > 0 {
		concurrency = f.concurrency
	}

	suite := &skilleval.Suite{
		Evaluator:   &skilleval.Evaluator{Config: cfg},
		Concurrency: concurrency,
		Compare:     f.compare,
	}

	printHeader(len(evals), cfg.DefaultModel, concurrency, f.compare)

	wallStart := time.Now()

	workerResults := suite.Run(evals, func(completed, total int, wr skilleval.WorkerResult) {
		if f.compare {
			printCompareProgress(completed, total, wr, concurrency)
		} else {
			printSingleProgress(completed, total, wr, concurrency)
		}
		if wr.WriteErr != nil {
			fmt.Fprintf(os.Stderr, "warning: artifact write failed for %s: %v\n", workerResultID(wr), wr.WriteErr)
		}
	})

	sort.Slice(workerResults, func(i, j int) bool {
		return workerResultID(workerResults[i]) < workerResultID(workerResults[j])
	})

	elapsed := time.Since(wallStart)

	var exitCode int
	if f.compare {
		exitCode = compareSummaryDisplay(workerResults, elapsed)
	} else {
		exitCode = singleSummaryDisplay(workerResults, elapsed)
	}

	// Compile and write the run summary.
	records := make([]skilleval.PerEvalRecord, len(workerResults))
	for i, wr := range workerResults {
		records[i] = skilleval.WorkerResultToRecord(wr)
	}
	s := skilleval.CompileSummary(records, elapsed, wallStart, cfg.DefaultModel)
	if summaryPath, err := skilleval.WriteSummary(cfg.SummariesDir, wallStart, s); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write summary: %v\n", err)
	} else {
		fmt.Printf("\nSummary written to %s\n", summaryPath)
	}

	return exitCode
}

func workerResultID(wr skilleval.WorkerResult) string {
	if wr.Compare != nil {
		return wr.Compare.EvalID
	}
	return wr.Result.EvalID
}

func printHeader(total int, model string, concurrency int, compare bool) {
	if compare {
		if concurrency > 1 {
			fmt.Printf("\nRunning %d evals against %s in compare mode (concurrency: %d)...\n\n", total, model, concurrency)
		} else {
			fmt.Printf("\nRunning %d evals against %s in compare mode...\n\n", total, model)
		}
	} else {
		if concurrency > 1 {
			fmt.Printf("\nRunning %d evals against %s (concurrency: %d)...\n\n", total, model, concurrency)
		} else {
			fmt.Printf("\nRunning %d evals against %s...\n\n", total, model)
		}
	}
}

// --- Single mode display ---

func printSingleProgress(completed, total int, wr skilleval.WorkerResult, concurrency int) {
	r := wr.Result
	durSec := float64(r.DurationMs) / 1000.0

	prefix := ""
	if concurrency > 1 {
		prefix = fmt.Sprintf("[%d/%d] ", completed, total)
	}

	if r.Err != nil {
		fmt.Printf("%sERROR %-8s  %-8s (%.1fs)\n", prefix, r.EvalID, r.TestsID, durSec)
		fmt.Printf("      error: %v\n", r.Err)
	} else if r.Passed {
		fmt.Printf("%sPASS  %-8s  %-8s (%.1fs)\n", prefix, r.EvalID, r.TestsID, durSec)
	} else {
		fmt.Printf("%sFAIL  %-8s  %-8s (%.1fs)\n", prefix, r.EvalID, r.TestsID, durSec)
		for _, a := range r.Assertions {
			if !a.Passed {
				fmt.Printf("      %s %q - failed\n", a.Type, a.Value)
			}
		}
	}
}

func singleSummaryDisplay(results []skilleval.WorkerResult, elapsed time.Duration) int {
	passed := 0
	failed := 0
	var failures []skilleval.WorkerResult
	for _, wr := range results {
		r := wr.Result
		if r.Err != nil || !r.Passed {
			failed++
			failures = append(failures, wr)
		} else {
			passed++
		}
	}

	fmt.Printf("\nRun complete: %d passed, %d failed in %s.\n", passed, failed, formatDuration(elapsed))

	if len(failures) > 0 {
		fmt.Printf("\nFailures:\n")
		for _, wr := range failures {
			r := wr.Result
			fmt.Printf("  %-8s  %-8s  %s\n", r.EvalID, r.TestsID, primaryFailureReason(r))
		}
	}

	if failed > 0 {
		return 1
	}
	return 0
}

// --- Compare mode display ---

func printCompareProgress(completed, total int, wr skilleval.WorkerResult, concurrency int) {
	cr := wr.Compare
	durSec := float64(cr.WithPrompt.DurationMs+cr.WithoutPrompt.DurationMs) / 1000.0

	prefix := ""
	if concurrency > 1 {
		prefix = fmt.Sprintf("[%d/%d] ", completed, total)
	}

	if cr.Err != nil {
		fmt.Printf("%s%-14s%-8s  %-8s (%.1fs)\n", prefix, "ERROR", cr.EvalID, cr.TestsID, durSec)
		fmt.Printf("      error: %v\n", cr.Err)
	} else {
		label := classificationLabel(cr.Classification)
		fmt.Printf("%s%-14s%-8s  %-8s (%.1fs)\n", prefix, label, cr.EvalID, cr.TestsID, durSec)
	}
}

func classificationLabel(c skilleval.Classification) string {
	switch c {
	case skilleval.LoadBearing:
		return "LOAD-BEARING"
	case skilleval.Obsolete:
		return "OBSOLETE"
	case skilleval.Insufficient:
		return "INSUFFICIENT"
	case skilleval.Harmful:
		return "HARMFUL"
	default:
		return "UNKNOWN"
	}
}

type notableEntry struct {
	testsID string
	evalID  string
}

func compareSummaryDisplay(results []skilleval.WorkerResult, elapsed time.Duration) int {
	counts := map[skilleval.Classification]int{}
	var errored []notableEntry
	var obsolete []notableEntry
	var harmful []notableEntry

	for _, wr := range results {
		cr := wr.Compare
		entry := notableEntry{testsID: cr.TestsID, evalID: cr.EvalID}
		if cr.Err != nil {
			errored = append(errored, entry)
		} else {
			counts[cr.Classification]++
			switch cr.Classification {
			case skilleval.Obsolete:
				obsolete = append(obsolete, entry)
			case skilleval.Harmful:
				harmful = append(harmful, entry)
			}
		}
	}

	classified := len(results) - len(errored)

	fmt.Printf("\nRun complete: %d evals classified in %s.\n", len(results), formatDuration(elapsed))
	fmt.Printf("\nClassifications:\n")
	printClassLine("Load-bearing:", counts[skilleval.LoadBearing], classified)
	printClassLine("Obsolete:", counts[skilleval.Obsolete], classified)
	printClassLine("Insufficient:", counts[skilleval.Insufficient], classified)
	printClassLine("Harmful:", counts[skilleval.Harmful], classified)
	fmt.Printf("  %-14s %d\n", "Errors:", len(errored))

	if len(obsolete) > 0 || len(harmful) > 0 || len(errored) > 0 {
		fmt.Printf("\nNotable findings:\n")
		if len(obsolete) > 0 {
			fmt.Printf("  Obsolete (consider removal):\n")
			for _, e := range obsolete {
				fmt.Printf("    %-8s  %s\n", e.testsID, e.evalID)
			}
		}
		if len(harmful) > 0 {
			fmt.Printf("  Harmful (investigate):\n")
			for _, e := range harmful {
				fmt.Printf("    %-8s  %s\n", e.testsID, e.evalID)
			}
		}
		if len(errored) > 0 {
			fmt.Printf("  Errors:\n")
			for _, e := range errored {
				fmt.Printf("    %-8s  %s\n", e.testsID, e.evalID)
			}
		}
	}

	if len(errored) > 0 {
		return 1
	}
	return 0
}

func printClassLine(label string, count, classified int) {
	pct := 0.0
	if classified > 0 {
		pct = float64(count) / float64(classified) * 100.0
	}
	fmt.Printf("  %-16s %-4d (%.0f%%)\n", label, count, pct)
}

// --- compile-summary subcommand ---

func runCompileSummary(args []string) int {
	fs := flag.NewFlagSet("compile-summary", flag.ContinueOnError)
	configPath := fs.String("config", ".skill-eval.yml", "path to config file")
	tsFlag := fs.String("timestamp", "", "compile summary for evals at this exact timestamp ("+skilleval.TimestampFormat+")")
	sinceFlag := fs.String("since", "", "compile summary for evals since this timestamp (inclusive)")
	untilFlag := fs.String("until", "", "compile summary for evals until this timestamp (inclusive)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *tsFlag == "" && (*sinceFlag == "" || *untilFlag == "") {
		fmt.Fprintf(os.Stderr, "error: provide --timestamp or both --since and --until\n")
		return 2
	}

	cfg, err := skilleval.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	var records []skilleval.PerEvalRecord
	var summaryPath string

	if *tsFlag != "" {
		records, err = skilleval.ReadResultsFromDir(cfg.ResultsDir, *tsFlag, *tsFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		ts, _ := time.Parse(skilleval.TimestampFormat, *tsFlag)
		model := modelFromRecords(records, cfg.DefaultModel)
		s := skilleval.CompileSummary(records, 0, ts, model)
		summaryPath, err = skilleval.WriteSummary(cfg.SummariesDir, ts, s)
	} else {
		records, err = skilleval.ReadResultsFromDir(cfg.ResultsDir, *sinceFlag, *untilFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		ts, _ := time.Parse(skilleval.TimestampFormat, *sinceFlag)
		model := modelFromRecords(records, cfg.DefaultModel)
		s := skilleval.CompileSummary(records, 0, ts, model)
		summaryPath, err = skilleval.WriteAggregateSummary(cfg.SummariesDir, *sinceFlag, *untilFlag, s)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing summary: %v\n", err)
		return 1
	}

	fmt.Printf("Summary written to %s (%d evals)\n", summaryPath, len(records))
	return 0
}

func modelFromRecords(records []skilleval.PerEvalRecord, fallback string) string {
	if len(records) > 0 {
		return records[0].Model
	}
	return fallback
}

// --- shared helpers ---

func primaryFailureReason(r skilleval.EvalResult) string {
	if r.Err != nil {
		return "error: " + r.Err.Error()
	}
	for _, a := range r.Assertions {
		if !a.Passed {
			return fmt.Sprintf("%s %q - failed", a.Type, a.Value)
		}
	}
	return "unknown failure"
}

func applyFilter(evals []skilleval.Eval, expr string) []skilleval.Eval {
	var filtered []skilleval.Eval
	for _, e := range evals {
		if strings.HasPrefix(e.ID, expr) || e.Tests == expr {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, s)
}
