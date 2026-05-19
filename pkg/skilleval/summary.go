package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Summary is the compiled per-run summary written to evals/summaries/.
type Summary struct {
	RanAt                string               `yaml:"-"`
	Model                string               `yaml:"-"`
	Mode                 string               `yaml:"-"`
	TotalEvals           int                  `yaml:"-"`
	TotalDurationSeconds int64                `yaml:"-"`
	TotalEvalTimeSeconds int64                `yaml:"-"`
	TotalPassed          int                  `yaml:"-"`
	TotalFailed          int                  `yaml:"-"`
	Classifications      *ClassificationCounts `yaml:"-"`
	Results              []SummaryResult      `yaml:"-"`
}

// ClassificationCounts holds the per-classification counts for compare-mode summaries.
type ClassificationCounts struct {
	LoadBearing  int `yaml:"load-bearing"`
	Obsolete     int `yaml:"obsolete"`
	Insufficient int `yaml:"insufficient"`
	Harmful      int `yaml:"harmful"`
	Error        int `yaml:"error"`
}

// SummaryResult is one row in a summary's results list.
type SummaryResult struct {
	EvalID         string `yaml:"eval_id"`
	Tests          string `yaml:"tests"`
	Status         string `yaml:"status,omitempty"`
	DurationMs     int64  `yaml:"duration_ms"`
	FailureReason  string `yaml:"failure_reason,omitempty"`
	Classification string `yaml:"classification,omitempty"`
}

// PerEvalRecord is the normalized form of a per-eval result used for summary compilation.
// It can be sourced from in-memory WorkerResults or from parsed disk YAML files.
type PerEvalRecord struct {
	EvalID         string
	TestsID        string
	Model          string
	RanAt          time.Time
	Mode           string // "single" or "compare"
	DurationMs     int64
	Status         string // single: "pass"/"fail"/"error"
	FailureReason  string
	Classification string // compare: "load-bearing", "obsolete", "insufficient", "harmful", "error"
}

// WorkerResultToRecord converts a WorkerResult to a PerEvalRecord.
func WorkerResultToRecord(wr WorkerResult) PerEvalRecord {
	if wr.Compare != nil {
		cr := wr.Compare
		durMs := cr.WithPrompt.DurationMs + cr.WithoutPrompt.DurationMs
		class := string(cr.Classification)
		if cr.Err != nil {
			class = "error"
		}
		return PerEvalRecord{
			EvalID:         cr.EvalID,
			TestsID:        cr.TestsID,
			Model:          cr.Model,
			RanAt:          cr.RanAt,
			Mode:           "compare",
			DurationMs:     durMs,
			Classification: class,
		}
	}
	r := wr.Result
	status := "pass"
	var failReason string
	if r.Err != nil {
		status = "error"
		failReason = "error: " + r.Err.Error()
	} else if !r.Passed {
		status = "fail"
		for _, a := range r.Assertions {
			if !a.Passed {
				failReason = fmt.Sprintf("%s %q - failed", a.Type, a.Value)
				break
			}
		}
	}
	return PerEvalRecord{
		EvalID:        r.EvalID,
		TestsID:       r.TestsID,
		Model:         r.Model,
		RanAt:         r.RanAt,
		Mode:          "single",
		DurationMs:    r.DurationMs,
		Status:        status,
		FailureReason: failReason,
	}
}

// CompileSummary aggregates PerEvalRecords into a Summary.
// wallElapsed is the actual wall-clock duration; pass 0 if unknown (e.g. compile-summary from disk).
// ranAt is the suite run start time (wallStart in suite mode; parsed timestamp in compile-summary).
// model is the model name; pass cfg.DefaultModel or the model from the first record.
// Results in the returned Summary are sorted by EvalID.
func CompileSummary(records []PerEvalRecord, wallElapsed time.Duration, ranAt time.Time, model string) Summary {
	s := Summary{
		RanAt:                ranAt.UTC().Format(time.RFC3339),
		Model:                model,
		TotalEvals:           len(records),
		TotalDurationSeconds: int64(wallElapsed.Seconds()),
		Results:              []SummaryResult{},
	}

	if len(records) == 0 {
		return s
	}

	s.Mode = records[0].Mode
	results := make([]SummaryResult, len(records))
	var totalEvalMs int64

	if s.Mode == "compare" {
		counts := ClassificationCounts{}
		for i, rec := range records {
			totalEvalMs += rec.DurationMs
			results[i] = SummaryResult{
				EvalID:         rec.EvalID,
				Tests:          rec.TestsID,
				DurationMs:     rec.DurationMs,
				Classification: rec.Classification,
			}
			switch rec.Classification {
			case "load-bearing":
				counts.LoadBearing++
			case "obsolete":
				counts.Obsolete++
			case "insufficient":
				counts.Insufficient++
			case "harmful":
				counts.Harmful++
			case "error":
				counts.Error++
			}
		}
		s.Classifications = &counts
	} else {
		for i, rec := range records {
			totalEvalMs += rec.DurationMs
			results[i] = SummaryResult{
				EvalID:        rec.EvalID,
				Tests:         rec.TestsID,
				DurationMs:    rec.DurationMs,
				Status:        rec.Status,
				FailureReason: rec.FailureReason,
			}
			if rec.Status == "pass" {
				s.TotalPassed++
			} else {
				s.TotalFailed++
			}
		}
	}

	s.TotalEvalTimeSeconds = totalEvalMs / 1000
	sort.Slice(results, func(i, j int) bool {
		return results[i].EvalID < results[j].EvalID
	})
	s.Results = results

	return s
}

