// P0-RET-02: Reducer-only写约束
// 所有状态写入必须通过reducer纯函数进行
package engine

import (
	"gridbot/pkg/model"
)

// ReducerEffect reducer执行后的副作用（暂时保留，阶段3最小闭环不使用）
type ReducerEffect struct {
	// 可选：需要生成的Task列表
	Tasks []model.Task
	// 可选：需要发送的告警
	Alerts []string
}

// ApplyPriceTick 应用PriceTick事件到状态
// 纯函数：不修改输入state，返回新状态
func ApplyPriceTick(state *model.GridStateSnapshot, ev model.PriceTickEvent) (*model.GridStateSnapshot, ReducerEffect) {
	// 更新市场状态
	state.Market.LastPriceTicks = ev.PriceTicks
	state.Market.LastPriceAtMs = ev.EventAtMs

	return state, ReducerEffect{}
}

// ApplyOrderUpdate 应用OrderUpdate事件到状态（UDS确认）
// 这是双保险闭环的关键：只有UDS确认才真正更新OrderSlot状态
func ApplyOrderUpdate(state *model.GridStateSnapshot, ev model.OrderUpdateEvent, prefix string) (*model.GridStateSnapshot, ReducerEffect) {
	// P0-E-03: CLID映射
	purpose, levelID, cycle, err := ParseCLID(ev.ClientOrderID, prefix)
	if err != nil {
		// 未知前缀/格式错误，忽略
		return state, ReducerEffect{}
	}

	// 查找level
	level := FindLevelByID(state, levelID)
	if level == nil {
		// Level不存在，忽略（可能是旧订单）
		return state, ReducerEffect{}
	}

	// 检查cycle是否匹配
	if level.Cycle != cycle {
		// Cycle不匹配，忽略（可能是旧订单）
		return state, ReducerEffect{}
	}

	// 获取订单槽
	slot := GetOrderSlot(level, purpose)
	if slot == nil {
		return state, ReducerEffect{}
	}

	// 更新OrderSlot状态
	slot.OrderID = ev.OrderID
	slot.PriceTicks = ev.PriceTicks
	slot.OrigQtyTicks = ev.OrigQtyTicks
	slot.ExecutedQtyTicks = ev.ExecutedQtyTicks
	slot.AvgPriceTicks = ev.AvgPriceTicks
	slot.UpdateAtMs = ev.UpdateAtMs
	slot.LastExchangeStatus = ev.Status
	slot.LastSource = model.EventSourceUDS

	// 映射交易所状态到本地状态
	previousState := slot.State // 记录旧状态，用于检测Entry FILLED
	switch ev.Status {
	case "NEW":
		slot.State = model.OrderStateOpen
	case "PARTIALLY_FILLED":
		slot.State = model.OrderStatePartial
	case "FILLED":
		slot.State = model.OrderStateFilled
	case "CANCELED":
		slot.State = model.OrderStateCanceled
	case "REJECTED":
		slot.State = model.OrderStateRejected
	case "EXPIRED":
		slot.State = model.OrderStateExpired
	default:
		// 未知状态，保持原状态
	}

	// FAIL-04: Entry FILLED时递增Cycle（保证下一轮网格幂等性）
	// 必须同时满足：1) 是Entry订单 2) 状态切换到FILLED 3) 之前不是FILLED
	if purpose == model.OrderPurposeEntry &&
		slot.State == model.OrderStateFilled &&
		previousState != model.OrderStateFilled {
		// 调用Cycle递增reducer（符合reducer-only约束）
		state, _ = ApplyLevelCycleIncrement(state, levelID)
	}

	// 更新CLID索引
	state.CLIDIndex[ev.ClientOrderID] = ev.OrderID

	return state, ReducerEffect{}
}

// ApplyTradeUpdate 应用TradeUpdate事件到状态
func ApplyTradeUpdate(state *model.GridStateSnapshot, ev model.TradeUpdateEvent) (*model.GridStateSnapshot, ReducerEffect) {
	// TODO: 实现成交更新逻辑
	// 当前阶段3最小闭环不需要
	return state, ReducerEffect{}
}

