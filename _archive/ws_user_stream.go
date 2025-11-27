package binance

import (
	"encoding/json"
	"gridbot/config"
	"gridbot/core/model"
	"gridbot/internal/logger"
	"gridbot/internal/wsconn"
	"strconv"
	"sync"
	"time"
)

// UserStreamWebSocket 私有用户数据流WebSocket
type UserStreamWebSocket struct {
	baseURL   string
	listenKey string
	eventChan chan model.EngineEvent
	wsConfig  config.WSConfig

	wsClient *wsconn.WSClient

	// adapter引用（用于双保险状态机）
	adapter *BinanceAdapter

	// 下单上下文管理（用于副通道）
	contexts map[string]*model.OrderContext
	ctxMu    sync.RWMutex

	// 重连回调
	OnReconnected  func()
	isFirstConnect bool // 标记是否首次连接
}

// NewUserStreamWebSocket 创建用户数据流WebSocket
func NewUserStreamWebSocket(baseURL, listenKey string, eventChan chan model.EngineEvent, wsConfig config.WSConfig, adapter *BinanceAdapter) *UserStreamWebSocket {
	return &UserStreamWebSocket{
		baseURL:        baseURL,
		listenKey:      listenKey,
		eventChan:      eventChan,
		wsConfig:       wsConfig,
		adapter:        adapter,
		contexts:       make(map[string]*model.OrderContext),
		isFirstConnect: true, // 初始化为首次连接
	}
}

// RegisterContext 注册下单上下文（用于副通道）
func (ws *UserStreamWebSocket) RegisterContext(ctx *model.OrderContext) {
	ws.ctxMu.Lock()
	defer ws.ctxMu.Unlock()
	ws.contexts[ctx.ClientOrderID] = ctx
}

// UnregisterContext 注销下单上下文
func (ws *UserStreamWebSocket) UnregisterContext(clientOrderID string) {
	ws.ctxMu.Lock()
	defer ws.ctxMu.Unlock()
	delete(ws.contexts, clientOrderID)
}

// getContext 获取下单上下文
func (ws *UserStreamWebSocket) getContext(clientOrderID string) (*model.OrderContext, bool) {
	ws.ctxMu.RLock()
	defer ws.ctxMu.RUnlock()
	ctx, ok := ws.contexts[clientOrderID]
	return ctx, ok
}

// Start 启动WebSocket连接
func (ws *UserStreamWebSocket) Start() error {
	url := ws.baseURL + "/ws/" + ws.listenKey

	logger.Info("连接User Stream WebSocket: %s...", url[:len(ws.baseURL)+20])

	// 创建WSClient（UserStream类型，激进探活）
	cfg := wsconn.WSConfig{
		PingInterval:        time.Duration(ws.wsConfig.PingIntervalSec) * time.Second,
		PingIdleThreshold:   time.Duration(ws.wsConfig.PingIdleThresholdSec) * time.Second,
		IdleTimeout:         time.Duration(ws.wsConfig.IdleTimeoutSec) * time.Second,
		PongTimeout:         time.Duration(ws.wsConfig.PongTimeoutSec) * time.Second,
		HealthCheckInterval: time.Duration(ws.wsConfig.HealthCheckIntervalSec) * time.Second,
		ReconnectBaseDelay:  time.Duration(ws.wsConfig.ReconnectBaseDelaySec) * time.Second,
		ReconnectMaxDelay:   time.Duration(ws.wsConfig.ReconnectMaxDelaySec) * time.Second,
		ReconnectMaxRetries: ws.wsConfig.ReconnectMaxRetries,
	}
	// UserStream使用激进策略，如果配置为0则使用默认值
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
	ws.wsClient = wsconn.NewUserStreamClient(url, cfg)

	// 设置回调
	ws.wsClient.OnOpen = func() {
		logger.Info("User Stream WebSocket连接成功")

		// 推送连接成功事件
		ws.eventChan <- model.EngineEvent{
			Type: model.EventWsConnected,
			Data: model.WsConnectedEvent{Type: model.WsTypePrivate},
		}
	}

	ws.wsClient.OnClose = func(err error) {
		logger.Info("User Stream WebSocket断开: %v", err)
		// 推送断开事件
		ws.eventChan <- model.EngineEvent{
			Type: model.EventWsDisconnected,
			Data: model.WsDisconnectedEvent{Type: model.WsTypePrivate},
		}
	}

	// V4.0重连回调：重连成功后触发短对账
	ws.wsClient.OnReconnected = func() {
		logger.Info("[UserStream] 重连成功，触发短对账")
		if ws.OnReconnected != nil {
			ws.OnReconnected()
		}
	}

	ws.wsClient.OnMessage = func(message []byte) {
		ws.handleMessage(message)
	}

	// 连接
	return ws.wsClient.Connect()
}

