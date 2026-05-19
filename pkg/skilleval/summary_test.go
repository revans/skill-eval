package skilleval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// --- CompileSummary tests ---

func TestCompileSummary_SingleMode(t *testing.T) {
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	records := []PerEvalRecord{
		{EvalID: "EV-002", TestsID: "RU-001", Model: "claude-test", Mode: "single", DurationMs: 1000, Status: "pass"},
		{EvalID: "EV-001", TestsID: "RU-001", Model: "claude-test", Mode: "single", DurationMs: 2000, Status: "fail", FailureReason: `contains "x" - failed`},
		{EvalID: "EV-003", TestsID: "RU-002", Model: "claude-test", Mode: "single", DurationMs: 1500, Status: "error", FailureReason: "error: timeout"},
	}

	s := CompileSummary(records, 5*time.Second, ts, "claude-test")

	if s.Mode != "single" {
		t.Errorf("mode = %q, want single", s.Mode)
	}
	if s.TotalEvals != 3 {
		t.Errorf("total_evals = %d, want 3", s.TotalEvals)
	}
	if s.TotalPassed != 1 {
		t.Errorf("total_passed = %d, want 1", s.TotalPassed)
	}
	if s.TotalFailed != 2 {
		t.Errorf("total_failed = %d, want 2", s.TotalFailed)
	}
	if s.TotalDurationSeconds != 5 {
		t.Errorf("total_duration_seconds = %d, want 5", s.TotalDurationSeconds)
	}
	// (1000+2000+1500)/1000 = 4
	if s.TotalEvalTimeSeconds != 4 {
		t.Errorf("total_eval_time_seconds = %d, want 4", s.TotalEvalTimeSeconds)
	}
	if s.Model != "claude-test" {
		t.Errorf("model = %q", s.Model)
	}
	if s.RanAt != "2026-05-02T14:23:00Z" {
		t.Errorf("ran_at = %q", s.RanAt)
	}
	if s.Classifications != nil {
		t.Error("classifications should be nil for single mode")
	}
}

func TestCompileSummary_SingleMode_SortedByEvalID(t *testing.T) {
	records := []PerEvalRecord{
		{EvalID: "EV-003", TestsID: "RU-001", Mode: "single", DurationMs: 100, Status: "pass"},
		{EvalID: "EV-001", TestsID: "RU-001", Mode: "single", DurationMs: 100, Status: "pass"},
		{EvalID: "EV-002", TestsID: "RU-001", Mode: "single", DurationMs: 100, Status: "pass"},
	}
	s := CompileSummary(records, 0, time.Now(), "m")

	if len(s.Results) != 3 {
		t.Fatalf("results count = %d", len(s.Results))
	}
	if s.Results[0].EvalID != "EV-001" {
		t.Errorf("results[0] = %q, want EV-001", s.Results[0].EvalID)
	}
	if s.Results[1].EvalID != "EV-002" {
		t.Errorf("results[1] = %q, want EV-002", s.Results[1].EvalID)
	}
	if s.Results[2].EvalID != "EV-003" {
		t.Errorf("results[2] = %q, want EV-003", s.Results[2].EvalID)
	}
}

func TestCompileSummary_SingleMode_FailureReasonPropagated(t *testing.T) {
	records := []PerEvalRecord{
		{EvalID: "EV-001", TestsID: "RU-001", Mode: "single", DurationMs: 100, Status: "pass"},
		{EvalID: "EV-002", TestsID: "RU-001", Mode: "single", DurationMs: 100, Status: "fail", FailureReason: `contains "params.expect" - failed`},
	}
	s := CompileSummary(records, 0, time.Now(), "m")

	if s.Results[0].FailureReason != "" {
		t.Errorf("passing eval should have no failure_reason, got %q", s.Results[0].FailureReason)
	}
	if s.Results[1].FailureReason == "" {
		t.Error("failing eval should have failure_reason")
	}
}

