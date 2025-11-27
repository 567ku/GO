// P0-E-01: Engine单写者主循环
// P0-E-02: Freeze/Reconcile状态机
package engine

import (
	"context"
	"fmt"
	"gridbot/pkg/executor"
	"gridbot/pkg/model"
	"gridbot/pkg/store"
	"sync"
	"sync/atomic"
	"time"
)

var ErrEngineNotRunning = fmt.Errorf("engine not running")

// Engine 网格交易引擎（单写者）
// 职责：
// 1. 单写者事件循环：所有状态修改只在Engine goroutine中发生
// 2. 状态机：RUNNING/FREEZE/RECONCILING，控制写操作闸门
// 3. 任务编排：生成Task，下发给Executor，Lease管理
type Engine struct {
	// 唯一状态源（只允许Engine goroutine写入）
	state *model.GridStateSnapshot

	// 事件通道（所有事件的唯一入口）
	eventCh chan model.EngineEvent

	// 配置
	config EngineConfig

	// Stage 4A: Snapshot存储
	snapshotStore *store.SnapshotStore

	// Stage 5A: Executor查询接口（依赖注入）
	executor ExecutorQueryInterface

	// FAIL-01/02: 任务执行器（用于提交写任务）
	taskExecutor executor.Executor

	// P0-2: Engine运行期上下文（绑定生命周期）
	// 禁止使用context.Background()，Stop/Cancel可收敛
	ctx context.Context

	// 状态机（P0-E-02）
	mode model.EngineMode // RUNNING/FREEZE/RECONCILING

	// P0-E-02-A: reconcile控制
	reconcileInFlight bool // 是否正在reconcile（防止重复触发）

	// Stage 5D: 运维指标（原子操作字段）
	eventsProcessed        int64 // 已处理事件总数
	lastEventAtMs          int64 // 最后事件时间
	leaseTimeoutCount      int64 // Lease超时累计
	freezeCount            int64 // Freeze累计次数
	lastFreezeAtMs         int64 // 最后Freeze时间
	reconcileCount         int64 // Reconcile累计次数
	lastReconcileAtMs      int64 // 最后Reconcile时间
	taskSubmitFailureCount int64 // P1-D: 任务提交失败累计
	lastSubmitFailureAtMs  int64 // P1-D: 最近一次提交失败时间

	// Stage 5D: 运维指标（需加锁访问的字段）
	metricsLock      sync.RWMutex
	lastFreezeReason string // 最后Freeze原因
	marketWSHealth   string // Market WS健康状态
	udsWSHealth      string // UDS WS健康状态
	tradeWSHealth    string // Trade WS健康状态

	// 并发控制
	mu         sync.RWMutex // 保护mode读写
	stopCh     chan struct{}
	stopClosed int32 // P0-ENG-STOP-01: Stop幂等标志位（0未关闭/1已关闭，atomic）
	wg         sync.WaitGroup
	started    bool
}

// EngineConfig Engine配置
type EngineConfig struct {
	// 网格配置
	Prefix              string
	Symbol              string
	Side                model.GridSide
	StepTicks           int64
	WinMinTicks         int64
	WinMaxTicks         int64
	TPKeepOutsideLevels int

	// 数量配置
	EntryQtyTicks  int64
	PricePrecision int
	QtyPrecision   int

	// Lease配置
	LeasePlaceMs  int
	LeaseModifyMs int
	LeaseCancelMs int
	MaxAttempt    int

	// Stage 4A: Snapshot配置
	SnapshotPath string // Snapshot文件路径（如: data/grid_state.json）

	// P0-ENG-46: ReconcileQuery配置
	ReconcileQueryTimeoutMs int // Reconcile查询超时（毫秒，默认30000）
	ReconcileMaxAttempts    int // Reconcile最大尝试次数（默认3）
	ReconcileRetryDelayMs   int // Reconcile重试延迟（毫秒，默认1000）

	// P1-OPS-11: Metrics配置
	EnableMetricsSummary bool // 是否启用Metrics定时输出（默认false）

	// P1-ENG-12: PositionMode配置
	PositionMode model.PositionMode // 仓位模式（ONE_WAY/HEDGE，默认ONE_WAY）

	// 事件通道缓冲区大小
	EventChSize int
}

