// Package model 定义v1.3冻结的所有枚举、结构体和字段
// 严格按照《网格需求文档-v1.3-字段与接口规范.md》实现
package model

import (
	"fmt"
	"regexp"
	"time"
)

// ============= 1. 枚举定义（强制，必须按字符串实现） =============

// GridSide 网格方向
type GridSide string

const (
	GridSideLong  GridSide = "LONG"
	GridSideShort GridSide = "SHORT"
)

// EngineMode 引擎模式
type EngineMode string

const (
	EngineModeRunning     EngineMode = "RUNNING"
	EngineModeFreeze      EngineMode = "FREEZE"
	EngineModeReconciling EngineMode = "RECONCILING"
)

// PositionMode 持仓模式
type PositionMode string

const (
	PositionModeOneWay PositionMode = "ONE_WAY"
	PositionModeHedge  PositionMode = "HEDGE"
)

// OrderPurpose 订单用途
type OrderPurpose string

const (
	OrderPurposeEntry OrderPurpose = "ENTRY"
	OrderPurposeTP    OrderPurpose = "TP"
)

// OrderState 订单状态（本地）
type OrderState string

const (
	OrderStateNone      OrderState = "NONE"
	OrderStateSubmitted OrderState = "SUBMITTED" // 请求已发出，等待确认
	OrderStateOpen      OrderState = "OPEN"      // 交易所 NEW
	OrderStatePartial   OrderState = "PARTIAL"   // executedQty > 0 且未终态
	OrderStateFilled    OrderState = "FILLED"    // 终态
	OrderStateCanceled  OrderState = "CANCELED"  // 终态
	OrderStateRejected  OrderState = "REJECTED"  // 终态
	OrderStateExpired   OrderState = "EXPIRED"   // 终态
)

// IsTerminal 判断是否为终态（不可变不变量）
func (s OrderState) IsTerminal() bool {
	return s == OrderStateFilled || s == OrderStateCanceled ||
		s == OrderStateRejected || s == OrderStateExpired
}

// TaskType 任务类型
type TaskType string

const (
	TaskTypePlaceEntry        TaskType = "PLACE_ENTRY"
	TaskTypePlaceTP           TaskType = "PLACE_TP"
	TaskTypeModifyEntry       TaskType = "MODIFY_ENTRY" // 仅原生modify
	TaskTypeCancelOrder       TaskType = "CANCEL_ORDER"
	TaskTypeQueryOpenOrders   TaskType = "QUERY_OPEN_ORDERS"
	TaskTypeQueryRecentTrades TaskType = "QUERY_RECENT_TRADES"
	TaskTypeQueryOrder        TaskType = "QUERY_ORDER"
	TaskTypeQueryPositionMode TaskType = "QUERY_POSITION_MODE"
)

// TaskState 任务状态
type TaskState string

const (
	TaskStatePending  TaskState = "PENDING"
	TaskStateInFlight TaskState = "INFLIGHT"
	TaskStateDone     TaskState = "DONE"
	TaskStateFailed   TaskState = "FAILED"
	TaskStateDead     TaskState = "DEAD"
)

// EventSource 事件来源（用于lastSource字段）
type EventSource string

const (
	EventSourceResp  EventSource = "RESP"  // 请求回包
	EventSourceUDS   EventSource = "UDS"   // 用户数据流
	EventSourceQuery EventSource = "QUERY" // 主动查询
)

// ============= 2. 配置字段（Config Schema） =============

// ExchangeConfig 交易所配置
type ExchangeConfig struct {
	Name                  string `yaml:"name"`                     // 固定 binance_um
	Symbol                string `yaml:"symbol"`                   // 如 BTCUSDT
	APIMode               string `yaml:"api_mode"`                 // websocket/rest
	RecvWindowMs          int    `yaml:"recv_window_ms"`           // 默认 5000
	WSTradeURL            string `yaml:"ws_trade_url"`             // WS 下单连接
	WSMarketURL           string `yaml:"ws_market_url"`            // 公共行情
	UDSURL                string `yaml:"uds_url"`                  // 用户数据流
	ListenKeyKeepaliveSec int    `yaml:"listen_key_keepalive_sec"` // 25min
	SessionMaxSec         int    `yaml:"session_max_sec"`          // 24h
	PingPongRequired      bool   `yaml:"ping_pong_required"`       // 必须响应ping/pong
}

