package skilleval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- ShortModelName ---

func TestShortModelName_Sonnet(t *testing.T) {
	if got := ShortModelName("claude-sonnet-4-6"); got != "Sonnet" {
		t.Errorf("got %q, want Sonnet", got)
	}
}

func TestShortModelName_Haiku(t *testing.T) {
	if got := ShortModelName("claude-haiku-4-5"); got != "Haiku" {
		t.Errorf("got %q, want Haiku", got)
	}
}

func TestShortModelName_Opus(t *testing.T) {
	if got := ShortModelName("claude-opus-4-7"); got != "Opus" {
		t.Errorf("got %q, want Opus", got)
	}
}

func TestShortModelName_WithDateSuffix(t *testing.T) {
	// "claude-haiku-4-5-20251001" — date part is numeric, haiku is the alpha name
	if got := ShortModelName("claude-haiku-4-5-20251001"); got != "Haiku" {
		t.Errorf("got %q, want Haiku", got)
	}
}

func TestShortModelName_TestModel(t *testing.T) {
	if got := ShortModelName("claude-test"); got != "Test" {
		t.Errorf("got %q, want Test", got)
	}
}

func TestShortModelName_Fallback(t *testing.T) {
	// No alpha segment → fall back to full name
	if got := ShortModelName("42-43-44"); got != "42-43-44" {
		t.Errorf("got %q, want full identifier", got)
	}
}

// --- matrixClassLabel ---

func TestMatrixClassLabel_Load(t *testing.T) {
	if got := matrixClassLabel(LoadBearing); got != "LOAD" {
		t.Errorf("got %q, want LOAD", got)
	}
}

func TestMatrixClassLabel_Obsolete(t *testing.T) {
	if got := matrixClassLabel(Obsolete); got != "OBSOLETE" {
		t.Errorf("got %q, want OBSOLETE", got)
	}
}

func TestMatrixClassLabel_Insufficient(t *testing.T) {
	if got := matrixClassLabel(Insufficient); got != "INSUFFICIENT" {
		t.Errorf("got %q, want INSUFFICIENT", got)
	}
}

func TestMatrixClassLabel_Harmful(t *testing.T) {
	if got := matrixClassLabel(Harmful); got != "HARMFUL" {
		t.Errorf("got %q, want HARMFUL", got)
	}
}

// --- Conflict detection ---

func TestConflictNote_LoadVsHarmful(t *testing.T) {
	classMap := map[string]Classification{
		"claude-sonnet-4-6": LoadBearing,
		"claude-haiku-4-5":  Harmful,
	}
	models := []string{"claude-sonnet-4-6", "claude-haiku-4-5"}
	shorts := []string{"Sonnet", "Haiku"}
	conflict, notes := buildConflictNote("EV-001", models, shorts, classMap)
	if !conflict {
		t.Fatal("expected conflict for Load vs Harmful")
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "EV-001") {
		t.Errorf("notes missing eval ID\n%s", joined)
	}
	if !strings.Contains(joined, "HARMFUL") {
		t.Errorf("notes missing HARMFUL\n%s", joined)
	}
	if !strings.Contains(joined, "LOAD-BEARING") {
		t.Errorf("notes missing LOAD-BEARING\n%s", joined)
	}
}

func TestConflictNote_LoadVsObsolete(t *testing.T) {
	classMap := map[string]Classification{
		"claude-sonnet-4-6": LoadBearing,
		"claude-haiku-4-5":  Obsolete,
	}
	conflict, notes := buildConflictNote("EV-002",
		[]string{"claude-sonnet-4-6", "claude-haiku-4-5"},
		[]string{"Sonnet", "Haiku"},
		classMap)
	if !conflict {
		t.Fatal("expected conflict for Load vs Obsolete")
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "OBSOLETE") {
		t.Errorf("notes missing OBSOLETE\n%s", joined)
	}
}

func TestConflictNote_InsufficientVsOther(t *testing.T) {
	classMap := map[string]Classification{
		"claude-sonnet-4-6": Insufficient,
		"claude-haiku-4-5":  LoadBearing,
	}
	conflict, _ := buildConflictNote("EV-003",
		[]string{"claude-sonnet-4-6", "claude-haiku-4-5"},
		[]string{"Sonnet", "Haiku"},
		classMap)
	if !conflict {
		t.Fatal("expected conflict for Insufficient vs Load")
	}
}

