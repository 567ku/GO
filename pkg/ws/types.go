package ws

import (
	"math/rand"
	"time"

	"gridbot/pkg/model"
)

// ============= 1. 连接状态（v1.3 冻结定义） =============

// ConnState 连接状态（WS 内部使用，比 model.WSState 更细粒度）
type ConnState string

const (
	ConnStateDisconnected ConnState = "DISCONNECTED" // 断开
	ConnStateConnecting   ConnState = "CONNECTING"   // 连接中
	ConnStateConnected    ConnState = "CONNECTED"    // 已连接
	ConnStateSubscribed   ConnState = "SUBSCRIBED"   // 已订阅（Market专用）
	ConnStateReady        ConnState = "READY"        // 就绪（UDS/Trade专用）
	ConnStateReconnecting ConnState = "RECONNECTING" // 重连中
)

// ============= 2. 连接配置（统一三条 WS 的共性参数） =============

// ConnConfig WS 连接配置
type ConnConfig struct {
	// 基础配置
	URL        string        // WebSocket URL
	PingPeriod time.Duration // Ping 间隔（0表示禁用客户端ping）

	// P0-WS-02: 超时口径收敛 - 只保留两类概念
	ReadWait  time.Duration // 读取超时（ReadDeadline续命窗口，在SetPongHandler和每次成功读消息后续期）
	WriteWait time.Duration // 写入超时

	// Deprecated: PongWait 已废弃，使用ReadWait作为唯一读超时窗口
	// 保留此字段仅为配置兼容，代码中不应再引用
	PongWait time.Duration // DEPRECATED: 请使用ReadWait

	// 重连配置
	ReconnectEnabled    bool          // 是否启用重连
	ReconnectInitDelay  time.Duration // 初始退避延迟
	ReconnectMaxBackoff time.Duration // 最大退避延迟（无最大重连次数限制，持续重连）

	// 健康检查（P0-WS-02：使用IdleTimeout作为业务层健康判定窗口）
	HealthCheckPeriod time.Duration // 健康检查周期
	IdleTimeout       time.Duration // 空闲超时（基于lastMessageAt，无消息判定为不健康）
}

// DefaultConnConfig 默认连接配置
// P0-WS-02: 超时口径收敛 - 确保自洽
// 约束：ReadWait > PingPeriod * 2 （避免边界抖动误判）
// 约束：IdleTimeout < ReadWait （健康检查先于读超时触发）
func DefaultConnConfig() ConnConfig {
	return ConnConfig{
		PingPeriod: 20 * time.Second, // Ping间隔
		WriteWait:  10 * time.Second, // 写超时
		ReadWait:   60 * time.Second, // 读超时（> PingPeriod*2 = 40s）

		// DEPRECATED: PongWait保留仅为兼容，实际不使用
		PongWait: 10 * time.Second, // 已废弃，不再使用

		ReconnectEnabled:    true,
		ReconnectInitDelay:  1 * time.Second,
		ReconnectMaxBackoff: 60 * time.Second,

		HealthCheckPeriod: 5 * time.Second,  // 健康检查周期
		IdleTimeout:       40 * time.Second, // 空闲超时（< ReadWait = 60s）
	}
}

// ============= 3. 事件辅助（发送给 Engine） =============

// NewWSStateEvent 创建 WS 状态事件（P0-2.3a：必须落地Reason字段）
// 参数：
//   - channel: WS通道类型（MARKET/UDS/TRADE）
//   - state: 状态（CONNECTED/DISCONNECTED/RECONNECTED）
//   - reason: 原因（CONNECTED/RECONNECTED可用固定字符串，DISCONNECTED必须是err.Error()或明确原因）
func NewWSStateEvent(channel model.WSChannel, state model.WSState, reason string) model.WSStateEvent {
	return model.WSStateEvent{
		Channel: channel,
		State:   state,
		AtMs:    model.NowMs(), // 阶段2收口：使用统一时间工具
		Reason:  reason,        // P0-2.3a：必须落地Reason字段
	}
}

// SendWSStateEvent 发送 WS 状态事件到 Engine
func SendWSStateEvent(eventCh chan<- model.EngineEvent, channel model.WSChannel, state model.WSState, reason string) {
	eventCh <- model.EngineEvent{
		Type: model.EventTypeWSState,
		Data: NewWSStateEvent(channel, state, reason),
	}
}

