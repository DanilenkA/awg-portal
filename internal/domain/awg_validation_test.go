package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInterface_ValidateAWGParams_Disabled verifies that when AWGEnabled
// is false, no AWG-related validation errors are raised regardless of
// the values of the obfuscation parameters.
func TestInterface_ValidateAWGParams_Disabled(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		// AWGEnabled false — params may be zero or garbage, must not error.
		AWGJc:99999,
		AWGJmin: -1,
		AWGH1:0,
	}
	assert.NoError(t, iface.validateAWGParams())
}

func TestInterface_ValidateAWGParams_Enabled_AllZero(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		// All zero params.
	}
	err := iface.validateAWGParams()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWGEnabled=true")
}

func TestInterface_ValidateAWGParams_Enabled_Valid(t *testing.T) {
	// Jc4, Jmin70, Jmax600 (>Jmin), H1..H4 pairwise unique ≥5, S* in [0..1280]
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		AWGJc:4,
		AWGJmin:70,
		AWGJmax:600,
		AWGS1:25, AWGS2:35, AWGS3:10, AWGS4:5,
		AWGH1:111, AWGH2:222, AWGH3:333, AWGH4:444,
	}
	assert.NoError(t, iface.validateAWGParams())
}

func TestInterface_ValidateAWGParams_JcOutOfRange(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		AWGJc:99999,
		AWGJmin:70,
		AWGJmax:600,
		AWGS1:25, AWGS2:35, AWGS3:10, AWGS4:5,
		AWGH1:111, AWGH2:222, AWGH3:333, AWGH4:444,
	}
	err := iface.validateAWGParams()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Jc")
}

func TestInterface_ValidateAWGParams_JmaxNotGreaterThanJmin(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		AWGJc:4,
		AWGJmin:600,
		AWGJmax:70, // < Jmin
		AWGS1:25, AWGS2:35, AWGS3:10, AWGS4:5,
		AWGH1:111, AWGH2:222, AWGH3:333, AWGH4:444,
	}
	err := iface.validateAWGParams()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Jmax")
}

func TestInterface_ValidateAWGParams_DuplicateH(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		AWGJc:4,
		AWGJmin:70,
		AWGJmax:600,
		AWGS1:25, AWGS2:35, AWGS3:10, AWGS4:5,
		// H1 == H2 — must be unique.
		AWGH1:111, AWGH2:111, AWGH3:333, AWGH4:444,
	}
	err := iface.validateAWGParams()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pairwise unique")
}

func TestInterface_ValidateAWGParams_HBelowFive(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		AWGJc:4,
		AWGJmin:70,
		AWGJmax:600,
		AWGS1:25, AWGS2:35, AWGS3:10, AWGS4:5,
		// H1 <5
		AWGH1:1, AWGH2:222, AWGH3:333, AWGH4:444,
	}
	err := iface.validateAWGParams()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ">= 5")
}

// TestInterface_Validate_CallsAWGValidation verifies that the main
// Validate() method delegates to validateAWGParams and propagates errors.
func TestInterface_Validate_CallsAWGValidation(t *testing.T) {
	iface := &Interface{
		Identifier: "wg0",
		AWGEnabled: true,
		AWGJc:99999, // bad
		AWGJmin:70,
		AWGJmax:600,
		AWGS1:25, AWGS2:35, AWGS3:10, AWGS4:5,
		AWGH1:111, AWGH2:222, AWGH3:333, AWGH4:444,
	}
	err := iface.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid awg params")
}
