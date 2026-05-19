package skilleval

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	matrixCol1Width    = 24 // width of the eval-identifier column
	matrixModelColWidth = 12 // width of each per-model classification column
)

// RenderMatrix renders the multi-model comparison table, per-model summary,
// notes (including cross-model conflicts), and artifacts path.
// Returns 0 on success, 1 if any (eval, model) pair errored.
func RenderMatrix(evals []Eval, models []string, results []ModelTaskResult, cfg Config, w io.Writer) int {
	// Index results: evalID → model → result
	indexed := make(map[string]map[string]ModelTaskResult)
	for _, r := range results {
		if indexed[r.EvalID] == nil {
			indexed[r.EvalID] = make(map[string]ModelTaskResult)
		}
		indexed[r.EvalID][r.Model] = r
	}

	shortNames := make([]string, len(models))
	for i, m := range models {
		shortNames[i] = ShortModelName(m)
	}

	// Header row: blank col1 + model short names
	fmt.Fprintf(w, "%-*s", matrixCol1Width, "")
	for _, sn := range shortNames {
		fmt.Fprintf(w, "%-*s", matrixModelColWidth, sn)
	}
	fmt.Fprintf(w, "\n")

	// Data rows
	errCount := 0
	for _, e := range evals {
		label := fmt.Sprintf("%s (%s)", e.Tests, e.ID)
		fmt.Fprintf(w, "%-*s", matrixCol1Width, label)
		modelResults := indexed[e.ID]
		var timings []string
		for i, m := range models {
			r := modelResults[m]
			cr := r.Compare
			if cr.Err != nil {
				fmt.Fprintf(w, "%-*s", matrixModelColWidth, "ERROR")
				errCount++
			} else {
				fmt.Fprintf(w, "%-*s", matrixModelColWidth, matrixClassLabel(cr.Classification))
			}
			durSec := float64(cr.WithPrompt.DurationMs+cr.WithoutPrompt.DurationMs) / 1000.0
			timings = append(timings, fmt.Sprintf("%s %.1fs", strings.ToLower(shortNames[i]), durSec))
		}
		fmt.Fprintf(w, "(%s)\n", strings.Join(timings, ", "))
	}

	// Per-model summary
	fmt.Fprintf(w, "\nPer-model summary:\n")
	for _, m := range models {
		counts := map[Classification]int{}
		for _, e := range evals {
			if r, ok := indexed[e.ID][m]; ok && r.Compare.Err == nil {
				counts[r.Compare.Classification]++
			}
		}
		var parts []string
		for _, c := range []Classification{LoadBearing, Obsolete, Insufficient, Harmful} {
			if n := counts[c]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, string(c)))
			}
		}
		summary := strings.Join(parts, ", ")
		if summary == "" {
			summary = "all errors"
		}
		fmt.Fprintf(w, "  %s: %s\n", m, summary)
	}

	// Notes section
	notes := buildMatrixNotes(evals, models, shortNames, indexed)
	if len(notes) > 0 {
		fmt.Fprintf(w, "\nNotes:\n")
		for _, n := range notes {
			fmt.Fprintf(w, "%s\n", n)
		}
	}

	// Artifacts path
	artifactsDir := matrixArtifactsRoot(evals, cfg.ResultsDir)
	fmt.Fprintf(w, "\nArtifacts: %s\n", artifactsDir)

	if errCount > 0 {
		return 1
	}
	return 0
}

// matrixClassLabel returns the abbreviated classification label used in matrix cells.
func matrixClassLabel(c Classification) string {
	switch c {
	case LoadBearing:
		return "LOAD"
	case Obsolete:
		return "OBSOLETE"
	case Insufficient:
		return "INSUFFICIENT"
	case Harmful:
		return "HARMFUL"
	default:
		return "ERROR"
	}
}

// ShortModelName returns a human-readable short name for a model identifier.
// It finds the last all-alphabetic segment that isn't "claude", then capitalizes it.
// Falls back to the full identifier if no such segment exists.
func ShortModelName(model string) string {
	parts := strings.Split(model, "-")
	var last string
	for _, p := range parts {
		if p == "" || p == "claude" {
			continue
		}
		allAlpha := true
		for _, c := range p {
			if !unicode.IsLetter(c) {
				allAlpha = false
				break
			}
		}
		if allAlpha {
			last = p
		}
	}
	if last == "" {
		return model
	}
	return strings.ToUpper(last[:1]) + last[1:]
}

