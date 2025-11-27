// P0-A/P0-B: WSExecutor 真实断言测试（reduceOnly bool + 精度格式化）
package executor

import (
	"fmt"
	"gridbot/pkg/model"
	"gridbot/pkg/ws"
	"runtime"
	"sync"
	"testing"
	"time"
)

// ========== P0-A: reduceOnly 必须是 bool 类型 ==========

func TestBuildRequest_ReduceOnlyBool(t *testing.T) {
	tradeClient := &ws.TradeClient{}
	eventCh := make(chan model.EngineEvent, 10)
	exec := NewWSExecutor(tradeClient, eventCh, 2, 3, 2048) // P1-1: 增加 outboxSize 参数

	// 测试用例1：reduceOnly = true
	reduceOnlyTrue := true
	task := &model.Task{
		TaskID:        "test_reduce_true",
		Type:          model.TaskTypePlaceTP,
		Symbol:        "BTCUSDT",
		ClientOrderID: "TEST:TP:100:0",
		PriceTicks:    50000,
		QtyTicks:      1000,
		PositionSide:  "BOTH",
		ReduceOnly:    &reduceOnlyTrue,
	}

	// 直接调用 buildRequest（同包可访问未导出方法）
	_, params, err := exec.buildRequest(task)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// P0-A 验收：断言 reduceOnly 的类型是 bool，不是 string
	reduceOnlyVal, exists := params["reduceOnly"]
	if !exists {
		t.Errorf("Expected reduceOnly in params, not found")
	}

	// 类型断言
	if boolVal, ok := reduceOnlyVal.(bool); !ok {
		t.Errorf("Expected reduceOnly to be bool type, got %T", reduceOnlyVal)
	} else if !boolVal {
		t.Errorf("Expected reduceOnly=true, got false")
	}

	// 测试用例2：reduceOnly = nil（不应出现在params中）
	task2 := &model.Task{
		TaskID:        "test_reduce_nil",
		Type:          model.TaskTypePlaceEntry,
		Symbol:        "BTCUSDT",
		ClientOrderID: "TEST:E:100:0",
		PriceTicks:    50000,
		QtyTicks:      1000,
		PositionSide:  "BOTH",
		ReduceOnly:    nil, // Entry 订单不需要 reduceOnly
	}

	_, params2, err := exec.buildRequest(task2)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// 验证：reduceOnly 不应存在
	if _, exists := params2["reduceOnly"]; exists {
		t.Errorf("Expected reduceOnly absent for Entry order, but found")
	}
}

// ========== P0-B: 精度注入有效断言测试 ==========