func TestCompileSummary_CompareMode(t *testing.T) {
	records := []PerEvalRecord{
		{EvalID: "EV-001", TestsID: "RU-001", Mode: "compare", DurationMs: 4000, Classification: "load-bearing"},
		{EvalID: "EV-002", TestsID: "RU-001", Mode: "compare", DurationMs: 3000, Classification: "obsolete"},
		{EvalID: "EV-003", TestsID: "RU-002", Mode: "compare", DurationMs: 5000, Classification: "insufficient"},
		{EvalID: "EV-004", TestsID: "RU-002", Mode: "compare", DurationMs: 4000, Classification: "harmful"},
		{EvalID: "EV-005", TestsID: "RU-003", Mode: "compare", DurationMs: 2000, Classification: "error"},
	}
	s := CompileSummary(records, 10*time.Second, time.Now(), "claude-test")

	if s.Mode != "compare" {
		t.Errorf("mode = %q, want compare", s.Mode)
	}
	if s.TotalEvals != 5 {
		t.Errorf("total_evals = %d, want 5", s.TotalEvals)
	}
	if s.Classifications == nil {
		t.Fatal("classifications should not be nil for compare mode")
	}
	if s.Classifications.LoadBearing != 1 {
		t.Errorf("load-bearing = %d, want 1", s.Classifications.LoadBearing)
	}
	if s.Classifications.Obsolete != 1 {
		t.Errorf("obsolete = %d, want 1", s.Classifications.Obsolete)
	}
	if s.Classifications.Insufficient != 1 {
		t.Errorf("insufficient = %d, want 1", s.Classifications.Insufficient)
	}
	if s.Classifications.Harmful != 1 {
		t.Errorf("harmful = %d, want 1", s.Classifications.Harmful)
	}
	if s.Classifications.Error != 1 {
		t.Errorf("error = %d, want 1", s.Classifications.Error)
	}
	// Compare mode has no total_passed/total_failed
	if s.TotalPassed != 0 || s.TotalFailed != 0 {
		t.Errorf("compare mode should have zero total_passed/total_failed, got %d/%d", s.TotalPassed, s.TotalFailed)
	}
	// total_eval_time: (4000+3000+5000+4000+2000)/1000 = 18
	if s.TotalEvalTimeSeconds != 18 {
		t.Errorf("total_eval_time_seconds = %d, want 18", s.TotalEvalTimeSeconds)
	}
}

func TestCompileSummary_ZeroEvals(t *testing.T) {
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	s := CompileSummary(nil, 0, ts, "claude-test")

	if s.TotalEvals != 0 {
		t.Errorf("total_evals = %d, want 0", s.TotalEvals)
	}
	if len(s.Results) != 0 {
		t.Errorf("results should be empty, got %d", len(s.Results))
	}
	if s.Model != "claude-test" {
		t.Errorf("model = %q", s.Model)
	}
	if s.RanAt != "2026-05-02T14:23:00Z" {
		t.Errorf("ran_at = %q", s.RanAt)
	}
}

// --- WorkerResultToRecord tests ---

func TestWorkerResultToRecord_SinglePass(t *testing.T) {
	wr := WorkerResult{
		Result: EvalResult{
			EvalID:  "EV-001",
			TestsID: "RU-001",
			Model:   "claude-test",
			RanAt:   time.Now(),
			Passed:  true,
			DurationMs: 1234,
			Assertions: []AssertionResult{
				{Type: "contains", Value: "params.expect", Passed: true},
			},
		},
	}
	rec := WorkerResultToRecord(wr)

	if rec.EvalID != "EV-001" {
		t.Errorf("EvalID = %q", rec.EvalID)
	}
	if rec.Mode != "single" {
		t.Errorf("Mode = %q", rec.Mode)
	}
	if rec.Status != "pass" {
		t.Errorf("Status = %q", rec.Status)
	}
	if rec.DurationMs != 1234 {
		t.Errorf("DurationMs = %d", rec.DurationMs)
	}
	if rec.FailureReason != "" {
		t.Errorf("FailureReason should be empty for pass, got %q", rec.FailureReason)
	}
}

