package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"gridbot/pkg/model"
)

// ============= UDS WS（私域推送 - User Data Stream） =============
// 职责：
//   1. 管理 listenKey 生命周期（获取、续期、失效重建）
//   2. 订阅订单更新/成交更新
//   3. 解析成 OrderUpdateEvent/TradeUpdateEvent 发给 Engine
// 禁止：
//   - 在此触发重连后的对账决策（只发事件给 Engine）
//   - 在此做任何网格状态修改

// ListenKeyProvider listenKey 提供者接口
type ListenKeyProvider interface {
	GetListenKey() (string, error)
	KeepAliveListenKey(listenKey string) error
}

// UDSClient UDS WS 客户端
type UDSClient struct {
	baseURL           string // WebSocket 基础 URL
	listenKeyProvider ListenKeyProvider
	config            ConnConfig
	keepAlivePeriod   time.Duration // listenKey 续期周期

	// P1-2: ReadLimit 防 OOM
	maxMessageSize int64 // 最大消息大小（字节）

	// Stage 4C: 精度注入
	pricePrecision int // 价格精度（从QtyConfig注入）
	qtyPrecision   int // 数量精度（从QtyConfig注入）
	quotePrecision int // 金额精度（从QtyConfig注入）

	// Engine 事件通道
	eventCh chan<- model.EngineEvent

	// 连接管理
	conn        *websocket.Conn
	connMu      sync.RWMutex
	connState   ConnState
	listenKey   string
	listenKeyMu sync.RWMutex

	// 控制（P0-2.1：拆分两套WaitGroup）
	stopCh   chan struct{}
	stopOnce sync.Once      // P0-2.5：Stop幂等
	mainWG   sync.WaitGroup // 管理长期goroutine（connectionLoop, keepAliveLoop）
	pumpWG   sync.WaitGroup // 管理单次连接周期的readPump/writePump

	// 健康状态（使用 atomic 防止竞争）
	lastMessageAtMs int64 // UnixMilli
	lastPongAtMs    int64 // UnixMilli

	// P0-2.4：重连计数与首次连接标记（并发安全）
	reconnectCount int64 // atomic：成功重连次数
	connectedOnce  int32 // atomic：是否曾经成功连接过（0=否，1=是）
}

// NewUDSClient 创建 UDS WS 客户端
// Stage 4C: 添加pricePrecision/qtyPrecision/quotePrecision参数（从QtyConfig注入）
func NewUDSClient(baseURL string, provider ListenKeyProvider, keepAlivePeriod time.Duration, config ConnConfig, pricePrecision, qtyPrecision, quotePrecision int, eventCh chan<- model.EngineEvent) *UDSClient {
	// P1-2: ReadLimit 默认512KB，防止OOM
	maxMessageSize := int64(512 * 1024)

	return &UDSClient{
		baseURL:           baseURL,
		listenKeyProvider: provider,
		config:            config,
		keepAlivePeriod:   keepAlivePeriod,
		maxMessageSize:    maxMessageSize,
		pricePrecision:    pricePrecision, // Stage 4C: 注入精度
		qtyPrecision:      qtyPrecision,   // Stage 4C: 注入精度
		quotePrecision:    quotePrecision, // Stage 4C: 注入精度
		eventCh:           eventCh,
		connState:         ConnStateDisconnected,
		stopCh:            make(chan struct{}),
	}
}

// Start 启动 UDS WS
func (c *UDSClient) Start() error {
	c.connMu.Lock()
	if c.connState != ConnStateDisconnected {
		c.connMu.Unlock()
		return fmt.Errorf("uds client已在运行")
	}
	c.connMu.Unlock()

	// P0-2.1：在mainWG中管理connectionLoop和keepAliveLoop
	c.mainWG.Add(2)
	go c.connectionLoop()
	go c.keepAliveLoop()

	return nil
}

