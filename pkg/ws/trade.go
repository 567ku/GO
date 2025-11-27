package ws

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"gridbot/pkg/model"
)

// ============= Trade WS（私域下单 - Trade WebSocket API） =============
// 职责：
//   1. 执行 order.place/order.modify/order.cancel/query 的 WS 请求
//   2. 维护请求相关性（reqId -> pending）
//   3. 回包转换成 ExecutorResultEvent 发给 Engine
// 禁止：
//   - 在此做对账/缺口扫描/窗口移动等策略运算
//   - 直接修改 Level/OrderSlot/Profit/Cycle

// SignProvider 签名提供者接口
type SignProvider interface {
	Sign(params map[string]interface{}) string
}

// TradeClient Trade WS 客户端
type TradeClient struct {
	apiKey     string
	apiSecret  string
	url        string
	config     ConnConfig
	reqTimeout time.Duration

	// P1-2: ReadLimit 防 OOM
	maxMessageSize int64 // 最大消息大小（字节）

	// Engine 事件通道
	eventCh chan<- model.EngineEvent

	// 连接管理
	conn      *websocket.Conn
	connMu    sync.RWMutex
	connState ConnState

	// 请求相关性管理
	pendingMu   sync.RWMutex
	pendingReqs map[string]*PendingRequest // reqId -> PendingRequest

	// 写入队列（避免并发写）
	writeCh chan interface{}

	// 控制（P0-2.1：拆分两套WaitGroup）
	stopCh   chan struct{}
	stopOnce sync.Once      // P0-2.5：Stop幂等
	mainWG   sync.WaitGroup // 管理长期goroutine（connectionLoop, timeoutScanLoop）
	pumpWG   sync.WaitGroup // 管理单次连接周期的readPump/writePump

	// 健康状态（使用 atomic 防止竞争）
	lastMessageAtMs int64 // UnixMilli
	lastPongAtMs    int64 // UnixMilli

	// P0-2.4：重连计数与首次连接标记（并发安全）
	reconnectCount int64 // atomic：成功重连次数
	connectedOnce  int32 // atomic：是否曾经成功连接过（0=否，1=是）
}

// NewTradeClient 创建 Trade WS 客户端
func NewTradeClient(apiKey, apiSecret, url string, reqTimeout time.Duration, config ConnConfig, eventCh chan<- model.EngineEvent) *TradeClient {
	// P1-2: ReadLimit 默认512KB，防止OOM
	maxMessageSize := int64(512 * 1024)

	return &TradeClient{
		apiKey:         apiKey,
		apiSecret:      apiSecret,
		url:            url,
		config:         config,
		reqTimeout:     reqTimeout,
		maxMessageSize: maxMessageSize,
		eventCh:        eventCh,
		connState:      ConnStateDisconnected,
		pendingReqs:    make(map[string]*PendingRequest),
		writeCh:        make(chan interface{}, 100),
		stopCh:         make(chan struct{}),
	}
}

// Start 启动 Trade WS
func (c *TradeClient) Start() error {
	c.connMu.Lock()
	if c.connState != ConnStateDisconnected {
		c.connMu.Unlock()
		return fmt.Errorf("trade client已在运行")
	}
	c.connMu.Unlock()

	// P0-2.1：在mainWG中管理connectionLoop和timeoutScanLoop
	c.mainWG.Add(2)
	go c.connectionLoop()
	go c.timeoutScanLoop()

	return nil
}

