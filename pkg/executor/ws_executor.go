// P0-E-04: WS Executor实现（Task -> TradeWS -> ExecutorResultEvent）
package executor

import (
	"context"
	"fmt"
	"gridbot/pkg/model"
	"gridbot/pkg/ws"
	"sync"
	"sync/atomic"
	"time"
)

// WSExecutor TradeWS执行器实现
// 职责：
// 1. 将Task转换为TradeWS请求
// 2. 接收TradeWSResponse
// 3. 转换为ExecutorResultEvent并回流到Engine
type WSExecutor struct {
	tradeClient *ws.TradeClient
	eventCh     chan<- model.EngineEvent // 回流ExecutorResultEvent到Engine

	// FAIL-03: 精度注入（Stage 4C）
	pricePrecision int
	qtyPrecision   int

	// pending任务跟踪（用于Cancel）
	pendingMu    sync.RWMutex
	pendingTasks map[string]*model.Task // taskID -> Task

	// P0-1: Outbox + Pump 机制（禁止丢弃 ExecutorResult）
	resultOutbox chan model.EngineEvent // 有界缓冲区（避免直接写 eventCh 导致丢弃）
	ctx          context.Context        // Executor 生命周期
	cancel       context.CancelFunc

	// P0-1: 背压指标
	backpressureCount    int64 // 累计背压次数
	lastBackpressureAtMs int64 // 最近一次背压时间

	// P0-2: 背压日志限频 + 开关
	lastBackpressureLogAt   int64 // 最近一次背压日志时间（限频用）
	EnableExecutorOutboxLog bool  // 背压日志开关（默认false）

	// P1-1: Outbox深度指标
	currentOutboxDepth int64 // 当前 outbox 深度（实时）
	maxOutboxDepth     int64 // 历史峰值

	// P1-EX-01: Stop 幂等化
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewWSExecutor 创建WS执行器
// FAIL-03: 注入精度参数（符合Stage 4C精度注入规范）
// P1-1: 支持配置化 outboxSize（默认2048）
func NewWSExecutor(tradeClient *ws.TradeClient, eventCh chan<- model.EngineEvent, pricePrecision, qtyPrecision, outboxSize int) *WSExecutor {
	// 默认 outboxSize
	if outboxSize <= 0 {
		outboxSize = 2048
	}

	ctx, cancel := context.WithCancel(context.Background())

	exec := &WSExecutor{
		tradeClient:    tradeClient,
		eventCh:        eventCh,
		pricePrecision: pricePrecision,
		qtyPrecision:   qtyPrecision,
		pendingTasks:   make(map[string]*model.Task),
		// P1-1: 创建 outbox（配置化缓冲区大小）
		resultOutbox: make(chan model.EngineEvent, outboxSize),
		ctx:          ctx,
		cancel:       cancel,
		stopCh:       make(chan struct{}),
	}

	// P0-1: 启动 resultPump goroutine
	exec.wg.Add(1)
	go exec.resultPump()

	return exec
}

// Submit 提交任务（P0-E-04: 双保险闭环）
func (e *WSExecutor) Submit(ctx context.Context, task *model.Task) error {
	// 注册到pending map（用于Cancel）
	e.pendingMu.Lock()
	e.pendingTasks[task.TaskID] = task
	e.pendingMu.Unlock()

	// 根据TaskType构造请求
	method, params, err := e.buildRequest(task)
	if err != nil {
		// 构造请求失败，立即回流失败事件
		e.sendExecutorResult(task, false, -1, err.Error(), nil)
		return err
	}

	// 发送TradeWS请求（异步）
	go func() {
		resp, err := e.tradeClient.SendRequest(
			task.TaskID,
			task.ClientOrderID, // 使用ClientOrderID作为reqID
			method,
			params,
		)

		// 无论成功失败，都回流ExecutorResultEvent
		if err != nil {
			e.sendExecutorResult(task, false, -1, err.Error(), nil)
		} else {
			e.sendExecutorResult(task, resp.OK, resp.ErrorCode, resp.ErrorMsg, resp.RawData)
		}

		// 从pending map中移除
		e.pendingMu.Lock()
		delete(e.pendingTasks, task.TaskID)
		e.pendingMu.Unlock()
	}()

	return nil
}

// buildRequest 根据Task构造TradeWS请求
func (e *WSExecutor) buildRequest(task *model.Task) (method string, params map[string]interface{}, err error) {
	params = make(map[string]interface{})

	switch task.Type {
	case model.TaskTypePlaceEntry, model.TaskTypePlaceTP:
		method = "order.place"
		params["symbol"] = task.Symbol
		params["side"] = e.getSide(task)
		params["type"] = "LIMIT"
		params["timeInForce"] = "GTC"
		// FAIL-03: 使用注入的精度（非Stage 4C硬编码）
		params["price"] = model.FormatPriceDecimal(task.PriceTicks, e.pricePrecision)
		params["quantity"] = model.FormatQtyDecimal(task.QtyTicks, e.qtyPrecision)
		params["newClientOrderId"] = task.ClientOrderID
		params["positionSide"] = task.PositionSide
		// P0-3: reduceOnly必须是bool类型（禁止"true"字符串）
		if task.ReduceOnly != nil && *task.ReduceOnly {
			params["reduceOnly"] = true // bool类型，非string
		}

	case model.TaskTypeCancelOrder:
		method = "order.cancel"
		params["symbol"] = task.Symbol
		if task.OrderID > 0 {
			params["orderId"] = task.OrderID
		} else {
			params["origClientOrderId"] = task.ClientOrderID
		}

	case model.TaskTypeQueryOrder:
		method = "order.status"
		params["symbol"] = task.Symbol
		if task.OrderID > 0 {
			params["orderId"] = task.OrderID
		} else {
			params["origClientOrderId"] = task.ClientOrderID
		}

	default:
		return "", nil, fmt.Errorf("unsupported task type: %s", task.Type)
	}

	return method, params, nil
}

// getSide 获取订单方向
func (e *WSExecutor) getSide(task *model.Task) string {
	// Entry: LONG用BUY，SHORT用SELL
	// TP: LONG用SELL，SHORT用BUY（平仓）
	if task.Purpose == model.OrderPurposeEntry {
		// Entry订单
		if task.PositionSide == "LONG" {
			return "BUY"
		}
		return "SELL"
	}

	// TP订单（平仓）
	if task.PositionSide == "LONG" {
		return "SELL"
	}
	return "BUY"
}

// sendExecutorResult 发送ExecutorResultEvent到outbox（P0-1: 禁止丢弃）
func (e *WSExecutor) sendExecutorResult(task *model.Task, ok bool, errorCode int, errorMsg string, rawData map[string]interface{}) {
	event := model.EngineEvent{
		Type: model.EventTypeExecutorResult,
		Data: model.ExecutorResultEvent{
			TaskID:         task.TaskID,
			TaskType:       task.Type,
			OK:             ok,
			ReqID:          task.ClientOrderID,
			ErrorCode:      errorCode,
			ErrorMsg:       errorMsg,
			AtMs:           model.NowMs(),
			ExchangeTimeMs: 0,   // TODO: 从rawData提取
			ParsedOrder:    nil, // TODO: 从rawData解析
		},
	}

	// P0-1: 写入 outbox（带背压检测，禁止丢弃）
	select {
	case e.resultOutbox <- event:
		// 正常入队
		// P1-1: 更新 outbox 深度指标
		e.updateOutboxDepth(len(e.resultOutbox))
	case <-e.ctx.Done():
		// Executor 关闭中，允许丢弃（shutdown 丢弃记录日志）
		e.recordShutdownDrop(task.TaskID, task.ClientOrderID, task.Type)
	default:
		// outbox 满，记录背压，然后阻塞等待（不丢弃）
		e.recordBackpressure(task.TaskID, task.ClientOrderID, task.Type, len(e.resultOutbox))
		select {
		case e.resultOutbox <- event:
			// 阻塞后成功入队
			e.updateOutboxDepth(len(e.resultOutbox))
		case <-e.ctx.Done():
			// 关闭期间丢弃
			e.recordShutdownDrop(task.TaskID, task.ClientOrderID, task.Type)
		}
	}
}

// Cancel 取消任务（最大努力）
func (e *WSExecutor) Cancel(ctx context.Context, taskID string) error {
	e.pendingMu.RLock()
	task, ok := e.pendingTasks[taskID]
	e.pendingMu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// 构造取消任务
	cancelTask := &model.Task{
		TaskID:        taskID + ":cancel",
		Type:          model.TaskTypeCancelOrder,
		Symbol:        task.Symbol,
		LevelID:       task.LevelID,
		Purpose:       task.Purpose,
		ClientOrderID: task.ClientOrderID,
		OrderID:       0, // 使用ClientOrderID取消
	}

	// 提交取消任务
	return e.Submit(ctx, cancelTask)
}

// Stop 停止执行器（P1-EX-01: 幂等化，多次调用安全）
func (e *WSExecutor) Stop() error {
	// P1-EX-01: 使用 sync.Once 保护 close(stopCh) / cancel，防止panic
	e.stopOnce.Do(func() {
		e.cancel()      // 取消 context
		close(e.stopCh) // 关闭 stopCh（只会执行一次）
	})

	// wait 放在 Once 外部，允许多次调用 Stop 都能等待 pump 退出
	e.wg.Wait()
	return nil
}

// ========== P0-1: Outbox + Pump 机制 ==========

// resultPump 从 outbox 读取并转发到 Engine eventCh（背压不丢弃）
func (e *WSExecutor) resultPump() {
	defer e.wg.Done()

	for {
		select {
		case event := <-e.resultOutbox:
			// 从 outbox 读取到事件，转发到 Engine eventCh（阻塞式）
			select {
			case e.eventCh <- event:
				// 成功转发
			case <-e.ctx.Done():
				// Executor 关闭，退出 pump
				return
			case <-e.stopCh:
				// P1-2: stopCh 一致性，保持 Stop() 行为一致
				return
			}

		case <-e.ctx.Done():
			// Executor 关闭，退出pump
			return

		case <-e.stopCh:
			// P1-2: stopCh 一致性，保持 Stop() 行为一致
			return
		}
	}
}

// recordBackpressure 记录背压事件（P0-1 + P0-2: 限频日志）
func (e *WSExecutor) recordBackpressure(taskID, clid string, taskType model.TaskType, outboxLen int) {
	// P0-1: 更新指标
	atomic.AddInt64(&e.backpressureCount, 1)
	atomic.StoreInt64(&e.lastBackpressureAtMs, model.NowMs())

	// P0-2: 限频日志（1s内最多1条）
	if !e.EnableExecutorOutboxLog {
		return // 开关关闭，不输出日志
	}

	nowMs := model.NowMs()
	lastLogMs := atomic.LoadInt64(&e.lastBackpressureLogAt)

	// 限频：1s内只输出一次
	if nowMs-lastLogMs < 1000 {
		return // 距离上次日志不足1s，跳过
	}

	// CAS更新日志时间（避免并发多次输出）
	if atomic.CompareAndSwapInt64(&e.lastBackpressureLogAt, lastLogMs, nowMs) {
		// P1-1: 结构化日志
		fmt.Printf("{\"level\":\"WARN\",\"component\":\"executor_outbox\",\"event\":\"backpressure\",\"taskID\":\"%s\",\"clid\":\"%s\",\"taskType\":\"%s\",\"outboxLen\":%d,\"timestamp\":\"%s\"}\n",
			taskID, clid, taskType, outboxLen, time.Now().Format(time.RFC3339))
	}
}

// recordShutdownDrop 记录 shutdown 期间丢弃的事件（P1-1）
func (e *WSExecutor) recordShutdownDrop(taskID, clid string, taskType model.TaskType) {
	// P1-1: shutdown 丢弃记录日志
	fmt.Printf("{\"level\":\"WARN\",\"component\":\"executor_outbox\",\"event\":\"shutdown_drop\",\"taskID\":\"%s\",\"clid\":\"%s\",\"taskType\":\"%s\",\"timestamp\":\"%s\"}\n",
		taskID, clid, taskType, time.Now().Format(time.RFC3339))
}

// GetBackpressureMetrics 获取背压指标（P0-1）
func (e *WSExecutor) GetBackpressureMetrics() (count int64, lastAtMs int64) {
	return atomic.LoadInt64(&e.backpressureCount), atomic.LoadInt64(&e.lastBackpressureAtMs)
}

// updateOutboxDepth 更新 outbox 深度指标（P1-1）
func (e *WSExecutor) updateOutboxDepth(currentDepth int) {
	depth := int64(currentDepth)
	atomic.StoreInt64(&e.currentOutboxDepth, depth)

	// 更新峰值
	for {
		oldMax := atomic.LoadInt64(&e.maxOutboxDepth)
		if depth <= oldMax {
			break // 未超过峰值
		}
		if atomic.CompareAndSwapInt64(&e.maxOutboxDepth, oldMax, depth) {
			break // 成功更新峰值
		}
	}
}

// GetOutboxMetrics 获取 outbox 深度指标（P1-1）
func (e *WSExecutor) GetOutboxMetrics() (currentDepth, maxDepth int64) {
	return atomic.LoadInt64(&e.currentOutboxDepth), atomic.LoadInt64(&e.maxOutboxDepth)
}
