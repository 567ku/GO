//go:build ignore
// +build ignore

package binance

// 旧版本WS Manager（已废弃）
// 新版本请使用 pkg/ws/manager.go
// 本文件保留仅供历史参考

import (
	"fmt"
	"gridbot/config"
	"gridbot/core/model"
	"gridbot/internal/logger"
	"sync"
	"time"
)

// WSManager WebSocket管理器，统一管理三条WS连接
type WSManager struct {
	adapter  *BinanceAdapter
	wsConfig config.WSConfig

	// 三条WS连接
	tickerWS     *TickerWebSocket
	userStreamWS *UserStreamWebSocket
	orderWS      *WsOrderClient

	// 健康状态（由Engine维护，这里只负责推送事件）
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup
	started  bool
}

// NewWSManager 创建WS管理器
func NewWSManager(adapter *BinanceAdapter, wsConfig config.WSConfig) *WSManager {
	return &WSManager{
		adapter:  adapter,
		wsConfig: wsConfig,
		stopChan: make(chan struct{}),
	}
}

// StartAllConnections 启动所有WS连接
func (m *WSManager) StartAllConnections() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("WSManager already started")
	}

	logger.Info("=== WSManager: 开始启动所有WebSocket连接 ===")

	// 1. 启动公共行情WS
	logger.Info("WSManager: 启动公共行情WS...")
	m.tickerWS = NewTickerWebSocket(
		m.adapter.wsPublicURL,
		m.adapter.symbol,
		m.adapter.eventChan,
		m.wsConfig,
	)
	if err := m.tickerWS.Start(); err != nil {
		return fmt.Errorf("启动公共行情WS失败: %w", err)
	}
	logger.Info("WSManager: 公共行情WS启动成功")

	// 2. 启动私有UserStream WS（需要先获取listenKey）
	logger.Info("WSManager: 启动私有UserStreamWS...")
	listenKey, err := m.adapter.GetListenKey()
	if err != nil {
		return fmt.Errorf("获取listenKey失败: %w", err)
	}
	m.userStreamWS = NewUserStreamWebSocket(
		m.adapter.wsPrivateURL,
		listenKey,
		m.adapter.eventChan,
		m.wsConfig,
		m.adapter,
	)

	// 设置 UserStream 重连回调（将触发短对账事件）
	m.userStreamWS.OnReconnected = func() {
		// 推送 UserStream 重连事件
		m.adapter.eventChan <- model.EngineEvent{
			Type: model.EventTimer,
			Data: model.TimerEvent{Type: "userstream_reconnected", Time: time.Now()},
		}
	}

	if err := m.userStreamWS.Start(); err != nil {
		return fmt.Errorf("启动私有UserStreamWS失败: %w", err)
	}
	logger.Info("WSManager: 私有UserStreamWS启动成功 (listenKey=****)")

	// 3. 启动下单WS（可选）
	if m.adapter.wsOrderURL != "" {
		logger.Info("WSManager: 启动下单WS (url=%s)...", m.adapter.wsOrderURL)
		m.orderWS = NewWsOrderClient(
			m.adapter.apiKey,
			m.adapter.apiSecret,
			m.adapter.wsOrderURL,
			m.wsConfig,
		)
		if err := m.orderWS.Connect(); err != nil {
			logger.Info("WSManager: 下单WS连接失败，将使用REST下单: %v", err)
			// 不返回错误，允许降级到REST下单
			m.orderWS = nil
		} else {
			logger.Info("WSManager: 下单WS启动成功")
			// 推送连接成功事件
			m.adapter.eventChan <- model.EngineEvent{
				Type: model.EventWsConnected,
				Data: model.WsConnectedEvent{Type: model.WsTypeTrade},
			}
		}
	}

	// 4. 启动事件驱动监控（订阅unhealthyChan）
	m.wg.Add(4)
	go m.watchTickerUnhealthy()
	go m.watchUserStreamUnhealthy()
	go m.watchOrderStreamUnhealthy()
	go m.keepAliveListenKey()

	// 5. 可选：保留轻量级全局健康日志（仅记录，不触发重连）
	m.wg.Add(1)
	go m.logHealthStatus()

	m.started = true
	logger.Info("=== WSManager: 所有WebSocket连接启动完成 ===")
	return nil
}

