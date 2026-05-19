package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScanTestFiles creates a temp directory tree with the given relative
// paths and returns the root dir.
func writeScanTestFiles(t *testing.T, paths []string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("# prompt"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// --- discoverFiles ---

func TestDiscoverFiles_Dir_FindsMD(t *testing.T) {
	root := writeScanTestFiles(t, []string{
		"a.md",
		"sub/b.md",
		"sub/deep/c.md",
		"skip.txt",
		"skip.go",
	})
	files, err := discoverFiles(root, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 .md files, got %d: %v", len(files), files)
	}
}

func TestDiscoverFiles_Dir_Sorted(t *testing.T) {
	root := writeScanTestFiles(t, []string{"c.md", "a.md", "b.md"})
	files, err := discoverFiles(root, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 1; i < len(files); i++ {
		if files[i] < files[i-1] {
			t.Errorf("files not sorted: %v", files)
		}
	}
}

func TestDiscoverFiles_Dir_Empty(t *testing.T) {
	root := t.TempDir()
	files, err := discoverFiles(root, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestDiscoverFiles_Dir_NonExistent(t *testing.T) {
	_, err := discoverFiles("/nonexistent/path/xyz", "")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestDiscoverFiles_Glob_Matches(t *testing.T) {
	root := writeScanTestFiles(t, []string{"rules/RU-001.md", "rules/RU-002.md", "other/PA-001.md"})
	pattern := filepath.Join(root, "rules", "*.md")
	files, err := discoverFiles("", pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files from glob, got %d: %v", len(files), files)
	}
}

func TestDiscoverFiles_Glob_NoMatches(t *testing.T) {
	root := writeScanTestFiles(t, []string{"a.md"})
	pattern := filepath.Join(root, "nonexistent", "*.md")
	files, err := discoverFiles("", pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestDiscoverFiles_BothDirAndGlob_Deduplicates(t *testing.T) {
	root := writeScanTestFiles(t, []string{"a.md", "b.md"})
	// Glob matches the same files the dir walk finds.
	pattern := filepath.Join(root, "*.md")
	files, err := discoverFiles(root, pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be 2, not 4.
	if len(files) != 2 {
		t.Errorf("expected 2 deduplicated files, got %d: %v", len(files), files)
	}
}

// --- RunScan ---

func makeScanTestDir(t *testing.T, evalsContent *string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	evalsPath := filepath.Join(dir, "evals.yml")
	if evalsContent != nil {
		if err := os.WriteFile(evalsPath, []byte(*evalsContent), 0644); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(dir, ".skill-eval.yml")
	cfg := fmt.Sprintf("default_model: test-model\nevals_file: %s\n", evalsPath)
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, evalsPath, configPath
}

func TestRunScan_MissingDirAndGlob(t *testing.T) {
	_, _, configPath := makeScanTestDir(t, nil)
	code := RunScan([]string{"--config", configPath})
	if code != 2 {
		t.Errorf("expected exit code 2 for missing --dir/--glob, got %d", code)
	}
}

func TestRunScan_Dir_CreatesEntries(t *testing.T) {
	_, evalsPath, configPath := makeScanTestDir(t, nil)

	promptDir := t.TempDir()
	for _, name := range []string{"my-prompt.md", "code-review.md"} {
		os.WriteFile(filepath.Join(promptDir, name), []byte("# prompt"), 0644)
	}

	code := RunScan([]string{"--config", configPath, "--dir", promptDir})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, _ := os.ReadFile(evalsPath)
	content := string(data)

	if !strings.Contains(content, "tests: my-prompt") {
		t.Errorf("expected tests: my-prompt: %s", content)
	}
	if !strings.Contains(content, "tests: code-review") {
		t.Errorf("expected tests: code-review: %s", content)
	}
	if !strings.Contains(content, "id: EV-001") {
		t.Errorf("expected EV-001: %s", content)
	}
	if !strings.Contains(content, "id: EV-002") {
		t.Errorf("expected EV-002: %s", content)
	}
}

func TestRunScan_SkipsAlreadyCovered(t *testing.T) {
	existing := "- id: EV-001\n  tests: my-prompt\n  prompt: p\n  input: i\n  assert:\n    - contains: foo\n"
	_, evalsPath, configPath := makeScanTestDir(t, &existing)

	promptDir := t.TempDir()
	os.WriteFile(filepath.Join(promptDir, "my-prompt.md"), []byte("# prompt"), 0644)
	os.WriteFile(filepath.Join(promptDir, "new-prompt.md"), []byte("# prompt"), 0644)

	code := RunScan([]string{"--config", configPath, "--dir", promptDir})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, _ := os.ReadFile(evalsPath)
	content := string(data)

	if strings.Count(content, "tests: my-prompt") != 1 {
		t.Errorf("my-prompt should appear exactly once (not duplicated): %s", content)
	}
	if !strings.Contains(content, "tests: new-prompt") {
		t.Errorf("expected new-prompt to be added: %s", content)
	}
}

func TestRunScan_DryRun_NoChanges(t *testing.T) {
	_, evalsPath, configPath := makeScanTestDir(t, nil)

	promptDir := t.TempDir()
	os.WriteFile(filepath.Join(promptDir, "my-prompt.md"), []byte("# prompt"), 0644)

	code := RunScan([]string{"--config", configPath, "--dir", promptDir, "--dry-run"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	if _, err := os.Stat(evalsPath); err == nil {
		t.Error("dry-run should not create evals.yml")
	}
}

func TestRunScan_NoFiles_ExitsZero(t *testing.T) {
	_, _, configPath := makeScanTestDir(t, nil)
	emptyDir := t.TempDir()

	code := RunScan([]string{"--config", configPath, "--dir", emptyDir})
	if code != 0 {
		t.Errorf("expected exit code 0 for empty dir, got %d", code)
	}
}

func TestRunScan_AllAlreadyCovered_ExitsZero(t *testing.T) {
	existing := "- id: EV-001\n  tests: my-prompt\n  prompt: p\n  input: i\n  assert:\n    - contains: foo\n"
	_, _, configPath := makeScanTestDir(t, &existing)

	promptDir := t.TempDir()
	os.WriteFile(filepath.Join(promptDir, "my-prompt.md"), []byte("# prompt"), 0644)

	code := RunScan([]string{"--config", configPath, "--dir", promptDir})
	if code != 0 {
		t.Errorf("expected exit code 0 when all covered, got %d", code)
	}
}

func TestRunScan_Glob_CreatesEntries(t *testing.T) {
	_, evalsPath, configPath := makeScanTestDir(t, nil)

	promptDir := t.TempDir()
	os.WriteFile(filepath.Join(promptDir, "RU-001-params.md"), []byte("# prompt"), 0644)
	os.WriteFile(filepath.Join(promptDir, "RU-002-naming.md"), []byte("# prompt"), 0644)

	pattern := filepath.Join(promptDir, "RU-*.md")
	code := RunScan([]string{"--config", configPath, "--glob", pattern})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, _ := os.ReadFile(evalsPath)
	content := string(data)

	if !strings.Contains(content, "tests: RU-001") {
		t.Errorf("expected tests: RU-001: %s", content)
	}
	if !strings.Contains(content, "tests: RU-002") {
		t.Errorf("expected tests: RU-002: %s", content)
	}
}

func TestRunScan_IDSequenceContinuesFromExisting(t *testing.T) {
	existing := "- id: EV-005\n  tests: other\n  prompt: p\n  input: i\n  assert:\n    - contains: foo\n"
	_, evalsPath, configPath := makeScanTestDir(t, &existing)

	promptDir := t.TempDir()
	os.WriteFile(filepath.Join(promptDir, "new.md"), []byte("# prompt"), 0644)

	code := RunScan([]string{"--config", configPath, "--dir", promptDir})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, _ := os.ReadFile(evalsPath)
	if !strings.Contains(string(data), "id: EV-006") {
		t.Errorf("expected EV-006 to continue from existing EV-005: %s", string(data))
	}
}