// NewEngine 创建Engine实例
func NewEngine(config EngineConfig) *Engine {
	// P1-ENG-12: 设置PositionMode默认值
	if config.PositionMode == "" {
		config.PositionMode = model.PositionModeOneWay // 默认ONE_WAY模式（安全值）
	}

	// P0-ENG-46: 设置Reconcile配置默认值
	if config.ReconcileQueryTimeoutMs <= 0 {
		config.ReconcileQueryTimeoutMs = 30000 // 30秒
	}
	if config.ReconcileMaxAttempts <= 0 {
		config.ReconcileMaxAttempts = 3
	}
	if config.ReconcileRetryDelayMs <= 0 {
		config.ReconcileRetryDelayMs = 1000 // 1秒
	}

	// 初始化空状态
	state := &model.GridStateSnapshot{
		Version:        "1.3",
		Symbol:         config.Symbol,
		Prefix:         config.Prefix,
		EngineMode:     model.EngineModeRunning, // 初始状态为RUNNING
		Side:           config.Side,
		PositionMode:   config.PositionMode, // P1-ENG-12: 注入PositionMode
		PricePrecision: config.PricePrecision,
		QtyPrecision:   config.QtyPrecision,
		Window: model.WindowState{
			WinMinTicks:         config.WinMinTicks,
			WinMaxTicks:         config.WinMaxTicks,
			StepTicks:           config.StepTicks,
			TPKeepOutsideLevels: config.TPKeepOutsideLevels,
		},
		Qty: model.QtyState{
			Mode:          "BASE",
			EntryQtyTicks: config.EntryQtyTicks,
			BaseQtyTicks:  config.EntryQtyTicks,
		},
		Levels:    make([]model.LevelState, 0),
		CLIDIndex: make(map[string]int64),
	}

	eventChSize := config.EventChSize
	if eventChSize <= 0 {
		eventChSize = 1000 // 默认缓冲区大小
	}

	// Stage 4A: 创建Snapshot存储（如果指定了路径）
	var snapshotStore *store.SnapshotStore
	if config.SnapshotPath != "" {
		snapshotStore = store.NewSnapshotStore(config.SnapshotPath)
	}

	return &Engine{
		state:         state,
		eventCh:       make(chan model.EngineEvent, eventChSize),
		config:        config,
		mode:          model.EngineModeRunning,
		stopCh:        make(chan struct{}),
		snapshotStore: snapshotStore,
		// Stage 5A: executor通过SetExecutor注入（可选）
		executor: nil,
		// FAIL-01/02: taskExecutor通过SetTaskExecutor注入（可选）
		taskExecutor: nil,
		// P0-2: 初始化为Background（兼容性），Run时绑定真实ctx
		ctx: context.Background(),
	}
}

// SetExecutor 设置Executor查询接口（Stage 5A: 依赖注入）
// 必须在Run之前调用
func (e *Engine) SetExecutor(executor ExecutorQueryInterface) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executor = executor
}

// SetTaskExecutor 设置任务执行器（FAIL-01/02: 打通写任务提交链路）
// 必须在Run之前调用
func (e *Engine) SetTaskExecutor(taskExecutor executor.Executor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.taskExecutor = taskExecutor
}