// GridConfig 网格配置
type GridConfig struct {
	Prefix              string   `yaml:"prefix"`                 // GridPrefix（订单隔离标识）
	Side                GridSide `yaml:"side"`                   // LONG/SHORT
	StepTicks           int64    `yaml:"step_ticks"`             // 网格步长（ticks）
	WinMinTicks         int64    `yaml:"win_min_ticks"`          // 窗口下沿
	WinMaxTicks         int64    `yaml:"win_max_ticks"`          // 窗口上沿
	TPKeepOutsideLevels int      `yaml:"tp_keep_outside_levels"` // TP扩展保留格数（默认5）
	TimeInForce         string   `yaml:"time_in_force"`          // 默认 GTC
	AllowMarketFill     bool     `yaml:"allow_market_fill"`      // 允许限价单瞬间成交
	TickCoalesce        bool     `yaml:"tick_coalesce"`          // 行情可合并
}

// QtyConfig 数量配置
type QtyConfig struct {
	Mode             string `yaml:"mode"`               // BASE/QUOTE
	BaseQtyTicks     int64  `yaml:"base_qty_ticks"`     // BASE模式数量
	QuoteUsdTicks    int64  `yaml:"quote_usd_ticks"`    // QUOTE模式投入U
	PricePrecision   int    `yaml:"price_precision"`    // 价格精度
	QtyPrecision     int    `yaml:"qty_precision"`      // 数量精度
	QuotePrecision   int    `yaml:"quote_precision"`    // QUOTE金额精度（固定2，表示0.01U）
	MinQtyTicks      int64  `yaml:"min_qty_ticks"`      // 最小qty
	MinNotionalTicks int64  `yaml:"min_notional_ticks"` // 最小名义（按quotePrecision）
	RoundMode        string `yaml:"round_mode"`         // 取整策略（默认DOWN）
}

// CompoundConfig 复利配置
type CompoundConfig struct {
	Enabled               bool  `yaml:"enabled"`                  // 是否启用
	ThresholdUsdTicks     int64 `yaml:"threshold_usd_ticks"`      // 触发阈值
	IncrementBaseQtyTicks int64 `yaml:"increment_base_qty_ticks"` // 每次增加qty
	MaxBaseQtyTicks       int64 `yaml:"max_base_qty_ticks"`       // 最大qty
	RequoteOpenEntries    bool  `yaml:"requote_open_entries"`     // 是否修改未成交Entry
}

