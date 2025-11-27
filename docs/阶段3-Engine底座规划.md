# 阶段3 - Engine底座规划（最小闭环）

## 一、阶段目标

**最小闭环定义**：
```
PriceTick + UDS(OrderUpdate) + ExecutorResult + GapScan(placeEntry/TP) + Freeze/Reconcile + Lease
```

**后置到阶段4/5**：
- 复利策略细节扩展
- 复杂错误码策略
- 深度对账优化

---

## 二、Engine职责（只做三件事）

### 2.1 单写者事件循环
**规则**：
- 只允许 Engine goroutine 写入 `GridState`（窗口/level/order/利润池）
- 所有状态修改从 `eventCh` 进入
- WS/Executor 只能发事件，禁止直接写状态

**契约**：
```go
// 单写者原则（严格遵守）
// ✅ 允许：Engine.Run() 内通过 reducer 修改状态
// ❌ 禁止：WS/Executor 直接修改 GridState
// ❌ 禁止：Engine 外部 goroutine 写入状态
```

### 2.2 状态机（写操作闸门）
**三种模式**：
- `RUNNING`：正常运行，允许所有操作
- `FREEZE`：冻结状态，禁止写 Task（place/cancel/modify），只允许 query
- `RECONCILING`：对账中，禁止写 Task，执行 reconcile 流程

**转换规则**：
```
RUNNING → FREEZE：WS 断线事件
FREEZE → RECONCILING：WS 重连完成
RECONCILING → RUNNING：对账通过
RECONCILING → FREEZE：对账失败
```

### 2.3 任务编排（Task生成与下发）
**职责**：
- GapScan：扫描窗口内缺口，生成 Task
- Lease管理：设置超时时间，避免永久卡住
- 重试/回收：超时/失败任务重新调度或降级

**明确不做**：
- ❌ 不直接调用 Binance API（只能通过 Executor 接口）
- ❌ 不做 WS 层策略（已在阶段2冻结）
- ❌ 不做复利策略细节（阶段3只保留最小闭环）

---

## 三、模块结构（pkg/ 目录）

### 3.1 目录设计
```
pkg/
├── model/              # 数据模型（已完成）
│   ├── types.go        # 事件/配置/状态契约
│   └── ticks.go        # 精度转换工具
├── ws/                 # WebSocket底座（已完成）
│   ├── types.go
│   ├── market.go
│   ├── uds.go
│   ├── trade.go
│   └── manager.go
├── engine/             # Engine底座（阶段3核心）
│   ├── engine.go       # 事件循环、状态机、dispatch
│   ├── reducer.go      # 纯函数化状态更新（event -> mutations）
│   ├── planner.go      # GapScan/Task生成（state -> tasks）
│   └── clid.go         # CLID解析与反解析
├── executor/           # Executor接口（阶段3核心）
│   ├── executor.go     # 接口定义（Submit/Cancel/Query）
│   └── ws_executor.go  # WS实现（Task -> TradeWS -> ExecutorResult）
└── store/              # 状态持久化（最小实现）
    └── snapshot.go     # 定时写 + 原子替换
```

### 3.2 核心模块说明

#### engine/engine.go - 事件循环与状态机
```go
type Engine struct {
	state     *model.GridStateSnapshot  // 唯一状态源
	eventCh   chan model.EngineEvent    // 事件入口
	executor  executor.Executor         // 任务执行器
	store     store.SnapshotStore       // 状态持久化
	
	// 状态机
	mode      model.EngineMode          // RUNNING/FREEZE/RECONCILING
	
	// 闸门控制
	canWrite  bool                      // 是否允许写操作
}

// Run 单写者事件循环（唯一入口）
func (e *Engine) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-e.eventCh:
			e.dispatch(ev)  // 分发事件
		}
	}
}
```

#### engine/reducer.go - 纯函数化状态更新
```go
// Reducer 纯函数化状态更新（无副作用）
// 输入：当前状态 + 事件
// 输出：新状态 + 副作用列表（Task/Log）

// ReducePriceTick 处理价格tick
func ReducePriceTick(state *model.GridStateSnapshot, ev model.PriceTickEvent) (mutations []StateMutation) {
	// 更新市场状态
	// 窗口移动逻辑（如果需要）
	return mutations
}

// ReduceOrderUpdate 处理订单更新（UDS）
func ReduceOrderUpdate(state *model.GridStateSnapshot, ev model.OrderUpdateEvent) (mutations []StateMutation) {
	// CLID -> LevelID 映射
	// 更新 OrderSlot 状态
	// 触发 TP 生成（Entry FILLED时）
	return mutations
}

// ReduceExecutorResult 处理执行器结果
func ReduceExecutorResult(state *model.GridStateSnapshot, ev model.ExecutorResultEvent) (mutations []StateMutation) {
	// 双保险闭环：ExecutorResult + UDS
	// 失败处理：REJECTED/FAILED 分支
	// Lease 释放
	return mutations
}
```

