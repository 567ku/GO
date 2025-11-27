// Package model - ticks转换与取整（纯整型算法，禁止float）
// 严格按照《网格需求文档-v1.3-字段与接口规范》实现
// 内部用ticks（int64）运算，避免float误差；发WS时转DECIMAL字符串
package model

import (
	"fmt"
	"math/big"
	"strings"
)

// ============= Ticks 转换（仅用于展示，禁止用于交易路径） =============
// 警告：以下函数包含 float 运算，仅用于日志/UI展示
// 交易路径必须使用 FormatPriceDecimal/ParsePriceDecimal（纯整型）

//nolint:unused // 仅用于展示，禁止交易路径使用
func priceToTicksForDisplay(price float64, precision int) int64 {
	// 使用big.Float避免精度丢失
	multiplier := new(big.Float).SetInt(pow10BigInt(precision))
	priceBig := new(big.Float).SetFloat64(price)
	result := new(big.Float).Mul(priceBig, multiplier)

	// 四舍五入
	ticks, _ := result.Int64()
	return ticks
}

//nolint:unused // 仅用于展示，禁止交易路径使用
func ticksToPriceForDisplay(ticks int64, precision int) float64 {
	multiplier := pow10Float(precision)
	return float64(ticks) / multiplier
}

//nolint:unused // 仅用于展示，禁止交易路径使用
func qtyToTicksForDisplay(qty float64, precision int) int64 {
	multiplier := new(big.Float).SetInt(pow10BigInt(precision))
	qtyBig := new(big.Float).SetFloat64(qty)
	result := new(big.Float).Mul(qtyBig, multiplier)

	ticks, _ := result.Int64()
	return ticks
}

//nolint:unused // 仅用于展示，禁止交易路径使用
func ticksToQtyForDisplay(ticks int64, precision int) float64 {
	multiplier := pow10Float(precision)
	return float64(ticks) / multiplier
}

// ============= 价格/数量格式化为DECIMAL字符串（纯整型算法，禁止float）=============

// FormatPriceDecimal 格式化价格为DECIMAL字符串（纯整型实现）
// Binance WS API要求price/quantity为JSON字符串
func FormatPriceDecimal(ticks int64, precision int) string {
	return formatDecimalPure(ticks, precision)
}

// FormatQtyDecimal 格式化数量为DECIMAL字符串（纯整型实现）
func FormatQtyDecimal(ticks int64, precision int) string {
	return formatDecimalPure(ticks, precision)
}

// formatDecimalPure 纯整型格式化为DECIMAL字符串
// 示例：ticks=1005, precision=1 => "100.5"
//
//	ticks=10050, precision=2 => "100.50"
func formatDecimalPure(ticks int64, precision int) string {
	if precision <= 0 {
		return fmt.Sprintf("%d", ticks)
	}

	// 处理负数
	sign := ""
	if ticks < 0 {
		sign = "-"
		ticks = -ticks
	}

	// 计算整数部分和小数部分
	divisor := pow10Int64(precision)
	intPart := ticks / divisor
	fracPart := ticks % divisor

	// 格式化小数部分（补齐前导零）
	fracStr := fmt.Sprintf("%0*d", precision, fracPart)

	return fmt.Sprintf("%s%d.%s", sign, intPart, fracStr)
}

// ParsePriceDecimal 解析DECIMAL字符串为ticks（纯整型实现）
func ParsePriceDecimal(priceStr string, precision int) (int64, error) {
	return parseDecimalPure(priceStr, precision)
}

// ParseQtyDecimal 解析DECIMAL字符串为ticks（纯整型实现）
func ParseQtyDecimal(qtyStr string, precision int) (int64, error) {
	return parseDecimalPure(qtyStr, precision)
}

