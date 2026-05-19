package skilleval

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// RunScan executes the scan subcommand. It discovers prompt files via --dir
// (recursive .md walk) and/or --glob, scaffolds eval entries for files not
// already covered in evals.yml, and appends them.
func RunScan(args []string) int {
	fset := flag.NewFlagSet("scan", flag.ContinueOnError)
	dirFlag := fset.String("dir", "", "scan all .md files in this directory (recursive)")
	globFlag := fset.String("glob", "", "scan files matching this glob pattern")
	dryRun := fset.Bool("dry-run", false, "print what would be added without modifying evals.yml")
	configPath := fset.String("config", ".skill-eval.yml", "path to config file")
	aiFlag := fset.Bool("ai", false, "generate input and assertions using Claude (review before running)")

	if err := fset.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *dirFlag == "" && *globFlag == "" {
		fmt.Fprintf(os.Stderr,
			"Error: skill-eval scan requires --dir or --glob\n"+
				"Example: skill-eval scan --dir prompts/\n"+
				"Example: skill-eval scan --glob \"rules/*.md\"\n",
		)
		return 2
	}

	evalsFile := initEvalsFile(*configPath)

	files, err := discoverFiles(*dirFlag, *globFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 2
	}

	if len(files) == 0 {
		fmt.Printf("\nNo prompt files found.\n")
		return 0
	}

	existing, parseErr := parseEvalsForInit(evalsFile)
	if parseErr == errMalformedEvals {
		fmt.Fprintf(os.Stderr,
			"Error: %s is malformed and cannot be parsed.\nFix the file before running scan.\n",
			evalsFile,
		)
		return 2
	}
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "Error: reading %s: %v\n", evalsFile, parseErr)
		return 2
	}

	covered := make(map[string]bool, len(existing))
	for _, e := range existing {
		covered[e.Tests] = true
	}

	var aiModel string
	var aiTimeout int
	if *aiFlag {
		aiModel, aiTimeout = loadAIConfig(*configPath)
	}

	type scaffoldEntry struct {
		evalID   string
		promptID string
		path     string
		text     string
	}

	workEntries := append([]evalInitEntry{}, existing...)
	var toAdd []scaffoldEntry
	var skipped []string

	for _, f := range files {
		pid := ExtractSubstrateID(f)
		if covered[pid] {
			skipped = append(skipped, f)
			continue
		}
		nextID := nextEvalID(workEntries)

		var text string
		if *aiFlag {
			fmt.Printf("  Generating %s (%s)...\n", nextID, f)
			fields, err := generateEvalFields(f, aiModel, aiTimeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: AI generation failed for %s (%v); using placeholder.\n", f, err)
				text = formatPlaceholderEntry(nextID, pid, f)
			} else {
				text = formatAIEntry(nextID, pid, f, fields)
			}
		} else {
			text = formatPlaceholderEntry(nextID, pid, f)
		}

		toAdd = append(toAdd, scaffoldEntry{nextID, pid, f, text})
		workEntries = append(workEntries, evalInitEntry{ID: nextID, Tests: pid})
		covered[pid] = true
	}

	fmt.Printf("\n")

	if len(skipped) > 0 {
		noun := "files"
		if len(skipped) == 1 {
			noun = "file"
		}
		fmt.Printf("Skipping %d %s already covered in %s:\n", len(skipped), noun, evalsFile)
		for _, f := range skipped {
			fmt.Printf("  %s\n", f)
		}
		fmt.Printf("\n")
	}

	if len(toAdd) == 0 {
		fmt.Printf("All prompt files already have evals. Nothing to add.\n")
		return 0
	}

	noun := "entries"
	if len(toAdd) == 1 {
		noun = "entry"
	}

	if *dryRun {
		fmt.Printf("Would add %d %s to %s (--dry-run, no changes made):\n\n", len(toAdd), noun, evalsFile)
	} else {
		fmt.Printf("Adding %d %s to %s:\n\n", len(toAdd), noun, evalsFile)
	}

	for _, e := range toAdd {
		fmt.Printf("  %-8s  %-24s  %s\n", e.evalID, e.promptID, e.path)
	}

	if *dryRun {
		return 0
	}

	for _, e := range toAdd {
		if err := appendToEvalsFile(evalsFile, e.text); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: failed to write %s: %v\n", evalsFile, err)
			return 1
		}
	}

	if *aiFlag {
		fmt.Printf("\nDone. Review the AI-generated fields in %s before running evals.\n", evalsFile)
	} else {
		fmt.Printf("\nDone. Fill in the TODO fields in %s before running evals.\n", evalsFile)
	}
	return 0
}

// discoverFiles returns a deduplicated, sorted list of files found via dir
// (recursive .md walk) and/or glob pattern. Both are optional but at least
// one must be non-empty.
func discoverFiles(dir, glob string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string

	if dir != "" {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && filepath.Ext(path) == ".md" && !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning directory %q: %w", dir, err)
		}
	}

	if glob != "" {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", glob, err)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				files = append(files, m)
			}
		}
	}

	sort.Strings(files)
	return files, nil
}
