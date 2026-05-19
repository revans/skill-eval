package skilleval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ResolveByPromptFile ---

func TestResolveByPromptFile_Match(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", PromptFile: "rules/RU-001.md"},
		{ID: "EV-002", PromptFile: "rules/RU-002.md"},
		{ID: "EV-003", PromptFile: "rules/RU-001.md"},
	}
	got := ResolveByPromptFile(evals, "rules/RU-001.md")
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2", len(got))
	}
	if got[0].ID != "EV-001" || got[1].ID != "EV-003" {
		t.Errorf("unexpected IDs: %v, %v", got[0].ID, got[1].ID)
	}
}

func TestResolveByPromptFile_NoMatch(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", PromptFile: "rules/RU-001.md"},
	}
	got := ResolveByPromptFile(evals, "rules/RU-999.md")
	if len(got) != 0 {
		t.Errorf("expected 0 matches, got %d", len(got))
	}
}

func TestResolveByPromptFile_InlinePromptNotMatched(t *testing.T) {
	// Evals with inline Prompt have PromptFile="", so they never match a real file path.
	evals := []Eval{
		{ID: "EV-001", Prompt: "inline prompt", PromptFile: ""},
		{ID: "EV-002", PromptFile: "rules/RU-001.md"},
	}
	got := ResolveByPromptFile(evals, "rules/RU-001.md")
	if len(got) != 1 || got[0].ID != "EV-002" {
		t.Errorf("inline-prompt eval should not be matched; got %v", got)
	}
}

func TestResolveByPromptFile_EmptyList(t *testing.T) {
	got := ResolveByPromptFile(nil, "rules/RU-001.md")
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

// --- ResolveByID ---

func TestResolveByID_Found(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", Tests: "RU-001"},
		{ID: "EV-002", Tests: "RU-002"},
	}
	e, ok := ResolveByID(evals, "EV-002")
	if !ok {
		t.Fatal("expected to find EV-002")
	}
	if e.Tests != "RU-002" {
		t.Errorf("got tests %q, want RU-002", e.Tests)
	}
}

func TestResolveByID_NotFound(t *testing.T) {
	evals := []Eval{{ID: "EV-001"}}
	_, ok := ResolveByID(evals, "EV-999")
	if ok {
		t.Error("expected not found")
	}
}

func TestResolveByID_EmptyList(t *testing.T) {
	_, ok := ResolveByID(nil, "EV-001")
	if ok {
		t.Error("expected not found on empty list")
	}
}

// --- RunTargeted helpers ---

func targetedCfg(t *testing.T) Config {
	t.Helper()
	return Config{
		DefaultModel:          "claude-test",
		ResultsDir:            t.TempDir(),
		SummariesDir:          t.TempDir(),
		PerEvalTimeoutSeconds: 30,
	}
}

func targetedEval(id, prompt, matchPhrase string) Eval {
	return Eval{
		ID:     id,
		Tests:  "TGT-001",
		Prompt: prompt,
		Input:  "Write a Rails method.",
		Assert: []Assertion{{Type: "contains", Value: matchPhrase}},
	}
}

// --- RunTargeted single eval ---

func TestRunTargeted_Single_LoadBearing(t *testing.T) {
	const sentinel = "TGT_LOAD_BEARING_SENTINEL"
	writeStdinAwareStub(t, sentinel, "params.expect here", "no match output")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T01", sentinel, "params.expect")}

	var buf bytes.Buffer
	code := RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	out := buf.String()
	if !strings.Contains(out, "WITH prompt:") {
		t.Errorf("output missing 'WITH prompt:' section\n%s", out)
	}
	if !strings.Contains(out, "WITHOUT prompt:") {
		t.Errorf("output missing 'WITHOUT prompt:' section\n%s", out)
	}
	if !strings.Contains(out, "LOAD-BEARING") {
		t.Errorf("output missing LOAD-BEARING classification\n%s", out)
	}
	if !strings.Contains(out, "Artifacts:") {
		t.Errorf("output missing Artifacts line\n%s", out)
	}
}

func TestRunTargeted_Single_OutputFormat_AssertionRows(t *testing.T) {
	writeStdinAwareStub(t, "SENTINEL_ASSERT", "params.expect present", "no match")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T02", "SENTINEL_ASSERT", "params.expect")}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	// Each assertion should appear with ✓ or ✗
	if !strings.Contains(out, "✓") && !strings.Contains(out, "✗") {
		t.Errorf("output should contain assertion marks\n%s", out)
	}
	// Result line
	if !strings.Contains(out, "Result:") {
		t.Errorf("output should contain Result: line\n%s", out)
	}
}