func TestBuildRequest_PrecisionInjection(t *testing.T) {
	tradeClient := &ws.TradeClient{}
	eventCh := make(chan model.EngineEvent, 10)

	// 测试用例1：精度 (1, 3) - PLACE_ENTRY
	exec1 := NewWSExecutor(tradeClient, eventCh, 1, 3, 2048) // P1-1: 增加 outboxSize 参数
	task1 := &model.Task{
		TaskID:        "test_precision_entry",
		Type:          model.TaskTypePlaceEntry,
		Symbol:        "BTCUSDT",
		ClientOrderID: "TEST:E:100:0",
		PriceTicks:    50123, // 50123 ticks
		QtyTicks:      1234,  // 1234 ticks
		PositionSide:  "BOTH",
	}

	_, params1, err := exec1.buildRequest(task1)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// P0-B 验收：断言 price 小数位数符合注入精度（1位）
	price1, ok := params1["price"].(string)
	if !ok {
		t.Fatalf("Expected price to be string, got %T", params1["price"])
	}
	// FormatPriceDecimal(50123, 1) 应该是 "5012.3"（1位小数）
	expectedPrice1 := "5012.3"
	if price1 != expectedPrice1 {
		t.Errorf("Expected price=%s (precision=1), got %s", expectedPrice1, price1)
	}

	// 断言 quantity 小数位数符合注入精度（3位）
	qty1, ok := params1["quantity"].(string)
	if !ok {
		t.Fatalf("Expected quantity to be string, got %T", params1["quantity"])
	}
	// FormatQtyDecimal(1234, 3) 应该是 "1.234"（3位小数）
	expectedQty1 := "1.234"
	if qty1 != expectedQty1 {
		t.Errorf("Expected quantity=%s (precision=3), got %s", expectedQty1, qty1)
	}

	// 测试用例2：精度 (2, 4) - PLACE_TP
	exec2 := NewWSExecutor(tradeClient, eventCh, 2, 4, 2048) // P1-1: 增加 outboxSize 参数
	task2 := &model.Task{
		TaskID:        "test_precision_tp",
		Type:          model.TaskTypePlaceTP,
		Symbol:        "BTCUSDT",
		ClientOrderID: "TEST:TP:100:0",
		PriceTicks:    50123, // 50123 ticks
		QtyTicks:      1234,  // 1234 ticks
		PositionSide:  "BOTH",
	}

	_, params2, err := exec2.buildRequest(task2)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// 断言 price 小数位数符合注入精度（2位）
	price2, ok := params2["price"].(string)
	if !ok {
		t.Fatalf("Expected price to be string, got %T", params2["price"])
	}
	// FormatPriceDecimal(50123, 2) 应该是 "501.23"（2位小数）
	expectedPrice2 := "501.23"
	if price2 != expectedPrice2 {
		t.Errorf("Expected price=%s (precision=2), got %s", expectedPrice2, price2)
	}

	// 断言 quantity 小数位数符合注入精度（4位）
	qty2, ok := params2["quantity"].(string)
	if !ok {
		t.Fatalf("Expected quantity to be string, got %T", params2["quantity"])
	}
	// FormatQtyDecimal(1234, 4) 应该是 "0.1234"（4位小数）
	expectedQty2 := "0.1234"
	if qty2 != expectedQty2 {
		t.Errorf("Expected quantity=%s (precision=4), got %s", expectedQty2, qty2)
	}
}

// ========== P0-1: ExecutorResult 禁止丢弃（Outbox + Pump + Backpressure） ==========

func TestExecutorResult_NoDrop_WithBackpressure(t *testing.T) {
	// 构造一个很小的 eventCh（模拟 Engine 处理慢）
	smallEventCh := make(chan model.EngineEvent, 2) // 只有2个缓冲
	tradeClient := &ws.TradeClient{}                // fake client

	exec := NewWSExecutor(tradeClient, smallEventCh, 2, 3, 2048) // P1-1: 增加 outboxSize 参数
	defer exec.Stop()

	// 模拟阻塞消费（不从 eventCh 读取）
	var wg sync.WaitGroup
	const totalResults = 3000 // 发送3000个 ExecutorResult（超过 outbox 2048 缓冲）

	// 在后台慢慢消费 eventCh（模拟Engine处理慢）
	receivedCount := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalResults; i++ {
			<-smallEventCh
			receivedCount++
			if i%500 == 0 {
				time.Sleep(10 * time.Millisecond) // 偶尔慢消费
			}
		}
	}()

	// 快速发送多个 ExecutorResult（触发 outbox backpressure）
	for i := 0; i < totalResults; i++ {
		task := &model.Task{
			TaskID:        fmt.Sprintf("task_%d", i),
			Type:          model.TaskTypePlaceEntry,
			ClientOrderID: fmt.Sprintf("TEST_%d", i),
		}
		// 调用 sendExecutorResult（应该不丢弃）
		exec.sendExecutorResult(task, true, 0, "", nil)
	}

	// 等待所有事件被消费
	wg.Wait()

	// P0-1 验收：断言所有 ExecutorResult 都被收到（不丢弃）
	if receivedCount != totalResults {
		t.Errorf("Expected %d ExecutorResults received (no drop), got %d", totalResults, receivedCount)
	}

	// P0-1 验收：断言 backpressure 指标递增（至少发生一次）
	count, lastAtMs := exec.GetBackpressureMetrics()
	if count == 0 {
		t.Errorf("Expected backpressure count > 0, got %d", count)
	}
	if lastAtMs == 0 {
		t.Errorf("Expected lastBackpressureAtMs > 0, got %d", lastAtMs)
	}

	t.Logf("✅ P0-1 验收通过：%d ExecutorResults 全部送达，背压次数=%d", totalResults, count)
}

