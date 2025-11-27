package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gridbot/pkg/model"
)

// ============= Stage 4C: 精度注入单元测试 =============
// 验证：
//   1. MarketClient 使用注入的 pricePrecision 正确解析行情
//   2. UDSClient 使用注入的 pricePrecision/qtyPrecision/quotePrecision 正确解析订单更新
//   3. 不同精度配置产生不同的 ticks 结果
//   4. Manager 启动时验证精度配置必填

// TestMarketClient_PrecisionInjection_Market 验证 MarketClient 精度注入
func TestMarketClient_PrecisionInjection_Market(t *testing.T) {
	// Phase 1: 创建两个不同精度的 MarketClient
	eventCh := make(chan model.EngineEvent, 10)
	config := DefaultConnConfig()

	// Precision = 1 (价格单位0.1，如BTC: 50000.0)
	client1 := NewMarketClient("BTCUSDT", "wss://example.com", config, 1, eventCh)
	// Precision = 2 (价格单位0.01，如ETH: 3000.00)
	client2 := NewMarketClient("ETHUSDT", "wss://example.com", config, 2, eventCh)

	// Phase 2: 模拟接收相同字符串价格 "1234.56"
	tickerMsg := []byte(`{
		"e": "24hrTicker",
		"E": 1700000000000,
		"s": "BTCUSDT",
		"c": "1234.56"
	}`)

	// Phase 3: 解析并验证 ticks 差异
	client1.handleMessage(tickerMsg)
	client2.handleMessage(tickerMsg)

	// 验证：precision=1，ticks=12345 (1234.5 * 10^1)
	event1 := <-eventCh
	priceEvent1 := event1.Data.(model.PriceTickEvent)
	if priceEvent1.PriceTicks != 12345 {
		t.Errorf("precision=1: expected ticks=12345, got %d", priceEvent1.PriceTicks)
	}

	// 验证：precision=2，ticks=123456 (1234.56 * 10^2)
	event2 := <-eventCh
	priceEvent2 := event2.Data.(model.PriceTickEvent)
	if priceEvent2.PriceTicks != 123456 {
		t.Errorf("precision=2: expected ticks=123456, got %d", priceEvent2.PriceTicks)
	}
}