// parseDecimalPure 纯整型解析DECIMAL字符串
// 示例："100.5" precision=1 => 1005
//
//	"100.50" precision=2 => 10050
func parseDecimalPure(str string, precision int) (int64, error) {
	str = strings.TrimSpace(str)
	if str == "" {
		return 0, fmt.Errorf("空字符串")
	}

	// 处理负数
	sign := int64(1)
	if str[0] == '-' {
		sign = -1
		str = str[1:]
	} else if str[0] == '+' {
		str = str[1:]
	}

	// 分割整数部分和小数部分
	parts := strings.Split(str, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("非法DECIMAL格式: %s", str)
	}

	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}

	// 解析整数部分（使用big.Int防溢出）
	intValue := big.NewInt(0)
	for _, ch := range intPart {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("非法字符: %c", ch)
		}
		intValue.Mul(intValue, big.NewInt(10))
		intValue.Add(intValue, big.NewInt(int64(ch-'0')))
	}

	// 解析小数部分（使用big.Int防溢出）
	fracValue := big.NewInt(0)
	if fracPart != "" {
		// 截断或补齐到precision位
		if len(fracPart) > precision {
			fracPart = fracPart[:precision]
		}
		for _, ch := range fracPart {
			if ch < '0' || ch > '9' {
				return 0, fmt.Errorf("非法字符: %c", ch)
			}
			fracValue.Mul(fracValue, big.NewInt(10))
			fracValue.Add(fracValue, big.NewInt(int64(ch-'0')))
		}
		// 补齐到precision位
		for len(fracPart) < precision {
			fracValue.Mul(fracValue, big.NewInt(10))
			fracPart += "0"
		}
	}

	// 合并整数和小数部分
	multiplier := pow10BigInt(precision)
	result := new(big.Int).Mul(intValue, multiplier)
	result.Add(result, fracValue)

	// 溢出检查：确保结果在int64范围内
	if !result.IsInt64() {
		return 0, fmt.Errorf("解析结果溢出int64: %s (precision=%d)", str, precision)
	}

	ticks := result.Int64()
	return sign * ticks, nil
}

// ============= 取整（按roundMode）=============

// RoundMode 取整模式
type RoundMode string

const (
	RoundModeDown RoundMode = "DOWN" // 向下取整（默认）
	RoundModeUp   RoundMode = "UP"   // 向上取整
	RoundModeNear RoundMode = "NEAR" // 四舍五入
)

// RoundTicks 按模式取整ticks
func RoundTicks(ticks int64, step int64, mode RoundMode) int64 {
	if step == 0 {
		return ticks
	}

	switch mode {
	case RoundModeUp:
		return CeilDiv(ticks, step) * step
	case RoundModeNear:
		return ((ticks + step/2) / step) * step
	default: // DOWN
		return (ticks / step) * step
	}
}

// RoundPriceTicks 按价格步长取整价格ticks
func RoundPriceTicks(priceTicks int64, tickSize int64, mode RoundMode) int64 {
	return RoundTicks(priceTicks, tickSize, mode)
}

// RoundQtyTicks 按数量步长取整数量ticks
func RoundQtyTicks(qtyTicks int64, stepSize int64, mode RoundMode) int64 {
	return RoundTicks(qtyTicks, stepSize, mode)
}

// ============= QUOTE模式：按U计算数量（纯整型算法）=============

// CalculateQtyFromQuote 从QUOTE模式计算qty（纯整型算法）
// quoteUsdTicks: 投入U（按quotePrecision，如200表示2.00U当quotePrecision=2）
// priceTicks: 下单价格（按pricePrecision）
// quotePrecision: QUOTE精度（固定2，表示0.01U）
// pricePrecision: 价格精度
// qtyPrecision: 数量精度
// stepSize: 数量步长（ticks）
// roundMode: 取整模式（默认DOWN）
// 返回: qtyTicks
//
// 公式（纯整型）：
// qtyTicks = (quoteUsdTicks * 10^pricePrecision) / priceTicks
// 然后转换到qtyPrecision并按stepSize取整
func CalculateQtyFromQuote(
	quoteUsdTicks int64,
	priceTicks int64,
	quotePrecision int,
	pricePrecision int,
	qtyPrecision int,
	stepSize int64,
	roundMode RoundMode,
) (int64, error) {
	if priceTicks == 0 {
		return 0, fmt.Errorf("价格不能为0")
	}
	if quoteUsdTicks <= 0 {
		return 0, fmt.Errorf("投入金额必须大于0")
	}

	// 使用big.Int防止溢出
	// qtyTicks = (quoteUsdTicks * 10^(qtyPrecision+pricePrecision-quotePrecision)) / priceTicks
	quote := big.NewInt(quoteUsdTicks)
	price := big.NewInt(priceTicks)

	// 计算精度调整因子
	// 目标：得到以qtyPrecision为单位的结果
	// quoteUsd是以quotePrecision为单位
	// price是以pricePrecision为单位
	// qty = quoteUsd / price => qty精度 = quotePrecision - pricePrecision
	// 需要调整到qtyPrecision
	precisionAdjust := qtyPrecision + pricePrecision - quotePrecision

	// 禁止负数精度调整（会导致精度丢失）
	if precisionAdjust < 0 {
		return 0, fmt.Errorf("精度配置错误: qtyPrecision(%d) + pricePrecision(%d) - quotePrecision(%d) = %d < 0，会导致精度丢失",
			qtyPrecision, pricePrecision, quotePrecision, precisionAdjust)
	}

	multiplier := pow10BigInt(precisionAdjust)
	result := new(big.Int).Mul(quote, multiplier)
	result.Div(result, price)

	if !result.IsInt64() {
		return 0, fmt.Errorf("计算结果溢出int64")
	}

	qtyTicks := result.Int64()

	// 按stepSize取整
	if stepSize > 0 {
		qtyTicks = RoundTicks(qtyTicks, stepSize, roundMode)
	}

	return qtyTicks, nil
}

