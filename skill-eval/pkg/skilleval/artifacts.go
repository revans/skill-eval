package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// TimestampFormat is the filesystem-safe timestamp format used in artifact filenames.
// Colons are replaced with hyphens to be safe on all operating systems.
const TimestampFormat = "2006-01-02-T15-04"

// ArtifactPaths holds the resolved paths for a single eval's output files.
// WithoutPromptMD is always constructed; it is only written in compare mode.
type ArtifactPaths struct {
	Dir             string
	WithPromptMD    string
	WithoutPromptMD string
	ResultYML       string
}

// BuildArtifactPaths constructs artifact paths for an eval at the given timestamp.
func BuildArtifactPaths(resultsDir, testsID, evalID string, ts time.Time) ArtifactPaths {
	dir := filepath.Join(resultsDir, testsID, evalID)
	stamp := ts.UTC().Format(TimestampFormat)
	return ArtifactPaths{
		Dir:             dir,
		WithPromptMD:    filepath.Join(dir, stamp+"-with-prompt.md"),
		WithoutPromptMD: filepath.Join(dir, stamp+"-without-prompt.md"),
		ResultYML:       filepath.Join(dir, stamp+"-result.yml"),
	}
}

// BuildModelArtifactPaths constructs per-model artifact paths for multi-model targeted runs.
// The model name is inserted as a subdirectory under the eval directory.
func BuildModelArtifactPaths(resultsDir, testsID, evalID, model string, ts time.Time) ArtifactPaths {
	dir := filepath.Join(resultsDir, testsID, evalID, model)
	stamp := ts.UTC().Format(TimestampFormat)
	return ArtifactPaths{
		Dir:             dir,
		WithPromptMD:    filepath.Join(dir, stamp+"-with-prompt.md"),
		WithoutPromptMD: filepath.Join(dir, stamp+"-without-prompt.md"),
		ResultYML:       filepath.Join(dir, stamp+"-result.yml"),
	}
}

