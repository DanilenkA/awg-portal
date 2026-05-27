package domain

import (
	"testing"

	"github.com/h44z/wg-portal/internal/lowlevel"
)

func TestInterface_GetAWGParams_RoundTrip(t *testing.T) {
	params, err := lowlevel.GenerateAWGParams()
	if err != nil {
		t.Fatalf("GenerateAWGParams() failed: %v", err)
	}
	if params.IsZero() {
		t.Fatal("GenerateAWGParams() returned zero params")
	}

	iface := &Interface{
		Identifier:  "wg0",
		AWGEnabled:  true,
		AWGJc:       params.Jc,
		AWGJmin:     params.Jmin,
		AWGJmax:     params.Jmax,
		AWGS1:       params.S1,
		AWGS2:       params.S2,
		AWGS3:       params.S3,
		AWGS4:       params.S4,
		AWGH1:       params.H1,
		AWGH2:       params.H2,
		AWGH3:       params.H3,
		AWGH4:       params.H4,
	}

	got := iface.GetAWGParams()
	if got != params {
		t.Errorf("GetAWGParams() mismatch:\n  got:  %+v\n  want: %+v", got, params)
	}
	if !iface.AWGEnabled {
		t.Error("AWGEnabled should be true")
	}
}

func TestPhysicalInterface_AWGParams(t *testing.T) {
	params := lowlevel.AWGParams{
		Jc: 4, Jmin: 60, Jmax: 500,
		S1: 30, S2: 40, S3: 15, S4: 5,
		H1: 100, H2: 200, H3: 300, H4: 400,
	}

	pi := &PhysicalInterface{}
	pi.SetAWGParams(params)

	got, ok := pi.GetAWGParams()
	if !ok {
		t.Fatal("GetAWGParams() should be ok when params are set")
	}
	if got != params {
		t.Errorf("GetAWGParams() = %+v, want %+v", got, params)
	}

	// Zero params on empty PhysicalInterface
	piZero := &PhysicalInterface{}
	_, ok = piZero.GetAWGParams()
	if ok {
		t.Error("GetAWGParams() should not be ok when no params set")
	}
}

func TestMergeToPhysicalInterface_AWGParamsCarried(t *testing.T) {
	iface := &Interface{
		Identifier: "wg1",
		AWGEnabled: true,
		AWGJc:      5, AWGJmin: 70, AWGJmax: 600,
		AWGS1: 25, AWGS2: 35, AWGS3: 10, AWGS4: 5,
		AWGH1: 111, AWGH2: 222, AWGH3: 333, AWGH4: 444,
	}

	pi := &PhysicalInterface{Identifier: "wg1"}
	MergeToPhysicalInterface(pi, iface)

	got, ok := pi.GetAWGParams()
	if !ok {
		t.Fatal("AWG params should be carried via MergeToPhysicalInterface")
	}
	if got.Jc != 5 || got.H1 != 111 {
		t.Errorf("AWG params mismatch: got %+v", got)
	}
}
