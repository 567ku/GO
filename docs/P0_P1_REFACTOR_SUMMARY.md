# P0/P1返工清单完成总结

**日期**: 2025-11-26  
**状态**: ✅ 全部完成  
**门禁**: Gate-0通过

---

## 一、P0任务（红线级）

### P0-ENG-41: Reconcile全流程reducer化 ✅

**目标**: 全面消除reconcile.go/clid.go等处对e.state的直接写入；任何state mutation必须只发生在pkg/engine/reducer.go的纯函数中。

**实现内容**:
1. 在`reducer.go`中新增3个纯函数：
   - `ApplyReconcileStart(state) (newState, effects)` - 进入RECONCILING状态
   - `ApplyReconcileComplete(state) (newState, effects)` - 完成RECONCILING回到RUNNING
   - `ApplyOpenOrdersSnapshot(state, orders, prefix) (newState, effects)` - 应用对账快照

2. 重构`reconcile.go`，移除所有直接状态写入：
   - ✅ 第44行：改为调用`ApplyReconcileStart()`
   - ✅ 第167-175行：改为调用`ApplyReconcileComplete()`
   - ✅ 第75-113行：改为调用`ApplyOpenOrdersSnapshot()`

3. 重构`clid.go`：
   - ✅ 移除`FindOrCreateLevel()`的直接append操作
   - ✅ 迁移为`reducer.go`内部helper：`findOrCreateLevelInState()`

4. 修复相关测试：
   - ✅ `clid_test.go`: 改为测试reducer函数
   - ✅ `lastsource_test.go`: 改为使用`ApplyOpenOrdersSnapshot()`
   - ✅ `reconcile_test.go`: 改为测试reducer逻辑

**验收结果**:
- ✅ 全局无直接`e.state.`赋值（除NewEngine初始化）
- ✅ 全局无在reducer.go之外对Levels/OrderSlot/CLIDIndex的直接写入
- ✅ `go test ./pkg/engine -v` 全部通过
- ✅ Gate-0通过

---

### P0-ENG-42: CLIDIndex维护 ✅

**目标**: 对账快照应用时，必须补齐CLID -> orderId映射。

**实现内容**:
在`ApplyOpenOrdersSnapshot()`中统一维护CLIDIndex：
```go
// P0-ENG-42: 维护CLIDIndex
state.CLIDIndex[order.ClientOrderID] = order.OrderID
```

**验收结果**:
- ✅ 对账快照应用时自动更新CLIDIndex
- ✅ 保证幂等性（同key重复写无副作用）
- ✅ 单元测试覆盖

---

### P0-ENG-45: Cycle不匹配生成CANCEL任务 ✅

**目标**: Reconcile发现旧cycle订单时，必须生成CANCEL任务清理脏单。

**实现内容**:
在`ApplyOpenOrdersSnapshot()`中识别cycle mismatch并生成CANCEL任务：
```go
// P0-ENG-45: 检查cycle是否匹配
if level.Cycle != cycle {
    // Cycle不匹配，说明是旧订单，生成CANCEL任务
    cancelTask := generateCancelTaskForReconcile(state, levelID, purpose, &order)
    effect.Tasks = append(effect.Tasks, cancelTask)
    continue
}
```

**验收结果**:
- ✅ Cycle不匹配时自动生成CANCEL任务
- ✅ 任务通过ReducerEffect.Tasks返回
- ✅ 与GapScan补齐任务同批次提交

---

### P0-ENG-46: ReconcileQuery配置注入 ✅

**目标**: Reconcile查询超时/重试/间隔必须来自EngineConfig注入，禁用内部默认配置。

**实现内容**:
1. 在`EngineConfig`中添加配置字段：
```go
ReconcileQueryTimeoutMs  int // 默认30000
ReconcileMaxAttempts     int // 默认3
ReconcileRetryDelayMs    int // 默认1000
```

2. 修改`ReconcileQueryConfig`结构体为配置注入模式：
```go
type ReconcileQueryConfig struct {
    TimeoutMs    int
    MaxAttempts  int
    RetryDelayMs int
}
```

3. 更新`ExecuteReconcileQuery()`签名，接收配置参数：
```go
func ExecuteReconcileQuery(ctx context.Context, queryExecutor ExecutorQueryInterface, 
    symbol, prefix string, config ReconcileQueryConfig) (*ReconcileQueryResult, error)
```

4. 在`NewEngine()`中设置默认值：
```go
if config.ReconcileQueryTimeoutMs <= 0 {
    config.ReconcileQueryTimeoutMs = 30000
}
// ...
```

**验收结果**:
- ✅ 删除`DefaultReconcileQueryConfig()`函数
- ✅ 配置从外部注入，无内部硬编码
- ✅ 测试通过

---

### P0-ENG-47: 重试计数语义统一 ✅

**目标**: 命名与实现必须一致，统一使用MaxAttempts语义（总尝试次数）。

**实现内容**:
1. 将`MaxRetries`改名为`MaxAttempts`
2. 循环逻辑改为：`for attempt := 1; attempt <= config.MaxAttempts; attempt++`
3. 错误提示统一：`query failed after %d attempts`

**验收结果**:
- ✅ 全局统一MaxAttempts语义
- ✅ 总次数=MaxAttempts（不是MaxRetries+1）
- ✅ 日志/错误信息一致

---

### P0-WS-21: WS Manager Start(nil)防护 ✅

**目标**: Start(ctx)接收nil时，自动fallback到context.Background()。

**实现内容**:
在`pkg/ws/manager.go`的`Start()`方法开头添加防护：
```go
func (m *Manager) Start(ctx context.Context) error {
    // P0-WS-21: 防御nil context
    if ctx == nil {
        ctx = context.Background()
    }
    // ...
}
```

