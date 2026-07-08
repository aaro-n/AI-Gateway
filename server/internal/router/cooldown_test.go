package router

import (
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	// Starts closed
	if !cm.AllowRequest(pid, pmid) {
		t.Fatal("expected closed circuit to allow requests")
	}

	// Directly set state to bypass RecordInterval dedup
	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	state.ConsecutiveErrors = CooldownThreshold
	cm.triggerCircuitOpen(state, time.Now())

	if cm.AllowRequest(pid, pmid) {
		t.Fatal("expected open circuit to deny requests")
	}
	if !cm.IsCooldown(pid, pmid) {
		t.Fatal("expected cooldown to be active")
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	// Force open
	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	state.ConsecutiveErrors = CooldownThreshold
	cm.triggerCircuitOpen(state, time.Now())

	if cm.AllowRequest(pid, pmid) {
		t.Fatal("expected open circuit to deny")
	}

	// Manually expire cooldown to transition to half-open
	past := time.Now().Add(-1 * time.Hour)
	state.CooldownUntil = &past
	state.CircuitState = CircuitOpen

	// Should now be half-open (AllowRequest returns true for first probe)
	cm.IsCooldown(pid, pmid) // triggers transition
	if !cm.AllowRequest(pid, pmid) {
		t.Fatal("expected half-open circuit to allow probe request")
	}

	// Success closes the circuit
	cm.RecordSuccess(pid, pmid)
	if !cm.AllowRequest(pid, pmid) {
		t.Fatal("expected closed circuit after success")
	}
	if cm.IsCooldown(pid, pmid) {
		t.Fatal("expected no cooldown after success")
	}
}

func TestCircuitBreaker_Record429Counts(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	// Set combined count directly (bypass RecordInterval)
	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	state.Consecutive429 = 2
	state.ConsecutiveErrors = 1
	cm.triggerCircuitOpen(state, time.Now())

	if cm.AllowRequest(pid, pmid) {
		t.Fatal("expected open after 3 combined errors")
	}
}

func TestCircuitBreaker_RecordIntervalDedup(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	// Rapid errors within RecordInterval should only count once
	cm.RecordError(pid, pmid)
	cm.RecordError(pid, pmid) // should be deduped

	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	if state.ConsecutiveErrors != 1 {
		t.Fatalf("expected 1 error after dedup, got %d", state.ConsecutiveErrors)
	}
}

func TestCircuitBreaker_SuccessResetsErrorCount(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	cm.RecordError(pid, pmid)
	cm.RecordSuccess(pid, pmid)

	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	if state.ConsecutiveErrors != 0 {
		t.Fatalf("expected 0 errors after success, got %d", state.ConsecutiveErrors)
	}
	if state.CircuitState != CircuitClosed {
		t.Fatal("expected closed after success reset")
	}
}

func TestCircuitBreaker_ClearCooldown(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	// Force open
	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	state.ConsecutiveErrors = CooldownThreshold
	cm.triggerCircuitOpen(state, time.Now())

	if cm.AllowRequest(pid, pmid) {
		t.Fatal("expected open")
	}

	cm.ClearCooldown(pid, pmid)
	if !cm.AllowRequest(pid, pmid) {
		t.Fatal("expected closed after manual clear")
	}
}

func TestCircuitBreaker_ClearAllForProvider(t *testing.T) {
	cm := NewCooldownManager()
	pid := uint(1)

	// Force both models open
	for _, pmid := range []uint{1, 2} {
		key := cm.getKey(pid, pmid)
		state := cm.getState(key)
		state.ConsecutiveErrors = CooldownThreshold
		cm.triggerCircuitOpen(state, time.Now())
	}

	if cm.AllowRequest(pid, 1) {
		t.Fatal("expected model 1 open")
	}
	if cm.AllowRequest(pid, 2) {
		t.Fatal("expected model 2 open")
	}

	cm.ClearAllForProvider(pid)
	if !cm.AllowRequest(pid, 1) {
		t.Fatal("expected model 1 closed after clear all")
	}
	if !cm.AllowRequest(pid, 2) {
		t.Fatal("expected model 2 closed after clear all")
	}
}

func TestCooldownEndTimes(t *testing.T) {
	cm := NewCooldownManager()
	pid, pmid := uint(1), uint(1)

	endTime := cm.GetCooldownEndTime(pid, pmid)
	if endTime != nil {
		t.Fatal("expected nil cooldown end time for fresh state")
	}

	// Force open
	key := cm.getKey(pid, pmid)
	state := cm.getState(key)
	state.ConsecutiveErrors = CooldownThreshold
	cm.triggerCircuitOpen(state, time.Now())

	endTime = cm.GetCooldownEndTime(pid, pmid)
	if endTime == nil {
		t.Fatal("expected cooldown end time after open")
	}
	if !endTime.After(time.Now()) {
		t.Fatal("cooldown end should be in the future")
	}
}
