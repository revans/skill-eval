package skilleval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildArtifactPaths(t *testing.T) {
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	paths := BuildArtifactPaths("evals/results", "RU-001", "EV-001", ts)

	wantDir := filepath.Join("evals/results", "RU-001", "EV-001")
	if paths.Dir != wantDir {
		t.Errorf("Dir: got %q, want %q", paths.Dir, wantDir)
	}
	if !strings.HasSuffix(paths.WithPromptMD, "2026-05-02-T14-23-with-prompt.md") {
		t.Errorf("WithPromptMD: got %q, want suffix 2026-05-02-T14-23-with-prompt.md", paths.WithPromptMD)
	}
	if !strings.HasSuffix(paths.ResultYML, "2026-05-02-T14-23-result.yml") {
		t.Errorf("ResultYML: got %q, want suffix 2026-05-02-T14-23-result.yml", paths.ResultYML)
	}
}

func TestPrepareEvalDir_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "nested", "dir")
	if err := PrepareEvalDir(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory was not created")
	}
}

func TestPrepareEvalDir_WipesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	evalDir := filepath.Join(dir, "EV-001")
	if err := os.MkdirAll(evalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "old-result.yml"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "old-output.md"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := PrepareEvalDir(evalDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(evalDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("file %q was not deleted", e.Name())
		}
	}
}

func TestPrepareEvalDir_PreservesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	evalDir := filepath.Join(dir, "EV-001")
	subDir := filepath.Join(evalDir, "claude-haiku-4-5")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "flat-file.md"), []byte("flat"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := PrepareEvalDir(evalDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Subdirectory should survive.
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Error("subdirectory was incorrectly deleted")
	}
	// Flat file should be gone.
	if _, err := os.Stat(filepath.Join(evalDir, "flat-file.md")); !os.IsNotExist(err) {
		t.Error("flat file should have been deleted")
	}
}

func TestPrepareEvalDir_LeavesOtherDirsUntouched(t *testing.T) {
	dir := t.TempDir()
	evalDir := filepath.Join(dir, "EV-001")
	otherDir := filepath.Join(dir, "EV-002")
	keepFile := filepath.Join(otherDir, "keep.md")

	if err := os.MkdirAll(evalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keepFile, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "wipe.md"), []byte("wipe"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := PrepareEvalDir(evalDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(keepFile); os.IsNotExist(err) {
		t.Error("other eval's file was incorrectly deleted")
	}
}

func TestWriteWithPromptMD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.md")
	if err := WriteWithPromptMD(path, "model output here"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "model output here" {
		t.Errorf("got %q", string(data))
	}
}

func TestWriteResultYML_InlinePrompt(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	paths := BuildArtifactPaths(dir, "RU-001", "EV-001", ts)
	if err := os.MkdirAll(paths.Dir, 0755); err != nil {
		t.Fatal(err)
	}

	r := EvalResult{
		EvalID:     "EV-001",
		TestsID:    "RU-001",
		Model:      "claude-sonnet-4-6",
		RanAt:      ts,
		Prompt:     "Use params.expect.",
		Input:      "Write a create action.",
		Output:     "Here is code with params.expect",
		DurationMs: 2341,
		Assertions: []AssertionResult{
			{Type: "contains", Value: "params.expect", Passed: true},
		},
		Passed: true,
	}

	if err := WriteResultYML(paths, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(paths.ResultYML)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	checks := []string{"EV-001", "RU-001", "single", "claude-sonnet-4-6", "inline", "pass", "contains"}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("result YAML missing %q; content:\n%s", want, content)
		}
	}
	if strings.Contains(content, "prompt_file") {
		t.Error("prompt_file should not appear for inline prompt")
	}
}

func TestWriteResultYML_FilePrompt(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	paths := BuildArtifactPaths(dir, "RU-001", "EV-001", ts)
	if err := os.MkdirAll(paths.Dir, 0755); err != nil {
		t.Fatal(err)
	}

	r := EvalResult{
		EvalID:     "EV-001",
		TestsID:    "RU-001",
		Model:      "claude-sonnet-4-6",
		RanAt:      ts,
		PromptFile: "substrate/rules/RU-001.md",
		Input:      "Write a create action.",
		Output:     "Here is code with params.expect",
		DurationMs: 2341,
		Assertions: []AssertionResult{
			{Type: "contains", Value: "params.expect", Passed: true},
		},
		Passed: true,
	}

	if err := WriteResultYML(paths, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(paths.ResultYML)
	content := string(data)

	if !strings.Contains(content, "prompt_source: file") {
		t.Errorf("expected prompt_source: file; content:\n%s", content)
	}
	if !strings.Contains(content, "prompt_file: substrate/rules/RU-001.md") {
		t.Errorf("expected prompt_file; content:\n%s", content)
	}
}

func TestWriteResultYML_FailStatus(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC()
	paths := BuildArtifactPaths(dir, "RU-001", "EV-001", ts)
	if err := os.MkdirAll(paths.Dir, 0755); err != nil {
		t.Fatal(err)
	}

	r := EvalResult{
		EvalID:  "EV-001",
		TestsID: "RU-001",
		Model:   "claude-sonnet-4-6",
		RanAt:   ts,
		Prompt:  "p",
		Input:   "i",
		Output:  "no match here",
		Assertions: []AssertionResult{
			{Type: "contains", Value: "params.expect", Passed: false},
		},
		Passed: false,
	}

	if err := WriteResultYML(paths, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(paths.ResultYML)
	content := string(data)
	if !strings.Contains(content, "status: fail") {
		t.Errorf("expected status: fail; content:\n%s", content)
	}
}
