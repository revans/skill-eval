package skilleval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConstructWithPrompt_Inline(t *testing.T) {
	eval := Eval{
		Prompt: "Use params.expect for mass assignment.",
		Input:  "Write a Rails controller create action.",
	}
	got, err := ConstructWithPrompt(eval)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Use params.expect for mass assignment.\n\nWrite a Rails controller create action."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConstructWithPrompt_File(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "rule.md")
	if err := os.WriteFile(promptFile, []byte("Always use strict mode."), 0644); err != nil {
		t.Fatal(err)
	}
	eval := Eval{
		PromptFile: promptFile,
		Input:      "Write a JavaScript function.",
	}
	got, err := ConstructWithPrompt(eval)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Always use strict mode.\n\nWrite a JavaScript function."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConstructWithPrompt_FileMultiline(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "rule.md")
	promptContent := "# Rule\n\nUse this pattern.\n\nExamples follow."
	if err := os.WriteFile(promptFile, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}
	eval := Eval{
		PromptFile: promptFile,
		Input:      "Do the task.",
	}
	got, err := ConstructWithPrompt(eval)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := promptContent + "\n\n" + "Do the task."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConstructWithPrompt_MissingFile(t *testing.T) {
	eval := Eval{
		PromptFile: "/nonexistent/path/rule.md",
		Input:      "Do the task.",
	}
	_, err := ConstructWithPrompt(eval)
	if err == nil {
		t.Error("expected error for missing prompt file")
	}
}

func TestConstructWithoutPrompt(t *testing.T) {
	eval := Eval{
		Prompt: "Some prompt that should be ignored.",
		Input:  "Write a Rails controller.",
	}
	got := ConstructWithoutPrompt(eval)
	if got != eval.Input {
		t.Errorf("ConstructWithoutPrompt = %q, want %q", got, eval.Input)
	}
}

func TestConstructWithoutPrompt_EmptyPrompt(t *testing.T) {
	eval := Eval{
		Input: "Only the input.",
	}
	got := ConstructWithoutPrompt(eval)
	if got != "Only the input." {
		t.Errorf("ConstructWithoutPrompt = %q", got)
	}
}