// Run 单写者事件循环（P0-E-01）
// 所有状态修改都从这个goroutine发生
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return fmt.Errorf("engine already started")
	}
	e.started = true

	// P0-4: 检查stopCh是否已关闭，如已关闭则重建（支持Stop后再次Run）
	// P0-ENG-STOP-01: 同步重置stopClosed标志位，允许新的Stop调用
	select {
	case <-e.stopCh:
		// stopCh已关闭，重建
		e.stopCh = make(chan struct{})
		atomic.StoreInt32(&e.stopClosed, 0) // 重置标志位
	default:
		// stopCh未关闭，正常
	}

	// P0-2: 绑定Engine生命周期ctx（禁止Background）
	if ctx == nil {
		ctx = context.Background() // 兼容性兀底
	}
	e.ctx = ctx // 绑定运行期ctx，Stop/Cancel可收敛
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.started = false
		e.mu.Unlock()
	}()

	// P0-E-05-A: 启动LeaseScan定时器
	leaseScanInterval := 5 * time.Second // 默认5秒扫描一次
	leaseTicker := time.NewTicker(leaseScanInterval)
	defer leaseTicker.Stop()

	// P1-OPS-11: Metrics Summary定时器（可开关）
	var metricsTicker *time.Ticker
	if e.config.EnableMetricsSummary {
		metricsSummaryInterval := 10 * time.Second
		metricsTicker = time.NewTicker(metricsSummaryInterval)
		defer metricsTicker.Stop()
	}

	for {
		// P1-OPS-11: 根据配置决定是否监听metricsTicker
		var metricsTickerCh <-chan time.Time
		if e.config.EnableMetricsSummary && metricsTicker != nil {
			metricsTickerCh = metricsTicker.C
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopCh:
			return nil
		case ev := <-e.eventCh:
			// Stage 5D: 记录事件处理
			e.RecordEvent()
			// 分发事件
			e.dispatch(ev)
		case <-leaseTicker.C:
			// 定时触发LeaseScan
			// 非阻塞投递，满了就丢弃（避免阻塞主循环）
			select {
			case e.eventCh <- model.EngineEvent{
				Type: model.EventTypeTimer,
				Data: model.TimerEvent{
					Kind: model.TimerKindLeaseScan,
					AtMs: model.NowMs(),
				},
			}:
				// 投递成功
			default:
				// 通道满，丢弃本次扫描（下次再来）
			}
		case <-metricsTickerCh:
			// P1-OPS-11: 定时输出指标summary（10秒一次）
			e.LogMetricsSummary()
		}
	}
}

// dispatch 事件分发器
func (e *Engine) dispatch(ev model.EngineEvent) {
	switch ev.Type {
	case model.EventTypePriceTick:
		e.handlePriceTick(ev.Data.(model.PriceTickEvent))
	case model.EventTypeOrderUpdate:
		e.handleOrderUpdate(ev.Data.(model.OrderUpdateEvent))
	case model.EventTypeTradeUpdate:
		e.handleTradeUpdate(ev.Data.(model.TradeUpdateEvent))
	case model.EventTypeWSState:
		e.handleWSState(ev.Data.(model.WSStateEvent))
	case model.EventTypeTimer:
		e.handleTimer(ev.Data.(model.TimerEvent))
	case model.EventTypeExecutorResult:
		e.handleExecutorResult(ev.Data.(model.ExecutorResultEvent))
	case model.EventTypeStateRequest:
		// P0-RACE-02: 处理状态快照请求
		e.handleStateRequest(ev.Data.(model.StateRequestEvent))
	default:
		// 未知事件类型，忽略
	}
}

// Stop 停止Engine（P0-ENG-STOP-01: 幂等，重复调用不panic）
func (e *Engine) Stop() {
	// P0-ENG-STOP-01: 使用CAS保护close(stopCh)，防止重复close导致panic
	if atomic.CompareAndSwapInt32(&e.stopClosed, 0, 1) {
		// 首次Stop：关闭stopCh
		close(e.stopCh)
	}
	// 后续重复调用：CAS失败，直接返回（幂等）
	// 注意：这里不等待wg，保持原有行为（Stop只发信号，不阻塞）
}

// GetEventCh 获取事件通道（只读）
// WS/Executor可以向此通道发送事件
func (e *Engine) GetEventCh() chan<- model.EngineEvent {
	return e.eventCh
}

