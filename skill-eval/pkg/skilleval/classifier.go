package skilleval

// Classification is the result of a compare-mode eval.
// It describes the relationship between the prompt's presence and the model's
// ability to satisfy the eval's assertions.
type Classification string

const (
	// LoadBearing: passed with prompt, failed without. The prompt is doing work.
	LoadBearing Classification = "load-bearing"
	// Obsolete: passed both with and without prompt. The model no longer needs the prompt.
	Obsolete Classification = "obsolete"
	// Insufficient: failed both with and without prompt. The prompt, eval, or task needs investigation.
	Insufficient Classification = "insufficient"
	// Harmful: failed with prompt, passed without. The prompt is degrading model output.
	Harmful Classification = "harmful"
)

// Classify derives a classification from the two pass/fail outcomes.
// It is a pure function with four possible inputs and four possible outputs.
func Classify(withPromptPassed, withoutPromptPassed bool) Classification {
	switch {
	case withPromptPassed && !withoutPromptPassed:
		return LoadBearing
	case withPromptPassed && withoutPromptPassed:
		return Obsolete
	case !withPromptPassed && !withoutPromptPassed:
		return Insufficient
	default: // !withPromptPassed && withoutPromptPassed
		return Harmful
	}
}
