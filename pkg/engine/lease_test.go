// P0-E-05-A: LeaseScan定时器测试
package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestEngine_LeaseScanTimer_ReclaimsExpired 测试LeaseScan定时器回收超时任务
func TestEngine_LeaseScanTimer_ReclaimsExpired(t *testing.T) {
	// 创建Engine
	cfg := EngineConfig{
		Prefix:        "TEST",
		Symbol:        "BTCUSDT",
		Side:          model.GridSideLong,
		StepTicks:     100000,
		WinMinTicks:   49000000,
		WinMaxTicks:   51000000,
		EntryQtyTicks: 1000,
		LeasePlaceMs:  100, // 100ms超时（测试用）
		MaxAttempt:    3,   // 最大重试3次
		EventChSize:   100,
	}
	engine := NewEngine(cfg)

	// 初始化一个level，状态为SUBMITTED（模拟INFLIGHT）
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    123456,
				UpdateAtMs: model.NowMs() - 200, // 200ms前提交，已超时
				Attempt:    1,                   // 已重试1次
			},
		},
	}

	// 手动触发LeaseScan事件（不等定时器）
	engine.handleTimer(model.TimerEvent{
		Kind: model.TimerKindLeaseScan,
		AtMs: model.NowMs(),
	})

	// 验证：Entry应该被回收到NONE，Attempt递增到2
	if engine.state.Levels[0].Entry.State != model.OrderStateNone {
		t.Errorf("expected Entry.State=NONE, got %v", engine.state.Levels[0].Entry.State)
	}
	if engine.state.Levels[0].Entry.Attempt != 2 {
		t.Errorf("expected Entry.Attempt=2, got %d", engine.state.Levels[0].Entry.Attempt)
	}
}

// TestEngine_LeaseScanTimer_MaxAttemptFreeze 测试超过maxAttempt触发FREEZE
func TestEngine_LeaseScanTimer_MaxAttemptFreeze(t *testing.T) {
	// 创建Engine
	cfg := EngineConfig{
		Prefix:        "TEST",
		Symbol:        "BTCUSDT",
		Side:          model.GridSideLong,
		StepTicks:     100000,
		WinMinTicks:   49000000,
		WinMaxTicks:   51000000,
		EntryQtyTicks: 1000,
		LeasePlaceMs:  100, // 100ms超时
		MaxAttempt:    3,   // 最大重试3次
		EventChSize:   100,
	}
	engine := NewEngine(cfg)

	// 初始化一个level，Attempt已达上限
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    123456,
				UpdateAtMs: model.NowMs() - 200,
				Attempt:    3, // 已达上限
			},
		},
	}

	// 手动触发LeaseScan（不等待真实定时器）
	engine.handleTimer(model.TimerEvent{
		Kind: model.TimerKindLeaseScan,
		AtMs: model.NowMs(),
	})

	// 验证：Attempt递增到4，触发FREEZE
	if engine.state.Levels[0].Entry.Attempt != 4 {
		t.Errorf("expected Entry.Attempt=4, got %d", engine.state.Levels[0].Entry.Attempt)
	}
	if engine.state.EngineMode != model.EngineModeFreeze {
		t.Errorf("expected EngineMode=FREEZE, got %v", engine.state.EngineMode)
	}
}

// TestEngine_LeaseScanTimer_NoTimeout 测试无超时任务场景
func TestEngine_LeaseScanTimer_NoTimeout(t *testing.T) {
	// 创建Engine
	cfg := EngineConfig{
		Prefix:        "TEST",
		Symbol:        "BTCUSDT",
		Side:          model.GridSideLong,
		StepTicks:     100000,
		WinMinTicks:   49000000,
		WinMaxTicks:   51000000,
		EntryQtyTicks: 1000,
		LeasePlaceMs:  100,
		MaxAttempt:    3,
		EventChSize:   100,
	}
	engine := NewEngine(cfg)

	// 初始化一个level，设置为未来时间（确保不会超时）
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    123456,
				UpdateAtMs: model.NowMs() + 10000, // 未来10秒，绝对不会超时
				Attempt:    1,
			},
		},
	}

	// 直接触发LeaseScan（不等待真实定时器）
	engine.handleTimer(model.TimerEvent{
		Kind: model.TimerKindLeaseScan,
		AtMs: model.NowMs(),
	})

	// 验证：状态不变
	if engine.state.Levels[0].Entry.State != model.OrderStateSubmitted {
		t.Errorf("expected Entry.State=SUBMITTED, got %v", engine.state.Levels[0].Entry.State)
	}
	if engine.state.Levels[0].Entry.Attempt != 1 {
		t.Errorf("expected Entry.Attempt=1, got %d", engine.state.Levels[0].Entry.Attempt)
	}
}