// Close 关闭WebSocket
func (ws *UserStreamWebSocket) Close() error {
	if ws.wsClient != nil {
		return ws.wsClient.Close()
	}
	logger.Info("User Stream WebSocket已关闭")
	return nil
}

// IsHealthy 检查是否健康
func (ws *UserStreamWebSocket) IsHealthy() bool {
	if ws.wsClient == nil {
		return false
	}
	return ws.wsClient.IsHealthy()
}

// handleMessage 处理用户数据流消息
func (ws *UserStreamWebSocket) handleMessage(message []byte) {
	// 使用map灵活解析
	var baseMsg map[string]interface{}

	if err := json.Unmarshal(message, &baseMsg); err != nil {
		logger.Error("解析消息失败: %v, 原始消息: %s", err, string(message))
		return
	}

	// 获取事件类型
	eventType, ok := baseMsg["e"].(string)
	if !ok {
		logger.Error("无法获取事件类型, 原始消息: %s", string(message))
		return
	}

	switch eventType {
	case "ORDER_TRADE_UPDATE":
		ws.handleOrderUpdate(message)
	case "ACCOUNT_UPDATE":
		// 账户更新事件（暂不处理）
		logger.Debug("收到ACCOUNT_UPDATE事件")
	case "TRADE_LITE":
		// 轻量级成交事件，已在 ORDER_TRADE_UPDATE 中处理，这里忽略
		// logger.Debug("收到TRADE_LITE事件")
	default:
		logger.Debug("未处理的事件类型: %s", eventType)
	}
}

