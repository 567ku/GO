# Stage 5: 真实Reconcile + WAL + 可观测性

> **原则**：少文档、硬门禁、可审计

---

## 📋 快速导航

- 📄 [执行计划](STAGE5_PLAN.md) - 4个子阶段详细规划
- 🔒 [门禁清单](STAGE5_GATES.md) - Gate-2A/2B/2C/2D 定义
- 📝 [PR模板](PR_TEMPLATE_STAGE4.md) - 复用阶段4模板

---

## 🎯 核心目标

### Stage 5A: 真实Reconcile Query（P0）
- ✅ QueryOpenOrders 真实查询（替代空快照）
- ✅ 断线→RECONCILING→对账→GapScan→RUNNING 全链路
- ✅ Reconcile 超时保护（默认30秒）

### Stage 5B: 最小WAL（P0）
- ✅ WAL 只记 INFLIGHT Task 和 Lease 状态
- ✅ 启动时 Snapshot + WAL 回放恢复一致性
- ✅ WAL 文件大小上限（10MB）+ 自动压缩

### Stage 5C: 可观测性（P0）
- ✅ 关键指标日志：event backlog、task inflight、lease timeout、freeze count、ws health
- ✅ 日志结构化（JSON Lines）
- ✅ 预留 Prometheus 接口

### Stage 5D: 压测与长稳（P1）
- ✅ WS reconnect 1000 次不增 goroutine
- ✅ Engine 吞吐上限（10000 event/s）
- ✅ 24小时长稳无泄漏（可选）

---

## 🔥 硬门禁

| 门禁 | 命令 | 通过标准 |
|------|------|---------|
| **Gate-2A** | `go test ./pkg/engine -run TestReconcile -count=20` | 20次全PASS |
| **Gate-2B** | `go test ./pkg/store -run TestWAL -count=20` | 20次全PASS |
| **Gate-2C** | `go test ./pkg/observability -run TestLogger -v` | 日志结构化 |
| **Gate-2D** | `go test ./pkg/ws -run TestReconnect_Stress -count=1000` | 无泄漏 |

**继承 Stage 4 门禁**：
- Gate-0（本地）：编译+单测+静态检查
- Gate-1（CI）：Linux Race 检测

---

## 📦 交付物

### 新增文件（预计）

```
pkg/engine/
  ├── reconcile_query.go          (234 lines)  # 真实查询实现
  ├── reconcile_query_test.go     (187 lines)  # 对账测试
  ├── wal_replay.go                (156 lines)  # WAL回放
  └── wal_replay_test.go           (213 lines)  # WAL回放测试

pkg/store/
  ├── wal.go                       (187 lines)  # WAL存储
  └── wal_test.go                  (245 lines)  # WAL单测

pkg/observability/
  ├── metrics.go                   (123 lines)  # 指标收集
  └── logger.go                    (98 lines)   # 日志封装

pkg/ws/
  └── stress_test.go               (167 lines)  # WS压测

pkg/engine/
  └── stress_test.go               (189 lines)  # Engine压测
```

**总计**：~1800 行代码

---

### 修改文件（预计）

```
M pkg/engine/reconcile.go         (+45 -12)  # 补齐真实查询
M pkg/engine/reducer.go            (+23 -5)   # WAL写入钩子
M pkg/engine/engine.go             (+34 -8)   # 指标上报
M pkg/ws/manager.go                (+15 -3)   # WS健康日志
```

---

## ⏱️ 时间线

| 阶段 | 开始 | 完成 | 工期 | 责任人 |
|------|------|------|------|--------|
| 5A   | Day 1 | Day 2 | 2天 | Qoder |
| 5B   | Day 3 | Day 4 | 2天 | Qoder |
| 5C   | Day 5 | Day 5 | 1天 | Qoder |
| 5D   | Day 6 | Day 7 | 2天 | Qoder |

**总工期**：7天（1周）

---

## 🚫 两条红线（继承 Stage 4）

1. **v1.3 冻结契约**：不允许随意修改 `pkg/model/types.go`
2. **Engine 写入绕过 reducer**：不允许直接写入状态

---

## 📝 收口标准

每个 Stage 必须提交：

### 1. 门禁命令清单 + 输出粘贴

```bash
# Stage 5A
$ go test ./pkg/engine -run TestReconcile -count=20
ok      gridbot/pkg/engine      12.345s
```

### 2. 变更包 diff/patch

```bash
+ pkg/engine/reconcile_query.go (234 lines)
M pkg/engine/reconcile.go (+45 -12)
```

### 3. 风险与回滚 3 行

```
风险：QueryOpenOrders 可能超时（> 30s）
回滚：feature flag ReconcileOnWSReconnect=false
降级：保留空快照兜底路径
```

---

## 🔄 与 Stage 4 的关系

**继承**：
- PR 模板（[PR_TEMPLATE_STAGE4.md](PR_TEMPLATE_STAGE4.md)）
- 门禁规范（[GATES.md](GATES.md)）
- 两条红线

**新增**：
- 真实 Reconcile Query（替代空快照）
- 最小 WAL（解决恢复后一致性）
- 可观测性（运维闭环）
- 压测与长稳（生产就绪）

---

## ✅ 验收标准

### Stage 5A
- [ ] QueryOpenOrders 查询成功率 > 99%
- [ ] Reconcile 超时保护生效
- [ ] 断线→RECONCILING→RUNNING 时序确定性

### Stage 5B
- [ ] WAL 崩溃恢复 20 次全 PASS
- [ ] WAL 回放幂等性验证
- [ ] WAL 文件大小 < 10MB

### Stage 5C
- [ ] 关键指标全部采集
- [ ] 日志格式符合 JSON Lines
- [ ] 异步日志不阻塞主循环

### Stage 5D
- [ ] WS 1000 次重连无泄漏
- [ ] Engine 吞吐 > 10000 event/s
- [ ] 24 小时长稳无 panic（可选）

---

## 🚀 开始执行

1. **阅读规划**：[STAGE5_PLAN.md](STAGE5_PLAN.md)
2. **检查门禁**：[STAGE5_GATES.md](STAGE5_GATES.md)
3. **创建分支**：`git checkout -b stage5-reconcile`
4. **逐个攻克**：5A → 5B → 5C → 5D

---

**创建时间**：2025-11-26  
**维护人**：@Qoder  
**状态**：规划完成，等待执行
