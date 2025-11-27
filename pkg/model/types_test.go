package model

import (
	"testing"
)

// ============= 测试 clientOrderId 校验 =============

func TestValidateClientOrderID(t *testing.T) {
	tests := []struct {
		name    string
		clid    string
		wantErr bool
	}{
		{"合法-基本格式", "GRID_A1:E:12:3", false},
		{"合法-包含所有允许字符", "Grid.A-B_C:E:/123:456", false},
		{"合法-最长36字符", "123456789012345678901234567890123456", false},
		{"非法-超过36字符", "1234567890123456789012345678901234567", true},
		{"非法-包含非法字符@", "GRID@A1:E:12:3", true},
		{"非法-包含非法字符#", "GRID#A1:E:12:3", true},
		{"非法-空字符串", "", true},
		{"合法-单字符", "A", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateClientOrderID(tt.clid)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateClientOrderID(%s) error = %v, wantErr %v", tt.clid, err, tt.wantErr)
			}
		})
	}
}

// ============= 测试网格配置校验 =============

func TestValidateGridConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *GridConfig
		wantErr bool
	}{
		{
			name: "合法配置",
			cfg: &GridConfig{
				Prefix:              "GRID_A1",
				Side:                GridSideLong,
				StepTicks:           100,
				WinMinTicks:         10000,
				WinMaxTicks:         20000,
				TPKeepOutsideLevels: 5,
			},
			wantErr: false,
		},
		{
			name: "非法-step<=0",
			cfg: &GridConfig{
				Prefix:      "GRID_A1",
				StepTicks:   0,
				WinMinTicks: 10000,
				WinMaxTicks: 20000,
			},
			wantErr: true,
		},
		{
			name: "非法-winMin>=winMax",
			cfg: &GridConfig{
				Prefix:      "GRID_A1",
				StepTicks:   100,
				WinMinTicks: 20000,
				WinMaxTicks: 10000,
			},
			wantErr: true,
		},
		{
			name: "非法-winMin未对齐",
			cfg: &GridConfig{
				Prefix:      "GRID_A1",
				StepTicks:   100,
				WinMinTicks: 10050, // 不是100的整数倍
				WinMaxTicks: 20000,
			},
			wantErr: true,
		},
		{
			name: "非法-winMax未对齐",
			cfg: &GridConfig{
				Prefix:      "GRID_A1",
				StepTicks:   100,
				WinMinTicks: 10000,
				WinMaxTicks: 20050, // 不是100的整数倍
			},
			wantErr: true,
		},
		{
			name: "非法-(winMax-winMin)不是step的整数倍",
			cfg: &GridConfig{
				Prefix:      "GRID_A1",
				StepTicks:   300,
				WinMinTicks: 10000,
				WinMaxTicks: 10400, // (10400-10000)=400，不是300的整数倍
			},
			wantErr: true,
		},
		{
			name: "非法-tpKeepOutsideLevels<0",
			cfg: &GridConfig{
				Prefix:              "GRID_A1",
				StepTicks:           100,
				WinMinTicks:         10000,
				WinMaxTicks:         20000,
				TPKeepOutsideLevels: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGridConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGridConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============= 测试 CLID 生成（带长度校验）=============

func TestBuildCLID(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		levelID int
		cycle   int
		wantErr bool
		errMsg  string
	}{
		{
			name:    "合法-正常CLID",
			prefix:  "GRID_A1",
			levelID: 12,
			cycle:   3,
			wantErr: false,
		},
		{
			name:    "合法-最大长度CLID",
			prefix:  "GRID_ABC123",
			levelID: 999,
			cycle:   999,
			wantErr: false,
		},
		{
			name:    "非法-prefix过长导致超36字符",
			prefix:  "GRID_VERYLONGPREFIXNAME12345",
			levelID: 12345,
			cycle:   9999,
			wantErr: true,
			errMsg:  "生成的CLID超过36字符",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryCLID, err := BuildEntryCLID(tt.prefix, tt.levelID, tt.cycle)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildEntryCLID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				// 验证生成的CLID合规性
				if err := ValidateClientOrderID(entryCLID); err != nil {
					t.Errorf("生成的Entry CLID不合规: %v", err)
				}
				// 验证长度
				if len(entryCLID) > 36 {
					t.Errorf("生成的Entry CLID超过36字符: %s（长度%d）", entryCLID, len(entryCLID))
				}
			}

			tpCLID, err := BuildTPCLID(tt.prefix, tt.levelID, tt.cycle)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildTPCLID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if err := ValidateClientOrderID(tpCLID); err != nil {
					t.Errorf("生成的TP CLID不合规: %v", err)
				}
				if len(tpCLID) > 36 {
					t.Errorf("生成的TP CLID超过36字符: %s（长度%d）", tpCLID, len(tpCLID))
				}
			}
		})
	}
}

// ============= 测试 CeilDiv =============

func TestCeilDiv(t *testing.T) {
	tests := []struct {
		name string
		a    int64
		b    int64
		want int64
	}{
		{"整除", 100, 10, 10},
		{"向上取整-1", 101, 10, 11},
		{"向上取整-2", 105, 10, 11},
		{"除数为0", 100, 0, 0},
		{"被除数为0", 0, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CeilDiv(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CeilDiv(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ============= 测试订单状态终态判断 =============

func TestOrderState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    OrderState
		terminal bool
	}{
		{OrderStateNone, false},
		{OrderStateSubmitted, false},
		{OrderStateOpen, false},
		{OrderStatePartial, false},
		{OrderStateFilled, true},
		{OrderStateCanceled, true},
		{OrderStateRejected, true},
		{OrderStateExpired, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.terminal {
				t.Errorf("OrderState(%s).IsTerminal() = %v, want %v", tt.state, got, tt.terminal)
			}
		})
	}
}

// ============= 测试 TradeUpdateEvent 的 RealizedPnlTicks 可选字段 =============

func TestTradeUpdateEvent_RealizedPnl(t *testing.T) {
	// nil表示缺失
	event1 := TradeUpdateEvent{
		RealizedPnlTicks: nil,
	}
	if event1.RealizedPnlTicks != nil {
		t.Errorf("RealizedPnlTicks应为nil")
	}

	// 有值
	pnl := int64(12345)
	event2 := TradeUpdateEvent{
		RealizedPnlTicks: &pnl,
	}
	if event2.RealizedPnlTicks == nil || *event2.RealizedPnlTicks != 12345 {
		t.Errorf("RealizedPnlTicks应为12345")
	}

	// 0值也应该用指针表示（区别于nil）
	zero := int64(0)
	event3 := TradeUpdateEvent{
		RealizedPnlTicks: &zero,
	}
	if event3.RealizedPnlTicks == nil {
		t.Errorf("RealizedPnlTicks=0应用指针表示，不应为nil")
	}
	if *event3.RealizedPnlTicks != 0 {
		t.Errorf("RealizedPnlTicks应为0")
	}
}

// ============= 测试 ExecutorResultEvent 的 ReqID 字段 =============

func TestExecutorResultEvent_ReqID(t *testing.T) {
	event := ExecutorResultEvent{
		TaskID:         "task-001",
		ReqID:          "req-123",
		OK:             true,
		ExchangeTimeMs: 1234567890,
	}

	if event.ReqID == "" {
		t.Errorf("ReqID不应为空")
	}
	if event.ReqID != "req-123" {
		t.Errorf("ReqID应为req-123，实际为%s", event.ReqID)
	}
	if event.ExchangeTimeMs != 1234567890 {
		t.Errorf("ExchangeTimeMs应为1234567890")
	}
}
