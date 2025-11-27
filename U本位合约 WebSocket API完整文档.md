# **主题**：Binance U 本位合约 WebSocket API 完整文档，包括基本信息（核心配置、连接维护、请求格式、响应格式、访问限制、身份验证方法等）、公共 WebSocket API（最新价格查询）、账户数据流（listenKey 管理及过期推送）、交易相关 WebSocket API（下单接口等）

# U 本位合约 WebSocket API 完整文档

文档说明：本文档整合 U 本位合约（USDⓈ-M Futures）WebSocket API 核心功能，包含基础配置、公共接口、账户数据流、交易接口及推送事件，与之前的 REST API 文档保持格式统一，适用于开发者集成实时交易与数据订阅功能。

## 一、WebSocket API 基本信息

### 1.1 核心配置

生产环境 Base URL（公域接口）：`wss://ws-fapi.binance.com/ws-fapi/v1`

测试网 Base URL（公域接口）：`wss://testnet.binancefuture.com/ws-fapi/v1`

账户数据流 Base URL（私域接口）：`wss://fstream.binance.com`

连接有效期：单次连接有效期 24 小时，到期后自动断开

时间标准：所有时间戳均为 UTC 毫秒级

字段规则：字段名称和值区分大小写；INT 参数为 JSON 整数，DECIMAL 参数为 JSON 字符串

### 1.2 连接维护（ping/pong 机制）

服务器每 3 分钟发送 1 个 ping 消息

客户端需在 10 分钟内回复 pong 消息，且 payload 需与 ping 一致，否则连接断开

允许主动发送空 payload 的 pong 消息，但不保证连接持续

### 1.3 请求格式

通用请求结构

```JSON

```

请求字段说明

|名称|类型|是否必需|描述|
|---|---|---|---|
|id|INT/STRING/null|YES|匹配请求与响应的唯一标识，可重复使用（同一时间不重复即可）|
|method|STRING|YES|请求函数名，支持版本前缀（如 "v3/order.place"）|
|params|OBJECT|NO|请求参数集合，无参数时可省略|
### 1.4 响应格式

成功响应示例

```JSON

```

失败响应示例

```JSON

```

响应字段说明

|名称|类型|是否必需|描述|
|---|---|---|---|
|id|INT/STRING/null|YES|与请求 id 一致，用于匹配|
|status|INT|YES|响应状态码（200 为成功，4xx 为客户端错误，5xx 为服务端错误）|
|result|OBJECT/ARRAY|YES|成功时返回的核心数据|
|error|OBJECT|NO|失败时返回的错误信息（code 为错误码，msg 为描述）|
|rateLimits|ARRAY|NO|速率限制状态，包含限制类型、间隔、上限、已用次数|
### 1.5 访问限制

速率限制与 REST API 共享，规则一致

WebSocket 握手尝试消耗 5 权重

ping/pong 帧限制：每秒最多 5 次

rateLimits 字段控制：可通过连接字符串（`?returnRateLimits=false`）或请求参数隐藏 / 显示

### 1.6 身份验证方法

仅支持 Ed25519 密钥进行会话身份验证，核心方法如下：

#### 1.6.1 会话登录（session.logon）

功能：验证 API 密钥，关联当前 WebSocket 连接

权重：2

请求示例：

```JSON

```

响应示例：

```JSON

```

参数说明：

|名称|类型|是否必需|描述|
|---|---|---|---|
|apiKey|STRING|YES|你的 API 密钥|
|signature|STRING|YES|签名结果（按参数字母排序生成）|
|timestamp|INT|YES|毫秒时间戳|
|recvWindow|INT|NO|最大 60000 毫秒|
#### 1.6.2 查询会话状态（session.status）

功能：查询当前连接的认证状态

权重：2

请求示例：

```JSON

```

响应示例：同 session.logon 成功响应（未认证时 apiKey 和 authorizedSince 为 null）

#### 1.6.3 会话退出（session.logout）

功能：清除当前连接的 API 密钥关联（连接不关闭）

权重：2

请求示例：

```JSON

```

响应示例：同 session.logon 成功响应（apiKey 和 authorizedSince 为 null）

### 1.7 签名与安全（SIGNED 请求示例）

推荐使用 Ed25519 API 密钥，Python 示例代码如下：

```Python

```

## 二、公共 WebSocket API（公域接口）

### 2.1 最新价格查询（ticker.price）

接口描述：返回单个或所有合约的最近成交价格