// Stop 停止 UDS WS（P0-2.5：幂等实现）
func (c *UDSClient) Stop() error {
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
func (c *UDSClient) GetHealth() ConnHealth {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	return ConnHealth{
		Channel:        model.WSChannelUDS,
		State:          c.connState,
		LastMessageAt:  time.UnixMilli(atomic.LoadInt64(&c.lastMessageAtMs)),
		LastPongAt:     time.UnixMilli(atomic.LoadInt64(&c.lastPongAtMs)),
		ReconnectCount: int(atomic.LoadInt64(&c.reconnectCount)), // P0-2.4：atomic读取
		IsHealthy:      c.isHealthyLocked(),
	}
}

// connectionLoop 连接循环（处理重连）（P0-2.1：只管理mainWG）
func (c *UDSClient) connectionLoop() {
	defer c.mainWG.Done() // P0-2.1：从c.wg改为mainWG

	attempt := 0

	for {
		select {
		case <-c.stopCh:
			c.disconnect()
			return
		default:
		}

		// 获取/更新 listenKey
		if err := c.refreshListenKey(); err != nil {
			// listenKey 获取失败，等待重试
			attempt++
			backoff := calculateBackoffWithJitter(c.config.ReconnectInitDelay, c.config.ReconnectMaxBackoff, attempt)
			select {
			case <-c.stopCh:
				return
			case <-time.After(backoff):
			}
			continue
		}

		// 连接
		c.setState(ConnStateConnecting)
		if err := c.connect(); err != nil {
			c.setState(ConnStateDisconnected)
			SendWSStateEvent(c.eventCh, model.WSChannelUDS, model.WSStateDisconnected, err.Error())

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
		c.setState(ConnStateReady)

		// P0-2.4：判断是首次连接还是重连
		isFirstConnection := atomic.CompareAndSwapInt32(&c.connectedOnce, 0, 1)
		if isFirstConnection {
			SendWSStateEvent(c.eventCh, model.WSChannelUDS, model.WSStateConnected, "connected")
		} else {
			// 重连成功
			atomic.AddInt64(&c.reconnectCount, 1)
			SendWSStateEvent(c.eventCh, model.WSChannelUDS, model.WSStateReconnected, "reconnected")
		}

		// P0-2.1：进入读写循环（使用pumpWG）
		c.pumpWG.Add(2)
		go c.readPump()
		go c.writePump()
		c.pumpWG.Wait() // 只等待pumpWG，不等待mainWG（避免死锁）

		// 读写循环退出，连接已断开
		c.disconnect()
		c.setState(ConnStateDisconnected)
		SendWSStateEvent(c.eventCh, model.WSChannelUDS, model.WSStateDisconnected, "connection lost")
	}
}

// refreshListenKey 刷新 listenKey
func (c *UDSClient) refreshListenKey() error {
	c.listenKeyMu.Lock()
	defer c.listenKeyMu.Unlock()

	// 获取新的 listenKey
	key, err := c.listenKeyProvider.GetListenKey()
	if err != nil {
		return fmt.Errorf("get listenKey failed: %w", err)
	}

	c.listenKey = key
	return nil
}

// connect 建立 WebSocket 连接（P0-2.2：统一使用ReadWait）
func (c *UDSClient) connect() error {
	c.listenKeyMu.RLock()
	key := c.listenKey
	c.listenKeyMu.RUnlock()

	if key == "" {
		return fmt.Errorf("listenKey is empty")
	}

	url := c.baseURL + "/ws/" + key

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, nil)
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
func (c *UDSClient) disconnect() {
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
func (c *UDSClient) readPump() {
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
func (c *UDSClient) writePump() {
	defer c.pumpWG.Done() // P0-2.1：从c.wg改为pumpWG

	conn := c.getConn()
	if conn == nil {
		return
	}

	pingTicker := time.NewTicker(c.config.PingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-pingTicker.C:
			// 发送 Ping
			if err := c.sendPing(); err != nil {
				return
			}

			// 健康检查
			if !c.IsHealthy() {
				// 触发重连
				c.disconnect()
				return
			}
		}
	}
}

// sendPing 发送 Ping
func (c *UDSClient) sendPing() error {
	conn := c.getConn()
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	if c.config.WriteWait > 0 {
		conn.SetWriteDeadline(time.Now().Add(c.config.WriteWait))
	}

	return conn.WriteMessage(websocket.PingMessage, nil)
}

// keepAliveLoop listenKey 续期循环（P0-2.1：使用mainWG）
func (c *UDSClient) keepAliveLoop() {
	defer c.mainWG.Done() // P0-2.1：从c.wg改为mainWG

	ticker := time.NewTicker(c.keepAlivePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.listenKeyMu.RLock()
			key := c.listenKey
			c.listenKeyMu.RUnlock()

			if key == "" {
				continue
			}

			// 续期 listenKey
			if err := c.listenKeyProvider.KeepAliveListenKey(key); err != nil {
				// 续期失败，触发重新获取（通过断开连接触发重连）
				c.disconnect()
			}
		}
	}
}

// handleMessage 处理消息
func (c *UDSClient) handleMessage(message []byte) {
	var baseMsg struct {
		EventType string `json:"e"` // 事件类型
	}

	if err := json.Unmarshal(message, &baseMsg); err != nil {
		return
	}

	switch baseMsg.EventType {
	case "ORDER_TRADE_UPDATE":
		c.handleOrderUpdate(message)
	case "ACCOUNT_UPDATE":
		// 账户更新事件（暂不处理）
	default:
		// 未知事件类型
	}
}

// handleOrderUpdate 处理订单更新
func (c *UDSClient) handleOrderUpdate(message []byte) {
	var msg struct {
		EventType string `json:"e"`
		EventTime int64  `json:"E"`
		OrderData struct {
			Symbol               string `json:"s"`
			ClientOrderID        string `json:"c"`
			Side                 string `json:"S"`
			OrderType            string `json:"o"`
			TimeInForce          string `json:"f"`
			OrigQty              string `json:"q"`
			Price                string `json:"p"`
			AvgPrice             string `json:"ap"`
			StopPrice            string `json:"sp"`
			ExecutionType        string `json:"x"`
			OrderStatus          string `json:"X"`
			OrderID              int64  `json:"i"`
			LastFilledQty        string `json:"l"`
			AccumulatedFilledQty string `json:"z"`
			LastFilledPrice      string `json:"L"`
			CommissionAsset      string `json:"N"`
			Commission           string `json:"n"`
			OrderTradeTime       int64  `json:"T"`
			TradeID              int64  `json:"t"`
			BidsNotional         string `json:"b"`
			AskNotional          string `json:"a"`
			IsMaker              bool   `json:"m"`
			IsReduceOnly         bool   `json:"R"`
			WorkingType          string `json:"wt"`
			OrigOrderType        string `json:"ot"`
			PositionSide         string `json:"ps"`
			IsClosePosition      bool   `json:"cp"`
			ActivationPrice      string `json:"AP"`
			CallbackRate         string `json:"cr"`
			RealizedProfit       string `json:"rp"`
		} `json:"o"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	orderData := msg.OrderData

	// Stage 4C: 精度注入完成 - 使用注入的pricePrecision/qtyPrecision（移除硬编码）
	priceTicks, err := model.ParsePriceDecimal(orderData.Price, c.pricePrecision)
	if err != nil {
		priceTicks = 0 // 解析失败使用0
	}

	origQtyTicks, err := model.ParseQtyDecimal(orderData.OrigQty, c.qtyPrecision)
	if err != nil {
		origQtyTicks = 0
	}

	executedQtyTicks, err := model.ParseQtyDecimal(orderData.AccumulatedFilledQty, c.qtyPrecision)
	if err != nil {
		executedQtyTicks = 0
	}

	avgPriceTicks, err := model.ParsePriceDecimal(orderData.AvgPrice, c.pricePrecision)
	if err != nil {
		avgPriceTicks = 0
	}

	// 发送 OrderUpdateEvent
	c.eventCh <- model.EngineEvent{
		Type: model.EventTypeOrderUpdate,
		Data: model.OrderUpdateEvent{
			Symbol:           orderData.Symbol,
			ClientOrderID:    orderData.ClientOrderID,
			OrderID:          orderData.OrderID,
			Status:           orderData.OrderStatus,
			Side:             orderData.Side,
			PositionSide:     orderData.PositionSide,
			PriceTicks:       priceTicks,
			OrigQtyTicks:     origQtyTicks,
			ExecutedQtyTicks: executedQtyTicks,
			AvgPriceTicks:    avgPriceTicks,
			UpdateAtMs:       msg.EventTime,
		},
	}

	// 如果有成交，发送 TradeUpdateEvent
	if orderData.ExecutionType == "TRADE" {
		var realizedPnlTicks *int64
		if orderData.RealizedProfit != "" && orderData.RealizedProfit != "0" {
			// Stage 4C: 使用quotePrecision解析PNL（金额类数据）
			pnl, err := model.ParsePriceDecimal(orderData.RealizedProfit, c.quotePrecision)
			if err == nil {
				realizedPnlTicks = &pnl
			}
		}

		lastQtyTicks, err := model.ParseQtyDecimal(orderData.LastFilledQty, c.qtyPrecision)
		if err != nil {
			lastQtyTicks = 0
		}

		lastPriceTicks, err := model.ParsePriceDecimal(orderData.LastFilledPrice, c.pricePrecision)
		if err != nil {
			lastPriceTicks = 0
		}

		c.eventCh <- model.EngineEvent{
			Type: model.EventTypeTradeUpdate,
			Data: model.TradeUpdateEvent{
				Symbol:           orderData.Symbol,
				ClientOrderID:    orderData.ClientOrderID,
				OrderID:          orderData.OrderID,
				QtyTicks:         lastQtyTicks,
				PriceTicks:       lastPriceTicks,
				TradeTimeMs:      orderData.OrderTradeTime,
				RealizedPnlTicks: realizedPnlTicks,
			},
		}
	}
}

// IsHealthy 检查是否健康
func (c *UDSClient) IsHealthy() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.isHealthyLocked()
}

// isHealthyLocked 检查是否健康（需持锁）
// P0-WS-02: 只使用IdleTimeout作为业务层健康判定窗口，不再使用PongWait
func (c *UDSClient) isHealthyLocked() bool {
	if c.connState != ConnStateReady {
		return false
	}

	// P0-WS-02: 统一使用IdleTimeout判断空闲超时（基于lastMessageAt）
	if c.config.IdleTimeout > 0 {
		lastMsgMs := atomic.LoadInt64(&c.lastMessageAtMs)
		if time.Since(time.UnixMilli(lastMsgMs)) > c.config.IdleTimeout {
			return false
		}
	}

	return true
}

// setState 设置连接状态
func (c *UDSClient) setState(state ConnState) {
	c.connMu.Lock()
	c.connState = state
	c.connMu.Unlock()
}

// getConn 获取连接
func (c *UDSClient) getConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}
