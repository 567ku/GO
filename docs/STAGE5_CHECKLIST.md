# Stage 5 执行检查清单

> **使用方法**：每完成一项勾选 ✅，失败标记 ❌，跳过标记 ⏭️

---

## Stage 5A: 真实Reconcile Query

### 开发任务
- [ ] 实现 `pkg/engine/reconcile_query.go`（QueryOpenOrders）
- [ ] 实现 `pkg/engine/reconcile_query.go`（AccountPosition）
- [ ] 修改 `pkg/engine/reconcile.go`（调用真实查询）
- [ ] 修改 `pkg/engine/reducer.go`（Reconcile完成后触发GapScan）
- [ ] 创建 `pkg/engine/reconcile_query_test.go`（单元测试）

### 门禁验证
- [ ] Gate-2A.1: `go test ./pkg/engine -run TestReconcile_QueryOpenOrders -count=20` ✅
- [ ] Gate-2A.2: `go test ./pkg/engine -run TestReconcile_E2E_Timeline -v` ✅
- [ ] Gate-2A.3: `go test ./pkg/engine -run TestReconcile_Timeout -v` ✅

### 交付物
- [ ] 门禁命令清单 + 输出粘贴
- [ ] 变更包 diff/patch
- [ ] 风险与回滚 3 行

### Reviewer 检查
- [ ] QueryOpenOrders 区分"自己的前缀"
- [ ] Reconcile 期间禁止写 Task
- [ ] 对账完成后触发 GapScan
- [ ] 超时保护生效（默认30秒）

---

## Stage 5B: 最小WAL

### 开发任务
- [ ] 实现 `pkg/store/wal.go`（WAL存储管理器）
- [ ] 实现 `pkg/store/wal_test.go`（WAL单元测试）
- [ ] 实现 `pkg/engine/wal_replay.go`（WAL回放逻辑）
- [ ] 实现 `pkg/engine/wal_replay_test.go`（WAL回放测试）
- [ ] 修改 `pkg/engine/engine.go`（Bootstrap添加WAL回放）
- [ ] 修改 `pkg/engine/reducer.go`（Task状态变更时写WAL）

### 门禁验证
- [ ] Gate-2B.1: `go test ./pkg/store -run TestWAL_CrashRecovery -count=20` ✅
- [ ] Gate-2B.2: `go test ./pkg/engine -run TestWAL_ReplayIdempotent -count=20` ✅
- [ ] Gate-2B.3: `go test ./pkg/store -run TestWAL_FileSizeLimit -v` ✅

### 交付物
- [ ] 门禁命令清单 + 输出粘贴
- [ ] 变更包 diff/patch
- [ ] 风险与回滚 3 行

### Reviewer 检查
- [ ] WAL 只记 INFLIGHT Task 和 Lease
- [ ] WAL 支持原子追加（append-only）
- [ ] WAL 文件大小 < 10MB
- [ ] WAL 回放幂等性

---

## Stage 5C: 可观测性

### 开发任务
- [ ] 实现 `pkg/observability/metrics.go`（指标收集器）
- [ ] 实现 `pkg/observability/logger.go`（结构化日志）
- [ ] 修改 `pkg/engine/engine.go`（添加指标上报）
- [ ] 修改 `pkg/ws/manager.go`（添加WS健康日志）

### 门禁验证
- [ ] Gate-2C.1: `go test ./pkg/observability -run TestLogger_StructuredOutput -v` ✅
- [ ] Gate-2C.2: `go test ./pkg/observability -run TestMetrics_Collection -v` ✅
- [ ] Gate-2C.3: `go test ./pkg/observability -run TestLogger_AsyncPerformance -v` ✅

### 交付物
- [ ] 门禁命令清单 + 输出粘贴
- [ ] 变更包 diff/patch
- [ ] 风险与回滚 3 行

### Reviewer 检查
- [ ] 日志格式符合 JSON Lines
- [ ] 关键指标全部采集（5个指标）
- [ ] 异步日志不阻塞主循环
- [ ] 日志级别可动态调整

---

## Stage 5D: 压测与长稳

### 开发任务
- [ ] 创建 `pkg/ws/stress_test.go`（WS压测）
- [ ] 创建 `pkg/engine/stress_test.go`（Engine压测）

### 门禁验证
- [ ] Gate-2D.1: `go test ./pkg/ws -run TestReconnect_Stress -count=1000 -timeout 30m` ✅
- [ ] Gate-2D.2: `go test ./pkg/engine -run TestEngine_Throughput -v` ✅
- [ ] Gate-2D.3: `go test ./pkg/engine -run TestEngine_Backlog -v` ✅
- [ ] Gate-2D.4: `go test ./pkg/engine -run TestEngine_LongRunning -timeout 24h` ⏭️（可选）

### 交付物
- [ ] 门禁命令清单 + 输出粘贴
- [ ] 变更包 diff/patch
- [ ] 风险与回滚 3 行

### Reviewer 检查
- [ ] goroutine 数量可控（< 100）
- [ ] 内存占用可控（< 500MB）
- [ ] CPU 占用可控（< 50%）
- [ ] Event backlog 可控（< 1000）

---

## 总体检查

### 代码质量
- [ ] 所有新增代码通过 `go vet ./pkg/...`
- [ ] 所有新增代码通过 `go test ./pkg/...`
- [ ] 所有新增代码通过 `CGO_ENABLED=1 go test -race ./pkg/...`（CI）

### 文档完整性
- [ ] 每个 Stage 有门禁输出粘贴
- [ ] 每个 Stage 有变更包 diff
- [ ] 每个 Stage 有风险与回滚说明

### PR 规范
- [ ] PR Title 符合格式（如 `stage5A: reconcile query + gap scan`）
- [ ] PR Description 填写完整（引用 PR_TEMPLATE_STAGE4.md）
- [ ] 未违反两条红线
- [ ] 至少 1 位 Reviewer 批准

---

## 进度追踪

| Stage | 状态 | 完成日期 | 备注 |
|-------|------|---------|------|
| 5A    | ⏳   | -       |      |
| 5B    | ⏳   | -       |      |
| 5C    | ⏳   | -       |      |
| 5D    | ⏳   | -       |      |

**图例**：
- ⏳ 进行中
- ✅ 已完成
- ❌ 失败
- ⏭️ 跳过

---

**创建时间**：2025-11-26  
**维护人**：@Qoder
