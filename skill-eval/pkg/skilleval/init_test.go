package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ExtractSubstrateID ---

func TestExtractSubstrateID_SimplePrefix(t *testing.T) {
	if id := ExtractSubstrateID("RU-001-params-expect.md"); id != "RU-001" {
		t.Errorf("expected RU-001, got %q", id)
	}
}

func TestExtractSubstrateID_TwoLetterPrefix(t *testing.T) {
	if id := ExtractSubstrateID("PA-002-eager-loading.md"); id != "PA-002" {
		t.Errorf("expected PA-002, got %q", id)
	}
}

func TestExtractSubstrateID_MultiLetterPrefix(t *testing.T) {
	if id := ExtractSubstrateID("RULE-042-something.md"); id != "RULE-042" {
		t.Errorf("expected RULE-042, got %q", id)
	}
}

func TestExtractSubstrateID_NoDescription(t *testing.T) {
	if id := ExtractSubstrateID("RU-001.md"); id != "RU-001" {
		t.Errorf("expected RU-001, got %q", id)
	}
}

func TestExtractSubstrateID_ManyDescriptionSegments(t *testing.T) {
	if id := ExtractSubstrateID("RU-001-foo-bar-baz.md"); id != "RU-001" {
		t.Errorf("expected RU-001, got %q", id)
	}
}

func TestExtractSubstrateID_WithDirectory(t *testing.T) {
	if id := ExtractSubstrateID("prompts/CA-005-validation.md"); id != "CA-005" {
		t.Errorf("expected CA-005, got %q", id)
	}
}

func TestExtractSubstrateID_WithAbsolutePath(t *testing.T) {
	if id := ExtractSubstrateID("/some/deep/path/RU-099-thing.md"); id != "RU-099" {
		t.Errorf("expected RU-099, got %q", id)
	}
}

func TestExtractSubstrateID_HighNumber(t *testing.T) {
	if id := ExtractSubstrateID("RU-1234-something.md"); id != "RU-1234" {
		t.Errorf("expected RU-1234, got %q", id)
	}
}

// Files that don't match the PREFIX-NUMBER pattern fall back to the filename stem.

func TestExtractSubstrateID_ArbitraryName_UsesStem(t *testing.T) {
	if id := ExtractSubstrateID("foo/bar/baz.md"); id != "baz" {
		t.Errorf("expected stem %q, got %q", "baz", id)
	}
}

func TestExtractSubstrateID_LowercasePrefix_UsesStem(t *testing.T) {
	if id := ExtractSubstrateID("ru-001-something.md"); id != "ru-001-something" {
		t.Errorf("expected stem %q, got %q", "ru-001-something", id)
	}
}

func TestExtractSubstrateID_NoNumber_UsesStem(t *testing.T) {
	if id := ExtractSubstrateID("RU-abc-something.md"); id != "RU-abc-something" {
		t.Errorf("expected stem %q, got %q", "RU-abc-something", id)
	}
}

func TestExtractSubstrateID_NoDash_UsesStem(t *testing.T) {
	if id := ExtractSubstrateID("RU001-something.md"); id != "RU001-something" {
		t.Errorf("expected stem %q, got %q", "RU001-something", id)
	}
}

func TestExtractSubstrateID_HyphenatedName_UsesStem(t *testing.T) {
	if id := ExtractSubstrateID("my-code-review.md"); id != "my-code-review" {
		t.Errorf("expected stem %q, got %q", "my-code-review", id)
	}
}

// --- nextEvalID ---

func TestNextEvalID_NoEntries(t *testing.T) {
	id := nextEvalID(nil)
	if id != "EV-001" {
		t.Errorf("expected EV-001, got %q", id)
	}
}

func TestNextEvalID_EmptySlice(t *testing.T) {
	id := nextEvalID([]evalInitEntry{})
	if id != "EV-001" {
		t.Errorf("expected EV-001, got %q", id)
	}
}