// ApplyExecutorResult 应用ExecutorResult事件到状态（双保险闭环第一步）
// 规则：ExecutorResult只做状态推进，不等于最终确认（必须等UDS）
func ApplyExecutorResult(state *model.GridStateSnapshot, ev model.ExecutorResultEvent, prefix string) (*model.GridStateSnapshot, ReducerEffect) {
	if !ev.OK {
		// P0-RET-04: 执行失败，根据ErrorCode分类处理
		errorAction := classifyErrorCode(ev.ErrorCode)

		// 根据TaskID解析CLID（如果可能）
		// 当前TaskID可能不包含CLID，先跳过
		// TODO: 完善TaskID -> CLID的反解析

		// 根据错误动作处理
		switch errorAction {
		case ErrorActionReject:
			// REJECT类错误：不重试，标记为REJECTED
			// TODO: 找到对应的OrderSlot，更新为REJECTED状态
			// 当前简化：只记录错误，等待UDS确认
		case ErrorActionFreeze:
			// 严重错误（余额不足、API权限等）：触发FREEZE
			state.EngineMode = model.EngineModeFreeze
		case ErrorActionRetry:
			// 网络类错误：可重试，保持原状态
			// 由Lease超时机制自动重试
		default:
			// 未知错误：保守处理，保持原状态
		}

		return state, ReducerEffect{}
	}

	// 执行成功：从ParsedOrder中提取状态信息
	if ev.ParsedOrder == nil {
		// 没有解析到订单，可能是CANCEL操作成功
		return state, ReducerEffect{}
	}

	// P0-E-03: CLID映射
	purpose, levelID, cycle, err := ParseCLID(ev.ParsedOrder.ClientOrderID, prefix)
	if err != nil {
		return state, ReducerEffect{}
	}

	// 查找level
	level := FindLevelByID(state, levelID)
	if level == nil {
		return state, ReducerEffect{}
	}

	// 检查cycle
	if level.Cycle != cycle {
		return state, ReducerEffect{}
	}

	// 获取订单槽
	slot := GetOrderSlot(level, purpose)
	if slot == nil {
		return state, ReducerEffect{}
	}

	// 更新OrderSlot状态（ExecutorResult视角）
	slot.OrderID = ev.ParsedOrder.OrderID
	slot.PriceTicks = ev.ParsedOrder.PriceTicks
	slot.OrigQtyTicks = ev.ParsedOrder.OrigQtyTicks
	slot.ExecutedQtyTicks = ev.ParsedOrder.ExecutedQtyTicks
	slot.AvgPriceTicks = ev.ParsedOrder.AvgPriceTicks
	slot.UpdateAtMs = model.NowMs() // ExecutorResult没有UpdateAtMs，使用当前时间
	slot.LastExchangeStatus = ev.ParsedOrder.Status
	slot.LastSource = model.EventSourceResp // ExecutorResultEvent的来源是请求响应链路（RESP）

	// 映射状态
	switch ev.ParsedOrder.Status {
	case "NEW":
		slot.State = model.OrderStateOpen
	case "PARTIALLY_FILLED":
		slot.State = model.OrderStatePartial
	case "FILLED":
		slot.State = model.OrderStateFilled
	case "CANCELED":
		slot.State = model.OrderStateCanceled
	case "REJECTED":
		slot.State = model.OrderStateRejected
	case "EXPIRED":
		slot.State = model.OrderStateExpired
	default:
		// 保持原状态
	}

	return state, ReducerEffect{}
}

// ApplyWSStateChange 应用WS状态变更（Freeze/Unfreeze）
func ApplyWSStateChange(state *model.GridStateSnapshot, ev model.WSStateEvent) (*model.GridStateSnapshot, ReducerEffect) {
	// 私域WS（UDS/TRADE）断线 → 立即FREEZE
	if ev.Channel == model.WSChannelUDS || ev.Channel == model.WSChannelTrade {
		if ev.State == model.WSStateDisconnected {
			state.EngineMode = model.EngineModeFreeze
		}

		// 重连完成 → 进入RECONCILING
		if ev.State == model.WSStateReconnected {
			state.EngineMode = model.EngineModeReconciling
		}
	}

	return state, ReducerEffect{}
}

// ApplyLeaseScanResult 应用Lease扫描结果（回收超时任务）
// P0-RET-05: 实现Lease回收逻辑
func ApplyLeaseScanResult(state *model.GridStateSnapshot, expiredLevels []int, maxAttempt int) (*model.GridStateSnapshot, ReducerEffect) {
	needFreeze := false

	for _, levelID := range expiredLevels {
		level := FindLevelByID(state, levelID)
		if level == nil {
			continue
		}

		// 检查Entry槽
		if level.Entry.State == model.OrderStateSubmitted {
			level.Entry.Attempt++

			if level.Entry.Attempt > maxAttempt {
				// 超过重试上限 → FREEZE
				needFreeze = true
			} else {
				// 回退到NONE，等待下一轮GapScan重试
				level.Entry.State = model.OrderStateNone
				level.Entry.OrderID = 0
				// Attempt计数器保留，下次继续递增
			}
		}

		// 检查TP槽
		if level.TP.State == model.OrderStateSubmitted {
			level.TP.Attempt++

			if level.TP.Attempt > maxAttempt {
				// 超过重试上限 → FREEZE
				needFreeze = true
			} else {
				// 回退到NONE，等待下一轮GapScan重试
				level.TP.State = model.OrderStateNone
				level.TP.OrderID = 0
			}
		}
	}

	// 如果有任何任务超过重试上限，触发FREEZE
	if needFreeze {
		state.EngineMode = model.EngineModeFreeze
	}

	return state, ReducerEffect{}
}

