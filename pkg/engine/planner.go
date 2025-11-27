// P0-E-06: GapScan缺口扫描（最小闭环）
package engine

import (
	"gridbot/pkg/model"
)

// GapScan 缺口扫描（最小闭环）
// 规则：
// 1. 区间外：Entry全撤
// 2. TP：区间外保留TPKeepOutsideLevels格
// 3. 区间内缺口立即补齐
func GapScan(state *model.GridStateSnapshot) []model.Task {
	var tasks []model.Task

	// 计算窗口范围（基于当前价格）
	minLevel, maxLevel := calcWindowRange(state)

	// P0-RET-06: 区间外Entry撤单 + TP保留N格逻辑
	for i := range state.Levels {
		level := &state.Levels[i]
		levelID := level.LevelID

		// 区间外Entry全撤（跳过终态和NONE）
		if (levelID < minLevel || levelID > maxLevel) && !level.Entry.State.IsTerminal() && level.Entry.State != model.OrderStateNone {
			tasks = append(tasks, generateCancelTask(state, levelID, model.OrderPurposeEntry))
		}

		// TP保留逻辑：超出window + TPKeepOutsideLevels的TP才撤（跳过终态和NONE）
		tpKeepRange := state.Window.TPKeepOutsideLevels
		// 注意：Entry的终态也不能撤TP（可能是历史成交）
		if !level.TP.State.IsTerminal() && level.TP.State != model.OrderStateNone &&
			!level.Entry.State.IsTerminal() &&
			(levelID < minLevel-tpKeepRange || levelID > maxLevel+tpKeepRange) {
			tasks = append(tasks, generateCancelTask(state, levelID, model.OrderPurposeTP))
		}
	}

	// 扫描所有level（区间内补齐）
	for levelID := minLevel; levelID <= maxLevel; levelID++ {
		priceTicks := calcLevelPrice(state, levelID)

		// 查找或创廼level
		level := FindLevelByID(state, levelID)
		if level == nil {
			// Level不存在，创建新level并生成placeEntry任务
			tasks = append(tasks, generatePlaceEntryTask(state, levelID, priceTicks))
			continue
		}

		// Entry缺口检测
		if needPlaceEntry(level) {
			tasks = append(tasks, generatePlaceEntryTask(state, levelID, priceTicks))
		}

		// TP缺口检测（Entry FILLED后）
		if needPlaceTP(level) {
			tasks = append(tasks, generatePlaceTPTask(state, levelID, priceTicks))
		}
	}

	return tasks
}

// calcWindowRange 计算窗口范围（levelID）
func calcWindowRange(state *model.GridStateSnapshot) (minLevel, maxLevel int) {
	currentPrice := state.Market.LastPriceTicks
	stepTicks := state.Window.StepTicks
	winMinTicks := state.Window.WinMinTicks
	winMaxTicks := state.Window.WinMaxTicks

	// 计算最小和最大levelID
	// levelID = (priceTicks - winMinTicks) / stepTicks
	minLevel = int((winMinTicks) / stepTicks)
	maxLevel = int((winMaxTicks) / stepTicks)

	// 根据当前价格调整范围（避免过大）
	currentLevel := int(currentPrice / stepTicks)
	safeRange := 50 // 最多扫描50格（避免内存爆炸）

	if minLevel < currentLevel-safeRange {
		minLevel = currentLevel - safeRange
	}
	if maxLevel > currentLevel+safeRange {
		maxLevel = currentLevel + safeRange
	}

	return minLevel, maxLevel
}

// calcLevelPrice 计算level价格
func calcLevelPrice(state *model.GridStateSnapshot, levelID int) int64 {
	return int64(levelID) * state.Window.StepTicks
}

// needPlaceEntry 判断是否需要放置Entry
func needPlaceEntry(level *model.LevelState) bool {
	// Entry不存在或已终态（FILLED/CANCELED/REJECTED）
	return level.Entry.State == model.OrderStateNone ||
		level.Entry.State.IsTerminal()
}

// needPlaceTP 判断是否需要放置TP
func needPlaceTP(level *model.LevelState) bool {
	// Entry已FILLED，且TP不存在或已终态
	return level.Entry.State == model.OrderStateFilled &&
		(level.TP.State == model.OrderStateNone || level.TP.State.IsTerminal())
}

