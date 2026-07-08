package router

import (
	"fmt"
	"sync"
	"time"
)

const (
	// Cooldown / Circuit Breaker defaults
	CooldownThreshold  = 3 // 连续失败次数阈值
	CooldownDuration   = 30 * time.Minute
	RecordInterval     = 3 * time.Second // 最小记录间隔，防止瞬时峰值触发
	HalfOpenMaxRetries = 1               // 半开状态允许的探测请求数
)

// CircuitState 熔断器状态
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // 正常
	CircuitOpen                         // 熔断打开，拒绝请求
	CircuitHalfOpen                     // 半开，允许少量探测
)

// CooldownState 冷却/熔断状态
type CooldownState struct {
	Consecutive429    int
	ConsecutiveErrors int // 通用错误计数（含 5xx, 超时等）
	CooldownUntil     *time.Time
	Last429Time       *time.Time
	LastErrorTime     *time.Time
	CircuitState      CircuitState // 熔断器状态
	HalfOpenAttempts  int          // 半开状态下已尝试的请求数
}

// CooldownManager 管理 Provider-Model 级别的冷却与熔断。
// 同时实现了 Circuit Breaker 模式：
//   - Closed → 连续错误超过阈值 → Open
//   - Open → 冷却时间过后 → HalfOpen
//   - HalfOpen → 探测成功 → Closed; 探测失败 → Open
type CooldownManager struct {
	mu     sync.RWMutex
	states map[string]*CooldownState
}

func NewCooldownManager() *CooldownManager {
	return &CooldownManager{
		states: make(map[string]*CooldownState),
	}
}

func (m *CooldownManager) getKey(providerID uint, providerModelID uint) string {
	return fmt.Sprintf("%d:%d", providerID, providerModelID)
}

func (m *CooldownManager) getState(key string) *CooldownState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, exists := m.states[key]; exists {
		return state
	}
	state := &CooldownState{
		Consecutive429:    0,
		ConsecutiveErrors: 0,
		CooldownUntil:     nil,
		Last429Time:       nil,
		LastErrorTime:     nil,
		CircuitState:      CircuitClosed,
		HalfOpenAttempts:  0,
	}
	m.states[key] = state
	return state
}