// ========== P0-RET-04: 错误码分类 ==========

// ErrorAction 错误动作类型
type ErrorAction int

const (
	// ErrorActionRetry 可重试错误（网络问题、临时性错误）
	ErrorActionRetry ErrorAction = iota
	// ErrorActionReject 拒绝类错误（不重试，标记为REJECTED）
	ErrorActionReject
	// ErrorActionFreeze 严重错误（触发FREEZE）
	ErrorActionFreeze
)

// classifyErrorCode 分类错误码（根据币安U本位API错误码规范）
// 参考：U本位合约错误代码分类及详解.md
func classifyErrorCode(code int) ErrorAction {
	switch {
	// REJECT类错误：订单被拒绝，不可重试
	case code == -2010: // NEW_ORDER_REJECTED - 订单被拒绝
		return ErrorActionReject
	case code == -2011: // CANCEL_REJECTED - 撤单被拒绝
		return ErrorActionReject
	case code == -4045: // 价格低于最小价
		return ErrorActionReject
	case code == -4046: // 价格高于最大价
		return ErrorActionReject
	case code == -4164: // Order's notional must be no smaller than
		return ErrorActionReject
	case code == -4003: // 数量低于最小数量
		return ErrorActionReject
	case code == -4162: // Reduce-only order rejected
		return ErrorActionReject

	// FREEZE类错误：严重问题，需要人工干预
	case code == -2019: // 余额不足
		return ErrorActionFreeze
	case code == -1021: // Timestamp out of range
		return ErrorActionFreeze
	case code == -2015: // Invalid API-key, IP, or permissions
		return ErrorActionFreeze
	case code == -1003: // Too many requests
		return ErrorActionFreeze
	case code == -4131: // Account restricted
		return ErrorActionFreeze

	// RETRY类错误：临时性问题，可重试
	case code == -1001: // Internal error
		return ErrorActionRetry
	case code == -1006: // Unexpected response
		return ErrorActionRetry
	case code == -1007: // Timeout waiting for response
		return ErrorActionRetry
	case code == 0: // 无错误码（网络问题）
		return ErrorActionRetry

	default:
		// 未知错误：保守处理，允许重试
		return ErrorActionRetry
	}
}

// ========== P0-ENG-41: Reconcile全流程reducer化 ==========

// ApplyReconcileStart reducer: 进入RECONCILING状态
func ApplyReconcileStart(state *model.GridStateSnapshot) (*model.GridStateSnapshot, ReducerEffect) {
	state.EngineMode = model.EngineModeReconciling
	return state, ReducerEffect{}
}

// ApplyReconcileComplete reducer: 完成RECONCILING，回到RUNNING
func ApplyReconcileComplete(state *model.GridStateSnapshot) (*model.GridStateSnapshot, ReducerEffect) {
	state.EngineMode = model.EngineModeRunning
	return state, ReducerEffect{}
}

// ApplyOpenOrdersSnapshot reducer: 应用对账快照到本地状态
// 包含：Level/OrderSlot映射、CLIDIndex维护、LastSource标记、cycle mismatch处理
func ApplyOpenOrdersSnapshot(state *model.GridStateSnapshot, orders []model.ParsedOrderUpdate, prefix string) (*model.GridStateSnapshot, ReducerEffect) {
	effect := ReducerEffect{
		Tasks: make([]model.Task, 0),
	}

	// 遍历所有交易所返回的订单
	for _, order := range orders {
		// P0-E-03: CLID映射
		purpose, levelID, cycle, err := ParseCLID(order.ClientOrderID, prefix)
		if err != nil {
			// 未知前缀/格式错误，忽略
			continue
		}

		// 查找或创建level（reducer内部helper）
		level := findOrCreateLevelInState(state, levelID, order.PriceTicks)
		if level.Cycle == 0 {
			// 新创建的level，初始化cycle
			level.Cycle = cycle
		}

		// P0-ENG-45: 检查cycle是否匹配
		if level.Cycle != cycle {
			// Cycle不匹配，说明是旧订单，生成CANCEL任务
			cancelTask := generateCancelTaskForReconcile(state, levelID, purpose, &order)
			effect.Tasks = append(effect.Tasks, cancelTask)
			continue
		}

		// 获取订单槽
		slot := GetOrderSlot(level, purpose)
		if slot == nil {
			continue
		}

		// 应用快照到OrderSlot（终态优先）
		applyOrderSnapshotToSlot(slot, &order)

		// P0-ENG-42: 维护CLIDIndex
		state.CLIDIndex[order.ClientOrderID] = order.OrderID
	}

	return state, effect
}

