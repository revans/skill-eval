package skilleval

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ModelTask is an (eval, model) work unit for targeted runs.
type ModelTask struct {
	Eval     Eval
	Model    string
	PerModel bool // true = write artifacts to a model subdirectory
}

// ModelTaskResult holds the complete outcome of one (eval, model) targeted comparison.
type ModelTaskResult struct {
	EvalID   string
	TestsID  string
	Model    string
	Compare  CompareResult
	Paths    ArtifactPaths
	WriteErr error
}

// ResolveByPromptFile returns all evals whose PromptFile exactly matches path.
// Evals with an inline prompt: field are never matched.
func ResolveByPromptFile(evals []Eval, path string) []Eval {
	var matched []Eval
	for _, e := range evals {
		if e.PromptFile == path {
			matched = append(matched, e)
		}
	}
	return matched
}

// ResolveByID finds the eval with the given ID.
// Returns (Eval{}, false) if not found.
func ResolveByID(evals []Eval, id string) (Eval, bool) {
	for _, e := range evals {
		if e.ID == id {
			return e, true
		}
	}
	return Eval{}, false
}

// RunTargeted runs evals in targeted mode for the given model list.
//   - len(models)==1: single-model output (same format as Phase 5)
//   - len(models)>1: matrix output
//
// No summary is written to evals/summaries/.
func RunTargeted(evals []Eval, cfg Config, models []string, concurrency int, w io.Writer) int {
	printTargetedHeader(evals, cfg, models, concurrency, w)

	tasks := buildModelTasks(evals, cfg.ResultsDir, models)
	results := runModelPool(tasks, &Evaluator{Config: cfg}, concurrency)

	sort.Slice(results, func(i, j int) bool {
		if results[i].EvalID != results[j].EvalID {
			return results[i].EvalID < results[j].EvalID
		}
		return results[i].Model < results[j].Model
	})

	if len(models) == 1 {
		if len(evals) == 1 {
			return renderSingleEvalSingleModel(results[0], w)
		}
		return renderMultiEvalSingleModel(results, cfg, w)
	}
	return RenderMatrix(evals, models, results, cfg, w)
}

// buildModelTasks expands (eval, model) pairs and determines artifact layout per eval.
// Layout is per-model if: more than one model is requested, OR the eval directory
// already contains subdirectories from a previous multi-model run.
func buildModelTasks(evals []Eval, resultsDir string, models []string) []ModelTask {
	multiModel := len(models) > 1
	var tasks []ModelTask
	for _, e := range evals {
		evalDir := filepath.Join(resultsDir, e.Tests, e.ID)
		perModel := multiModel
		if !perModel {
			has, _ := EvalDirHasSubdirs(evalDir)
			perModel = has
		}
		for _, m := range models {
			tasks = append(tasks, ModelTask{Eval: e, Model: m, PerModel: perModel})
		}
	}
	return tasks
}

// runModelPool executes all (eval, model) tasks using a worker pool.
func runModelPool(tasks []ModelTask, ev *Evaluator, concurrency int) []ModelTaskResult {
	if len(tasks) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}

	taskCh := make(chan ModelTask, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	resultCh := make(chan ModelTaskResult, len(tasks))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				resultCh <- runOneModelTask(task, ev)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var results []ModelTaskResult
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}

