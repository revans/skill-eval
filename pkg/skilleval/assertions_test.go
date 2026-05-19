package skilleval

import "testing"

func TestRunAssertion(t *testing.T) {
	tests := []struct {
		name     string
		a        Assertion
		output   string
		wantPass bool
	}{
		{"contains pass", Assertion{"contains", "params.expect"}, "use params.expect here", true},
		{"contains fail", Assertion{"contains", "params.expect"}, "use params.permit here", false},
		{"not_contains pass", Assertion{"not_contains", "params.permit"}, "use params.expect here", true},
		{"not_contains fail", Assertion{"not_contains", "params.permit"}, "use params.permit here", false},
		{"matches pass", Assertion{"matches", `params\.(expect|permit)`}, "use params.expect here", true},
		{"matches fail", Assertion{"matches", `^params`}, "use params.expect here", false},
		{"not_matches pass", Assertion{"not_matches", `^params`}, "use params.expect here", true},
		{"not_matches fail", Assertion{"not_matches", `params\.expect`}, "use params.expect here", false},
		{"contains empty output", Assertion{"contains", "foo"}, "", false},
		{"not_contains empty output", Assertion{"not_contains", "foo"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunAssertion(tt.a, tt.output)
			if got.Passed != tt.wantPass {
				t.Errorf("RunAssertion type=%s value=%q output=%q: got passed=%v, want %v",
					tt.a.Type, tt.a.Value, tt.output, got.Passed, tt.wantPass)
			}
			if got.Type != tt.a.Type || got.Value != tt.a.Value {
				t.Errorf("RunAssertion returned wrong type/value: got %s/%s, want %s/%s",
					got.Type, got.Value, tt.a.Type, tt.a.Value)
			}
		})
	}
}

func TestRunAssertions_AllPass(t *testing.T) {
	assertions := []Assertion{
		{"contains", "params.expect"},
		{"not_contains", "params.permit"},
	}
	results, passed := RunAssertions(assertions, "use params.expect for safety")
	if !passed {
		t.Error("expected all assertions to pass")
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestRunAssertions_OneFails(t *testing.T) {
	assertions := []Assertion{
		{"contains", "params.expect"},
		{"contains", "params.permit"},
	}
	results, passed := RunAssertions(assertions, "use params.expect for safety")
	if passed {
		t.Error("expected overall failure when one assertion fails")
	}
	if results[0].Passed != true {
		t.Error("first assertion should pass")
	}
	if results[1].Passed != false {
		t.Error("second assertion should fail")
	}
}

func TestRunAssertions_Empty(t *testing.T) {
	results, passed := RunAssertions(nil, "anything")
	if !passed {
		t.Error("empty assertion list should pass")
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}
