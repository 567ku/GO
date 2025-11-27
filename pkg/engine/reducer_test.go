// P0-RET-02: Reducer单元测试
package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestApplyPriceTick 测试PriceTick reducer
func TestApplyPriceTick(t *testing.T) {
	state := &model.GridStateSnapshot{
		Market: model.MarketState{},
	}

	ev := model.PriceTickEvent{
		PriceTicks: 50000000,
		EventAtMs:  1234567890,
	}

	newState, _ := ApplyPriceTick(state, ev)

	if newState.Market.LastPriceTicks != 50000000 {
		t.Errorf("expected LastPriceTicks=50000000, got %d", newState.Market.LastPriceTicks)
	}
	if newState.Market.LastPriceAtMs != 1234567890 {
		t.Errorf("expected LastPriceAtMs=1234567890, got %d", newState.Market.LastPriceAtMs)
	}
}

// TestApplyOrderUpdate 测试OrderUpdate reducer（双保险闭环确认）
func TestApplyOrderUpdate(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix: "TEST",
		Levels: []model.LevelState{
			{
				LevelID: 100,
				Cycle:   1,
				Entry: model.OrderSlot{
					Purpose: model.OrderPurposeEntry,
					State:   model.OrderStateSubmitted,
				},
			},
		},
		CLIDIndex: make(map[string]int64),
	}

	ev := model.OrderUpdateEvent{
		ClientOrderID:    "TEST:E:100:1",
		OrderID:          123456,
		Status:           "NEW",
		PriceTicks:       50000000,
		OrigQtyTicks:     1000000,
		ExecutedQtyTicks: 0,
		AvgPriceTicks:    0,
		UpdateAtMs:       1234567890,
	}

	newState, _ := ApplyOrderUpdate(state, ev, "TEST")

	// 验证状态更新
	if len(newState.Levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(newState.Levels))
	}

	entry := &newState.Levels[0].Entry
	if entry.State != model.OrderStateOpen {
		t.Errorf("expected State=OPEN, got %v", entry.State)
	}
	if entry.OrderID != 123456 {
		t.Errorf("expected OrderID=123456, got %d", entry.OrderID)
	}
	if entry.PriceTicks != 50000000 {
		t.Errorf("expected PriceTicks=50000000, got %d", entry.PriceTicks)
	}
	if entry.LastSource != model.EventSourceUDS {
		t.Errorf("expected LastSource=UDS, got %v", entry.LastSource)
	}

	// 验证CLID索引
	if newState.CLIDIndex["TEST:E:100:1"] != 123456 {
		t.Errorf("expected CLIDIndex[TEST:E:100:1]=123456, got %d", newState.CLIDIndex["TEST:E:100:1"])
	}
}

// TestApplyOrderUpdate_StateMappings 测试订单状态映射
func TestApplyOrderUpdate_StateMappings(t *testing.T) {
	testCases := []struct {
		exchangeStatus string
		expectedState  model.OrderState
	}{
		{"NEW", model.OrderStateOpen},
		{"PARTIALLY_FILLED", model.OrderStatePartial},
		{"FILLED", model.OrderStateFilled},
		{"CANCELED", model.OrderStateCanceled},
		{"REJECTED", model.OrderStateRejected},
		{"EXPIRED", model.OrderStateExpired},
	}

	for _, tc := range testCases {
		t.Run(tc.exchangeStatus, func(t *testing.T) {
			state := &model.GridStateSnapshot{
				Prefix: "TEST",
				Levels: []model.LevelState{
					{
						LevelID: 100,
						Cycle:   1,
						Entry: model.OrderSlot{
							Purpose: model.OrderPurposeEntry,
							State:   model.OrderStateSubmitted,
						},
					},
				},
				CLIDIndex: make(map[string]int64),
			}

			ev := model.OrderUpdateEvent{
				ClientOrderID: "TEST:E:100:1",
				OrderID:       123456,
				Status:        tc.exchangeStatus,
				UpdateAtMs:    1234567890,
			}

			newState, _ := ApplyOrderUpdate(state, ev, "TEST")

			if newState.Levels[0].Entry.State != tc.expectedState {
				t.Errorf("expected State=%v, got %v", tc.expectedState, newState.Levels[0].Entry.State)
			}
		})
	}
}

// TestApplyExecutorResult 测试ExecutorResult reducer（双保险闭环第一步）
func TestApplyExecutorResult(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix: "TEST",
		Levels: []model.LevelState{
			{
				LevelID: 100,
				Cycle:   1,
				Entry: model.OrderSlot{
					Purpose: model.OrderPurposeEntry,
					State:   model.OrderStateNone,
				},
			},
		},
	}

	// 成功的ExecutorResult
	ev := model.ExecutorResultEvent{
		TaskID: "task-123",
		OK:     true,
		ParsedOrder: &model.ParsedOrderUpdate{
			ClientOrderID:    "TEST:E:100:1",
			OrderID:          123456,
			Status:           "NEW",
			PriceTicks:       50000000,
			OrigQtyTicks:     1000000,
			ExecutedQtyTicks: 0,
			AvgPriceTicks:    0,
		},
	}

	newState, _ := ApplyExecutorResult(state, ev, "TEST")

	// 验证状态更新
	entry := &newState.Levels[0].Entry
	if entry.State != model.OrderStateOpen {
		t.Errorf("expected State=OPEN, got %v", entry.State)
	}
	if entry.OrderID != 123456 {
		t.Errorf("expected OrderID=123456, got %d", entry.OrderID)
	}
	if entry.LastSource != model.EventSourceResp {
		t.Errorf("expected LastSource=RESP, got %v", entry.LastSource)
	}
}

