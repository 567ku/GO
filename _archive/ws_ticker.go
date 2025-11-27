package binance

import (
	"encoding/json"
	"fmt"
	"gridbot/config"
	"gridbot/core/model"
	"gridbot/internal/logger"
	"gridbot/internal/wsconn"
	"strings"
	"time"
)

// TickerWebSocket 公共Ticker WebSocket
type TickerWebSocket struct {
	baseURL   string
	symbol    string
	eventChan chan model.EngineEvent
	wsConfig  config.WSConfig

	wsClient *wsconn.WSClient
}

// NewTickerWebSocket 创建Ticker WebSocket
func NewTickerWebSocket(baseURL, symbol string, eventChan chan model.EngineEvent, wsConfig config.WSConfig) *TickerWebSocket {
	return &TickerWebSocket{
		baseURL:   baseURL,
		symbol:    symbol,
		eventChan: eventChan,
		wsConfig:  wsConfig,
	}
}

// Start 启动WebSocket连接
func (ws *TickerWebSocket) Start() error {
	// 构建URL
	streamName := strings.ToLower(ws.symbol) + "@ticker"
	url := ws.baseURL + "/" + streamName

	logger.Info("连接Ticker WebSocket: %s", url)

	// 创建WSClient（PublicTicker类型，不发ping）
	cfg := wsconn.WSConfig{
		PingInterval:        0, // PublicTicker禁用pingLoop
		PingIdleThreshold:   0,
		IdleTimeout:         time.Duration(ws.wsConfig.IdleTimeoutSec) * time.Second,
		PongTimeout:         0,
		HealthCheckInterval: time.Duration(ws.wsConfig.HealthCheckIntervalSec) * time.Second,
		ReconnectBaseDelay:  time.Duration(ws.wsConfig.ReconnectBaseDelaySec) * time.Second,
		ReconnectMaxDelay:   time.Duration(ws.wsConfig.ReconnectMaxDelaySec) * time.Second,
		ReconnectMaxRetries: ws.wsConfig.ReconnectMaxRetries,
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 30 * time.Second // 默认30秒无行情判断线
	}
	ws.wsClient = wsconn.NewPublicTickerClient(url, cfg)

	// 设置回调
	ws.wsClient.OnOpen = func() {
		logger.Info("Ticker WebSocket连接成功")
		// 推送连接成功事件
		ws.eventChan <- model.EngineEvent{
			Type: model.EventWsConnected,
			Data: model.WsConnectedEvent{Type: model.WsTypePublic},
		}
	}

	ws.wsClient.OnClose = func(err error) {
		logger.Info("Ticker WebSocket断开: %v", err)
		// 推送断开事件
		ws.eventChan <- model.EngineEvent{
			Type: model.EventWsDisconnected,
			Data: model.WsDisconnectedEvent{Type: model.WsTypePublic},
		}
	}

	ws.wsClient.OnMessage = func(message []byte) {
		ws.handleMessage(message)
	}

	// 连接
	return ws.wsClient.Connect()
}

// Close 关闭WebSocket
func (ws *TickerWebSocket) Close() error {
	if ws.wsClient != nil {
		return ws.wsClient.Close()
	}
	return nil
}

// IsHealthy 检查是否健康
func (ws *TickerWebSocket) IsHealthy() bool {
	if ws.wsClient == nil {
		return false
	}
	return ws.wsClient.IsHealthy()
}

// handleMessage 处理Ticker消息
func (ws *TickerWebSocket) handleMessage(message []byte) {
	// 使用map灵活解析
	var tickerMsg map[string]interface{}

	if err := json.Unmarshal(message, &tickerMsg); err != nil {
		logger.Error("解析Ticker消息失败: %v, 原始消息: %s", err, string(message))
		return
	}

	// 检查事件类型
	eventType, _ := tickerMsg["e"].(string)
	if eventType != "24hrTicker" {
		return
	}

	// 解析symbol
	symbol, _ := tickerMsg["s"].(string)

	// 解析最新成交价（可能是string或float64）
	var price float64
	switch v := tickerMsg["c"].(type) { // 最新成交价字段是 "c"
	case string:
		fmt.Sscanf(v, "%f", &price)
	case float64:
		price = v
	default:
		logger.Error("无法解析最新成交价字段: %v", tickerMsg["c"])
		return
	}

	// 解析时间戳
	timestamp, _ := tickerMsg["E"].(float64)

	// 推送事件给引擎
	ws.eventChan <- model.EngineEvent{
		Type: model.EventTicker,
		Data: model.TickerEvent{
			Symbol:    symbol,
			Price:     price,
			Timestamp: int64(timestamp),
		},
	}

	// 不记录每次价格更新，避免日志过多
	// logger.Debug("收到最新成交价: %s @ %.2f", symbol, price)
}
