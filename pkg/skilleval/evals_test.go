package skilleval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeEvalsFile is a test helper that writes YAML content to a temp file.
func writeEvalsFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadEvals_Valid_InlinePrompt(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "Use params.expect."
  input: "Write a create action."
  assert:
    - contains: "params.expect"
    - not_contains: "params.permit"
`)
	evals, err := LoadEvals(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("expected 1 eval, got %d", len(evals))
	}
	e := evals[0]
	if e.ID != "EV-001" {
		t.Errorf("ID: got %q", e.ID)
	}
	if e.Tests != "RU-001" {
		t.Errorf("Tests: got %q", e.Tests)
	}
	if e.Prompt != "Use params.expect." {
		t.Errorf("Prompt: got %q", e.Prompt)
	}
	if e.PromptFile != "" {
		t.Errorf("PromptFile should be empty, got %q", e.PromptFile)
	}
	if len(e.Assert) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(e.Assert))
	}
	if e.Assert[0].Type != "contains" || e.Assert[0].Value != "params.expect" {
		t.Errorf("first assertion: got %+v", e.Assert[0])
	}
	if e.Assert[1].Type != "not_contains" || e.Assert[1].Value != "params.permit" {
		t.Errorf("second assertion: got %+v", e.Assert[1])
	}
}

func TestLoadEvals_Valid_PromptFile(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "rule.md")
	if err := os.WriteFile(promptFile, []byte("rule content"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "evals.yml")
	content := "- id: EV-001\n  tests: RU-001\n  prompt_file: " + promptFile + "\n  input: do it\n  assert:\n    - contains: foo\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	evals, err := LoadEvals(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evals[0].PromptFile != promptFile {
		t.Errorf("PromptFile: got %q", evals[0].PromptFile)
	}
}

func TestLoadEvals_Valid_ModelsBlock(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  models:
    primary: claude-sonnet-4-6
    secondaries:
      - claude-haiku-4-5
  assert:
    - contains: foo
`)
	evals, err := LoadEvals(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evals[0].Models == nil {
		t.Fatal("expected models block to be set")
	}
	if evals[0].Models.Primary != "claude-sonnet-4-6" {
		t.Errorf("Primary: got %q", evals[0].Models.Primary)
	}
	if len(evals[0].Models.Secondaries) != 1 || evals[0].Models.Secondaries[0] != "claude-haiku-4-5" {
		t.Errorf("Secondaries: got %v", evals[0].Models.Secondaries)
	}
}

func TestLoadEvals_Valid_NoModelsBlock(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-013
  prompt: "p"
  input: "i"
  assert:
    - matches: "foo+"
`)
	evals, err := LoadEvals(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evals[0].Models != nil {
		t.Error("expected models block to be nil")
	}
}

func TestLoadEvals_Valid_MultipleEvals(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p1"
  input: "i1"
  assert:
    - contains: foo
- id: EV-002
  tests: RU-002
  prompt: "p2"
  input: "i2"
  assert:
    - not_contains: bar
`)
	evals, err := LoadEvals(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evals) != 2 {
		t.Fatalf("expected 2 evals, got %d", len(evals))
	}
}

// --- Validation error cases ---

func TestLoadEvals_Error_BothPromptFields(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "inline"
  prompt_file: "some/file.md"
  input: "i"
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("error should mention 'both': %v", err)
	}
}

func TestLoadEvals_Error_NeitherPromptField(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  input: "i"
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
	if !strings.Contains(err.Error(), "neither") {
		t.Errorf("error should mention 'neither': %v", err)
	}
}

func TestLoadEvals_Error_MissingID(t *testing.T) {
	path := writeEvalsFile(t, `
- tests: RU-001
  prompt: "p"
  input: "i"
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestLoadEvals_Error_MissingTests(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  prompt: "p"
  input: "i"
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for missing tests")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
}

func TestLoadEvals_Error_MissingInput(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for missing input")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
}

func TestLoadEvals_Error_EmptyAssert(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  assert: []
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for empty assert list")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
}

func TestLoadEvals_Error_MissingAssert(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for missing assert")
	}
}

func TestLoadEvals_Error_UnknownAssertionType(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  assert:
    - unknown_type: "value"
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for unknown assertion type")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown_type") {
		t.Errorf("error should name the bad type: %v", err)
	}
}

func TestLoadEvals_Error_DuplicateID(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  assert:
    - contains: foo
- id: EV-001
  tests: RU-002
  prompt: "p"
  input: "i"
  assert:
    - contains: bar
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name the duplicate ID: %v", err)
	}
}

func TestLoadEvals_Error_MissingPromptFile(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt_file: "/nonexistent/path/rule.md"
  input: "i"
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for missing prompt_file")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
}

func TestLoadEvals_Error_ModelsWithoutPrimary(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  models:
    secondaries:
      - claude-haiku-4-5
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for models without primary")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
	if !strings.Contains(err.Error(), "primary") {
		t.Errorf("error should mention 'primary': %v", err)
	}
}

func TestLoadEvals_Error_EmptySecondaries(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  models:
    primary: claude-sonnet-4-6
    secondaries: []
  assert:
    - contains: foo
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for empty secondaries list")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
	if !strings.Contains(err.Error(), "secondaries") {
		t.Errorf("error should mention 'secondaries': %v", err)
	}
}

func TestLoadEvals_Error_InvalidRegex(t *testing.T) {
	path := writeEvalsFile(t, `
- id: EV-001
  tests: RU-001
  prompt: "p"
  input: "i"
  assert:
    - matches: "[invalid regex"
`)
	_, err := LoadEvals(path)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should name eval: %v", err)
	}
}

func TestLoadEvals_Error_MissingFile(t *testing.T) {
	_, err := LoadEvals("/nonexistent/evals.yml")
	if err == nil {
		t.Fatal("expected error for missing evals file")
	}
}
