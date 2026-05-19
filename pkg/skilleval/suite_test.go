package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
)

// stubClaude writes a stub claude script that echoes fixed output and returns
// a PATH string that puts the stub directory first. Restores PATH on cleanup.
func stubClaude(t *testing.T, output string) {
	t.Helper()
	stubDir := t.TempDir()
	script := "#!/bin/bash\ncat > /dev/null\nprintf '%s\\n' " + shellescape(output) + "\n"
	path := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", stubDir+":"+orig)
}

// shellescape wraps a string in single quotes for safe shell embedding.
func shellescape(s string) string {
	// Replace every ' with '\'' to safely embed in single-quoted shell strings.
	escaped := ""
	for _, c := range s {
		if c == '\'' {
			escaped += "'\\''"
		} else {
			escaped += string(c)
		}
	}
	return "'" + escaped + "'"
}

// testEvals creates n minimal evals whose stub output will contain matchValue.
func testEvals(n int, testsID, matchValue string) []Eval {
	evals := make([]Eval, n)
	for i := range evals {
		evals[i] = Eval{
			ID:     fmt.Sprintf("EV-%03d", i+1),
			Tests:  testsID,
			Prompt: "test prompt",
			Input:  "test input",
			Assert: []Assertion{{Type: "contains", Value: matchValue}},
		}
	}
	return evals
}

func testConfig(t *testing.T) Config {
	return Config{
		DefaultModel:          "claude-sonnet-4-6",
		ResultsDir:            t.TempDir(),
		PerEvalTimeoutSeconds: 30,
	}
}

func testSuite(cfg Config, concurrency int) *Suite {
	return &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: concurrency,
	}
}

// TestSuite_WorkerPool verifies 10 evals complete correctly at concurrency 4.
func TestSuite_WorkerPool(t *testing.T) {
	const output = "params.expect present here"
	stubClaude(t, output)

	cfg := testConfig(t)
	evals := testEvals(10, "RU-001", "params.expect")
	suite := testSuite(cfg, 4)

	var callbackCount int
	results := suite.Run(evals, func(completed, total int, wr WorkerResult) {
		callbackCount = completed
	})

	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	if callbackCount != 10 {
		t.Errorf("expected final callbackCount=10, got %d", callbackCount)
	}
	for _, wr := range results {
		if wr.Result.Err != nil {
			t.Errorf("%s: unexpected error: %v", wr.Result.EvalID, wr.Result.Err)
		}
		if !wr.Result.Passed {
			t.Errorf("%s: expected pass, assertions: %+v", wr.Result.EvalID, wr.Result.Assertions)
		}
	}
}

// TestSuite_Concurrency1_EquivalentToSequential verifies that concurrency 1
// produces correct results for all evals — regression against Phase 1 behavior.
func TestSuite_Concurrency1_EquivalentToSequential(t *testing.T) {
	const output = "has_one :activation"
	stubClaude(t, output)

	cfg := testConfig(t)
	evals := testEvals(5, "RU-002", "has_one")
	suite := testSuite(cfg, 1)

	results := suite.Run(evals, nil)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	for _, wr := range results {
		if wr.Result.Err != nil {
			t.Errorf("%s: unexpected error: %v", wr.Result.EvalID, wr.Result.Err)
		}
		if !wr.Result.Passed {
			t.Errorf("%s: expected pass", wr.Result.EvalID)
		}
		if wr.Result.Model != "claude-sonnet-4-6" {
			t.Errorf("%s: wrong model: %s", wr.Result.EvalID, wr.Result.Model)
		}
	}

	// All unique eval IDs should be present.
	seen := make(map[string]bool)
	for _, wr := range results {
		seen[wr.Result.EvalID] = true
	}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("EV-%03d", i)
		if !seen[id] {
			t.Errorf("missing result for %s", id)
		}
	}
}

