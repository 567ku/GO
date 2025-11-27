package ws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gridbot/pkg/model"
)

// ============= WS Manager（统一总控） =============
// 职责：
//   1. 启动/停止三条 WS（Market/UDS/Trade）
//   2. 统一健康监控（lastPongAt/lastMsgAt/errCount）
//   3. 统一重连策略（已内置在各 client）
//   4. listenKey 生命周期管理（获取、续期）
//   5. 断线事件发给 Engine，触发 Freeze/Reconcile
// 禁止：
//   - 在 manager 中做"对账差异分析""缺口扫描""窗口移动"
//   - manager 直接调用下单/撤单（只能由 Executor/Task 驱动）

// Manager WS 管理器
type Manager struct {
	config ManagerConfig

	// 三条 WS 客户端
	marketClient *MarketClient
	udsClient    *UDSClient
	tradeClient  *TradeClient

	// listenKey 提供者
	listenKeyProvider ListenKeyProvider

	// Engine 事件通道
	eventCh chan<- model.EngineEvent

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 状态
	mu      sync.RWMutex
	started bool
}

// NewManager 创建 WS 管理器
func NewManager(config ManagerConfig, listenKeyProvider ListenKeyProvider, eventCh chan<- model.EngineEvent) *Manager {
	return &Manager{
		config:            config,
		listenKeyProvider: listenKeyProvider,
		eventCh:           eventCh,
	}
}

// Start 启动所有 WS 连接
func (m *Manager) Start(ctx context.Context) error {
	// P0-WS-21: 防御nil context
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("manager already started")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// 1. 启动 Market WS（P1-1: 从配置获取symbol，禁止硬编码）
	if m.config.MarketURL != "" {
		if m.config.Symbol == "" {
			return fmt.Errorf("market ws requires symbol in config")
		}
		// Stage 4C: 精度注入验证
		if m.config.PricePrecision <= 0 {
			return fmt.Errorf("market ws requires pricePrecision > 0 in config")
		}
		m.marketClient = NewMarketClient(
			m.config.Symbol, // P1-1: 从配置注入
			m.config.MarketURL,
			m.config.ConnConfig,
			m.config.PricePrecision, // Stage 4C: 精度注入
			m.eventCh,
		)
		if err := m.marketClient.Start(); err != nil {
			return fmt.Errorf("start market ws failed: %w", err)
		}
	}

	// 2. 启动 UDS WS
	if m.config.UDSURL != "" && m.listenKeyProvider != nil {
		// Stage 4C: 精度注入验证
		if m.config.PricePrecision <= 0 || m.config.QtyPrecision <= 0 || m.config.QuotePrecision <= 0 {
			return fmt.Errorf("uds ws requires pricePrecision/qtyPrecision/quotePrecision > 0 in config")
		}
		m.udsClient = NewUDSClient(
			m.config.UDSURL,
			m.listenKeyProvider,
			m.config.ListenKeyKeepAlivePeriod,
			m.config.ConnConfig,
			m.config.PricePrecision, // Stage 4C: 精度注入
			m.config.QtyPrecision,   // Stage 4C: 精度注入
			m.config.QuotePrecision, // Stage 4C: 精度注入
			m.eventCh,
		)
		if err := m.udsClient.Start(); err != nil {
			return fmt.Errorf("start uds ws failed: %w", err)
		}
	}

	// 3. 启动 Trade WS（P1-1: 从配置获取apiKey/apiSecret，禁止硬编码）
	if m.config.TradeURL != "" {
		if m.config.APIKey == "" || m.config.APISecret == "" {
			return fmt.Errorf("trade ws requires apiKey and apiSecret in config")
		}
		m.tradeClient = NewTradeClient(
			m.config.APIKey,    // P1-1: 从配置注入
			m.config.APISecret, // P1-1: 从配置注入
			m.config.TradeURL,
			m.config.TradeRequestTimeout,
			m.config.ConnConfig,
			m.eventCh,
		)
		if err := m.tradeClient.Start(); err != nil {
			return fmt.Errorf("start trade ws failed: %w", err)
		}
	}

	// 4. 启动健康监控
	m.wg.Add(1)
	go m.healthMonitorLoop()

	m.started = true
	return nil
}

// Stop 停止所有 WS 连接
func (m *Manager) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 停止三条 WS
	var errs []error

	if m.marketClient != nil {
		if err := m.marketClient.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop market ws: %w", err))
		}
	}

	if m.udsClient != nil {
		if err := m.udsClient.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop uds ws: %w", err))
		}
	}

	if m.tradeClient != nil {
		if err := m.tradeClient.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop trade ws: %w", err))
		}
	}

	// 等待所有 goroutine 退出
	m.wg.Wait()

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}

	return nil
}

// GetHealth 获取所有 WS 的健康状态
func (m *Manager) GetHealth() map[model.WSChannel]ConnHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health := make(map[model.WSChannel]ConnHealth)

	if m.marketClient != nil {
		health[model.WSChannelMarket] = m.marketClient.GetHealth()
	}

	if m.udsClient != nil {
		health[model.WSChannelUDS] = m.udsClient.GetHealth()
	}

	if m.tradeClient != nil {
		health[model.WSChannelTrade] = m.tradeClient.GetHealth()
	}

	return health
}

// IsHealthy 检查所有 WS 是否健康
func (m *Manager) IsHealthy() bool {
	health := m.GetHealth()

	for _, h := range health {
		if !h.IsHealthy {
			return false
		}
	}

	return true
}

// GetTradeClient 获取 Trade 客户端（供 Executor 使用）
func (m *Manager) GetTradeClient() *TradeClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tradeClient
}

// healthMonitorLoop 健康监控循环
func (m *Manager) healthMonitorLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.logHealthStatus()
		}
	}
}

// logHealthStatus 记录健康状态（仅日志，不触发重连）
func (m *Manager) logHealthStatus() {
	health := m.GetHealth()

	for channel, h := range health {
		status := "HEALTHY"
		if !h.IsHealthy {
			status = "UNHEALTHY"
		}

		// TODO: 使用实际的日志库
		_ = fmt.Sprintf("[WS Manager] %s: %s, state=%s, reconnects=%d, lastMsg=%v",
			channel, status, h.State, h.ReconnectCount, time.Since(h.LastMessageAt))
	}
}

// ============= 辅助方法（供外部调用） =============

// SendTradeRequest 通过 Trade WS 发送请求
func (m *Manager) SendTradeRequest(taskID, reqID, method string, params map[string]interface{}) (*TradeWSResponse, error) {
	m.mu.RLock()
	client := m.tradeClient
	m.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("trade client not available")
	}

	return client.SendRequest(taskID, reqID, method, params)
}

// IsTradeWSReady 检查 Trade WS 是否就绪
func (m *Manager) IsTradeWSReady() bool {
	m.mu.RLock()
	client := m.tradeClient
	m.mu.RUnlock()

	if client == nil {
		return false
	}

	return client.IsHealthy()
}

// IsUDSReady 检查 UDS WS 是否就绪
func (m *Manager) IsUDSReady() bool {
	m.mu.RLock()
	client := m.udsClient
	m.mu.RUnlock()

	if client == nil {
		return false
	}

	return client.IsHealthy()
}