// ============= 4. 请求相关性（TradeWS 专用） =============

// PendingRequest 待处理请求
type PendingRequest struct {
	TaskID     string                // 关联的 Task ID
	ReqID      string                // 请求 ID（WS request id）
	CreatedAt  time.Time             // 创建时间
	DeadlineAt time.Time             // 超时时间
	RespChan   chan *TradeWSResponse // 响应通道
}

// TradeWSResponse TradeWS 响应
type TradeWSResponse struct {
	ReqID      string                 // 请求 ID
	OK         bool                   // 是否成功
	StatusCode int                    // HTTP 状态码（WS API 返回）
	ErrorCode  int                    // 错误码（失败时）
	ErrorMsg   string                 // 错误消息
	RawData    map[string]interface{} // 原始响应数据
	ReceivedAt time.Time              // 接收时间
}

// ============= 5. 健康状态（Manager 监控用） =============

// ConnHealth 连接健康状态
type ConnHealth struct {
	Channel          model.WSChannel // 通道类型
	State            ConnState       // 当前内部状态
	LastConnectedAt  time.Time       // 最后连接时间
	LastDisconnectAt time.Time       // 最后断开时间
	LastMessageAt    time.Time       // 最后消息时间
	LastPongAt       time.Time       // 最后 Pong 时间
	ReconnectCount   int             // 重连次数
	MessageCount     int64           // 消息计数
	ErrorCount       int             // 错误计数
	IsHealthy        bool            // 是否健康
}

// ============= 6. Manager 配置 =============

// ManagerConfig WS Manager 配置
type ManagerConfig struct {
	// 三条 WS 的 URL
	MarketURL string // 公共行情 WS URL
	UDSURL    string // UDS WS URL（需要 listenKey）
	TradeURL  string // Trade WS URL

	// P1-1: 禁止硬编码，必须从配置注入
	Symbol    string // 交易对（如 BTCUSDT）
	APIKey    string // API Key（Trade WS 专用）
	APISecret string // API Secret（Trade WS 专用）

	// Stage 4C: 精度注入（从 QtyConfig 注入到 WS 层）
	PricePrecision int // 价格精度（从 QtyConfig.PricePrecision 注入）
	QtyPrecision   int // 数量精度（从 QtyConfig.QtyPrecision 注入）
	QuotePrecision int // 金额精度（从 QtyConfig.QuotePrecision 注入）

	// P1-2: ReadLimit 防 OOM
	MaxMessageSize int64 // 最大消息大小（字节），0表示不限制，建议512KB-1MB

	// 连接配置（三条 WS 共享）
	ConnConfig ConnConfig

	// UDS 专用
	ListenKeyKeepAlivePeriod time.Duration // listenKey 续期周期（建议 25-30 分钟）

	// Trade 专用
	TradeRequestTimeout time.Duration // TradeWS 请求超时
}

// DefaultManagerConfig 默认 Manager 配置
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		ConnConfig:               DefaultConnConfig(),
		ListenKeyKeepAlivePeriod: 30 * time.Minute,
		TradeRequestTimeout:      2 * time.Second,
		MaxMessageSize:           512 * 1024, // P1-2: 默认512KB防OOM
	}
}

// ============= 7. 回调函数类型 =============

// OnStateChangeFunc WS 状态变化回调
type OnStateChangeFunc func(channel model.WSChannel, state model.WSState, reason string)

// OnMessageFunc WS 消息回调
type OnMessageFunc func(channel model.WSChannel, message []byte)

// OnErrorFunc WS 错误回调
type OnErrorFunc func(channel model.WSChannel, err error)

// ============= 8. 辅助函数 =============

// calculateBackoffWithJitter 计算带 jitter 的退避时间
// 公式: delay = min(base * 2^(attempt-1), max) * rand(0.8, 1.2)
// 防止多条 WS 同时重连造成共振
func calculateBackoffWithJitter(base, max time.Duration, attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	// 指数退避: base * 2^(attempt-1)
	backoff := base
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff > max {
			backoff = max
			break
		}
	}

	// 添加 jitter: rand(0.8, 1.2)
	jitter := 0.8 + rand.Float64()*0.4 // 0.8 ~ 1.2
	return time.Duration(float64(backoff) * jitter)
}
