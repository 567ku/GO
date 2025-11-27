package ws

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"gridbot/pkg/model"
)

// ============= Market WS（公共行情） =============
// 职责：
//   1. 订阅 ticker/price 推送
//   2. 解析成 PriceTickEvent 发给 Engine
//   3. 允许 tick 合并（只保留最新价格）
// 禁止：
//   - 在此做任何交易逻辑、下单、改状态

// MarketClient Market WS 客户端
type MarketClient struct {
	symbol string // 交易对
	url    string // WebSocket URL
	config ConnConfig

	// P1-2: ReadLimit 防 OOM
	maxMessageSize int64 // 最大消息大小（字节）

	// Stage 4C: 精度注入
	pricePrecision int // 价格精度（从QtyConfig注入）

	// Engine 事件通道
	eventCh chan<- model.EngineEvent

	// 连接管理
	conn      *websocket.Conn
	connMu    sync.RWMutex
	connState ConnState

	// 控制（P0-2.1：拆分两套WaitGroup）
	stopCh   chan struct{}
	stopOnce sync.Once      // P0-2.5：Stop幂等
	mainWG   sync.WaitGroup // 管理长期goroutine（connectionLoop）
	pumpWG   sync.WaitGroup // 管理单次连接周期的readPump/writePump

	// 健康状态（使用 atomic 防止竞争）
	lastMessageAtMs int64 // UnixMilli
	lastPongAtMs    int64 // UnixMilli

	// P0-2.4：重连计数与首次连接标记（并发安全）
	reconnectCount int64 // atomic：成功重连次数
	connectedOnce  int32 // atomic：是否曾经成功连接过（0=否，1=是）
}

// NewMarketClient 创建 Market WS 客户端
// Stage 4C: 添加pricePrecision参数（从QtyConfig注入）
func NewMarketClient(symbol, baseURL string, config ConnConfig, pricePrecision int, eventCh chan<- model.EngineEvent) *MarketClient {
	streamName := strings.ToLower(symbol) + "@ticker"
	url := baseURL + "/ws/" + streamName

	// P1-2: ReadLimit 默认512KB，防止OOM
	maxMessageSize := int64(512 * 1024)
	if config.ReadWait > 0 {
		// 如果有自定义配置，可以后续扩展
		maxMessageSize = 512 * 1024
	}

	return &MarketClient{
		symbol:         symbol,
		url:            url,
		config:         config,
		maxMessageSize: maxMessageSize,
		pricePrecision: pricePrecision, // Stage 4C: 注入精度
		eventCh:        eventCh,
		connState:      ConnStateDisconnected,
		stopCh:         make(chan struct{}),
	}
}

// Start 启动 Market WS
func (c *MarketClient) Start() error {
	c.connMu.Lock()
	if c.connState != ConnStateDisconnected {
		c.connMu.Unlock()
		return fmt.Errorf("market client已在运行")
	}
	c.connMu.Unlock()

	// P0-2.1：只在mainWG中管理connectionLoop
	c.mainWG.Add(1)
	go c.connectionLoop()

	return nil
}

// Stop 停止 Market WS（P0-2.5：幂等实现）
func (c *MarketClient) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		// 主动断开连接，解除ReadMessage阻塞
		c.disconnect()
	})
	// 等待mainWG（只管理长期goroutine）
	c.mainWG.Wait()
	return nil
}

// GetHealth 获取健康状态
func (c *MarketClient) GetHealth() ConnHealth {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	return ConnHealth{
		Channel:        model.WSChannelMarket,
		State:          c.connState,
		LastMessageAt:  time.UnixMilli(atomic.LoadInt64(&c.lastMessageAtMs)),
		LastPongAt:     time.UnixMilli(atomic.LoadInt64(&c.lastPongAtMs)),
		ReconnectCount: int(atomic.LoadInt64(&c.reconnectCount)), // P0-2.4：atomic读取
		IsHealthy:      c.isHealthyLocked(),
	}
}

// connectionLoop 连接循环（处理重连）（P0-2.1：只管理mainWG）
func (c *MarketClient) connectionLoop() {
	defer c.mainWG.Done() // P0-2.1：从原c.wg改为mainWG

	attempt := 0

	for {
		select {
		case <-c.stopCh:
			c.disconnect()
			return
		default:
		}

		// 连接
		c.setState(ConnStateConnecting)
		if err := c.connect(); err != nil {
			c.setState(ConnStateDisconnected)
			SendWSStateEvent(c.eventCh, model.WSChannelMarket, model.WSStateDisconnected, err.Error())

			// 计算带 jitter 的退避时间（防止共振）
			attempt++
			backoff := calculateBackoffWithJitter(c.config.ReconnectInitDelay, c.config.ReconnectMaxBackoff, attempt)

			// 退避等待
			select {
			case <-c.stopCh:
				return
			case <-time.After(backoff):
			}
			continue
		}

		// 连接成功，重置退避
		attempt = 0

		// P0-2.4：判断是首次连接还是重连
		isFirstConnection := atomic.CompareAndSwapInt32(&c.connectedOnce, 0, 1)
		if isFirstConnection {
			c.setState(ConnStateConnected)
			SendWSStateEvent(c.eventCh, model.WSChannelMarket, model.WSStateConnected, "connected")
		} else {
			// 重连成功
			atomic.AddInt64(&c.reconnectCount, 1)
			c.setState(ConnStateConnected)
			SendWSStateEvent(c.eventCh, model.WSChannelMarket, model.WSStateReconnected, "reconnected")
		}

		// P0-2.1：进入读写循环（使用pumpWG）
		c.pumpWG.Add(2)
		go c.readPump()
		go c.writePump()
		c.pumpWG.Wait() // 只等待pumpWG，不等待mainWG（避免死锁）

		// 读写循环退出，连接已断开
		c.disconnect()
		c.setState(ConnStateDisconnected)
		SendWSStateEvent(c.eventCh, model.WSChannelMarket, model.WSStateDisconnected, "connection lost")
	}
}

