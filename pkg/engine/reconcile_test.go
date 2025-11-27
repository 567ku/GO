// P0-RET-03: Reconcile单元测试
package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestApplyOrderSnapshot_Reducer 测试订单快照应用逻辑（通过reducer）
func TestApplyOrderSnapshot_Reducer(t *testing.T) {
	prefix := "TEST"

	testCases := []struct {
		name           string
		localState     model.OrderState
		exchangeStatus string
		expectedState  model.OrderState
	}{
		{"本地NONE+交易所NEW", model.OrderStateNone, "NEW", model.OrderStateOpen},
		{"本地SUBMITTED+交易所FILLED", model.OrderStateSubmitted, "FILLED", model.OrderStateFilled},
		{"本地OPEN+交易所CANCELED", model.OrderStateOpen, "CANCELED", model.OrderStateCanceled},
		{"本地PARTIAL+交易所FILLED", model.OrderStatePartial, "FILLED", model.OrderStateFilled},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := &model.GridStateSnapshot{
				Prefix:    prefix,
				CLIDIndex: make(map[string]int64),
				Levels: []model.LevelState{
					{
						LevelID:    100,
						PriceTicks: 50000000,
						Cycle:      1,
						Entry: model.OrderSlot{
							Purpose: model.OrderPurposeEntry,
							State:   tc.localState,
						},
					},
				},
			}

			orders := []model.ParsedOrderUpdate{
				{
					ClientOrderID:    "TEST:E:100:1",
					OrderID:          123456,
					Status:           tc.exchangeStatus,
					PriceTicks:       50000000,
					OrigQtyTicks:     1000000,
					ExecutedQtyTicks: 1000000,
					AvgPriceTicks:    50000000,
				},
			}

			// 应用快照（通过reducer）
			state, _ = ApplyOpenOrdersSnapshot(state, orders, prefix)

			if state.Levels[0].Entry.State != tc.expectedState {
				t.Errorf("expected State=%v, got %v", tc.expectedState, state.Levels[0].Entry.State)
			}
			if state.Levels[0].Entry.OrderID != 123456 {
				t.Errorf("expected OrderID=123456, got %d", state.Levels[0].Entry.OrderID)
			}
			if state.Levels[0].Entry.LastSource != model.EventSourceQuery {
				t.Errorf("expected LastSource=QUERY, got %v", state.Levels[0].Entry.LastSource)
			}
		})
	}
}

// TestApplyOpenOrdersSnapshot_Reducer 测试开仓订单快照应用（通过reducer）
func TestApplyOpenOrdersSnapshot_Reducer(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:    "TEST",
		Levels:    make([]model.LevelState, 0),
		CLIDIndex: make(map[string]int64),
		Window: model.WindowState{
			WinMinTicks:         49000000,
			WinMaxTicks:         51000000,
			StepTicks:           100000, // 避免除零
			TPKeepOutsideLevels: 5,
		},
	}

	// 模拟交易所返回的开仓订单
	orders := []model.ParsedOrderUpdate{
		{
			ClientOrderID:    "TEST:E:100:1",
			OrderID:          123456,
			Status:           "NEW",
			PriceTicks:       50000000,
			OrigQtyTicks:     1000000,
			ExecutedQtyTicks: 0,
			AvgPriceTicks:    0,
		},
		{
			ClientOrderID:    "TEST:E:101:1",
			OrderID:          123457,
			Status:           "PARTIALLY_FILLED",
			PriceTicks:       50100000,
			OrigQtyTicks:     1000000,
			ExecutedQtyTicks: 500000,
			AvgPriceTicks:    50100000,
		},
	}

	// 应用快照（通过reducer）
	state, _ = ApplyOpenOrdersSnapshot(state, orders, "TEST")

	// 验证状态
	if len(state.Levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(state.Levels))
	}

	// 验证level 100
	level100 := FindLevelByID(state, 100)
	if level100 == nil {
		t.Fatal("level 100 not found")
	}
	if level100.Entry.State != model.OrderStateOpen {
		t.Errorf("level 100 Entry: expected State=OPEN, got %v", level100.Entry.State)
	}
	if level100.Entry.OrderID != 123456 {
		t.Errorf("level 100 Entry: expected OrderID=123456, got %d", level100.Entry.OrderID)
	}

	// 验证level 101
	level101 := FindLevelByID(state, 101)
	if level101 == nil {
		t.Fatal("level 101 not found")
	}
	if level101.Entry.State != model.OrderStatePartial {
		t.Errorf("level 101 Entry: expected State=PARTIAL, got %v", level101.Entry.State)
	}
	if level101.Entry.ExecutedQtyTicks != 500000 {
		t.Errorf("level 101 Entry: expected ExecutedQtyTicks=500000, got %d", level101.Entry.ExecutedQtyTicks)
	}
}

// TestReconcileTransitionsToRunning 测试reconcile流程最终回到RUNNING
func TestReconcileTransitionsToRunning(t *testing.T) {
	state := &model.GridStateSnapshot{
		EngineMode: model.EngineModeReconciling,
		Prefix:     "TEST",
		Levels:     make([]model.LevelState, 0),
		CLIDIndex:  make(map[string]int64),
	}

	e := &Engine{
		state: state,
		config: EngineConfig{
			Prefix: "TEST",
		},
	}

	// 模拟reconcile完成
	e.completeReconcile()

	// 验证状态
	if e.state.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING, got %v", e.state.EngineMode)
	}

	if e.GetMode() != model.EngineModeRunning {
		t.Errorf("expected Engine.mode=RUNNING, got %v", e.GetMode())
	}
}

// TestApplyReconcileStart reducer测试
func TestApplyReconcileStart(t *testing.T) {
	state := &model.GridStateSnapshot{
		EngineMode: model.EngineModeFreeze,
	}

	newState, _ := ApplyReconcileStart(state)

	if newState.EngineMode != model.EngineModeReconciling {
		t.Errorf("expected EngineMode=RECONCILING, got %v", newState.EngineMode)
	}
}

// TestApplyReconcileComplete reducer测试
func TestApplyReconcileComplete(t *testing.T) {
	state := &model.GridStateSnapshot{
		EngineMode: model.EngineModeReconciling,
	}

	newState, _ := ApplyReconcileComplete(state)

	if newState.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING, got %v", newState.EngineMode)
	}
}