func TestNextEvalID_OneEntry(t *testing.T) {
	entries := []evalInitEntry{{ID: "EV-001"}}
	id := nextEvalID(entries)
	if id != "EV-002" {
		t.Errorf("expected EV-002, got %q", id)
	}
}

func TestNextEvalID_ThreeDigitPadding(t *testing.T) {
	entries := []evalInitEntry{{ID: "EV-009"}}
	id := nextEvalID(entries)
	if id != "EV-010" {
		t.Errorf("expected EV-010, got %q", id)
	}
}

func TestNextEvalID_MaxFromMany(t *testing.T) {
	entries := []evalInitEntry{
		{ID: "EV-001"},
		{ID: "EV-005"},
		{ID: "EV-003"},
	}
	id := nextEvalID(entries)
	if id != "EV-006" {
		t.Errorf("expected EV-006, got %q", id)
	}
}

func TestNextEvalID_GapsUseMax(t *testing.T) {
	// Gaps are not gap-filled; we use max+1.
	entries := []evalInitEntry{
		{ID: "EV-001"},
		{ID: "EV-003"},
		{ID: "EV-009"},
	}
	id := nextEvalID(entries)
	if id != "EV-010" {
		t.Errorf("expected EV-010 (max+1), got %q", id)
	}
}

func TestNextEvalID_HighNumber(t *testing.T) {
	entries := []evalInitEntry{{ID: "EV-100"}}
	id := nextEvalID(entries)
	if id != "EV-101" {
		t.Errorf("expected EV-101, got %q", id)
	}
}

func TestNextEvalID_VeryHighNumber(t *testing.T) {
	entries := []evalInitEntry{{ID: "EV-999"}}
	id := nextEvalID(entries)
	if id != "EV-1000" {
		t.Errorf("expected EV-1000, got %q", id)
	}
}

func TestNextEvalID_IgnoresNonEVIDs(t *testing.T) {
	entries := []evalInitEntry{
		{ID: "EV-003"},
		{ID: "RU-001"}, // not an EV id
		{ID: ""},       // empty
	}
	id := nextEvalID(entries)
	if id != "EV-004" {
		t.Errorf("expected EV-004, got %q", id)
	}
}

// --- parseEvalsForInit ---

func writeInitTestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseEvalsForInit_FileNotExist(t *testing.T) {
	entries, err := parseEvalsForInit("/nonexistent/path/evals.yml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for missing file")
	}
}

func TestParseEvalsForInit_EmptyFile(t *testing.T) {
	path := writeInitTestFile(t, "")
	entries, err := parseEvalsForInit(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty file, got %d", len(entries))
	}
}

func TestParseEvalsForInit_WhitespaceOnly(t *testing.T) {
	path := writeInitTestFile(t, "   \n\n  ")
	entries, err := parseEvalsForInit(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for whitespace-only file, got %d", len(entries))
	}
}