// GetState 获取状态快照（只读）
// P0-RACE-02: 改为事件请求模式，避免读写竞态
// 注意：返回的是副本，不能修改
func (e *Engine) GetState(ctx context.Context) (model.GridStateSnapshot, error) {
	e.mu.RLock()
	running := e.started
	e.mu.RUnlock()
	if !running {
		return model.GridStateSnapshot{}, ErrEngineNotRunning
	}
	// 验收红1: GetState必须可超时、可退出
	replyCh := make(chan *model.GridStateSnapshot, 1) // buffer=1，避免阻塞engine loop

	// 发送状态请求事件到engine loop
	select {
	case e.eventCh <- model.EngineEvent{
		Type: model.EventTypeStateRequest,
		Data: model.StateRequestEvent{
			ReplyCh: replyCh,
		},
	}:
		// 请求已发送，等待回复
	case <-ctx.Done():
		return model.GridStateSnapshot{}, ctx.Err()
	case <-e.stopCh:
		return model.GridStateSnapshot{}, fmt.Errorf("engine stopped")
	}

	// 等待engine loop回复
	select {
	case snapshot := <-replyCh:
		if snapshot == nil {
			return model.GridStateSnapshot{}, fmt.Errorf("engine returned nil snapshot")
		}
		return *snapshot, nil
	case <-ctx.Done():
		return model.GridStateSnapshot{}, ctx.Err()
	case <-e.stopCh:
		return model.GridStateSnapshot{}, fmt.Errorf("engine stopped")
	}
}

// ========== P0-E-02: 状态机（写操作闸门） ==========

// SetMode 设置引擎模式（只能在Engine goroutine中调用）
// P0-RET-02: 已废弃，应使用reducer更新状态
func (e *Engine) SetMode(mode model.EngineMode) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.mode == mode {
		return
	}

	// 只更新Engine的mode字段，不直接写state
	// state.EngineMode应通过reducer更新
	e.mode = mode
}

// GetMode 获取引擎模式（线程安全）
func (e *Engine) GetMode() model.EngineMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mode
}

// CanSubmitWriteTask 判断是否允许提交写Task（P0-E-02）
// RUNNING: 允许所有操作
// FREEZE/RECONCILING: 禁止写Task（place/cancel/modify），只允许query
func (e *Engine) CanSubmitWriteTask() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mode == model.EngineModeRunning
}

// assignLease 根据TaskType分配正确的Lease时间（P0-1）
// 严禁统一使用LeasePlaceMs，必须按类型区分：
// - PLACE -> LeasePlaceMs
// - CANCEL -> LeaseCancelMs
// - MODIFY -> LeaseModifyMs
func (e *Engine) assignLease(task *model.Task) {
	now := model.NowMs()
	switch task.Type {
	case model.TaskTypePlaceEntry, model.TaskTypePlaceTP:
		task.LeaseExpireAtMs = now + int64(e.config.LeasePlaceMs)
	case model.TaskTypeCancelOrder:
		task.LeaseExpireAtMs = now + int64(e.config.LeaseCancelMs)
	case model.TaskTypeModifyEntry:
		task.LeaseExpireAtMs = now + int64(e.config.LeaseModifyMs)
	default:
		// 未知类型，使用默认Place Lease（兼容性）
		task.LeaseExpireAtMs = now + int64(e.config.LeasePlaceMs)
	}
}

// ========== 事件处理器（最小实现） ==========

// handlePriceTick 处理价格tick事件
func (e *Engine) handlePriceTick(ev model.PriceTickEvent) {
	// P0-RET-02: 使用reducer更新状态
	var effects ReducerEffect
	e.state, effects = ApplyPriceTick(e.state, ev)
	_ = effects // 阶段3暂未使用effects

	// P0-E-06: 触发GapScan，生成Task
	if e.CanSubmitWriteTask() {
		tasks := GapScan(e.state)

		// FAIL-01: 打通写任务提交链路
		if e.taskExecutor != nil && len(tasks) > 0 {
			// P0-2: 使用Engine.ctx绑定生命周期（禁止Background）
			for i := range tasks {
				// P0-1: 根据TaskType分配正确的Lease（禁止统一LeasePlaceMs）
				e.assignLease(&tasks[i])

				// 提交任务给执行器
				if err := e.taskExecutor.Submit(e.ctx, &tasks[i]); err != nil {
					// P1-D: 提交失败，记录metrics+log（不静默吞掉）
					e.RecordTaskSubmitFailure(tasks[i].Type, tasks[i].TaskID, err)
					// 由Lease超时机制自动重试
				}
			}
		}
	}
}