请求方法：`ticker.price`

请求权重：单交易对 1，无交易对 2

请求参数：

|名称|类型|是否必需|描述|
|---|---|---|---|
|symbol|STRING|NO|交易对（如 BTCUSDT），不传则返回所有交易对|
请求示例（查询单个交易对）：

```JSON

```

响应示例（单个交易对）：

```JSON

```

响应示例（所有交易对）：

```JSON

```

## 三、账户数据流（私有 WebSocket）

### 3.1 核心说明

账户数据流需通过`listenKey`订阅，`listenKey`有效期 60 分钟

订阅地址：`wss://fstream.binance.com/ws/<listenKey>`（替换`<listenKey>`为实际值）

单一账户的单一连接推送保证时间序，建议用`E`字段（事件时间）排序

剧烈行情下优先使用该流获取订单、持仓信息，延迟低于 REST API

### 3.2 listenKey 管理（REST 方式）

#### 3.2.1 生成 listenKey（USER_STREAM）

HTTP 请求：`POST /fapi/v1/listenKey`

请求权重：1

请求参数：无

响应示例：

```JSON

```

#### 3.2.2 延长 listenKey 有效期（USER_STREAM）

HTTP 请求：`PUT /fapi/v1/listenKey`

请求权重：1

请求参数：无

响应示例：

```JSON

```

#### 3.2.3 关闭 listenKey（USER_STREAM）

HTTP 请求：`DELETE /fapi/v1/listenKey`

请求权重：1

请求参数：无

响应示例：`{}`

### 3.3 listenKey 管理（WebSocket 方式）

#### 3.3.1 生成 listenKey（userDataStream.start）

请求方法：`userDataStream.start`

请求权重：1

请求参数：

|名称|类型|是否必需|描述|
|---|---|---|---|
|apiKey|STRING|YES|你的 API 密钥|
请求示例：

```JSON

```

响应示例：

```JSON

```

#### 3.3.2 延长 listenKey 有效期（userDataStream.ping）

请求方法：`userDataStream.ping`

请求权重：1

请求参数：无（需已通过 session.logon 认证）

请求示例：

```JSON

```

响应示例：

```JSON

```

#### 3.3.3 关闭 listenKey（userDataStream.stop）

请求方法：`userDataStream.stop`

请求权重：1

请求参数：无（需已通过 session.logon 认证）

请求示例：

```JSON

```

响应示例：

```JSON

```

### 3.4 listenKey 过期推送

事件类型：`listenKeyExpired`

触发条件：当前连接的`listenKey`过期（仅连接中有效`listenKey`过期时推送）

响应示例：

```JSON

```

说明：收到该事件后，数据流停止更新，需重新生成`listenKey`并订阅

## 四、交易相关 WebSocket API（私域接口）

### 4.1 下单接口（order.place）

#### 接口描述

创建 U 本位合约订单（需先通过 session.logon 完成身份认证）

#### 请求方法

`order.place`

#### 请求权重

0

#### 请求参数