// TestSuite_HighConcurrency runs 20 evals at concurrency 20.
// Combined with `go test -race`, this catches data races in the worker pool.
func TestSuite_HighConcurrency(t *testing.T) {
	const output = "strict mode enabled"
	stubClaude(t, output)

	cfg := testConfig(t)
	evals := testEvals(20, "RU-003", "strict mode")
	suite := testSuite(cfg, 20)

	results := suite.Run(evals, nil)

	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}
	for _, wr := range results {
		if wr.Result.Err != nil {
			t.Errorf("%s: error: %v", wr.Result.EvalID, wr.Result.Err)
		}
	}
}

// TestSuite_ProgressCounter verifies the completed counter increments 1..N
// in strict order within the callback.
func TestSuite_ProgressCounter(t *testing.T) {
	stubClaude(t, "target phrase")

	cfg := testConfig(t)
	evals := testEvals(8, "RU-004", "target phrase")
	suite := testSuite(cfg, 4)

	var lastSeen int32
	var outOfOrder bool
	results := suite.Run(evals, func(completed, total int, wr WorkerResult) {
		// completed must always equal lastSeen+1 (callback is serial on main goroutine)
		prev := atomic.LoadInt32(&lastSeen)
		if int32(completed) != prev+1 {
			outOfOrder = true
		}
		atomic.StoreInt32(&lastSeen, int32(completed))

		if total != 8 {
			t.Errorf("total should be 8, got %d", total)
		}
	})

	if outOfOrder {
		t.Error("callback completed counter did not increment sequentially")
	}
	if len(results) != 8 {
		t.Fatalf("expected 8 results, got %d", len(results))
	}
	if atomic.LoadInt32(&lastSeen) != 8 {
		t.Errorf("expected final counter=8, got %d", atomic.LoadInt32(&lastSeen))
	}
}

// TestSuite_FailingAssertions verifies failure cases are correctly reported.
func TestSuite_FailingAssertions(t *testing.T) {
	// Stub output does NOT contain the required phrase.
	stubClaude(t, "completely unrelated output")

	cfg := testConfig(t)
	evals := testEvals(3, "RU-005", "phrase that never appears")
	suite := testSuite(cfg, 2)

	results := suite.Run(evals, nil)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, wr := range results {
		if wr.Result.Err != nil {
			t.Errorf("%s: unexpected subprocess error: %v", wr.Result.EvalID, wr.Result.Err)
		}
		if wr.Result.Passed {
			t.Errorf("%s: expected fail, got pass", wr.Result.EvalID)
		}
	}
}

// TestSuite_EmptyEvals verifies that an empty eval list returns nil without panic.
func TestSuite_EmptyEvals(t *testing.T) {
	cfg := testConfig(t)
	suite := testSuite(cfg, 4)
	results := suite.Run(nil, nil)
	if results != nil {
		t.Errorf("expected nil for empty eval list, got %v", results)
	}
}

// TestSuite_ArtifactsWrittenConcurrently verifies that concurrent workers each
// write artifacts to their own eval-specific directories without collision.
func TestSuite_ArtifactsWrittenConcurrently(t *testing.T) {
	stubClaude(t, "expected content here")

	cfg := testConfig(t)
	evals := testEvals(6, "RU-006", "expected content")
	suite := testSuite(cfg, 6)

	results := suite.Run(evals, nil)

	if len(results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(results))
	}

	// Sort so we can check each eval's directory by index.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Result.EvalID < results[j].Result.EvalID
	})

	for _, wr := range results {
		if _, err := os.Stat(wr.Paths.Dir); os.IsNotExist(err) {
			t.Errorf("%s: artifact dir does not exist: %s", wr.Result.EvalID, wr.Paths.Dir)
		}
		if _, err := os.Stat(wr.Paths.ResultYML); os.IsNotExist(err) {
			t.Errorf("%s: result.yml not written", wr.Result.EvalID)
		}
		if _, err := os.Stat(wr.Paths.WithPromptMD); os.IsNotExist(err) {
			t.Errorf("%s: with-prompt.md not written", wr.Result.EvalID)
		}
	}
}