// EvalDirHasSubdirs reports whether dir contains any subdirectories.
// Returns false if the directory does not exist.
func EvalDirHasSubdirs(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading eval dir %q: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

// PrepareEvalDirForModel prepares the eval directory for a per-model targeted run:
// 1. Creates the eval directory if absent.
// 2. Wipes flat (non-directory) files at the eval level — removes artifacts from
//    any previous single-model run while leaving other model subdirs untouched.
// 3. Creates and wipes the model-specific subdirectory.
func PrepareEvalDirForModel(evalDir, model string) error {
	if err := os.MkdirAll(evalDir, 0755); err != nil {
		return fmt.Errorf("creating eval dir %q: %w", evalDir, err)
	}
	entries, err := os.ReadDir(evalDir)
	if err != nil {
		return fmt.Errorf("reading eval dir %q: %w", evalDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			_ = os.Remove(filepath.Join(evalDir, e.Name()))
		}
	}
	return PrepareEvalDir(filepath.Join(evalDir, model))
}

// PrepareEvalDir creates the eval directory if absent and wipes existing files.
// Subdirectories (from previous multi-model runs) are left untouched.
func PrepareEvalDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating eval dir %q: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading eval dir %q: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("removing %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// WriteWithPromptMD writes raw model output to the with-prompt markdown artifact.
func WriteWithPromptMD(path, output string) error {
	return os.WriteFile(path, []byte(output), 0644)
}

// WriteWithoutPromptMD writes raw model output to the without-prompt markdown artifact.
func WriteWithoutPromptMD(path, output string) error {
	return os.WriteFile(path, []byte(output), 0644)
}

// WriteResultYML writes the structured per-eval result YAML (single-mode form).
func WriteResultYML(paths ArtifactPaths, r EvalResult) error {
	status := "pass"
	if r.Err != nil {
		status = "error"
	} else if !r.Passed {
		status = "fail"
	}

	assertResults := marshalAssertions(r.Assertions)

	outputFile := ""
	if r.Err == nil {
		outputFile = filepath.Base(paths.WithPromptMD)
	}

	yr := resultYML{
		EvalID: r.EvalID,
		Tests:  r.TestsID,
		RanAt:  r.RanAt.Format(time.RFC3339),
		Mode:   "single",
		Model:  r.Model,
		Input:  r.Input,
		WithPrompt: runBlock{
			OutputFile: outputFile,
			DurationMs: r.DurationMs,
			Assertions: assertResults,
			Status:     status,
		},
	}

	if r.PromptFile != "" {
		yr.PromptSource = "file"
		yr.PromptFile = r.PromptFile
	} else {
		yr.PromptSource = "inline"
		yr.Prompt = r.Prompt
	}

	data, err := yaml.Marshal(yr)
	if err != nil {
		return fmt.Errorf("marshaling result YAML: %w", err)
	}
	return os.WriteFile(paths.ResultYML, data, 0644)
}

// WriteCompareResultYML writes the extended per-eval result YAML for compare mode.
// Both with-prompt and without-prompt sections are included, plus the classification.
func WriteCompareResultYML(paths ArtifactPaths, cr CompareResult) error {
	withStatus := runStatus(cr.WithPrompt)
	withoutStatus := runStatus(cr.WithoutPrompt)

	withOutputFile := ""
	if cr.WithPrompt.Err == nil {
		withOutputFile = filepath.Base(paths.WithPromptMD)
	}
	withoutOutputFile := ""
	if cr.WithoutPrompt.Err == nil {
		withoutOutputFile = filepath.Base(paths.WithoutPromptMD)
	}

	yr := compareResultYML{
		EvalID: cr.EvalID,
		Tests:  cr.TestsID,
		RanAt:  cr.RanAt.Format(time.RFC3339),
		Mode:   "compare",
		Model:  cr.Model,
		Input:  cr.Input,
		WithPrompt: runBlock{
			OutputFile: withOutputFile,
			DurationMs: cr.WithPrompt.DurationMs,
			Assertions: marshalAssertions(cr.WithPrompt.Assertions),
			Status:     withStatus,
		},
		WithoutPrompt: runBlock{
			OutputFile: withoutOutputFile,
			DurationMs: cr.WithoutPrompt.DurationMs,
			Assertions: marshalAssertions(cr.WithoutPrompt.Assertions),
			Status:     withoutStatus,
		},
	}

	if cr.PromptFile != "" {
		yr.PromptSource = "file"
		yr.PromptFile = cr.PromptFile
	} else {
		yr.PromptSource = "inline"
		yr.Prompt = cr.Prompt
	}

	// Classification is omitted when either run errored.
	if cr.Err == nil {
		yr.Classification = string(cr.Classification)
	}

	data, err := yaml.Marshal(yr)
	if err != nil {
		return fmt.Errorf("marshaling compare result YAML: %w", err)
	}
	return os.WriteFile(paths.ResultYML, data, 0644)
}

func runStatus(r EvalResult) string {
	if r.Err != nil {
		return "error"
	}
	if r.Passed {
		return "pass"
	}
	return "fail"
}

func marshalAssertions(results []AssertionResult) []assertionResultYML {
	out := make([]assertionResultYML, len(results))
	for i, a := range results {
		res := "pass"
		if !a.Passed {
			res = "fail"
		}
		out[i] = assertionResultYML{Type: a.Type, Value: a.Value, Result: res}
	}
	return out
}

// --- YAML serialization types ---

type resultYML struct {
	EvalID       string   `yaml:"eval_id"`
	Tests        string   `yaml:"tests"`
	RanAt        string   `yaml:"ran_at"`
	Mode         string   `yaml:"mode"`
	Model        string   `yaml:"model"`
	PromptSource string   `yaml:"prompt_source"`
	PromptFile   string   `yaml:"prompt_file,omitempty"`
	Prompt       string   `yaml:"prompt,omitempty"`
	Input        string   `yaml:"input"`
	WithPrompt   runBlock `yaml:"with_prompt"`
}

type compareResultYML struct {
	EvalID         string   `yaml:"eval_id"`
	Tests          string   `yaml:"tests"`
	RanAt          string   `yaml:"ran_at"`
	Mode           string   `yaml:"mode"`
	Model          string   `yaml:"model"`
	PromptSource   string   `yaml:"prompt_source"`
	PromptFile     string   `yaml:"prompt_file,omitempty"`
	Prompt         string   `yaml:"prompt,omitempty"`
	Input          string   `yaml:"input"`
	WithPrompt     runBlock `yaml:"with_prompt"`
	WithoutPrompt  runBlock `yaml:"without_prompt"`
	Classification string   `yaml:"classification,omitempty"`
}

type runBlock struct {
	OutputFile string               `yaml:"output_file"`
	DurationMs int64                `yaml:"duration_ms"`
	Assertions []assertionResultYML `yaml:"assertions"`
	Status     string               `yaml:"status"`
}

type assertionResultYML struct {
	Type   string `yaml:"type"`
	Value  string `yaml:"value"`
	Result string `yaml:"result"`
}