// handleOrderUpdate 处理订单更新事件（UDS）
func (e *Engine) handleOrderUpdate(ev model.OrderUpdateEvent) {
	// P0-RET-02: 使用reducer更新状态
	var effects ReducerEffect
	e.state, effects = ApplyOrderUpdate(e.state, ev, e.config.Prefix)
	_ = effects
}

// handleTradeUpdate 处理成交更新事件
func (e *Engine) handleTradeUpdate(ev model.TradeUpdateEvent) {
	// P0-RET-02: 使用reducer更新状态
	var effects ReducerEffect
	e.state, effects = ApplyTradeUpdate(e.state, ev)
	_ = effects
}

// handleWSState 处理WS状态事件（P0-E-02: Freeze机制）
func (e *Engine) handleWSState(ev model.WSStateEvent) {
	// Stage 5D: 更新WS健康状态
	e.UpdateWSHealth(ev.Channel, ev.State)

	// P0-RET-02: 使用reducer更新状态
	var effects ReducerEffect
	e.state, effects = ApplyWSStateChange(e.state, ev)
	_ = effects

	// 同步更新Engine的mode字段（线程安全）
	e.mu.Lock()
	e.mode = e.state.EngineMode
	reconciling := e.state.EngineMode == model.EngineModeReconciling
	inFlight := e.reconcileInFlight
	e.mu.Unlock()

	// Stage 5D: 记录Freeze事件
	if e.state.EngineMode == model.EngineModeFreeze {
		e.RecordFreeze(ev.Reason)
	}

	// P0-E-02-A: 触发reconcile流程
	if reconciling && !inFlight {
		// 进入RECONCILING且未在reconcile中,启动reconcile
		// Stage 5A: 同步执行reconcile（保持单写者原则）
		// Note: StartReconcile会自己设置reconcileInFlight=true
		e.RecordReconcile() // Stage 5D: 记录Reconcile事件
		// P0-2: 使用Engine.ctx绑定生命周期
		if err := e.StartReconcile(e.ctx, e.executor); err != nil {
			// TODO: 记录reconcile失败日志
			_ = err
		}
	}
}

// handleTimer 处理定时器事件
func (e *Engine) handleTimer(ev model.TimerEvent) {
	switch ev.Kind {
	case model.TimerKindLeaseScan:
		// P0-E-05: Lease超时回收
		e.scanLeaseTimeout()
	case model.TimerKindReconcileScheduled:
		// TODO: 定时对账
	}
}