// RuntimeConfig 运行时配置
type RuntimeConfig struct {
	LeasePlaceMs              int    `yaml:"lease_place_ms"`               // PLACE/MODIFY lease（默认3000）
	LeaseModifyMs             int    `yaml:"lease_modify_ms"`              // MODIFY lease（默认3000）
	LeaseCancelMs             int    `yaml:"lease_cancel_ms"`              // CANCEL lease（默认2000）
	MaxAttempt                int    `yaml:"max_attempt"`                  // 重试上限（默认8）
	BackoffProfile            string `yaml:"backoff_profile"`              // 退避策略名（默认exp_200ms_5s）
	ReconcileOnStart          bool   `yaml:"reconcile_on_start"`           // 启动对账（默认true）
	ReconcileOnWSReconnect    bool   `yaml:"reconcile_on_ws_reconnect"`    // WS重连对账（默认true）
	FreezeOnWSDisconnect      bool   `yaml:"freeze_on_ws_disconnect"`      // WS断线Freeze（默认true）
	ReconcileTimeWindowSec    int    `yaml:"reconcile_time_window_sec"`    // recentTrades查询窗口（默认3600）
	Persist                   bool   `yaml:"persist"`                      // 是否落盘快照（默认true）
	SnapshotPath              string `yaml:"snapshot_path"`                // 快照路径（默认data/grid_state.json）
	WALPath                   string `yaml:"wal_path"`                     // WAL路径（默认data/grid_wal.log）
	BootstrapBatchSize        int    `yaml:"bootstrap_batch_size"`         // 启动批量大小（默认100）
	BootstrapBatchCooldownSec int    `yaml:"bootstrap_batch_cooldown_sec"` // 启动批量间隔（默认20秒，可配置）
	BootstrapApplyOnStart     bool   `yaml:"bootstrap_apply_on_start"`     // 启动批量节流开关（默认true）
	BootstrapApplyOnReconcile bool   `yaml:"bootstrap_apply_on_reconcile"` // 对账批量节流开关（默认true）
	QueryDelayMs              int    `yaml:"query_delay_ms"`               // Query延迟（默认250）
	MaxQueryAttempt           int    `yaml:"max_query_attempt"`            // Query重试次数（默认3）
	QueryBackoffMs            []int  `yaml:"query_backoff_ms"`             // Query退避（默认[250, 750, 1500]）
	// WS ping/pong配置（不写死，全部可配置）
	WSPingIntervalSec int `yaml:"ws_ping_interval_sec"` // WS ping间隔（默认10秒，Binance要求3分钟ping）
	WSPongTimeoutSec  int `yaml:"ws_pong_timeout_sec"`  // WS pong超时（默认180秒，Binance要求10分钟内pong）
	WSIdleTimeoutSec  int `yaml:"ws_idle_timeout_sec"`  // WS空闲超时（默认600秒）
}

// ============= 3. 状态快照字段（Snapshot Schema） =============

// GridStateSnapshot 状态快照（顶层）
type GridStateSnapshot struct {
	Version        string       `json:"version"`        // 版本号（固定"1.3"）
	SavedAtMs      int64        `json:"savedAtMs"`      // 保存时间
	Symbol         string       `json:"symbol"`         // 交易对
	Prefix         string       `json:"prefix"`         // GridPrefix
	EngineMode     EngineMode   `json:"engineMode"`     // 引擎模式
	PositionMode   PositionMode `json:"positionMode"`   // 持仓模式
	Side           GridSide     `json:"side"`           // 方向
	PricePrecision int          `json:"pricePrecision"` // 价格精度
	QtyPrecision   int          `json:"qtyPrecision"`   // 数量精度

	Window   WindowState   `json:"window"`   // 窗口状态
	Market   MarketState   `json:"market"`   // 市场状态
	Qty      QtyState      `json:"qty"`      // 数量状态
	Compound CompoundState `json:"compound"` // 复利状态

	Levels    []LevelState     `json:"levels"`    // 所有level状态
	CLIDIndex map[string]int64 `json:"clidIndex"` // clientOrderId -> orderId映射
}

// DeepCopy Stage 5C: 深拷贝GridStateSnapshot，防止外部误写
func (s *GridStateSnapshot) DeepCopy() *GridStateSnapshot {
	if s == nil {
		return nil
	}

	// 拷贝基础字段
	copy := &GridStateSnapshot{
		Version:        s.Version,
		SavedAtMs:      s.SavedAtMs,
		Symbol:         s.Symbol,
		Prefix:         s.Prefix,
		EngineMode:     s.EngineMode,
		PositionMode:   s.PositionMode,
		Side:           s.Side,
		PricePrecision: s.PricePrecision,
		QtyPrecision:   s.QtyPrecision,

		// 结构体直接值拷贝即可（无嵌套引用）
		Window:   s.Window,
		Market:   s.Market,
		Qty:      s.Qty,
		Compound: s.Compound,
	}

	// 深拷贝Levels切片
	if len(s.Levels) > 0 {
		copy.Levels = make([]LevelState, len(s.Levels))
		for i := range s.Levels {
			// LevelState是值类型，直接赋值即可（OrderSlot也是值类型）
			copy.Levels[i] = s.Levels[i]
		}
	}

	// 深拷贝CLIDIndex map
	if len(s.CLIDIndex) > 0 {
		copy.CLIDIndex = make(map[string]int64, len(s.CLIDIndex))
		for k, v := range s.CLIDIndex {
			copy.CLIDIndex[k] = v
		}
	}

	return copy
}

