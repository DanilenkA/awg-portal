package domain

import (
	"testing"

	"github.com/h44z/wg-portal/internal/lowlevel"
)

func TestInterface_GetAWGParams_RoundTrip(t *testing.T) {
	// Generate params
	params, err := lowlevel.GenerateAWGParams()
	if err != nil {
		t.Fatalf("GenerateAWGParams() failed: %v", err)
	}
	if params.IsZero() {
		t.Fatal("GenerateAWGParams() returned zero params")
	}

	// Create interface with AWG params
	iface := &Interface{
		Identifier: "wg0",
		AWGJc:   params.Jc,
		AWGJmin: params.Jmin,
		AWGJmax: params.Jmax,
		AWGS1:   params.S1,
		AWGS2:   params.S2,
		AWGH1:   params.H1,
		AWGH2:   params.H2,
		AWGH3:   params.H3,
		AWGH4:   params.H4,
	}

	// Read back via GetAWGParams
	got := iface.GetAWGParams()
	if got != params {
		t.Errorf("GetAWGParams() = %+v, want %+v", got, params)
	}
	if got.IsZero() {
		t.Error("GetAWGParams() returned zero params")
	}
}

func TestPhysicalInterface_AWGParams(t *testing.T) {
	params := lowlevel.AWGParams{
		Jc: 5, Jmin: 10, Jmax: 40,
		S1: 25, S2: 35,
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

	// Test zero params
	piZero := &PhysicalInterface{}
	_, ok = piZero.GetAWGParams()
	if ok {
		t.Error("GetAWGParams() should not be ok when no params set")
	}
}

func TestMergeToPhysicalInterface_AWGParams_CarriedOver(t *testing.T) {
	params := lowlevel.AWGParams{
		Jc: 3, Jmin: 15, Jmax: 45,
		S1: 20, S2: 30,
		H1: 111, H2: 222, H3: 333, H4: 444,
	}

	iface := &Interface{
		Identifier: "wg1",
		AWGJc:   params.Jc,
		AWGJmin: params.Jmin,
		AWGJmax: params.Jmax,
		AWGS1:   params.S1,
		AWGS2:   params.S2,
		AWGH1:   params.H1,
		AWGH2:   params.H2,
		AWGH3:   params.H3,
		AWGH4:   params.H4,
	}

	pi := &PhysicalInterface{
		Identifier: "wg1",
	}
	MergeToPhysicalInterface(pi, iface)

	got, ok := pi.GetAWGParams()
	if !ok {
		t.Fatal("AWG params should be carried over via MergeToPhysicalInterface")
	}
	if got != params {
		t.Errorf("AWG params mismatch after merge:\n  got:  %+v\n  want: %+v", got, params)
	}
}