// watchTickerUnhealthy 监听Ticker WS不健康事件
func (m *WSManager) watchTickerUnhealthy() {
	defer m.wg.Done()

	if m.tickerWS == nil || m.tickerWS.wsClient == nil {
		return
	}

	unhealthyChan := m.tickerWS.wsClient.GetUnhealthyChan()

	for {
		select {
		case <-m.stopChan:
			return
		case reason := <-unhealthyChan:
			logger.Info("[WSManager] Ticker WS不健康: reason=%s, 开始重连...", reason)

			// 推送断开事件
			m.adapter.eventChan <- model.EngineEvent{
				Type: model.EventWsDisconnected,
				Data: model.WsDisconnectedEvent{Type: model.WsTypePublic},
			}

			// 重连Ticker WS（简单重启，行情WS无状态）
			go m.reconnectTicker()
		}
	}
}

// watchUserStreamUnhealthy 监听UserStream WS不健康事件
func (m *WSManager) watchUserStreamUnhealthy() {
	defer m.wg.Done()

	if m.userStreamWS == nil || m.userStreamWS.wsClient == nil {
		return
	}

	unhealthyChan := m.userStreamWS.wsClient.GetUnhealthyChan()

	for {
		select {
		case <-m.stopChan:
			return
		case reason := <-unhealthyChan:
			logger.Info("[WSManager] UserStream WS不健康: reason=%s, 开始重连...", reason)

			// 推送断开事件
			m.adapter.eventChan <- model.EngineEvent{
				Type: model.EventWsDisconnected,
				Data: model.WsDisconnectedEvent{Type: model.WsTypePrivate},
			}

			// 重连UserStream WS（需要重新获取listenKey）
			go m.reconnectUserStream()
		}
	}
}

// watchOrderStreamUnhealthy 监听订单WS不健康事件
func (m *WSManager) watchOrderStreamUnhealthy() {
	defer m.wg.Done()

	if m.orderWS == nil || m.orderWS.wsClient == nil {
		return
	}

	unhealthyChan := m.orderWS.wsClient.GetUnhealthyChan()

	for {
		select {
		case <-m.stopChan:
			return
		case reason := <-unhealthyChan:
			logger.Info("[WSManager] 订单WS不健康: reason=%s, 开始重连...", reason)

			// 推送断开事件
			m.adapter.eventChan <- model.EngineEvent{
				Type: model.EventWsDisconnected,
				Data: model.WsDisconnectedEvent{Type: model.WsTypeTrade},
			}

			// 重连订单WS
			go m.reconnectOrderStream()
		}
	}
}

// reconnectTicker 重连Ticker WS
func (m *WSManager) reconnectTicker() {
	// 指数退避重连
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second
	attempt := 0

	for {
		select {
		case <-m.stopChan:
			return
		default:
		}

		attempt++
		delay := baseDelay * time.Duration(1<<uint(min(attempt-1, 6)))
		if delay > maxDelay {
			delay = maxDelay
		}

		logger.Info("[WSManager] Ticker WS重连延迟 %v (attempt=%d)", delay, attempt)
		time.Sleep(delay)

		// 关闭旧连接
		if m.tickerWS != nil {
			m.tickerWS.Close()
		}

		// 创建新连接
		m.mu.Lock()
		m.tickerWS = NewTickerWebSocket(
			m.adapter.wsPublicURL,
			m.adapter.symbol,
			m.adapter.eventChan,
			m.wsConfig,
		)
		m.mu.Unlock()

		if err := m.tickerWS.Start(); err != nil {
			logger.Info("[WSManager] Ticker WS重连失败: %v", err)
			continue
		}

		// 重连成功，调用OnReconnectedInternal重置状态
		if m.tickerWS.wsClient != nil {
			m.tickerWS.wsClient.OnReconnectedInternal()
		}

		logger.Info("[WSManager] Ticker WS重连成功")

		// 推送连接成功事件
		m.adapter.eventChan <- model.EngineEvent{
			Type: model.EventWsConnected,
			Data: model.WsConnectedEvent{Type: model.WsTypePublic},
		}

		// 重新监听不健康事件
		m.wg.Add(1)
		go m.watchTickerUnhealthy()
		return
	}
}