func TestConflictNote_AllSame_NoConflict(t *testing.T) {
	classMap := map[string]Classification{
		"claude-sonnet-4-6": LoadBearing,
		"claude-haiku-4-5":  LoadBearing,
	}
	conflict, _ := buildConflictNote("EV-004",
		[]string{"claude-sonnet-4-6", "claude-haiku-4-5"},
		[]string{"Sonnet", "Haiku"},
		classMap)
	if conflict {
		t.Error("should not be a conflict when all models agree")
	}
}

func TestConflictNote_AllObsolete_NoConflict(t *testing.T) {
	classMap := map[string]Classification{
		"m1": Obsolete,
		"m2": Obsolete,
	}
	conflict, _ := buildConflictNote("EV-005",
		[]string{"m1", "m2"}, []string{"M1", "M2"}, classMap)
	if conflict {
		t.Error("all-obsolete should not be a conflict")
	}
}

// --- RenderMatrix output ---

func makeMatrixResult(evalID, testsID, model string, class Classification) ModelTaskResult {
	return ModelTaskResult{
		EvalID:  evalID,
		TestsID: testsID,
		Model:   model,
		Compare: CompareResult{
			EvalID:         evalID,
			TestsID:        testsID,
			Model:          model,
			Classification: class,
		},
	}
}

func TestRenderMatrix_BasicOutput(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", Tests: "RU-001", Prompt: "p", Input: "i", Assert: []Assertion{{Type: "contains", Value: "x"}}},
	}
	models := []string{"claude-sonnet-4-6", "claude-haiku-4-5"}
	results := []ModelTaskResult{
		makeMatrixResult("EV-001", "RU-001", "claude-sonnet-4-6", LoadBearing),
		makeMatrixResult("EV-001", "RU-001", "claude-haiku-4-5", LoadBearing),
	}
	cfg := Config{DefaultModel: "claude-sonnet-4-6", ResultsDir: t.TempDir()}

	var buf bytes.Buffer
	code := RenderMatrix(evals, models, results, cfg, &buf)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	out := buf.String()

	if !strings.Contains(out, "Sonnet") {
		t.Errorf("missing Sonnet header\n%s", out)
	}
	if !strings.Contains(out, "Haiku") {
		t.Errorf("missing Haiku header\n%s", out)
	}
	if !strings.Contains(out, "LOAD") {
		t.Errorf("missing LOAD classification\n%s", out)
	}
	if !strings.Contains(out, "RU-001 (EV-001)") {
		t.Errorf("missing eval identifier\n%s", out)
	}
	if !strings.Contains(out, "Per-model summary:") {
		t.Errorf("missing per-model summary\n%s", out)
	}
	if !strings.Contains(out, "Artifacts:") {
		t.Errorf("missing Artifacts\n%s", out)
	}
}

func TestRenderMatrix_ConflictInNotes(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", Tests: "RU-001", Prompt: "p", Input: "i", Assert: []Assertion{{Type: "contains", Value: "x"}}},
	}
	models := []string{"claude-sonnet-4-6", "claude-haiku-4-5"}
	results := []ModelTaskResult{
		makeMatrixResult("EV-001", "RU-001", "claude-sonnet-4-6", LoadBearing),
		makeMatrixResult("EV-001", "RU-001", "claude-haiku-4-5", Harmful),
	}
	cfg := Config{DefaultModel: "claude-sonnet-4-6", ResultsDir: t.TempDir()}

	var buf bytes.Buffer
	RenderMatrix(evals, models, results, cfg, &buf)
	out := buf.String()

	if !strings.Contains(out, "Notes:") {
		t.Errorf("expected Notes: section for conflict\n%s", out)
	}
	if !strings.Contains(out, "conflicting") {
		t.Errorf("expected 'conflicting' in notes\n%s", out)
	}
}

