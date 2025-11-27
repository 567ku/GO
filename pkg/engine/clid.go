// P0-E-03: Level/OrderSlot状态归因 - CLID映射
package engine

import (
	"fmt"
	"gridbot/pkg/model"
	"strconv"
	"strings"
)

// ParseCLID 解析 clientOrderId
// 格式：{prefix}:E:{levelId}:{cycle}  (Entry)
//
//	{prefix}:T:{levelId}:{cycle}  (TP)
//
// 返回：purpose, levelID, cycle, error
func ParseCLID(clid string, prefix string) (model.OrderPurpose, int, int, error) {
	// 检查前缀
	if !strings.HasPrefix(clid, prefix+":") {
		return "", 0, 0, fmt.Errorf("CLID前缀不匹配: expected=%s, got=%s", prefix, clid)
	}

	// 移除前缀
	suffix := strings.TrimPrefix(clid, prefix+":")
	parts := strings.Split(suffix, ":")

	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("CLID格式错误: expected {prefix}:{E|T}:{levelId}:{cycle}, got=%s", clid)
	}

	// 解析 purpose
	var purpose model.OrderPurpose
	switch parts[0] {
	case "E":
		purpose = model.OrderPurposeEntry
	case "T":
		purpose = model.OrderPurposeTP
	default:
		return "", 0, 0, fmt.Errorf("CLID purpose错误: expected E或T, got=%s", parts[0])
	}

	// 解析 levelID
	levelID, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("CLID levelID解析失败: %w", err)
	}

	// 解析 cycle
	cycle, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, 0, fmt.Errorf("CLID cycle解析失败: %w", err)
	}

	return purpose, levelID, cycle, nil
}

// FindLevelByID 查找指定levelID的level
// 如果不存在，返回nil
func FindLevelByID(state *model.GridStateSnapshot, levelID int) *model.LevelState {
	for i := range state.Levels {
		if state.Levels[i].LevelID == levelID {
			return &state.Levels[i]
		}
	}
	return nil
}

// GetOrderSlot 获取指定level的订单槽
// purpose: ENTRY/TP
func GetOrderSlot(level *model.LevelState, purpose model.OrderPurpose) *model.OrderSlot {
	switch purpose {
	case model.OrderPurposeEntry:
		return &level.Entry
	case model.OrderPurposeTP:
		return &level.TP
	default:
		return nil
	}
}