// TestApplyExecutorResult_Failed 测试ExecutorResult失败场景
func TestApplyExecutorResult_Failed(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix: "TEST",
		Levels: []model.LevelState{
			{
				LevelID: 100,
				Cycle:   1,
				Entry: model.OrderSlot{
					Purpose: model.OrderPurposeEntry,
					State:   model.OrderStateSubmitted,
				},
			},
		},
	}

	// 失败的ExecutorResult
	ev := model.ExecutorResultEvent{
		TaskID:    "task-123",
		OK:        false,
		ErrorCode: -2010,
		ErrorMsg:  "Order would immediately trigger",
	}

	newState, _ := ApplyExecutorResult(state, ev, "TEST")

	// 失败时不应修改状态（等待后续处理）
	if newState.Levels[0].Entry.State != model.OrderStateSubmitted {
		t.Errorf("expected State=SUBMITTED (unchanged), got %v", newState.Levels[0].Entry.State)
	}
}

// TestApplyWSStateChange 测试WS状态变更（Freeze机制）
func TestApplyWSStateChange(t *testing.T) {
	testCases := []struct {
		name         string
		channel      model.WSChannel
		wsState      model.WSState
		expectedMode model.EngineMode
	}{
		{"UDS断线→FREEZE", model.WSChannelUDS, model.WSStateDisconnected, model.EngineModeFreeze},
		{"TRADE断线→FREEZE", model.WSChannelTrade, model.WSStateDisconnected, model.EngineModeFreeze},
		{"UDS重连→RECONCILING", model.WSChannelUDS, model.WSStateReconnected, model.EngineModeReconciling},
		{"TRADE重连→RECONCILING", model.WSChannelTrade, model.WSStateReconnected, model.EngineModeReconciling},
		{"MARKET断线→不变", model.WSChannelMarket, model.WSStateDisconnected, model.EngineModeRunning},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := &model.GridStateSnapshot{
				EngineMode: model.EngineModeRunning,
			}

			ev := model.WSStateEvent{
				Channel: tc.channel,
				State:   tc.wsState,
			}

			newState, _ := ApplyWSStateChange(state, ev)

			if newState.EngineMode != tc.expectedMode {
				t.Errorf("expected EngineMode=%v, got %v", tc.expectedMode, newState.EngineMode)
			}
		})
	}
}

// ========== P0-RET-04: ExecutorResult错误码映射测试 ==========

// TestClassifyErrorCode 测试错误码分类
func TestClassifyErrorCode(t *testing.T) {
	testCases := []struct {
		code           int
		expectedAction ErrorAction
		description    string
	}{
		// REJECT类
		{-2010, ErrorActionReject, "NEW_ORDER_REJECTED"},
		{-2011, ErrorActionReject, "CANCEL_REJECTED"},
		{-4045, ErrorActionReject, "价格低于最小价"},
		{-4164, ErrorActionReject, "名义值过小"},
		{-4162, ErrorActionReject, "Reduce-only拒绝"},

		// FREEZE类
		{-2019, ErrorActionFreeze, "余额不足"},
		{-1021, ErrorActionFreeze, "时间戳超范围"},
		{-2015, ErrorActionFreeze, "API权限问题"},
		{-1003, ErrorActionFreeze, "请求过频"},
		{-4131, ErrorActionFreeze, "账户被限制"},

		// RETRY类
		{-1001, ErrorActionRetry, "内部错误"},
		{-1006, ErrorActionRetry, "意外响应"},
		{-1007, ErrorActionRetry, "超时"},
		{0, ErrorActionRetry, "无错误码（网络问题）"},
		{-9999, ErrorActionRetry, "未知错误码"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			action := classifyErrorCode(tc.code)
			if action != tc.expectedAction {
				t.Errorf("code %d: expected %v, got %v", tc.code, tc.expectedAction, action)
			}
		})
	}
}

// TestApplyExecutorResult_ErrorClassification 测试ExecutorResult错误分类处理
func TestApplyExecutorResult_ErrorClassification(t *testing.T) {
	testCases := []struct {
		name         string
		errorCode    int
		expectedMode model.EngineMode
	}{
		{"REJECT类错误不触发FREEZE", -2010, model.EngineModeRunning},
		{"RETRY类错误不触发FREEZE", -1001, model.EngineModeRunning},
		{"FREEZE类错误触发FREEZE", -2019, model.EngineModeFreeze},
		{"API权限错误触发FREEZE", -2015, model.EngineModeFreeze},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := &model.GridStateSnapshot{
				EngineMode: model.EngineModeRunning,
			}

			ev := model.ExecutorResultEvent{
				TaskID:    "task-123",
				OK:        false,
				ErrorCode: tc.errorCode,
				ErrorMsg:  "test error",
			}

			newState, _ := ApplyExecutorResult(state, ev, "TEST")

			if newState.EngineMode != tc.expectedMode {
				t.Errorf("expected EngineMode=%v, got %v", tc.expectedMode, newState.EngineMode)
			}
		})
	}
}

