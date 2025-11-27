package store

import (
	"gridbot/pkg/model"
	"os"
	"path/filepath"
	"testing"
)

// TestSnapshotStore_SaveAndLoad 测试Save和Load基本功能
func TestSnapshotStore_SaveAndLoad(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "test_snapshot.json")

	store := NewSnapshotStore(snapshotPath)

	// 创建测试快照
	originalSnapshot := &model.GridStateSnapshot{
		Version:    "1.3",
		SavedAtMs:  model.NowMs(),
		Symbol:     "BTCUSDT",
		Prefix:     "TEST",
		EngineMode: model.EngineModeRunning,
		Side:       model.GridSideLong,
		Window: model.WindowState{
			WinMinTicks: 49000000,
			WinMaxTicks: 51000000,
			StepTicks:   100000,
		},
		Market: model.MarketState{
			LastPriceTicks: 50000000,
		},
		Levels: []model.LevelState{
			{
				LevelID:    500,
				PriceTicks: 50000000,
				Entry: model.OrderSlot{
					Purpose: model.OrderPurposeEntry,
					State:   model.OrderStateOpen,
					OrderID: 123456,
				},
			},
		},
		CLIDIndex: map[string]int64{
			"TEST:E:500:1": 123456,
		},
	}

	// 保存快照
	if err := store.Save(originalSnapshot); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 验证文件存在
	if !store.Exists() {
		t.Error("Snapshot file should exist after Save")
	}

	// 加载快照
	loadedSnapshot, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证数据一致性
	if loadedSnapshot.Symbol != originalSnapshot.Symbol {
		t.Errorf("Symbol mismatch: got %s, want %s", loadedSnapshot.Symbol, originalSnapshot.Symbol)
	}
	if loadedSnapshot.EngineMode != originalSnapshot.EngineMode {
		t.Errorf("EngineMode mismatch: got %v, want %v", loadedSnapshot.EngineMode, originalSnapshot.EngineMode)
	}
	if len(loadedSnapshot.Levels) != len(originalSnapshot.Levels) {
		t.Errorf("Levels count mismatch: got %d, want %d", len(loadedSnapshot.Levels), len(originalSnapshot.Levels))
	}
}

// TestSnapshotStore_AtomicWrite 测试原子写入（崩溃安全）
func TestSnapshotStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "atomic_test.json")

	store := NewSnapshotStore(snapshotPath)

	snapshot := &model.GridStateSnapshot{
		Version: "1.3",
		Symbol:  "BTCUSDT",
		Prefix:  "TEST",
	}

	// 第一次保存
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// 第二次保存（覆盖）- 应该原子替换
	snapshot.EngineMode = model.EngineModeFreeze
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	// 验证临时文件已被清理
	tmpFile := snapshotPath + ".tmp"
	if _, err := os.Stat(tmpFile); err == nil {
		t.Error("Temporary file should be cleaned up after atomic rename")
	}

	// 加载并验证是最新数据
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.EngineMode != model.EngineModeFreeze {
		t.Errorf("Expected FREEZE mode, got %v", loaded.EngineMode)
	}
}

// TestSnapshotStore_ChecksumVerification 测试校验和验证
func TestSnapshotStore_ChecksumVerification(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "checksum_test.json")

	store := NewSnapshotStore(snapshotPath)

	snapshot := &model.GridStateSnapshot{
		Version: "1.3",
		Symbol:  "BTCUSDT",
		Prefix:  "TEST",
	}

	// 保存快照
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 验证完整性
	if err := store.Verify(); err != nil {
		t.Errorf("Verify failed: %v", err)
	}

	// 故意损坏文件（修改一个字节）
	data, _ := os.ReadFile(snapshotPath)
	data[len(data)-10] = 'X' // 修改最后附近的字节
	os.WriteFile(snapshotPath, data, 0644)

	// 验证应该失败
	if err := store.Verify(); err == nil {
		t.Error("Verify should fail for corrupted file")
	}

	// Load也应该失败
	if _, err := store.Load(); err == nil {
		t.Error("Load should fail for corrupted file")
	}
}

// TestSnapshotStore_NotFound 测试文件不存在的场景
func TestSnapshotStore_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "nonexistent.json")

	store := NewSnapshotStore(snapshotPath)

	// Exists应该返回false
	if store.Exists() {
		t.Error("Exists should return false for non-existent file")
	}

	// Load应该返回ErrSnapshotNotFound
	_, err := store.Load()
	if err != ErrSnapshotNotFound {
		t.Errorf("Load should return ErrSnapshotNotFound, got: %v", err)
	}

	// Verify应该返回ErrSnapshotNotFound
	err = store.Verify()
	if err != ErrSnapshotNotFound {
		t.Errorf("Verify should return ErrSnapshotNotFound, got: %v", err)
	}
}

// TestSnapshotStore_Delete 测试删除功能
func TestSnapshotStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "delete_test.json")

	store := NewSnapshotStore(snapshotPath)

	snapshot := &model.GridStateSnapshot{
		Version: "1.3",
		Symbol:  "BTCUSDT",
	}

	// 保存
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 验证存在
	if !store.Exists() {
		t.Error("File should exist before delete")
	}

	// 删除
	if err := store.Delete(); err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	// 验证不存在
	if store.Exists() {
		t.Error("File should not exist after delete")
	}

	// 重复删除不应报错
	if err := store.Delete(); err != nil {
		t.Errorf("Delete should be idempotent: %v", err)
	}
}

// TestSnapshotStore_ConcurrentSave 测试并发保存（应该串行执行）
func TestSnapshotStore_ConcurrentSave(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "concurrent_test.json")

	store := NewSnapshotStore(snapshotPath)

	// 并发保存多个快照
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			snapshot := &model.GridStateSnapshot{
				Version: "1.3",
				Symbol:  "BTCUSDT",
				Prefix:  "TEST",
				Levels: []model.LevelState{
					{LevelID: id},
				},
			}
			if err := store.Save(snapshot); err != nil {
				t.Errorf("Concurrent save %d failed: %v", id, err)
			}
			done <- true
		}(i)
	}

	// 等待所有保存完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证文件存在且可加载（不应损坏）
	if _, err := store.Load(); err != nil {
		t.Errorf("Final load failed: %v", err)
	}

	// 验证校验和正确
	if err := store.Verify(); err != nil {
		t.Errorf("Final verify failed: %v", err)
	}
}