#### engine/planner.go - Task生成器
```go
// GapScan 缺口扫描（最小闭环）
func GapScan(state *model.GridStateSnapshot) []model.Task {
	// 规则1：区间外 Entry 全撤
	// 规则2：TP 区间外保留 TPKeepOutsideLevels 格
	// 规则3：区间内缺口立即补齐
	
	var tasks []model.Task
	
	// 扫描窗口内所有 level
	for levelID := minLevel; levelID <= maxLevel; levelID++ {
		level := findLevel(state, levelID)
		
		// Entry 缺口检测
		if needPlaceEntry(level) {
			tasks = append(tasks, generatePlaceEntryTask(level))
		}
		
		// TP 缺口检测
		if needPlaceTP(level) {
			tasks = append(tasks, generatePlaceTPTask(level))
		}
	}
	
	return tasks
}
```

#### executor/executor.go - 接口定义
```go
// Executor 任务执行器接口
type Executor interface {
	// Submit 提交任务（异步）
	// 返回：nil表示提交成功，将来通过ExecutorResultEvent回流
	Submit(ctx context.Context, task *model.Task) error
	
	// Cancel 取消任务（最大努力）
	Cancel(ctx context.Context, taskID string) error
	
	// Query 查询订单（同步/异步可配置）
	Query(ctx context.Context, req QueryRequest) (*QueryResponse, error)
}
```

#### executor/ws_executor.go - WS实现
```go
// WSExecutor TradeWS 执行器实现
type WSExecutor struct {
	tradeClient *ws.TradeClient
	eventCh     chan<- model.EngineEvent  // 回流ExecutorResultEvent
}

// Submit 提交任务到 TradeWS
func (e *WSExecutor) Submit(ctx context.Context, task *model.Task) error {
	// Task -> TradeWS 请求
	resp, err := e.tradeClient.SendRequest(task.TaskID, task.ClientOrderID, method, params)
	
	// TradeWSResponse -> ExecutorResultEvent
	event := model.EngineEvent{
		Type: model.EventTypeExecutorResult,
		Data: convertToExecutorResult(resp, task),
	}
	
	// 回流到 Engine
	e.eventCh <- event
	return nil
}
```

---

## 四、阶段3 P0任务清单（实施顺序）

### P0-E-00 — 入口门禁（必须先满足）

#### P0-E-00a: 事件契约冻结验证
**目标**：确保 `model/types.go` 中的关键字段不再随意改动

**验收标准**：
- [ ] `EngineEvent` 字段稳定（Type + Data）
- [ ] `ExecutorResultEvent` 字段稳定（TaskID/ReqID/ErrorCode 等）
- [ ] `Task` 字段稳定（TaskID/LevelID/ClientOrderID/Lease 等）
- [ ] 后续改动必须走 patch 清单（记录变更原因）

#### P0-E-00b: 旧文件隔离
**目标**：隔离旧版本 WS 文件，避免 CI/IDE 混编

**方案**：
```go
// 在旧文件顶部添加 build ignore
//go:build ignore
// +build ignore

// 或移动到 _archive/ 目录
_archive/
├── ws_manager.go
├── ws_order.go
├── ws_ticker.go
└── ws_user_stream.go
```

**验收标准**：
- [ ] `go build ./...` 不会编译旧文件
- [ ] IDE 不会报重复定义错误

---

### P0-E-01 — Engine单写者主循环

**目标**：所有状态修改都从一个 goroutine 发生

**实现**：
```go
func (e *Engine) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-e.eventCh:
			e.dispatch(ev)
		}
	}
}

func (e *Engine) dispatch(ev model.EngineEvent) {
	switch ev.Type {
	case model.EventTypePriceTick:
		e.handlePriceTick(ev.Data.(model.PriceTickEvent))
	case model.EventTypeOrderUpdate:
		e.handleOrderUpdate(ev.Data.(model.OrderUpdateEvent))
	case model.EventTypeExecutorResult:
		e.handleExecutorResult(ev.Data.(model.ExecutorResultEvent))
	// ... 其他事件
	}
}
```

**验收标准**：
- [ ] `go test -race ./...` 通过（Linux CI）
- [ ] 任意地方不得直接写 `state`（只允许 reducer 内写）
- [ ] 所有事件都通过 `eventCh` 进入

---

### P0-E-02 — Freeze/Reconcile状态机

**目标**：写操作闸门，禁止在 FREEZE/RECONCILING 下发写 Task

