// P0-E-03: CLID映射单元测试
package engine

import (
	"gridbot/pkg/model"
	"testing"
)

// TestParseCLID_Entry 测试Entry CLID解析
func TestParseCLID_Entry(t *testing.T) {
	prefix := "GRID01"
	clid := "GRID01:E:5:2"

	purpose, levelID, cycle, err := ParseCLID(clid, prefix)
	if err != nil {
		t.Fatalf("ParseCLID failed: %v", err)
	}

	if purpose != model.OrderPurposeEntry {
		t.Errorf("purpose mismatch: expected=%s, got=%s", model.OrderPurposeEntry, purpose)
	}

	if levelID != 5 {
		t.Errorf("levelID mismatch: expected=5, got=%d", levelID)
	}

	if cycle != 2 {
		t.Errorf("cycle mismatch: expected=2, got=%d", cycle)
	}
}

// TestParseCLID_TP 测试TP CLID解析
func TestParseCLID_TP(t *testing.T) {
	prefix := "GRID01"
	clid := "GRID01:T:10:0"

	purpose, levelID, cycle, err := ParseCLID(clid, prefix)
	if err != nil {
		t.Fatalf("ParseCLID failed: %v", err)
	}

	if purpose != model.OrderPurposeTP {
		t.Errorf("purpose mismatch: expected=%s, got=%s", model.OrderPurposeTP, purpose)
	}

	if levelID != 10 {
		t.Errorf("levelID mismatch: expected=10, got=%d", levelID)
	}

	if cycle != 0 {
		t.Errorf("cycle mismatch: expected=0, got=%d", cycle)
	}
}

// TestParseCLID_WrongPrefix 测试错误前缀
func TestParseCLID_WrongPrefix(t *testing.T) {
	prefix := "GRID01"
	clid := "GRID02:E:5:2" // 错误前缀

	_, _, _, err := ParseCLID(clid, prefix)
	if err == nil {
		t.Fatal("ParseCLID应该返回错误（前缀不匹配）")
	}
}

// TestParseCLID_InvalidFormat 测试无效格式
func TestParseCLID_InvalidFormat(t *testing.T) {
	prefix := "GRID01"

	testCases := []string{
		"GRID01:E:5",     // 缺少cycle
		"GRID01:E:5:2:3", // 多余字段
		"GRID01:X:5:2",   // 无效purpose
		"GRID01:E:abc:2", // 无效levelID
		"GRID01:E:5:xyz", // 无效cycle
	}

	for _, clid := range testCases {
		_, _, _, err := ParseCLID(clid, prefix)
		if err == nil {
			t.Errorf("ParseCLID应该返回错误（无效格式）: %s", clid)
		}
	}
}

// TestFindLevelByID 测试查找level
func TestFindLevelByID(t *testing.T) {
	state := &model.GridStateSnapshot{
		Levels: []model.LevelState{
			{LevelID: 1, PriceTicks: 10000},
			{LevelID: 2, PriceTicks: 20000},
			{LevelID: 3, PriceTicks: 30000},
		},
	}

	// 查找存在的level
	level := FindLevelByID(state, 2)
	if level == nil {
		t.Fatal("应该找到levelID=2的level")
	}
	if level.PriceTicks != 20000 {
		t.Errorf("priceTicks mismatch: expected=20000, got=%d", level.PriceTicks)
	}

	// 查找不存在的level
	level = FindLevelByID(state, 999)
	if level != nil {
		t.Error("不应该找到levelID=999的level")
	}
}

// TestFindOrCreateLevel_Reducer 测试查找或创建level（通过reducer）
func TestFindOrCreateLevel_Reducer(t *testing.T) {
	state := &model.GridStateSnapshot{
		Prefix:    "TEST",
		CLIDIndex: make(map[string]int64),
		Levels: []model.LevelState{
			{LevelID: 1, PriceTicks: 10000, Cycle: 1},
		},
	}

	// 模拟对账快照：包含已存在的level和新level
	orders := []model.ParsedOrderUpdate{
		{
			ClientOrderID:    "TEST:E:1:1",
			OrderID:          123,
			Status:           "NEW",
			PriceTicks:       10000,
			OrigQtyTicks:     1000,
			ExecutedQtyTicks: 0,
		},
		{
			ClientOrderID:    "TEST:E:2:1",
			OrderID:          456,
			Status:           "NEW",
			PriceTicks:       20000,
			OrigQtyTicks:     1000,
			ExecutedQtyTicks: 0,
		},
	}

	// 应用快照（会自动创建缺失的level）
	state, _ = ApplyOpenOrdersSnapshot(state, orders, "TEST")

	// 验证：现在应该有2个level
	if len(state.Levels) != 2 {
		t.Errorf("应该创建新level: expected=2, got=%d", len(state.Levels))
	}

	// 验证level 2的初始状态
	level2 := FindLevelByID(state, 2)
	if level2 == nil {
		t.Fatal("应该创建 level 2")
	}
	if level2.Entry.State != model.OrderStateOpen {
		t.Errorf("Entry state应该为OPEN: got=%s", level2.Entry.State)
	}
	if level2.TP.State != model.OrderStateNone {
		t.Errorf("TP state应该为NONE: got=%s", level2.TP.State)
	}
}

// TestGetOrderSlot 测试获取订单槽
func TestGetOrderSlot(t *testing.T) {
	level := &model.LevelState{
		LevelID: 1,
		Entry: model.OrderSlot{
			Purpose: model.OrderPurposeEntry,
			State:   model.OrderStateOpen,
		},
		TP: model.OrderSlot{
			Purpose: model.OrderPurposeTP,
			State:   model.OrderStateNone,
		},
	}

	// 获取Entry槽
	slot := GetOrderSlot(level, model.OrderPurposeEntry)
	if slot == nil {
		t.Fatal("应该获取到Entry槽")
	}
	if slot.State != model.OrderStateOpen {
		t.Errorf("Entry state mismatch: expected=OPEN, got=%s", slot.State)
	}

	// 获取TP槽
	slot = GetOrderSlot(level, model.OrderPurposeTP)
	if slot == nil {
		t.Fatal("应该获取到TP槽")
	}
	if slot.State != model.OrderStateNone {
		t.Errorf("TP state mismatch: expected=NONE, got=%s", slot.State)
	}
}
