# 阶段4 门禁清单（Gates）

> **强制执行**：所有 PR 必须通过以下门禁，否则阻断合并。

---

## 🔴 Gate-0（本地必过，阻断合并）

**执行时机**：提交 PR 前，本地必须全部通过。

### 1. 编译 + 全包单测

```bash
go test ./...
```

**通过标准**：
- 所有测试 PASS
- 无 FAIL / panic / timeout
- 耗时合理（< 5分钟）

---

### 2. WS 稳定性门禁（50 次重连，不泄漏）

```bash
go test ./pkg/ws -run TestReconnect -count=50
```

**通过标准**：
- 50 次全部 PASS
- 无 goroutine 泄漏
- 无内存泄漏（可选：加 `-memprofile`）

---

### 3. 格式/静态检查（二选一，团队统一）

**选项A（轻量）**：
```bash
go vet ./...
```

**选项B（更强，建议 CI 用 golangci-lint）**：
```bash
golangci-lint run ./...
```

**通过标准**：
- 无 vet/lint 错误
- 无未处理的 TODO/FIXME（除非有 issue 跟踪）

---

## 🟠 Gate-1（CI 必过，阻断合并）

**执行时机**：PR 提交后，CI 自动执行。

### 4. Linux Race 门禁（只在 CI 执行）

```bash
CGO_ENABLED=1 go test -race ./pkg/...
```

**通过标准**：
- 所有测试 PASS
- 无数据竞争（WARNING: DATA RACE）
- 无死锁（all goroutines are asleep）

> **备注**：如果你们 CI 运行 `./...` 也没问题，推荐 `./pkg/...` 先锁住范围，避免未来 tools/examples 误伤。

---

## 🟡 Gate-2（阶段专属门禁，按需执行）

### Stage 4A：Snapshot 稳定性门禁

```bash
go test ./pkg/store -run TestSnapshot -count=20
```

**通过标准**：
- 20 次全部 PASS
- 无文件损坏
- 无并发写入冲突

---

### Stage 4B：一致性集成测试门禁

```bash
go test ./pkg/engine -run TestE2E -count=20
```

**通过标准**：
- 20 次全部 PASS
- 无状态不一致
- 无时序抖动

---

### Stage 4C：精度注入门禁

```bash
go test ./pkg/ws -run TestPrecision -v
```

**通过标准**：
- 所有 precision 测试 PASS
- 无硬编码 precision 残留
- 无 TODO 常量路径

---

## 🔵 可选门禁（性能/压测，非阻断）

### 性能基准测试

```bash
go test -bench=. -benchmem ./pkg/...
```

**建议标准**：
- 无明显性能退化（> 20%）
- 内存分配合理（无异常增长）

---

### 压测门禁（长时间运行）

```bash
go test ./pkg/ws -run TestStress -timeout 30m
```

**建议标准**：
- 30 分钟稳定运行
- 无 goroutine 持续增长
- 无内存泄漏

---

## ⛔ 两条"永远有效"的红线（Reviewer 一票否决）

### 红线1：v1.3 冻结契约

**禁止行为**：
- ❌ 随意修改 `pkg/model/types.go` 字段名/类型/语义
- ❌ 修改已冻结的枚举值（如 `EngineMode`/`OrderState`）
- ❌ 修改 JSON tag（如 `json:"orderId"` → `json:"order_id"`）

**允许行为**：
- ✅ 新增字段（必须有默认值/向后兼容）
- ✅ 走 patch 清单流程（需同步更新测试）

---

### 红线2：Engine 写入绕过 reducer

**禁止行为**：
- ❌ 在 handler/executor 中直接写入 `engine.state.Levels[i].Entry.State = ...`
- ❌ 在 WS 层直接修改状态
- ❌ 绕过 reducer 的任何写入操作

**允许行为**：
- ✅ 所有状态变更通过 `state = reducer.ApplyXxx(state, event)` 调用
- ✅ Engine event loop 内统一写入

---

## 门禁执行顺序（推荐）

```
graph TD
    A[开发完成] --> B[Gate-0.1: go test ./...]
    B --> C{通过?}
    C -->|否| D[修复失败测试]
    D --> B
    C -->|是| E[Gate-0.2: WS 稳定性]
    E --> F{通过?}
    F -->|否| G[修复泄漏/竞争]
    G --> E
    F -->|是| H[Gate-0.3: vet/lint]
    H --> I{通过?}
    I -->|否| J[修复静态检查]
    J --> H
    I -->|是| K[提交 PR]
    K --> L[Gate-1: Linux Race]
    L --> M{通过?}
    M -->|否| N[修复数据竞争]
    N --> L
    M -->|是| O[Gate-2: 阶段专属]
    O --> P{通过?}
    P -->|否| Q[修复阶段门禁]
    Q --> O
    P -->|是| R[Code Review]
    R --> S{Reviewer 批准?}
    S -->|否| T[修改代码]
    T --> B
    S -->|是| U[合并]
    
    style C fill:#fff4e6,stroke:#ffa94d
    style F fill:#fff4e6,stroke:#ffa94d
    style I fill:#fff4e6,stroke:#ffa94d
    style M fill:#ffe8e8,stroke:#ff6b6b
    style P fill:#ffe8e8,stroke:#ff6b6b
    style S fill:#e8f5e9,stroke:#66bb6a
    style U fill:#e8f5e9,stroke:#66bb6a
```

---

## 门禁失败常见问题

### Q1: `go test ./...` 失败但 `go test ./pkg/...` 通过？

**原因**：根目录或 `tools/` 下有测试失败。

**解决**：
```bash
# 定位失败的包
go test ./... -v | grep FAIL

# 修复或隔离
go test ./pkg/...  # 只测核心包
```

---

### Q2: WS 稳定性门禁偶发失败？

**原因**：goroutine 泄漏或连接未正确关闭。

**解决**：
```bash
# 查看 goroutine 数量
go test ./pkg/ws -run TestReconnect -count=1 -v | grep NumGoroutine

# 检查 Stop() 方法是否幂等
# 检查 pumpWG.Wait() 是否死锁
```

---

### Q3: Linux Race 门禁失败但本地 Windows 通过？

**原因**：Windows 不支持 `-race` 标志，某些竞争只在 Linux 出现。

**解决**：
```bash
# 在 Linux/WSL/Docker 中执行
CGO_ENABLED=1 go test -race ./pkg/...

# 定位竞争位置
# 修复 atomic/mutex 保护
```

---

### Q4: 精度注入门禁失败？

**原因**：仍有硬编码 precision 或 TODO 未移除。

**解决**：
```bash
# 搜索残留
grep -r "const.*Precision" pkg/ws/
grep -r "TODO.*precision" pkg/ws/

# 确保从 ManagerConfig 注入
grep -r "pricePrecision int" pkg/ws/
```

---

## 门禁豁免流程（仅特殊情况）

**适用场景**：
- 红线2修复需要临时绕过 reducer（极少数情况）
- 性能优化需要暂时放宽门禁（需记录 issue）

**申请流程**：
1. 在 PR 中明确说明豁免原因
2. 提供风险评估和回滚方案
3. 需至少 2 位 Reviewer 批准
4. 合并后 7 天内必须提交补丁修复

---

## 门禁版本历史

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2025-11-26 | 初始版本（阶段4） |

---

**维护人**：@团队负责人  
**最后更新**：2025-11-26

```

```

```