// connect 建立 WebSocket 连接（P0-2.2：统一使用ReadWait）
func (c *MarketClient) connect() error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	// P1-2: 设置ReadLimit防OOM（必须在读取消息之前设置）
	if c.maxMessageSize > 0 {
		conn.SetReadLimit(c.maxMessageSize)
	}

	// 初始化时间戳
	now := time.Now()
	atomic.StoreInt64(&c.lastMessageAtMs, now.UnixMilli())
	atomic.StoreInt64(&c.lastPongAtMs, now.UnixMilli())

	// P0-2.2：设置初始ReadDeadline（只使用ReadWait）
	if c.config.ReadWait > 0 {
		conn.SetReadDeadline(now.Add(c.config.ReadWait))
	}

	// P0-2.2：设置Pong Handler（统一使用ReadWait）
	conn.SetPongHandler(func(string) error {
		// 1. atomic 更新 lastPongAt
		atomic.StoreInt64(&c.lastPongAtMs, model.NowMs())

		// 2. 立即设置 ReadDeadline（P0-2.2：使用ReadWait而非PongWait）
		if c.config.ReadWait > 0 {
			conn.SetReadDeadline(time.Now().Add(c.config.ReadWait))
		}
		return nil
	})

	return nil
}

// disconnect 断开连接
func (c *MarketClient) disconnect() {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		// 设置deadline解除ReadMessage阻塞
		c.conn.SetReadDeadline(time.Now())
		c.conn.SetWriteDeadline(time.Now())
		c.conn.Close()
		c.conn = nil
	}
}

// readPump 读取循环（P0-2.1：使用pumpWG）
func (c *MarketClient) readPump() {
	defer c.pumpWG.Done() // P0-2.1：从c.wg改为pumpWG

	conn := c.getConn()
	if conn == nil {
		return
	}

	// P0-2.2：设置读取超时（统一使用ReadWait）
	if c.config.ReadWait > 0 {
		conn.SetReadDeadline(time.Now().Add(c.config.ReadWait))
	}

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			return
		}

		// 更新最后消息时间（atomic）
		atomic.StoreInt64(&c.lastMessageAtMs, model.NowMs())

		// P0-2.2：重置读取超时（统一使用ReadWait）
		if c.config.ReadWait > 0 {
			conn.SetReadDeadline(time.Now().Add(c.config.ReadWait))
		}

		// 处理消息
		c.handleMessage(message)
	}
}

// writePump 写入循环（发送 Ping）（P0-2.1：使用pumpWG）
func (c *MarketClient) writePump() {
	defer c.pumpWG.Done() // P0-2.1：从c.wg改为pumpWG

	// Market WS 不主动发 ping（使用服务端 ping）
	// 仅保留此循环以便将来扩展
	ticker := time.NewTicker(c.config.HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			// 健康检查（仅记录，不发送 ping）
			if !c.IsHealthy() {
				// 触发重连
				c.disconnect()
				return
			}
		}
	}
}

// handleMessage 处理消息
func (c *MarketClient) handleMessage(message []byte) {
	var tickerMsg struct {
		EventType string `json:"e"` // 事件类型
		EventTime int64  `json:"E"` // 事件时间
		Symbol    string `json:"s"` // 交易对
		LastPrice string `json:"c"` // 最新成交价
	}

	if err := json.Unmarshal(message, &tickerMsg); err != nil {
		return
	}

	// 只处理 24hrTicker 事件
	if tickerMsg.EventType != "24hrTicker" {
		return
	}

	// Stage 4C: 精度注入完成 - 使用注入的pricePrecision（移除硬编码）
	priceTicks, err := model.ParsePriceDecimal(tickerMsg.LastPrice, c.pricePrecision)
	if err != nil {
		// 解析失败，忽略此消息
		return
	}

	// 发送 PriceTickEvent 到 Engine
	c.eventCh <- model.EngineEvent{
		Type: model.EventTypePriceTick,
		Data: model.PriceTickEvent{
			Symbol:     tickerMsg.Symbol,
			PriceTicks: priceTicks,
			EventAtMs:  tickerMsg.EventTime,
		},
	}
}

// IsHealthy 检查是否健康
func (c *MarketClient) IsHealthy() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.isHealthyLocked()
}

// isHealthyLocked 检查是否健康（需持锁）
func (c *MarketClient) isHealthyLocked() bool {
	if c.connState != ConnStateConnected {
		return false
	}

	// 检查是否超过空闲超时
	if c.config.IdleTimeout > 0 {
		lastMsgMs := atomic.LoadInt64(&c.lastMessageAtMs)
		if time.Since(time.UnixMilli(lastMsgMs)) > c.config.IdleTimeout {
			return false
		}
	}

	return true
}

// setState 设置连接状态
func (c *MarketClient) setState(state ConnState) {
	c.connMu.Lock()
	c.connState = state
	c.connMu.Unlock()
}

// getConn 获取连接
func (c *MarketClient) getConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}
