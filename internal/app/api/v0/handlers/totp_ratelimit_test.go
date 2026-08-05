package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTotpRateLimiter_AllowsBelowThreshold(t *testing.T) {
	rl := newTotpLoginRateLimit()
	key := "user1|10.0.0.1"

	for i := 0; i < totpRateLimitMaxAttempts-1; i++ {
		require.True(t, rl.allow(key), "attempt %d should be allowed", i)
		rl.recordFailure(key)
	}
}

func TestTotpRateLimiter_LocksAfterThreshold(t *testing.T) {
	rl := newTotpLoginRateLimit()
	key := "user2|10.0.0.2"

	for i := 0; i < totpRateLimitMaxAttempts; i++ {
		rl.recordFailure(key)
	}

	require.False(t, rl.allow(key), "should be locked after %d failures", totpRateLimitMaxAttempts)
}

func TestTotpRateLimiter_ResetClearsState(t *testing.T) {
	rl := newTotpLoginRateLimit()
	key := "user3|10.0.0.3"

	for i := 0; i < totpRateLimitMaxAttempts; i++ {
		rl.recordFailure(key)
	}
	require.False(t, rl.allow(key))

	rl.reset(key)
	require.True(t, rl.allow(key), "should be allowed again after reset")
}

func TestTotpRateLimiter_PerKeyIsolation(t *testing.T) {
	rl := newTotpLoginRateLimit()
	attacker := "user4|10.0.0.4"
	victim := "user5|10.0.0.5"

	for i := 0; i < totpRateLimitMaxAttempts; i++ {
		rl.recordFailure(attacker)
	}

	require.False(t, rl.allow(attacker))
	require.True(t, rl.allow(victim), "other keys must not be affected")
}