**规则**（硬约束）：
- `FREEZE/RECONCILING`：禁止 place/cancel/modify，只允许 query
- `RUNNING`：允许所有操作
- WS 断线 → 立即 `FREEZE`
- 重连完成 → `RECONCILING` → 通过后 → `RUNNING`

**实现**：
```go
func (e *Engine) canSubmitWriteTask() bool {
	return e.mode == model.EngineModeRunning
}

func (e *Engine) handleWSDisconnect() {
	e.mode = model.EngineModeFreeze
	e.canWrite = false
	// 清理 INFLIGHT 任务
	e.cleanupInflightTasks()
}
```

**验收标准**：
- [ ] FREEZE 状态下，`planner.GapScan()` 生成的写 Task 必须为 0
- [ ] WS 断线事件立即触发 `FREEZE`
- [ ] 单元测试覆盖所有状态转换

---

### P0-E-03 — Level/OrderSlot状态归因（CLID映射）

**目标**：任何 UDS/Trade 回包，能 100% 映射到 `LevelState.entry/tp` 的 `OrderSlot`

**实现**：
```go
// ParseCLID 解析 clientOrderId
// 格式：{prefix}:E:{levelId}:{cycle}  (Entry)
//       {prefix}:T:{levelId}:{cycle}  (TP)
func ParseCLID(clid string, prefix string) (purpose model.OrderPurpose, levelID int, cycle int, err error) {
	// 实现解析逻辑
}

// BuildEntryCLID 生成 Entry CLID
func BuildEntryCLID(prefix string, levelID int, cycle int) string {
	return fmt.Sprintf("%s:E:%d:%d", prefix, levelID, cycle)
}

// BuildTPCLID 生成 TP CLID
func BuildTPCLID(prefix string, levelID int, cycle int) string {
	return fmt.Sprintf("%s:T:%d:%d", prefix, levelID, cycle)
}
```

**验收标准**：
- [ ] 任意 `OrderUpdateEvent` 都可落到某个 Level
- [ ] 未知前缀/未知订单必须记录并忽略（不能 panic）
- [ ] 单元测试覆盖所有解析场景

---

### P0-E-04 — Executor接口与双保险闭环

**目标**：`Task -> TradeWS -> ExecutorResultEvent -> Engine`

**规则**：
- Engine 只信 `ExecutorResultEvent` + `UDS`，不信"调用成功假设"
- 同一 Task 必须带 `taskId` + `reqID` + `clientOrderId`

**实现**：
```go
// WSExecutor.Submit
func (e *WSExecutor) Submit(ctx context.Context, task *model.Task) error {
	resp, err := e.tradeClient.SendRequest(
		task.TaskID,
		generateReqID(),
		buildMethod(task),
		buildParams(task),
	)
	
	// 无论成功失败，都要回流 ExecutorResultEvent
	event := model.EngineEvent{
		Type: model.EventTypeExecutorResult,
		Data: model.ExecutorResultEvent{
			TaskID:    task.TaskID,
			OK:        err == nil && resp.OK,
			ErrorCode: resp.ErrorCode,
			ErrorMsg:  resp.ErrorMsg,
			// ...
		},
	}
	
	e.eventCh <- event
	return nil
}
```

**验收标准**：
- [ ] 下单成功：Engine 能从 `ExecutorResult`/`UDS` 推进到 `OPEN`/`FILLED`
- [ ] 下单失败：Engine 进入 `REJECTED`/`FAILED` 分支，不卡死 `INFLIGHT`
- [ ] 单元测试覆盖双保险闭环

---

### P0-E-05 — Lease超时回收

**目标**：任何 INFLIGHT Task 超时可回收并重试/降级

**实现**：
```go
// TimerEvent(LEASE_SCAN) 定期触发
func (e *Engine) handleLeaseScan() {
	now := model.NowMs()
	
	// 扫描所有 INFLIGHT 任务
	for levelID, level := range e.state.Levels {
		if level.Entry.State == model.OrderStateSubmitted {
			if now > level.Entry.LeaseExpireAtMs {
				// 超时回收
				e.recoverTask(levelID, model.OrderPurposeEntry)
			}
		}
	}
}

func (e *Engine) recoverTask(levelID int, purpose model.OrderPurpose) {
	task := e.findTask(levelID, purpose)
	
	// 重试次数检查
	if task.Attempt >= e.config.MaxAttempt {
		// 转 FREEZE_RECONCILE（拉闸等对账）
		e.mode = model.EngineModeFreeze
		return
	}
	
	// 重新调度
	task.Attempt++
	task.State = model.TaskStatePending
	e.submitTask(task)
}
```

**验收标准**：
- [ ] 人为模拟"Executor 不回包"，60s 内不会永远卡 INFLIGHT
- [ ] 重试次数到上限 → 转 `FREEZE`
- [ ] 单元测试覆盖超时回收逻辑

