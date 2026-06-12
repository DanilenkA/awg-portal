package wireguard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DanilenkA/awg-portal/internal/domain"
)

func TestAWGModeChanged(t *testing.T) {
	tests := []struct {
		name string
		oldI *domain.Interface
		newI *domain.Interface
		expected bool
	}{
		{
			name: "nil oldIface → false",
			oldI: nil,
			newI: &domain.Interface{AWGEnabled: true},
			expected: false,
		},
		{
			name: "nil newIface → false",
			oldI: &domain.Interface{AWGEnabled: true},
			newI: nil,
			expected: false,
		},
		{
			name: "no change in plain WG",
			oldI: &domain.Interface{},
			newI: &domain.Interface{},
			expected: false,
		},
		{
			name: "no change in AWG",
			oldI: &domain.Interface{AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			newI: &domain.Interface{AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			expected: false,
		},
		{
			name: "plain WG → AWG (params present)",
			oldI: &domain.Interface{},
			newI: &domain.Interface{AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			expected: true,
		},
		{
			name: "AWG → plain WG",
			oldI: &domain.Interface{AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			newI: &domain.Interface{},
			expected: true,
		},
		{
			name: "AWG params changed (H1) — NOT a mode switch, applied via UAPI",
			oldI: &domain.Interface{
				AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400,
			},
			newI: &domain.Interface{
				AWGEnabled: true, AWGJc:4, AWGH1:999, AWGH2:200, AWGH3:300, AWGH4:400,
			},
			expected: false,
		},
		{
			name: "AWG params changed (Jc) — NOT a mode switch, applied via UAPI",
			oldI: &domain.Interface{
				AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400,
			},
			newI: &domain.Interface{
				AWGEnabled: true, AWGJc:5, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400,
			},
			expected: false,
		},
		{
			// Flipping AWGEnabled on without setting any obfuscation
			// parameters is NOT a mode switch — EmitAWG() requires
			// both AWGEnabled and non-zero params, so neither old
			// nor new state qualifies as "AWG". The validation hook
			// will reject the half-configured state later; here we
			// just confirm the mode-switch detector stays calm.
			name: "AWGEnabled flipped on with no params — EmitAWG stays false, NOT a mode switch",
			oldI: &domain.Interface{},
			newI: &domain.Interface{AWGEnabled: true},
			expected: false,
		},
		{
			// AWGEnabled flipped off with stale params: EmitAWG
			// goes true→false (params are still in the row, but
			// the flag is gone). This IS a mode switch because the
			// predicate changed.
			name: "AWGEnabled flipped off with stale params — EmitAWG goes true→false, IS a mode switch",
			oldI: &domain.Interface{AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			newI: &domain.Interface{AWGEnabled: false, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			expected: true,
		},
		{
			// Half-configured → fully configured: EmitAWG goes
			// false→true. Even though only one field changed
			// (Jmin was 0, now non-zero), the AWG predicate flipped,
			// so a real mode switch is required.
			name: "AWGEnabled=true with no params → AWGEnabled=true with params — IS a mode switch",
			oldI: &domain.Interface{AWGEnabled: true},
			newI: &domain.Interface{AWGEnabled: true, AWGJc:4, AWGH1:100, AWGH2:200, AWGH3:300, AWGH4:400},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, awgModeChanged(tt.oldI, tt.newI))
		})
	}
}
