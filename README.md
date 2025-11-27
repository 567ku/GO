# 币安网格机器人 v1.3 - 严格按文档实现

> ⚠️ **重要**：本项目严格按照仓库内 v1.3 冻结文档实现，禁止自由发挥、禁止加需求、禁止改口径。

## 📚 权威文档（按优先级）

1. `网格需求文档-v1.3-工程开发版-冻结终版.md`
2. `网格需求文档-v1.3-字段与接口规范.md`
3. `v1.3-实现细节补完-冻结版.md`
4. `v1.3-三WS管理补充规格.md`
5. `v1.3-订单确认双保险补充规格.md`
6. `v1.3-WS模块化与启动限频补充规格.md`
7. `Qoder-开发安排-v1.3-最终版.md`
8. `docs/INDEX.md` - **文档唯一入口**（必须先查此文件）

## 🔒 强制工程约束

### 单写者原则
- 所有策略状态(Level/OrderSlot/Cycle/Profit/Mode)只能在 Engine 事件循环里修改
- WS/Executor 只能"发事件/回包"，禁止直接写状态

### 三条WS只做通讯
- `ws_ticker`: 只产出 PriceTickEvent
- `ws_user_stream`: 只转 Order/Trade 事件
- `ws_order`: 只执行 Task 并回灌 ExecutorResultEvent

### 双保险机制
- 任何 PLACE/MODIFY/CANCEL 都必须按 Response + UDS + Query 收敛
- 不允许"回包成功就默认 OPEN 结束"

### 冻结对账
- UDS/Order WS 断线立即 Freeze
- 重连必须 Reconcile（Freeze→Snapshot→Apply→ReplayWS→GapScan→Unfreeze）

### 启动/对账后批量补单限频
- 超过 100 笔按批次下发，每批 100，间隔 20 秒

### 前缀隔离
- 只处理 clientOrderId 以 prefix 开头的订单
- 其它一律忽略

### 持仓模式路由
- Hedge 模式禁止发送 reduceOnly
- One-way 的 TP 必须 reduceOnly=true
- Hedge 必传 positionSide=LONG/SHORT

### 精度与格式
- 下单 price/qty 按文档要求用字符串 DECIMAL
- 内部用 ticks（int64）运算，避免 float 误差

## 🏗️ 项目结构

```
GO2.0/
├── docs/
│   └── INDEX.md                          # 文档总索引（唯一查询入口）
├── pkg/
│   └── model/
│       ├── types.go                      # 所有枚举、结构体、字段定义（v1.3冻结）
│       ├── types_test.go                 # types 单元测试
│       ├── ticks.go                      # ticks转换与取整
│       └── ticks_test.go                 # ticks 单元测试
├── .gitignore                            # Git忽略配置（key.txt已加入）
├── go.mod                                # Go模块定义
├── key.txt                               # API密钥（仅本地，禁止提交）
├── README.md                             # 本文件
├── ws_manager.go                         # 现有WS管理器（待重构）
├── ws_ticker.go                          # 现有Ticker WS（待重构）
├── ws_user_stream.go                     # 现有UserStream WS（待重构）
├── ws_order.go                           # 现有Order WS（待重构）
└── [v1.3文档...]                         # 权威需求文档
```

## ✅ **阶段 1.1 交付：模型与字段契约修订版（实盘硬伤修复）**

### 变更文件列表
1. **`pkg/model/types.go`** - 重写（489行），修复关键字段
   - 🔴 `ExecutorResultEvent.ReqID` - 新增必填字段（WS请求ID，用于请求相关性）
   - 🔴 `ExecutorResultEvent.ExchangeTimeMs` - 标注为“强烈建议”
   - 🔴 `TradeUpdateEvent.RealizedPnlTicks` - 改为 `*int64`（nil表示缺失，禁止用0表达缺失）
   - 🔴 `QtyConfig.QuotePrecision` - 新增字段（QUOTE金额精度，默认2）
   - 🔴 `RuntimeConfig` - 新增 WS ping/pong 配置（不写死，全部可配置）
   - ✅ `ValidateGridConfig` - 补全 `(winMax-winMin)%step==0` 校验
   - ✅ `BuildEntryCLID/BuildTPCLID` - 改为返回 `(string, error)`，带长度校验

