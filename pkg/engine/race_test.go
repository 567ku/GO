// P0-RACE-02: 并发GetState + 写入竞态测试
// 验收：-race不报错
package engine

import (
	"context"
	"gridbot/pkg/model"
	"sync"
	"testing"
	"time"
)

// TestConsistency_ConcurrentGetState 验证并发GetState + 写入不race
// 验收红线4: 增加一个最小单测：并发 GetState + 写入不 race
func TestConsistency_ConcurrentGetState(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST_RACE",
		Symbol:              "BTCUSDT",
		Side:                model.GridSideLong,
		StepTicks:           100,
		WinMinTicks:         49000000,
		WinMaxTicks:         51000000,
		TPKeepOutsideLevels: 5,
		EntryQtyTicks:       1000,
		PricePrecision:      1,
		QtyPrecision:        3,
		EventChSize:         1000, // 大缓冲避免阻塞
	}

	engine := NewEngine(config)

	// 启动Engine
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	// 等待Engine启动
	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	const numReaders = 10
	const numWrites = 100

	// 启动并发读取goroutines（高频GetState）
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				getCtx, getCancel := context.WithTimeout(ctx, 500*time.Millisecond)
				snapshot, err := engine.GetState(getCtx)
				getCancel()

				if err != nil {
					// context超时或engine停止是正常的
					if err != context.DeadlineExceeded && err.Error() != "engine stopped" {
						t.Errorf("Reader %d: GetState failed: %v", id, err)
					}
					return
				}

				// 读取快照数据（验证不会panic）
				_ = snapshot.Symbol
				_ = snapshot.Market.LastPriceTicks
				_ = len(snapshot.Levels)
				_ = len(snapshot.CLIDIndex)
			}
		}(i)
	}

	// 同时事件循环持续处理tick/update（高频写入）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numWrites; i++ {
			// 发送PriceTick事件（触发状态修改）
			select {
			case engine.GetEventCh() <- model.EngineEvent{
				Type: model.EventTypePriceTick,
				Data: model.PriceTickEvent{
					Symbol:     "BTCUSDT",
					PriceTicks: 50000000 + int64(i*100),
					EventAtMs:  model.NowMs(),
				},
			}:
			case <-ctx.Done():
				return
			}

			// 发送OrderUpdate事件（触发状态修改）
			select {
			case engine.GetEventCh() <- model.EngineEvent{
				Type: model.EventTypeOrderUpdate,
				Data: model.OrderUpdateEvent{
					Symbol:           "BTCUSDT",
					ClientOrderID:    "TEST_RACE:E:1:0",
					OrderID:          int64(10000 + i),
					Status:           "NEW",
					Side:             "BUY",
					PriceTicks:       50000000,
					OrigQtyTicks:     1000,
					ExecutedQtyTicks: 0,
					UpdateAtMs:       model.NowMs(),
				},
			}:
			case <-ctx.Done():
				return
			}

			time.Sleep(5 * time.Millisecond) // 适当延迟避免刷爆eventCh
		}
	}()

	// 等待所有goroutines完成
	wg.Wait()

	t.Log("✅ 并发GetState + 写入测试完成，无race检测到")
}

// TestConsistency_GetState_Timeout 验证GetState超时控制
// 验收红线1: GetState必须可超时、可退出
func TestConsistency_GetState_Timeout(t *testing.T) {
	config := EngineConfig{
		Prefix:              "TEST_TIMEOUT",
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
	engineCtx, engineCancel := context.WithCancel(context.Background())
	defer engineCancel()

	go func() {
		_ = engine.Run(engineCtx)
	}()

	// 等待启动
	time.Sleep(50 * time.Millisecond)

	// 测试1: 正常调用能返回
	normalCtx, normalCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer normalCancel()

	_, err := engine.GetState(normalCtx)
	if err != nil {
		t.Errorf("正常GetState应该成功: %v", err)
	}

	// 测试2: context超时能正确返回错误
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer timeoutCancel()
	time.Sleep(10 * time.Millisecond) // 确保context已超时

	_, err = engine.GetState(timeoutCtx)
	if err != context.DeadlineExceeded {
		t.Errorf("超时GetState应该返回DeadlineExceeded: got=%v", err)
	}

	// 测试3: Engine停止后GetState能退出
	engine.Stop()
	time.Sleep(50 * time.Millisecond)

	stoppedCtx, stoppedCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stoppedCancel()

	_, err = engine.GetState(stoppedCtx)
	if err == nil {
		t.Error("Engine停止后GetState应该返回错误")
	}

	t.Log("✅ GetState超时控制测试通过")
}
