package skilleval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegration_RunEval tests the full eval execution pipeline using a stub
// claude binary that returns canned output without hitting the real API.
func TestIntegration_RunEval(t *testing.T) {
	// Create a temp dir for artifacts.
	artifactDir := t.TempDir()

	// Write a stub claude script that outputs content satisfying the test assertions.
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	stubScript := `#!/bin/bash
# Stub claude: reads stdin (the prompt), outputs canned content.
cat > /dev/null
echo "Here is a Rails controller using params.expect for mass assignment protection."
echo "The method: User.new(params.expect(user: [:name, :email]))"
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Prepend stub dir to PATH so our stub is found instead of the real claude.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", stubDir+":"+origPath)

	eval := Eval{
		ID:     "EV-001",
		Tests:  "RU-001",
		Prompt: "Use params.expect for mass assignment protection.",
		Input:  "Write a Rails controller create action for User.",
		Assert: []Assertion{
			{Type: "contains", Value: "params.expect"},
			{Type: "not_contains", Value: "params.permit"},
		},
	}

	cfg := Config{
		DefaultModel:          "claude-sonnet-4-6",
		ResultsDir:            artifactDir,
		PerEvalTimeoutSeconds: 30,
	}

	evaluator := &Evaluator{Config: cfg}
	result := evaluator.Run(eval)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if !result.Passed {
		t.Errorf("expected eval to pass; assertions: %+v", result.Assertions)
	}
	if result.EvalID != "EV-001" {
		t.Errorf("EvalID: got %q", result.EvalID)
	}
	if result.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: got %q", result.Model)
	}
	if !strings.Contains(result.Output, "params.expect") {
		t.Errorf("output should contain params.expect; got: %q", result.Output)
	}
	if result.DurationMs < 0 {
		t.Errorf("duration should be non-negative, got %d", result.DurationMs)
	}
	if len(result.Assertions) != 2 {
		t.Fatalf("expected 2 assertion results, got %d", len(result.Assertions))
	}
	if !result.Assertions[0].Passed {
		t.Error("first assertion (contains) should pass")
	}
	if !result.Assertions[1].Passed {
		t.Error("second assertion (not_contains) should pass")
	}
}

// TestIntegration_FailingAssertion verifies the runner reports failure correctly.
func TestIntegration_FailingAssertion(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	// This stub outputs content that will NOT satisfy the assertion.
	stubScript := `#!/bin/bash
cat > /dev/null
echo "Here is code using params.permit which is the old way."
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", stubDir+":"+origPath)

	eval := Eval{
		ID:     "EV-002",
		Tests:  "RU-001",
		Prompt: "Use params.expect.",
		Input:  "Write a create action.",
		Assert: []Assertion{
			{Type: "contains", Value: "params.expect"},
			{Type: "not_contains", Value: "params.permit"},
		},
	}

	cfg := Config{
		DefaultModel:          "claude-sonnet-4-6",
		PerEvalTimeoutSeconds: 30,
	}

	evaluator := &Evaluator{Config: cfg}
	result := evaluator.Run(eval)

	if result.Err != nil {
		t.Fatalf("unexpected subprocess error: %v", result.Err)
	}
	if result.Passed {
		t.Error("expected eval to fail")
	}
	// First assertion fails (output lacks params.expect).
	if result.Assertions[0].Passed {
		t.Error("first assertion should fail")
	}
	// Second assertion fails (output contains params.permit).
	if result.Assertions[1].Passed {
		t.Error("second assertion (not_contains params.permit) should fail")
	}
}

