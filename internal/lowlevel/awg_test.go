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
		{"only Jc set", AWGParams{Jc: 5}, false},
		{"only H1 set", AWGParams{H1: 42}, false},
		{"all H set no Jc", AWGParams{H1: 1, H2: 2, H3: 3, H4: 4}, false},
		{"full params", AWGParams{Jc: 5, Jmin: 60, Jmax: 500, S1: 20, S2: 80, H1: 10, H2: 20, H3: 30, H4: 40}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.params.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateAWGParams_Ranges(t *testing.T) {
	for i := 0; i < 50; i++ {
		p, err := GenerateAWGParams()
		if err != nil {
			t.Fatalf("GenerateAWGParams() error = %v", err)
		}
		if p.IsZero() {
			t.Fatal("GenerateAWGParams() returned zero params")
		}
		if p.Jc < 3 || p.Jc > 10 {
			t.Errorf("Jc out of range [3,10]: %d", p.Jc)
		}
		if p.Jmin < 50 || p.Jmin > 100 {
			t.Errorf("Jmin out of range [50,100]: %d", p.Jmin)
		}
		if p.Jmax < p.Jmin || p.Jmax > 1000 {
			t.Errorf("Jmax out of range [%d,1000]: %d", p.Jmin, p.Jmax)
		}
		if p.S1 < 15 || p.S1 > 150 {
			t.Errorf("S1 out of range [15,150]: %d", p.S1)
		}
		if p.S2 < 15 || p.S2 > 150 {
			t.Errorf("S2 out of range [15,150]: %d", p.S2)
		}
		if p.S1+56 == p.S2 {
			t.Errorf("S1+56 == S2 constraint violated: S1=%d S2=%d", p.S1, p.S2)
		}
		if p.H1 == p.H2 || p.H1 == p.H3 || p.H1 == p.H4 ||
			p.H2 == p.H3 || p.H2 == p.H4 || p.H3 == p.H4 {
			t.Errorf("H values not pairwise unique: %d %d %d %d", p.H1, p.H2, p.H3, p.H4)
		}
		if p.H1 < 5 || p.H2 < 5 || p.H3 < 5 || p.H4 < 5 {
			t.Errorf("H values must be >= 5: %d %d %d %d", p.H1, p.H2, p.H3, p.H4)
		}
	}
}

func TestApplyAWGParams_ZeroParams(t *testing.T) {
	if err := ApplyAWGParams("wg0", AWGParams{}); err != nil {
		t.Errorf("ApplyAWGParams with zero params should return nil, got: %v", err)
	}
}

func TestApplyAWGParams_NoSocket(t *testing.T) {
	err := ApplyAWGParams("wg0", AWGParams{
		Jc: 4, Jmin: 60, Jmax: 500,
		S1: 30, S2: 40,
		H1: 100, H2: 200, H3: 300, H4: 400,
	})
	if err == nil {
		t.Skip("amneziawg-go seems to be running; skipping socket test")
	}
	t.Logf("Expected error (no AWG daemon): %v", err)
}

func TestSocketPath(t *testing.T) {
	expected := "/var/run/amneziawg/wg0.sock"
	if got := socketPath("wg0"); got != expected {
		t.Errorf("socketPath() = %s, want %s", got, expected)
	}
}

func TestWGctrlSocketPath(t *testing.T) {
	expected := "/var/run/wireguard/wg0.sock"
	if got := wgctrlSocketPath("wg0"); got != expected {
		t.Errorf("wgctrlSocketPath() = %s, want %s", got, expected)
	}
}