func TestWorkerResultToRecord_SingleFail(t *testing.T) {
	wr := WorkerResult{
		Result: EvalResult{
			EvalID:  "EV-002",
			TestsID: "RU-001",
			Model:   "claude-test",
			Passed:  false,
			DurationMs: 500,
			Assertions: []AssertionResult{
				{Type: "contains", Value: "params.expect", Passed: false},
				{Type: "not_contains", Value: "params.permit", Passed: true},
			},
		},
	}
	rec := WorkerResultToRecord(wr)

	if rec.Status != "fail" {
		t.Errorf("Status = %q, want fail", rec.Status)
	}
	if !strings.Contains(rec.FailureReason, "params.expect") {
		t.Errorf("FailureReason should mention first failing assertion, got %q", rec.FailureReason)
	}
}

func TestWorkerResultToRecord_Compare(t *testing.T) {
	wr := WorkerResult{
		Compare: &CompareResult{
			EvalID:         "EV-003",
			TestsID:        "RU-002",
			Model:          "claude-test",
			RanAt:          time.Now(),
			Classification: LoadBearing,
			WithPrompt:     EvalResult{DurationMs: 2000},
			WithoutPrompt:  EvalResult{DurationMs: 1800},
		},
	}
	rec := WorkerResultToRecord(wr)

	if rec.Mode != "compare" {
		t.Errorf("Mode = %q, want compare", rec.Mode)
	}
	if rec.Classification != "load-bearing" {
		t.Errorf("Classification = %q, want load-bearing", rec.Classification)
	}
	if rec.DurationMs != 3800 {
		t.Errorf("DurationMs = %d, want 3800 (sum of both runs)", rec.DurationMs)
	}
}

func TestWorkerResultToRecord_CompareError(t *testing.T) {
	wr := WorkerResult{
		Compare: &CompareResult{
			EvalID:  "EV-004",
			TestsID: "RU-001",
			Model:   "claude-test",
			Err:     fmt.Errorf("timeout"),
		},
	}
	rec := WorkerResultToRecord(wr)

	if rec.Classification != "error" {
		t.Errorf("Classification = %q, want error for errored compare run", rec.Classification)
	}
}

// --- WriteSummary tests ---

func TestWriteSummary_SingleMode_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	records := []PerEvalRecord{
		{EvalID: "EV-001", TestsID: "RU-001", Mode: "single", DurationMs: 1000, Status: "pass"},
	}
	s := CompileSummary(records, 5*time.Second, ts, "claude-test")

	path, err := WriteSummary(dir, ts, s)
	if err != nil {
		t.Fatalf("WriteSummary: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("summary file not created at %s", path)
	}
	if !strings.HasSuffix(path, "2026-05-02-T14-23.yml") {
		t.Errorf("unexpected path: %s", path)
	}
}

func TestWriteSummary_SingleMode_YAMLFormat(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 5, 2, 14, 23, 0, 0, time.UTC)
	records := []PerEvalRecord{
		{EvalID: "EV-001", TestsID: "RU-001", Mode: "single", DurationMs: 2341, Status: "pass"},
		{EvalID: "EV-002", TestsID: "RU-001", Mode: "single", DurationMs: 1500, Status: "fail", FailureReason: `contains "x" - failed`},
	}
	s := CompileSummary(records, 10*time.Second, ts, "claude-sonnet-4-6")

	path, err := WriteSummary(dir, ts, s)
	if err != nil {
		t.Fatalf("WriteSummary: %v", err)
	}

	data, _ := os.ReadFile(path)
	yml := string(data)

	for _, want := range []string{
		"ran_at:", "model:", "mode: single",
		"total_evals:", "total_duration_seconds:", "total_eval_time_seconds:",
		"total_passed:", "total_failed:",
		"eval_id: EV-001", "eval_id: EV-002",
		"status: pass", "status: fail",
		"failure_reason:",
	} {
		if !strings.Contains(yml, want) {
			t.Errorf("YAML missing %q\ncontent:\n%s", want, yml)
		}
	}

	// Compare-mode fields must NOT appear in single-mode YAML.
	if strings.Contains(yml, "classifications:") {
		t.Errorf("single-mode YAML should not have classifications:\n%s", yml)
	}
}