func TestRunTargeted_Single_Insufficient(t *testing.T) {
	stubClaude(t, "no match at all")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T03", "some prompt", "params.expect")}

	var buf bytes.Buffer
	code := RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (insufficient is not an error)", code)
	}
	out := buf.String()
	if !strings.Contains(out, "INSUFFICIENT") {
		t.Errorf("output missing INSUFFICIENT\n%s", out)
	}
}

func TestRunTargeted_Single_ClassificationExplanation(t *testing.T) {
	writeStdinAwareStub(t, "EXPLAIN_SENT", "params.expect present", "no match")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T04", "EXPLAIN_SENT", "params.expect")}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	// load-bearing explanation text
	if !strings.Contains(out, "The prompt is doing work") {
		t.Errorf("expected classification explanation\n%s", out)
	}
}

func TestRunTargeted_Single_Error_ReturnsOne(t *testing.T) {
	// No claude in PATH → subprocess error.
	stubDir := t.TempDir()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", stubDir) // stubDir has no claude binary

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T05", "some prompt", "params.expect")}

	var buf bytes.Buffer
	code := RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 on error", code)
	}
	out := buf.String()
	if !strings.Contains(out, "ERROR") {
		t.Errorf("output should contain ERROR\n%s", out)
	}
}

// --- RunTargeted multi eval ---

func TestRunTargeted_Multi_OneLinePerEval(t *testing.T) {
	stubClaude(t, "params.expect here")

	cfg := targetedCfg(t)
	evals := []Eval{
		targetedEval("EV-T10", "prompt", "params.expect"),
		targetedEval("EV-T11", "prompt", "params.expect"),
	}
	evals[1].Tests = "TGT-002"

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	if !strings.Contains(out, "EV-T10") {
		t.Errorf("output missing EV-T10\n%s", out)
	}
	if !strings.Contains(out, "EV-T11") {
		t.Errorf("output missing EV-T11\n%s", out)
	}
	if !strings.Contains(out, "Summary:") {
		t.Errorf("output missing Summary: section\n%s", out)
	}
	if !strings.Contains(out, "Artifacts:") {
		t.Errorf("output missing Artifacts: line\n%s", out)
	}
}

func TestRunTargeted_Multi_NotesSection_Obsolete(t *testing.T) {
	// Both runs pass → obsolete.
	stubClaude(t, "params.expect here")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T12", "prompt", "params.expect")}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	// Single eval goes to printTargetedSingle, not printTargetedMulti.
	// Use 2 evals to hit printTargetedMulti.
	evals = append(evals, targetedEval("EV-T13", "prompt", "params.expect"))

	buf.Reset()
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	if !strings.Contains(out, "Notes:") {
		t.Errorf("expected Notes: section for obsolete evals\n%s", out)
	}
	if !strings.Contains(out, "obsolete") {
		t.Errorf("expected 'obsolete' note\n%s", out)
	}
}

