package ws

import (
	"testing"
	"time"

	"gridbot/pkg/model"
)

// TestNewWSStateEvent_ReasonField 测试 NewWSStateEvent 必须携带 Reason 字段（P0-2.3a）
func TestNewWSStateEvent_ReasonField(t *testing.T) {
	tests := []struct {
		name    string
		channel model.WSChannel
		state   model.WSState
		reason  string
	}{
		{
			name:    "CONNECTED with fixed reason",
			channel: model.WSChannelMarket,
			state:   model.WSStateConnected,
			reason:  "connected",
		},
		{
			name:    "DISCONNECTED with error reason",
			channel: model.WSChannelUDS,
			state:   model.WSStateDisconnected,
			reason:  "dial timeout",
		},
		{
			name:    "RECONNECTED with fixed reason",
			channel: model.WSChannelTrade,
			state:   model.WSStateReconnected,
			reason:  "reconnected",
		},
		{
			name:    "DISCONNECTED with detailed reason",
			channel: model.WSChannelMarket,
			state:   model.WSStateDisconnected,
			reason:  "connection lost: EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := NewWSStateEvent(tt.channel, tt.state, tt.reason)

			// 验证 Reason 字段必须落地
			if evt.Reason != tt.reason {
				t.Errorf("NewWSStateEvent() Reason = %v, want %v", evt.Reason, tt.reason)
			}

			// 验证其他字段
			if evt.Channel != tt.channel {
				t.Errorf("NewWSStateEvent() Channel = %v, want %v", evt.Channel, tt.channel)
			}

			if evt.State != tt.state {
				t.Errorf("NewWSStateEvent() State = %v, want %v", evt.State, tt.state)
			}

			// 验证 AtMs 必须非零
			if evt.AtMs == 0 {
				t.Error("NewWSStateEvent() AtMs should not be zero")
			}
		})
	}
}

// TestNewWSStateEvent_EmptyReason 测试空 Reason 也应该被正确设置（防御性测试）
func TestNewWSStateEvent_EmptyReason(t *testing.T) {
	evt := NewWSStateEvent(model.WSChannelMarket, model.WSStateConnected, "")

	// 即使传入空字符串，也应该设置 Reason 字段（不应该被忽略）
	if evt.Reason != "" {
		t.Errorf("NewWSStateEvent() with empty reason, Reason = %v, want empty string", evt.Reason)
	}
}

// TestCalculateBackoffWithJitter 测试退避算法（带 jitter）
func TestCalculateBackoffWithJitter(t *testing.T) {
	base := 1000 * time.Millisecond
	max := 60000 * time.Millisecond

	tests := []struct {
		name    string
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "attempt 1",
			attempt: 1,
			wantMin: time.Duration(float64(base) * 0.8), // 800ms
			wantMax: time.Duration(float64(base) * 1.2), // 1200ms
		},
		{
			name:    "attempt 2",
			attempt: 2,
			wantMin: time.Duration(float64(2*base) * 0.8), // 1600ms
			wantMax: time.Duration(float64(2*base) * 1.2), // 2400ms
		},
		{
			name:    "attempt 7 (should hit max)",
			attempt: 7,
			wantMin: time.Duration(float64(max) * 0.8), // 48s
			wantMax: time.Duration(float64(max) * 1.2), // 72s (但会被限制到max)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := calculateBackoffWithJitter(base, max, tt.attempt)

			// 验证 jitter 范围
			if backoff < tt.wantMin || backoff > tt.wantMax {
				t.Errorf("calculateBackoffWithJitter() = %v, want range [%v, %v]", backoff, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestCalculateBackoffWithJitter_ZeroAttempt 测试零或负数 attempt
func TestCalculateBackoffWithJitter_ZeroAttempt(t *testing.T) {
	base := 1000 * time.Millisecond
	max := 60000 * time.Millisecond

	// 零或负数 attempt 应该被视为 1
	backoff := calculateBackoffWithJitter(base, max, 0)

	wantMin := time.Duration(float64(base) * 0.8)
	wantMax := time.Duration(float64(base) * 1.2)

	if backoff < wantMin || backoff > wantMax {
		t.Errorf("calculateBackoffWithJitter(attempt=0) = %v, want range [%v, %v]", backoff, wantMin, wantMax)
	}
}
