package handlers

import (
	"sync"
	"time"
)

// totpLoginRateLimiter is an in-memory sliding-window rate limiter for TOTP
// (second factor) attempts. It is deliberately in-memory (not the persistent
// User.Locked flag): the lockout is temporary and resets on restart, so an
// attacker cannot permanently lock a victim by spamming failures.
//
// This is the single most important TOTP mitigation — without it a 6-digit
// code is brute-forceable (see the Microsoft "AuthQuake" case).
const (
	totpRateLimitMaxAttempts = 5                // failures allowed within the window before lockout
	totpRateLimitWindow      = 15 * time.Minute // sliding window / lockout duration
	totpRateLimitCleanup     = 5 * time.Minute  // how often stale entries are purged
)

type totpRateLimitEntry struct {
	failures   []time.Time
	lockedTill time.Time
}

type totpLoginRateLimit struct {
	mu      sync.Mutex
	entries map[string]*totpRateLimitEntry
}

// newTotpLoginRateLimit creates the limiter and starts background cleanup.
func newTotpLoginRateLimit() *totpLoginRateLimit {
	rl := &totpLoginRateLimit{entries: make(map[string]*totpRateLimitEntry)}
	go rl.cleanupLoop()
	return rl
}

// allow reports whether an attempt is permitted for the given key (user|ip).
func (rl *totpLoginRateLimit) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.entries[key]
	if !ok {
		return true
	}
	if time.Now().Before(e.lockedTill) {
		return false
	}
	return true
}

// recordFailure registers a failed attempt and locks the key once the threshold is reached.
func (rl *totpLoginRateLimit) recordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	e, ok := rl.entries[key]
	if !ok {
		e = &totpRateLimitEntry{}
		rl.entries[key] = e
	}

	// keep only failures within the window
	cutoff := now.Add(-totpRateLimitWindow)
	kept := e.failures[:0]
	for _, f := range e.failures {
		if f.After(cutoff) {
			kept = append(kept, f)
		}
	}
	e.failures = append(kept, now)

	if len(e.failures) >= totpRateLimitMaxAttempts {
		e.lockedTill = now.Add(totpRateLimitWindow)
		e.failures = e.failures[:0] // reset counter after lockout
	}
}

// reset clears the state for the key (called after a successful second factor).
func (rl *totpLoginRateLimit) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, key)
}

// cleanupLoop periodically purges stale entries to bound memory usage.
func (rl *totpLoginRateLimit) cleanupLoop() {
	ticker := time.NewTicker(totpRateLimitCleanup)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-totpRateLimitWindow)
		for key, e := range rl.entries {
			if time.Now().After(e.lockedTill) && len(e.failures) == 0 {
				delete(rl.entries, key)
				continue
			}
			// drop fully-expired failure lists
			allExpired := true
			for _, f := range e.failures {
				if f.After(cutoff) {
					allExpired = false
					break
				}
			}
			if allExpired && time.Now().After(e.lockedTill) {
				delete(rl.entries, key)
			}
		}
		rl.mu.Unlock()
	}
}

// totpRateLimiter is the package-level limiter shared by the auth endpoint.
var totpRateLimiter = newTotpLoginRateLimit()