// stubClaudeStdinAware writes a stub that outputs one of two strings depending on
// whether stdin contains the sentinel string. Used for compare-mode tests.
func stubClaudeStdinAware(t *testing.T, sentinel, withSentinelOutput, withoutSentinelOutput string) {
	t.Helper()
	stubDir := t.TempDir()
	script := "#!/bin/bash\n" +
		"input=$(cat)\n" +
		"if echo \"$input\" | grep -qF " + shellescape(sentinel) + "; then\n" +
		"  printf '%s\\n' " + shellescape(withSentinelOutput) + "\n" +
		"else\n" +
		"  printf '%s\\n' " + shellescape(withoutSentinelOutput) + "\n" +
		"fi\n"
	path := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", stubDir+":"+orig)
}

func compareEval(id, sentinel, matchPhrase string) Eval {
	return Eval{
		ID:     id,
		Tests:  "CMP-001",
		Prompt: sentinel,
		Input:  "Write a method.",
		Assert: []Assertion{{Type: "contains", Value: matchPhrase}},
	}
}

// TestSuite_Compare_Obsolete: both runs pass → Obsolete.
func TestSuite_Compare_Obsolete(t *testing.T) {
	stubClaude(t, "target phrase here")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 1,
		Compare:     true,
	}
	evals := []Eval{compareEval("EV-C01", "SENTINEL", "target phrase")}

	results := suite.Run(evals, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	wr := results[0]
	if wr.Compare == nil {
		t.Fatal("expected non-nil Compare in compare mode")
	}
	if wr.Compare.Err != nil {
		t.Fatalf("unexpected error: %v", wr.Compare.Err)
	}
	if wr.Compare.Classification != Obsolete {
		t.Errorf("classification = %q, want %q", wr.Compare.Classification, Obsolete)
	}
	if !wr.Compare.WithPrompt.Passed {
		t.Error("with-prompt run should pass")
	}
	if !wr.Compare.WithoutPrompt.Passed {
		t.Error("without-prompt run should pass")
	}
}

// TestSuite_Compare_LoadBearing: with passes, without fails → LoadBearing.
func TestSuite_Compare_LoadBearing(t *testing.T) {
	const sentinel = "LOAD_BEARING_TEST_SENTINEL"
	stubClaudeStdinAware(t, sentinel, "target phrase present", "no match here")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 1,
		Compare:     true,
	}
	evals := []Eval{compareEval("EV-C02", sentinel, "target phrase")}

	results := suite.Run(evals, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	wr := results[0]
	if wr.Compare.Err != nil {
		t.Fatalf("unexpected error: %v", wr.Compare.Err)
	}
	if wr.Compare.Classification != LoadBearing {
		t.Errorf("classification = %q, want %q", wr.Compare.Classification, LoadBearing)
	}
	if !wr.Compare.WithPrompt.Passed {
		t.Error("with-prompt run should pass")
	}
	if wr.Compare.WithoutPrompt.Passed {
		t.Error("without-prompt run should fail")
	}
}

// TestSuite_Compare_Insufficient: both fail → Insufficient.
func TestSuite_Compare_Insufficient(t *testing.T) {
	stubClaude(t, "completely unrelated output")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 1,
		Compare:     true,
	}
	evals := []Eval{compareEval("EV-C03", "SENTINEL", "phrase that never appears")}

	results := suite.Run(evals, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	wr := results[0]
	if wr.Compare.Err != nil {
		t.Fatalf("unexpected error: %v", wr.Compare.Err)
	}
	if wr.Compare.Classification != Insufficient {
		t.Errorf("classification = %q, want %q", wr.Compare.Classification, Insufficient)
	}
}

