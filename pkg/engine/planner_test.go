// P0-E-06: GapScan单元测试
package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestGapScan_EmptyState 测试空状态下的GapScan
func TestGapScan_EmptyState(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:         "TEST",
		Symbol:         "BTCUSDT",
		Side:           model.GridSideLong,
		PositionMode:   model.PositionModeOneWay,
		PricePrecision: 1,
		QtyPrecision:   3,
		Window: model.WindowState{
			WinMinTicks:         49000000,
			WinMaxTicks:         51000000,
			StepTicks:           100000,
			TPKeepOutsideLevels: 5,
		},
		Market: model.MarketState{
			LastPriceTicks: 50000000,
			LastPriceAtMs:  model.NowMs(),
		},
		Qty: model.QtyState{
			Mode:          "BASE",
			EntryQtyTicks: 1000,
		},
		Levels:    []model.LevelState{},
		CLIDIndex: make(map[string]int64),
	}

	// 执行GapScan
	tasks := GapScan(state)

	// 验证生成了Entry任务
	if len(tasks) == 0 {
		t.Error("GapScan应该生成至少一个任务")
	}

	// 验证任务类型都是PLACE_ENTRY
	for _, task := range tasks {
		if task.Type != model.TaskTypePlaceEntry {
			t.Errorf("任务类型应该为PLACE_ENTRY: got=%s", task.Type)
		}
	}
}

// TestGapScan_EntryFilled 测试Entry FILLED后生成TP
func TestGapScan_EntryFilled(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:         "TEST",
		Symbol:         "BTCUSDT",
		Side:           model.GridSideLong,
		PositionMode:   model.PositionModeOneWay,
		PricePrecision: 1,
		QtyPrecision:   3,
		Window: model.WindowState{
			WinMinTicks:         49000000,
			WinMaxTicks:         51000000,
			StepTicks:           100000,
			TPKeepOutsideLevels: 5,
		},
		Market: model.MarketState{
			LastPriceTicks: 50000000,
			LastPriceAtMs:  model.NowMs(),
		},
		Qty: model.QtyState{
			Mode:          "BASE",
			EntryQtyTicks: 1000,
		},
		Levels: []model.LevelState{
			{
				LevelID:    500,
				PriceTicks: 50000000,
				Cycle:      0,
				Entry: model.OrderSlot{
					Purpose:          model.OrderPurposeEntry,
					State:            model.OrderStateFilled,
					ExecutedQtyTicks: 1000,
				},
				TP: model.OrderSlot{
					Purpose: model.OrderPurposeTP,
					State:   model.OrderStateNone,
				},
			},
		},
		CLIDIndex: make(map[string]int64),
	}

	// 执行GapScan
	tasks := GapScan(state)

	// 验证生成了TP任务
	foundTP := false
	for _, task := range tasks {
		if task.Type == model.TaskTypePlaceTP && task.LevelID == 500 {
			foundTP = true

			// 验证TP价格（LONG: Entry价格 + step）
			expectedTPPrice := int64(50000000 + 100000)
			if task.PriceTicks != expectedTPPrice {
				t.Errorf("TP价格错误: expected=%d, got=%d", expectedTPPrice, task.PriceTicks)
			}

			// 验证reduceOnly
			if task.ReduceOnly == nil || !*task.ReduceOnly {
				t.Error("ONE_WAY模式TP必须reduceOnly=true")
			}
			break
		}
	}

	if !foundTP {
		t.Error("应该生成TP任务")
	}
}

// TestNeedPlaceEntry 测试needPlaceEntry逻辑
func TestNeedPlaceEntry(t *testing.T) {
	testCases := []struct {
		name     string
		state    model.OrderState
		expected bool
	}{
		{"NONE状态需要放置", model.OrderStateNone, true},
		{"FILLED状态需要放置", model.OrderStateFilled, true},
		{"CANCELED状态需要放置", model.OrderStateCanceled, true},
		{"REJECTED状态需要放置", model.OrderStateRejected, true},
		{"OPEN状态不需要放置", model.OrderStateOpen, false},
		{"SUBMITTED状态不需要放置", model.OrderStateSubmitted, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			level := &model.LevelState{
				Entry: model.OrderSlot{
					State: tc.state,
				},
			}

			result := needPlaceEntry(level)
			if result != tc.expected {
				t.Errorf("%s: expected=%v, got=%v", tc.name, tc.expected, result)
			}
		})
	}
}