|名称|类型|是否必需|描述|
|---|---|---|---|
|apiKey|STRING|YES|你的 API 密钥（需与 session.logon 一致）|
|signature|STRING|YES|签名结果（按参数字母排序生成，参考 1.7 签名示例）|
|timestamp|LONG|YES|毫秒时间戳|
|symbol|STRING|YES|交易对（如 BTCUSDT）|
|side|ENUM|YES|买卖方向：SELL（卖）、BUY（买）|
|positionSide|ENUM|NO|持仓方向：单向持仓模式默认且仅可填 BOTH；双向持仓模式必填，可选 LONG/SHORT|
|type|ENUM|YES|订单类型：LIMIT/MARKET/STOP/TAKE_PROFIT/STOP_MARKET/TAKE_PROFIT_MARKET/TRAILING_STOP_MARKET|
|reduceOnly|STRING|NO|是否仅平仓：true/false；非双开模式默认 false；双开模式不接受此参数；closePosition 为 true 时不支持|
|quantity|DECIMAL|NO|下单数量；closePosition 为 true 时不支持此参数|
|price|DECIMAL|NO|委托价格；与 priceMatch 互斥，不可同时传|
|newClientOrderId|STRING|NO|用户自定义订单号，不可重复，需满足正则：^[.A-Z:/a-z0-9_-]{1,36}$|
|stopPrice|DECIMAL|NO|触发价；仅 STOP/STOP_MARKET/TAKE_PROFIT/TAKE_PROFIT_MARKET 需传|
|closePosition|STRING|NO|是否全部平仓：true/false；仅支持 STOP_MARKET/TAKE_PROFIT_MARKET；与 quantity 互斥，自带仅平仓属性|
|activationPrice|DECIMAL|NO|追踪止损激活价格；仅 TRAILING_STOP_MARKET 需传，默认当前市场价格|
|callbackRate|DECIMAL|NO|追踪止损回调比例（1 代表 1%）；仅 TRAILING_STOP_MARKET 需传，取值范围 [0.1, 10]|
|timeInForce|ENUM|NO|订单有效方式|
|workingType|ENUM|NO|stopPrice 触发类型：MARK_PRICE（标记价格）/CONTRACT_PRICE（合约最新价），默认 CONTRACT_PRICE|
|priceProtect|STRING|NO|条件单触发保护：TRUE/FALSE，默认 FALSE；仅 STOP/STOP_MARKET/TAKE_PROFIT/TAKE_PROFIT_MARKET 需传|
|newOrderRespType|ENUM|NO|响应类型：ACK/RESULT，默认 ACK|
|priceMatch|ENUM|NO|价格匹配模式：OPPONENT/OPPONENT_5/OPPONENT_10/OPPONENT_20/QUEUE/QUEUE_5/QUEUE_10/QUEUE_20；与 price 互斥|
|selfTradePreventionMode|ENUM|NO|自成交保护模式：NONE/EXPIRE_TAKER/EXPIRE_MAKER/EXPIRE_BOTH，默认 NONE|
|goodTillDate|LONG|NO|GTD 订单自动取消时间；timeInForce 为 GTD 时必传；时间戳仅保留秒级，需大于当前时间+600s 且小于 253402300799000|
|recvWindow|LONG|NO|签名有效窗口时间（默认无需传）|
#### 特殊说明

1. 不同订单类型的强制参数要求：

|    订单类型|    强制参数|
|---|---|
|    LIMIT|    timeInForce + quantity +（price 或 priceMatch）|
|    MARKET|    quantity|
|    STOP/TAKE_PROFIT|    quantity + stopPrice|
|    STOP_MARKET/TAKE_PROFIT_MARKET|    stopPrice +（price 或 priceMatch）|
|    TRAILING_STOP_MARKET|    callbackRate|
2. 条件单触发规则：

    - 止损单（STOP/STOP_MARKET）：买入时最新价/标记价 ≥ stopPrice；卖出时最新价/标记价 ≤ stopPrice

    - 止盈单（TAKE_PROFIT/TAKE_PROFIT_MARKET）：买入时最新价/标记价 ≤ stopPrice；卖出时最新价/标记价 ≥ stopPrice

    - 触发保护（priceProtect=true）：触发时标记价与合约最新价价差需 ≤ 对应交易对的 triggerProtect 阈值（可通过 GET /fapi/v1/exchangeInfo 查询）

3. 追踪止损单（TRAILING_STOP_MARKET）特殊条件：

    - 买入订单：activationPrice 必须小于当前最新价，否则报错 `-2021: Order would immediately trigger`

    - 卖出订单：activationPrice 必须大于当前最新价，否则报错 `-2021: Order would immediately trigger`

    - 触发逻辑：买入时合约价/标记价低于 activationPrice 后，回升回调比例对应的价格；卖出时合约价/标记价高于 activationPrice 后，回落回调比例对应的价格

4. newOrderRespType=RESULT 时：

    - MARKET 订单直接返回成交结果

    - 特殊 timeInForce 的 LIMIT 订单直接返回成交或过期拒绝结果

5. closePosition=true 特殊逻辑：

    - 仅支持 STOP_MARKET/TAKE_PROFIT_MARKET

    - 触发后平掉当前所有对应方向仓位（卖单平多头，买单平空头）

    - 不支持 quantity 和 reduceOnly 参数

    - 双开模式下：LONG 方向不支持 BUY 订单；SHORT 方向不支持 SELL 订单

#### 请求示例

```JSON

```

#### 响应示例

```JSON

```



## 4.2

# 修改订单 (TRADE)

## 接口描述

修改订单功能，当前只支持限价（LIMIT）订单修改，修改后会在撮合队列里重新排序

## 方式

`order.modify`

## 请求

