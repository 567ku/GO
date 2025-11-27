# 阶段4 PR 模板

> **使用说明**：创建 PR 时复制此模板，逐条填写并勾选。未填写完整的 PR 不予合并。

---

## PR Title（格式）

**示例**：
- `stage4A: snapshot/wal atomic save + crash recovery`
- `stage4B: integration tests for consistency (UDS vs RESP)`
- `stage4C: precision injection (remove TODO constants)`
- `ci: add linux race gate`

---

## PR Description（必须填，禁止长篇文档）

### 1) Scope（本 PR 做什么 / 不做什么）

**✅ 做**：
- [ ] ...
- [ ] ...

**❌ 不做（明确排除）**：
- [ ] 不新增策略分支
- [ ] 不改动 v1.3 冻结字段结构（如需改动必须走 patch 清单）

---

### 2) Design constraints（硬约束自检）

- [ ] **Engine 单写者**：所有状态变更仅在 Engine event loop / reducer 内
- [ ] **WS 层只通信**：不做策略、不做对账决策
- [ ] **LastSource 仅枚举值**：RESP/UDS/QUERY（全局禁止字符串直写）
- [ ] **LeaseScan 去重**：同 levelID 扫描周期内最多计数一次

---

### 3) Files changed（关键文件列表）

- `pkg/...`
- `pkg/...`
- `pkg/...`

---

### 4) Test evidence（必须贴命令 + 结果摘要）

**要求**：粘贴"命令 + PASS 输出"，不要截图。

```bash
# 全包单测
go test ./...

# WS 稳定性门禁（50 次重连）
go test ./pkg/ws -run TestReconnect -count=50

# Linux CI only: Race 检测
CGO_ENABLED=1 go test -race ./pkg/...
```

**输出示例**：
```
ok      gridbot/pkg/store       0.738s
ok      gridbot/pkg/engine      2.145s
ok      gridbot/pkg/ws          1.023s
```

---

### 5) Risk & rollback（风险点 + 回滚方式）

**风险点**：
- [ ] ...
- [ ] ...

**回滚**：
- revert commit / feature flag（如有）/ 降级路径（如有）

---

### 6) Review checklist（Reviewer 勾选）

- [ ] 关键路径有单测/集成测覆盖
- [ ] 无 archive/旧文件参与构建（`go test ./...` 绿）
- [ ] 无新增 goroutine 泄漏风险（必要时提 NumGoroutine 断言）
- [ ] 无新增数据竞争（Linux -race 绿）
- [ ] 未违反 v1.3 冻结契约（pkg/model/types.go）
- [ ] 未绕过 reducer 直接写入状态

---

## 阶段4 分支交付标准（每个子阶段必须满足）

### Stage 4A：Snapshot/WAL + Crash Recovery（P0）

**必须新增**：
- [ ] `pkg/store/snapshot.go`（或同等模块），支持：
  - 原子写（tmp→rename）
  - 校验（version/savedAtMs/symbol/prefix）
  - 失败降级：写失败不可阻塞主循环（可异步队列/限频重试）

**必须新增测试**：
- [ ] Snapshot round-trip：写→读→结构一致
- [ ] Crash recovery 演练（最小）：构造 state→persist→load→planner 不产生异常任务

**门禁额外要求**：
```bash
go test ./pkg/store -run TestSnapshot -count=20
```

---

### Stage 4B：一致性集成测试（P0）

**必须新增**：
- [ ] 同一订单同时走两条链路（UDS/RESP），乱序到达仍收敛一致
- [ ] RECONNECT→RECONCILING→RUNNING 端到端时序稳定性断言（避免偶发抖动）

**门禁额外要求**：
```bash
go test ./pkg/engine -run TestE2E -count=20
```

---

### Stage 4C：精度注入（P1，建议尽快做）

**必须完成**：
- [ ] WS Market/UDS 解析 precision 全部来自 `QtyConfig` 注入
- [ ] 删除 hardcode precision 与 TODO 常量路径
- [ ] 单测覆盖：不同 precision 下 ticks 解析一致

---

## ⛔ 两条"永远有效"的红线（Reviewer 一票否决）

1. **v1.3 冻结契约**（`pkg/model/types.go`）随意改字段/语义：❌ 不允许。必须走 patch 清单 + 对应测试更新。
2. **Engine 写入绕过 reducer**：❌ 不允许。状态变更只能通过 reducer 纯函数 or 统一入口。

---

## 提交前自检

- [ ] 我已阅读并填写完整以上所有章节
- [ ] 我已执行本地 Gate-0 门禁（见 `docs/GATES.md`）
- [ ] 我已确认未违反两条红线
- [ ] 我已准备好接受 Code Review

---

**提交人签名**：@yourname  
**提交时间**：YYYY-MM-DD