// TestNeedPlaceTP 测试needPlaceTP逻辑
func TestNeedPlaceTP(t *testing.T) {
	testCases := []struct {
		name       string
		entryState model.OrderState
		tpState    model.OrderState
		expected   bool
	}{
		{"Entry未FILLED不需要TP", model.OrderStateOpen, model.OrderStateNone, false},
		{"Entry FILLED且TP为NONE需要", model.OrderStateFilled, model.OrderStateNone, true},
		{"Entry FILLED且TP已OPEN不需要", model.OrderStateFilled, model.OrderStateOpen, false},
		{"Entry FILLED且TP已FILLED需要", model.OrderStateFilled, model.OrderStateFilled, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			level := &model.LevelState{
				Entry: model.OrderSlot{
					State: tc.entryState,
				},
				TP: model.OrderSlot{
					State: tc.tpState,
				},
			}

			result := needPlaceTP(level)
			if result != tc.expected {
				t.Errorf("%s: expected=%v, got=%v", tc.name, tc.expected, result)
			}
		})
	}
}

// ========== P0-RET-06: 区间外撤单测试 ==========

// TestGapScan_CancelsOutsideEntry 测试区间外Entry撤单
func TestGapScan_CancelsOutsideEntry(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:         "TEST",
		Symbol:         "BTCUSDT",
		Side:           model.GridSideLong,
		PositionMode:   model.PositionModeOneWay,
		PricePrecision: 1,
		QtyPrecision:   3,
		Window: model.WindowState{
			WinMinTicks:         49000000, // 490
			WinMaxTicks:         51000000, // 510
			StepTicks:           100000,   // 1
			TPKeepOutsideLevels: 5,
		},
		Market: model.MarketState{
			LastPriceTicks: 50000000, // 500
			LastPriceAtMs:  model.NowMs(),
		},
		Qty: model.QtyState{
			Mode:          "BASE",
			EntryQtyTicks: 1000,
		},
		Levels: []model.LevelState{
			// 区间内：LevelID=500 (价格50000000)
			{
				LevelID:    500,
				PriceTicks: 50000000,
				Entry: model.OrderSlot{
					State:   model.OrderStateOpen,
					OrderID: 111111,
				},
			},
			// 区间外：LevelID=400 (价格40000000)
			{
				LevelID:    400,
				PriceTicks: 40000000,
				Entry: model.OrderSlot{
					State:         model.OrderStateOpen,
					OrderID:       222222,
					ClientOrderID: "TEST:E:400:0",
				},
			},
			// 区间外：LevelID=600 (价格60000000)
			{
				LevelID:    600,
				PriceTicks: 60000000,
				Entry: model.OrderSlot{
					State:         model.OrderStateOpen,
					OrderID:       333333,
					ClientOrderID: "TEST:E:600:0",
				},
			},
		},
		CLIDIndex: make(map[string]int64),
	}

	// 执行GapScan
	tasks := GapScan(state)

	// 验证生成了CANCEL任务
	cancelCount := 0
	for _, task := range tasks {
		if task.Type == model.TaskTypeCancelOrder && task.Purpose == model.OrderPurposeEntry {
			cancelCount++
			// 验证只撤销区间外的Entry
			if task.LevelID != 400 && task.LevelID != 600 {
				t.Errorf("不应该撤销LevelID=%d的Entry", task.LevelID)
			}
		}
	}

	if cancelCount != 2 {
		t.Errorf("应该生成2个CANCEL任务，got=%d", cancelCount)
	}
}