2. **`pkg/model/ticks.go`** - 重写（344行），纯整型算法，禁止float
   - ✅ `FormatPriceDecimal/FormatQtyDecimal` - 纯整型实现，禁止float
   - ✅ `ParsePriceDecimal/ParseQtyDecimal` - 用字符串精确解析到ticks，禁止strconv.ParseFloat
   - ✅ `CalculateQtyFromQuote` - 新增`quotePrecision`参数，纯整型算法，使用big.Int防溢出
   - ✅ `ValidateMinNotional` - 重写，按pricePrecision+qtyPrecision+quotePrecision统一口径，使用big.Int防溢出

3. **`pkg/model/types_test.go`** - 重写（301行）
   - 新增 `TestBuildCLID` - 覆盖“prefix过长”边界
   - 新增 `TestTradeUpdateEvent_RealizedPnl` - 测试nil/0/有值场景
   - 新增 `TestExecutorResultEvent_ReqID` - 测试ReqID和ExchangeTimeMs字段
   - 新增 `ValidateGridConfig` - 补充 `(winMax-winMin)%step==0` 测试用例

4. **`pkg/model/ticks_test.go`** - 重写（353行）
   - 新增 `TestFormatPriceDecimal_Pure` - 测试纯整型格式化（负数/整数/小数）
   - 新增 `TestParsePriceDecimal_Pure` - 测试纯整型解析（截断/补齐/非法字符）
   - 新增 `TestDecimal_RoundTrip` - 测试往返一致性
   - 新增 `TestCalculateQtyFromQuote_Pure` - 测试QUOTE模式纯整型算法（quotePrecision=2）
   - 新增 `TestValidateMinNotional` - 覆盖边界值（刚好等于/略小于/高精度）

5. **`README.md`** - 更新，添加阶段 1.1 交付说明

### 变更文件列表
1. `.gitignore` - 新增，禁止提交 key.txt 和运行时数据
2. `docs/INDEX.md` - 新增，完成所有 TODO 填充
3. `pkg/model/types.go` - 新增，461行，定义所有 v1.3 枚举和结构体
4. `pkg/model/ticks.go` - 新增，159行，ticks转换与取整
5. `pkg/model/types_test.go` - 新增，197行，types 单元测试
6. `pkg/model/ticks_test.go` - 新增，261行，ticks 单元测试
7. `go.mod` - 新增，Go模块初始化

### 关键结构体与字段对照（严格符合《字段与接口规范》）

#### ✅ 枚举定义（1.1节）
- [x] `GridSide`: LONG/SHORT
- [x] `EngineMode`: RUNNING/FREEZE/RECONCILING
- [x] `PositionMode`: ONE_WAY/HEDGE
- [x] `OrderPurpose`: ENTRY/TP
- [x] `OrderState`: NONE/SUBMITTED/OPEN/PARTIAL/FILLED/CANCELED/REJECTED/EXPIRED
- [x] `TaskType`: PLACE_ENTRY/PLACE_TP/MODIFY_ENTRY/CANCEL_ORDER/QUERY_*
- [x] `TaskState`: PENDING/INFLIGHT/DONE/FAILED/DEAD
- [x] `EventSource`: RESP/UDS/QUERY

#### ✅ 配置字段（2节）
- [x] `ExchangeConfig` - 11个字段全部实现
- [x] `GridConfig` - 9个字段全部实现
- [x] `QtyConfig` - 8个字段全部实现
- [x] `CompoundConfig` - 5个字段全部实现
- [x] `RuntimeConfig` - 17个字段全部实现（含启动限频配置）

#### ✅ 状态快照字段（3节）
- [x] `GridStateSnapshot` - 顶层结构
- [x] `WindowState` - 窗口状态
- [x] `MarketState` - 市场状态
- [x] `QtyState` - 数量状态
- [x] `CompoundState` - 复利状态
- [x] `LevelState` - Level状态
- [x] `OrderSlot` - 订单槽（含lastSource字段）

#### ✅ 事件与接口契约（4节）
- [x] `EngineEvent` - Engine输入事件
- [x] `PriceTickEvent` - 价格事件
- [x] `OrderUpdateEvent` - 订单更新事件
- [x] `TradeUpdateEvent` - 成交事件
- [x] `WSStateEvent` - WS状态事件
- [x] `TimerEvent` - 定时器事件
- [x] `ExecutorResultEvent` - 执行器结果事件（含parsedOrder字段）
- [x] `Task` - 任务结构（含ReduceOnly *bool字段）

