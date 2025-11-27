// Stage 4A: 崩溃恢复演练测试
package engine

import (
	"gridbot/pkg/model"
	"os"
	"path/filepath"
	"testing"
)

// TestEngine_Bootstrap_CrashRecovery 崩溃恢复演练
// 模拟：运行 → 保存snapshot → kill -9（崩溃） → 重启 → 状态一致
func TestEngine_Bootstrap_CrashRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "crash_recovery.json")

	// ========== 第一次启动：运行并保存snapshot ==========
	t.Log("Phase 1: Initial run and save snapshot")

	cfg1 := EngineConfig{
		Prefix:        "TEST",
		Symbol:        "BTCUSDT",
		Side:          model.GridSideLong,
		StepTicks:     100000,
		WinMinTicks:   49000000,
		WinMaxTicks:   51000000,
		EntryQtyTicks: 1000,
		LeasePlaceMs:  3000,
		MaxAttempt:    8,
		EventChSize:   100,
		SnapshotPath:  snapshotPath,
	}

	engine1 := NewEngine(cfg1)

	// Bootstrap（首次启动，应该返回false）
	recovered, err := engine1.Bootstrap()
	if err != nil {
		t.Fatalf("First bootstrap should not error: %v", err)
	}
	if recovered {
		t.Error("First bootstrap should return false (no snapshot exists)")
	}

	// 模拟运行：添加一些level
	engine1.state.Levels = []model.LevelState{
		{
			LevelID:    500,
			PriceTicks: 50000000,
			Cycle:      1,
			Entry: model.OrderSlot{
				Purpose:       model.OrderPurposeEntry,
				State:         model.OrderStateOpen,
				OrderID:       123456,
				ClientOrderID: "TEST:E:500:1",
			},
		},
		{
			LevelID:    501,
			PriceTicks: 50100000,
			Cycle:      0,
			Entry: model.OrderSlot{
				Purpose: model.OrderPurposeEntry,
				State:   model.OrderStateNone,
			},
		},
	}
	engine1.state.EngineMode = model.EngineModeRunning
	engine1.state.CLIDIndex["TEST:E:500:1"] = 123456

	// 保存snapshot
	if err := engine1.SaveSnapshot(); err != nil {
		t.Fatalf("Save snapshot failed: %v", err)
	}

	t.Logf("Saved snapshot with %d levels", len(engine1.state.Levels))

	// 验证snapshot文件存在
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatal("Snapshot file should exist after save")
	}

	// 模拟崩溃：直接丢弃engine1（相当于kill -9）
	engine1 = nil
	t.Log("Simulated crash (engine1 discarded)")

	// ========== 第二次启动：从snapshot恢复 ==========
	t.Log("Phase 2: Restart and recover from snapshot")

	cfg2 := EngineConfig{
		Prefix:        "TEST",
		Symbol:        "BTCUSDT",
		Side:          model.GridSideLong,
		StepTicks:     100000,
		WinMinTicks:   49000000,
		WinMaxTicks:   51000000,
		EntryQtyTicks: 1000,
		SnapshotPath:  snapshotPath,
	}

	engine2 := NewEngine(cfg2)

	// Bootstrap（应该从snapshot恢复）
	recovered, err = engine2.Bootstrap()
	if err != nil {
		t.Fatalf("Second bootstrap failed: %v", err)
	}
	if !recovered {
		t.Error("Second bootstrap should return true (snapshot recovered)")
	}

	// 验证状态一致性
	if len(engine2.state.Levels) != 2 {
		t.Errorf("Expected 2 levels after recovery, got %d", len(engine2.state.Levels))
	}

	// 验证level 500
	if engine2.state.Levels[0].LevelID != 500 {
		t.Errorf("Expected level 500, got %d", engine2.state.Levels[0].LevelID)
	}
	if engine2.state.Levels[0].Entry.OrderID != 123456 {
		t.Errorf("Expected OrderID 123456, got %d", engine2.state.Levels[0].Entry.OrderID)
	}
	if engine2.state.Levels[0].Entry.State != model.OrderStateOpen {
		t.Errorf("Expected OPEN state, got %v", engine2.state.Levels[0].Entry.State)
	}

	// 验证CLID索引
	if orderId, ok := engine2.state.CLIDIndex["TEST:E:500:1"]; !ok || orderId != 123456 {
		t.Errorf("CLID index not recovered: got %d, ok=%v", orderId, ok)
	}

	// 验证EngineMode
	if engine2.state.EngineMode != model.EngineModeRunning {
		t.Errorf("Expected RUNNING mode, got %v", engine2.state.EngineMode)
	}

	t.Log("Crash recovery successful: state consistent after restart")
}

// TestEngine_Bootstrap_NoSnapshot 测试无snapshot的首次启动
func TestEngine_Bootstrap_NoSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "nonexistent.json")

	cfg := EngineConfig{
		Prefix:       "TEST",
		Symbol:       "BTCUSDT",
		SnapshotPath: snapshotPath,
	}

	engine := NewEngine(cfg)

	// Bootstrap应该返回false（无snapshot）
	recovered, err := engine.Bootstrap()
	if err != nil {
		t.Errorf("Bootstrap should not error when no snapshot: %v", err)
	}
	if recovered {
		t.Error("Bootstrap should return false when no snapshot exists")
	}

	// 状态应该是初始状态
	if len(engine.state.Levels) != 0 {
		t.Errorf("Initial state should have 0 levels, got %d", len(engine.state.Levels))
	}
}

// TestEngine_Bootstrap_CorruptedSnapshot 测试损坏的snapshot
func TestEngine_Bootstrap_CorruptedSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "corrupted.json")

	// 创建一个损坏的snapshot文件
	corruptedData := []byte("{ this is not valid json }")
	if err := os.WriteFile(snapshotPath, corruptedData, 0644); err != nil {
		t.Fatalf("Write corrupted file failed: %v", err)
	}

	cfg := EngineConfig{
		Prefix:       "TEST",
		Symbol:       "BTCUSDT",
		SnapshotPath: snapshotPath,
	}

	engine := NewEngine(cfg)

	// Bootstrap应该返回错误（snapshot损坏）
	recovered, err := engine.Bootstrap()
	if err == nil {
		t.Error("Bootstrap should return error for corrupted snapshot")
	}
	if recovered {
		t.Error("Bootstrap should return false for corrupted snapshot")
	}

	t.Logf("Expected error received: %v", err)
}

// TestEngine_SaveSnapshot_Periodic 测试定期保存snapshot
func TestEngine_SaveSnapshot_Periodic(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "periodic.json")

	cfg := EngineConfig{
		Prefix:       "TEST",
		Symbol:       "BTCUSDT",
		SnapshotPath: snapshotPath,
	}

	engine := NewEngine(cfg)

	// 第一次保存
	engine.state.Levels = []model.LevelState{
		{LevelID: 1, PriceTicks: 50000000},
	}
	if err := engine.SaveSnapshot(); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// 修改状态
	engine.state.Levels = append(engine.state.Levels, model.LevelState{
		LevelID:    2,
		PriceTicks: 50100000,
	})

	// 第二次保存（应该覆盖）
	if err := engine.SaveSnapshot(); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	// 重新加载验证
	engine2 := NewEngine(cfg)
	recovered, err := engine2.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if !recovered {
		t.Error("Should recover from snapshot")
	}

	// 验证是最新状态（2个levels）
	if len(engine2.state.Levels) != 2 {
		t.Errorf("Expected 2 levels, got %d", len(engine2.state.Levels))
	}
}
