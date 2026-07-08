package middleware

import (
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, 1*time.Second)
	keyID := uint(1)

	if !rl.Allow(keyID) {
		t.Fatal("expected first request allowed")
	}
	if !rl.Allow(keyID) {
		t.Fatal("expected second request allowed")
	}
	if !rl.Allow(keyID) {
		t.Fatal("expected third request allowed")
	}

	// 4th should be denied
	if rl.Allow(keyID) {
		t.Fatal("expected fourth request denied (limit=3)")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)
	keyID := uint(1)

	rl.Allow(keyID)
	rl.Allow(keyID)
	if rl.Allow(keyID) {
		t.Fatal("expected third request denied")
	}

	// Wait for window to pass
	time.Sleep(60 * time.Millisecond)

	if !rl.Allow(keyID) {
		t.Fatal("expected request allowed after window expiry")
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(2, 1*time.Second)

	// Fill up key 1
	rl.Allow(1)
	rl.Allow(1)
	if rl.Allow(1) {
		t.Fatal("expected key 1 denied")
	}

	// Key 2 should still be allowed
	if !rl.Allow(2) {
		t.Fatal("expected key 2 allowed (separate rate limit)")
	}
	if !rl.Allow(2) {
		t.Fatal("expected key 2 second request allowed")
	}
}

func TestRateLimiter_ZeroLimit(t *testing.T) {
	rl := NewRateLimiter(0, 1*time.Second)
	// With maxReq=0, no requests should be allowed
	if rl.Allow(1) {
		t.Fatal("expected deny with zero limit")
	}
}

func TestGlobalRateLimiter(t *testing.T) {
	SetGlobalRateLimiter(5, 1*time.Second)
	handler := GlobalRateLimiter()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	SetGlobalRateLimiter(0, 1*time.Second)
	handler = GlobalRateLimiter()
	_ = handler
}