---

### P0-E-06 — GapScan缺口扫描（最小闭环）

**目标**：在 RUNNING/RECONCILING 下，计算"窗口内缺哪些 entry/tp"

**最小规则**：
- 区间外：Entry 全撤
- TP：区间外保留 `TPKeepOutsideLevels` 格
- 回到区间内：缺口立即补齐

**实现**：
```go
func GapScan(state *model.GridStateSnapshot) []model.Task {
	var tasks []model.Task
	
	// 计算窗口范围
	minLevel := calcMinLevel(state.Market.LastPriceTicks, state.Window)
	maxLevel := calcMaxLevel(state.Market.LastPriceTicks, state.Window)
	
	// 扫描所有 level
	for levelID := minLevel; levelID <= maxLevel; levelID++ {
		level := findOrCreateLevel(state, levelID)
		
		// Entry 缺口
		if level.Entry.State == model.OrderStateNone {
			tasks = append(tasks, generatePlaceEntryTask(state, level))
		}
		
		// TP 缺口（Entry FILLED后）
		if level.Entry.State == model.OrderStateFilled && level.TP.State == model.OrderStateNone {
			tasks = append(tasks, generatePlaceTPTask(state, level))
		}
	}
	
	return tasks
}
```

**验收标准**：
- [ ] 给定 snapshot（缺 3 张 entry），GapScan 生成恰好 3 张 placeEntry 任务
- [ ] TP 保留逻辑正确（区间外保留 5 格）
- [ ] 单元测试覆盖所有缺口场景

---

## 五、验收标准（阶段3整体）

### 5.1 功能验收
- [ ] **PriceTick → Market状态更新**：最新价正确更新
- [ ] **OrderUpdate(UDS) → OrderSlot更新**：CLID 映射正确
- [ ] **ExecutorResult → Lease释放**：双保险闭环工作
- [ ] **GapScan → Task生成**：缺口正确识别
- [ ] **Freeze/Reconcile → 写闸门**：状态机正确拦截
- [ ] **Lease超时 → 回收重试**：不会永久卡住

### 5.2 质量验收
- [ ] `go test -race ./pkg/engine` 通过
- [ ] `go test -race ./pkg/executor` 通过
- [ ] 单元测试覆盖率 > 80%
- [ ] 无 goroutine 泄漏（通过 pprof 验证）

### 5.3 文档验收
- [ ] Engine 设计文档完整
- [ ] Executor 接口文档完整
- [ ] 状态机转换图清晰
- [ ] CLID 格式规范文档

---

## 六、不做清单（避免臃肿）

### ❌ 阶段3明确不做
- 复利策略细节扩展（只保留"利润池+阈值调整 entryQty"最小闭环）
- 复杂错误码策略（只做 OK/REJECTED/FAILED 三分支）
- 深度对账优化（只做基础 reconcile 流程）
- REST API 执行器（只做 WS 执行器）
- WAL 日志（只做快照落盘）
- 性能优化（后置到阶段4）

### ✅ 阶段3必须做
- 单写者事件循环（race-free）
- 状态机（RUNNING/FREEZE/RECONCILING）
- 双保险闭环（ExecutorResult + UDS）
- Lease 超时回收（防卡死）
- GapScan 缺口扫描（最小规则）
- 基础快照落盘（定时写 + 原子替换）

---

## 七、成功标准（Demo可运行）

**最小闭环可运行**：
```bash
# 1. 启动 Engine
$ go run main.go

# 2. 观察日志输出
[INFO] Engine started, mode=RUNNING
[INFO] PriceTick: BTCUSDT 50000.0
[INFO] GapScan: found 10 gaps, generating tasks...
[INFO] Task submitted: PLACE_ENTRY levelID=5
[INFO] ExecutorResult: OK, ENTRY placed, orderId=12345
[INFO] OrderUpdate(UDS): ENTRY OPEN, levelID=5
[INFO] OrderUpdate(UDS): ENTRY FILLED, levelID=5
[INFO] GapScan: TP gap detected, levelID=5
[INFO] Task submitted: PLACE_TP levelID=5
[INFO] ExecutorResult: OK, TP placed, orderId=12346

# 3. WS断线测试
[WARN] WS disconnected: TRADE
[INFO] Engine mode changed: RUNNING -> FREEZE
[INFO] All INFLIGHT tasks cleaned up

# 4. WS重连测试
[INFO] WS reconnected: TRADE
[INFO] Engine mode changed: FREEZE -> RECONCILING
[INFO] Reconcile started...
[INFO] Reconcile completed, mode=RUNNING
```

---

**报告生成时间**：2025-11-26  
**规划人**：Qoder AI  
**状态**：✅ 待实施
