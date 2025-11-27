package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gridbot/config"
	"gridbot/core/model"
	"gridbot/internal/logger"
	"gridbot/internal/wsconn"
	"sort"
	"sync"
	"time"
)

// 订单WS专用错误
var (
	ErrOrderWSUnavailable = errors.New("order WS unavailable")
	ErrOrderWSTimeout     = errors.New("order WS request timeout")
)

// WsOrderClient WS 下单客户端（Trade WebSocket API）
type WsOrderClient struct {
	apiKey    string
	apiSecret string
	url       string
	wsConfig  config.WSConfig

	wsClient *wsconn.WSClient

	// 请求管理
	mu             sync.RWMutex
	pendingReqs    map[string]chan *wsOrderResponse // 待处理请求（key=id）
	requestTimeout time.Duration                    // 请求超时

	// 旧版兼容：下单上下文管理
	contexts map[string]*model.OrderContext
	ctxMu    sync.RWMutex
}

// wsOrderResponse WS订单响应
type wsOrderResponse struct {
	Status int                    `json:"status"` // HTTP状态码
	Result map[string]interface{} `json:"result"` // 成功响应
	Error  *wsOrderError          `json:"error"`  // 错误响应
}

// wsOrderError WS订单错误
type wsOrderError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// NewWsOrderClient 创建 WS 下单客户端
func NewWsOrderClient(apiKey, apiSecret, url string, wsConfig config.WSConfig) *WsOrderClient {
	timeout := time.Duration(wsConfig.OrderWSTimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = 2000 * time.Millisecond // 默认2秒
	}

	return &WsOrderClient{
		apiKey:         apiKey,
		apiSecret:      apiSecret,
		url:            url,
		wsConfig:       wsConfig,
		pendingReqs:    make(map[string]chan *wsOrderResponse),
		requestTimeout: timeout,
		contexts:       make(map[string]*model.OrderContext),
	}
}

func (c *WsOrderClient) Connect() error {
	// 创建 WSClient（OrderStream类型，激进探活）
	cfg := wsconn.WSConfig{
		PingInterval:        time.Duration(c.wsConfig.PingIntervalSec) * time.Second,
		PingIdleThreshold:   time.Duration(c.wsConfig.PingIdleThresholdSec) * time.Second,
		IdleTimeout:         time.Duration(c.wsConfig.IdleTimeoutSec) * time.Second,
		PongTimeout:         time.Duration(c.wsConfig.PongTimeoutSec) * time.Second,
		HealthCheckInterval: time.Duration(c.wsConfig.HealthCheckIntervalSec) * time.Second,
		ReconnectBaseDelay:  time.Duration(c.wsConfig.ReconnectBaseDelaySec) * time.Second,
		ReconnectMaxDelay:   time.Duration(c.wsConfig.ReconnectMaxDelaySec) * time.Second,
		ReconnectMaxRetries: c.wsConfig.ReconnectMaxRetries,
	}
	// OrderStream使用激进策略，如果配置为0则使用默认值
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 10 * time.Second
	}
	if cfg.PingIdleThreshold == 0 {
		cfg.PingIdleThreshold = 25 * time.Second
	}
	if cfg.PongTimeout == 0 {
		cfg.PongTimeout = 8 * time.Second
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 40 * time.Second
	}
	c.wsClient = wsconn.NewOrderStreamClient(c.url, cfg)

	// 设置回调
	c.wsClient.OnOpen = func() {
		logger.Info("[OrderWS] WebSocket已连接，使用order.place+HMAC方式下单 (ws-fapi/v1)")
	}

	c.wsClient.OnClose = func(err error) {
		logger.Info("[OrderWS] WebSocket连接已断开: %v", err)
	}

	c.wsClient.OnMessage = func(message []byte) {
		c.handleMessage(message)
	}

	c.wsClient.OnReconnected = func() {
		logger.Info("[OrderWS] WebSocket重连成功")
	}

	// 建立连接
	return c.wsClient.Connect()
}

// IsAvailable 检查订单WS是否可用（仅依赖连接状态+心跳健康）
func (c *WsOrderClient) IsAvailable() bool {
	if c.wsClient == nil {
		return false
	}
	return c.wsClient.IsHealthy()
}

// IsHealthy 检查是否健康（向后兼容）
func (c *WsOrderClient) IsHealthy() bool {
	return c.IsAvailable()
}

