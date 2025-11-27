# docs/INDEX.md（模板：把分散文档“钉死成入口”）
> 规则：Qoder 实现任何模块前，必须先从这里定位到“权威章节”，不得凭记忆乱写参数。

## 1. 下单/改单/撤单/查询（WS API）
来源：`U本位合约 WebSocket API完整文档.md`
- order.place：章节位置：TODO（在该文件内搜索“order.place”并填写页码/标题）
- order.modify：章节位置：TODO（搜索“order.modify”）
- order.cancel：章节位置：TODO（搜索“order.cancel”）
- queryOrder/queryOpenOrders：章节位置：TODO

## 2. 用户数据流（listenKey）
来源：`U本位合约REST API完整文档（交易相关）.md` 或对应 UserDataStreams 章节
- 创建 listenKey：TODO（搜索“listenKey”“userDataStream”）
- keepalive listenKey：TODO
- 关闭 listenKey：TODO
- 推送事件类型（ORDER_TRADE_UPDATE 等）：TODO

## 3. 错误码与处理策略
来源：`U本位合约错误代码分类及详解.md`
- 限频：TODO（搜索 -1003）
- 订单不存在：TODO（搜索 -2011）
- positionSide/持仓模式冲突：TODO（搜索 -4061）
- timestamp/recvWindow：TODO（搜索 -1021）

## 4. 安全与密钥
- key.txt：只允许本地读取；必须加入 .gitignore；日志中禁止打印 key/secret。

## 5. 与 v1.3 需求的对应关系
- v1.3 冻结终版：`网格需求文档-v1.3-工程开发版-冻结终版.md`（根目录或 docs）
- 字段与接口规范：`网格需求文档-v1.3-字段与接口规范.md`
- WS 模块补充规格：`v1.3-三WS管理补充规格.md`
- 订单确认双保险：`v1.3-订单确认双保险补充规格.md`
- 本补充：`v1.3-WS模块化与启动限频补充规格.md`