func TestRenderMatrix_ObsoleteInNotes(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", Tests: "RU-001", Prompt: "p", Input: "i", Assert: []Assertion{{Type: "contains", Value: "x"}}},
		{ID: "EV-002", Tests: "RU-001", Prompt: "p", Input: "i", Assert: []Assertion{{Type: "contains", Value: "x"}}},
	}
	models := []string{"claude-sonnet-4-6", "claude-haiku-4-5"}
	results := []ModelTaskResult{
		makeMatrixResult("EV-001", "RU-001", "claude-sonnet-4-6", LoadBearing),
		makeMatrixResult("EV-001", "RU-001", "claude-haiku-4-5", LoadBearing),
		makeMatrixResult("EV-002", "RU-001", "claude-sonnet-4-6", Obsolete),
		makeMatrixResult("EV-002", "RU-001", "claude-haiku-4-5", LoadBearing),
	}
	cfg := Config{DefaultModel: "claude-sonnet-4-6", ResultsDir: t.TempDir()}

	var buf bytes.Buffer
	RenderMatrix(evals, models, results, cfg, &buf)
	out := buf.String()

	if !strings.Contains(out, "Notes:") {
		t.Errorf("expected Notes: for obsolete\n%s", out)
	}
	if !strings.Contains(out, "EV-002") {
		t.Errorf("expected EV-002 in notes\n%s", out)
	}
}

func TestRenderMatrix_AllLoadBearing_NoNotes(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", Tests: "RU-001", Prompt: "p", Input: "i", Assert: []Assertion{{Type: "contains", Value: "x"}}},
	}
	models := []string{"m1", "m2"}
	results := []ModelTaskResult{
		makeMatrixResult("EV-001", "RU-001", "m1", LoadBearing),
		makeMatrixResult("EV-001", "RU-001", "m2", LoadBearing),
	}
	cfg := Config{DefaultModel: "m1", ResultsDir: t.TempDir()}

	var buf bytes.Buffer
	RenderMatrix(evals, models, results, cfg, &buf)
	out := buf.String()

	if strings.Contains(out, "Notes:") {
		t.Errorf("unexpected Notes: section when all load-bearing\n%s", out)
	}
}

func TestRenderMatrix_ErrorReturnsOne(t *testing.T) {
	evals := []Eval{
		{ID: "EV-001", Tests: "RU-001", Prompt: "p", Input: "i", Assert: []Assertion{{Type: "contains", Value: "x"}}},
	}
	models := []string{"m1"}
	results := []ModelTaskResult{
		{EvalID: "EV-001", TestsID: "RU-001", Model: "m1", Compare: CompareResult{
			EvalID: "EV-001", TestsID: "RU-001", Model: "m1",
			Err: &errStub{"subprocess failed"},
		}},
	}
	cfg := Config{DefaultModel: "m1", ResultsDir: t.TempDir()}

	var buf bytes.Buffer
	code := RenderMatrix(evals, models, results, cfg, &buf)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 on error", code)
	}
}

type errStub struct{ msg string }

func (e *errStub) Error() string { return e.msg }

// --- Per-model artifact layout ---

func TestBuildModelArtifactPaths_Paths(t *testing.T) {
	resultsDir := "/results"
	paths := BuildModelArtifactPaths(resultsDir, "RU-001", "EV-001", "claude-sonnet-4-6", time.Time{})

	wantDir := "/results/RU-001/EV-001/claude-sonnet-4-6"
	if paths.Dir != wantDir {
		t.Errorf("Dir = %q, want %q", paths.Dir, wantDir)
	}
	if !strings.HasSuffix(paths.WithPromptMD, "-with-prompt.md") {
		t.Errorf("WithPromptMD missing suffix: %q", paths.WithPromptMD)
	}
	if !strings.Contains(paths.WithPromptMD, "claude-sonnet-4-6") {
		t.Errorf("WithPromptMD missing model dir: %q", paths.WithPromptMD)
	}
}

func TestEvalDirHasSubdirs_NoDir(t *testing.T) {
	has, err := EvalDirHasSubdirs("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected false for nonexistent dir")
	}
}

func TestEvalDirHasSubdirs_OnlyFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "result.yml"), []byte("data"), 0644)
	has, err := EvalDirHasSubdirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected false for dir with only files")
	}
}