// ========== P0-RET-05: Lease回收逻辑测试 ==========

// TestApplyLeaseScanResult_Retry 测试Lease超时后重试逻辑
func TestApplyLeaseScanResult_Retry(t *testing.T) {
	state := &model.GridStateSnapshot{
		EngineMode: model.EngineModeRunning,
		Levels: []model.LevelState{
			{
				LevelID: 100,
				Entry: model.OrderSlot{
					State:   model.OrderStateSubmitted,
					OrderID: 123456,
					Attempt: 2, // 已重试2次
				},
				TP: model.OrderSlot{
					State:   model.OrderStateSubmitted,
					OrderID: 789012,
					Attempt: 1,
				},
			},
		},
	}

	expiredLevels := []int{100}
	maxAttempt := 8

	newState, _ := ApplyLeaseScanResult(state, expiredLevels, maxAttempt)

	// 验证Entry回退到NONE，Attempt递增
	if newState.Levels[0].Entry.State != model.OrderStateNone {
		t.Errorf("expected Entry.State=NONE, got %v", newState.Levels[0].Entry.State)
	}
	if newState.Levels[0].Entry.OrderID != 0 {
		t.Errorf("expected Entry.OrderID=0, got %d", newState.Levels[0].Entry.OrderID)
	}
	if newState.Levels[0].Entry.Attempt != 3 {
		t.Errorf("expected Entry.Attempt=3, got %d", newState.Levels[0].Entry.Attempt)
	}

	// 验证TP回退到NONE，Attempt递增
	if newState.Levels[0].TP.State != model.OrderStateNone {
		t.Errorf("expected TP.State=NONE, got %v", newState.Levels[0].TP.State)
	}
	if newState.Levels[0].TP.Attempt != 2 {
		t.Errorf("expected TP.Attempt=2, got %d", newState.Levels[0].TP.Attempt)
	}

	// 验证未触发FREEZE
	if newState.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING, got %v", newState.EngineMode)
	}
}

// TestApplyLeaseScanResult_MaxAttemptFreeze 测试超过maxAttempt后触发FREEZE
func TestApplyLeaseScanResult_MaxAttemptFreeze(t *testing.T) {
	state := &model.GridStateSnapshot{
		EngineMode: model.EngineModeRunning,
		Levels: []model.LevelState{
			{
				LevelID: 100,
				Entry: model.OrderSlot{
					State:   model.OrderStateSubmitted,
					OrderID: 123456,
					Attempt: 8, // 已达到最大重试次数
				},
			},
		},
	}

	expiredLevels := []int{100}
	maxAttempt := 8

	newState, _ := ApplyLeaseScanResult(state, expiredLevels, maxAttempt)

	// 验证Attempt递增到9
	if newState.Levels[0].Entry.Attempt != 9 {
		t.Errorf("expected Entry.Attempt=9, got %d", newState.Levels[0].Entry.Attempt)
	}

	// 验证触发FREEZE
	if newState.EngineMode != model.EngineModeFreeze {
		t.Errorf("expected EngineMode=FREEZE, got %v", newState.EngineMode)
	}

	// 验证状态保持SUBMITTED（不再重试）
	if newState.Levels[0].Entry.State != model.OrderStateSubmitted {
		t.Errorf("expected Entry.State=SUBMITTED (frozen), got %v", newState.Levels[0].Entry.State)
	}
}

// TestApplyLeaseScanResult_NoExpiredLevels 测试无超时任务场景
func TestApplyLeaseScanResult_NoExpiredLevels(t *testing.T) {
	state := &model.GridStateSnapshot{
		EngineMode: model.EngineModeRunning,
		Levels: []model.LevelState{
			{
				LevelID: 100,
				Entry: model.OrderSlot{
					State:   model.OrderStateSubmitted,
					Attempt: 2,
				},
			},
		},
	}

	expiredLevels := []int{} // 无超时任务
	maxAttempt := 8

	newState, _ := ApplyLeaseScanResult(state, expiredLevels, maxAttempt)

	// 验证状态不变
	if newState.Levels[0].Entry.State != model.OrderStateSubmitted {
		t.Errorf("expected Entry.State=SUBMITTED, got %v", newState.Levels[0].Entry.State)
	}
	if newState.Levels[0].Entry.Attempt != 2 {
		t.Errorf("expected Entry.Attempt=2, got %d", newState.Levels[0].Entry.Attempt)
	}
	if newState.EngineMode != model.EngineModeRunning {
		t.Errorf("expected EngineMode=RUNNING, got %v", newState.EngineMode)
	}
}
