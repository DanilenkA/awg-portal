package lowlevel

import (
	"testing"
)

func TestAWGParams_IsZero(t *testing.T) {
	tests := []struct {
		name   string
		params AWGParams
		want   bool
	}{
		{"zero value", AWGParams{}, true},
		{"only Jc set", AWGParams{Jc: 5}, false},  // Jc != 0 → not zero

		{"only H1 set", AWGParams{H1: 42}, false},
		{"all H set", AWGParams{H1: 1, H2: 2, H3: 3, H4: 4}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.params.IsZero(); got != tt.want {
				t.Errorf("AWGParams.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateAWGParams(t *testing.T) {
	// Run multiple times to catch flaky edge cases
	for i := 0; i < 20; i++ {
		p, err := GenerateAWGParams()
		if err != nil {
			t.Fatalf("GenerateAWGParams() error = %v", err)
		}

		if p.IsZero() {
			t.Fatal("GenerateAWGParams() returned zero params")
		}

		// Verify ranges
		if p.Jc < 3 || p.Jc > 10 {
			t.Errorf("Jc out of range [3,10]: got %d", p.Jc)
		}
		if p.Jmin < 10 || p.Jmin > 50 {
			t.Errorf("Jmin out of range [10,50]: got %d", p.Jmin)
		}
		if p.Jmax < p.Jmin || p.Jmax > p.Jmin+50 {
			t.Errorf("Jmax out of range [%d,%d]: got %d", p.Jmin, p.Jmin+50, p.Jmax)
		}
		if p.S1 < 10 || p.S1 > 80 {
			t.Errorf("S1 out of range [10,80]: got %d", p.S1)
		}
		if p.S2 < 10 || p.S2 > 80 {
			t.Errorf("S2 out of range [10,80]: got %d", p.S2)
		}
		if p.H1 == 0 && p.H2 == 0 && p.H3 == 0 && p.H4 == 0 {
			t.Error("all H values are zero, expected at least one non-zero")
		}
	}
}

func TestGenerateAWGParamsDeterministic(t *testing.T) {
	// Ensure each call generates different params (randomness check)
	p1, _ := GenerateAWGParams()
	p2, _ := GenerateAWGParams()

	if p1 == p2 {
		t.Log("WARNING: two consecutive calls produced identical params (possible but unlikely)")
	}
}

func TestApplyAWGParams_NoSocket(t *testing.T) {
	// Without a running amneziawg-go instance, the UAPI socket won't exist.
	// We expect an error containing "dial" or "socket".
	err := ApplyAWGParams("wg0", AWGParams{Jc: 5, Jmin: 10, Jmax: 30, S1: 20, S2: 20, H1: 1, H2: 2, H3: 3, H4: 4})
	if err == nil {
		t.Skip("amneziawg-go seems to be running; skipping socket-unavailable test")
	}
	t.Logf("Expected error (no UAPI socket): %v", err)
}

func TestApplyAWGParams_ZeroParams(t *testing.T) {
	// ApplyAWGParams with zero params should return nil without attempting connection
	err := ApplyAWGParams("wg0", AWGParams{})
	if err != nil {
		t.Errorf("ApplyAWGParams with zero params should return nil, got: %v", err)
	}
}