// TestSuite_Compare_Harmful: with fails, without passes → Harmful.
func TestSuite_Compare_Harmful(t *testing.T) {
	const sentinel = "HARMFUL_TEST_SENTINEL"
	// Sentinel present → output does NOT match; absent → output matches.
	stubClaudeStdinAware(t, sentinel, "no match here", "target phrase present")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 1,
		Compare:     true,
	}
	evals := []Eval{compareEval("EV-C04", sentinel, "target phrase")}

	results := suite.Run(evals, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	wr := results[0]
	if wr.Compare.Err != nil {
		t.Fatalf("unexpected error: %v", wr.Compare.Err)
	}
	if wr.Compare.Classification != Harmful {
		t.Errorf("classification = %q, want %q", wr.Compare.Classification, Harmful)
	}
	if wr.Compare.WithPrompt.Passed {
		t.Error("with-prompt run should fail")
	}
	if !wr.Compare.WithoutPrompt.Passed {
		t.Error("without-prompt run should pass")
	}
}

// TestSuite_Compare_CompareField_NilInSingleMode verifies the Compare field stays
// nil when Suite.Compare is false.
func TestSuite_Compare_CompareField_NilInSingleMode(t *testing.T) {
	stubClaude(t, "target phrase")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 1,
		Compare:     false,
	}
	evals := testEvals(2, "RU-010", "target phrase")

	results := suite.Run(evals, nil)

	for _, wr := range results {
		if wr.Compare != nil {
			t.Errorf("%s: Compare should be nil in single mode", wr.Result.EvalID)
		}
	}
}

// TestSuite_Compare_ArtifactsWritten verifies both MD files and compare YAML
// are written to disk in compare mode.
func TestSuite_Compare_ArtifactsWritten(t *testing.T) {
	const sentinel = "COMPARE_ARTIFACT_SENTINEL"
	stubClaudeStdinAware(t, sentinel, "match phrase here", "no match here")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 1,
		Compare:     true,
	}
	evals := []Eval{compareEval("EV-C05", sentinel, "match phrase")}

	results := suite.Run(evals, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	wr := results[0]
	if wr.WriteErr != nil {
		t.Fatalf("artifact write error: %v", wr.WriteErr)
	}

	if _, err := os.Stat(wr.Paths.Dir); os.IsNotExist(err) {
		t.Error("artifact dir does not exist")
	}
	if _, err := os.Stat(wr.Paths.WithPromptMD); os.IsNotExist(err) {
		t.Error("with-prompt.md not written")
	}
	if _, err := os.Stat(wr.Paths.WithoutPromptMD); os.IsNotExist(err) {
		t.Error("without-prompt.md not written")
	}
	if _, err := os.Stat(wr.Paths.ResultYML); os.IsNotExist(err) {
		t.Error("result.yml not written")
	}
}

// TestSuite_Compare_ConcurrentWorkers verifies compare mode works under concurrency.
func TestSuite_Compare_ConcurrentWorkers(t *testing.T) {
	stubClaude(t, "consistent match phrase")

	cfg := testConfig(t)
	suite := &Suite{
		Evaluator:   &Evaluator{Config: cfg},
		Concurrency: 4,
		Compare:     true,
	}
	evals := testEvals(8, "RU-011", "consistent match phrase")
	// Override each eval's prompt so ConstructWithPrompt works.
	for i := range evals {
		evals[i].Prompt = "some prompt"
	}

	results := suite.Run(evals, nil)

	if len(results) != 8 {
		t.Fatalf("expected 8 results, got %d", len(results))
	}
	for _, wr := range results {
		if wr.Compare == nil {
			t.Errorf("%s: Compare should not be nil", workerResultIDFromTest(wr))
		}
		if wr.Compare != nil && wr.Compare.Err != nil {
			t.Errorf("%s: unexpected error: %v", wr.Compare.EvalID, wr.Compare.Err)
		}
	}
}

func workerResultIDFromTest(wr WorkerResult) string {
	if wr.Compare != nil {
		return wr.Compare.EvalID
	}
	return wr.Result.EvalID
}
