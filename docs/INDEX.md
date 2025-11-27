# docs/INDEX.md（文档索引 - 唯一查询入口）
> 规则：Qoder 实现任何模块前，必须先从这里定位到"权威章节"，不得凭记忆乱写参数。

## 1. 下单/改单/撤单/查询（WS API）
来源：`U本位合约 WebSocket API完整文档.md`
- **order.place**：章节位置：**四、交易相关 WebSocket API → 4.1 下单接口（order.place）** （第 310 行起）
- **order.modify**：章节位置：**四、交易相关 WebSocket API → 4.2 改单接口（order.modify）** （第 418 行起）
- **order.cancel**：章节位置：**四、交易相关 WebSocket API → 4.3 撤单接口（order.cancel）** （第 534 行起）
- **queryOrder/queryOpenOrders**：章节位置：**四、交易相关 WebSocket API → 4.4 查询接口** （第 600+ 行）

## 2. 用户数据流（listenKey）
来源：`U本位合约 WebSocket API完整文档.md` 或 `U本位合约REST API完整文档（交易相关）.md`
- **创建 listenKey**：章节位置：**三、账户数据流（私有 WebSocket）→ 3.2 listenKey 管理 → 3.2.1 生成 listenKey（USER_STREAM）** （第 174 行起）
- **keepalive listenKey**：章节位置：**三、账户数据流（私有 WebSocket）→ 3.2 listenKey 管理 → 3.2.2 续期 listenKey（KEEPALIVE_USER_STREAM）**
- **关闭 listenKey**：章节位置：**三、账户数据流（私有 WebSocket）→ 3.2 listenKey 管理 → 3.2.3 关闭 listenKey（CLOSE_USER_STREAM）**
- **推送事件类型（ORDER_TRADE_UPDATE 等）**：章节位置：**三、账户数据流（私有 WebSocket）→ 3.3 推送事件** （第 200+ 行）

## 3. 错误码与处理策略
来源：`U本位合约错误代码分类及详解.md`
- **限频（-1003）**：章节位置：**二、核心错误代码分类与详解 → 限频/WAF 相关错误 → -1003**
- **订单不存在（-2011）**：章节位置：**二、核心错误代码分类与详解 → 订单操作相关错误 → -2011**
- **positionSide/持仓模式冲突（-4061）**：章节位置：**二、核心错误代码分类与详解 → 持仓模式相关错误 → -4061**
- **timestamp/recvWindow（-1021）**：章节位置：**二、核心错误代码分类与详解 → 签名与时间戳相关错误 → -1021**

## 4. 安全与密钥
- **key.txt**：只允许本地读取；**必须加入 .gitignore**；日志中**禁止打印 key/secret**。

## 5. 与 v1.3 需求的对应关系
- **v1.3 冻结终版**：`网格需求文档-v1.3-工程开发版-冻结终版.md`（根目录）
- **字段与接口规范**：`网格需求文档-v1.3-字段与接口规范.md`（根目录）
- **实现细节补完**：`v1.3-实现细节补完-冻结版.md`（根目录）
- **WS 模块补充规格**：`v1.3-三WS管理补充规格.md`（根目录）
- **订单确认双保险**：`v1.3-订单确认双保险补充规格.md`（根目录）
- **WS模块化与启动限频**：`v1.3-WS模块化与启动限频补充规格.md`（根目录）
- **开发安排**：`Qoder-开发安排-v1.3-最终版.md`（根目录）

## 6. WebSocket 连接维护
来源：`U本位合约 WebSocket API完整文档.md`
- **ping/pong 机制**：章节位置：**一、WebSocket API 基本信息 → 1.2 连接维护（ping/pong 机制）** （第 23 行起）
  - 服务器每 3 分钟发送 1 个 ping
  - 客户端需在 10 分钟内回复 pong，payload 需与 ping 一致
- **连接有效期**：章节位置：**一、WebSocket API 基本信息 → 1.1 核心配置** （第 11 行）
  - 单次连接有效期 24 小时，到期后自动断开

## 7. 字段类型与格式
来源：`U本位合约 WebSocket API完整文档.md`
- **DECIMAL 字段**：章节位置：**一、WebSocket API 基本信息 → 1.1 核心配置**
  - **DECIMAL 参数必须为 JSON 字符串**（price/quantity 等）
  - INT 参数为 JSON 整数
- **newClientOrderId 合规**：章节位置：`网格需求文档-v1.3-字段与接口规范.md` → **0.3 clientOrderId 合规**
  - 必须符合正则：`^[\.A-Z\:/a-z0-9_-]{1,36}$`

## 8. 持仓模式与下单参数路由
来源：`网格需求文档-v1.3-工程开发版-冻结终版.md`
- **持仓模式查询**：章节位置：**4. 持仓模式与下单参数路由（冻结规则）→ 4.1 启动/重连时查询持仓模式**
  - REST API: `GET /fapi/v1/positionSide/dual`
- **参数路由规则**：章节位置：**4. 持仓模式与下单参数路由（冻结规则）→ 4.2 参数路由（必须按此实现）**
  - One-way: Entry 不传 reduceOnly，TP 必须 reduceOnly=true，positionSide=BOTH
  - Hedge: **禁止发送 reduceOnly**，必须传 positionSide=LONG/SHORT