```JavaScript

{
    "id": "c8c271ba-de70-479e-870c-e64951c753d9",
    "method": "order.modify",
    "params": {
        "apiKey": "HMOchcfiT9ZRZnhjp2XjGXhsOBd6msAhKz9joQaWwZ7arcJTlD2hGPHQj1lGdTjR",
        "orderId": 328971409,
        "origType": "LIMIT",
        "positionSide": "SHORT",
        "price": "43769.1",
        "priceMatch": "NONE",
        "quantity": "0.11",
        "side": "SELL",
        "symbol": "BTCUSDT",
        "timestamp": 1703426755754,
        "signature": "d30c9f0736a307f5a9988d4a40b688662d18324b17367d51421da5484e835923"
    }
}
```

## 请求权重

10s order rate limit(X-MBX-ORDER-COUNT-10S)为1 1min order rate limit(X-MBX-ORDER-COUNT-1M)为1 IP rate limit(x-mbx-used-weight-1m)为0

## 请求参数

> - `orderId` 与 `origClientOrderId` 必须至少发送一个，同时发送则以 order id为准
> 
> - `quantity` 与 `price` 均必须发送，这点和 dapi 修改订单不同
> 
> - 当新订单的`quantity` 或 `price`不满足PRICE_FILTER / PERCENT_FILTER / LOT_SIZE限制，修改会被拒绝，原订单依旧被保留
> 
> - 订单会在下列情况下被取消：
> 
>     - 原订单被部分执行且新订单`quantity` <= `executedQty`
> 
>     - 原订单是`GTX`，新订单的价格会导致订单立刻执行
> 
> - 同一订单修改次数最多10000次
> 
> 

## 响应示例

```JavaScript

{
    "id": "c8c271ba-de70-479e-870c-e64951c753d9",
    "status": 200,
    "result": {
        "orderId": 328971409,
        "symbol": "BTCUSDT",
        "status": "NEW",
        "clientOrderId": "xGHfltUMExx0TbQstQQfRX",
        "price": "43769.10",
        "avgPrice": "0.00",
        "origQty": "0.110",
        "executedQty": "0.000",
        "cumQty": "0.000",
        "cumQuote": "0.00000",
        "timeInForce": "GTC",
        "type": "LIMIT",
        "reduceOnly": false,
        "closePosition": false,
        "side": "SELL",
        "positionSide": "SHORT",
        "stopPrice": "0.00",
        "workingType": "CONTRACT_PRICE",
        "priceProtect": false,
        "origType": "LIMIT",
        "priceMatch": "NONE",
        "selfTradePreventionMode": "NONE",
        "goodTillDate": 0,
        "updateTime": 1703426756190
    },
    "rateLimits": [
        {
            "rateLimitType": "ORDERS",
            "interval": "SECOND",
            "intervalNum": 10,
            "limit": 300,
            "count": 1
        },
        {
            "rateLimitType": "ORDERS",
            "interval": "MINUTE",
            "intervalNum": 1,
            "limit": 1200,
            "count": 1
        },
        {
            "rateLimitType": "REQUEST_WEIGHT",
            "interval": "MINUTE",
            "intervalNum": 1,
            "limit": 2400,
            "count": 1
        }
    ]
}
```

4.3

# 撤销订单 (TRADE)

## 接口描述

撤销订单

## 方式

`order.cancel`

## 请求

```JavaScript

{
           "id": "5633b6a2-90a9-4192-83e7-925c90b6a2fd",
    "method": "order.cancel", 
    "params": { 
      "apiKey": "HsOehcfih8ZRxnhjp2XjGXhsOBd6msAhKz9joQaWwZ7arcJTlD2hGOGQj1lGdTjR", 
      "orderId": 283194212,
      "symbol": "BTCUSDT", 
      "timestamp": 1703439070722, 
      "signature": "b09c49815b4e3f1f6098cd9fbe26a933a9af79803deaaaae03c29f719c08a8a8" 
    }
}
```

## 请求权重

**1**

## 请求参数

> - `orderId` 与 `origClientOrderId` 必须至少发送一个
> 
> 

## 响应示例