// reconnectUserStream 重连UserStream WS
func (m *WSManager) reconnectUserStream() {
	// 指数退避重连
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second
	attempt := 0

	for {
		select {
		case <-m.stopChan:
			return
		default:
		}

		attempt++
		delay := baseDelay * time.Duration(1<<uint(min(attempt-1, 6)))
		if delay > maxDelay {
			delay = maxDelay
		}

		logger.Info("[WSManager] UserStream WS重连延迟 %v (attempt=%d)", delay, attempt)
		time.Sleep(delay)

		// 关闭旧连接
		if m.userStreamWS != nil {
			m.userStreamWS.Close()
		}

		// 重新获取listenKey
		listenKey, err := m.adapter.GetListenKey()
		if err != nil {
			logger.Info("[WSManager] 获取listenKey失败: %v", err)
			continue
		}

		// 创建新连接
		m.mu.Lock()
		m.userStreamWS = NewUserStreamWebSocket(
			m.adapter.wsPrivateURL,
			listenKey,
			m.adapter.eventChan,
			m.wsConfig,
			m.adapter,
		)

		// 设置重连回调
		m.userStreamWS.OnReconnected = func() {
			m.adapter.eventChan <- model.EngineEvent{
				Type: model.EventTimer,
				Data: model.TimerEvent{Type: "userstream_reconnected", Time: time.Now()},
			}
		}
		m.mu.Unlock()

		if err := m.userStreamWS.Start(); err != nil {
			logger.Info("[WSManager] UserStream WS重连失败: %v", err)
			continue
		}

		// 重连成功，调用OnReconnectedInternal重置状态
		if m.userStreamWS.wsClient != nil {
			m.userStreamWS.wsClient.OnReconnectedInternal()
		}

		logger.Info("[WSManager] UserStream WS重连成功")

		// 推送连接成功事件
		m.adapter.eventChan <- model.EngineEvent{
			Type: model.EventWsConnected,
			Data: model.WsConnectedEvent{Type: model.WsTypePrivate},
		}

		// 触发短对账（仅一次）
		if m.userStreamWS.OnReconnected != nil {
			logger.Info("[WSManager] UserStream重连成功，触发短对账")
			go m.userStreamWS.OnReconnected()
		}

		// 重新监听不健康事件
		m.wg.Add(1)
		go m.watchUserStreamUnhealthy()
		return
	}
}

// reconnectOrderStream 重连订单WS
func (m *WSManager) reconnectOrderStream() {
	// 指数退避重连
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second
	attempt := 0

	for {
		select {
		case <-m.stopChan:
			return
		default:
		}

		attempt++
		delay := baseDelay * time.Duration(1<<uint(min(attempt-1, 6)))
		if delay > maxDelay {
			delay = maxDelay
		}

		logger.Info("[WSManager] 订单WS重连延迟 %v (attempt=%d)", delay, attempt)
		time.Sleep(delay)

		// 关闭旧连接
		if m.orderWS != nil {
			m.orderWS.Close()
		}

		// 创建新连接
		m.mu.Lock()
		m.orderWS = NewWsOrderClient(
			m.adapter.apiKey,
			m.adapter.apiSecret,
			m.adapter.wsOrderURL,
			m.wsConfig,
		)
		m.mu.Unlock()

		if err := m.orderWS.Connect(); err != nil {
			logger.Info("[WSManager] 订单WS重连失败: %v", err)
			continue
		}

		// 重连成功，调用OnReconnectedInternal重置状态
		if m.orderWS.wsClient != nil {
			m.orderWS.wsClient.OnReconnectedInternal()
		}

		logger.Info("[WSManager] 订单WS重连成功")

		// 推送连接成功事件
		m.adapter.eventChan <- model.EngineEvent{
			Type: model.EventWsConnected,
			Data: model.WsConnectedEvent{Type: model.WsTypeTrade},
		}

		// 重新监听不健康事件
		m.wg.Add(1)
		go m.watchOrderStreamUnhealthy()
		return
	}
}