func TestEvalDirHasSubdirs_WithSubdir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "claude-sonnet-4-6"), 0755)
	has, err := EvalDirHasSubdirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected true for dir with subdirectory")
	}
}

func TestPrepareEvalDirForModel_WipesFlatFiles(t *testing.T) {
	dir := t.TempDir()
	// Write a flat file (simulating a previous single-model run).
	flatFile := filepath.Join(dir, "2026-05-01-T12-00-result.yml")
	os.WriteFile(flatFile, []byte("old"), 0644)

	if err := PrepareEvalDirForModel(dir, "claude-sonnet-4-6"); err != nil {
		t.Fatalf("PrepareEvalDirForModel: %v", err)
	}

	// Flat file should be gone.
	if _, err := os.Stat(flatFile); !os.IsNotExist(err) {
		t.Error("flat file should have been wiped")
	}
	// Model subdirectory should exist.
	modelDir := filepath.Join(dir, "claude-sonnet-4-6")
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		t.Error("model subdirectory should have been created")
	}
}

func TestPrepareEvalDirForModel_LeavesOtherModelDirs(t *testing.T) {
	dir := t.TempDir()
	// Create an existing model subdir (from a previous multi-model run).
	otherModelDir := filepath.Join(dir, "claude-haiku-4-5")
	os.Mkdir(otherModelDir, 0755)
	os.WriteFile(filepath.Join(otherModelDir, "result.yml"), []byte("haiku"), 0644)

	if err := PrepareEvalDirForModel(dir, "claude-sonnet-4-6"); err != nil {
		t.Fatalf("PrepareEvalDirForModel: %v", err)
	}

	// Other model's directory should be untouched.
	if _, err := os.Stat(otherModelDir); os.IsNotExist(err) {
		t.Error("other model dir should be preserved")
	}
	if _, err := os.Stat(filepath.Join(otherModelDir, "result.yml")); os.IsNotExist(err) {
		t.Error("other model's files should be preserved")
	}
}

func TestPrepareEvalDirForModel_WipesModelSubdirFiles(t *testing.T) {
	dir := t.TempDir()
	// Existing model subdir with old results.
	modelDir := filepath.Join(dir, "claude-sonnet-4-6")
	os.Mkdir(modelDir, 0755)
	oldFile := filepath.Join(modelDir, "2026-01-01-T00-00-result.yml")
	os.WriteFile(oldFile, []byte("old"), 0644)

	if err := PrepareEvalDirForModel(dir, "claude-sonnet-4-6"); err != nil {
		t.Fatalf("PrepareEvalDirForModel: %v", err)
	}

	// Old file in model subdir should be gone.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old model subdir file should have been wiped")
	}
}

// --- Concurrency cap with multi-model ---

// TestRunTargeted_MultiModel_ConcurrencyCap verifies that the global concurrency
// cap applies across (eval, model) pairs — not per model. Running 2 evals × 2 models
// (4 total tasks) with concurrency=2 must complete all 4 tasks successfully, proving
// the pool drains the full task queue regardless of task count vs worker count.
func TestRunTargeted_MultiModel_ConcurrencyCap(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{
		targetedEval("EV-CC01", "prompt", "params.expect"),
		targetedEval("EV-CC02", "prompt", "params.expect"),
	}
	models := []string{"m1", "m2"}

	var buf bytes.Buffer
	// concurrency=2, tasks=4 — workers must loop to drain all tasks
	RunTargeted(evals, cfg, models, 2, &buf)

	// All 4 (eval, model) combinations must have produced an artifact dir.
	for _, evalID := range []string{"EV-CC01", "EV-CC02"} {
		for _, model := range models {
			dir := filepath.Join(cfg.ResultsDir, "TGT-001", evalID, model)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("missing artifact dir for (%s, %s): %s", evalID, model, dir)
			}
		}
	}
}

// --- Multi-model RunTargeted integration ---

