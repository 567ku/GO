package model

import (
	"testing"
)

// ============= 测试纯整型DECIMAL格式化 =============

func TestFormatPriceDecimal_Pure(t *testing.T) {
	tests := []struct {
		name      string
		ticks     int64
		precision int
		want      string
	}{
		{"价格100.5精度1", 1005, 1, "100.5"},
		{"价格100.50精度2", 10050, 2, "100.50"},
		{"价格0.001精度3", 1, 3, "0.001"},
		{"价格99999精度0", 99999, 0, "99999"},
		{"负数-10.5精度1", -105, 1, "-10.5"},
		{"整数100精度2", 10000, 2, "100.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPriceDecimal(tt.ticks, tt.precision)
			if got != tt.want {
				t.Errorf("FormatPriceDecimal(%d, %d) = %s, want %s", tt.ticks, tt.precision, got, tt.want)
			}
		})
	}
}

func TestFormatQtyDecimal_Pure(t *testing.T) {
	tests := []struct {
		name      string
		ticks     int64
		precision int
		want      string
	}{
		{"数量0.001精度3", 1, 3, "0.001"},
		{"数量1.5精度1", 15, 1, "1.5"},
		{"数量100精度0", 100, 0, "100"},
		{"数量0.0001精度4", 1, 4, "0.0001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatQtyDecimal(tt.ticks, tt.precision)
			if got != tt.want {
				t.Errorf("FormatQtyDecimal(%d, %d) = %s, want %s", tt.ticks, tt.precision, got, tt.want)
			}
		})
	}
}

// ============= 测试纯整型DECIMAL解析 =============

func TestParsePriceDecimal_Pure(t *testing.T) {
	tests := []struct {
		name      string
		priceStr  string
		precision int
		want      int64
		wantErr   bool
	}{
		{"解析100.5精度1", "100.5", 1, 1005, false},
		{"解析100.50精度2", "100.50", 2, 10050, false},
		{"解析0.001精度3", "0.001", 3, 1, false},
		{"解析整数100精度2", "100", 2, 10000, false},
		{"解析整数100精度1", "100", 1, 1000, false},
		{"解析负数-10.5精度1", "-10.5", 1, -105, false},
		{"解析小数部分超长（截断）", "100.123456", 3, 100123, false},
		{"解析非法字符串", "abc", 1, 0, true},
		{"解析多个小数点", "100.5.5", 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePriceDecimal(tt.priceStr, tt.precision)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePriceDecimal(%s, %d) error = %v, wantErr %v", tt.priceStr, tt.precision, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParsePriceDecimal(%s, %d) = %d, want %d", tt.priceStr, tt.precision, got, tt.want)
			}
		})
	}
}

// ============= 测试纯整型格式化与解析的往返一致性 =============

func TestDecimal_RoundTrip(t *testing.T) {
	tests := []struct {
		ticks     int64
		precision int
	}{
		{1005, 1},
		{10050, 2},
		{1, 3},
		{99999, 0},
		{-105, 1},
		{123456789, 4},
	}

	for _, tt := range tests {
		// Format -> Parse
		formatted := FormatPriceDecimal(tt.ticks, tt.precision)
		parsed, err := ParsePriceDecimal(formatted, tt.precision)
		if err != nil {
			t.Errorf("RoundTrip failed: %v", err)
		}
		if parsed != tt.ticks {
			t.Errorf("RoundTrip不一致: ticks=%d, formatted=%s, parsed=%d", tt.ticks, formatted, parsed)
		}
	}
}

// ============= 测试取整 =============

func TestRoundTicks(t *testing.T) {
	tests := []struct {
		name  string
		ticks int64
		step  int64
		mode  RoundMode
		want  int64
	}{
		{"向下取整-整除", 100, 10, RoundModeDown, 100},
		{"向下取整-不整除", 105, 10, RoundModeDown, 100},
		{"向上取整-整除", 100, 10, RoundModeUp, 100},
		{"向上取整-不整除", 105, 10, RoundModeUp, 110},
		{"四舍五入-入", 104, 10, RoundModeNear, 100},
		{"四舍五入-舍", 105, 10, RoundModeNear, 110},
		{"负数向下取整", -105, 10, RoundModeDown, -100}, // 负数除法向下取整：-105/10=-10余-5，向下=-10
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RoundTicks(tt.ticks, tt.step, tt.mode)
			if got != tt.want {
				t.Errorf("RoundTicks(%d, %d, %s) = %d, want %d", tt.ticks, tt.step, tt.mode, got, tt.want)
			}
		})
	}
}

// ============= 测试QUOTE模式计算qty（纯整型算法）=============

