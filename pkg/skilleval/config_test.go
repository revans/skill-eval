package skilleval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_WithFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".skill-eval.yml")
	content := "default_model: claude-sonnet-4-6\nper_eval_timeout_seconds: 30\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultModel != "claude-sonnet-4-6" {
		t.Errorf("DefaultModel: got %q, want %q", cfg.DefaultModel, "claude-sonnet-4-6")
	}
	if cfg.PerEvalTimeoutSeconds != 30 {
		t.Errorf("PerEvalTimeoutSeconds: got %d, want 30", cfg.PerEvalTimeoutSeconds)
	}
	// Unset fields should use defaults.
	if cfg.EvalsFile != "evals.yml" {
		t.Errorf("EvalsFile: got %q, want default %q", cfg.EvalsFile, "evals.yml")
	}
	if cfg.ResultsDir != "evals/results" {
		t.Errorf("ResultsDir: got %q, want default %q", cfg.ResultsDir, "evals/results")
	}
	if cfg.SummariesDir != "evals/summaries" {
		t.Errorf("SummariesDir: got %q, want default %q", cfg.SummariesDir, "evals/summaries")
	}
	if cfg.Concurrency != 1 {
		t.Errorf("Concurrency: got %d, want default 1", cfg.Concurrency)
	}
}

func TestLoadConfig_PartialOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".skill-eval.yml")
	content := "default_model: claude-opus-4-7\nevals_file: custom-evals.yml\nconcurrency: 4\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultModel != "claude-opus-4-7" {
		t.Errorf("DefaultModel: got %q", cfg.DefaultModel)
	}
	if cfg.EvalsFile != "custom-evals.yml" {
		t.Errorf("EvalsFile: got %q", cfg.EvalsFile)
	}
	if cfg.Concurrency != 4 {
		t.Errorf("Concurrency: got %d", cfg.Concurrency)
	}
	// Unset fields use defaults.
	if cfg.PerEvalTimeoutSeconds != 60 {
		t.Errorf("PerEvalTimeoutSeconds: got %d, want 60", cfg.PerEvalTimeoutSeconds)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	// Missing file is allowed; but default_model is still required.
	_, err := LoadConfig("/nonexistent/path/.skill-eval.yml")
	if err == nil {
		t.Error("expected error for missing default_model when file absent")
	}
}

func TestLoadConfig_MissingDefaultModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".skill-eval.yml")
	content := "evals_file: other.yml\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for missing default_model")
	}
}

func TestLoadConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".skill-eval.yml")
	if err := os.WriteFile(path, []byte(":::invalid yaml:::"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}