// runOneModelTask executes a single (eval, model) comparison and writes artifacts.
func runOneModelTask(task ModelTask, ev *Evaluator) ModelTaskResult {
	cfg := ev.Config
	ts := time.Now().UTC()
	evalDir := filepath.Join(cfg.ResultsDir, task.Eval.Tests, task.Eval.ID)

	var paths ArtifactPaths
	if task.PerModel {
		paths = BuildModelArtifactPaths(cfg.ResultsDir, task.Eval.Tests, task.Eval.ID, task.Model, ts)
		if err := PrepareEvalDirForModel(evalDir, task.Model); err != nil {
			return ModelTaskResult{
				EvalID:  task.Eval.ID,
				TestsID: task.Eval.Tests,
				Model:   task.Model,
				Compare: CompareResult{
					EvalID: task.Eval.ID, TestsID: task.Eval.Tests, Model: task.Model,
					RanAt: ts, Err: fmt.Errorf("preparing artifact dir: %w", err),
				},
				Paths: paths,
			}
		}
	} else {
		paths = BuildArtifactPaths(cfg.ResultsDir, task.Eval.Tests, task.Eval.ID, ts)
		if err := PrepareEvalDir(paths.Dir); err != nil {
			return ModelTaskResult{
				EvalID:  task.Eval.ID,
				TestsID: task.Eval.Tests,
				Model:   task.Model,
				Compare: CompareResult{
					EvalID: task.Eval.ID, TestsID: task.Eval.Tests, Model: task.Model,
					RanAt: ts, Err: fmt.Errorf("preparing artifact dir: %w", err),
				},
				Paths: paths,
			}
		}
	}

	cr := ev.RunCompareWithModel(task.Eval, task.Model)
	r := ModelTaskResult{EvalID: task.Eval.ID, TestsID: task.Eval.Tests, Model: task.Model, Compare: cr, Paths: paths}

	if cr.WithPrompt.Err == nil {
		if err := WriteWithPromptMD(paths.WithPromptMD, cr.WithPrompt.Output); err != nil {
			r.WriteErr = err
		}
	}
	if cr.WithoutPrompt.Err == nil {
		if err := WriteWithoutPromptMD(paths.WithoutPromptMD, cr.WithoutPrompt.Output); err != nil && r.WriteErr == nil {
			r.WriteErr = err
		}
	}
	if err := WriteCompareResultYML(paths, cr); err != nil && r.WriteErr == nil {
		r.WriteErr = err
	}

	return r
}

// --- Header ---

func printTargetedHeader(evals []Eval, cfg Config, models []string, concurrency int, w io.Writer) {
	multiModel := len(models) > 1

	var subject string
	if len(evals) == 1 {
		e := evals[0]
		subject = fmt.Sprintf("%s against %s", e.ID, e.Tests)
	} else {
		testsID := sharedTestsID(evals)
		if testsID != "" {
			subject = fmt.Sprintf("%d evals against %s", len(evals), testsID)
		} else {
			subject = fmt.Sprintf("%d evals", len(evals))
		}
	}

	if multiModel {
		if concurrency > 1 {
			fmt.Fprintf(w, "\nRunning %s in compare mode across %d models (concurrency: %d)...\n", subject, len(models), concurrency)
		} else {
			fmt.Fprintf(w, "\nRunning %s in compare mode across %d models...\n", subject, len(models))
		}
	} else {
		fmt.Fprintf(w, "\nRunning %s in compare mode...\n", subject)
		fmt.Fprintf(w, "Model: %s\n", models[0])
	}
	fmt.Fprintf(w, "\n")
}

// --- Single-model display (identical format to Phase 5) ---

func renderSingleEvalSingleModel(r ModelTaskResult, w io.Writer) int {
	cr := r.Compare
	if cr.Err != nil {
		fmt.Fprintf(w, "ERROR\n\n%v\n\nArtifacts: %s\n", cr.Err, r.Paths.Dir)
		return 1
	}

	fmt.Fprintf(w, "WITH prompt:\n")
	printAssertionResults(cr.WithPrompt.Assertions, w)
	fmt.Fprintf(w, "  Result: %s (%.1fs)\n\n", evalPassLabel(cr.WithPrompt), float64(cr.WithPrompt.DurationMs)/1000.0)

	fmt.Fprintf(w, "WITHOUT prompt:\n")
	printAssertionResults(cr.WithoutPrompt.Assertions, w)
	fmt.Fprintf(w, "  Result: %s (%.1fs)\n\n", evalPassLabel(cr.WithoutPrompt), float64(cr.WithoutPrompt.DurationMs)/1000.0)

	label := strings.ToUpper(string(cr.Classification))
	fmt.Fprintf(w, "Classification: %s\n\n%s\n\n", label, classificationExplanation(cr.Classification))
	fmt.Fprintf(w, "Artifacts: %s\n", r.Paths.Dir)
	return 0
}