// TestExecutorResult_ShutdownDrop 测试 shutdown 期间允许丢弃
func TestExecutorResult_ShutdownDrop(t *testing.T) {
	eventCh := make(chan model.EngineEvent, 2)
	tradeClient := &ws.TradeClient{}

	exec := NewWSExecutor(tradeClient, eventCh, 2, 3, 2048) // P1-1: 增加 outboxSize 参数

	// 立即 Stop（触发 ctx.Done()）
	exec.Stop()

	// 尝试发送 ExecutorResult（应该被 shutdown drop）
	task := &model.Task{
		TaskID:        "test_shutdown",
		Type:          model.TaskTypePlaceEntry,
		ClientOrderID: "TEST_SHUTDOWN",
	}

	// sendExecutorResult 不会 panic，但会记录 shutdown_drop 日志
	exec.sendExecutorResult(task, true, 0, "", nil)

	t.Logf("✅ Shutdown drop test passed (no panic)")
}

// ========== P1-1: 不同 outboxSize 下背压计数正确 ==========

// TestOutboxSize_SmallBuffer_Backpressure 测试小缓冲（32）背压
func TestOutboxSize_SmallBuffer_Backpressure(t *testing.T) {
	smallEventCh := make(chan model.EngineEvent, 2)
	tradeClient := &ws.TradeClient{}

	// P1-1: 小 outboxSize = 32
	exec := NewWSExecutor(tradeClient, smallEventCh, 2, 3, 32)
	defer exec.Stop()

	const totalResults = 100
	receivedCount := 0
	var wg sync.WaitGroup

	// 慢消费
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalResults; i++ {
			<-smallEventCh
			receivedCount++
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// 快速发送
	for i := 0; i < totalResults; i++ {
		task := &model.Task{
			TaskID:        fmt.Sprintf("task_%d", i),
			Type:          model.TaskTypePlaceEntry,
			ClientOrderID: fmt.Sprintf("TEST_%d", i),
		}
		exec.sendExecutorResult(task, true, 0, "", nil)
	}

	wg.Wait()

	// 验收：无丢失
	if receivedCount != totalResults {
		t.Errorf("Expected %d results, got %d", totalResults, receivedCount)
	}

	// 验收：小缓冲应该触发更多背压
	count, _ := exec.GetBackpressureMetrics()
	if count == 0 {
		t.Errorf("Expected backpressure with small buffer (32), got count=0")
	}

	// 验收：检查 outbox 深度指标
	currentDepth, maxDepth := exec.GetOutboxMetrics()
	if maxDepth == 0 {
		t.Errorf("Expected maxOutboxDepth > 0, got %d", maxDepth)
	}

	t.Logf("✅ OutboxSize=32: %d results, backpressure=%d, maxDepth=%d, currentDepth=%d",
		totalResults, count, maxDepth, currentDepth)
}

// TestOutboxSize_MediumBuffer_Backpressure 测试中等缓冲（64）背压
func TestOutboxSize_MediumBuffer_Backpressure(t *testing.T) {
	smallEventCh := make(chan model.EngineEvent, 2)
	tradeClient := &ws.TradeClient{}

	// P1-1: 中等 outboxSize = 64
	exec := NewWSExecutor(tradeClient, smallEventCh, 2, 3, 64)
	defer exec.Stop()

	const totalResults = 100
	receivedCount := 0
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalResults; i++ {
			<-smallEventCh
			receivedCount++
			time.Sleep(5 * time.Millisecond)
		}
	}()

	for i := 0; i < totalResults; i++ {
		task := &model.Task{
			TaskID:        fmt.Sprintf("task_%d", i),
			Type:          model.TaskTypePlaceEntry,
			ClientOrderID: fmt.Sprintf("TEST_%d", i),
		}
		exec.sendExecutorResult(task, true, 0, "", nil)
	}

	wg.Wait()

	if receivedCount != totalResults {
		t.Errorf("Expected %d results, got %d", totalResults, receivedCount)
	}

	count, _ := exec.GetBackpressureMetrics()
	_, maxDepth := exec.GetOutboxMetrics()

	t.Logf("✅ OutboxSize=64: %d results, backpressure=%d, maxDepth=%d",
		totalResults, count, maxDepth)
}