// Stop 停止 Trade WS（P0-2.5：幂等实现）
func (c *TradeClient) Stop() error {
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
func (c *TradeClient) GetHealth() ConnHealth {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	return ConnHealth{
		Channel:        model.WSChannelTrade,
		State:          c.connState,
		LastMessageAt:  time.UnixMilli(atomic.LoadInt64(&c.lastMessageAtMs)),
		LastPongAt:     time.UnixMilli(atomic.LoadInt64(&c.lastPongAtMs)),
		ReconnectCount: int(atomic.LoadInt64(&c.reconnectCount)), // P0-2.4：atomic读取
		IsHealthy:      c.isHealthyLocked(),
	}
}

// connectionLoop 连接循环（处理重连）（P0-2.1：只管理mainWG）
func (c *TradeClient) connectionLoop() {
	defer c.mainWG.Done() // P0-2.1：从c.wg改为mainWG

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
			SendWSStateEvent(c.eventCh, model.WSChannelTrade, model.WSStateDisconnected, err.Error())

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
			SendWSStateEvent(c.eventCh, model.WSChannelTrade, model.WSStateConnected, "connected")
		} else {
			// 重连成功
			atomic.AddInt64(&c.reconnectCount, 1)
			SendWSStateEvent(c.eventCh, model.WSChannelTrade, model.WSStateReconnected, "reconnected")
		}

		// P0-2.1：进入读写循环（使用pumpWG）
		c.pumpWG.Add(2)
		go c.readPump()
		go c.writePump()
		c.pumpWG.Wait() // 只等待pumpWG，不等待mainWG（避免死锁）

		// 读写循环退出，连接已断开
		c.disconnect()
		c.setState(ConnStateDisconnected)
		SendWSStateEvent(c.eventCh, model.WSChannelTrade, model.WSStateDisconnected, "connection lost")

		// 清理所有待处理请求（超时）
		c.cleanupPendingRequests("connection lost")
	}
}