// WriteSummary marshals s to summariesDir/{timestamp}.yml.
// Creates summariesDir if it does not exist.
// Returns the path written.
func WriteSummary(summariesDir string, ts time.Time, s Summary) (string, error) {
	if err := os.MkdirAll(summariesDir, 0755); err != nil {
		return "", fmt.Errorf("creating summaries dir %q: %w", summariesDir, err)
	}
	stamp := ts.UTC().Format(TimestampFormat)
	path := filepath.Join(summariesDir, stamp+".yml")
	return path, marshalAndWrite(path, s)
}

// WriteAggregateSummary writes a summary spanning a time range to
// summariesDir/aggregate-{since}-to-{until}.yml.
// Returns the path written.
func WriteAggregateSummary(summariesDir, since, until string, s Summary) (string, error) {
	if err := os.MkdirAll(summariesDir, 0755); err != nil {
		return "", fmt.Errorf("creating summaries dir %q: %w", summariesDir, err)
	}
	filename := fmt.Sprintf("aggregate-%s-to-%s.yml", since, until)
	path := filepath.Join(summariesDir, filename)
	return path, marshalAndWrite(path, s)
}

func marshalAndWrite(path string, s Summary) error {
	var data []byte
	var err error
	if s.Mode == "compare" {
		counts := ClassificationCounts{}
		if s.Classifications != nil {
			counts = *s.Classifications
		}
		yr := compareSummaryYML{
			RanAt:                s.RanAt,
			Model:                s.Model,
			Mode:                 s.Mode,
			TotalEvals:           s.TotalEvals,
			TotalDurationSeconds: s.TotalDurationSeconds,
			TotalEvalTimeSeconds: s.TotalEvalTimeSeconds,
			Classifications:      counts,
			Results:              s.Results,
		}
		data, err = yaml.Marshal(yr)
	} else {
		yr := singleSummaryYML{
			RanAt:                s.RanAt,
			Model:                s.Model,
			Mode:                 s.Mode,
			TotalEvals:           s.TotalEvals,
			TotalDurationSeconds: s.TotalDurationSeconds,
			TotalEvalTimeSeconds: s.TotalEvalTimeSeconds,
			TotalPassed:          s.TotalPassed,
			TotalFailed:          s.TotalFailed,
			Results:              s.Results,
		}
		data, err = yaml.Marshal(yr)
	}
	if err != nil {
		return fmt.Errorf("marshaling summary: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// --- YAML serialization types ---

type singleSummaryYML struct {
	RanAt                string          `yaml:"ran_at"`
	Model                string          `yaml:"model"`
	Mode                 string          `yaml:"mode"`
	TotalEvals           int             `yaml:"total_evals"`
	TotalDurationSeconds int64           `yaml:"total_duration_seconds"`
	TotalEvalTimeSeconds int64           `yaml:"total_eval_time_seconds"`
	TotalPassed          int             `yaml:"total_passed"`
	TotalFailed          int             `yaml:"total_failed"`
	Results              []SummaryResult `yaml:"results"`
}

type compareSummaryYML struct {
	RanAt                string               `yaml:"ran_at"`
	Model                string               `yaml:"model"`
	Mode                 string               `yaml:"mode"`
	TotalEvals           int                  `yaml:"total_evals"`
	TotalDurationSeconds int64                `yaml:"total_duration_seconds"`
	TotalEvalTimeSeconds int64                `yaml:"total_eval_time_seconds"`
	Classifications      ClassificationCounts `yaml:"classifications"`
	Results              []SummaryResult      `yaml:"results"`
}