func TestRunTargeted_MultiModel_ArtifactsInModelSubdirs(t *testing.T) {
	writeStdinAwareStub(t, "MULTI_MODEL_SENT", "params.expect here", "no match")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-MM01", "MULTI_MODEL_SENT", "params.expect")}
	models := []string{"claude-sonnet-4-6", "claude-haiku-4-5"}
	// Stub outputs the same for both models (sentinel detected either way since
	// both model names go through same stub — load-bearing for sentinel-matching).

	var buf bytes.Buffer
	RunTargeted(evals, cfg, models, 1, &buf)

	// Artifacts should be in per-model subdirs.
	sonnetDir := filepath.Join(cfg.ResultsDir, "TGT-001", "EV-MM01", "claude-sonnet-4-6")
	haikuDir := filepath.Join(cfg.ResultsDir, "TGT-001", "EV-MM01", "claude-haiku-4-5")

	if _, err := os.Stat(sonnetDir); os.IsNotExist(err) {
		t.Errorf("expected sonnet artifact dir to exist: %s", sonnetDir)
	}
	if _, err := os.Stat(haikuDir); os.IsNotExist(err) {
		t.Errorf("expected haiku artifact dir to exist: %s", haikuDir)
	}
}

func TestRunTargeted_MultiModel_MatrixOutput(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-MM02", "prompt", "params.expect")}
	models := []string{"m1", "m2"}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, models, 1, &buf)
	out := buf.String()

	if !strings.Contains(out, "Per-model summary:") {
		t.Errorf("expected matrix output with Per-model summary\n%s", out)
	}
	if !strings.Contains(out, "Artifacts:") {
		t.Errorf("expected Artifacts line\n%s", out)
	}
}

func TestRunTargeted_MultiModel_NoSummaryWritten(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-MM03", "prompt", "params.expect")}
	models := []string{"m1", "m2"}

	var buf bytes.Buffer
	RunTargeted(evals, cfg, models, 1, &buf)

	entries, err := os.ReadDir(cfg.SummariesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("multi-model targeted must not write summaries; found %d file(s)", len(entries))
	}
}

func TestRunTargeted_MixedState_FlatToPerModel(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-MM04", "prompt", "params.expect")}

	// Simulate a previous single-model run by placing flat files.
	evalDir := filepath.Join(cfg.ResultsDir, "TGT-001", "EV-MM04")
	os.MkdirAll(evalDir, 0755)
	flatFile := filepath.Join(evalDir, "2026-01-01-T00-00-result.yml")
	os.WriteFile(flatFile, []byte("old flat run"), 0644)

	// Now run multi-model — should wipe flat files and create model subdirs.
	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{"m1", "m2"}, 1, &buf)

	// Flat file should be gone.
	if _, err := os.Stat(flatFile); !os.IsNotExist(err) {
		t.Error("flat file should be wiped when transitioning to per-model layout")
	}
	// Model subdirs should exist.
	if _, err := os.Stat(filepath.Join(evalDir, "m1")); os.IsNotExist(err) {
		t.Error("m1 subdir should exist after multi-model run")
	}
}

func TestRunTargeted_MixedState_PerModelToSingleModel_MaintainsLayout(t *testing.T) {
	stubClaude(t, "params.expect")

	cfg := targetedCfg(t)
	evals := []Eval{targetedEval("EV-MM05", "prompt", "params.expect")}

	// Simulate a previous multi-model run by placing a model subdir.
	evalDir := filepath.Join(cfg.ResultsDir, "TGT-001", "EV-MM05")
	oldModelDir := filepath.Join(evalDir, "m1")
	os.MkdirAll(oldModelDir, 0755)
	os.WriteFile(filepath.Join(oldModelDir, "old-result.yml"), []byte("old"), 0644)

	// Now run single-model — should maintain per-model layout.
	var buf bytes.Buffer
	RunTargeted(evals, cfg, []string{"m2"}, 1, &buf)

	// Old model dir should still exist (not wiped).
	if _, err := os.Stat(oldModelDir); os.IsNotExist(err) {
		t.Error("old model dir should be preserved during single-model run in per-model layout")
	}
	// New model dir should exist.
	if _, err := os.Stat(filepath.Join(evalDir, "m2")); os.IsNotExist(err) {
		t.Error("new m2 model dir should exist")
	}
}

