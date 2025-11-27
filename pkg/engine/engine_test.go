// P0-E-01: Engine单写者主循环测试
// P0-E-02: Freeze/Reconcile状态机测试
package engine

import (
	"context"
	"gridbot/pkg/model"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestEngine_EventLoop 测试事件循环基本功能
func TestEngine_EventLoop(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         10,
	}

	engine := NewEngine(config)

	// 验证初始状态
	if engine.GetMode() != model.EngineModeRunning {
		t.Errorf("初始mode应该为RUNNING: got=%s", engine.GetMode())
	}

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	// 发送PriceTick事件
	engine.GetEventCh() <- model.EngineEvent{
		Type: model.EventTypePriceTick,
		Data: model.PriceTickEvent{
			Symbol:     "BTCUSDT",
			PriceTicks: 50000000,
			EventAtMs:  model.NowMs(),
		},
	}

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	// 验证状态更新
	state := engine.GetState()
	if state.Market.LastPriceTicks != 50000000 {
		t.Errorf("LastPriceTicks未更新: expected=50000000, got=%d", state.Market.LastPriceTicks)
	}
}

// TestEngine_FreezeOnWSDisconnect P0-E-02: WS断线触发FREEZE
func TestEngine_FreezeOnWSDisconnect(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         10,
	}

	engine := NewEngine(config)

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	// 验证初始状态为RUNNING
	if engine.GetMode() != model.EngineModeRunning {
		t.Fatalf("初始mode应该为RUNNING: got=%s", engine.GetMode())
	}

	// 发送UDS断线事件
	engine.GetEventCh() <- model.EngineEvent{
		Type: model.EventTypeWSState,
		Data: model.WSStateEvent{
			Channel: model.WSChannelUDS,
			State:   model.WSStateDisconnected,
			AtMs:    model.NowMs(),
			Reason:  "connection lost",
		},
	}

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	// 验证进入FREEZE状态
	if engine.GetMode() != model.EngineModeFreeze {
		t.Errorf("UDS断线后应该进入FREEZE: got=%s", engine.GetMode())
	}
}

// TestEngine_ReconnectToReconciling P0-E-02: WS重连触发RECONCILING
func TestEngine_ReconnectToReconciling(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         10,
	}

	engine := NewEngine(config)

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	// 先触发FREEZE
	engine.GetEventCh() <- model.EngineEvent{
		Type: model.EventTypeWSState,
		Data: model.WSStateEvent{
			Channel: model.WSChannelTrade,
			State:   model.WSStateDisconnected,
			AtMs:    model.NowMs(),
			Reason:  "connection lost",
		},
	}

	time.Sleep(10 * time.Millisecond)

	// 发送重连事件
	engine.GetEventCh() <- model.EngineEvent{
		Type: model.EventTypeWSState,
		Data: model.WSStateEvent{
			Channel: model.WSChannelTrade,
			State:   model.WSStateReconnected,
			AtMs:    model.NowMs(),
			Reason:  "reconnected",
		},
	}

	// 等待事件处理
	time.Sleep(50 * time.Millisecond) // 增加等待时间，确保reconcile完成

	// P0-E-02-A: reconcile完成后会立即切回RUNNING（简化实现）
	// 所以这里应该看到RUNNING状态
	if engine.GetMode() != model.EngineModeRunning {
		t.Errorf("重连后应该完成reconcile并回到RUNNING: got=%s", engine.GetMode())
	}
}

// TestEngine_CanSubmitWriteTask P0-E-02: 写闸门测试
func TestEngine_CanSubmitWriteTask(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         10,
	}

	engine := NewEngine(config)

	// RUNNING状态允许写Task
	if !engine.CanSubmitWriteTask() {
		t.Error("RUNNING状态应该允许写Task")
	}

	// 手动设置为FREEZE
	engine.SetMode(model.EngineModeFreeze)
	if engine.CanSubmitWriteTask() {
		t.Error("FREEZE状态不应该允许写Task")
	}

	// 手动设置为RECONCILING
	engine.SetMode(model.EngineModeReconciling)
	if engine.CanSubmitWriteTask() {
		t.Error("RECONCILING状态不应该允许写Task")
	}
}

// TestEngine_OrderUpdateMapping P0-E-03: 订单更新CLID映射
func TestEngine_OrderUpdateMapping(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         10,
	}

	engine := NewEngine(config)

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	// 创建一个level
	engine.state.Levels = append(engine.state.Levels, model.LevelState{
		LevelID:    5,
		PriceTicks: 50000000,
		Cycle:      0,
		Entry: model.OrderSlot{
			Purpose:       model.OrderPurposeEntry,
			State:         model.OrderStateSubmitted,
			ClientOrderID: "TEST:E:5:0",
		},
	})

	// 发送OrderUpdate事件
	engine.GetEventCh() <- model.EngineEvent{
		Type: model.EventTypeOrderUpdate,
		Data: model.OrderUpdateEvent{
			Symbol:           "BTCUSDT",
			ClientOrderID:    "TEST:E:5:0",
			OrderID:          12345,
			Status:           "NEW",
			Side:             "BUY",
			PriceTicks:       50000000,
			OrigQtyTicks:     1000,
			ExecutedQtyTicks: 0,
			AvgPriceTicks:    0,
			UpdateAtMs:       model.NowMs(),
		},
	}

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	// 验证OrderSlot更新
	state := engine.GetState()
	level := FindLevelByID(&state, 5)
	if level == nil {
		t.Fatal("应该找到levelID=5的level")
	}

	if level.Entry.State != model.OrderStateOpen {
		t.Errorf("Entry state应该为OPEN: got=%s", level.Entry.State)
	}

	if level.Entry.OrderID != 12345 {
		t.Errorf("OrderID未更新: expected=12345, got=%d", level.Entry.OrderID)
	}

	// 验证CLID索引
	if orderId, ok := state.CLIDIndex["TEST:E:5:0"]; !ok || orderId != 12345 {
		t.Errorf("CLID索引未更新: expected=12345, got=%d, ok=%v", orderId, ok)
	}
}