```JavaScript

{
  "id": "5633b6a2-90a9-4192-83e7-925c90b6a2fd",
  "status": 200,
  "result": {
    "clientOrderId": "myOrder1",
    "cumQty": "0",
    "cumQuote": "0",
    "executedQty": "0",
    "orderId": 283194212,
    "origQty": "11",
    "origType": "TRAILING_STOP_MARKET",
    "price": "0",
    "reduceOnly": false,
    "side": "BUY",
    "positionSide": "SHORT",
    "status": "CANCELED",
    "stopPrice": "9300",                
    "closePosition": false,  
    "symbol": "BTCUSDT",
    "timeInForce": "GTC",
    "type": "TRAILING_STOP_MARKET",
    "activatePrice": "9020",            
    "priceRate": "0.3",                
    "updateTime": 1571110484038,
    "workingType": "CONTRACT_PRICE",
    "priceProtect": false,           
    "priceMatch": "NONE",              
    "selfTradePreventionMode": "NONE",
    "goodTillDate": 0                 
  },
  "rateLimits": [
    {
      "rateLimitType": "REQUEST_WEIGHT",
      "interval": "MINUTE",
      "intervalNum": 1,
      "limit": 2400,
      "count": 1
    }
  ]
}
```

4.4

# 查询订单 (USER_DATA)

## 接口描述

查询订单状态

- 请注意，如果订单满足如下条件，不会被查询到：

    - 订单的最终状态为 `CANCELED` 或者 `EXPIRED` **并且** 订单没有任何的成交记录 **并且** 订单生成时间 + 3天 < 当前时间

    - 订单创建时间 + 90天 < 当前时间

## 方式

`order.status`

## 请求

```JavaScript

{
    "id": "0ce5d070-a5e5-4ff2-b57f-1556741a4204",
    "method": "order.status",
    "params": {
        "apiKey": "HMOchcfii9ZRZnhjp2XjGXhsOBd6msAhKz9joQaWwZ7arcJTlD2hGPHQj1lGdTjR",
        "orderId": 328999071,
        "symbol": "BTCUSDT",
        "timestamp": 1703441060152,
        "signature": "ba48184fc38a71d03d2b5435bd67c1206e3191e989fe99bda1bc643a880dfdbf"
    }
}
```

## 请求权重

**1**

## 请求参数

注意:

> - 至少需要发送 `orderId` 与 `origClientOrderId`中的一个
> 
> - `orderId`在`symbol`维度是自增的
> 
> 

## 响应示例

```JavaScript

{
          "avgPrice": "0.00000",                                // 平均成交价
          "clientOrderId": "abc",                                // 用户自定义的订单号
          "cumQuote": "0",                                        // 成交金额
          "executedQty": "0",                                        // 成交量
          "orderId": 1573346959,                                // 系统订单号
          "origQty": "0.40",                                        // 原始委托数量
          "origType": "TRAILING_STOP_MARKET",        // 触发前订单类型
          "price": "0",                                                // 委托价格
          "reduceOnly": false,                                // 是否仅减仓
          "side": "BUY",                                                // 买卖方向
          "positionSide": "SHORT",                         // 持仓方向
          "status": "NEW",                                        // 订单状态
          "stopPrice": "9300",                            // 触发价，对`TRAILING_STOP_MARKET`无效
          "closePosition": false,             // 是否条件全平仓
          "symbol": "BTCUSDT",                                // 交易对
          "time": 1579276756075,                                // 订单时间
          "timeInForce": "GTC",                                // 有效方法
          "type": "TRAILING_STOP_MARKET",                // 订单类型
          "activatePrice": "9020",                        // 跟踪止损激活价格, 仅`TRAILING_STOP_MARKET` 订单返回此字段
          "priceRate": "0.3",                                        // 跟踪止损回调比例, 仅`TRAILING_STOP_MARKET` 订单返回此字段
          "updateTime": 1579276756075,                // 更新时间
          "workingType": "CONTRACT_PRICE",    // 条件价格触发类型
         "priceProtect": false               // 是否开启条件单触发保护
}
```

## 五、账户信息推送事件

### 5.1 余额与持仓更新（ACCOUNT_UPDATE）

事件描述：账户资金、仓位、保证金模式变动时推送（无变动不推送）

事件类型：`ACCOUNT_UPDATE`

响应示例：

```JSON

```

### 5.2 订单交易更新（ORDER_TRADE_UPDATE）

事件描述：订单创建、成交、状态变更时推送

事件类型：`ORDER_TRADE_UPDATE`

响应示例：

```JSON

```

### 5.3 精简交易推送（TRADE_LITE）

事件描述：仅推送交易相关核心字段，延迟低于 ORDER_TRADE_UPDATE

事件类型：`TRADE_LITE`

响应示例：

```JSON

```
> （注：文档部分内容可能由 AI 生成）