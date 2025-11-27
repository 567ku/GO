package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestLeaseScan_DedupLevel_WhenEntryAndTPExpired 验证：同一level的Entry和TP同时超时时，levelID只被处理一次
func TestLeaseScan_DedupLevel_WhenEntryAndTPExpired(t *testing.T) {
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

	// 初始化一个level，Entry和TP都超时
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    123456,
				UpdateAtMs: model.NowMs() - 200, // 200ms前，已超时
				Attempt:    1,                   // 已重试1次
			},
			TP: model.OrderSlot{
				Purpose:    model.OrderPurposeTP,
				State:      model.OrderStateSubmitted,
				OrderID:    789012,
				UpdateAtMs: model.NowMs() - 200, // 200ms前，也超时
				Attempt:    1,                   // 已重试1次
			},
		},
	}

	// 手动触发LeaseScan
	engine.handleTimer(model.TimerEvent{
		Kind: model.TimerKindLeaseScan,
		AtMs: model.NowMs(),
	})

	// 验证：Entry和TP的Attempt都递增到2（说明都被处理了）
	if engine.state.Levels[0].Entry.Attempt != 2 {
		t.Errorf("expected Entry.Attempt=2, got %d", engine.state.Levels[0].Entry.Attempt)
	}
	if engine.state.Levels[0].TP.Attempt != 2 {
		t.Errorf("expected TP.Attempt=2, got %d", engine.state.Levels[0].TP.Attempt)
	}

	// 验证：不会触发FREEZE（如果没去重，attempt会变成3，接近上限）
	if engine.state.EngineMode == model.EngineModeFreeze {
		t.Errorf("should not trigger FREEZE with attempt=2")
	}
}

// TestLeaseScan_DedupLevel_NoDoubleFreeze 验证：Entry和TP同时超时且都达到上限时，不会因为重复计数而误判
func TestLeaseScan_DedupLevel_NoDoubleFreeze(t *testing.T) {
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

	// 初始化一个level，Entry和TP都达到上限-1（第3次重试）
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    123456,
				UpdateAtMs: model.NowMs() - 200, // 200ms前，已超时
				Attempt:    3,                   // 已达上限
			},
			TP: model.OrderSlot{
				Purpose:    model.OrderPurposeTP,
				State:      model.OrderStateSubmitted,
				OrderID:    789012,
				UpdateAtMs: model.NowMs() - 200, // 200ms前，也超时
				Attempt:    3,                   // 已达上限
			},
		},
	}

	// 手动触发LeaseScan
	engine.handleTimer(model.TimerEvent{
		Kind: model.TimerKindLeaseScan,
		AtMs: model.NowMs(),
	})

	// 验证：Entry和TP的Attempt都递增到4（只递增一次）
	if engine.state.Levels[0].Entry.Attempt != 4 {
		t.Errorf("expected Entry.Attempt=4, got %d", engine.state.Levels[0].Entry.Attempt)
	}
	if engine.state.Levels[0].TP.Attempt != 4 {
		t.Errorf("expected TP.Attempt=4, got %d", engine.state.Levels[0].TP.Attempt)
	}

	// 验证：触发FREEZE（因为超限）
	if engine.state.EngineMode != model.EngineModeFreeze {
		t.Errorf("expected FREEZE, got %v", engine.state.EngineMode)
	}

	// 关键验证：如果没去重，attempt会变成5或6，这里应该只是4
	// （这个测试确保去重逻辑生效，避免"双倍计数"导致误判）
}

// TestLeaseScan_DedupLevel_OnlyEntry 验证：只有Entry超时，TP正常的场景
func TestLeaseScan_DedupLevel_OnlyEntry(t *testing.T) {
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

	// 初始化一个level，只有Entry超时
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    123456,
				UpdateAtMs: model.NowMs() - 200, // 200ms前，已超时
				Attempt:    1,
			},
			TP: model.OrderSlot{
				Purpose: model.OrderPurposeTP,
				State:   model.OrderStateNone, // TP没有订单
				Attempt: 0,
			},
		},
	}

	// 手动触发LeaseScan
	engine.handleTimer(model.TimerEvent{
		Kind: model.TimerKindLeaseScan,
		AtMs: model.NowMs(),
	})

	// 验证：Entry的Attempt递增到2
	if engine.state.Levels[0].Entry.Attempt != 2 {
		t.Errorf("expected Entry.Attempt=2, got %d", engine.state.Levels[0].Entry.Attempt)
	}

	// 验证：TP的Attempt不变
	if engine.state.Levels[0].TP.Attempt != 0 {
		t.Errorf("expected TP.Attempt=0, got %d", engine.state.Levels[0].TP.Attempt)
	}
}