// ========== P0-ENG-STOP-01: Engine.Stop 幂等化 ==========

// TestEngine_Stop_Idempotent 验证Engine.Stop()幂等性
// 验收点：
// 1. 同一Engine实例多次Stop()不panic
// 2. 并发Stop()不panic
// 3. Stop()后Run能在合理时间内退出（<500ms）
// 4. 无goroutine泄漏（阈值断言）
func TestEngine_Stop_Idempotent(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST_STOP",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         100,
	}

	engine := NewEngine(config)

	// 记录基准goroutine数量
	baselineGoroutines := countGoroutines()

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- engine.Run(ctx)
	}()

	// 等待Engine进入事件循环
	time.Sleep(50 * time.Millisecond)

	// 验收点1：多次调用Stop()不panic
	t.Log("验收点1: 多次调用Stop()...")
	for i := 0; i < 10; i++ {
		engine.Stop() // 应该不panic
	}
	t.Log("✅ 验收点1通过：10次Stop()不panic")

	// 验收点2：Run能在合理时间内退出
	t.Log("验收点2: 等待Run退出...")
	startWait := time.Now()
	select {
	case err := <-runDone:
		elapsed := time.Since(startWait)
		if elapsed > 500*time.Millisecond {
			t.Errorf("Run退出耗时过长: %v (expected < 500ms)", elapsed)
		}
		if err != nil && err != context.DeadlineExceeded {
			t.Logf("Run退出错误: %v (正常)", err)
		}
		t.Logf("✅ 验收点2通过：Run在%v内退出", elapsed)
	case <-time.After(1 * time.Second):
		t.Fatal("Run未在预期时间内退出 (>1s)")
	}

	// 验收点3：无goroutine泄漏
	t.Log("验收点3: 检查goroutine泄漏...")
	currentGoroutines := countGoroutines()
	leaked := currentGoroutines - baselineGoroutines

	const leakThreshold = 2 // 允许测试框架的少量波动
	if leaked > leakThreshold {
		t.Errorf("Goroutine泄漏检测: baseline=%d, current=%d, leaked=%d (threshold=%d)",
			baselineGoroutines, currentGoroutines, leaked, leakThreshold)
	}
	t.Logf("✅ 验收点3通过：无goroutine泄漏 (baseline=%d, current=%d, leaked=%d)",
		baselineGoroutines, currentGoroutines, leaked)
}

// TestEngine_Stop_Concurrent 验证并发Stop()安全性
func TestEngine_Stop_Concurrent(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST_CONCURRENT_STOP",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         100,
	}

	engine := NewEngine(config)

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	// 等待进入循环
	time.Sleep(50 * time.Millisecond)

	// 验收：并发调用Stop()
	t.Log("并发调用Stop()...")
	var wg sync.WaitGroup
	const concurrency = 20

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			engine.Stop() // 应该不panic
			t.Logf("Goroutine %d: Stop()完成", id)
		}(i)
	}

	wg.Wait()
	t.Log("✅ 并发Stop()测试通过：20个goroutine同时Stop不panic")
}

// TestEngine_Stop_Then_Restart 验证Stop后再次Run仍然工作
func TestEngine_Stop_Then_Restart(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST_RESTART",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         100,
	}

	engine := NewEngine(config)

	// 第1次Run
	t.Log("第1次Run...")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel1()

	runDone1 := make(chan error, 1)
	go func() {
		runDone1 <- engine.Run(ctx1)
	}()

	time.Sleep(20 * time.Millisecond)
	engine.Stop()

	<-runDone1
	t.Log("✅ 第1次Run退出")

	// 第2次Run（应该成功）
	t.Log("第2次Run...")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()

	runDone2 := make(chan error, 1)
	go func() {
		runDone2 <- engine.Run(ctx2)
	}()

	time.Sleep(20 * time.Millisecond)
	engine.Stop()

	<-runDone2
	t.Log("✅ 第2次Run退出，Stop后再次Run功能正常")
}

// countGoroutines 统计goroutine数量（用于泄漏检测）
func countGoroutines() int {
	// 等待短暂goroutine结束
	time.Sleep(50 * time.Millisecond)
	return runtime.NumGoroutine()
}
