# Qoder 开发安排（v1.3 一次性交付路线）
> 目标：让 Qoder 按步骤开发，不走偏、不漏项、每一步都有可验收产物。

---

## 0) 预置输入（你已准备）
仓库根目录约定：
```
/docs/
  U本位合约 WebSocket API完整文档.md
  U本位合约REST API完整文档（交易相关）.md
  U本位合约错误代码分类及详解.md
  INDEX.md          # 新增：文档总索引（唯一入口）
/config/
  config.yaml
/key.txt            # 本地读取，禁止提交
/cmd/gridbot/main.go
/pkg/ws/
  ws_manager.go
  ws_ticker.go
  ws_user_stream.go
  ws_order.go
/pkg/engine/
  engine.go
  reconcile.go
  gapscan.go
/pkg/executor/
  executor.go
/pkg/model/
  types.go          # 枚举/struct/schema
  ticks.go          # ticks转换与取整
/data/
  grid_state.json
  grid_wal.log
```

---

## 1) 第一阶段：模型与字段契约（不写业务，先写“唯一真源”）
产物：`/pkg/model/types.go` + `ValidateConfig()` + `ValidateCLID()`
必须包含：
- v1.3《字段与接口规范》里的所有枚举/结构体/字段
- ticks 表达（priceTicks/qtyTicks 与 precision）
- clientOrderId 正则与长度校验
验收：编译通过 + 单元测试覆盖校验函数

---

## 2) 第二阶段：WSConn 底座（通用连接，不带业务）
产物：`/pkg/ws/conn.go`
必须包含：
- dial / readPump / writePump
- pong handler + read deadline
- reconnect backoff + jitter
- 状态变化回调（onStateChange）
验收：写一个 demo 连接公开 WS，掉线后自动重连，goroutine 不泄漏

---

## 3) 第三阶段：三条 WS 客户端（薄封装）
### 3.1 ws_ticker.go
- 订阅行情
- 转成 PriceTickEvent 发 engineEventCh

### 3.2 ws_user_stream.go
- REST 获取 listenKey + keepalive
- 连接 UDSWS，解析订单/成交推送
- 断线/重连发 WSStateEvent

### 3.3 ws_order.go
- 接收 Task（place/modify/cancel/query）
- pending(reqId->taskId) + 请求超时
- 回包转 ExecutorResultEvent

验收：三条都能独立连通；收到消息能正确输出 event；OrderWS 能完成一次下单回包解析（可用 testnet 或 mock）。

---

## 4) 第四阶段：ws_manager.go（编排层）
必须实现：
- 启动/停止三条 WS
- 健康监控与统一重连策略
- 把 WSStateEvent 发给 Engine
- listenKey 续期策略
- 不写交易逻辑

验收：断 UDS 或 OrderWS -> Engine 收到 DISCONNECTED 事件（先用假 Engine 打印即可）。

---

## 5) 第五阶段：Engine 单写者骨架 + mode gate
产物：`/pkg/engine/engine.go`
必须实现：
- 事件队列（PriceTick/UDS/ExecutorResult/Timer/WSState）
- EngineMode：RUNNING/FREEZE/RECONCILING
- Gate：FREEZE 时拒绝 PLACE/CANCEL/MODIFY，只允许 QUERY
- Level-serial 令牌（每个 level 最多 1 in-flight）
- Lease 扫描定时器：超时回滚任务重试

验收：构造 2 个 level，注入事件，确保状态只在 OnEvent 中改。

---

## 6) 第六阶段：Reconcile 固定流程（Freeze->Snapshot->Apply->ReplayWS->GapScan->Unfreeze）
产物：`/pkg/engine/reconcile.go`
必须实现：
- Snapshot：openOrders/recentTrades/positionMode（prefix 过滤）
- Apply：终态优先收敛 OrderSlot/Level
- ReplayWS：对账期间缓存事件回放
- GapScan：输出 TaskPlan
验收：模拟“本地缺单/交易所多单”能生成正确 cancel/place 计划

---

## 7) 第七阶段：窗口移动 + 撤/留规则 + GapScan 规则
产物：`/pkg/engine/gapscan.go`
必须实现：
- k=ceil 一次性移动窗口
- 区间外 Entry 全撤
- TP 保留扩展窗口 ±5格
- 窗口移动完成后立即 GapScan
验收：给定窗口、订单集合，输出任务列表与预期一致

---

## 8) 第八阶段：启动批量布单限频（100 单 + 20 秒）
产物：`PlanDispatcher`
必须实现：
- bootstrap_batch_size=100
- bootstrap_batch_cooldown_sec=20
- 仅对 start/reconcile 后的大批量计划启用
验收：模拟 200 个 place 任务，实际发出分两批，间隔 20s

---

## 9) 第九阶段：复利闭环记账
产物：`/pkg/engine/compound.go`（或 engine 内）
必须实现：
- 只在 level Entry FILLED + TP FILLED 时记账
- poolA>=threshold 触发 entryQty 增加，受 max 约束
验收：模拟 3 次盈利闭环，entryQty 正确阶梯增长

---

## 10) 第十阶段：最小实盘冒烟测试（小额，严格日志）
范围：1 个币对、10 个格子、BASE qty 很小  
关注：
- 掉线 Freeze/Reconcile 能恢复
- 不碰非前缀订单
- 不出现同 level 双 OPEN
- 启动补单不再“只下 100 就停”

---

## 交付定义（Qoder 每阶段必须输出）
- 代码文件 + 单元测试（至少对：ticks/clid/ceilDiv/gapscan/reconcile）
- 日志样例（关键链路）
- 一个“如何运行”的 README（包含 config、key 读取、testnet 开关）


---

## 强制前置：先阅读并落实“实现细节补完（冻结版）”
> 这一步是为了解决“同一需求不同人写法不一致”的最后一公里问题。

在进入任何业务编码前，必须先做到：
1) 把《v1.3-实现细节补完-冻结版.md》A~F 里写死的校验与公式落在代码里（至少：
   - winMin/winMax 对齐校验
   - Level 生成规则
   - LONG/SHORT 启动挂单判定
   - QUOTE->qty 换算与取整
   - poolA 记账口径 + 固定增量复利公式
   - ExecutorResultEvent 字段完整性
   - WS 缓存上限与溢出策略
   - recentTrades 时间窗公式
   - backoff(exp_200ms_5s)公式
   - Preflight 清单
2) 为以上规则写单元测试（否则后续一定分叉）。

对应文档：
- 《v1.3-实现细节补完-冻结版.md》