// WindowState 窗口状态
type WindowState struct {
	WinMinTicks         int64 `json:"winMinTicks"`         // 窗口下沿
	WinMaxTicks         int64 `json:"winMaxTicks"`         // 窗口上沿
	StepTicks           int64 `json:"stepTicks"`           // 网格步长
	TPKeepOutsideLevels int   `json:"tpKeepOutsideLevels"` // TP扩展保留格数
}

// MarketState 市场状态
type MarketState struct {
	LastPriceTicks int64 `json:"lastPriceTicks"` // 最新价
	LastPriceAtMs  int64 `json:"lastPriceAtMs"`  // 最新价时间
}

// QtyState 数量状态
type QtyState struct {
	Mode          string `json:"mode"`          // BASE/QUOTE
	EntryQtyTicks int64  `json:"entryQtyTicks"` // 当前Entry数量（复利后会改变）
	BaseQtyTicks  int64  `json:"baseQtyTicks"`  // 初始base qty
	QuoteUsdTicks int64  `json:"quoteUsdTicks"` // 初始quote usd
}

// CompoundState 复利状态
type CompoundState struct {
	Enabled               bool  `json:"enabled"`               // 开关
	PoolATicks            int64 `json:"poolATicks"`            // 累计池A
	ThresholdUsdTicks     int64 `json:"thresholdUsdTicks"`     // 触发阈值
	IncrementBaseQtyTicks int64 `json:"incrementBaseQtyTicks"` // 每次增加qty
	MaxBaseQtyTicks       int64 `json:"maxBaseQtyTicks"`       // 最大qty
}

// LevelState 单个level状态
type LevelState struct {
	LevelID    int       `json:"levelId"`    // level唯一ID
	PriceTicks int64     `json:"priceTicks"` // 格子价格
	Cycle      int       `json:"cycle"`      // 循环次数
	Entry      OrderSlot `json:"entry"`      // Entry订单槽
	TP         OrderSlot `json:"tp"`         // TP订单槽
}

// OrderSlot 订单槽（Entry/TP通用）
type OrderSlot struct {
	Purpose            OrderPurpose `json:"purpose"`            // ENTRY/TP
	ClientOrderID      string       `json:"clientOrderId"`      // 本轮固定ID
	OrderID            int64        `json:"orderId"`            // 交易所orderId
	State              OrderState   `json:"state"`              // 本地状态
	PriceTicks         int64        `json:"priceTicks"`         // 价格
	OrigQtyTicks       int64        `json:"origQtyTicks"`       // 原始数量
	ExecutedQtyTicks   int64        `json:"executedQtyTicks"`   // 已成交数量
	AvgPriceTicks      int64        `json:"avgPriceTicks"`      // 成交均价
	UpdateAtMs         int64        `json:"updateAtMs"`         // 最后更新时间
	LastExchangeStatus string       `json:"lastExchangeStatus"` // 交易所原始状态
	LastSource         EventSource  `json:"lastSource"`         // 最后更新来源
	Attempt            int          `json:"attempt"`            // P0-RET-05: 重试次数计数器
}

// ============= 4. 事件与接口契约（Engine <-> Executor <-> WS/REST） =============

// EngineEvent Engine输入事件（唯一入口）
type EngineEvent struct {
	Type EventType
	Data interface{}
}

// EventType 事件类型
type EventType int

const (
	EventTypePriceTick EventType = iota + 1
	EventTypeOrderUpdate
	EventTypeTradeUpdate
	EventTypeWSState
	EventTypeTimer
	EventTypeExecutorResult
)

// PriceTickEvent 价格tick事件
type PriceTickEvent struct {
	Symbol     string
	PriceTicks int64
	EventAtMs  int64
}

// OrderUpdateEvent 订单更新事件
type OrderUpdateEvent struct {
	Symbol           string
	ClientOrderID    string
	OrderID          int64
	Status           string // NEW/FILLED/CANCELED...
	Side             string
	PositionSide     string
	PriceTicks       int64
	OrigQtyTicks     int64
	ExecutedQtyTicks int64
	AvgPriceTicks    int64
	UpdateAtMs       int64
}