// AllowRequest 检查是否允许请求通过（熔断器准入）。
// 返回 true 表示允许，false 表示被熔断拒绝。
func (m *CooldownManager) AllowRequest(providerID uint, providerModelID uint) bool {
	key := m.getKey(providerID, providerModelID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if state, exists := m.states[key]; exists {
		switch state.CircuitState {
		case CircuitClosed:
			return true
		case CircuitHalfOpen:
			return state.HalfOpenAttempts < HalfOpenMaxRetries
		case CircuitOpen:
			if state.CooldownUntil != nil && time.Now().After(*state.CooldownUntil) {
				return false // 将在 IsCooldown 中处理状态转换
			}
			return false
		}
	}
	return true
}

// IsCooldown 检查是否处于冷却期
func (m *CooldownManager) IsCooldown(providerID uint, providerModelID uint) bool {
	key := m.getKey(providerID, providerModelID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, exists := m.states[key]; exists {
		if state.CooldownUntil != nil && time.Now().Before(*state.CooldownUntil) {
			return true
		}
		// 冷却期已过但处于 Open 状态 → 转为 HalfOpen
		if state.CircuitState == CircuitOpen && state.CooldownUntil != nil && time.Now().After(*state.CooldownUntil) {
			state.CircuitState = CircuitHalfOpen
			state.HalfOpenAttempts = 0
		}
	}
	return false
}

// Record429 记录上游 429 限流错误
func (m *CooldownManager) Record429(providerID uint, providerModelID uint) {
	key := m.getKey(providerID, providerModelID)
	state := m.getState(key)

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if state.Last429Time != nil && now.Sub(*state.Last429Time) < RecordInterval {
		return
	}

	state.Last429Time = &now
	state.Consecutive429++
	m.triggerCircuitOpen(state, now)
}

// RecordError 记录通用上游错误（5xx, 超时等），触发熔断计数。
func (m *CooldownManager) RecordError(providerID uint, providerModelID uint) {
	key := m.getKey(providerID, providerModelID)
	state := m.getState(key)

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if state.LastErrorTime != nil && now.Sub(*state.LastErrorTime) < RecordInterval {
		return
	}

	state.LastErrorTime = &now
	state.ConsecutiveErrors++
	m.triggerCircuitOpen(state, now)
}

// triggerCircuitOpen 当错误计数超过阈值时打开熔断器
func (m *CooldownManager) triggerCircuitOpen(state *CooldownState, now time.Time) {
	totalErrors := state.Consecutive429 + state.ConsecutiveErrors
	if totalErrors >= CooldownThreshold {
		cooldownUntil := now.Add(CooldownDuration)
		state.CooldownUntil = &cooldownUntil
		state.CircuitState = CircuitOpen
		state.HalfOpenAttempts = 0
	}
}

// RecordSuccess 记录成功请求，重置错误计数
func (m *CooldownManager) RecordSuccess(providerID uint, providerModelID uint) {
	key := m.getKey(providerID, providerModelID)
	state := m.getState(key)

	m.mu.Lock()
	defer m.mu.Unlock()

	state.Consecutive429 = 0
	state.ConsecutiveErrors = 0
	state.Last429Time = nil
	state.LastErrorTime = nil

	// 半开状态探测成功 → 关闭熔断器
	if state.CircuitState == CircuitHalfOpen {
		state.CircuitState = CircuitClosed
		state.CooldownUntil = nil
		state.HalfOpenAttempts = 0
	}
}

// RecordRateLimit 记录限流并用于路由（别名，保持兼容）
func (m *CooldownManager) RecordRateLimit(providerID uint, providerModelID uint) {
	m.Record429(providerID, providerModelID)
}

// GetCooldownEndTime 获取冷却结束时间
func (m *CooldownManager) GetCooldownEndTime(providerID uint, providerModelID uint) *time.Time {
	key := m.getKey(providerID, providerModelID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if state, exists := m.states[key]; exists {
		return state.CooldownUntil
	}
	return nil
}

// GetEarliestCooldownEnd 获取最早结束冷却的 Provider
func (m *CooldownManager) GetEarliestCooldownEnd(providers []RouteResult) *RouteResult {
	var earliest *RouteResult
	var earliestTime *time.Time

	for _, result := range providers {
		endTime := m.GetCooldownEndTime(result.Provider.ID, result.ProviderModel.ID)
		if endTime != nil {
			if earliestTime == nil || endTime.Before(*earliestTime) {
				earliestTime = endTime
				earliest = &result
			}
		}
	}

	return earliest
}

// ClearExpiredCooldowns 清理已过期的冷却状态
func (m *CooldownManager) ClearExpiredCooldowns() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, state := range m.states {
		if state.CooldownUntil != nil && now.After(*state.CooldownUntil) {
			if state.CircuitState == CircuitOpen {
				state.CircuitState = CircuitHalfOpen
				state.HalfOpenAttempts = 0
			} else {
				state.CooldownUntil = nil
				state.Consecutive429 = 0
				state.ConsecutiveErrors = 0
				state.CircuitState = CircuitClosed
			}
		}
	}
}

// ClearCooldown 手动清除指定 Provider-Model 的冷却
func (m *CooldownManager) ClearCooldown(providerID uint, providerModelID uint) {
	key := m.getKey(providerID, providerModelID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, exists := m.states[key]; exists {
		state.CooldownUntil = nil
		state.Consecutive429 = 0
		state.ConsecutiveErrors = 0
		state.Last429Time = nil
		state.LastErrorTime = nil
		state.CircuitState = CircuitClosed
		state.HalfOpenAttempts = 0
	}
}

// ClearAllForProvider 清除指定 Provider 的所有冷却
func (m *CooldownManager) ClearAllForProvider(providerID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := fmt.Sprintf("%d:", providerID)
	for key, state := range m.states {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			state.CooldownUntil = nil
			state.Consecutive429 = 0
			state.ConsecutiveErrors = 0
			state.Last429Time = nil
			state.LastErrorTime = nil
			state.CircuitState = CircuitClosed
			state.HalfOpenAttempts = 0
		}
	}
}