// TestIntegration_ArtifactsWritten verifies artifact files are created on disk.
func TestIntegration_ArtifactsWritten(t *testing.T) {
	artifactDir := t.TempDir()

	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	stubScript := `#!/bin/bash
cat > /dev/null
echo "output with params.expect"
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", stubDir+":"+origPath)

	eval := Eval{
		ID:    "EV-003",
		Tests: "RU-005",
		Prompt: "p",
		Input: "i",
		Assert: []Assertion{{Type: "contains", Value: "params.expect"}},
	}

	cfg := Config{
		DefaultModel:          "claude-haiku-4-5",
		ResultsDir:            artifactDir,
		PerEvalTimeoutSeconds: 30,
	}

	evaluator := &Evaluator{Config: cfg}
	result := evaluator.Run(eval)
	if result.Err != nil {
		t.Fatalf("run error: %v", result.Err)
	}

	paths := BuildArtifactPaths(artifactDir, eval.Tests, eval.ID, result.RanAt)
	if err := PrepareEvalDir(paths.Dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteWithPromptMD(paths.WithPromptMD, result.Output); err != nil {
		t.Fatal(err)
	}
	if err := WriteResultYML(paths, result); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(paths.WithPromptMD); os.IsNotExist(err) {
		t.Error("with-prompt markdown file was not created")
	}
	if _, err := os.Stat(paths.ResultYML); os.IsNotExist(err) {
		t.Error("result YAML file was not created")
	}

	data, _ := os.ReadFile(paths.ResultYML)
	if !strings.Contains(string(data), "EV-003") {
		t.Errorf("result YAML should contain EV-003; content: %s", string(data))
	}
}

// --- Compare-mode integration tests ---

// writeStdinAwareStub creates a stub that outputs different text depending on whether
// stdin contains sentinel. Puts the stub dir first in PATH for the test.
func writeStdinAwareStub(t *testing.T, sentinel, withOutput, withoutOutput string) {
	t.Helper()
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	script := "#!/bin/bash\n" +
		"input=$(cat)\n" +
		"if echo \"$input\" | grep -qF " + shellq(sentinel) + "; then\n" +
		"  printf '%s\\n' " + shellq(withOutput) + "\n" +
		"else\n" +
		"  printf '%s\\n' " + shellq(withoutOutput) + "\n" +
		"fi\n"
	if err := os.WriteFile(stubPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", stubDir+":"+origPath)
}

// shellq wraps s in single quotes for safe shell embedding.
func shellq(s string) string {
	out := ""
	for _, c := range s {
		if c == '\'' {
			out += "'\\''"
		} else {
			out += string(c)
		}
	}
	return "'" + out + "'"
}

func compareIntegrationEval(id, prompt, matchPhrase string) Eval {
	return Eval{
		ID:     id,
		Tests:  "CMP-INT",
		Prompt: prompt,
		Input:  "Write a Rails method.",
		Assert: []Assertion{{Type: "contains", Value: matchPhrase}},
	}
}

// TestIntegration_Compare_LoadBearing: with-prompt passes, without fails.
func TestIntegration_Compare_LoadBearing(t *testing.T) {
	const sentinel = "INT_LOAD_BEARING_SENTINEL"
	writeStdinAwareStub(t, sentinel, "params.expect used here", "no match output")

	eval := compareIntegrationEval("EV-INT-C01", sentinel, "params.expect")
	cfg := Config{DefaultModel: "claude-sonnet-4-6", PerEvalTimeoutSeconds: 30}
	cr := (&Evaluator{Config: cfg}).RunCompare(eval)

	if cr.Err != nil {
		t.Fatalf("unexpected error: %v", cr.Err)
	}
	if cr.Classification != LoadBearing {
		t.Errorf("classification = %q, want %q", cr.Classification, LoadBearing)
	}
	if !cr.WithPrompt.Passed {
		t.Errorf("with-prompt should pass; output: %q", cr.WithPrompt.Output)
	}
	if cr.WithoutPrompt.Passed {
		t.Errorf("without-prompt should fail; output: %q", cr.WithoutPrompt.Output)
	}
}

// TestIntegration_Compare_Obsolete: both runs pass.
func TestIntegration_Compare_Obsolete(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(stubPath, []byte("#!/bin/bash\ncat > /dev/null\necho 'params.expect always'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", stubDir+":"+origPath)

	eval := compareIntegrationEval("EV-INT-C02", "some prompt", "params.expect")
	cfg := Config{DefaultModel: "claude-sonnet-4-6", PerEvalTimeoutSeconds: 30}
	cr := (&Evaluator{Config: cfg}).RunCompare(eval)

	if cr.Err != nil {
		t.Fatalf("unexpected error: %v", cr.Err)
	}
	if cr.Classification != Obsolete {
		t.Errorf("classification = %q, want %q", cr.Classification, Obsolete)
	}
}

// TestIntegration_Compare_Insufficient: both runs fail.
func TestIntegration_Compare_Insufficient(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(stubPath, []byte("#!/bin/bash\ncat > /dev/null\necho 'unrelated output'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", stubDir+":"+origPath)

	eval := compareIntegrationEval("EV-INT-C03", "some prompt", "params.expect")
	cfg := Config{DefaultModel: "claude-sonnet-4-6", PerEvalTimeoutSeconds: 30}
	cr := (&Evaluator{Config: cfg}).RunCompare(eval)

	if cr.Err != nil {
		t.Fatalf("unexpected error: %v", cr.Err)
	}
	if cr.Classification != Insufficient {
		t.Errorf("classification = %q, want %q", cr.Classification, Insufficient)
	}
}

// TestIntegration_Compare_Harmful: with-prompt fails, without passes.
func TestIntegration_Compare_Harmful(t *testing.T) {
	const sentinel = "INT_HARMFUL_SENTINEL"
	// Sentinel present → output does NOT match; absent → output matches.
	writeStdinAwareStub(t, sentinel, "no match output", "params.expect present")

	eval := compareIntegrationEval("EV-INT-C04", sentinel, "params.expect")
	cfg := Config{DefaultModel: "claude-sonnet-4-6", PerEvalTimeoutSeconds: 30}
	cr := (&Evaluator{Config: cfg}).RunCompare(eval)

	if cr.Err != nil {
		t.Fatalf("unexpected error: %v", cr.Err)
	}
	if cr.Classification != Harmful {
		t.Errorf("classification = %q, want %q", cr.Classification, Harmful)
	}
	if cr.WithPrompt.Passed {
		t.Error("with-prompt run should fail")
	}
	if !cr.WithoutPrompt.Passed {
		t.Error("without-prompt run should pass")
	}
}

// TestIntegration_Compare_CompareYMLFormat verifies the YAML contains mode=compare
// and a classification field.
func TestIntegration_Compare_CompareYMLFormat(t *testing.T) {
	const sentinel = "INT_YAML_SENTINEL"
	writeStdinAwareStub(t, sentinel, "params.expect here", "no match")

	artifactDir := t.TempDir()
	eval := compareIntegrationEval("EV-INT-C05", sentinel, "params.expect")
	cfg := Config{
		DefaultModel:          "claude-sonnet-4-6",
		ResultsDir:            artifactDir,
		PerEvalTimeoutSeconds: 30,
	}
	cr := (&Evaluator{Config: cfg}).RunCompare(eval)
	if cr.Err != nil {
		t.Fatalf("unexpected error: %v", cr.Err)
	}

	paths := BuildArtifactPaths(artifactDir, eval.Tests, eval.ID, cr.RanAt)
	if err := PrepareEvalDir(paths.Dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompareResultYML(paths, cr); err != nil {
		t.Fatalf("WriteCompareResultYML: %v", err)
	}

	data, err := os.ReadFile(paths.ResultYML)
	if err != nil {
		t.Fatal(err)
	}
	yml := string(data)

	for _, want := range []string{"mode: compare", "classification:", "with_prompt:", "without_prompt:", "EV-INT-C05"} {
		if !strings.Contains(yml, want) {
			t.Errorf("YAML missing %q\ncontent:\n%s", want, yml)
		}
	}
}
