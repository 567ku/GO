// P0-E-04: Executor接口定义
package executor

import (
	"context"
	"gridbot/pkg/model"
)

// Executor 任务执行器接口
// 职责：接收Task，转换为API请求，回流ExecutorResultEvent
// 规则：Engine只信ExecutorResultEvent + UDS，不信"调用成功假设"
type Executor interface {
	// Submit 提交任务（异步）
	// 返回：nil表示提交成功，将来通过ExecutorResultEvent回流
	// 注意：返回nil不代表下单成功，只代表请求已发送
	Submit(ctx context.Context, task *model.Task) error

	// Cancel 取消任务（最大努力）
	// 用于Lease超时回收时主动取消INFLIGHT任务
	Cancel(ctx context.Context, taskID string) error

	// Stop 停止执行器
	Stop() error
}

// QueryRequest 查询请求
type QueryRequest struct {
	Symbol        string
	OrderID       int64
	ClientOrderID string
}

// QueryResponse 查询响应
type QueryResponse struct {
	OrderID          int64
	ClientOrderID    string
	Status           string
	PriceTicks       int64
	OrigQtyTicks     int64
	ExecutedQtyTicks int64
	AvgPriceTicks    int64
}