// PlaceOrderWS 通过WS下单（按官方文档：每次请求携带apiKey+HMAC签名）
func (c *WsOrderClient) PlaceOrderWS(ctx context.Context, params model.PlaceOrderParams) (*model.OrderResult, error) {
	if !c.IsAvailable() {
		return nil, ErrOrderWSUnavailable
	}

	timestamp := time.Now().UnixMilli()
	reqID := params.ClientOrderID

	// 构建下单参数（必须包含apiKey）
	reqParams := map[string]interface{}{
		"apiKey":           c.apiKey, // 必须字段：每次请求都要带apiKey
		"symbol":           params.Symbol,
		"side":             params.Side,
		"type":             params.Type,
		"quantity":         fmt.Sprintf("%.4f", params.Quantity),
		"newClientOrderId": params.ClientOrderID,
		"recvWindow":       5000,
		"timestamp":        timestamp,
	}

	// 添加价格（LIMIT单需要）
	if params.Type == "LIMIT" {
		reqParams["price"] = fmt.Sprintf("%.2f", params.Price)
		reqParams["timeInForce"] = params.TimeInForce
	}

	// 添加positionSide（期货必须）
	if params.PositionSide != "" {
		reqParams["positionSide"] = params.PositionSide
	}

	// 生成签名（签名包含apiKey在内的所有参数）
	signature := c.sign(reqParams)
	reqParams["signature"] = signature

	// 构建请求
	req := map[string]interface{}{
		"id":     reqID,
		"method": "order.place",
		"params": reqParams,
	}

	// 准备响应通道
	respChan := make(chan *wsOrderResponse, 1)
	c.mu.Lock()
	c.pendingReqs[reqID] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingReqs, reqID)
		c.mu.Unlock()
	}()

	// 发送请求
	if err := c.wsClient.SendJSON(req); err != nil {
		logger.Error("[OrderWS] 发送下单请求失败: %v", err)
		return nil, fmt.Errorf("send order request failed: %w", err)
	}

	logger.Info("[OrderWS] 已发送下单请求: %s, side=%s, price=%.2f, qty=%.4f",
		params.ClientOrderID, params.Side, params.Price, params.Quantity)

	// 等待响应
	select {
	case resp := <-respChan:
		if resp.Status == 200 && resp.Result != nil {
			// 解析成功响应
			result := &model.OrderResult{
				ClientOrderID: params.ClientOrderID,
			}

			if orderID, ok := resp.Result["orderId"].(float64); ok {
				result.OrderID = int64(orderID)
			}
			if status, ok := resp.Result["status"].(string); ok {
				result.Status = status
			}
			if execQty, ok := resp.Result["executedQty"].(string); ok {
				fmt.Sscanf(execQty, "%f", &result.ExecutedQty)
			}

			logger.Info("[OrderWS] WS下单成功: symbol=%s, side=%s, qty=%.4f, price=%.2f, orderId=%d",
				params.Symbol, params.Side, params.Quantity, params.Price, result.OrderID)

			return result, nil
		}

		// 处理错误响应
		if resp.Error != nil {
			logger.Error("[OrderWS] WS下单失败: code=%d, msg=%s -> fallback to REST",
				resp.Error.Code, resp.Error.Msg)
			return nil, fmt.Errorf("order rejected: code=%d, msg=%s", resp.Error.Code, resp.Error.Msg)
		}

		return nil, fmt.Errorf("invalid response: status=%d", resp.Status)

	case <-time.After(c.requestTimeout):
		logger.Error("[OrderWS] 下单请求超时: %s", params.ClientOrderID)
		return nil, ErrOrderWSTimeout

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendOrder 发送下单请求（向后兼容旧接口）
func (c *WsOrderClient) SendOrder(params model.PlaceOrderParams, ctx *model.OrderContext) error {
	if !c.IsAvailable() {
		return fmt.Errorf("WS 订单连接未建立或不健康")
	}

	// 注册上下文
	c.registerContext(ctx)

	// 使用新的PlaceOrderWS接口（忽略上下文超时）
	_, err := c.PlaceOrderWS(context.Background(), params)
	if err != nil {
		c.unregisterContext(ctx.ClientOrderID)
		return err
	}

	return nil
}

// handleMessage 处理WS消息
func (c *WsOrderClient) handleMessage(message []byte) {
	var msg struct {
		ID     string                 `json:"id"`
		Status int                    `json:"status"`
		Result map[string]interface{} `json:"result"`
		Error  *wsOrderError          `json:"error"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		logger.Error("[OrderWS] 解析消息失败: %v, msg=%s", err, string(message))
		return
	}

	// 查找对应的待处理请求
	c.mu.RLock()
	respChan, exists := c.pendingReqs[msg.ID]
	c.mu.RUnlock()

	if !exists {
		// 可能是旧版兼容逻辑或其他消息
		logger.Debug("[OrderWS] 收到未匹配的消息: id=%s", msg.ID)
		return
	}

	// 发送响应
	resp := &wsOrderResponse{
		Status: msg.Status,
		Result: msg.Result,
		Error:  msg.Error,
	}

	select {
	case respChan <- resp:
	default:
		logger.Error("[OrderWS] 响应通道已满: id=%s", msg.ID)
	}
}

// sign 生成签名
func (c *WsOrderClient) sign(params map[string]interface{}) string {
	// 按字母顺序排列参数
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建查询字符串
	query := ""
	for _, k := range keys {
		if query != "" {
			query += "&"
		}
		query += fmt.Sprintf("%s=%v", k, params[k])
	}

	// HMAC-SHA256
	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(query))
	return hex.EncodeToString(h.Sum(nil))
}

// registerContext 注册订单上下文（向后兼容）
func (c *WsOrderClient) registerContext(ctx *model.OrderContext) {
	c.ctxMu.Lock()
	defer c.ctxMu.Unlock()
	c.contexts[ctx.ClientOrderID] = ctx
}

// unregisterContext 注销订单上下文（向后兼容）
func (c *WsOrderClient) unregisterContext(clientOrderID string) {
	c.ctxMu.Lock()
	defer c.ctxMu.Unlock()
	delete(c.contexts, clientOrderID)
}

// getContext 获取订单上下文（向后兼容）
func (c *WsOrderClient) getContext(clientOrderID string) (*model.OrderContext, bool) {
	c.ctxMu.RLock()
	defer c.ctxMu.RUnlock()
	ctx, exists := c.contexts[clientOrderID]
	return ctx, exists
}

// Close 关闭WS连接
func (c *WsOrderClient) Close() error {
	if c.wsClient != nil {
		return c.wsClient.Close()
	}
	return nil
}
