package main

import (
	"strings"
	"testing"
)

// --- isTargetedMode ---

func TestIsTargetedMode_PromptFile(t *testing.T) {
	f := cliFlags{promptFile: "rules/RU-001.md"}
	if !isTargetedMode(f) {
		t.Error("expected targeted mode when --prompt-file set")
	}
}

func TestIsTargetedMode_EvalID(t *testing.T) {
	f := cliFlags{evalID: "EV-001"}
	if !isTargetedMode(f) {
		t.Error("expected targeted mode when --eval set")
	}
}

func TestIsTargetedMode_Both(t *testing.T) {
	f := cliFlags{promptFile: "rules/RU-001.md", evalID: "EV-001"}
	if !isTargetedMode(f) {
		t.Error("expected targeted mode when both flags set")
	}
}

func TestIsTargetedMode_Neither(t *testing.T) {
	f := cliFlags{}
	if isTargetedMode(f) {
		t.Error("expected suite mode when neither flag set")
	}
}

func TestIsTargetedMode_SuiteFlagsOnly(t *testing.T) {
	f := cliFlags{filter: "RU-001", compare: true, concurrency: 4}
	if isTargetedMode(f) {
		t.Error("expected suite mode with only suite flags set")
	}
}

// --- validateFlags mutual exclusions ---

func TestValidateFlags_SuiteMode_Valid(t *testing.T) {
	f := cliFlags{compare: true, filter: "RU-001"}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("expected valid, got code=%d msg=%q", code, msg)
	}
}

func TestValidateFlags_TargetedMode_Valid_PromptFile(t *testing.T) {
	f := cliFlags{promptFile: "rules/RU-001.md"}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("expected valid, got code=%d msg=%q", code, msg)
	}
}

func TestValidateFlags_TargetedMode_Valid_EvalID(t *testing.T) {
	f := cliFlags{evalID: "EV-001"}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("expected valid, got code=%d msg=%q", code, msg)
	}
}

func TestValidateFlags_TargetedMode_Valid_BothFlags(t *testing.T) {
	// --prompt-file and --eval together is valid (spec: can be combined)
	f := cliFlags{promptFile: "rules/RU-001.md", evalID: "EV-001"}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("expected valid combination, got code=%d msg=%q", code, msg)
	}
}

func TestValidateFlags_FilterWithEval(t *testing.T) {
	f := cliFlags{evalID: "EV-001", filter: "RU-001"}
	_, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--filter + --eval should error with code 2, got %d", code)
	}
}

func TestValidateFlags_FilterWithPromptFile(t *testing.T) {
	f := cliFlags{promptFile: "rules/RU-001.md", filter: "RU-001"}
	_, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--filter + --prompt-file should error with code 2, got %d", code)
	}
}

func TestValidateFlags_CompareWithEval(t *testing.T) {
	f := cliFlags{evalID: "EV-001", compare: true}
	_, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--compare + --eval should error with code 2, got %d", code)
	}
}

func TestValidateFlags_CompareWithPromptFile(t *testing.T) {
	f := cliFlags{promptFile: "rules/RU-001.md", compare: true}
	_, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--compare + --prompt-file should error with code 2, got %d", code)
	}
}

func TestValidateFlags_ErrorMessage_FilterConflict(t *testing.T) {
	f := cliFlags{evalID: "EV-001", filter: "RU-001"}
	msg, _ := validateFlags(f)
	if msg == "" {
		t.Error("expected non-empty error message for --filter conflict")
	}
}

func TestValidateFlags_ErrorMessage_CompareConflict(t *testing.T) {
	f := cliFlags{evalID: "EV-001", compare: true}
	msg, _ := validateFlags(f)
	if msg == "" {
		t.Error("expected non-empty error message for --compare conflict")
	}
}

// --- Phase 6: --model / --all-models flag validation ---

func TestValidateFlags_ModelFlag_InSuiteMode_Rejected(t *testing.T) {
	f := cliFlags{model: "claude-sonnet-4-6"}
	msg, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--model in suite mode should return code 2, got %d", code)
	}
	if !strings.Contains(msg, "--model") {
		t.Errorf("error should mention --model: %q", msg)
	}
}

func TestValidateFlags_AllModelsFlag_InSuiteMode_Rejected(t *testing.T) {
	f := cliFlags{allModels: true}
	msg, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--all-models in suite mode should return code 2, got %d", code)
	}
	if !strings.Contains(msg, "--all-models") {
		t.Errorf("error should mention --all-models: %q", msg)
	}
}

func TestValidateFlags_ModelAndAllModels_MutuallyExclusive(t *testing.T) {
	f := cliFlags{evalID: "EV-001", model: "claude-sonnet-4-6", allModels: true}
	msg, code := validateFlags(f)
	if code != 2 {
		t.Errorf("--model + --all-models should return code 2, got %d", code)
	}
	if !strings.Contains(msg, "--model") || !strings.Contains(msg, "--all-models") {
		t.Errorf("error should mention both flags: %q", msg)
	}
}

func TestValidateFlags_ModelFlag_InTargetedMode_Valid(t *testing.T) {
	f := cliFlags{evalID: "EV-001", model: "claude-sonnet-4-6"}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("--model in targeted mode should be valid, got code=%d msg=%q", code, msg)
	}
}

func TestValidateFlags_AllModelsFlag_InTargetedMode_Valid(t *testing.T) {
	f := cliFlags{evalID: "EV-001", allModels: true}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("--all-models in targeted mode should be valid, got code=%d msg=%q", code, msg)
	}
}

func TestValidateFlags_ModelFlag_WithPromptFile_Valid(t *testing.T) {
	f := cliFlags{promptFile: "rules/RU-001.md", model: "claude-sonnet-4-6,claude-haiku-4-5"}
	msg, code := validateFlags(f)
	if code != 0 {
		t.Errorf("--model with --prompt-file should be valid, got code=%d msg=%q", code, msg)
	}
}
