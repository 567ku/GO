# Stage 5 执行计划

> **原则**：少文档、硬门禁、可审计

---

## 优先级排序

| Stage | 任务 | P级 | 预计工期 | 门禁 |
|-------|------|-----|---------|------|
| **5A** | 真实Reconcile Query | P0 | 2天 | 20次重复测试 |
| **5B** | 最小WAL | P0 | 2天 | 崩溃恢复演练 |
| **5C** | 可观测性 | P0 | 1天 | 日志结构化验证 |
| **5D** | 压测与长稳 | P1 | 2天 | 1000次重连无泄漏 |

---

## Stage 5A: 真实Reconcile Query（P0）

### Scope

**✅ 做**：
- [ ] 实现 `QueryOpenOrders()` 真实查询（替代空快照）
- [ ] 实现 `AccountPosition()` 账户仓位查询
- [ ] 断线→RECONCILING→对账→GapScan→RUNNING 全链路
- [ ] Reconcile 与 GapScan 联动（对账后立即扫描缺口）

**❌ 不做**：
- ❌ 不做全量历史订单查询（只查 open orders）
- ❌ 不做账户资金流水查询
- ❌ 不做交易历史回放

### Design constraints

- [ ] QueryOpenOrders 必须区分"自己的前缀"（防止污染其他网格）
- [ ] Reconcile 期间禁止新增 PLACE/CANCEL Task
- [ ] 对账完成后立即触发 GapScan（确保窗口内有单）
- [ ] Reconcile 超时保护（默认30秒，可配置）

### Files changed

**新增**：
- `pkg/engine/reconcile_query.go` - 真实查询实现
- `pkg/engine/reconcile_query_test.go` - 对账集成测试

**修改**：
- `pkg/engine/reconcile.go` - 补齐真实查询调用
- `pkg/engine/reducer.go` - 添加 Reconcile 完成后触发 GapScan

### Test evidence

**门禁命令**：
```bash
# Gate-2A: Reconcile 稳定性（20次重复）
go test ./pkg/engine -run TestReconcile_QueryOpenOrders -count=20 -timeout 60s

# Gate-2A: 全链路时序测试
go test ./pkg/engine -run TestReconcile_E2E_Timeline -v
```

**通过标准**：
- 20次全部 PASS
- Reconcile → GapScan → RUNNING 时序确定性
- 无 goroutine 泄漏
- 超时保护生效

### Risk & rollback

**风险点**：
- QueryOpenOrders 可能返回大量订单（> 100），需分页处理
- Reconcile 期间如果断线，需支持重新进入 RECONCILING

**回滚**：
- revert commit（保留空快照兜底）
- feature flag: `ReconcileOnWSReconnect = false`（降级为空对账）

---

## Stage 5B: 最小WAL（P0）

### Scope

**✅ 做**：
- [ ] WAL 只记录 INFLIGHT Task 和 Lease 状态（最小可控范围）
- [ ] 启动时优先读 Snapshot，然后回放 WAL 恢复 INFLIGHT 任务
- [ ] WAL 文件格式：JSON Lines（每行一条记录）
- [ ] WAL 定期压缩（清理已终态的 Task）

**❌ 不做**：
- ❌ 不做全量事件 WAL（只记关键状态变更）
- ❌ 不做多文件 WAL 分片（单文件即可）
- ❌ 不做 WAL 快照合并（保持简单）

### Design constraints

- [ ] WAL 必须支持原子追加（append-only）
- [ ] WAL 文件大小上限（默认 10MB，超过则压缩）
- [ ] WAL 回放必须幂等（重复回放不改变最终状态）
- [ ] WAL 与 Snapshot 版本号一致性校验

### Files changed

**新增**：
- `pkg/store/wal.go` - WAL 存储管理器
- `pkg/store/wal_test.go` - WAL 单元测试
- `pkg/engine/wal_replay.go` - WAL 回放逻辑
- `pkg/engine/wal_replay_test.go` - WAL 回放测试

**修改**：
- `pkg/engine/engine.go` - Bootstrap() 添加 WAL 回放
- `pkg/engine/reducer.go` - Task 状态变更时写入 WAL

### Test evidence

**门禁命令**：
```bash
# Gate-2B: WAL 崩溃恢复演练
go test ./pkg/store -run TestWAL_CrashRecovery -count=20

# Gate-2B: WAL 回放幂等性
go test ./pkg/engine -run TestWAL_ReplayIdempotent -count=20
```

**通过标准**：
- 20次全部 PASS
- kill -9 后 INFLIGHT 任务恢复
- WAL 回放幂等（重复回放不出错）
- WAL 文件大小可控（< 10MB）

### Risk & rollback

**风险点**：
- WAL 写入失败可能导致 Task 丢失（需异步队列缓冲）
- WAL 文件损坏可能导致无法回放（需校验和保护）

**回滚**：
- revert commit（降级为纯 Snapshot）
- feature flag: `WALEnabled = false`

---

## Stage 5C: 可观测性（P0）

### Scope

**✅ 做**：
- [ ] 关键指标结构化日志：
  - event backlog（事件积压数量）
  - task inflight（飞行中任务数）
  - lease timeout count（超时次数）
  - freeze count（冻结次数）
  - ws health（WS 健康状态）
- [ ] 日志统一格式（JSON Lines）
- [ ] 预留 Prometheus metrics 接口（暂不实现）

