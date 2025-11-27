// Stage 5D: 最小运维指标
// 关键指标: event backlog、task inflight、lease timeout count、freeze count、ws health
package engine

import (
	"encoding/json"
	"fmt"
	"gridbot/pkg/model"
	"sync/atomic"
	"time"
)

// EngineMetrics Engine运维指标（并发安全）
type EngineMetrics struct {
	// 事件处理
	EventBacklog    int64 `json:"eventBacklog"`    // 事件积压数量
	EventsProcessed int64 `json:"eventsProcessed"` // 已处理事件总数
	EventsPerSecond int64 `json:"eventsPerSecond"` // 每秒事件吞吐（近似）
	LastEventAtMs   int64 `json:"lastEventAtMs"`   // 最后事件时间

	// 任务管理
	TaskInflight int64 `json:"taskInflight"` // 飞行中任务数（SUBMITTED状态订单数）

	// Lease超时
	LeaseTimeoutCount int64 `json:"leaseTimeoutCount"` // 累计Lease超时次数

	// Freeze统计
	FreezeCount    int64  `json:"freezeCount"`    // 累计Freeze次数
	LastFreezeAtMs int64  `json:"lastFreezeAtMs"` // 最后Freeze时间
	FreezeReason   string `json:"freezeReason"`   // 最后Freeze原因

	// Reconcile统计
	ReconcileCount    int64 `json:"reconcileCount"`    // 累计Reconcile次数
	LastReconcileAtMs int64 `json:"lastReconcileAtMs"` // 最后Reconcile时间

	// WS健康
	MarketWSHealth string `json:"marketWSHealth"` // CONNECTED/DISCONNECTED
	UDSWSHealth    string `json:"udsWSHealth"`    // CONNECTED/DISCONNECTED
	TradeWSHealth  string `json:"tradeWSHealth"`  // CONNECTED/DISCONNECTED

	// Engine状态
	CurrentMode string `json:"currentMode"` // RUNNING/FREEZE/RECONCILING
	LevelCount  int64  `json:"levelCount"`  // Level总数

	// P1-D: 任务提交失败统计
	TaskSubmitFailureCount int64 `json:"taskSubmitFailureCount"` // 累计任务提交失败次数
	LastSubmitFailureAtMs  int64 `json:"lastSubmitFailureAtMs"`  // 最近一次失败时间
}

// GetMetrics 获取当前运维指标（Stage 5D）
func (e *Engine) GetMetrics() *EngineMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	metrics := &EngineMetrics{
		EventBacklog:           int64(len(e.eventCh)),
		EventsProcessed:        atomic.LoadInt64(&e.eventsProcessed),
		LastEventAtMs:          atomic.LoadInt64(&e.lastEventAtMs),
		LeaseTimeoutCount:      atomic.LoadInt64(&e.leaseTimeoutCount),
		FreezeCount:            atomic.LoadInt64(&e.freezeCount),
		LastFreezeAtMs:         atomic.LoadInt64(&e.lastFreezeAtMs),
		ReconcileCount:         atomic.LoadInt64(&e.reconcileCount),
		LastReconcileAtMs:      atomic.LoadInt64(&e.lastReconcileAtMs),
		TaskSubmitFailureCount: atomic.LoadInt64(&e.taskSubmitFailureCount), // P1-D
		LastSubmitFailureAtMs:  atomic.LoadInt64(&e.lastSubmitFailureAtMs),  // P1-D
		CurrentMode:            string(e.mode),
		LevelCount:             int64(len(e.state.Levels)),
	}

	// 计算任务飞行中数量（SUBMITTED状态的订单）
	inflightCount := int64(0)
	for _, level := range e.state.Levels {
		if level.Entry.State == model.OrderStateSubmitted {
			inflightCount++
		}
		if level.TP.State == model.OrderStateSubmitted {
			inflightCount++
		}
	}
	metrics.TaskInflight = inflightCount

	// 复制Freeze原因（避免并发读写）
	e.metricsLock.RLock()
	metrics.FreezeReason = e.lastFreezeReason
	metrics.MarketWSHealth = e.marketWSHealth
	metrics.UDSWSHealth = e.udsWSHealth
	metrics.TradeWSHealth = e.tradeWSHealth
	e.metricsLock.RUnlock()

	// 计算近似吞吐量（最近1秒）
	now := model.NowMs()
	elapsed := now - metrics.LastEventAtMs
	if elapsed > 0 && elapsed < 1000 {
		// 近1秒内有事件，估算吞吐量
		metrics.EventsPerSecond = 1000 / elapsed
	}

	return metrics
}

// UpdateWSHealth 更新WS健康状态（Stage 5D）
func (e *Engine) UpdateWSHealth(channel model.WSChannel, state model.WSState) {
	e.metricsLock.Lock()
	defer e.metricsLock.Unlock()

	health := string(state) // CONNECTED/DISCONNECTED/RECONNECTED

	switch channel {
	case model.WSChannelMarket:
		e.marketWSHealth = health
	case model.WSChannelUDS:
		e.udsWSHealth = health
	case model.WSChannelTrade:
		e.tradeWSHealth = health
	}
}

// RecordEvent 记录事件处理（Stage 5D）
func (e *Engine) RecordEvent() {
	atomic.AddInt64(&e.eventsProcessed, 1)
	atomic.StoreInt64(&e.lastEventAtMs, model.NowMs())
}

// RecordFreeze 记录Freeze事件（Stage 5D）
func (e *Engine) RecordFreeze(reason string) {
	atomic.AddInt64(&e.freezeCount, 1)
	atomic.StoreInt64(&e.lastFreezeAtMs, model.NowMs())

	e.metricsLock.Lock()
	e.lastFreezeReason = reason
	e.metricsLock.Unlock()
}

// RecordReconcile 记录Reconcile事件（Stage 5D）
func (e *Engine) RecordReconcile() {
	atomic.AddInt64(&e.reconcileCount, 1)
	atomic.StoreInt64(&e.lastReconcileAtMs, model.NowMs())
}

// RecordLeaseTimeout 记录Lease超时（Stage 5D）
func (e *Engine) RecordLeaseTimeout() {
	atomic.AddInt64(&e.leaseTimeoutCount, 1)
}

// RecordTaskSubmitFailure 记录任务提交失败（P1-D）
func (e *Engine) RecordTaskSubmitFailure(taskType model.TaskType, taskID string, err error) {
	atomic.AddInt64(&e.taskSubmitFailureCount, 1)
	atomic.StoreInt64(&e.lastSubmitFailureAtMs, model.NowMs())

	// P1-D: 输出结构化日志（JSON Lines格式）
	fmt.Printf("{\"level\":\"ERROR\",\"component\":\"task_submit\",\"taskType\":\"%s\",\"taskID\":\"%s\",\"error\":\"%s\",\"timestamp\":\"%s\"}\n",
		taskType,
		taskID,
		err.Error(),
		time.Now().Format(time.RFC3339))
}

// LogMetricsSummary 输出结构化日志summary（JSON Lines格式）
func (e *Engine) LogMetricsSummary() {
	metrics := e.GetMetrics()

	// JSON序列化
	jsonBytes, err := json.Marshal(metrics)
	if err != nil {
		fmt.Printf("{\"level\":\"ERROR\",\"msg\":\"metrics marshal failed\",\"error\":\"%s\"}\n", err.Error())
		return
	}

	// 输出JSON Lines格式日志
	fmt.Printf("{\"level\":\"INFO\",\"component\":\"engine_metrics\",\"timestamp\":\"%s\",\"metrics\":%s}\n",
		time.Now().Format(time.RFC3339),
		string(jsonBytes))
}