// buildMatrixNotes generates actionable note lines for the notes section.
func buildMatrixNotes(evals []Eval, models []string, shortNames []string, indexed map[string]map[string]ModelTaskResult) []string {
	var notes []string

	for _, e := range evals {
		modelResults := indexed[e.ID]
		classes := make([]Classification, 0, len(models))
		classMap := make(map[string]Classification) // model → classification

		for _, m := range models {
			r := modelResults[m]
			if r.Compare.Err == nil {
				classes = append(classes, r.Compare.Classification)
				classMap[m] = r.Compare.Classification
			}
		}

		if len(classes) == 0 {
			continue
		}

		// Check for cross-model conflicts first.
		if conflict, conflictNotes := buildConflictNote(e.ID, models, shortNames, classMap); conflict {
			notes = append(notes, conflictNotes...)
			continue
		}

		// Per-classification actionable notes (no conflict).
		classSet := classificationSet(classes)
		if _, has := classSet[Harmful]; has {
			for _, m := range models {
				if classMap[m] == Harmful {
					sn := ShortModelName(m)
					notes = append(notes, fmt.Sprintf("  %s harmful on %s — model performs better without the prompt.", e.ID, sn))
				}
			}
		}
		if _, has := classSet[Obsolete]; has {
			for _, m := range models {
				if classMap[m] == Obsolete {
					sn := ShortModelName(m)
					notes = append(notes, fmt.Sprintf("  %s obsolete on %s — consider whether the prompt is needed\n  for %s's behavior on this model tier.", e.ID, sn, e.ID))
				}
			}
		}
		if _, has := classSet[Insufficient]; has {
			for _, m := range models {
				if classMap[m] == Insufficient {
					sn := ShortModelName(m)
					notes = append(notes, fmt.Sprintf("  %s insufficient on %s — neither run passes.", e.ID, sn))
				}
			}
		}
	}

	return notes
}

// buildConflictNote generates notes for cross-model classification conflicts.
// Returns (true, notes) if a conflict is detected.
func buildConflictNote(evalID string, models []string, shortNames []string, classMap map[string]Classification) (bool, []string) {
	classSet := make(map[Classification]bool)
	for _, c := range classMap {
		classSet[c] = true
	}

	isConflict := (classSet[LoadBearing] && (classSet[Obsolete] || classSet[Harmful])) ||
		(classSet[Insufficient] && len(classSet) > 1)

	if !isConflict {
		return false, nil
	}

	var notes []string
	notes = append(notes, fmt.Sprintf("  %s has conflicting classifications:", evalID))
	for i, m := range models {
		if c, ok := classMap[m]; ok {
			notes = append(notes, fmt.Sprintf("    - %s: %s", shortNames[i], strings.ToUpper(string(c))))
		}
	}

	// Generate human-readable explanation for common conflict patterns.
	if classSet[LoadBearing] && classSet[Harmful] {
		notes = append(notes, "  This prompt is required on some models but degrades output on others.")
		notes = append(notes, "  Consider tier-conditional deployment or rewrite.")
	} else if classSet[LoadBearing] && classSet[Obsolete] {
		notes = append(notes, "  This prompt is load-bearing on some models and obsolete on others.")
		notes = append(notes, "  Consider whether the prompt should be model-conditional.")
	} else {
		notes = append(notes, "  Classifications diverge across models — investigate before deployment.")
	}

	return true, notes
}

// classificationSet returns a set of unique classifications from the given slice.
func classificationSet(classes []Classification) map[Classification]bool {
	s := make(map[Classification]bool, len(classes))
	for _, c := range classes {
		s[c] = true
	}
	return s
}

// matrixArtifactsRoot returns the artifact directory for the matrix run.
func matrixArtifactsRoot(evals []Eval, resultsDir string) string {
	if len(evals) == 0 {
		return resultsDir
	}
	if len(evals) == 1 {
		e := evals[0]
		return filepath.Join(resultsDir, e.Tests, e.ID)
	}
	testsID := evals[0].Tests
	for _, e := range evals[1:] {
		if e.Tests != testsID {
			return resultsDir
		}
	}
	return filepath.Join(resultsDir, testsID)
}
