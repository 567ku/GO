// Stage 5A: 真实Reconcile Query
// 实现QueryOpenOrders真实查询，替代空快照
package engine

import (
	"context"
	"errors"
	"fmt"
	"gridbot/pkg/model"
	"time"
)

// QueryOpenOrdersRequest 查询开仓订单请求
type QueryOpenOrdersRequest struct {
	Symbol string
	Prefix string // GridPrefix，只查询自己的订单
}

// QueryOpenOrdersResponse 查询开仓订单响应
type QueryOpenOrdersResponse struct {
	Orders    []model.ParsedOrderUpdate
	Timestamp int64
	Error     error
}

// ExecutorQueryInterface Executor查询接口（用于依赖注入）
type ExecutorQueryInterface interface {
	QueryOpenOrders(ctx context.Context, req QueryOpenOrdersRequest) (*QueryOpenOrdersResponse, error)
}

// ReconcileQueryConfig Reconcile查询配置
type ReconcileQueryConfig struct {
	TimeoutMs    int // 查询超时（毫秒，默认30000）
	MaxAttempts  int // 最大尝试次数（默认3），totalAttempts=MaxAttempts
	RetryDelayMs int // 重试延迟（毫秒，默认1000）
}

// ReconcileQueryResult Reconcile查询结果
type ReconcileQueryResult struct {
	OpenOrders    []model.ParsedOrderUpdate
	TotalCount    int
	QueryDuration time.Duration
	Success       bool
	ErrorMsg      string
	// FAIL-06: 增加IsTimeout标志，替代字符串判断
	IsTimeout bool
}

// ExecuteReconcileQuery 执行Reconcile查询（带超时、重试）
// queryExecutor: 查询执行器接口（依赖注入）
// P0-ENG-46: 配置从外部传入，不使用内部默认值
func ExecuteReconcileQuery(ctx context.Context, queryExecutor ExecutorQueryInterface, symbol, prefix string, config ReconcileQueryConfig) (*ReconcileQueryResult, error) {
	// 检查queryExecutor是否有效
	if queryExecutor == nil {
		return nil, fmt.Errorf("queryExecutor is nil")
	}

	// 创建带超时的context
	timeout := time.Duration(config.TimeoutMs) * time.Millisecond
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()
	var lastErr error

	// P0-ENG-47: 重试计数语义统一 - MaxAttempts表示总尝试次数
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		if attempt > 1 {
			// 等待重试延迟
			retryDelay := time.Duration(config.RetryDelayMs) * time.Millisecond
			select {
			case <-queryCtx.Done():
				// FAIL-06: 超时，明确标记IsTimeout
				return &ReconcileQueryResult{
					Success:       false,
					ErrorMsg:      fmt.Sprintf("query timeout after %d attempts", attempt-1),
					QueryDuration: time.Since(startTime),
					IsTimeout:     true, // 明确标记超时
				}, queryCtx.Err()
			case <-time.After(retryDelay):
				// 继续重试
			}
		}

		// 执行查询
		req := QueryOpenOrdersRequest{
			Symbol: symbol,
			Prefix: prefix,
		}

		resp, err := queryExecutor.QueryOpenOrders(queryCtx, req)
		if err != nil {
			lastErr = err
			// FAIL-06: 检测是否超时错误
			if queryCtx.Err() == context.DeadlineExceeded {
				// 超时，立即返回
				return &ReconcileQueryResult{
					Success:       false,
					ErrorMsg:      fmt.Sprintf("query timeout after %d attempts", attempt),
					QueryDuration: time.Since(startTime),
					IsTimeout:     true,
				}, err
			}
			// 记录重试日志
			// TODO: 添加结构化日志
			continue
		}

		// 查询成功
		return &ReconcileQueryResult{
			OpenOrders:    resp.Orders,
			TotalCount:    len(resp.Orders),
			QueryDuration: time.Since(startTime),
			Success:       true,
		}, nil
	}

	// 所有重试均失败
	// P1-1: 最终失败路径也必须判断IsTimeout（基于lastErr）
	isTimeout := false
	if lastErr != nil {
		if errors.Is(lastErr, context.DeadlineExceeded) {
			isTimeout = true
		}
	}

	return &ReconcileQueryResult{
		Success:       false,
		ErrorMsg:      fmt.Sprintf("query failed after %d attempts: %v", config.MaxAttempts, lastErr),
		QueryDuration: time.Since(startTime),
		IsTimeout:     isTimeout, // P1-1: 最终路径设置IsTimeout
	}, lastErr
}

// FilterOwnOrders 过滤属于自己前缀的订单
// 防止污染其他网格的订单
func FilterOwnOrders(orders []model.ParsedOrderUpdate, prefix string) []model.ParsedOrderUpdate {
	var filtered []model.ParsedOrderUpdate

	for _, order := range orders {
		// 检查CLID前缀
		_, _, _, err := ParseCLID(order.ClientOrderID, prefix)
		if err != nil {
			// 前缀不匹配或格式错误，跳过
			continue
		}

		// 前缀匹配，保留
		filtered = append(filtered, order)
	}

	return filtered
}

// ValidateReconcileQueryResult 验证查询结果有效性
func ValidateReconcileQueryResult(result *ReconcileQueryResult) error {
	if result == nil {
		return fmt.Errorf("query result is nil")
	}

	if !result.Success {
		return fmt.Errorf("query failed: %s", result.ErrorMsg)
	}

	// 检查订单数量合理性（防止异常大量订单）
	if result.TotalCount > 1000 {
		return fmt.Errorf("too many orders returned: %d (limit: 1000)", result.TotalCount)
	}

	// 检查查询耗时（防止超时）
	if result.QueryDuration > 60*time.Second {
		return fmt.Errorf("query duration too long: %v", result.QueryDuration)
	}

	return nil
}

// ReconcileQueryMetrics Reconcile查询指标（用于可观测性）
type ReconcileQueryMetrics struct {
	TotalQueries    int64
	SuccessQueries  int64
	FailedQueries   int64
	TimeoutQueries  int64
	AverageDuration time.Duration
	LastQueryAt     int64
}

// UpdateMetrics 更新Reconcile查询指标（纯函数）
func (m *ReconcileQueryMetrics) UpdateMetrics(result *ReconcileQueryResult) {
	m.TotalQueries++
	if result.Success {
		m.SuccessQueries++
	} else {
		m.FailedQueries++
		// FAIL-06: 使用IsTimeout标志位判断超时（非字符串匹配）
		if result.IsTimeout {
			m.TimeoutQueries++
		}
	}
	m.LastQueryAt = model.NowMs()
	// TODO: 计算平均时长
}