func TestCalculateQtyFromQuote_Pure(t *testing.T) {
	tests := []struct {
		name           string
		quoteUsdTicks  int64
		priceTicks     int64
		quotePrecision int
		pricePrecision int
		qtyPrecision   int
		stepSize       int64
		roundMode      RoundMode
		want           int64
		wantErr        bool
	}{
		{
			name:           "100U买入@100.0（quotePrecision=2）",
			quoteUsdTicks:  10000, // 100.00U（精度2）
			priceTicks:     1000,  // 100.0（精度1）
			quotePrecision: 2,
			pricePrecision: 1,
			qtyPrecision:   3,
			stepSize:       0,
			roundMode:      RoundModeDown,
			want:           1000, // 1.000（精度3）
			wantErr:        false,
		},
		{
			name:           "50U买入@100.5（quotePrecision=2）",
			quoteUsdTicks:  5000, // 50.00U（精度2）
			priceTicks:     1005, // 100.5（精度1）
			quotePrecision: 2,
			pricePrecision: 1,
			qtyPrecision:   3,
			stepSize:       0,
			roundMode:      RoundModeDown,
			want:           497, // 0.497...（精度3）
			wantErr:        false,
		},
		{
			name:           "价格为0（错误）",
			quoteUsdTicks:  10000,
			priceTicks:     0,
			quotePrecision: 2,
			pricePrecision: 1,
			qtyPrecision:   3,
			stepSize:       0,
			roundMode:      RoundModeDown,
			want:           0,
			wantErr:        true,
		},
		{
			name:           "投入金额为0（错误）",
			quoteUsdTicks:  0,
			priceTicks:     1000,
			quotePrecision: 2,
			pricePrecision: 1,
			qtyPrecision:   3,
			stepSize:       0,
			roundMode:      RoundModeDown,
			want:           0,
			wantErr:        true,
		},
		{
			name:           "100U买入@50.5按stepSize=10取整（quotePrecision=2）",
			quoteUsdTicks:  10000, // 100.00U（精度2）
			priceTicks:     505,   // 50.5（精度1）
			quotePrecision: 2,
			pricePrecision: 1,
			qtyPrecision:   3,
			stepSize:       10, // 步长0.010
			roundMode:      RoundModeDown,
			want:           1980, // 1.980（向下取整到步长）
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateQtyFromQuote(
				tt.quoteUsdTicks,
				tt.priceTicks,
				tt.quotePrecision,
				tt.pricePrecision,
				tt.qtyPrecision,
				tt.stepSize,
				tt.roundMode,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateQtyFromQuote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CalculateQtyFromQuote() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ============= 测试最小值校验 =============

func TestValidateMinQty(t *testing.T) {
	tests := []struct {
		name        string
		qtyTicks    int64
		minQtyTicks int64
		wantErr     bool
	}{
		{"满足最小值", 100, 50, false},
		{"等于最小值", 50, 50, false},
		{"不足最小值", 40, 50, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMinQty(tt.qtyTicks, tt.minQtyTicks)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMinQty(%d, %d) error = %v, wantErr %v", tt.qtyTicks, tt.minQtyTicks, err, tt.wantErr)
			}
		})
	}
}

// ============= 测试minNotional校验（使用big.Int，正确的精度计算）=============

func TestValidateMinNotional(t *testing.T) {
	tests := []struct {
		name             string
		priceTicks       int64
		qtyTicks         int64
		minNotionalTicks int64
		pricePrecision   int
		qtyPrecision     int
		quotePrecision   int
		wantErr          bool
	}{
		{
			name:             "满足名义值-100.0*1.0=100.00",
			priceTicks:       1000, // 100.0（精度1）
			qtyTicks:         10,   // 1.0（精度1）
			minNotionalTicks: 5000, // 50.00（精度2）
			pricePrecision:   1,
			qtyPrecision:     1,
			quotePrecision:   2, // 名义值按2位精度
			wantErr:          false,
		},
		{
			name:             "不足名义值-10.0*1.0=10.00<20.00",
			priceTicks:       100,  // 10.0（精度1）
			qtyTicks:         10,   // 1.0（精度1）
			minNotionalTicks: 2000, // 20.00（精度2）
			pricePrecision:   1,
			qtyPrecision:     1,
			quotePrecision:   2,
			wantErr:          true,
		},
		{
			name:             "刚好等于最小值-5.00",
			priceTicks:       50,  // 5.0（精度1）
			qtyTicks:         10,  // 1.0（精度1）
			minNotionalTicks: 500, // 5.00（精度2）
			pricePrecision:   1,
			qtyPrecision:     1,
			quotePrecision:   2,
			wantErr:          false,
		},
		{
			name:             "略小于最小值-4.99",
			priceTicks:       50,  // 5.0（精度1）
			qtyTicks:         9,   // 0.9（精度1）
			minNotionalTicks: 500, // 5.00（精度2）
			pricePrecision:   1,
			qtyPrecision:     1,
			quotePrecision:   2,
			wantErr:          true,
		},
		{
			name:             "高精度-0.001*0.001=0.000001满足0.000001",
			priceTicks:       1, // 0.001（精度3）
			qtyTicks:         1, // 0.001（精度3）
			minNotionalTicks: 1, // 0.000001（精度6）
			pricePrecision:   3,
			qtyPrecision:     3,
			quotePrecision:   6,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMinNotional(
				tt.priceTicks,
				tt.qtyTicks,
				tt.minNotionalTicks,
				tt.pricePrecision,
				tt.qtyPrecision,
				tt.quotePrecision,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMinNotional() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