// findOrCreateLevelInState reducer内部helper：查找或创建level
// 这是reducer内部函数，只能在reducer中调用，确保状态修改的唯一入口
func findOrCreateLevelInState(state *model.GridStateSnapshot, levelID int, priceTicks int64) *model.LevelState {
	// 先查找
	level := FindLevelByID(state, levelID)
	if level != nil {
		return level
	}

	// 创建新level
	newLevel := model.LevelState{
		LevelID:    levelID,
		PriceTicks: priceTicks,
		Cycle:      0,
		Entry: model.OrderSlot{
			Purpose: model.OrderPurposeEntry,
			State:   model.OrderStateNone,
		},
		TP: model.OrderSlot{
			Purpose: model.OrderPurposeTP,
			State:   model.OrderStateNone,
		},
	}

	// 添加到state（只能在reducer中调用）
	state.Levels = append(state.Levels, newLevel)

	// 返回新创建的level的指针
	return &state.Levels[len(state.Levels)-1]
}

// applyOrderSnapshotToSlot reducer内部helper：应用单个订单快照到OrderSlot
// 规则：
// - 本地NONE：直接覆盖
// - 本地INFLIGHT：交易所终态优先
// - 本地终态：保持不变（除非交易所状态更新）
func applyOrderSnapshotToSlot(slot *model.OrderSlot, order *model.ParsedOrderUpdate) {
	// 更新基础字段
	slot.OrderID = order.OrderID
	slot.ClientOrderID = order.ClientOrderID
	slot.PriceTicks = order.PriceTicks
	slot.OrigQtyTicks = order.OrigQtyTicks
	slot.ExecutedQtyTicks = order.ExecutedQtyTicks
	slot.AvgPriceTicks = order.AvgPriceTicks
	slot.UpdateAtMs = model.NowMs()
	slot.LastExchangeStatus = order.Status
	slot.LastSource = model.EventSourceQuery // Reconcile的快照来源是Engine主动Query

	// 映射状态（终态优先）
	switch order.Status {
	case "NEW":
		// 交易所显示NEW，覆盖本地状态
		slot.State = model.OrderStateOpen
	case "PARTIALLY_FILLED":
		slot.State = model.OrderStatePartial
	case "FILLED":
		// 终态：覆盖本地状态
		slot.State = model.OrderStateFilled
	case "CANCELED":
		// 终态：覆盖本地状态
		slot.State = model.OrderStateCanceled
	case "REJECTED":
		slot.State = model.OrderStateRejected
	case "EXPIRED":
		slot.State = model.OrderStateExpired
	default:
		// 未知状态，保持原状态
	}
}

// generateCancelTaskForReconcile reducer内部helper：为对账生成CANCEL任务
// P0-ENG-45: Cycle不匹配时生成CANCEL任务清理脏单
func generateCancelTaskForReconcile(state *model.GridStateSnapshot, levelID int, purpose model.OrderPurpose, order *model.ParsedOrderUpdate) model.Task {
	return model.Task{
		TaskID:          generateTaskIDForReconcile(),
		Type:            model.TaskTypeCancelOrder,
		Symbol:          state.Symbol,
		LevelID:         levelID,
		Purpose:         purpose,
		ClientOrderID:   order.ClientOrderID,
		OrderID:         order.OrderID,
		Attempt:         0,
		LeaseExpireAtMs: 0, // 由Engine设置
		CreatedAtMs:     model.NowMs(),
		State:           model.TaskStatePending,
	}
}

// generateTaskIDForReconcile reducer内部helper：生成TaskID（P0-1: 使用ULID保证唯一性）
func generateTaskIDForReconcile() string {
	return model.GenerateReconcileTaskID()
}

// ========== FAIL-04: Cycle演进reducer ==========

// ApplyLevelCycleIncrement reducer: Entry FILLED后递增Cycle
// 用于确保下一轮网格生成新的CLID（幂等性保证）
func ApplyLevelCycleIncrement(state *model.GridStateSnapshot, levelID int) (*model.GridStateSnapshot, ReducerEffect) {
	level := FindLevelByID(state, levelID)
	if level == nil {
		// Level不存在，不做处理
		return state, ReducerEffect{}
	}

	// 递增Cycle
	level.Cycle++

	return state, ReducerEffect{}
}
