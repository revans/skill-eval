package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// anyResultYML is a unified read-back struct that handles both single-mode
// (resultYML) and compare-mode (compareResultYML) per-eval YAML files.
type anyResultYML struct {
	EvalID         string    `yaml:"eval_id"`
	Tests          string    `yaml:"tests"`
	RanAt          string    `yaml:"ran_at"`
	Mode           string    `yaml:"mode"`
	Model          string    `yaml:"model"`
	WithPrompt     runBlock  `yaml:"with_prompt"`
	WithoutPrompt  *runBlock `yaml:"without_prompt"` // nil for single-mode files
	Classification string    `yaml:"classification"`
}

// ReadResultsFromDir walks resultsDir and returns PerEvalRecords for every
// *-result.yml whose filename timestamp falls in [sinceStr, untilStr] (inclusive).
// Either bound may be empty to disable that side of the filter.
// Timestamps use TimestampFormat ("2006-01-02-T15-04").
func ReadResultsFromDir(resultsDir, sinceStr, untilStr string) ([]PerEvalRecord, error) {
	var since, until time.Time
	var err error
	if sinceStr != "" {
		since, err = time.Parse(TimestampFormat, sinceStr)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp %q: %w", sinceStr, err)
		}
	}
	if untilStr != "" {
		until, err = time.Parse(TimestampFormat, untilStr)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp %q: %w", untilStr, err)
		}
	}

	var records []PerEvalRecord

	err = filepath.WalkDir(resultsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, "-result.yml") {
			return nil
		}

		stamp := strings.TrimSuffix(name, "-result.yml")
		fileTime, parseErr := time.Parse(TimestampFormat, stamp)
		if parseErr != nil {
			return nil // skip files with unrecognized timestamp format
		}

		if !since.IsZero() && fileTime.Before(since) {
			return nil
		}
		if !until.IsZero() && fileTime.After(until) {
			return nil
		}

		rec, readErr := readPerEvalYML(path)
		if readErr != nil {
			return fmt.Errorf("reading %q: %w", path, readErr)
		}
		records = append(records, rec)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking results dir %q: %w", resultsDir, err)
	}

	return records, nil
}

func readPerEvalYML(path string) (PerEvalRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PerEvalRecord{}, err
	}

	var y anyResultYML
	if err := yaml.Unmarshal(data, &y); err != nil {
		return PerEvalRecord{}, fmt.Errorf("parsing YAML: %w", err)
	}

	ranAt, err := time.Parse(time.RFC3339, y.RanAt)
	if err != nil {
		return PerEvalRecord{}, fmt.Errorf("parsing ran_at %q: %w", y.RanAt, err)
	}

	rec := PerEvalRecord{
		EvalID:  y.EvalID,
		TestsID: y.Tests,
		Model:   y.Model,
		RanAt:   ranAt,
		Mode:    y.Mode,
	}

	if y.Mode == "compare" {
		durMs := y.WithPrompt.DurationMs
		if y.WithoutPrompt != nil {
			durMs += y.WithoutPrompt.DurationMs
		}
		rec.DurationMs = durMs
		rec.Classification = y.Classification
		if rec.Classification == "" {
			rec.Classification = "error"
		}
	} else {
		rec.DurationMs = y.WithPrompt.DurationMs
		rec.Status = y.WithPrompt.Status
		if rec.Status != "pass" {
			for _, a := range y.WithPrompt.Assertions {
				if a.Result == "fail" {
					rec.FailureReason = fmt.Sprintf("%s %q - failed", a.Type, a.Value)
					break
				}
			}
			if rec.Status == "error" && rec.FailureReason == "" {
				rec.FailureReason = "subprocess error"
			}
		}
	}

	return rec, nil
}
