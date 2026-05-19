package skilleval

import (
	"strings"
	"testing"
)

// --- ParseModelList ---

func TestParseModelList_Empty(t *testing.T) {
	if got := ParseModelList(""); got != nil {
		t.Errorf("expected nil for empty string, got %v", got)
	}
}

func TestParseModelList_Single(t *testing.T) {
	got := ParseModelList("claude-sonnet-4-6")
	if len(got) != 1 || got[0] != "claude-sonnet-4-6" {
		t.Errorf("got %v", got)
	}
}

func TestParseModelList_CommaSeparated(t *testing.T) {
	got := ParseModelList("claude-sonnet-4-6,claude-haiku-4-5")
	if len(got) != 2 || got[0] != "claude-sonnet-4-6" || got[1] != "claude-haiku-4-5" {
		t.Errorf("got %v", got)
	}
}

func TestParseModelList_WithWhitespace(t *testing.T) {
	got := ParseModelList("claude-sonnet-4-6 , claude-haiku-4-5")
	if len(got) != 2 || got[0] != "claude-sonnet-4-6" || got[1] != "claude-haiku-4-5" {
		t.Errorf("got %v", got)
	}
}

func TestParseModelList_DropsEmptyEntries(t *testing.T) {
	got := ParseModelList("claude-sonnet-4-6,,claude-haiku-4-5,")
	if len(got) != 2 {
		t.Errorf("expected 2 entries (empty dropped), got %v", got)
	}
}

func TestParseModelList_ThreeModels(t *testing.T) {
	got := ParseModelList("m1,m2,m3")
	if len(got) != 3 {
		t.Errorf("got %d models, want 3", len(got))
	}
}

// --- ResolveModels ---

func evalWithModels(id, primary string, secondaries ...string) Eval {
	return Eval{
		ID:     id,
		Tests:  "RU-001",
		Prompt: "p",
		Input:  "i",
		Assert: []Assertion{{Type: "contains", Value: "x"}},
		Models: &ModelsBlock{Primary: primary, Secondaries: secondaries},
	}
}

func evalNoModels(id string) Eval {
	return Eval{
		ID:     id,
		Tests:  "RU-001",
		Prompt: "p",
		Input:  "i",
		Assert: []Assertion{{Type: "contains", Value: "x"}},
	}
}

func TestResolveModels_Tier1_ModelFlag(t *testing.T) {
	e := evalWithModels("EV-001", "claude-opus-4-7")
	models, err := ResolveModels(e, []string{"claude-sonnet-4-6", "claude-haiku-4-5"}, false, "claude-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 || models[0] != "claude-sonnet-4-6" || models[1] != "claude-haiku-4-5" {
		t.Errorf("got %v, want [claude-sonnet-4-6 claude-haiku-4-5]", models)
	}
}

func TestResolveModels_Tier2_AllModels(t *testing.T) {
	e := evalWithModels("EV-001", "claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5")
	models, err := ResolveModels(e, nil, true, "claude-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("got %d models, want 3: %v", len(models), models)
	}
	if models[0] != "claude-opus-4-7" || models[1] != "claude-sonnet-4-6" || models[2] != "claude-haiku-4-5" {
		t.Errorf("got %v", models)
	}
}

func TestResolveModels_Tier2_AllModels_NoModelsBlock_Error(t *testing.T) {
	e := evalNoModels("EV-001")
	_, err := ResolveModels(e, nil, true, "claude-default")
	if err == nil {
		t.Fatal("expected error for --all-models with no models block")
	}
	if !strings.Contains(err.Error(), "--all-models") {
		t.Errorf("error should mention --all-models: %v", err)
	}
	if !strings.Contains(err.Error(), "EV-001") {
		t.Errorf("error should mention eval ID: %v", err)
	}
}

func TestResolveModels_Tier3_EvalPrimary(t *testing.T) {
	e := evalWithModels("EV-001", "claude-opus-4-7")
	models, err := ResolveModels(e, nil, false, "claude-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0] != "claude-opus-4-7" {
		t.Errorf("got %v, want [claude-opus-4-7]", models)
	}
}

func TestResolveModels_Tier4_ConfigDefault(t *testing.T) {
	e := evalNoModels("EV-001")
	models, err := ResolveModels(e, nil, false, "claude-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0] != "claude-default" {
		t.Errorf("got %v, want [claude-default]", models)
	}
}

func TestResolveModels_Tier1_OverridesAllModels(t *testing.T) {
	// --model flag takes priority over --all-models
	e := evalWithModels("EV-001", "claude-opus-4-7", "claude-haiku-4-5")
	models, err := ResolveModels(e, []string{"claude-sonnet-4-6"}, true, "claude-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0] != "claude-sonnet-4-6" {
		t.Errorf("modelFlag should win; got %v", models)
	}
}

func TestResolveModels_AllModels_PrimaryOnly(t *testing.T) {
	// --all-models with only primary (no secondaries)
	e := evalWithModels("EV-001", "claude-opus-4-7")
	models, err := ResolveModels(e, nil, true, "claude-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0] != "claude-opus-4-7" {
		t.Errorf("got %v, want [claude-opus-4-7]", models)
	}
}
