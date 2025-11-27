// P0-RET-03: 重连后reconcile闭环落地
// WS RECONNECTED后必须跑一次reconcile流程，最终回到RUNNING
package engine

import (
	"context"
	"fmt"
	"gridbot/pkg/model"
)

// ReconcilePhase reconcile阶段枚举
type ReconcilePhase int

const (
	ReconcilePhaseIdle ReconcilePhase = iota
	ReconcilePhaseQueryOpenOrders
	ReconcilePhaseApplySnapshot
	ReconcilePhaseGapScan
	ReconcilePhaseComplete
)

// ReconcileState reconcile状态
type ReconcileState struct {
	Phase         ReconcilePhase
	StartAtMs     int64
	OpenOrders    []model.ParsedOrderUpdate // 从交易所拉取的开仓订单
	CompletedAtMs int64
}

// StartReconcile 启动reconcile流程
// 在WS RECONNECTED后调用
// Stage 5A: 真实Reconcile Query
func (e *Engine) StartReconcile(ctx context.Context, queryExecutor ExecutorQueryInterface) error {
	// 检查是否已经在reconcile中
	e.mu.Lock()
	if e.reconcileInFlight {
		e.mu.Unlock()
		return fmt.Errorf("reconcile already in flight")
	}
	e.reconcileInFlight = true
	e.mu.Unlock()

	// 1. 进入RECONCILING状态（通过reducer）
	var effect ReducerEffect
	e.state, effect = ApplyReconcileStart(e.state)
	_ = effect // 当前暂不处理effect

	e.mu.Lock()
	e.mode = model.EngineModeReconciling
	e.mu.Unlock()

	// 2. 执行真实查询（替代空快照）
	// P0-ENG-46: 从配置注入查询参数
	queryConfig := ReconcileQueryConfig{
		TimeoutMs:    e.config.ReconcileQueryTimeoutMs,
		MaxAttempts:  e.config.ReconcileMaxAttempts,
		RetryDelayMs: e.config.ReconcileRetryDelayMs,
	}
	result, err := ExecuteReconcileQuery(ctx, queryExecutor, e.config.Symbol, e.config.Prefix, queryConfig)
	if err != nil {
		// 查询失败，降级为空快照
		// fmt.Printf("[DEBUG] ExecuteReconcileQuery failed: %v, degrading to empty snapshot\n", err)
		e.applyOpenOrdersSnapshot([]model.ParsedOrderUpdate{})
		// Note: applyOpenOrdersSnapshot 会调用 runReconcileGapScan → completeReconcile
		return err
	}

	// 3. 验证查询结果
	if err := ValidateReconcileQueryResult(result); err != nil {
		// 验证失败，降级为空快照
		e.applyOpenOrdersSnapshot([]model.ParsedOrderUpdate{})
		return err
	}

	// 4. 过滤自己的订单（防止污染其他网格）
	filteredOrders := FilterOwnOrders(result.OpenOrders, e.config.Prefix)

	// 5. 应用快照
	e.applyOpenOrdersSnapshot(filteredOrders)

	return nil
}

// applyOpenOrdersSnapshot 应用开仓订单快照到本地状态（通过reducer）
// P0-ENG-41: 不再直接修改state，只调用reducer函数
func (e *Engine) applyOpenOrdersSnapshot(orders []model.ParsedOrderUpdate) {
	// 调用reducer应用快照
	var effect ReducerEffect
	e.state, effect = ApplyOpenOrdersSnapshot(e.state, orders, e.config.Prefix)

	// FAIL-05: 将CANCEL任务传递给runReconcileGapScan统一提交
	// 应用完快照后，执行GapScan并合并所有任务
	e.runReconcileGapScan(effect.Tasks)
}

// runReconcileGapScan 在reconcile阶段运行GapScan
// FAIL-05: 先切回RUNNING，再执行GapScan并提交任务（符合闸门要求）
// cancelTasks: 从 ApplyOpenOrdersSnapshot 生成的CANCEL任务（cycle mismatch）
func (e *Engine) runReconcileGapScan(cancelTasks []model.Task) {
	// 关键：先切回RUNNING（写闸门打开）
	e.completeReconcile()

	// 然后执行GapScan（此时已是RUNNING，可以生成写任务）
	gapScanTasks := GapScan(e.state)

	// FAIL-01/05: 合并CANCEL任务和GapScan任务，统一提交（在RUNNING阶段）
	allTasks := append(cancelTasks, gapScanTasks...)

	if e.taskExecutor != nil && len(allTasks) > 0 {
		// P0-2: 使用Engine.ctx绑定生命周期（禁止Background）
		for i := range allTasks {
			// P0-1: 根据TaskType分配正确的Lease（禁止统一LeasePlaceMs）
			e.assignLease(&allTasks[i])

			// 提交任务给执行器
			if err := e.taskExecutor.Submit(e.ctx, &allTasks[i]); err != nil {
				// P1-D: 提交失败，记录metrics+log（不静默吞掉）
				e.RecordTaskSubmitFailure(allTasks[i].Type, allTasks[i].TaskID, err)
				// 由Lease超时机制自动重试
			}
		}
	}
}

// completeReconcile 完成reconcile流程，回到RUNNING（通过reducer）
// P0-ENG-41: 不再直接修改state，只调用reducer函数
func (e *Engine) completeReconcile() {
	// P0-E-02-A: 切回RUNNING（打开写闸门）
	var effect ReducerEffect
	e.state, effect = ApplyReconcileComplete(e.state)
	_ = effect // 当前暂不处理effect

	// 同步更新Engine的mode字段
	e.mu.Lock()
	e.mode = model.EngineModeRunning
	e.reconcileInFlight = false // 清零标记
	e.mu.Unlock()

	// TODO: 记录reconcile完成日志/时间戳
}