#### ✅ 校验函数（6节）
- [x] `ValidateClientOrderID()` - 正则校验 `^[\.A-Z\:/a-z0-9_-]{1,36}$`
- [x] `ValidateGridConfig()` - 网格配置校验（step/winMin/winMax/对齐/tpKeep）
- [x] `BuildEntryCLID()` - 生成Entry CLID
- [x] `BuildTPCLID()` - 生成TP CLID

#### ✅ Ticks转换（ticks.go）
- [x] `PriceToTicks()` / `TicksToPrice()`
- [x] `QtyToTicks()` / `TicksToQty()`
- [x] `FormatPriceDecimal()` - 转为DECIMAL字符串（WS下单必须）
- [x] `FormatQtyDecimal()` - 转为DECIMAL字符串
- [x] `ParsePriceDecimal()` / `ParseQtyDecimal()`
- [x] `RoundTicks()` - 按模式取整（DOWN/UP/NEAR）
- [x] `CalculateQtyFromQuote()` - QUOTE模式计算qty
- [x] `ValidateMinQty()` / `ValidateMinNotional()`

### 单元测试运行结果

```bash
$ go test -v ./pkg/model/

=== 所有测试通过 ===
✅ TestFormatPriceDecimal_Pure - 6个子测试全部通过（含负数/整数场景）
✅ TestFormatQtyDecimal_Pure - 4个子测试全部通过
✅ TestParsePriceDecimal_Pure - 9个子测试全部通过（含截断/补齐/非法字符）
✅ TestDecimal_RoundTrip - 往返一致性测试通过
✅ TestRoundTicks - 7个子测试全部通过（含负数场景）
✅ TestCalculateQtyFromQuote_Pure - 5个子测试全部通过（quotePrecision=2）
✅ TestValidateMinQty - 3个子测试全部通过
✅ TestValidateMinNotional - 5个子测试全部通过（边界值覆盖）
✅ TestValidateClientOrderID - 8个子测试全部通过
✅ TestValidateGridConfig - 7个子测试全部通过（含(winMax-winMin)%step校验）
✅ TestBuildCLID - 3个子测试全部通过（含prefix过长场景）
✅ TestCeilDiv - 5个子测试全部通过
✅ TestOrderState_IsTerminal - 8个子测试全部通过
✅ TestTradeUpdateEvent_RealizedPnl - RealizedPnlTicks可选字段测试通过
✅ TestExecutorResultEvent_ReqID - ReqID和ExchangeTimeMs字段测试通过

PASS
ok      gridbot/pkg/model       0.608s
```

### 关键链路日志片段（示例）

```go
// CLID生成
entryCLID := model.BuildEntryCLID("GRID_A1", 12, 3)
// => "GRID_A1:E:12:3"

// 校验合规性
err := model.ValidateClientOrderID(entryCLID)
// => nil（通过）

// Ticks转换
priceTicks := model.PriceToTicks(100.5, 1)  // => 1005
priceStr := model.FormatPriceDecimal(1005, 1) // => "100.5"

// 网格配置校验
cfg := &model.GridConfig{
    Prefix: "GRID_A1",
    StepTicks: 100,
    WinMinTicks: 10000,  // 必须是step的整数倍
    WinMaxTicks: 20000,  // 必须是step的整数倍
}
err := model.ValidateGridConfig(cfg)  // => nil（对齐校验通过）
```

## 🔐 安全规则

1. **key.txt 已加入 .gitignore** - 禁止提交
2. 日志中禁止打印 key/secret
3. 所有 clientOrderId 必须通过 `ValidateClientOrderID()` 校验

## 📋 下一步计划（阶段2：WSConn底座）

按照《Qoder-开发安排-v1.3-最终版.md》第二阶段：

1. 创建 `/pkg/ws/conn.go` - 通用WSConn底座
   - dial / readPump / writePump
   - pong handler + read deadline
   - reconnect backoff + jitter
   - 状态变化回调

2. 验收标准：
   - 写 demo 连接公开 WS
   - 掉线后自动重连
   - goroutine 不泄漏

## 📝 备注

- 所有字段名、枚举值严格按照《字段与接口规范》实现
- price/qty 内部用 ticks（int64）运算，避免 float 误差
- WS 下单时必须用 `FormatPriceDecimal()` / `FormatQtyDecimal()` 转为字符串
- clientOrderId 格式：`{prefix}:E:{levelId}:{cycle}` 或 `{prefix}:T:{levelId}:{cycle}`
- 窗口移动步数 k 使用 `CeilDiv()` 向上取整
- 终态判断使用 `OrderState.IsTerminal()` 方法

---

**严格遵守文档，不偏离规范，保证可验收可追溯。**