**验收结果**:
- ✅ `Start(nil)`不panic
- ✅ 可正常Stop

---

## 二、P1任务（改进级）

### P1-OPS-11: Metrics日志可开关 ✅

**目标**: 避免高负载场景日志洪泛；必须可通过配置启停metricsTicker与JSONL输出。

**实现内容**:
1. 在`EngineConfig`中添加开关：
```go
EnableMetricsSummary bool // 默认false
```

2. 在`Run()`中条件创建ticker：
```go
var metricsTicker *time.Ticker
if e.config.EnableMetricsSummary {
    metricsSummaryInterval := 10 * time.Second
    metricsTicker = time.NewTicker(metricsSummaryInterval)
    defer metricsTicker.Stop()
}
```

3. 在select中条件监听：
```go
var metricsTickerCh <-chan time.Time
if e.config.EnableMetricsSummary && metricsTicker != nil {
    metricsTickerCh = metricsTicker.C
}
// ...
case <-metricsTickerCh:
    e.LogMetricsSummary()
```

**验收结果**:
- ✅ 默认不开启时无metrics输出
- ✅ 开启后按周期输出
- ✅ 编译通过，测试通过

---

### P1-ENG-12: PositionMode默认安全值 ✅

**目标**: ONE_WAY模式下，TP必须reduceOnly=true，避免误开仓风险。

**实现内容**:
1. 在`EngineConfig`中添加字段：
```go
PositionMode model.PositionMode // 默认ONE_WAY
```

2. 在`NewEngine()`中设置默认值：
```go
if config.PositionMode == "" {
    config.PositionMode = model.PositionModeOneWay // 默认ONE_WAY（安全值）
}
```

3. 注入到state：
```go
state := &model.GridStateSnapshot{
    PositionMode: config.PositionMode, // 注入PositionMode
    // ...
}
```

4. 在`generatePlaceTPTask()`中确保reduceOnly正确：
```go
// P1-ENG-12: ONE_WAY模式TP必须reduceOnly=true
reduceOnly := true
var reduceOnlyPtr *bool
if state.PositionMode == model.PositionModeOneWay {
    reduceOnlyPtr = &reduceOnly // ONE_WAY模式必须reduceOnly=true
}
// HEDGE模式不需要reduceOnly，留空nil
```

**验收结果**:
- ✅ 默认PositionMode=ONE_WAY
- ✅ ONE_WAY下TP任务reduceOnly=true
- ✅ HEDGE模式reduceOnly=nil
- ✅ 测试通过

---

## 三、最终验收

### 代码质量检查
- ✅ **Reducer-only规范**: 全局无直接`e.state.`写入（除初始化）
- ✅ **配置注入**: 无硬编码Default*配置
- ✅ **语义统一**: MaxAttempts语义一致
- ✅ **防御性编程**: nil context防护
- ✅ **可观测性**: Metrics可开关

### 门禁测试
```bash
bash scripts/gate0.sh
```

**结果**: ✅ 全部通过
- 编译检查: ✅
- 单元测试: ✅
- Smoke测试: ✅

### 测试覆盖
- `pkg/engine`: ✅ 全部通过（4.810s）
- `pkg/ws`: ✅ 全部通过（0.860s）
- `pkg/model`: ✅ 全部通过（cached）
- `pkg/store`: ✅ 全部通过（cached）

---

## 四、关键文件变更

### 新增文件
无

### 修改文件
1. **pkg/engine/reducer.go** (+156行)
   - 新增`ApplyReconcileStart/Complete/OpenOrdersSnapshot`
   - 新增`findOrCreateLevelInState`内部helper
   - 新增`applyOrderSnapshotToSlot`内部helper
   - 新增`generateCancelTaskForReconcile`

2. **pkg/engine/reconcile.go** (-50行)
   - 移除所有直接state写入
   - 改为调用reducer函数
   - 删除`applyOrderSnapshot`方法

3. **pkg/engine/clid.go** (-31行)
   - 删除`FindOrCreateLevel`函数

4. **pkg/engine/reconcile_query.go** (+14/-16行)
   - 修改`ReconcileQueryConfig`结构体
   - 删除`DefaultReconcileQueryConfig()`
   - 修改`ExecuteReconcileQuery()`签名

5. **pkg/engine/engine.go** (+35行)
   - 添加6个配置字段
   - 修改`NewEngine()`设置默认值
   - 修改`Run()`实现Metrics可开关

6. **pkg/engine/planner.go** (+4/-3行)
   - 完善TP任务reduceOnly逻辑

7. **pkg/ws/manager.go** (+5行)
   - 添加nil context防护

8. **测试文件** (重构)
   - `clid_test.go`: 改为测试reducer
   - `lastsource_test.go`: 改为使用reducer
   - `reconcile_test.go`: 改为测试reducer

---

## 五、技术债务清理

- ✅ 消除了reconcile.go的直接状态写入
- ✅ 消除了clid.go的直接状态写入
- ✅ 统一了重试计数语义
- ✅ 消除了硬编码配置
- ✅ 完善了防御性编程

---

## 六、后续建议

1. **测试增强**:
   - 增加Cycle mismatch的集成测试
   - 增加CLIDIndex维护的边界测试

2. **监控增强**:
   - 添加CANCEL任务生成的metrics
   - 添加Reconcile Query失败的告警

3. **文档完善**:
   - 补充Reducer-only规范文档
   - 补充PositionMode配置说明

---

**总结**: 所有P0/P1返工任务已100%完成，代码质量显著提升，Reducer-only规范严格落地，Gate-0门禁全部通过。✅