// connect 建立 WebSocket 连接（P0-2.2：统一使用ReadWait）
func (c *TradeClient) connect() error {
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
func (c *TradeClient) disconnect() {
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
func (c *TradeClient) readPump() {
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

// writePump 写入循环（单goroutine写入，避免并发）（P0-2.1：使用pumpWG）
func (c *TradeClient) writePump() {
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
		case msg := <-c.writeCh:
			// 写入消息
			if err := c.writeJSON(msg); err != nil {
				return
			}
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

// writeJSON 写入 JSON 消息
func (c *TradeClient) writeJSON(v interface{}) error {
	conn := c.getConn()
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	if c.config.WriteWait > 0 {
		conn.SetWriteDeadline(time.Now().Add(c.config.WriteWait))
	}

	return conn.WriteJSON(v)
}

// sendPing 发送 Ping
func (c *TradeClient) sendPing() error {
	conn := c.getConn()
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	if c.config.WriteWait > 0 {
		conn.SetWriteDeadline(time.Now().Add(c.config.WriteWait))
	}

	return conn.WriteMessage(websocket.PingMessage, nil)
}

// handleMessage 处理响应消息
func (c *TradeClient) handleMessage(message []byte) {
	var resp struct {
		ID     string                 `json:"id"`
		Status int                    `json:"status"`
		Result map[string]interface{} `json:"result"`
		Error  *struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"error"`
	}

	if err := json.Unmarshal(message, &resp); err != nil {
		return
	}

	// 查找待处理请求
	c.pendingMu.Lock()
	pending, ok := c.pendingReqs[resp.ID]
	if !ok {
		c.pendingMu.Unlock()
		return
	}
	delete(c.pendingReqs, resp.ID)
	c.pendingMu.Unlock()

	// 构造响应
	wsResp := &TradeWSResponse{
		ReqID:      resp.ID,
		StatusCode: resp.Status,
		RawData:    resp.Result,
		ReceivedAt: time.Now(),
	}

	if resp.Error != nil {
		wsResp.OK = false
		wsResp.ErrorCode = resp.Error.Code
		wsResp.ErrorMsg = resp.Error.Msg
	} else {
		wsResp.OK = (resp.Status >= 200 && resp.Status < 300)
	}

	// 阶段2收口：当前TradeClient将响应回灌到pending.RespChan（请求相关性）
	// TODO(phase3): 在Executor/Manager侧统一转换为ExecutorResultEvent并发给Engine
	// 转换逻辑参考：pkg/model/types.go:349-361 (ExecutorResultEvent定义)
	// 确保双保险闭环的数据源一致：TradeWS回包 + UDS订单更新
	// 发送到响应通道（不阻塞）
	select {
	case pending.RespChan <- wsResp:
	default:
	}
}

// timeoutScanLoop 超时扫描循环（P0-2.1：使用mainWG）
func (c *TradeClient) timeoutScanLoop() {
	defer c.mainWG.Done() // P0-2.1：从c.wg改为mainWG

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.scanTimeoutRequests()
		}
	}
}

// scanTimeoutRequests 扫描超时请求
func (c *TradeClient) scanTimeoutRequests() {
	now := time.Now()

	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for reqID, pending := range c.pendingReqs {
		if now.After(pending.DeadlineAt) {
			// 超时，发送超时响应
			select {
			case pending.RespChan <- &TradeWSResponse{
				ReqID:      reqID,
				OK:         false,
				ErrorCode:  -1,
				ErrorMsg:   "request timeout",
				ReceivedAt: now,
			}:
			default:
			}

			delete(c.pendingReqs, reqID)
		}
	}
}

// cleanupPendingRequests 清理所有待处理请求
func (c *TradeClient) cleanupPendingRequests(reason string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for reqID, pending := range c.pendingReqs {
		select {
		case pending.RespChan <- &TradeWSResponse{
			ReqID:      reqID,
			OK:         false,
			ErrorCode:  -1,
			ErrorMsg:   reason,
			ReceivedAt: time.Now(),
		}:
		default:
		}
	}

	c.pendingReqs = make(map[string]*PendingRequest)
}

// SendRequest 发送请求（通用方法）
func (c *TradeClient) SendRequest(taskID, reqID, method string, params map[string]interface{}) (*TradeWSResponse, error) {
	// 检查连接状态
	if !c.IsHealthy() {
		return nil, fmt.Errorf("trade ws not ready")
	}

	// 添加 apiKey 和 timestamp
	params["apiKey"] = c.apiKey
	params["timestamp"] = model.NowMs() // 阶段2收口：使用统一时间工具

	// 生成签名
	signature := c.sign(params)
	params["signature"] = signature

	// 构造请求
	req := map[string]interface{}{
		"id":     reqID,
		"method": method,
		"params": params,
	}

	// 创建响应通道
	respChan := make(chan *TradeWSResponse, 1)
	pending := &PendingRequest{
		TaskID:     taskID,
		ReqID:      reqID,
		CreatedAt:  time.Now(),
		DeadlineAt: time.Now().Add(c.reqTimeout),
		RespChan:   respChan,
	}

	// 注册到 pending map
	c.pendingMu.Lock()
	c.pendingReqs[reqID] = pending
	c.pendingMu.Unlock()

	// 发送请求（通过 writeCh 避免并发写）
	select {
	case c.writeCh <- req:
	case <-time.After(c.config.WriteWait):
		c.pendingMu.Lock()
		delete(c.pendingReqs, reqID)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("write queue full")
	}

	// 等待响应
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(c.reqTimeout):
		c.pendingMu.Lock()
		delete(c.pendingReqs, reqID)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request timeout")
	}
}

// sign 生成签名
func (c *TradeClient) sign(params map[string]interface{}) string {
	// 按字典序排序参数
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构造查询字符串
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fmt.Sprintf("%v", params[k]))
	}

	// HMAC SHA256
	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(sb.String()))
	return hex.EncodeToString(h.Sum(nil))
}

// IsHealthy 检查是否健康
func (c *TradeClient) IsHealthy() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.isHealthyLocked()
}

// isHealthyLocked 检查是否健康（需持锁）
func (c *TradeClient) isHealthyLocked() bool {
	if c.connState != ConnStateReady {
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
func (c *TradeClient) setState(state ConnState) {
	c.connMu.Lock()
	c.connState = state
	c.connMu.Unlock()
}

// getConn 获取连接
func (c *TradeClient) getConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}