func renderMultiEvalSingleModel(results []ModelTaskResult, cfg Config, w io.Writer) int {
	errCount := 0
	counts := map[Classification]int{}
	var obsolete []string
	var harmful []string

	for _, r := range results {
		cr := r.Compare
		durSec := float64(cr.WithPrompt.DurationMs+cr.WithoutPrompt.DurationMs) / 1000.0
		if cr.Err != nil {
			fmt.Fprintf(w, "%-8s  ERROR         (%.1fs)\n", cr.EvalID, durSec)
			errCount++
		} else {
			label := strings.ToUpper(string(cr.Classification))
			fmt.Fprintf(w, "%-8s  %-14s(%.1fs)\n", cr.EvalID, label, durSec)
			counts[cr.Classification]++
			switch cr.Classification {
			case Obsolete:
				obsolete = append(obsolete, cr.EvalID)
			case Harmful:
				harmful = append(harmful, cr.EvalID)
			}
		}
	}

	fmt.Fprintf(w, "\nSummary:\n")
	for _, c := range []Classification{LoadBearing, Obsolete, Insufficient, Harmful} {
		if counts[c] > 0 {
			fmt.Fprintf(w, "  %-16s %d\n", capitalizeClassification(c)+":", counts[c])
		}
	}
	if errCount > 0 {
		fmt.Fprintf(w, "  %-16s %d\n", "Errors:", errCount)
	}

	if len(obsolete) > 0 || len(harmful) > 0 {
		fmt.Fprintf(w, "\nNotes:\n")
		for _, id := range obsolete {
			fmt.Fprintf(w, "  %s obsolete — model produces correct output without the prompt.\n", id)
		}
		for _, id := range harmful {
			fmt.Fprintf(w, "  %s harmful — model performs better without the prompt.\n", id)
		}
	}

	fmt.Fprintf(w, "\nArtifacts: %s\n", modelResultsArtifactsRoot(results, cfg.ResultsDir))

	if errCount > 0 {
		return 1
	}
	return 0
}

// --- Helpers ---

func printAssertionResults(assertions []AssertionResult, w io.Writer) {
	for _, a := range assertions {
		mark := "✓"
		if !a.Passed {
			mark = "✗"
		}
		desc := fmt.Sprintf("%s %q", a.Type, a.Value)
		fmt.Fprintf(w, "  %-35s%s\n", desc, mark)
	}
}

func evalPassLabel(r EvalResult) string {
	if r.Err != nil {
		return "ERROR"
	}
	if r.Passed {
		return "PASS"
	}
	return "FAIL"
}

func classificationExplanation(c Classification) string {
	switch c {
	case LoadBearing:
		return "The prompt is doing work. The model needs the prompt loaded to produce\ncorrect output for this eval."
	case Obsolete:
		return "The model produces correct output without the prompt. Consider whether\nthe prompt is still needed."
	case Insufficient:
		return "Neither run passed. Investigate the prompt, the eval assertions, or\nthe input task."
	case Harmful:
		return "The prompt is degrading model output. The model performs better without it."
	default:
		return ""
	}
}

func capitalizeClassification(c Classification) string {
	s := string(c)
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sharedTestsID(evals []Eval) string {
	if len(evals) == 0 {
		return ""
	}
	id := evals[0].Tests
	for _, e := range evals[1:] {
		if e.Tests != id {
			return ""
		}
	}
	return id
}

// modelResultsArtifactsRoot returns the most specific common artifact directory.
func modelResultsArtifactsRoot(results []ModelTaskResult, resultsDir string) string {
	if len(results) == 0 {
		return resultsDir
	}
	testsID := results[0].TestsID
	for _, r := range results[1:] {
		if r.TestsID != testsID {
			return resultsDir
		}
	}
	return filepath.Join(resultsDir, testsID)
}
