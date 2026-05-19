package skilleval

import (
	"fmt"
	"os"
)

// ConstructWithoutPrompt returns the bare input — the control condition for compare
// mode. The model sees only the task with no prompt context.
func ConstructWithoutPrompt(eval Eval) string {
	return eval.Input
}

// ConstructWithPrompt builds the with-prompt invocation text for an eval.
// The result is {prompt}\n\n{input} — a blank line separates the prompt fragment
// from the input task. If prompt_file is set, the file contents are used as the prompt.
func ConstructWithPrompt(eval Eval) (string, error) {
	promptText := eval.Prompt
	if eval.PromptFile != "" {
		data, err := os.ReadFile(eval.PromptFile)
		if err != nil {
			return "", fmt.Errorf("reading prompt file %q: %w", eval.PromptFile, err)
		}
		promptText = string(data)
	}
	return promptText + "\n\n" + eval.Input, nil
}
