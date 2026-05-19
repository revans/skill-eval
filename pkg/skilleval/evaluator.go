package skilleval

import (
	"fmt"
	"time"
)

// Evaluator runs evals against Claude using the configured model.
// It is the core unit of work; compare mode and multi-model wrapping build on it.
type Evaluator struct {
	Config Config
}

// EvalResult holds the complete outcome of running a single eval.
type EvalResult struct {
	EvalID     string
	TestsID    string
	Model      string
	RanAt      time.Time
	Prompt     string
	PromptFile string
	Input      string
	Output     string
	DurationMs int64
	Assertions []AssertionResult
	Passed     bool
	Err        error
}

// CompareResult holds the outcome of a compare-mode eval: both runs and a classification.
type CompareResult struct {
	EvalID         string
	TestsID        string
	Model          string
	RanAt          time.Time
	Prompt         string
	PromptFile     string
	Input          string
	WithPrompt     EvalResult
	WithoutPrompt  EvalResult
	Classification Classification
	Err            error // non-nil if either run errored; Classification is not set
}

// Run executes a single eval with the prompt loaded and returns the result.
func (e *Evaluator) Run(eval Eval) EvalResult {
	text, err := ConstructWithPrompt(eval)
	if err != nil {
		return EvalResult{
			EvalID:     eval.ID,
			TestsID:    eval.Tests,
			Model:      e.Config.DefaultModel,
			RanAt:      time.Now().UTC(),
			Prompt:     eval.Prompt,
			PromptFile: eval.PromptFile,
			Input:      eval.Input,
			Err:        fmt.Errorf("constructing prompt: %w", err),
		}
	}
	return e.runText(eval, text)
}

// RunCompare executes the eval twice — with and without the prompt — and classifies.
// The two runs are sequential within the caller's goroutine. Both use the same model
// and have independent timeouts and retry logic.
func (e *Evaluator) RunCompare(eval Eval) CompareResult {
	cr := CompareResult{
		EvalID:     eval.ID,
		TestsID:    eval.Tests,
		Model:      e.Config.DefaultModel,
		RanAt:      time.Now().UTC(),
		Prompt:     eval.Prompt,
		PromptFile: eval.PromptFile,
		Input:      eval.Input,
	}

	withText, err := ConstructWithPrompt(eval)
	if err != nil {
		cr.Err = fmt.Errorf("constructing with-prompt text: %w", err)
		return cr
	}

	cr.WithPrompt = e.runText(eval, withText)
	cr.WithoutPrompt = e.runText(eval, ConstructWithoutPrompt(eval))

	if cr.WithPrompt.Err != nil {
		cr.Err = fmt.Errorf("with-prompt run: %w", cr.WithPrompt.Err)
	} else if cr.WithoutPrompt.Err != nil {
		cr.Err = fmt.Errorf("without-prompt run: %w", cr.WithoutPrompt.Err)
	} else {
		cr.Classification = Classify(cr.WithPrompt.Passed, cr.WithoutPrompt.Passed)
	}

	return cr
}

// RunCompareWithModel executes a compare run using the specified model instead of the
// config default. Creates a temporary evaluator with the overridden model so the
// original config is unchanged.
func (e *Evaluator) RunCompareWithModel(eval Eval, model string) CompareResult {
	cfgCopy := e.Config
	cfgCopy.DefaultModel = model
	return (&Evaluator{Config: cfgCopy}).RunCompare(eval)
}

// runText invokes Claude with the given pre-constructed text and runs assertions.
// It is shared by Run and RunCompare to avoid duplicating the subprocess and assertion logic.
func (e *Evaluator) runText(eval Eval, text string) EvalResult {
	result := EvalResult{
		EvalID:     eval.ID,
		TestsID:    eval.Tests,
		Model:      e.Config.DefaultModel,
		RanAt:      time.Now().UTC(),
		Prompt:     eval.Prompt,
		PromptFile: eval.PromptFile,
		Input:      eval.Input,
	}

	cr, err := RunClaude(text, e.Config.DefaultModel, e.Config.PerEvalTimeoutSeconds)
	if err != nil {
		result.Err = err
		return result
	}

	result.Output = cr.Output
	result.DurationMs = cr.DurationMs

	assertions, passed := RunAssertions(eval.Assert, cr.Output)
	result.Assertions = assertions
	result.Passed = passed

	return result
}