// scanLeaseTimeout 扫描Lease超时任务（P0-E-05-A）
func (e *Engine) scanLeaseTimeout() {
	now := model.NowMs()
	var expiredLevels []int
	expiredSet := make(map[int]struct{}) // P0-ENG-32: 去重，避免Entry/TP双倍计数

	// 扫描所有level的INFLIGHT任务
	for i := range e.state.Levels {
		level := &e.state.Levels[i]

		// 检查Entry槽：INFLIGHT && 超时
		if level.Entry.State == model.OrderStateSubmitted {
			// 注意：LeaseExpireAtMs存储在Task中，这里简化处理
			// 实际应该通过TaskID查找对应的Task，这里先直接根据UpdateAtMs估算
			if level.Entry.UpdateAtMs > 0 && now-level.Entry.UpdateAtMs > int64(e.config.LeasePlaceMs) {
				// P0-ENG-32: 去重检查
				if _, exists := expiredSet[level.LevelID]; !exists {
					expiredLevels = append(expiredLevels, level.LevelID)
					expiredSet[level.LevelID] = struct{}{}
				}
			}
		}

		// 检查TP槽：INFLIGHT && 超时
		if level.TP.State == model.OrderStateSubmitted {
			if level.TP.UpdateAtMs > 0 && now-level.TP.UpdateAtMs > int64(e.config.LeasePlaceMs) {
				// P0-ENG-32: 去重检查
				if _, exists := expiredSet[level.LevelID]; !exists {
					expiredLevels = append(expiredLevels, level.LevelID)
					expiredSet[level.LevelID] = struct{}{}
				}
			}
		}
	}

	// 如果有超时任务，调用reducer回收
	if len(expiredLevels) > 0 {
		// Stage 5D: 记录Lease超时
		e.RecordLeaseTimeout()

		var effects ReducerEffect
		e.state, effects = ApplyLeaseScanResult(e.state, expiredLevels, e.config.MaxAttempt)
		_ = effects

		// 同步更新mode（如果触发了FREEZE）
		e.mu.Lock()
		e.mode = e.state.EngineMode
		e.mu.Unlock()
	}
}

// handleExecutorResult 处理执行器结果事件（P0-E-04: 双保险闭环）
func (e *Engine) handleExecutorResult(ev model.ExecutorResultEvent) {
	// P0-RET-02: 使用reducer更新状态
	var effects ReducerEffect
	e.state, effects = ApplyExecutorResult(e.state, ev, e.config.Prefix)
	_ = effects

	// P0-RET-04/05 已完成：
	// - 错误码分类（classifyErrorCode）在ApplyExecutorResult中实现
	// - Lease回收（ApplyLeaseScanResult）在handleTimer中实现
}

// handleStateRequest 处理状态快照请求（P0-RACE-02）
func (e *Engine) handleStateRequest(ev model.StateRequestEvent) {
	// 验收红2: DeepCopy必须真拷贝 map/slice
	snapshot := e.state.DeepCopy()

	// 验收红2: reply channel buffer=1，不会阻塞主循环
	select {
	case ev.ReplyCh <- snapshot:
		// 成功发送
	default:
		// channel已关闭或者请求方已放弃，忽略
	}
}

// ========== Stage 4A: Snapshot/WAL + 崩溃恢复 ==========

// Bootstrap 启动恢复逻辑
// 策略：
// 1. 尝试加载snapshot
// 2. 如果存在，验证并恢复状态
// 3. 如果不存在或损坏，使用初始状态（全新启动）
// 返回：是否从snapshot恢复（true表示恢复成功）
func (e *Engine) Bootstrap() (bool, error) {
	if e.snapshotStore == nil {
		// 未配置snapshot，跳过恢复
		return false, nil
	}

	// 检查snapshot文件是否存在
	if !e.snapshotStore.Exists() {
		// 文件不存在，全新启动
		return false, nil
	}

	// 加载并验证snapshot
	loadedState, err := e.snapshotStore.Load()
	if err != nil {
		// 加载失败（可能损坏），返回错误
		// 生产环境应该告警但继续使用初始状态
		return false, fmt.Errorf("load snapshot failed (will use fresh state): %w", err)
	}

	// 恢复状态
	e.state = loadedState
	e.mode = loadedState.EngineMode

	// TODO: Stage 4A WAL回放（如果需要）
	// 当前简化实现：只恢复snapshot，不回放WAL

	return true, nil
}

// SaveSnapshot 保存当前状态到snapshot
// 注意：只能在Engine goroutine中调用（保证状态一致性）
func (e *Engine) SaveSnapshot() error {
	if e.snapshotStore == nil {
		return fmt.Errorf("snapshot store not configured")
	}

	// 更新SavedAtMs字段
	e.state.SavedAtMs = model.NowMs()

	// 保存到磁盘
	if err := e.snapshotStore.Save(e.state); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return nil
}