// TradeUpdateEvent 成交更新事件
type TradeUpdateEvent struct {
	Symbol           string
	ClientOrderID    string
	OrderID          int64
	QtyTicks         int64
	PriceTicks       int64
	TradeTimeMs      int64
	RealizedPnlTicks *int64 // 可选（nil表示缺失，禁止用0表达缺失）
}

// WSStateEvent WS状态事件
type WSStateEvent struct {
	Channel WSChannel `json:"channel"` // MARKET/UDS/TRADE
	State   WSState   `json:"state"`   // CONNECTED/DISCONNECTED/RECONNECTED
	AtMs    int64     `json:"atMs"`
	Reason  string    `json:"reason"` // 原因（必须落到结构体）
}

// WSChannel WS通道类型
type WSChannel string

const (
	WSChannelMarket WSChannel = "MARKET"
	WSChannelUDS    WSChannel = "UDS"
	WSChannelTrade  WSChannel = "TRADE"
)

// WSState WS状态
type WSState string

const (
	WSStateConnected    WSState = "CONNECTED"
	WSStateDisconnected WSState = "DISCONNECTED"
	WSStateReconnected  WSState = "RECONNECTED"
)

// TimerEvent 定时器事件
type TimerEvent struct {
	Kind TimerKind // LEASE_SCAN / RECONCILE_SCHEDULED
	AtMs int64
}

// TimerKind 定时器类型
type TimerKind string

const (
	TimerKindLeaseScan          TimerKind = "LEASE_SCAN"
	TimerKindReconcileScheduled TimerKind = "RECONCILE_SCHEDULED"
)

// ExecutorResultEvent 执行器结果事件
type ExecutorResultEvent struct {
	TaskID         string             `json:"taskId"`
	TaskType       TaskType           `json:"taskType"`
	OK             bool               `json:"ok"`
	ReqID          string             `json:"reqId"`     // WS请求ID（必填，用于请求相关性）
	ErrorCode      int                `json:"errorCode"` // Binance code
	ErrorMsg       string             `json:"errorMsg"`
	RawResponse    string             `json:"rawResponse"` // 原始回包（可截断）
	AtMs           int64              `json:"atMs"`
	ExchangeTimeMs int64              `json:"exchangeTimeMs"` // 交易所时间（强烈建议）
	ParsedOrder    *ParsedOrderUpdate `json:"parsedOrder"`    // 可选
}

// ParsedOrderUpdate 解析后的订单更新
type ParsedOrderUpdate struct {
	ClientOrderID    string `json:"clientOrderId"`
	OrderID          int64  `json:"orderId"`
	Status           string `json:"status"`
	PriceTicks       int64  `json:"priceTicks"`
	OrigQtyTicks     int64  `json:"origQtyTicks"`
	ExecutedQtyTicks int64  `json:"executedQtyTicks"`
	AvgPriceTicks    int64  `json:"avgPriceTicks"`
}

// ============= 5. Task字段（Executor输入） =============

// Task 任务
type Task struct {
	TaskID          string       `json:"taskId"` // ULID/UUID
	Type            TaskType     `json:"type"`
	Symbol          string       `json:"symbol"`
	LevelID         int          `json:"levelId"`         // 关联格子（QUERY类可为-1）
	Purpose         OrderPurpose `json:"purpose"`         // ENTRY/TP
	ClientOrderID   string       `json:"clientOrderId"`   // PLACE/MODIFY的newClientOrderId
	OrderID         int64        `json:"orderId"`         // CANCEL/QUERY用
	PriceTicks      int64        `json:"priceTicks"`      // PLACE/MODIFY
	QtyTicks        int64        `json:"qtyTicks"`        // PLACE/MODIFY
	PositionSide    string       `json:"positionSide"`    // BOTH/LONG/SHORT
	ReduceOnly      *bool        `json:"reduceOnly"`      // One-way TP必须true；Hedge必须nil
	Attempt         int          `json:"attempt"`         // 由Engine管理
	LeaseExpireAtMs int64        `json:"leaseExpireAtMs"` // 由Engine管理
	CreatedAtMs     int64        `json:"createdAtMs"`
	State           TaskState    `json:"state"`
}