// TestUDSClient_PrecisionInjection_UDS 验证 UDSClient 精度注入
func TestUDSClient_PrecisionInjection_UDS(t *testing.T) {
	// Phase 1: 创建两个不同精度的 UDSClient
	eventCh := make(chan model.EngineEvent, 20)
	config := DefaultConnConfig()

	// Mock listenKey provider
	provider := &mockListenKeyProvider{key: "test_key"}

	// Client 1: pricePrecision=1, qtyPrecision=3, quotePrecision=2
	client1 := NewUDSClient("wss://example.com", provider, 30*time.Minute, config, 1, 3, 2, eventCh)
	// Client 2: pricePrecision=2, qtyPrecision=4, quotePrecision=3
	client2 := NewUDSClient("wss://example.com", provider, 30*time.Minute, config, 2, 4, 3, eventCh)

	// Phase 2: 模拟订单更新消息（价格=1234.56，数量=0.123，PNL=12.34）
	orderUpdateMsg := []byte(`{
		"e": "ORDER_TRADE_UPDATE",
		"E": 1700000000000,
		"o": {
			"s": "BTCUSDT",
			"c": "TEST:E:500:1",
			"S": "BUY",
			"o": "LIMIT",
			"f": "GTC",
			"q": "0.123",
			"p": "1234.56",
			"ap": "1234.56",
			"sp": "0",
			"x": "TRADE",
			"X": "FILLED",
			"i": 123456,
			"l": "0.123",
			"z": "0.123",
			"L": "1234.56",
			"N": "USDT",
			"n": "0.5",
			"T": 1700000000000,
			"t": 78901,
			"b": "0",
			"a": "0",
			"m": false,
			"R": false,
			"wt": "CONTRACT_PRICE",
			"ot": "LIMIT",
			"ps": "BOTH",
			"cp": false,
			"AP": "",
			"cr": "",
			"rp": "12.34"
		}
	}`)

	// Phase 3: 直接调用 handleOrderUpdate（绕过 handleMessage 的事件类型分发）
	client1.handleOrderUpdate(orderUpdateMsg)
	client2.handleOrderUpdate(orderUpdateMsg)

	// Client 1 验证：pricePrecision=1 → priceTicks=12345
	event1 := <-eventCh
	orderEvent1 := event1.Data.(model.OrderUpdateEvent)
	if orderEvent1.PriceTicks != 12345 {
		t.Errorf("client1 price: expected ticks=12345, got %d", orderEvent1.PriceTicks)
	}
	// qtyPrecision=3 → origQtyTicks=123 (0.123 * 10^3)
	if orderEvent1.OrigQtyTicks != 123 {
		t.Errorf("client1 qty: expected ticks=123, got %d", orderEvent1.OrigQtyTicks)
	}

	// TradeUpdateEvent 验证（Client 1）
	tradeEvent1 := <-eventCh
	trade1 := tradeEvent1.Data.(model.TradeUpdateEvent)
	// qtyPrecision=3 → qtyTicks=123
	if trade1.QtyTicks != 123 {
		t.Errorf("client1 trade qty: expected ticks=123, got %d", trade1.QtyTicks)
	}
	// quotePrecision=2 → realizedPnlTicks=1234 (12.34 * 10^2)
	if trade1.RealizedPnlTicks == nil || *trade1.RealizedPnlTicks != 1234 {
		t.Errorf("client1 PNL: expected ticks=1234, got %v", trade1.RealizedPnlTicks)
	}

	// Client 2 验证：pricePrecision=2 → priceTicks=123456
	event2 := <-eventCh
	orderEvent2 := event2.Data.(model.OrderUpdateEvent)
	if orderEvent2.PriceTicks != 123456 {
		t.Errorf("client2 price: expected ticks=123456, got %d", orderEvent2.PriceTicks)
	}
	// qtyPrecision=4 → origQtyTicks=1230 (0.123 * 10^4)
	if orderEvent2.OrigQtyTicks != 1230 {
		t.Errorf("client2 qty: expected ticks=1230, got %d", orderEvent2.OrigQtyTicks)
	}

	// TradeUpdateEvent 验证（Client 2）
	tradeEvent2 := <-eventCh
	trade2 := tradeEvent2.Data.(model.TradeUpdateEvent)
	// qtyPrecision=4 → qtyTicks=1230
	if trade2.QtyTicks != 1230 {
		t.Errorf("client2 trade qty: expected ticks=1230, got %d", trade2.QtyTicks)
	}
	// quotePrecision=3 → realizedPnlTicks=12340 (12.34 * 10^3)
	if trade2.RealizedPnlTicks == nil || *trade2.RealizedPnlTicks != 12340 {
		t.Errorf("client2 PNL: expected ticks=12340, got %v", trade2.RealizedPnlTicks)
	}
}

