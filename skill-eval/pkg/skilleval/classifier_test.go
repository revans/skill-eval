package skilleval

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		with    bool
		without bool
		want    Classification
	}{
		{true, false, LoadBearing},
		{true, true, Obsolete},
		{false, false, Insufficient},
		{false, true, Harmful},
	}
	for _, tt := range tests {
		got := Classify(tt.with, tt.without)
		if got != tt.want {
			t.Errorf("Classify(with=%v, without=%v) = %q, want %q", tt.with, tt.without, got, tt.want)
		}
	}
}

func TestClassificationConstants(t *testing.T) {
	if LoadBearing != "load-bearing" {
		t.Errorf("LoadBearing = %q", LoadBearing)
	}
	if Obsolete != "obsolete" {
		t.Errorf("Obsolete = %q", Obsolete)
	}
	if Insufficient != "insufficient" {
		t.Errorf("Insufficient = %q", Insufficient)
	}
	if Harmful != "harmful" {
		t.Errorf("Harmful = %q", Harmful)
	}
}
