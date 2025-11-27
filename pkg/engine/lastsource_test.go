package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestLastSource_EnumOnly 验证LastSource只允许EventSource枚举值
// 覆盖：Reconcile apply → QUERY；ExecutorResult apply → RESP；UDS更新 → UDS
func TestLastSource_EnumOnly(t *testing.T) {
	prefix := "TEST"

	// ========== 场景1：Reconcile apply → LastSource=QUERY ==========
	t.Run("Reconcile_Apply_Query", func(t *testing.T) {
		testState := &model.GridStateSnapshot{
			Prefix:    prefix,
			CLIDIndex: make(map[string]int64),
			Levels: []model.LevelState{
				{
					LevelID:    500,
					PriceTicks: 50000000,
					Cycle:      1,
					Entry: model.OrderSlot{
						Purpose: model.OrderPurposeEntry,
						State:   model.OrderStateNone,
					},
				},
			},
		}

		orders := []model.ParsedOrderUpdate{
			{
				OrderID:          123456,
				ClientOrderID:    "TEST:E:500:1",
				Status:           "NEW",
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 0,
				AvgPriceTicks:    0,
			},
		}

		// 应用快照（通过reducer）
		testState, _ = ApplyOpenOrdersSnapshot(testState, orders, prefix)

		// 验证：LastSource=QUERY
		if testState.Levels[0].Entry.LastSource != model.EventSourceQuery {
			t.Errorf("Reconcile apply: expected LastSource=QUERY, got %v", testState.Levels[0].Entry.LastSource)
		}
	})

	// ========== 场景2：ExecutorResult apply → LastSource=RESP ==========
	t.Run("ExecutorResult_Apply_Resp", func(t *testing.T) {
		// 重新创建state（避免用例间干扰）
		testState := &model.GridStateSnapshot{
			Prefix:    prefix,
			CLIDIndex: make(map[string]int64),
			Levels: []model.LevelState{
				{
					LevelID:    500,
					PriceTicks: 50000000,
					Cycle:      1,
					Entry: model.OrderSlot{
						Purpose: model.OrderPurposeEntry,
						State:   model.OrderStateSubmitted,
					},
				},
			},
		}

		ev := model.ExecutorResultEvent{
			TaskID:    "task_1",
			OK:        true,
			ErrorCode: 0, // 成功
			ParsedOrder: &model.ParsedOrderUpdate{
				OrderID:          123456,
				ClientOrderID:    "TEST:E:500:1", // 正确的CLID格式
				Status:           "NEW",
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 0,
				AvgPriceTicks:    0,
			},
		}

		// 应用ExecutorResult
		testState, _ = ApplyExecutorResult(testState, ev, prefix)

		// 调试输出
		t.Logf("After ApplyExecutorResult: State=%v, LastSource=%v (%s)",
			testState.Levels[0].Entry.State, testState.Levels[0].Entry.LastSource, string(testState.Levels[0].Entry.LastSource))

		// 验证：LastSource=RESP
		if testState.Levels[0].Entry.LastSource != model.EventSourceResp {
			t.Errorf("ExecutorResult apply: expected LastSource=RESP, got %v", testState.Levels[0].Entry.LastSource)
		}
	})

	// ========== 场景3：UDS更新 → LastSource=UDS ==========
	t.Run("UDS_Update_Uds", func(t *testing.T) {
		// 重新创建state
		testState := &model.GridStateSnapshot{
			Prefix:    prefix,
			CLIDIndex: make(map[string]int64),
			Levels: []model.LevelState{
				{
					LevelID:    500,
					PriceTicks: 50000000,
					Cycle:      1,
					Entry: model.OrderSlot{
						Purpose: model.OrderPurposeEntry,
						State:   model.OrderStateOpen,
					},
				},
			},
		}

		ev := model.OrderUpdateEvent{
			OrderID:          123456,
			ClientOrderID:    "TEST:E:500:1", // 正确的CLID格式
			Status:           "PARTIALLY_FILLED",
			PriceTicks:       50000000,
			OrigQtyTicks:     1000,
			ExecutedQtyTicks: 500,
			AvgPriceTicks:    50000000,
			UpdateAtMs:       model.NowMs(),
		}

		// 应用UDS更新
		testState, _ = ApplyOrderUpdate(testState, ev, prefix)

		// 验证：LastSource=UDS
		if testState.Levels[0].Entry.LastSource != model.EventSourceUDS {
			t.Errorf("UDS update: expected LastSource=UDS, got %v", testState.Levels[0].Entry.LastSource)
		}
	})

	// ========== 场景4：验证枚举值有效性（编译时保证） ==========
	t.Run("EventSource_Enum_Valid", func(t *testing.T) {
		// 编译时验证：这些赋值必须通过编译
		var _ model.EventSource = model.EventSourceResp
		var _ model.EventSource = model.EventSourceUDS
		var _ model.EventSource = model.EventSourceQuery

		// 运行时验证：枚举值符合预期
		if model.EventSourceResp != "RESP" {
			t.Errorf("EventSourceResp expected 'RESP', got %v", model.EventSourceResp)
		}
		if model.EventSourceUDS != "UDS" {
			t.Errorf("EventSourceUDS expected 'UDS', got %v", model.EventSourceUDS)
		}
		if model.EventSourceQuery != "QUERY" {
			t.Errorf("EventSourceQuery expected 'QUERY', got %v", model.EventSourceQuery)
		}
	})
}
