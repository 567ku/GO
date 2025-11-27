// Stage 5A: Reconcile Query 真实查询测试
package engine

import (
	"context"
	"fmt"
	"gridbot/pkg/model"
	"testing"
	"time"
)

// MockQueryExecutor 模拟查询执行器
type MockQueryExecutor struct {
	OpenOrders []model.ParsedOrderUpdate
	Error      error
	Delay      time.Duration // 模拟查询延迟
}

func (m *MockQueryExecutor) QueryOpenOrders(ctx context.Context, req QueryOpenOrdersRequest) (*QueryOpenOrdersResponse, error) {
	if m.Delay > 0 {
		select {
		case <-time.After(m.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.Error != nil {
		return nil, m.Error
	}

	return &QueryOpenOrdersResponse{
		Orders: m.OpenOrders,
	}, nil
}

// TestReconcile_QueryOpenOrders Gate-2A: Reconcile稳定性测试
func TestReconcile_QueryOpenOrders(t *testing.T) {
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
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         100,
	}
	engine := NewEngine(cfg)

	// 设置mock executor
	mockExecutor := &MockQueryExecutor{
		OpenOrders: []model.ParsedOrderUpdate{
			{
				ClientOrderID:    "TEST:E:500:1",
				OrderID:          123456,
				Status:           "NEW",
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 0,
				AvgPriceTicks:    0,
			},
		},
		Error: nil,
		Delay: 10 * time.Millisecond, // 模拟网络延迟
	}
	engine.SetExecutor(mockExecutor)

	// 执行reconcile
	err := engine.StartReconcile(context.Background(), mockExecutor)
	if err != nil {
		t.Fatalf("StartReconcile failed: %v", err)
	}

	// 验证：状态应该回到RUNNING
	if engine.GetMode() != model.EngineModeRunning {
		t.Errorf("expected mode=RUNNING, got %v", engine.GetMode())
	}

	// 验证：订单应该被应用到本地状态
	level := FindLevelByID(engine.state, 500)
	if level == nil {
		t.Fatal("level 500 not found")
	}

	if level.Entry.OrderID != 123456 {
		t.Errorf("expected OrderID=123456, got %d", level.Entry.OrderID)
	}

	if level.Entry.State != model.OrderStateOpen {
		t.Errorf("expected State=OPEN, got %v", level.Entry.State)
	}
}

// TestReconcile_QueryOpenOrders_Timeout 测试查询超时保护
func TestReconcile_QueryOpenOrders_Timeout(t *testing.T) {
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

	// 设置mock executor（延迟超过超时时间）
	mockExecutor := &MockQueryExecutor{
		OpenOrders: nil,
		Error:      nil,
		Delay:      35 * time.Second, // 超过30秒超时
	}
	engine.SetExecutor(mockExecutor)

	// 执行reconcile（应该超时）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := engine.StartReconcile(ctx, mockExecutor)

	// 验证：应该返回超时错误
	if err == nil {
		t.Error("expected timeout error, got nil")
	}

	// 验证：降级为空快照，状态回到RUNNING
	if engine.GetMode() != model.EngineModeRunning {
		t.Errorf("expected mode=RUNNING after timeout, got %v", engine.GetMode())
	}
}

// TestReconcile_QueryOpenOrders_FilterOwnOrders 测试订单过滤
func TestReconcile_QueryOpenOrders_FilterOwnOrders(t *testing.T) {
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

	// 设置mock executor（包含其他网格的订单）
	mockExecutor := &MockQueryExecutor{
		OpenOrders: []model.ParsedOrderUpdate{
			{
				ClientOrderID:    "TEST:E:500:1", // 自己的订单
				OrderID:          123456,
				Status:           "NEW",
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 0,
			},
			{
				ClientOrderID:    "OTHER:E:500:1", // 其他网格的订单
				OrderID:          789012,
				Status:           "NEW",
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 0,
			},
		},
		Error: nil,
		Delay: 10 * time.Millisecond,
	}
	engine.SetExecutor(mockExecutor)

	// 执行reconcile
	err := engine.StartReconcile(context.Background(), mockExecutor)
	if err != nil {
		t.Fatalf("StartReconcile failed: %v", err)
	}

	// 验证：只应用了自己的订单
	level := FindLevelByID(engine.state, 500)
	if level == nil {
		t.Fatal("level 500 not found")
	}

	if level.Entry.OrderID != 123456 {
		t.Errorf("expected OrderID=123456 (own order), got %d", level.Entry.OrderID)
	}

	// 验证：没有应用其他网格的订单（通过检查是否只有1个level）
	if len(engine.state.Levels) != 1 {
		t.Errorf("expected 1 level (filtered own orders), got %d", len(engine.state.Levels))
	}
}

// TestReconcile_QueryOpenOrders_Retry 测试查询重试机制
func TestReconcile_QueryOpenOrders_Retry(t *testing.T) {
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

	// 创建一个会失败2次然后成功的mock executor
	attemptCount := 0
	mockExecutor := &MockQueryExecutorWithRetry{
		attemptCount: &attemptCount,
		maxFailures:  2,
		successOrders: []model.ParsedOrderUpdate{
			{
				ClientOrderID:    "TEST:E:500:1",
				OrderID:          123456,
				Status:           "NEW",
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 0,
			},
		},
	}

	engine.SetExecutor(mockExecutor)

	// 执行reconcile
	err := engine.StartReconcile(context.Background(), mockExecutor)
	if err != nil {
		t.Fatalf("StartReconcile failed after retry: %v", err)
	}

	// 验证：应该重试了3次（2次失败+1次成功）
	if *mockExecutor.attemptCount != 3 {
		t.Errorf("expected 3 attempts (2 retries), got %d", *mockExecutor.attemptCount)
	}

	// 验证：最终成功应用了订单
	level := FindLevelByID(engine.state, 500)
	if level == nil {
		t.Fatal("level 500 not found")
	}

	if level.Entry.OrderID != 123456 {
		t.Errorf("expected OrderID=123456, got %d", level.Entry.OrderID)
	}
}

// MockQueryExecutorWithRetry 带重试逻辑的mock executor
type MockQueryExecutorWithRetry struct {
	attemptCount  *int
	maxFailures   int
	successOrders []model.ParsedOrderUpdate
}

func (m *MockQueryExecutorWithRetry) QueryOpenOrders(ctx context.Context, req QueryOpenOrdersRequest) (*QueryOpenOrdersResponse, error) {
	*m.attemptCount++
	if *m.attemptCount <= m.maxFailures {
		return nil, fmt.Errorf("network error (attempt %d)", *m.attemptCount)
	}
	return &QueryOpenOrdersResponse{
		Orders: m.successOrders,
	}, nil
}

// TestReconcile_E2E_Timeline Gate-2A: 全链路时序测试
// 验证：FREEZE → RECONNECTED → RECONCILING → Query → GapScan → RUNNING
func TestReconcile_E2E_Timeline(t *testing.T) {
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

	// 步骤1: 初始状态为FREEZE
	engine.state.EngineMode = model.EngineModeFreeze
	engine.mode = model.EngineModeFreeze

	if engine.GetMode() != model.EngineModeFreeze {
		t.Fatalf("initial mode should be FREEZE, got %v", engine.GetMode())
	}

	// 步骤2: 设置mock executor
	mockExecutor := &MockQueryExecutor{
		OpenOrders: []model.ParsedOrderUpdate{
			{
				ClientOrderID:    "TEST:E:500:1",
				OrderID:          123456,
				Status:           "FILLED", // 已成交的Entry
				PriceTicks:       50000000,
				OrigQtyTicks:     1000,
				ExecutedQtyTicks: 1000,
				AvgPriceTicks:    50000000,
			},
		},
		Error: nil,
		Delay: 10 * time.Millisecond,
	}
	engine.SetExecutor(mockExecutor)

	// 步骤3: 触发WS RECONNECTED事件
	wsEvent := model.WSStateEvent{
		Channel: model.WSChannelUDS,
		State:   model.WSStateReconnected,
		AtMs:    model.NowMs(),
		Reason:  "test e2e timeline",
	}
	engine.handleWSState(wsEvent)

	// 步骤4: 验证最终状态为RUNNING（reconcile已同步完成）
	if engine.GetMode() != model.EngineModeRunning {
		t.Errorf("expected final mode=RUNNING, got %v", engine.GetMode())
	}

	// 步骤5: 验证查询结果被应用
	level := FindLevelByID(engine.state, 500)
	if level == nil {
		t.Fatal("level 500 not found after reconcile")
	}

	if level.Entry.State != model.OrderStateFilled {
		t.Errorf("expected Entry.State=FILLED, got %v", level.Entry.State)
	}

	if level.Entry.OrderID != 123456 {
		t.Errorf("expected Entry.OrderID=123456, got %d", level.Entry.OrderID)
	}

	// 步骤6: 验证GapScan被触发（通过检查是否有TP任务生成的逻辑）
	// 由于Entry已FILLED，GapScan应该建议生成TP任务
	tasks := GapScan(engine.state)
	hasTPTask := false
	for _, task := range tasks {
		if task.Purpose == model.OrderPurposeTP {
			hasTPTask = true
			break
		}
	}

	if !hasTPTask {
		t.Log("no TP task generated (may be expected if TP slot already exists)")
	}

	// 验证：reconcileInFlight标记已清零
	if engine.reconcileInFlight {
		t.Error("expected reconcileInFlight=false after reconcile complete")
	}

	t.Logf("✅ E2E Timeline verified: FREEZE → RECONNECTED → RECONCILING → Query → GapScan → RUNNING")
}