// ========== P1-EX-01: WSExecutor Stop 幂等化 ==========

// TestWSExecutor_Stop_Idempotent 验证 Stop 方法幂等性
// 验收点：
// 1. 多次调用 Stop() 不会 panic（sync.Once 保护）
// 2. Stop() 后 resultPump goroutine 正确退出
// 3. 无 goroutine 泄漏（阈值断言）
func TestWSExecutor_Stop_Idempotent(t *testing.T) {
	eventCh := make(chan model.EngineEvent, 10)
	tradeClient := &ws.TradeClient{}

	exec := NewWSExecutor(tradeClient, eventCh, 2, 3, 2048)

	// 记录 Stop 前的 goroutine 数量（基准值）
	baselineGoroutines := countGoroutines()

	// 验收点1：多次调用 Stop() 不 panic
	err1 := exec.Stop() // 第1次
	if err1 != nil {
		t.Errorf("First Stop() failed: %v", err1)
	}

	err2 := exec.Stop() // 第2次
	if err2 != nil {
		t.Errorf("Second Stop() failed: %v", err2)
	}

	err3 := exec.Stop() // 第3次
	if err3 != nil {
		t.Errorf("Third Stop() failed: %v", err3)
	}

	t.Logf("✅ 验收点1通过：多次 Stop() 不 panic")

	// 验收点2：Stop 后 resultPump goroutine 退出（wg.Wait 已返回）
	// 如果 pump 未退出，wg.Wait() 会永久阻塞，导致测试超时
	time.Sleep(100 * time.Millisecond) // 给 pump 足够退出时间

	// 验收点3：无 goroutine 泄漏（阈值断言）
	currentGoroutines := countGoroutines()
	leaked := currentGoroutines - baselineGoroutines

	// 允许一定误差（测试框架本身可能创建少量 goroutine）
	const leakThreshold = 2
	if leaked > leakThreshold {
		t.Errorf("Goroutine leak detected: baseline=%d, current=%d, leaked=%d (threshold=%d)",
			baselineGoroutines, currentGoroutines, leaked, leakThreshold)
	}

	t.Logf("✅ 验收点2/3通过：pump 正确退出，无 goroutine 泄漏（baseline=%d, current=%d, leaked=%d）",
		baselineGoroutines, currentGoroutines, leaked)
}

// TestWSExecutor_Stop_PumpExitVerify 验证 Stop 后 pump 能立即退出
func TestWSExecutor_Stop_PumpExitVerify(t *testing.T) {
	eventCh := make(chan model.EngineEvent, 10)
	tradeClient := &ws.TradeClient{}

	exec := NewWSExecutor(tradeClient, eventCh, 2, 3, 2048)

	// 发送一些事件到 outbox
	for i := 0; i < 10; i++ {
		task := &model.Task{
			TaskID:        fmt.Sprintf("task_%d", i),
			Type:          model.TaskTypePlaceEntry,
			ClientOrderID: fmt.Sprintf("TEST_%d", i),
		}
		exec.sendExecutorResult(task, true, 0, "", nil)
	}

	// 调用 Stop()（应该立即返回，pump 退出）
	startTime := time.Now()
	err := exec.Stop()
	elapsed := time.Since(startTime)

	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}

	// 验收：Stop() 应该在合理时间内返回（pump 退出不阻塞）
	const stopTimeout = 500 * time.Millisecond
	if elapsed > stopTimeout {
		t.Errorf("Stop() took too long: %v (expected < %v)", elapsed, stopTimeout)
	}

	t.Logf("✅ Stop() 在 %v 内返回，pump 正确退出", elapsed)
}

// countGoroutines 统计当前 goroutine 数量
func countGoroutines() int {
	// 等待一小段时间，让可能的短暂 goroutine 结束
	time.Sleep(50 * time.Millisecond)
	return runtime.NumGoroutine()
}