// ============= 6. 校验函数（必须实现） =============

// ValidateClientOrderID 校验clientOrderId合规性
// 必须符合正则：^[\.A-Z\:/a-z0-9_-]{1,36}$
var clidRegex = regexp.MustCompile(`^[\.A-Z\:/a-z0-9_-]{1,36}$`)

func ValidateClientOrderID(clid string) error {
	if !clidRegex.MatchString(clid) {
		return fmt.Errorf("clientOrderId不合规: %s（必须符合^[\\.A-Z\\:/a-z0-9_-]{1,36}$）", clid)
	}
	return nil
}

// ValidateGridConfig 校验网格配置
func ValidateGridConfig(cfg *GridConfig) error {
	// step > 0
	if cfg.StepTicks <= 0 {
		return fmt.Errorf("step_ticks必须大于0")
	}

	// winMin < winMax
	if cfg.WinMinTicks >= cfg.WinMaxTicks {
		return fmt.Errorf("win_min_ticks必须小于win_max_ticks")
	}

	// (winMax-winMin) 必须是 step 的整数倍
	if (cfg.WinMaxTicks-cfg.WinMinTicks)%cfg.StepTicks != 0 {
		return fmt.Errorf("(win_max_ticks - win_min_ticks)必须是step_ticks的整数倍")
	}

	// 价格对齐（避免level漂移）
	if cfg.WinMinTicks%cfg.StepTicks != 0 {
		return fmt.Errorf("win_min_ticks必须是step_ticks的整数倍（对齐校验）")
	}
	if cfg.WinMaxTicks%cfg.StepTicks != 0 {
		return fmt.Errorf("win_max_ticks必须是step_ticks的整数倍（对齐校验）")
	}

	// tpKeepOutsideLevels >= 0
	if cfg.TPKeepOutsideLevels < 0 {
		return fmt.Errorf("tp_keep_outside_levels必须>=0")
	}

	// prefix合规
	testCLID := cfg.Prefix + ":E:0:0"
	if err := ValidateClientOrderID(testCLID); err != nil {
		return fmt.Errorf("prefix不合规（会导致CLID不合规）: %w", err)
	}

	return nil
}

// BuildEntryCLID 生成Entry的clientOrderId（带长度校验）
// 格式：{prefix}:E:{levelId}:{cycle}
// 返回错误如果生成的CLID超过36字符
func BuildEntryCLID(prefix string, levelID int, cycle int) (string, error) {
	clid := fmt.Sprintf("%s:E:%d:%d", prefix, levelID, cycle)
	if len(clid) > 36 {
		return "", fmt.Errorf("生成的CLID超过36字符: %s（长度%d）", clid, len(clid))
	}
	if err := ValidateClientOrderID(clid); err != nil {
		return "", err
	}
	return clid, nil
}

// BuildTPCLID 生成TP的clientOrderId（带长度校验）
// 格式：{prefix}:T:{levelId}:{cycle}
// 返回错误如果生成的CLID超过36字符
func BuildTPCLID(prefix string, levelID int, cycle int) (string, error) {
	clid := fmt.Sprintf("%s:T:%d:%d", prefix, levelID, cycle)
	if len(clid) > 36 {
		return "", fmt.Errorf("生成的CLID超过36字符: %s（长度%d）", clid, len(clid))
	}
	if err := ValidateClientOrderID(clid); err != nil {
		return "", err
	}
	return clid, nil
}

// ============= 7. 时间与数值辅助函数 =============

// NowMs 获取当前UTC毫秒时间戳
func NowMs() int64 {
	return time.Now().UnixMilli()
}

// CeilDiv 向上取整除法（用于窗口移动步数k计算）
// a,b > 0
func CeilDiv(a, b int64) int64 {
	if b == 0 {
		return 0
	}
	return (a + b - 1) / b
}
