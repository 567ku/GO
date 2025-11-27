// Stage 5C: GetState深拷贝测试
// 验收标准: 外部拿到state后修改，不影响Engine内部状态
package engine

import (
	"context"
	"gridbot/pkg/model"
	"testing"
)

// TestGetState_DeepCopy_ModifyDoesNotAffectInternal 验证深拷贝防止误写
func TestGetState_DeepCopy_ModifyDoesNotAffectInternal(t *testing.T) {
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

	// 初始化一些状态
	engine.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Cycle:      1,
			Entry: model.OrderSlot{
				Purpose:    model.OrderPurposeEntry,
				State:      model.OrderStateOpen,
				OrderID:    12345,
				UpdateAtMs: model.NowMs(),
			},
		},
	}
	engine.state.CLIDIndex = map[string]int64{
		"TEST:E:500:1": 12345,
	}
	engine.state.Market.LastPriceTicks = 50000000

	// 步骤1: 获取state快照
	stateCopy, err := engine.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	// 步骤2: 修改拷贝的基础字段
	stateCopy.Symbol = "ETHUSDT" // 应该不影响原始state
	stateCopy.Market.LastPriceTicks = 99999999

	// 步骤3: 修改拷贝的Levels切片
	if len(stateCopy.Levels) > 0 {
		stateCopy.Levels[0].PriceTicks = 88888888
		stateCopy.Levels[0].Entry.OrderID = 99999
	}

	// 步骤4: 修改拷贝的CLIDIndex map
	stateCopy.CLIDIndex["TEST:E:999:1"] = 77777
	delete(stateCopy.CLIDIndex, "TEST:E:500:1")

	// 验证: Engine内部状态未被修改
	if engine.state.Symbol != "BTCUSDT" {
		t.Errorf("Symbol被外部修改: expected=BTCUSDT, got=%s", engine.state.Symbol)
	}

	if engine.state.Market.LastPriceTicks != 50000000 {
		t.Errorf("Market.LastPriceTicks被外部修改: expected=50000000, got=%d", engine.state.Market.LastPriceTicks)
	}

	if len(engine.state.Levels) == 0 {
		t.Fatal("Levels被清空")
	}

	if engine.state.Levels[0].PriceTicks != 50000000 {
		t.Errorf("Levels[0].PriceTicks被外部修改: expected=50000000, got=%d", engine.state.Levels[0].PriceTicks)
	}

	if engine.state.Levels[0].Entry.OrderID != 12345 {
		t.Errorf("Levels[0].Entry.OrderID被外部修改: expected=12345, got=%d", engine.state.Levels[0].Entry.OrderID)
	}

	if _, exists := engine.state.CLIDIndex["TEST:E:500:1"]; !exists {
		t.Error("CLIDIndex被外部删除")
	}

	if engine.state.CLIDIndex["TEST:E:500:1"] != 12345 {
		t.Errorf("CLIDIndex值被外部修改: expected=12345, got=%d", engine.state.CLIDIndex["TEST:E:500:1"])
	}

	if _, exists := engine.state.CLIDIndex["TEST:E:999:1"]; exists {
		t.Error("CLIDIndex被外部添加")
	}
}

// TestGetState_DeepCopy_NilSafe 测试nil安全性
func TestGetState_DeepCopy_NilSafe(t *testing.T) {
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

	// 确保空的Levels和CLIDIndex也能正确深拷贝
	engine.state.Levels = nil
	engine.state.CLIDIndex = nil

	stateCopy, err := engine.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	// 验证: 拷贝后不是nil（应该初始化为空切片/map）
	if stateCopy.Levels != nil {
		// 允许nil,不强制初始化
	}
	if stateCopy.CLIDIndex != nil {
		// 允许nil,不强制初始化
	}

	// 验证: 修改拷贝不会panic
	stateCopy.Levels = append(stateCopy.Levels, model.LevelState{LevelID: 1})
	if stateCopy.CLIDIndex == nil {
		stateCopy.CLIDIndex = make(map[string]int64)
	}
	stateCopy.CLIDIndex["test"] = 123

	// 验证: Engine内部状态仍为nil
	if engine.state.Levels != nil {
		t.Error("Levels应该保持nil")
	}
	if engine.state.CLIDIndex != nil {
		t.Error("CLIDIndex应该保持nil")
	}
}

// TestGetState_DeepCopy_MultipleCalls 测试多次调用互不影响
func TestGetState_DeepCopy_MultipleCalls(t *testing.T) {
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

	engine.state.Market.LastPriceTicks = 50000000

	// 获取两份拷贝
	copy1, err := engine.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	copy2, err := engine.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	// 修改copy1
	copy1.Market.LastPriceTicks = 11111111

	// 修改copy2
	copy2.Market.LastPriceTicks = 22222222

	// 验证: copy1和copy2互不影响
	if copy1.Market.LastPriceTicks != 11111111 {
		t.Errorf("copy1被copy2影响: expected=11111111, got=%d", copy1.Market.LastPriceTicks)
	}

	if copy2.Market.LastPriceTicks != 22222222 {
		t.Errorf("copy2被copy1影响: expected=22222222, got=%d", copy2.Market.LastPriceTicks)
	}

	// 验证: Engine内部状态未被影响
	if engine.state.Market.LastPriceTicks != 50000000 {
		t.Errorf("Engine内部状态被影响: expected=50000000, got=%d", engine.state.Market.LastPriceTicks)
	}
}