func TestWriteSummary_CompareMode_YAMLFormat(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now()
	records := []PerEvalRecord{
		{EvalID: "EV-001", TestsID: "RU-001", Mode: "compare", DurationMs: 4000, Classification: "load-bearing"},
		{EvalID: "EV-002", TestsID: "RU-001", Mode: "compare", DurationMs: 3000, Classification: "obsolete"},
	}
	s := CompileSummary(records, 5*time.Second, ts, "claude-test")

	path, err := WriteSummary(dir, ts, s)
	if err != nil {
		t.Fatalf("WriteSummary: %v", err)
	}

	data, _ := os.ReadFile(path)
	yml := string(data)

	for _, want := range []string{
		"mode: compare", "classifications:", "load-bearing:", "obsolete:",
		"classification: load-bearing", "classification: obsolete",
	} {
		if !strings.Contains(yml, want) {
			t.Errorf("YAML missing %q\ncontent:\n%s", want, yml)
		}
	}
	// Single-mode counts must NOT appear in compare-mode YAML.
	if strings.Contains(yml, "total_passed:") {
		t.Errorf("compare-mode YAML should not have total_passed:\n%s", yml)
	}
	if strings.Contains(yml, "total_failed:") {
		t.Errorf("compare-mode YAML should not have total_failed:\n%s", yml)
	}
}

func TestWriteSummary_CreatesSummariesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "does", "not", "exist")
	ts := time.Now()
	s := CompileSummary(nil, 0, ts, "m")

	_, err := WriteSummary(dir, ts, s)
	if err != nil {
		t.Fatalf("WriteSummary should create dir, got: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("summaries dir was not created")
	}
}

func TestWriteAggregateSummary_Filename(t *testing.T) {
	dir := t.TempDir()
	s := CompileSummary(nil, 0, time.Now(), "m")
	path, err := WriteAggregateSummary(dir, "2026-05-01-T00-00", "2026-05-02-T23-59", s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "aggregate-2026-05-01-T00-00-to-2026-05-02-T23-59.yml") {
		t.Errorf("unexpected aggregate filename: %s", path)
	}
}

// --- ReadResultsFromDir tests ---

