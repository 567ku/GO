// P0-1: TaskID 唯一性测试
package model

import (
	"sync"
	"testing"
)

// TestGenerateTaskID_Uniqueness 测试 TaskID 唯一性（1e5次无冲突）
func TestGenerateTaskID_Uniqueness(t *testing.T) {
	const iterations = 100000 // 1e5 次生成

	ids := make(map[string]struct{}, iterations)
	var mu sync.Mutex

	// 并发生成（模拟高并发场景）
	const goroutines = 10
	var wg sync.WaitGroup
	perGoroutine := iterations / goroutines

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				id := GenerateTaskID()

				mu.Lock()
				if _, exists := ids[id]; exists {
					t.Errorf("Duplicate TaskID detected: %s", id)
				}
				ids[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 验证：所有ID都是唯一的
	if len(ids) != iterations {
		t.Errorf("Expected %d unique IDs, got %d", iterations, len(ids))
	}

	t.Logf("✅ Generated %d unique TaskIDs (no collisions)", len(ids))
}

// BenchmarkGenerateTaskID 基准测试 TaskID 生成性能
func BenchmarkGenerateTaskID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateTaskID()
	}
}

// TestGenerateReconcileTaskID_Uniqueness 测试 ReconcileTaskID 唯一性
func TestGenerateReconcileTaskID_Uniqueness(t *testing.T) {
	const iterations = 100000

	ids := make(map[string]struct{}, iterations)
	var mu sync.Mutex

	const goroutines = 10
	perGoroutine := iterations / goroutines

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				id := GenerateReconcileTaskID()

				mu.Lock()
				if _, exists := ids[id]; exists {
					t.Errorf("Duplicate ReconcileTaskID detected: %s", id)
				}
				ids[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if len(ids) != iterations {
		t.Errorf("Expected %d unique ReconcileTaskIDs, got %d", iterations, len(ids))
	}

	t.Logf("✅ Generated %d unique ReconcileTaskIDs (no collisions)", len(ids))
}

// TestTaskID_Format 测试 TaskID 格式正确性
func TestTaskID_Format(t *testing.T) {
	id := GenerateTaskID()

	// 验证格式: task_<timestamp>_<random16>_<counter4>
	// 示例: task_1732672800000_0123456789abcdef_0001
	if len(id) < 30 {
		t.Errorf("TaskID too short: %s (expected >= 30 chars)", id)
	}

	if id[:5] != "task_" {
		t.Errorf("TaskID must start with 'task_', got: %s", id)
	}

	t.Logf("✅ TaskID format validated: %s", id)
}