// TestGapScan_KeepsTPWithinNLevels 测试TP保留N格逻辑
func TestGapScan_KeepsTPWithinNLevels(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:         "TEST",
		Symbol:         "BTCUSDT",
		Side:           model.GridSideLong,
		PositionMode:   model.PositionModeOneWay,
		PricePrecision: 1,
		QtyPrecision:   3,
		Window: model.WindowState{
			WinMinTicks:         49000000, // LevelID=490
			WinMaxTicks:         51000000, // LevelID=510
			StepTicks:           100000,
			TPKeepOutsideLevels: 5, // TP保留5格
		},
		Market: model.MarketState{
			LastPriceTicks: 50000000,
			LastPriceAtMs:  model.NowMs(),
		},
		Qty: model.QtyState{
			Mode:          "BASE",
			EntryQtyTicks: 1000,
		},
		Levels: []model.LevelState{
			// 区间内TP：不撤销
			{
				LevelID:    500,
				PriceTicks: 50000000,
				TP: model.OrderSlot{
					State:         model.OrderStateOpen,
					OrderID:       111111,
					ClientOrderID: "TEST:T:500:0",
				},
			},
			// 区间外但在保留范围内：LevelID=486 (window_min=490, 490-5=485, 486>=485 保留)
			{
				LevelID:    486,
				PriceTicks: 48600000,
				TP: model.OrderSlot{
					State:         model.OrderStateOpen,
					OrderID:       222222,
					ClientOrderID: "TEST:T:486:0",
				},
			},
			// 区间外且超出保留范围：LevelID=400 (< 490-5=485)
			{
				LevelID:    400,
				PriceTicks: 40000000,
				TP: model.OrderSlot{
					State:         model.OrderStateOpen,
					OrderID:       333333,
					ClientOrderID: "TEST:T:400:0",
				},
			},
		},
		CLIDIndex: make(map[string]int64),
	}

	// 执行GapScan
	tasks := GapScan(state)

	// 验证生成了CANCEL TP任务
	cancelTPCount := 0
	for _, task := range tasks {
		if task.Type == model.TaskTypeCancelOrder && task.Purpose == model.OrderPurposeTP {
			cancelTPCount++
			// 验证只撤销超出保留范围的TP
			if task.LevelID != 400 {
				t.Errorf("应该撤销LevelID=400的TP，got=%d", task.LevelID)
			}
		}
	}

	if cancelTPCount != 1 {
		t.Errorf("应该生成1个CANCEL TP任务，got=%d", cancelTPCount)
	}
}

// TestGapScan_DoesNotCancelTerminalStates 测试不撤销终态订单
func TestGapScan_DoesNotCancelTerminalStates(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:         "TEST",
		Symbol:         "BTCUSDT",
		Side:           model.GridSideLong,
		PositionMode:   model.PositionModeOneWay,
		PricePrecision: 1,
		QtyPrecision:   3,
		Window: model.WindowState{
			WinMinTicks:         49000000,
			WinMaxTicks:         51000000,
			StepTicks:           100000,
			TPKeepOutsideLevels: 5,
		},
		Market: model.MarketState{
			LastPriceTicks: 50000000,
			LastPriceAtMs:  model.NowMs(),
		},
		Qty: model.QtyState{
			Mode:          "BASE",
			EntryQtyTicks: 1000,
		},
		Levels: []model.LevelState{
			// 区间外但已FILLED：不撤销
			{
				LevelID:    400,
				PriceTicks: 40000000,
				Entry: model.OrderSlot{
					State:   model.OrderStateFilled,
					OrderID: 111111,
				},
			},
			// 区间外但已CANCELED：不撤销
			{
				LevelID:    600,
				PriceTicks: 60000000,
				Entry: model.OrderSlot{
					State:   model.OrderStateCanceled,
					OrderID: 222222,
				},
			},
		},
		CLIDIndex: make(map[string]int64),
	}

	// 执行GapScan
	tasks := GapScan(state)

	// 验证没有生成CANCEL任务
	for _, task := range tasks {
		if task.Type == model.TaskTypeCancelOrder {
			t.Errorf("不应该撤销终态订单：task=%+v", task)
		}
	}
}
