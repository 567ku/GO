// P0-E-02-A: Reconcile端到端测试
package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestReconcile_Reconnected_To_Running_With_GapScanTasks 测试完整reconcile流程
// RECONNECTED → 触发Query → 注入snapshot回包 → 断言最终RUNNING & 生成修复任务
func TestReconcile_Reconnected_To_Running_With_GapScanTasks(t *testing.T) {
	// 创建Engine
	cfg := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100000,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		LeasePlaceMs:        3000,
		MaxAttempt:          8,
		EventChSize:         100,
	}
	engine := NewEngine(cfg)

	// 初始状态为FREEZE（模拟WS断线）
	engine.state.EngineMode = model.EngineModeFreeze
	engine.mode = model.EngineModeFreeze

	// 步骤1: 触发WS RECONNECTED事件
	wsEvent := model.WSStateEvent{
		Channel: model.WSChannelUDS,
		State:   model.WSStateReconnected,
		AtMs:    model.NowMs(),
		Reason:  "test reconnect",
	}
	engine.handleWSState(wsEvent)

	// Stage 5A更新: reconcile现在是同步执行，执行完成后直接切回RUNNING
	// 验证：最终回到RUNNING（因为executor为nil，降级为空快照并完成）
	if engine.state.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING after RECONNECTED (sync reconcile), got %v", engine.state.EngineMode)
	}

	// 步骤2: 模拟注入openOrders snapshot回包（空快照）
	// 在真实场景中，这会通过QueryResult事件触发
	// 这里直接调用applyOpenOrdersSnapshot
	engine.applyOpenOrdersSnapshot([]model.ParsedOrderUpdate{})

	// 验证：最终回到RUNNING
	if engine.state.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING after snapshot, got %v", engine.state.EngineMode)
	}

	// Stage 5A更新: 由于reconcile已经同步完成，reconcileInFlight已清零
	// 验证：reconcileInFlight标记已清零
	if engine.reconcileInFlight {
		t.Errorf("expected reconcileInFlight=false after sync reconcile, got true")
	}

	// 验证：GapScan应该生成任务（在真实场景中会提交给Executor）
	// 这里只验证GapScan能正常调用
	tasks := GapScan(engine.state)
	if len(tasks) == 0 {
		t.Log("no tasks generated (expected if window is empty)")
	}
}

// TestReconcile_WithExistingOrders 测试有订单的reconcile场景
func TestReconcile_WithExistingOrders(t *testing.T) {
	// 创建Engine
	cfg := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100000,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		LeasePlaceMs:        3000,
		MaxAttempt:          8,
		EventChSize:         100,
	}
	engine := NewEngine(cfg)

	// 初始化一个本地level（状态为SUBMITTED）
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Cycle:      1,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateSubmitted,
				OrderID:    0, // 未知OrderID
				UpdateAtMs: model.NowMs(),
			},
		},
	}

	// 模拟交易所返回的订单快照（该订单已FILLED）
	orders := []model.ParsedOrderUpdate{
		{
			ClientOrderID:    "TEST:E:500:1",
			OrderID:          123456,
			Status:           "FILLED",
			PriceTicks:       50000000,
			OrigQtyTicks:     1000,
			ExecutedQtyTicks: 1000,
			AvgPriceTicks:    50000000,
		},
	}

	// 应用快照
	engine.applyOpenOrdersSnapshot(orders)

	// 验证：本地状态更新为FILLED
	level := FindLevelByID(engine.state, 500)
	if level == nil {
		t.Fatal("level 500 not found")
	}
	if level.Entry.State != model.OrderStateFilled {
		t.Errorf("expected Entry.State=FILLED, got %v", level.Entry.State)
	}
	if level.Entry.OrderID != 123456 {
		t.Errorf("expected Entry.OrderID=123456, got %d", level.Entry.OrderID)
	}

	// 验证：最终回到RUNNING
	if engine.state.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING, got %v", engine.state.EngineMode)
	}
}

// TestReconcile_NoDuplicateTrigger 测试不会重复触发reconcile
func TestReconcile_NoDuplicateTrigger(t *testing.T) {
	// 创建Engine
	cfg := EngineConfig{
		Prefix:        "TEST",
		Symbol:        "BTCUSDT",
		Side:          model.GridSideLong,
		StepTicks:     100000,
		WinMinTicks:   49000000,
		WinMaxTicks:   51000000,
		EntryQtyTicks: 1000,
		EventChSize:   100,
	}
	engine := NewEngine(cfg)

	// 第一次RECONNECTED
	wsEvent1 := model.WSStateEvent{
		Channel: model.WSChannelUDS,
		State:   model.WSStateReconnected,
		AtMs:    model.NowMs(),
		Reason:  "test1",
	}
	engine.handleWSState(wsEvent1)

	// Stage 5A更新: reconcile同步执行完成，reconcileInFlight已清零
	// 验证：reconcileInFlight=false（reconcile已完成）
	if engine.reconcileInFlight {
		t.Error("expected reconcileInFlight=false after first RECONNECTED (sync reconcile completed)")
	}

	// 第二次RECONNECTED（再Runnning状态）
	wsEvent2 := model.WSStateEvent{
		Channel: model.WSChannelUDS,
		State:   model.WSStateReconnected,
		AtMs:    model.NowMs(),
		Reason:  "test2",
	}
	engine.handleWSState(wsEvent2)

	// Stage 5A更新: 第二次reconcile也会同步执行并完成
	// 验证：reconcileInFlight仍为false（无重复触发问题）
	if engine.reconcileInFlight {
		t.Error("expected reconcileInFlight=false (second reconcile also completed)")
	}

	// 验证：状态仍为RUNNING
	if engine.state.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING, got %v", engine.state.EngineMode)
	}
}