// handleOrderUpdate 处理订单更新
func (ws *UserStreamWebSocket) handleOrderUpdate(message []byte) {
	var msg struct {
		Event     string `json:"e"`
		Time      int64  `json:"E"`
		OrderData struct {
			Symbol              string `json:"s"`
			ClientOrderID       string `json:"c"`
			Side                string `json:"S"`
			OrderType           string `json:"o"`
			TimeInForce         string `json:"f"`
			OrigQty             string `json:"q"`
			Price               string `json:"p"`
			AvgPrice            string `json:"ap"`
			StopPrice           string `json:"sp"`
			ExecutionType       string `json:"x"`
			OrderStatus         string `json:"X"`
			OrderID             int64  `json:"i"`
			LastFilledQty       string `json:"l"`
			CumulativeFilledQty string `json:"z"`
			LastFilledPrice     string `json:"L"`
			CommissionAsset     string `json:"N"`
			Commission          string `json:"n"`
			OrderTradeTime      int64  `json:"T"`
			TradeID             int64  `json:"t"`
			RealizedProfit      string `json:"rp"`
			PositionSide        string `json:"ps"`
			IsReduceOnly        bool   `json:"R"`
			WorkingType         string `json:"wt"`
			OrigType            string `json:"ot"`
			CumQuote            string `json:"Z"`
		} `json:"o"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		logger.Error("解析ORDER_TRADE_UPDATE失败: %v", err)
		return
	}

	o := msg.OrderData

	// 解析数字字段
	price, _ := strconv.ParseFloat(o.Price, 64)
	qty, _ := strconv.ParseFloat(o.OrigQty, 64)
	executedQty, _ := strconv.ParseFloat(o.CumulativeFilledQty, 64)
	cumQuote, _ := strconv.ParseFloat(o.CumQuote, 64)

	// 【新增】集成internalOrderContext（双保险状态机）
	if ws.adapter != nil {
		if iCtx, ok := ws.adapter.getOrderContext(o.ClientOrderID); ok {
			// 构造wsOrderUpdate事件
			update := &wsOrderUpdate{
				Status:      o.OrderStatus,
				OrderID:     o.OrderID,
				ExecutedQty: executedQty,
				IsAck:       o.OrderStatus == "NEW" || o.OrderStatus == "REJECTED",
				// ErrorCode/ErrorMsg需要从Binance消息中解析（如果REJECTED的话）
			}

			// 非阻塞发送
			select {
			case iCtx.wsUpdateCh <- update:
			default:
				logger.Info("[UserStream] wsUpdateCh缓冲已满: %s", o.ClientOrderID)
			}
		}
	}

	// 检查是否有对应的下单上下文（副通道，旧逻辑兼容）
	if ctx, ok := ws.getContext(o.ClientOrderID); ok {
		// 发送到副通道（使用带缓冲的 channel，不需要 timeout）
		if o.OrderStatus == "NEW" || o.OrderStatus == "REJECTED" {
			// Ack 事件
			select {
			case ctx.WsAckCh <- model.WsOrderAck{
				ClientOrderID: o.ClientOrderID,
				OrderID:       o.OrderID,
				Status:        o.OrderStatus,
			}:
			default:
				// channel 已满，记录警告
				logger.Info("[WS-UserStream] WsAckCh 缓冲已满: %s", o.ClientOrderID)
			}
		}

		if o.OrderStatus == "FILLED" || o.OrderStatus == "PARTIALLY_FILLED" || o.OrderStatus == "CANCELED" {
			// Exec 事件
			select {
			case ctx.WsExecCh <- model.WsOrderExec{
				ClientOrderID: o.ClientOrderID,
				OrderID:       o.OrderID,
				Status:        o.OrderStatus,
				ExecutedQty:   executedQty,
				Price:         price,
			}:
			default:
				// channel 已满，记录警告
				logger.Info("[WS-UserStream] WsExecCh 缓冲已满: %s", o.ClientOrderID)
			}
		}
	}

	// 推送订单更新事件（保持现有逻辑）
	ws.eventChan <- model.EngineEvent{
		Type: model.EventOrderUpdate,
		Data: model.OrderUpdateEvent{
			Symbol:        o.Symbol,
			OrderID:       o.OrderID,
			ClientOrderID: o.ClientOrderID,
			Status:        o.OrderStatus,
			Side:          o.Side,
			Price:         price,
			Quantity:      qty,
			ExecutedQty:   executedQty,
			CumQuote:      cumQuote,
			UpdateTime:    o.OrderTradeTime,
			PositionSide:  o.PositionSide,
			Type:          o.OrderType,
			OrigType:      o.OrigType,
		},
	}

	logger.Info("[订单更新] %s %s %s@%.2f, 状态=%s, 成交=%s/%s, ClientOrderID=%s",
		o.Symbol, o.Side, o.OrigQty, price, o.OrderStatus,
		o.CumulativeFilledQty, o.OrigQty, o.ClientOrderID)

	// 如果有成交，推送成交事件
	if o.ExecutionType == "TRADE" {
		lastFilledQty, _ := strconv.ParseFloat(o.LastFilledQty, 64)
		lastFilledPrice, _ := strconv.ParseFloat(o.LastFilledPrice, 64)
		realizedPnl, _ := strconv.ParseFloat(o.RealizedProfit, 64)
		commission, _ := strconv.ParseFloat(o.Commission, 64)

		ws.eventChan <- model.EngineEvent{
			Type: model.EventUserTrade,
			Data: model.UserTradeEvent{
				Symbol:          o.Symbol,
				OrderID:         o.OrderID,
				ClientOrderID:   o.ClientOrderID,
				Side:            o.Side,
				Price:           lastFilledPrice,
				Quantity:        lastFilledQty,
				RealizedPnl:     realizedPnl,
				Commission:      commission,
				CommissionAsset: o.CommissionAsset,
				Time:            o.OrderTradeTime,
				PositionSide:    o.PositionSide,
				Buyer:           o.Side == "BUY",
				Maker:           o.ExecutionType == "MAKER",
			},
		}

		logger.Info("[成交] %s %s %.4f@%.2f, 手续费=%.4f %s",
			o.Symbol, o.Side, lastFilledQty, lastFilledPrice,
			commission, o.CommissionAsset)
	}
}