// logHealthStatus 轻量级全局健康日志（仅记录，不触发重连）
func (m *WSManager) logHealthStatus() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.mu.RLock()
			tickerHealthy := m.tickerWS != nil && m.tickerWS.IsHealthy()
			userStreamHealthy := m.userStreamWS != nil && m.userStreamWS.IsHealthy()
			orderStreamHealthy := m.orderWS != nil && m.orderWS.IsHealthy()
			m.mu.RUnlock()

			logger.Debug("[WSManager] 健康状态: Ticker=%v UserStream=%v OrderStream=%v",
				tickerHealthy, userStreamHealthy, orderStreamHealthy)

		case <-m.stopChan:
			return
		}
	}
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Close 关闭所有WS连接
func (m *WSManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	logger.Info("WSManager: 关闭所有WebSocket连接...")

	// 停止心跳监控
	close(m.stopChan)

	// 关闭三条WS
	if m.tickerWS != nil {
		m.tickerWS.Close()
	}
	if m.userStreamWS != nil {
		m.userStreamWS.Close()
	}
	if m.orderWS != nil {
		m.orderWS.Close()
	}

	// 等待协程退出
	m.wg.Wait()

	m.started = false
	logger.Info("WSManager: 所有WebSocket连接已关闭")
	return nil
}

// GetTickerWS 获取公共行情WS（供Adapter使用）
func (m *WSManager) GetTickerWS() *TickerWebSocket {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tickerWS
}

// GetUserStreamWS 获取私有UserStreamWS（供Adapter使用）
func (m *WSManager) GetUserStreamWS() *UserStreamWebSocket {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.userStreamWS
}

// GetOrderWS 获取下单WS（供Adapter使用）
func (m *WSManager) GetOrderWS() *WsOrderClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.orderWS
}

// keepAliveListenKey 定期续期listenKey（按配置间隔刷新）
func (m *WSManager) keepAliveListenKey() {
	defer m.wg.Done()

	// 从config读取刷新间隔，默认30分钟
	refreshInterval := time.Duration(m.wsConfig.ListenKeyRefreshIntervalMin) * time.Minute
	if refreshInterval == 0 {
		refreshInterval = 30 * time.Minute
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.adapter.renewListenKey(); err != nil {
				logger.Error("续期listenKey失败: %v，尝试重新创建", err)
				// 重新创建 listenKey 并重连 UserStream
				if err := m.recreateListenKey(); err != nil {
					logger.Error("重新创建listenKey失败: %v", err)
				}
			} else {
				logger.Debug("listenKey续期成功")
			}
		case <-m.stopChan:
			return
		}
	}
}

// recreateListenKey 重新创建listenKey并重连UserStream
func (m *WSManager) recreateListenKey() error {
	// 关闭旧的UserStream
	if m.userStreamWS != nil {
		m.userStreamWS.Close()
	}

	// 创建新的listenKey
	if err := m.adapter.createListenKey(); err != nil {
		return err
	}

	// 获取新listenKey
	listenKey, err := m.adapter.GetListenKey()
	if err != nil {
		return err
	}

	// 重新启动UserStream
	m.mu.Lock()
	m.userStreamWS = NewUserStreamWebSocket(
		m.adapter.wsPrivateURL,
		listenKey,
		m.adapter.eventChan,
		m.wsConfig, // 传入配置
		m.adapter,  // 传入adapter引用
	)
	m.mu.Unlock()

	return m.userStreamWS.Start()
}