func TestParseEvalsForInit_OneEntry(t *testing.T) {
	path := writeInitTestFile(t, `- id: EV-001
  tests: RU-001
  prompt: p
  input: i
  assert:
    - contains: foo
`)
	entries, err := parseEvalsForInit(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "EV-001" {
		t.Errorf("ID: expected EV-001, got %q", entries[0].ID)
	}
	if entries[0].Tests != "RU-001" {
		t.Errorf("Tests: expected RU-001, got %q", entries[0].Tests)
	}
}

func TestParseEvalsForInit_MultipleEntries(t *testing.T) {
	path := writeInitTestFile(t, `- id: EV-001
  tests: RU-001
  prompt: p
  input: i
  assert:
    - contains: foo
- id: EV-005
  tests: RU-002
  prompt: p
  input: i
  assert:
    - contains: bar
`)
	entries, err := parseEvalsForInit(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].ID != "EV-005" {
		t.Errorf("second entry ID: expected EV-005, got %q", entries[1].ID)
	}
}

func TestParseEvalsForInit_LineNumbers(t *testing.T) {
	content := `- id: EV-001
  tests: RU-001
  prompt: p
  input: i
  assert:
    - contains: foo
- id: EV-002
  tests: RU-002
  prompt: p
  input: i
  assert:
    - contains: bar
`
	path := writeInitTestFile(t, content)
	entries, err := parseEvalsForInit(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Line != 1 {
		t.Errorf("EV-001 expected line 1, got %d", entries[0].Line)
	}
	if entries[1].Line != 7 {
		t.Errorf("EV-002 expected line 7, got %d", entries[1].Line)
	}
}

func TestParseEvalsForInit_MalformedYAML(t *testing.T) {
	path := writeInitTestFile(t, "this: {is: not: valid: yaml:")
	_, err := parseEvalsForInit(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if err != errMalformedEvals {
		t.Errorf("expected errMalformedEvals sentinel, got %v", err)
	}
}

func TestParseEvalsForInit_NotAList(t *testing.T) {
	// Valid YAML but not a list at root.
	path := writeInitTestFile(t, "key: value\nother: thing\n")
	_, err := parseEvalsForInit(path)
	if err == nil {
		t.Fatal("expected error for non-list YAML")
	}
	if err != errMalformedEvals {
		t.Errorf("expected errMalformedEvals sentinel, got %v", err)
	}
}

func TestParseEvalsForInit_CommentsPreserved(t *testing.T) {
	// Comments don't affect parsing correctness.
	path := writeInitTestFile(t, `# Header comment
- id: EV-001
  tests: RU-001
  # inline comment
  prompt: p
  input: i
  assert:
    - contains: foo
`)
	entries, err := parseEvalsForInit(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "EV-001" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

// --- formatPlaceholderEntry ---

func TestFormatPlaceholderEntry_Structure(t *testing.T) {
	entry := formatPlaceholderEntry("EV-024", "RU-001", "substrate/rules/RU-001-params-expect.md")

	checks := []string{
		"- id: EV-024",
		"  tests: RU-001",
		"  prompt_file: substrate/rules/RU-001-params-expect.md",
		"  # TODO: describe the task the model should perform",
		`  input: ""`,
		"  assert:",
		"    # TODO: define assertions for what the output should contain",
		`    - contains: ""`,
	}
	for _, want := range checks {
		if !strings.Contains(entry, want) {
			t.Errorf("entry missing %q\nGot:\n%s", want, entry)
		}
	}
}

func TestFormatPlaceholderEntry_TrailingNewline(t *testing.T) {
	entry := formatPlaceholderEntry("EV-001", "RU-001", "some/path.md")
	if !strings.HasSuffix(entry, "\n") {
		t.Error("entry should end with a newline")
	}
}

func TestFormatPlaceholderEntry_NoModelsBlock(t *testing.T) {
	entry := formatPlaceholderEntry("EV-001", "RU-001", "some/path.md")
	if strings.Contains(entry, "models:") {
		t.Error("placeholder should not include a models: block")
	}
}

func TestFormatPlaceholderEntry_TODOsPresent(t *testing.T) {
	entry := formatPlaceholderEntry("EV-001", "RU-001", "some/path.md")
	if strings.Count(entry, "TODO") != 2 {
		t.Errorf("expected 2 TODO comments, entry:\n%s", entry)
	}
}

// --- appendToEvalsFile ---

func TestAppendToEvalsFile_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")

	entry := "- id: EV-001\n  tests: RU-001\n"
	if err := appendToEvalsFile(path, entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != entry {
		t.Errorf("expected %q, got %q", entry, string(data))
	}
}

func TestAppendToEvalsFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	entry := "- id: EV-001\n  tests: RU-001\n"
	if err := appendToEvalsFile(path, entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != entry {
		t.Errorf("expected %q, got %q", entry, string(data))
	}
}

func TestAppendToEvalsFile_WithTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")
	existing := "- id: EV-001\n  tests: RU-001\n"
	os.WriteFile(path, []byte(existing), 0644)

	entry := "- id: EV-002\n  tests: RU-002\n"
	if err := appendToEvalsFile(path, entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	want := existing + entry
	if string(data) != want {
		t.Errorf("expected %q, got %q", want, string(data))
	}
}

func TestAppendToEvalsFile_WithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")
	existing := "- id: EV-001\n  tests: RU-001" // no trailing newline
	os.WriteFile(path, []byte(existing), 0644)

	entry := "- id: EV-002\n  tests: RU-002\n"
	if err := appendToEvalsFile(path, entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	want := existing + "\n" + entry
	if string(data) != want {
		t.Errorf("expected %q, got %q", want, string(data))
	}
}

func TestAppendToEvalsFile_PreservesExistingComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evals.yml")
	// Existing file has a comment that must survive append.
	existing := "# This comment must survive\n- id: EV-001\n  tests: RU-001\n"
	os.WriteFile(path, []byte(existing), 0644)

	entry := "- id: EV-002\n"
	appendToEvalsFile(path, entry)

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# This comment must survive") {
		t.Error("existing comment was lost after append")
	}
}

// --- RunInit integration ---

// makeInitTestDir creates a temp dir with an optional evals.yml and a config file
// pointing to it. Returns dir path, evals file path, and config path.
func makeInitTestDir(t *testing.T, evalsContent *string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	evalsPath := filepath.Join(dir, "evals.yml")

	if evalsContent != nil {
		if err := os.WriteFile(evalsPath, []byte(*evalsContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	configPath := filepath.Join(dir, ".skill-eval.yml")
	configContent := fmt.Sprintf("default_model: test-model\nevals_file: %s\n", evalsPath)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	return dir, evalsPath, configPath
}

func TestRunInit_MissingPath(t *testing.T) {
	code := RunInit([]string{})
	if code != 2 {
		t.Errorf("expected exit code 2 for missing --path, got %d", code)
	}
}

func TestRunInit_ArbitraryFilename(t *testing.T) {
	_, evalsPath, configPath := makeInitTestDir(t, nil)
	code := RunInit([]string{"--config", configPath, "--path", "foo/bar/my-prompt.md"})
	if code != 0 {
		t.Fatalf("expected exit code 0 for arbitrary filename, got %d", code)
	}
	data, _ := os.ReadFile(evalsPath)
	if !strings.Contains(string(data), "tests: my-prompt") {
		t.Errorf("expected tests: my-prompt in output: %s", string(data))
	}
}

func TestRunInit_MalformedEvalsFile(t *testing.T) {
	content := "this: {not valid yaml:"
	_, _, configPath := makeInitTestDir(t, &content)
	code := RunInit([]string{"--config", configPath, "--path", "substrate/rules/RU-001-params.md"})
	if code != 2 {
		t.Errorf("expected exit code 2 for malformed evals file, got %d", code)
	}
}

func TestRunInit_CreatesNewFile(t *testing.T) {
	_, evalsPath, configPath := makeInitTestDir(t, nil)
	// evals.yml does not exist yet

	code := RunInit([]string{
		"--config", configPath,
		"--path", "substrate/rules/RU-042-new-rule.md",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, err := os.ReadFile(evalsPath)
	if err != nil {
		t.Fatalf("expected evals.yml to be created: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "id: EV-001") {
		t.Errorf("expected EV-001 in new file: %s", content)
	}
	if !strings.Contains(content, "tests: RU-042") {
		t.Errorf("expected tests: RU-042: %s", content)
	}
	if !strings.Contains(content, "prompt_file: substrate/rules/RU-042-new-rule.md") {
		t.Errorf("expected prompt_file: %s", content)
	}
	if !strings.Contains(content, "TODO") {
		t.Errorf("expected TODO comments: %s", content)
	}
}

func TestRunInit_AppendToExisting(t *testing.T) {
	existing := `- id: EV-001
  tests: RU-001
  prompt: p
  input: i
  assert:
    - contains: foo
- id: EV-005
  tests: RU-002
  prompt: p
  input: i
  assert:
    - contains: bar
`
	_, evalsPath, configPath := makeInitTestDir(t, &existing)

	code := RunInit([]string{
		"--config", configPath,
		"--path", "substrate/rules/RU-003-something.md",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, _ := os.ReadFile(evalsPath)
	content := string(data)

	// Next ID after EV-005 is EV-006.
	if !strings.Contains(content, "id: EV-006") {
		t.Errorf("expected EV-006 appended: %s", content)
	}
	// Existing content preserved.
	if !strings.Contains(content, "id: EV-001") {
		t.Errorf("existing EV-001 should still be present: %s", content)
	}
	if !strings.Contains(content, "id: EV-005") {
		t.Errorf("existing EV-005 should still be present: %s", content)
	}
}

func TestRunInit_DryRunDoesNotModifyFile(t *testing.T) {
	existing := `- id: EV-001
  tests: RU-001
  prompt: p
  input: i
  assert:
    - contains: foo
`
	_, evalsPath, configPath := makeInitTestDir(t, &existing)
	beforeStat, err := os.Stat(evalsPath)
	if err != nil {
		t.Fatal(err)
	}

	code := RunInit([]string{
		"--config", configPath,
		"--path", "substrate/rules/RU-001-params.md",
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	afterStat, err := os.Stat(evalsPath)
	if err != nil {
		t.Fatal(err)
	}
	if afterStat.Size() != beforeStat.Size() {
		t.Error("dry-run should not modify evals.yml size")
	}
	if !afterStat.ModTime().Equal(beforeStat.ModTime()) {
		t.Error("dry-run should not modify evals.yml mtime")
	}
}

func TestRunInit_DryRunReportsWouldAdd(t *testing.T) {
	// Test via file state: file unchanged is sufficient.
	// (stdout capture requires pipe tricks; we skip that here.)
	_, evalsPath, configPath := makeInitTestDir(t, nil)

	code := RunInit([]string{
		"--config", configPath,
		"--path", "substrate/rules/RU-001-params.md",
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	// File should NOT have been created.
	if _, err := os.Stat(evalsPath); err == nil {
		t.Error("dry-run should not create evals.yml")
	}
}

func TestRunInit_CorrectSubstrateID(t *testing.T) {
	_, evalsPath, configPath := makeInitTestDir(t, nil)

	RunInit([]string{
		"--config", configPath,
		"--path", "substrate/patterns/PA-007-eager-loading.md",
	})

	data, _ := os.ReadFile(evalsPath)
	if !strings.Contains(string(data), "tests: PA-007") {
		t.Errorf("expected tests: PA-007: %s", string(data))
	}
}

func TestRunInit_EmptyEvalsFile(t *testing.T) {
	empty := ""
	_, evalsPath, configPath := makeInitTestDir(t, &empty)

	code := RunInit([]string{
		"--config", configPath,
		"--path", "substrate/rules/RU-001-something.md",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	data, err := os.ReadFile(evalsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "id: EV-001") {
		t.Errorf("expected EV-001 for empty evals file: %s", string(data))
	}
}

func TestRunInit_DefaultsToEvalsYMLWhenNoConfig(t *testing.T) {
	// When config is not found, evalsFile defaults to "evals.yml".
	// We pass a nonexistent config and a valid path — it should succeed
	// (it will create "evals.yml" in the cwd, so we skip actually verifying
	// the file to avoid polluting the working directory). Instead, just
	// confirm exit code 0 vs 2 behavior.
	//
	// Since we can't safely write "evals.yml" in the test's cwd, we
	// use a dry-run so no file is created.
	code := RunInit([]string{
		"--config", "/nonexistent/.skill-eval.yml",
		"--path", "RU-001-something.md",
		"--dry-run",
	})
	if code != 0 {
		t.Errorf("expected exit code 0 with nonexistent config (falls back to default), got %d", code)
	}
}