**❌ 不做**：
- ❌ 不做 Prometheus 服务端集成（先日志后监控）
- ❌ 不做 Grafana Dashboard（P1 任务）
- ❌ 不做 Alerting（P1 任务）

### Design constraints

- [ ] 日志必须结构化（key=value，方便解析）
- [ ] 关键路径禁止同步日志（使用异步 buffer）
- [ ] 日志级别可动态调整（DEBUG/INFO/WARN/ERROR）
- [ ] 日志文件大小上限（默认 100MB，自动轮转）

### Files changed

**新增**：
- `pkg/observability/metrics.go` - 指标收集器
- `pkg/observability/logger.go` - 结构化日志封装

**修改**：
- `pkg/engine/engine.go` - 添加指标上报
- `pkg/ws/manager.go` - 添加 WS 健康日志

### Test evidence

**门禁命令**：
```bash
# Gate-2C: 日志结构化验证
go test ./pkg/observability -run TestLogger_StructuredOutput -v

# Gate-2C: 指标采集验证
go test ./pkg/observability -run TestMetrics_Collection -v
```

**通过标准**：
- 日志格式符合 JSON Lines
- 关键指标全部上报
- 无同步日志阻塞主循环

### Risk & rollback

**风险点**：
- 日志量过大可能影响性能（需控制日志级别）

**回滚**：
- revert commit（降级为简单 fmt.Printf）

---

## Stage 5D: 压测与长稳（P1）

### Scope

**✅ 做**：
- [ ] WS reconnect 1000 次不增 goroutine
- [ ] Engine 吞吐上限测试（10000 event/s）
- [ ] Engine 积压上限测试（1000 pending events）
- [ ] 长稳测试（24小时运行无泄漏）

**❌ 不做**：
- ❌ 不做生产环境压测（仅本地/CI）
- ❌ 不做多机房分布式压测

### Design constraints

- [ ] goroutine 数量必须可控（< 100）
- [ ] 内存占用必须可控（< 500MB）
- [ ] CPU 占用必须可控（< 50%）
- [ ] Event backlog 必须可控（< 1000）

### Files changed

**新增**：
- `pkg/ws/stress_test.go` - WS 压测
- `pkg/engine/stress_test.go` - Engine 压测

### Test evidence

**门禁命令**：
```bash
# Gate-2D: WS 1000次重连无泄漏
go test ./pkg/ws -run TestReconnect_Stress -count=1000 -timeout 30m

# Gate-2D: Engine 吞吐上限
go test ./pkg/engine -run TestEngine_Throughput -v

# Gate-2D: 24小时长稳（可选）
go test ./pkg/engine -run TestEngine_LongRunning -timeout 24h
```

**通过标准**：
- 1000次重连后 goroutine 数量稳定（< 100）
- Engine 吞吐 > 10000 event/s
- 24小时运行无 panic/leak

### Risk & rollback

**风险点**：
- 压测可能暴露隐藏的并发问题

**回滚**：
- 无需回滚（压测不改代码）

---

## 收口标准（每个 Stage）

### 门禁命令清单 + 输出粘贴

**格式**：
```bash
# Stage 5A
$ go test ./pkg/engine -run TestReconcile -count=20
ok      gridbot/pkg/engine      12.345s

# Stage 5B
$ go test ./pkg/store -run TestWAL -count=20
ok      gridbot/pkg/store       8.765s

# Stage 5C
$ go test ./pkg/observability -run TestLogger -v
=== RUN   TestLogger_StructuredOutput
--- PASS: TestLogger_StructuredOutput (0.01s)
PASS
ok      gridbot/pkg/observability       0.123s

# Stage 5D
$ go test ./pkg/ws -run TestReconnect_Stress -count=1000
ok      gridbot/pkg/ws  1234.567s
```

---

### 变更包 diff/patch

**格式**：
```bash
# 新增文件
+ pkg/engine/reconcile_query.go (234 lines)
+ pkg/store/wal.go (187 lines)

# 修改文件
M pkg/engine/reconcile.go (+45 -12 lines)
M pkg/engine/engine.go (+23 -5 lines)
```

---

### 风险与回滚 3 行

**格式**：
```
风险：QueryOpenOrders 可能超时（> 30s）
回滚：feature flag ReconcileOnWSReconnect=false
降级：保留空快照兜底路径
```

---

## 总体时间线

| 阶段 | 开始日期 | 完成日期 | 责任人 |
|------|---------|---------|--------|
| 5A   | Day 1   | Day 2   | Qoder  |
| 5B   | Day 3   | Day 4   | Qoder  |
| 5C   | Day 5   | Day 5   | Qoder  |
| 5D   | Day 6   | Day 7   | Qoder  |

**总工期**：7 天（1周）

---

## PR 模板引用

**每个 Stage 的 PR 必须引用**：
- `docs/PR_TEMPLATE_STAGE4.md`（复用阶段4模板）
- `docs/GATES.md`（门禁规范）

**逐条勾选**：
- [ ] Scope 清晰（做什么/不做什么）
- [ ] Design constraints 自检通过
- [ ] Test evidence 粘贴命令输出
- [ ] Risk & rollback 填写完整
- [ ] 未违反两条红线

---

**创建时间**：2025-11-26  
**维护人**：@Qoder