// writeFixtureResultYML writes a per-eval result YAML to resultsDir/{testsID}/{evalID}/{stamp}-result.yml.
func writeFixtureResultYML(t *testing.T, resultsDir, testsID, evalID, stamp string, yr interface{}) {
	t.Helper()
	dir := filepath.Join(resultsDir, testsID, evalID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(yr)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, stamp+"-result.yml"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func singleFixture(evalID, testsID, stamp, model, status string, durationMs int64) interface{} {
	assertions := []assertionResultYML{{Type: "contains", Value: "x", Result: status}}
	if status == "pass" {
		assertions[0].Result = "pass"
	} else {
		assertions[0].Result = "fail"
	}
	return resultYML{
		EvalID: evalID,
		Tests:  testsID,
		RanAt:  "2026-05-02T14:23:00Z",
		Mode:   "single",
		Model:  model,
		Input:  "test input",
		WithPrompt: runBlock{
			DurationMs: durationMs,
			Assertions: assertions,
			Status:     status,
		},
	}
}

func compareFixture(evalID, testsID, stamp, model, classification string, durationMs int64) interface{} {
	return compareResultYML{
		EvalID:         evalID,
		Tests:          testsID,
		RanAt:          "2026-05-02T14:23:00Z",
		Mode:           "compare",
		Model:          model,
		Input:          "test input",
		WithPrompt:     runBlock{DurationMs: durationMs / 2, Status: "pass"},
		WithoutPrompt:  runBlock{DurationMs: durationMs / 2, Status: "fail"},
		Classification: classification,
	}
}

func TestReadResultsFromDir_ByExactTimestamp(t *testing.T) {
	dir := t.TempDir()
	const stamp = "2026-05-02-T14-23"

	writeFixtureResultYML(t, dir, "RU-001", "EV-001", stamp, singleFixture("EV-001", "RU-001", stamp, "claude-test", "pass", 1000))
	writeFixtureResultYML(t, dir, "RU-001", "EV-002", stamp, singleFixture("EV-002", "RU-001", stamp, "claude-test", "fail", 2000))
	// Different timestamp — should NOT be included.
	writeFixtureResultYML(t, dir, "RU-001", "EV-003", "2026-05-02-T15-00", singleFixture("EV-003", "RU-001", "2026-05-02-T15-00", "claude-test", "pass", 500))

	records, err := ReadResultsFromDir(dir, stamp, stamp)
	if err != nil {
		t.Fatalf("ReadResultsFromDir: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	evalIDs := map[string]bool{}
	for _, r := range records {
		evalIDs[r.EvalID] = true
		if r.Mode != "single" {
			t.Errorf("%s: mode = %q, want single", r.EvalID, r.Mode)
		}
	}
	if !evalIDs["EV-001"] || !evalIDs["EV-002"] {
		t.Errorf("expected EV-001 and EV-002, got %v", evalIDs)
	}
}

func TestReadResultsFromDir_SinceUntilRange(t *testing.T) {
	dir := t.TempDir()

	writeFixtureResultYML(t, dir, "RU-001", "EV-001", "2026-05-01-T10-00", singleFixture("EV-001", "RU-001", "2026-05-01-T10-00", "m", "pass", 1000))
	writeFixtureResultYML(t, dir, "RU-001", "EV-002", "2026-05-02-T10-00", singleFixture("EV-002", "RU-001", "2026-05-02-T10-00", "m", "pass", 1000))
	writeFixtureResultYML(t, dir, "RU-001", "EV-003", "2026-05-03-T10-00", singleFixture("EV-003", "RU-001", "2026-05-03-T10-00", "m", "pass", 1000))
	// Out of range — should NOT be included.
	writeFixtureResultYML(t, dir, "RU-001", "EV-004", "2026-04-30-T10-00", singleFixture("EV-004", "RU-001", "2026-04-30-T10-00", "m", "pass", 1000))
	writeFixtureResultYML(t, dir, "RU-001", "EV-005", "2026-05-04-T10-00", singleFixture("EV-005", "RU-001", "2026-05-04-T10-00", "m", "pass", 1000))

	records, err := ReadResultsFromDir(dir, "2026-05-01-T00-00", "2026-05-03-T23-59")
	if err != nil {
		t.Fatalf("ReadResultsFromDir: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records in range, got %d", len(records))
	}

	evalIDs := map[string]bool{}
	for _, r := range records {
		evalIDs[r.EvalID] = true
	}
	for _, id := range []string{"EV-001", "EV-002", "EV-003"} {
		if !evalIDs[id] {
			t.Errorf("expected %s in results", id)
		}
	}
}

func TestReadResultsFromDir_CompareMode(t *testing.T) {
	dir := t.TempDir()
	const stamp = "2026-05-02-T14-23"

	writeFixtureResultYML(t, dir, "RU-001", "EV-001", stamp, compareFixture("EV-001", "RU-001", stamp, "claude-test", "load-bearing", 4000))

	records, err := ReadResultsFromDir(dir, stamp, stamp)
	if err != nil {
		t.Fatalf("ReadResultsFromDir: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Mode != "compare" {
		t.Errorf("Mode = %q, want compare", r.Mode)
	}
	if r.Classification != "load-bearing" {
		t.Errorf("Classification = %q, want load-bearing", r.Classification)
	}
	if r.DurationMs != 4000 {
		t.Errorf("DurationMs = %d, want 4000", r.DurationMs)
	}
}

func TestReadResultsFromDir_NoMatchReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFixtureResultYML(t, dir, "RU-001", "EV-001", "2026-05-01-T10-00", singleFixture("EV-001", "RU-001", "x", "m", "pass", 100))

	records, err := ReadResultsFromDir(dir, "2026-06-01-T00-00", "2026-06-01-T23-59")
	if err != nil {
		t.Fatalf("ReadResultsFromDir: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records for non-matching range, got %d", len(records))
	}
}

func TestReadResultsFromDir_NonExistentDirErrors(t *testing.T) {
	_, err := ReadResultsFromDir("/nonexistent/path", "2026-05-01-T00-00", "2026-05-01-T23-59")
	if err == nil {
		t.Error("expected error for non-existent results dir")
	}
}

func TestReadResultsFromDir_StatusPropagated(t *testing.T) {
	dir := t.TempDir()
	const stamp = "2026-05-02-T14-23"

	writeFixtureResultYML(t, dir, "RU-001", "EV-PASS", stamp, singleFixture("EV-PASS", "RU-001", stamp, "m", "pass", 1000))
	writeFixtureResultYML(t, dir, "RU-001", "EV-FAIL", stamp, singleFixture("EV-FAIL", "RU-001", stamp, "m", "fail", 1000))

	records, err := ReadResultsFromDir(dir, stamp, stamp)
	if err != nil {
		t.Fatal(err)
	}

	statuses := map[string]string{}
	for _, r := range records {
		statuses[r.EvalID] = r.Status
	}
	if statuses["EV-PASS"] != "pass" {
		t.Errorf("EV-PASS status = %q", statuses["EV-PASS"])
	}
	if statuses["EV-FAIL"] != "fail" {
		t.Errorf("EV-FAIL status = %q", statuses["EV-FAIL"])
	}
	if statuses["EV-FAIL"] == "pass" {
		t.Error("fail status should not be pass")
	}
}

// TestCompileSummary_RoundTrip verifies that writing and reading back produces
// the expected summary structure. This is the integration test that ties
// WorkerResultToRecord → CompileSummary → WriteSummary → ReadResultsFromDir together.
func TestCompileSummary_RoundTrip_Single(t *testing.T) {
	artifactDir := t.TempDir()
	summaryDir := t.TempDir()

	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(stubPath, []byte("#!/bin/bash\ncat > /dev/null\necho 'params.expect here'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", stubDir+":"+orig)

	eval := Eval{
		ID:    "EV-RT1",
		Tests: "RU-001",
		Prompt: "Use params.expect.",
		Input: "Write a controller.",
		Assert: []Assertion{{Type: "contains", Value: "params.expect"}},
	}
	cfg := Config{
		DefaultModel:          "claude-test",
		ResultsDir:            artifactDir,
		SummariesDir:          summaryDir,
		PerEvalTimeoutSeconds: 30,
	}
	wallStart := time.Now()
	evaluator := &Evaluator{Config: cfg}
	result := evaluator.Run(eval)
	if result.Err != nil {
		t.Fatalf("run error: %v", result.Err)
	}

	wr := WorkerResult{Result: result}
	paths := BuildArtifactPaths(artifactDir, eval.Tests, eval.ID, wallStart)
	if err := PrepareEvalDir(paths.Dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteWithPromptMD(paths.WithPromptMD, result.Output); err != nil {
		t.Fatal(err)
	}
	if err := WriteResultYML(paths, result); err != nil {
		t.Fatal(err)
	}

	// Compile and write summary.
	records := []PerEvalRecord{WorkerResultToRecord(wr)}
	elapsed := time.Since(wallStart)
	s := CompileSummary(records, elapsed, wallStart, cfg.DefaultModel)
	summaryPath, err := WriteSummary(summaryDir, wallStart, s)
	if err != nil {
		t.Fatalf("WriteSummary: %v", err)
	}

	// Verify summary file exists and has correct content.
	data, _ := os.ReadFile(summaryPath)
	yml := string(data)
	if !strings.Contains(yml, "EV-RT1") {
		t.Errorf("summary should contain EV-RT1; content:\n%s", yml)
	}
	if !strings.Contains(yml, "mode: single") {
		t.Errorf("summary should contain mode: single; content:\n%s", yml)
	}
	if !strings.Contains(yml, "total_passed: 1") {
		t.Errorf("summary should contain total_passed: 1; content:\n%s", yml)
	}
}