func TestRunTargeted_Multi_Error_ReturnsOne(t *testing.T) {
	stubDir := t.TempDir()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", stubDir)

	cfg := targetedCfg(t)
	evals := []Eval{
		targetedEval("EV-T14", "prompt", "params.expect"),
		targetedEval("EV-T15", "prompt", "params.expect"),
	}

	var buf bytes.Buffer
	code := RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunTargeted_Multi_ArtifactsDirShared(t *testing.T) {
	stubClaude(t, "params.expect here")

	cfg := targetedCfg(t)
	// Both evals share tests ID TGT-001 — artifacts root should be resultsDir/TGT-001.
	evals := []Eval{
		targetedEval("EV-T16", "prompt", "params.expect"),
		targetedEval("EV-T17", "prompt", "params.expect"),
	}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	want := filepath.Join(cfg.ResultsDir, "TGT-001")
	if !strings.Contains(out, want) {
		t.Errorf("expected artifacts dir %q in output\n%s", want, out)
	}
}

// --- No summary written ---

func TestRunTargeted_NoSummaryWritten(t *testing.T) {
	writeStdinAwareStub(t, "NO_SUMMARY_SENT", "params.expect here", "no match")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T20", "NO_SUMMARY_SENT", "params.expect")}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)

	entries, err := os.ReadDir(cfg.SummariesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("targeted mode must not write summary files; found %d file(s)", len(entries))
	}
}

// --- Artifacts written ---

func TestRunTargeted_Single_ArtifactsWritten(t *testing.T) {
	writeStdinAwareStub(t, "ART_SENT", "params.expect here", "no match")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T21", "ART_SENT", "params.expect")}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)

	// Artifacts should exist under resultsDir/TGT-001/EV-T21/
	evalDir := filepath.Join(cfg.ResultsDir, "TGT-001", "EV-T21")
	entries, err := os.ReadDir(evalDir)
	if err != nil {
		t.Fatalf("artifact dir not found: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected artifact files to be written")
	}

	// Check for expected artifact files.
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	hasWithPrompt := false
	hasResult := false
	for _, n := range names {
		if strings.HasSuffix(n, "-with-prompt.md") {
			hasWithPrompt = true
		}
		if strings.HasSuffix(n, "-result.yml") {
			hasResult = true
		}
	}
	if !hasWithPrompt {
		t.Errorf("missing *-with-prompt.md; files: %v", names)
	}
	if !hasResult {
		t.Errorf("missing *-result.yml; files: %v", names)
	}
}

// --- Header output ---

func TestRunTargeted_Header_SingleEval(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-T30", "prompt", "params.expect")}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	if !strings.Contains(out, "EV-T30") {
		t.Errorf("header should name the eval ID\n%s", out)
	}
	if !strings.Contains(out, "compare mode") {
		t.Errorf("header should mention compare mode\n%s", out)
	}
	if !strings.Contains(out, cfg.DefaultModel) {
		t.Errorf("header should show model\n%s", out)
	}
}

func TestRunTargeted_Header_MultiEval_SharedTests(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{
		targetedEval("EV-T31", "prompt", "params.expect"),
		targetedEval("EV-T32", "prompt", "params.expect"),
	}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	// Both share TGT-001 → header shows tests ID.
	if !strings.Contains(out, "TGT-001") {
		t.Errorf("header should show shared tests ID\n%s", out)
	}
}

func TestRunTargeted_Header_MultiEval_DifferentTests(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{
		targetedEval("EV-T33", "prompt", "params.expect"),
		targetedEval("EV-T34", "prompt", "params.expect"),
	}
	evals[1].Tests = "TGT-999"

	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{cfg.DefaultModel}, 1, &buf)
	out := buf.String()

	// Different tests IDs → generic "2 evals" header, no tests ID.
	if !strings.Contains(out, "2 evals") {
		t.Errorf("header should say '2 evals'\n%s", out)
	}
}

// --- sharedTestsID ---

func TestSharedTestsID_AllSame(t *testing.T) {
	evals := []Eval{{Tests: "RU-001"}, {Tests: "RU-001"}, {Tests: "RU-001"}}
	if got := sharedTestsID(evals); got != "RU-001" {
		t.Errorf("got %q, want RU-001", got)
	}
}

func TestSharedTestsID_Mixed(t *testing.T) {
	evals := []Eval{{Tests: "RU-001"}, {Tests: "RU-002"}}
	if got := sharedTestsID(evals); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSharedTestsID_Empty(t *testing.T) {
	if got := sharedTestsID(nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// --- modelResultsArtifactsRoot ---

func TestModelResultsArtifactsRoot_SharedTestsID(t *testing.T) {
	resultsDir := "/tmp/results"
	results := []ModelTaskResult{
		{TestsID: "RU-001"},
		{TestsID: "RU-001"},
	}
	got := modelResultsArtifactsRoot(results, resultsDir)
	want := filepath.Join(resultsDir, "RU-001")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestModelResultsArtifactsRoot_MixedTestsID(t *testing.T) {
	resultsDir := "/tmp/results"
	results := []ModelTaskResult{
		{TestsID: "RU-001"},
		{TestsID: "RU-002"},
	}
	got := modelResultsArtifactsRoot(results, resultsDir)
	if got != resultsDir {
		t.Errorf("got %q, want %q", got, resultsDir)
	}
}

func TestModelResultsArtifactsRoot_EmptyResults(t *testing.T) {
	resultsDir := "/tmp/results"
	got := modelResultsArtifactsRoot(nil, resultsDir)
	if got != resultsDir {
		t.Errorf("got %q, want %q", got, resultsDir)
	}
}