// ============= 校验最小值（使用big.Int防溢出）=============

// ValidateMinQty 校验数量是否满足最小值
func ValidateMinQty(qtyTicks int64, minQtyTicks int64) error {
	if qtyTicks < minQtyTicks {
		return fmt.Errorf("数量不足最小值: %d < %d", qtyTicks, minQtyTicks)
	}
	return nil
}

// ValidateMinNotional 校验名义值是否满足最小值（使用big.Int交叉相乘，避免除法截断）
// notional = price * qty
// 按统一精度计算（使用quotePrecision作为名义值精度）
func ValidateMinNotional(
	priceTicks int64,
	qtyTicks int64,
	minNotionalTicks int64,
	pricePrecision int,
	qtyPrecision int,
	quotePrecision int,
) error {
	// 使用big.Int防止溢出，并用交叉相乘避免除法截断
	// notional (按quotePrecision) = (priceTicks * qtyTicks) / 10^(pricePrecision + qtyPrecision - quotePrecision)
	//
	// 比较：notional >= minNotional
	// 即：(priceTicks * qtyTicks) / divisor >= minNotional
	// 交叉相乘：priceTicks * qtyTicks >= minNotional * divisor
	// 避免除法截断误判

	price := big.NewInt(priceTicks)
	qty := big.NewInt(qtyTicks)
	minNotional := big.NewInt(minNotionalTicks)

	// 左侧：priceTicks * qtyTicks
	lhs := new(big.Int).Mul(price, qty)

	// 精度调整因子
	precisionAdjust := pricePrecision + qtyPrecision - quotePrecision

	// 右侧：minNotional * 10^precisionAdjust
	var rhs *big.Int
	if precisionAdjust > 0 {
		divisor := pow10BigInt(precisionAdjust)
		rhs = new(big.Int).Mul(minNotional, divisor)
	} else if precisionAdjust < 0 {
		// 如果precisionAdjust < 0，需要乘以10^(-precisionAdjust)
		multiplier := pow10BigInt(-precisionAdjust)
		rhs = new(big.Int).Mul(minNotional, multiplier)
		// 左侧不变，右侧已调整
	} else {
		// precisionAdjust == 0，直接比较
		rhs = minNotional
	}

	// 比较：lhs >= rhs
	if lhs.Cmp(rhs) < 0 {
		return fmt.Errorf("名义值不足最小值: (price=%d * qty=%d) < minNotional=%d (按quotePrecision=%d, precisionAdjust=%d)",
			priceTicks, qtyTicks, minNotionalTicks, quotePrecision, precisionAdjust)
	}

	return nil
}

// ============= 辅助函数 =============

// pow10Int64 计算10^n（int64）
func pow10Int64(n int) int64 {
	result := int64(1)
	for i := 0; i < n; i++ {
		result *= 10
	}
	return result
}

// pow10Float 计算10^n（float64）
func pow10Float(n int) float64 {
	result := 1.0
	for i := 0; i < n; i++ {
		result *= 10.0
	}
	return result
}

// pow10BigInt 计算10^n（big.Int）
func pow10BigInt(n int) *big.Int {
	if n < 0 {
		return big.NewInt(1)
	}
	result := big.NewInt(1)
	ten := big.NewInt(10)
	for i := 0; i < n; i++ {
		result.Mul(result, ten)
	}
	return result
}