// TestManager_PrecisionValidation 验证 Manager 启动时精度配置必填
func TestManager_PrecisionValidation(t *testing.T) {
	eventCh := make(chan model.EngineEvent, 10)
	provider := &mockListenKeyProvider{key: "test_key"}

	testCases := []struct {
		name           string
		config         ManagerConfig
		expectedErrMsg string
	}{
		{
			name: "MarketWS缺失pricePrecision",
			config: ManagerConfig{
				MarketURL:      "wss://example.com",
				Symbol:         "BTCUSDT",
				PricePrecision: 0, // 缺失
				ConnConfig:     DefaultConnConfig(),
			},
			expectedErrMsg: "market ws requires pricePrecision > 0 in config",
		},
		{
			name: "UDSWS缺失pricePrecision",
			config: ManagerConfig{
				UDSURL:                   "wss://example.com",
				Symbol:                   "BTCUSDT",
				PricePrecision:           0, // 缺失
				QtyPrecision:             3,
				QuotePrecision:           2,
				ConnConfig:               DefaultConnConfig(),
				ListenKeyKeepAlivePeriod: 30 * time.Minute,
			},
			expectedErrMsg: "uds ws requires pricePrecision/qtyPrecision/quotePrecision > 0 in config",
		},
		{
			name: "UDSWS缺失qtyPrecision",
			config: ManagerConfig{
				UDSURL:                   "wss://example.com",
				Symbol:                   "BTCUSDT",
				PricePrecision:           1,
				QtyPrecision:             0, // 缺失
				QuotePrecision:           2,
				ConnConfig:               DefaultConnConfig(),
				ListenKeyKeepAlivePeriod: 30 * time.Minute,
			},
			expectedErrMsg: "uds ws requires pricePrecision/qtyPrecision/quotePrecision > 0 in config",
		},
		{
			name: "UDSWS缺失quotePrecision",
			config: ManagerConfig{
				UDSURL:                   "wss://example.com",
				Symbol:                   "BTCUSDT",
				PricePrecision:           1,
				QtyPrecision:             3,
				QuotePrecision:           0, // 缺失
				ConnConfig:               DefaultConnConfig(),
				ListenKeyKeepAlivePeriod: 30 * time.Minute,
			},
			expectedErrMsg: "uds ws requires pricePrecision/qtyPrecision/quotePrecision > 0 in config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewManager(tc.config, provider, eventCh)
			err := manager.Start(context.Background()) // 修复：使用非nil context
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tc.expectedErrMsg {
				t.Errorf("expected error message '%s', got '%s'", tc.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestPrecision_RealWorldScenario 真实场景测试：BTCUSDT (precision=1) vs ETHUSDT (precision=2)
func TestPrecision_RealWorldScenario(t *testing.T) {
	eventCh := make(chan model.EngineEvent, 10)
	config := DefaultConnConfig()

	// BTCUSDT: pricePrecision=1 (最小价格0.1 USDT)
	btcClient := NewMarketClient("BTCUSDT", "wss://example.com", config, 1, eventCh)

	// 价格字符串 "50123.4" → ticks=501234 (50123.4 * 10^1)
	btcTickerMsg := []byte(`{
		"e": "24hrTicker",
		"E": 1700000000000,
		"s": "BTCUSDT",
		"c": "50123.4"
	}`)

	btcClient.handleMessage(btcTickerMsg)
	event := <-eventCh
	priceEvent := event.Data.(model.PriceTickEvent)

	if priceEvent.PriceTicks != 501234 {
		t.Errorf("BTCUSDT price: expected ticks=501234, got %d", priceEvent.PriceTicks)
	}

	// ETHUSDT: pricePrecision=2 (最小价格0.01 USDT)
	ethClient := NewMarketClient("ETHUSDT", "wss://example.com", config, 2, eventCh)

	// 价格字符串 "3456.78" → ticks=345678 (3456.78 * 10^2)
	ethTickerMsg := []byte(`{
		"e": "24hrTicker",
		"E": 1700000000000,
		"s": "ETHUSDT",
		"c": "3456.78"
	}`)

	ethClient.handleMessage(ethTickerMsg)
	event2 := <-eventCh
	priceEvent2 := event2.Data.(model.PriceTickEvent)

	if priceEvent2.PriceTicks != 345678 {
		t.Errorf("ETHUSDT price: expected ticks=345678, got %d", priceEvent2.PriceTicks)
	}
}

// ============= Mock ListenKey Provider =============

type mockListenKeyProvider struct {
	key string
}

func (m *mockListenKeyProvider) GetListenKey() (string, error) {
	return m.key, nil
}

func (m *mockListenKeyProvider) KeepAliveListenKey(listenKey string) error {
	return nil
}

// ============= 补充：JSON 序列化验证（确保 ManagerConfig 精度字段可序列化） =============

func TestManagerConfig_JSONSerialization(t *testing.T) {
	config := ManagerConfig{
		MarketURL:      "wss://example.com",
		Symbol:         "BTCUSDT",
		PricePrecision: 1,
		QtyPrecision:   3,
		QuotePrecision: 2,
		ConnConfig:     DefaultConnConfig(),
	}

	// 序列化
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// 反序列化
	var decoded ManagerConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// 验证精度字段
	if decoded.PricePrecision != 1 {
		t.Errorf("pricePrecision: expected 1, got %d", decoded.PricePrecision)
	}
	if decoded.QtyPrecision != 3 {
		t.Errorf("qtyPrecision: expected 3, got %d", decoded.QtyPrecision)
	}
	if decoded.QuotePrecision != 2 {
		t.Errorf("quotePrecision: expected 2, got %d", decoded.QuotePrecision)
	}
}