// generatePlaceEntryTask 生成placeEntry任务
func generatePlaceEntryTask(state *model.GridStateSnapshot, levelID int, priceTicks int64) model.Task {
	// FAIL-04: 从 level 获取真实 cycle（非硬编码 0）
	level := FindLevelByID(state, levelID)
	cycle := 0
	if level != nil {
		cycle = level.Cycle
	}

	// 生成新的ClientOrderID
	clid, _ := model.BuildEntryCLID(state.Prefix, levelID, cycle)

	// 确定PositionSide
	positionSide := "BOTH" // ONE_WAY模式
	if state.PositionMode == model.PositionModeHedge {
		if state.Side == model.GridSideLong {
			positionSide = "LONG"
		} else {
			positionSide = "SHORT"
		}
	}

	return model.Task{
		TaskID:          generateTaskID(),
		Type:            model.TaskTypePlaceEntry,
		Symbol:          state.Symbol,
		LevelID:         levelID,
		Purpose:         model.OrderPurposeEntry,
		ClientOrderID:   clid,
		PriceTicks:      priceTicks,
		QtyTicks:        state.Qty.EntryQtyTicks,
		PositionSide:    positionSide,
		ReduceOnly:      nil, // Entry不需要reduceOnly
		Attempt:         0,
		LeaseExpireAtMs: 0, // 由Engine设置
		CreatedAtMs:     model.NowMs(),
		State:           model.TaskStatePending,
	}
}

// generatePlaceTPTask 生成placeTP任务
func generatePlaceTPTask(state *model.GridStateSnapshot, levelID int, priceTicks int64) model.Task {
	// TP价格 = Entry价格 + stepTicks（LONG）或 - stepTicks（SHORT）
	tpPriceTicks := priceTicks
	if state.Side == model.GridSideLong {
		tpPriceTicks += state.Window.StepTicks
	} else {
		tpPriceTicks -= state.Window.StepTicks
	}

	// FAIL-04: 从 level 获取真实 cycle（非硬编码 0）
	level := FindLevelByID(state, levelID)
	cycle := 0
	if level != nil {
		cycle = level.Cycle
	}

	// 生成新的ClientOrderID
	clid, _ := model.BuildTPCLID(state.Prefix, levelID, cycle)

	// 确定PositionSide
	positionSide := "BOTH" // ONE_WAY模式
	if state.PositionMode == model.PositionModeHedge {
		if state.Side == model.GridSideLong {
			positionSide = "LONG"
		} else {
			positionSide = "SHORT"
		}
	}

	// P1-ENG-12: ONE_WAY模式TP必须reduceOnly=true
	reduceOnly := true
	var reduceOnlyPtr *bool
	if state.PositionMode == model.PositionModeOneWay {
		reduceOnlyPtr = &reduceOnly // ONE_WAY模式必须reduceOnly=true
	}
	// HEDGE模式不需要reduceOnly，留空nil

	// 获取Entry的成交数量
	qtyTicks := state.Qty.EntryQtyTicks
	if level != nil && level.Entry.ExecutedQtyTicks > 0 {
		qtyTicks = level.Entry.ExecutedQtyTicks
	}

	return model.Task{
		TaskID:          generateTaskID(),
		Type:            model.TaskTypePlaceTP,
		Symbol:          state.Symbol,
		LevelID:         levelID,
		Purpose:         model.OrderPurposeTP,
		ClientOrderID:   clid,
		PriceTicks:      tpPriceTicks,
		QtyTicks:        qtyTicks,
		PositionSide:    positionSide,
		ReduceOnly:      reduceOnlyPtr,
		Attempt:         0,
		LeaseExpireAtMs: 0, // 由Engine设置
		CreatedAtMs:     model.NowMs(),
		State:           model.TaskStatePending,
	}
}

// generateTaskID 生成TaskID（P0-1: 使用ULID保证唯一性）
func generateTaskID() string {
	return model.GenerateTaskID()
}

// generateCancelTask 生成撤单任务
func generateCancelTask(state *model.GridStateSnapshot, levelID int, purpose model.OrderPurpose) model.Task {
	level := FindLevelByID(state, levelID)
	if level == nil {
		// 理论上不应该到这里
		return model.Task{}
	}

	var slot *model.OrderSlot
	if purpose == model.OrderPurposeEntry {
		slot = &level.Entry
	} else {
		slot = &level.TP
	}

	return model.Task{
		TaskID:          generateTaskID(),
		Type:            model.TaskTypeCancelOrder,
		Symbol:          state.Symbol,
		LevelID:         levelID,
		Purpose:         purpose,
		ClientOrderID:   slot.ClientOrderID,
		OrderID:         slot.OrderID,
		Attempt:         0,
		LeaseExpireAtMs: 0, // 由Engine设置
		CreatedAtMs:     model.NowMs(),
		State:           model.TaskStatePending,
	}
}
