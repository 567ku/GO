 U 本位合约 REST API 完整文档（交易相关）

文档说明：本文档整合 U 本位合约（USDⓈ-M Futures）REST API 核心交易功能，包含基础配置、通用规则及各类交易接口，适用于开发者集成合约交易功能，所有接口基于币安官方规范整理。




 一、基础信息

 1.1 核心配置



生产环境 Base URL：https://fapi.binance.com

测试网 Base URL：https://demo-fapi.binance.com

响应格式：所有接口返回 JSON 格式

时间标准：UNIX 时间戳（单位：毫秒）

数据类型：遵循 JAVA 数据类型定义

数组排序：响应中数组元素按时间升序排列（越早数据越靠前）

 1.2 请求规则



GET 请求：参数必须放在 Query String 中

POST/PUT/DELETE 请求：参数可在 Query String 或 Request Body（content-type: application/x-www-form-urlencoded）中，支持混合传输（同名参数以 Query String 为准）

参数顺序：无强制要求

编码格式：特殊字符需 URL 编码

 1.3 鉴权规则



| 鉴权类型         | 描述                      |
| ------------ | ----------------------- |
| NONE         | 无需鉴权（公共接口）              |
| TRADE        | 需 API-KEY + 签名（交易类接口）   |
| USER_DATA   | 需 API-KEY + 签名（用户数据类接口） |
| USER_STREAM | 需 API-KEY（用户数据流接口）      |
| MARKET_DATA | 需 API-KEY（市场数据接口）       |

 签名要求（TRADE/USER_DATA 类型）



签名算法：HMAC SHA256（推荐）或 RSA（PKCS8 格式公钥上传）

HMAC 密钥：API-Secret；RSA 密钥：本地私钥

签名对象：所有请求参数（Query String 在前，Request Body 在后）

必传参数：timestamp（请求发送时的毫秒时间戳）

可选参数：recvWindow（签名有效期窗口，默认 5000ms，最大 60000ms，不推荐超过 5 秒）

时间校验：请求时间戳需在服务器时间 ±5000ms 内，否则视为无效

 1.4 HTTP 状态码说明



| 状态码 | 含义                   | 关键处理规则                   |
| --- | -------------------- | ------------------------ |
| 4XX | 请求错误（参数 / 格式 / 权限问题） | 检查请求参数、接口权限及格式           |
| 403 | WAF 限制               | 合规化请求内容，避免违规操作           |
| 408 | 后端响应超时               | 稍后重试，优化请求频率              |
| 429 | 访问频次超限（警告）           | 立即停止请求，按规则退避重试           |
| 418 | IP 被封禁               | 频繁违反限速导致，封禁时长 2 分钟 - 3 天 |
| 5XX | 服务端问题                | 区分错误信息处理（见下文）            |

 503 状态码细分处理



| 错误信息                                                         | 执行状态      | 处理方式                             |
| ------------------------------------------------------------ | --------- | -------------------------------- |
| Unknown error, please check your request or try again later. | 未知（可能已执行） | 先通过订单查询确认状态，避免重复操作               |
| Service Unavailable.                                         | 100% 失败   | 退避重试（200ms→400ms→800ms，上限 3-5 次） |
| Request throttled by system-level protection...(-1008)       | 100% 失败   | 降低并发，平仓 / 仅减仓订单豁免此限制             |

 1.5 接口错误格式

所有异常响应统一返回以下格式：
{

 "code": -1121,

 "msg": "Invalid symbol."

}

1.7 访问限制



IP 级限制：基于 IP 计算请求权重，头信息X-MBX-USED-WEIGHT-(intervalNum)(intervalLetter)返回已使用权重

订单频率限制：头信息X-MBX-ORDER-COUNT-(intervalNum)(intervalLetter)返回已用下单次数

违规处理：频繁挂撤单、低效交易可能被加强限制

优化建议：优先使用 WebSocket 获取数据，减少 REST 请求压力





 二、术语解释



base asset（基础资产）：交易对中靠前的资产（如 BTCUSDT 中的 BTC）

quote asset（计价资产）：交易对中靠后的资产（如 BTCUSDT 中的 USDT）

名义价值（Notional）：订单成交额，计算方式 = 价格 × 数量（市价单用标记价格计算）





 三、枚举定义

 3.1 核心枚举



| 枚举类型                        | 可选值                                                                                       | 说明                                                                        |
| --------------------------- | ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| 交易对类型                       | FUTURE                                                                                    | 期货交易对                                                                     |
| 合约类型（contractType）          | PERPETUAL/CURRENT_MONTH/NEXT_MONTH/CURRENT_QUARTER/NEXT_QUARTER/PERPETUAL_DELIVERING | 永续合约 / 当月 / 次月 / 当季 / 次季 / 交割结算中合约                                        |
| 合约状态（contractStatus/status） | PENDING_TRADING/TRADING/PRE_DELIVERING/DELIVERING/DELIVERED/PRE_SETTLE/SETTLING/CLOSE  | 待上市 / 交易中 / 预交割 / 交割中 / 已交割 / 预结算 / 结算中 / 已下架                             |
| 订单状态（status）                | NEW/PARTIALLY_FILLED/FILLED/CANCELED/REJECTED/EXPIRED/EXPIRED_IN_MATCH                 | 新建 / 部分成交 / 全部成交 / 已撤销 / 被拒绝 / 过期 / STP 过期                                |
| 订单类型（type）                  | LIMIT/MARKET/STOP/TAKE_PROFIT/STOP_MARKET/TAKE_PROFIT_MARKET/TRAILING_STOP_MARKET   | 限价单 / 市价单 / 止损限价单 / 止盈限价单 / 止损市价单 / 止盈市价单 / 跟踪止损单                         |
| 订单方向（side）                  | BUY/SELL                                                                                  | 买入 / 卖出                                                                   |
| 持仓方向（positionSide）          | BOTH/LONG/SHORT                                                                           | 单向持仓 / 多头（双向）/ 空头（双向）                                                     |
| 有效方法（timeInForce）           | GTC/IOC/FOK/GTX/GTD                                                                       | 成交为止 / 立即成交否则撤销 / 全部成交否则撤销 / 无法挂单则撤销 / 指定时间前有效                            |
| 触发类型（workingType）           | MARKET_PRICE/CONTRACT_PRICE                                                             | 标记价格触发 / 合约最新价触发                                                          |
| 响应类型（newOrderRespType）      | ACK/RESULT                                                                                | 确认响应 / 结果响应（市价单直接返回成交结果）                                                  |
| 防止自成交模式                     | EXPIRE_TAKER/EXPIRE_MAKER/EXPIRE_BOTH                                                  | 撤销吃单方 / 撤销挂单方 / 双向撤销                                                      |
| 盘口价下单模式（priceMatch）         | OPPONENT/OPPONENT_5/OPPONENT_10/OPPONENT_20/QUEUE/QUEUE_5/QUEUE_10/QUEUE_20         | 对手价 / 对手 5 档价 / 对手 10 档价 / 对手 20 档价 / 同向价 / 同向 5 档价 / 同向 10 档价 / 同向 20 档价 |
| 限制种类（rateLimitType）         | REQUEST_WEIGHT/ORDERS                                                                    | 请求权重限制 / 下单次数限制                                                           |
| 限制间隔（interval）              | MINUTE                                                                                    | 分钟级限制（默认）                                                                 |





 四、过滤器（交易规则限制）

 4.1 交易对过滤器（symbol filters）



| 过滤器类型                           | 核心参数                                                          | 规则说明                                                               |
| ------------------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------ |
| PRICE_FILTER（价格过滤器）            | minPrice（最小价格）、maxPrice（最大价格）、tickSize（价格步进）                  | 价格需在 [minPrice, maxPrice] 区间，且满足 (price-minPrice) % tickSize == 0 |
| LOT_SIZE（订单尺寸过滤器）              | minQty（最小数量）、maxQty（最大数量）、stepSize（数量步进）                      | 数量需在 [minQty, maxQty] 区间，且满足 (quantity-minQty) % stepSize == 0    |
| MARKET_LOT_SIZE（市价订单尺寸）       | 同 LOT_SIZE                                                   | 仅对市价单生效                                                            |
| MAX_NUM_ORDERS（最多订单数）         | limit（默认 200）                                                 | 单个交易对最大挂单数量（含普通 + 条件单）                                             |
| MAX_NUM_ALGO_ORDERS（最多条件订单数） | limit（默认 100）                                                 | 单个交易对最大条件挂单数量（STOP/STOP_MARKET 等 5 类）                             |
| PERCENT_PRICE（价格振幅过滤器）         | multiplierUp（上限倍数）、multiplierDown（下限倍数）、multiplierDecimal（精度） | 买单价格≤标记价格 ×multiplierUp；卖单价格≥标记价格 ×multiplierDown                  |
| MIN_NOTIONAL（最小名义价值）           | notional（默认 5.0）                                              | 订单成交额≥该值（市价单用标记价格计算）                                               |





 五、交易接口（核心功能）

 5.1 下单（TRADE）

 接口描述

单次提交 1 个合约订单

 HTTP 请求

POST /fapi/v1/order

 请求权重



10 秒订单限制（X-MBX-ORDER-COUNT-10S）：1

1 分钟订单限制（X-MBX-ORDER-COUNT-1M）：1

IP 权重限制（x-mbx-used-weight-1m）：0

 请求参数



| 名称                      | 类型      | 是否必需 | 描述                                                                                |
| ----------------------- | ------- | ---- | --------------------------------------------------------------------------------- |
| symbol                  | STRING  | YES  | 交易对（如 BTCUSDT）                                                                    |
| side                    | ENUM    | YES  | 买卖方向：BUY/SELL                                                                     |
| positionSide            | ENUM    | NO   | 持仓方向：单向模式默认 BOTH；双向模式必填 LONG/SHORT                                                |
| type                    | ENUM    | YES  | 订单类型（见枚举定义）                                                                       |
| reduceOnly              | STRING  | NO   | 仅减仓标识：true/false；双开模式不接受；closePosition=true 时不支持                                  |
| quantity                | DECIMAL | NO   | 下单数量；closePosition=true 时不支持                                                      |
| price                   | DECIMAL | NO   | 委托价格                                                                              |
| newClientOrderId        | STRING  | NO   | 自定义订单号（正则：^[.A-Z:/a-z0-9_-]{1,36}$），不可重复                                       |
| stopPrice               | DECIMAL | NO   | 触发价；仅 STOP/STOP_MARKET/TAKE_PROFIT/TAKE_PROFIT_MARKET 需传                      |
| closePosition           | STRING  | NO   | 全部平仓标识：true/false；仅支持 STOP_MARKET/TAKE_PROFIT_MARKET；不与 quantity/reduceOnly 合用 |
| activationPrice         | DECIMAL | NO   | 跟踪止损激活价格；仅 TRAILING_STOP_MARKET 需传，默认当前市场价                                      |
| callbackRate            | DECIMAL | NO   | 跟踪止损回调比例（0.1-10，1=1%）；仅 TRAILING_STOP_MARKET 需传                                 |
| timeInForce             | ENUM    | NO   | 有效方法（见枚举定义）                                                                       |
| workingType             | ENUM    | NO   | 触发类型：MARKET_PRICE/CONTRACT_PRICE（默认）                                            |
| priceProtect            | STRING  | NO   | 条件单触发保护：TRUE/FALSE（默认）；仅条件单需传                                                     |
| newOrderRespType        | ENUM    | NO   | 响应类型：ACK/RESULT（默认）                                                               |
| priceMatch              | ENUM    | NO   | 盘口价模式；不可与 price 同时传                                                               |
| selfTradePreventionMode | ENUM    | NO   | 自成交保护模式：默认 NONE                                                                   |
| goodTillDate            | LONG    | NO   | GTD 模式自动取消时间（秒级时间戳，需 > 当前时间 + 600s 且 < 253402300799000）；timeInForce=GTD 时必传       |
| recvWindow              | LONG    | NO   | 签名有效期窗口（默认 5000ms）                                                                |
| timestamp               | LONG    | YES  | 毫秒时间戳                                                                             |

 订单类型强制参数



| 订单类型                              | 强制参数                             |
| --------------------------------- | -------------------------------- |
| LIMIT                             | timeInForce、quantity、price       |
| MARKET                            | quantity（closePosition=false 时）  |
| STOP/TAKE_PROFIT                 | quantity、price、stopPrice         |
| STOP_MARKET/TAKE_PROFIT_MARKET | stopPrice（closePosition=false 时） |
| TRAILING_STOP_MARKET            | callbackRate                     |

 条件单触发规则



1. 触发保护（priceProtect=true）：触发时 MARK_PRICE 与 CONTRACT_PRICE 价差≤交易对 triggerProtect 阈值（通过 GET /fapi/v1/exchangeInfo 获取）

2. 止损单（STOP/STOP_MARKET）：买入→最新价 / 标记价≥stopPrice；卖出→最新价 / 标记价≤stopPrice

3. 止盈单（TAKE_PROFIT/TAKE_PROFIT_MARKET）：买入→最新价 / 标记价≤stopPrice；卖出→最新价 / 标记价≥stopPrice

4. 跟踪止损单（TRAILING_STOP_MARKET）：买入→区间最低价 <activationPrice 且最新价≥最低价 + 回调幅度；卖出→区间最高价> activationPrice 且最新价≤最高价 - 回调幅度

5. 跟踪止损报错（-2021）：买入时 activationPrice 需 <最新价；卖出时需> 最新价

 特殊说明



newOrderRespType=RESULT 时：市价单直接返回成交结果；特殊 timeInForce 的限价单返回成交 / 过期结果

closePosition=true（STOP_MARKET/TAKE_PROFIT_MARKET）：触发后平全部对应仓位，双开模式下 LONG 不支持 BUY、SHORT 不支持 SELL

selfTradePreventionMode 仅对 timeInForce=IOC/GTC/GTD 生效

GTD 订单极端行情下自动取消可能延迟

 响应示例




{

 "clientOrderId": "testOrder",

 "cumQty": "0",

 "cumQuote": "0",

 "executedQty": "0",

 "orderId": 22542179,

 "avgPrice": "0.00000",

 "origQty": "10",

 "price": "0",

 "reduceOnly": false,

 "side": "SELL",

 "positionSide": "SHORT",

 "status": "NEW",

 "stopPrice": "0",

 "closePosition": false,

 "symbol": "BTCUSDT",

 "timeInForce": "GTD",

 "type": "TRAILING_STOP_MARKET",

 "origType": "TRAILING_STOP_MARKET",

 "activatePrice": "9020",

 "priceRate": "0.3",

 "updateTime": 1566818724722,

 "workingType": "CONTRACT_PRICE",

 "priceProtect": false,

 "priceMatch": "NONE",

 "selfTradePreventionMode": "NONE",

 "goodTillDate": 1693207680000

}


 5.2 批量下单（TRADE）

 接口描述

单次提交最多 5 个合约订单（并发处理，不保证撮合顺序）

 HTTP 请求

POST /fapi/v1/batchOrders

 请求权重



10 秒订单限制（X-MBX-ORDER-COUNT-10S）：5

1 分钟订单限制（X-MBX-ORDER-COUNT-1M）：1

IP 权重限制（x-mbx-used-weight-1m）：5

 请求参数



| 名称          | 类型   | 是否必需 | 描述                     |
| ----------- | ---- | ---- | ---------------------- |
| batchOrders | list | YES  | 订单列表（最多 5 个，单个订单参数见下文） |
| recvWindow  | LONG | NO   | 签名有效期窗口（默认 5000ms）     |
| timestamp   | LONG | YES  | 毫秒时间戳                  |

 单个订单参数（batchOrders 元素）



| 名称                      | 类型      | 是否必需 | 描述（与普通下单差异）                    |
| ----------------------- | ------- | ---- | ------------------------------ |
| symbol                  | STRING  | YES  | 同普通下单                          |
| side                    | ENUM    | YES  | 同普通下单                          |
| positionSide            | ENUM    | NO   | 同普通下单                          |
| type                    | ENUM    | YES  | 同普通下单                          |
| reduceOnly              | STRING  | NO   | 同普通下单                          |
| quantity                | DECIMAL | YES  | 必传（普通下单为非必填）                   |
| price                   | DECIMAL | NO   | 同普通下单                          |
| newClientOrderId        | STRING  | NO   | 同普通下单（单个订单需唯一）                 |
| stopPrice               | DECIMAL | NO   | 同普通下单                          |
| activationPrice         | DECIMAL | NO   | 同普通下单                          |
| callbackRate            | DECIMAL | NO   | 同普通下单（取值范围 0.1-4，普通下单为 0.1-10） |
| timeInForce             | ENUM    | NO   | 同普通下单                          |
| workingType             | ENUM    | NO   | 同普通下单                          |
| priceProtect            | STRING  | NO   | 同普通下单                          |
| newOrderRespType        | ENUM    | NO   | 同普通下单                          |
| priceMatch              | ENUM    | NO   | 同普通下单                          |
| selfTradePreventionMode | ENUM    | NO   | 同普通下单                          |
| goodTillDate            | LONG    | NO   | 同普通下单                          |

 请求示例（URL 参数格式）




/fapi/v1/batchOrders?batchOrders=[{"type":"LIMIT","timeInForce":"GTC","symbol":"BTCUSDT","side":"BUY","price":"10001","quantity":"0.001"},{"type":"MARKET","symbol":"ETHUSDT","side":"SELL","quantity":"0.1"}]&timestamp=1699999999999&recvWindow=5000


 特殊说明



响应顺序与 batchOrders 传入顺序一致

支持部分成功：单个订单报错不影响其他订单执行

订单规则与普通下单完全一致

 响应示例




[

 {

   "clientOrderId": "testOrder1",

   "cumQty": "0",

   "cumQuote": "0",

   "executedQty": "0",

   "orderId": 22542179,

   "avgPrice": "0.00000",

   "origQty": "0.001",

   "price": "10001",

   "reduceOnly": false,

   "side": "BUY",

   "positionSide": "BOTH",

   "status": "NEW",

   "stopPrice": "0",

   "closePosition": false,

   "symbol": "BTCUSDT",

   "timeInForce": "GTC",

   "type": "LIMIT",

   "origType": "LIMIT",

   "activatePrice": "0",

   "priceRate": "0",

   "updateTime": 1699999999999,

   "workingType": "CONTRACT_PRICE",

   "priceProtect": false,

   "priceMatch": "NONE",

   "selfTradePreventionMode": "NONE",

   "goodTillDate": 0

 },

 {

   "code": -2022,

   "msg": "ReduceOnly Order is rejected."

 }

]


 5.3 修改订单（TRADE）

 接口描述

仅支持限价（LIMIT）订单修改，修改后重新排序

 HTTP 请求

PUT /fapi/v1/order

 请求权重



10 秒订单限制（X-MBX-ORDER-COUNT-10S）：1

1 分钟订单限制（X-MBX-ORDER-COUNT-1M）：1

IP 权重限制（x-mbx-used-weight-1m）：0

 请求参数



| 名称                | 类型      | 是否必需 | 描述                                             |
| ----------------- | ------- | ---- | ---------------------------------------------- |
| orderId           | LONG    | NO   | 系统订单号（与 origClientOrderId 二选一，同时传以 orderId 为准） |
| origClientOrderId | STRING  | NO   | 原自定义订单号                                        |
| symbol            | STRING  | YES  | 交易对                                            |
| side              | ENUM    | YES  | 买卖方向：BUY/SELL                                  |
| quantity          | DECIMAL | YES  | 新下单数量                                          |
| price             | DECIMAL | YES  | 新委托价格                                          |
| priceMatch        | ENUM    | NO   | 盘口价模式；不可与 price 同时传                            |
| recvWindow        | LONG    | NO   | 签名有效期窗口（默认 5000ms）                             |
| timestamp         | LONG    | YES  | 毫秒时间戳                                          |

 特殊说明



quantity 和 price 必须同时传（与 dapi 不同）

新参数需满足 PRICE_FILTER/PERCENT_FILTER/LOT_SIZE 限制，否则修改失败（原订单保留）

原订单部分成交且新 quantity≤executedQty 时，原订单会被取消

GTX 订单修改后价格导致立即执行时，原订单会被取消

同一订单最多修改 10000 次

 响应示例




{

 "orderId": 20072994037,

 "symbol": "BTCUSDT",

 "pair": "BTCUSDT",

 "status": "NEW",

 "clientOrderId": "LJ9R4QZDihCaS8UAOOLpgW",

 "price": "30005",

 "avgPrice": "0.0",

 "origQty": "1",

 "executedQty": "0",

 "cumQty": "0",

 "cumBase": "0",

 "timeInForce": "GTC",

 "type": "LIMIT",

 "reduceOnly": false,

 "closePosition": false,

 "side": "BUY",

 "positionSide": "LONG",

 "stopPrice": "0",

 "workingType": "CONTRACT_PRICE",

 "priceProtect": false,

 "origType": "LIMIT",

 "priceMatch": "NONE",

 "selfTradePreventionMode": "NONE",

 "goodTillDate": 0,

 "updateTime": 1629182711600

}


 5.4 批量修改订单（TRADE）

 接口描述

批量修改最多 5 个订单（并发处理，不保证顺序）

 HTTP 请求

PUT /fapi/v1/batchOrders

 请求权重



10 秒订单限制（X-MBX-ORDER-COUNT-10S）：5

1 分钟订单限制（X-MBX-ORDER-COUNT-1M）：1

IP 权重限制（x-mbx-used-weight-1m）：5

 请求参数



| 名称          | 类型   | 是否必需 | 描述                     |
| ----------- | ---- | ---- | ---------------------- |
| batchOrders | list | YES  | 订单列表（最多 5 个，单个订单参数见下文） |
| recvWindow  | LONG | NO   | 签名有效期窗口（默认 5000ms）     |
| timestamp   | LONG | YES  | 毫秒时间戳                  |

 单个订单参数（batchOrders 元素）



| 名称                | 类型      | 是否必需 | 描述                             |
| ----------------- | ------- | ---- | ------------------------------ |
| orderId           | LONG    | NO   | 系统订单号（与 origClientOrderId 二选一） |
| origClientOrderId | STRING  | NO   | 原自定义订单号                        |
| symbol            | STRING  | YES  | 交易对                            |
| side              | ENUM    | YES  | 买卖方向：BUY/SELL                  |
| quantity          | DECIMAL | YES  | 新下单数量                          |
| price             | DECIMAL | YES  | 新委托价格                          |
| priceMatch        | ENUM    | NO   | 盘口价模式；不可与 price 同时传            |
| stopPrice         | DECIMAL | NO   | 触发价；仅条件单需传                     |

 特殊说明



响应顺序与 batchOrders 传入顺序一致

同一订单最多修改 10000 次

其他规则与普通修改订单一致

 响应示例




[

 {

   "orderId": 20072994037,

   "symbol": "BTCUSDT",

   "pair": "BTCUSDT",

   "status": "NEW",

   "clientOrderId": "LJ9R4QZDihCaS8UAOOLpgW",

   "price": "30005",

   "avgPrice": "0.0",

   "origQty": "1",

   "executedQty": "0",

   "cumQty": "0",

   "cumBase": "0",

   "timeInForce": "GTC",

   "type": "LIMIT",

   "reduceOnly": false,

   "closePosition": false,

   "side": "BUY",

   "positionSide": "LONG",

   "stopPrice": "0",

   "workingType": "CONTRACT_PRICE",

   "priceProtect": false,

   "origType": "LIMIT",

   "priceMatch": "NONE",

   "selfTradePreventionMode": "NONE",

   "goodTillDate": 0,

   "updateTime": 1629182711600

 },

 {

   "code": -2022,

   "msg": "ReduceOnly Order is rejected."

 }

]


 5.5 查询订单修改历史（USER_DATA）

 接口描述

查询订单的修改记录（保留最近三个月）

 HTTP 请求

GET /fapi/v1/orderAmendment

 请求权重

1

 请求参数



| 名称                | 类型     | 是否必需 | 描述                                             |
| ----------------- | ------ | ---- | ---------------------------------------------- |
| symbol            | STRING | YES  | 交易对                                            |
| orderId           | LONG   | NO   | 系统订单号（与 origClientOrderId 二选一，同时传以 orderId 为准） |
| origClientOrderId | STRING | NO   | 原自定义订单号                                        |
| startTime         | LONG   | NO   | 起始时间戳（毫秒）                                      |
| endTime           | LONG   | NO   | 结束时间戳（毫秒）                                      |
| limit             | INT    | NO   | 返回数量（默认 50，最大 100）                             |
| recvWindow        | LONG   | NO   | 签名有效期窗口（默认 5000ms）                             |
| timestamp         | LONG   | YES  | 毫秒时间戳                                          |

 响应示例




[

 {

   "amendmentId": 5363,

   "symbol": "BTCUSDT",

   "pair": "BTCUSDT",

   "orderId": 20072994037,

   "clientOrderId": "LJ9R4QZDihCaS8UAOOLpgW",

   "time": 1629184560899,

   "amendment": {

     "price": {

       "before": "30004",

       "after": "30003.2"

     },

     "origQty": {

       "before": "1",

       "after": "1"

     },

     "count": 3

   }

 }

]


 5.6 撤销订单（TRADE）

 接口描述

撤销单个未成交订单

 HTTP 请求

DELETE /fapi/v1/order

 请求权重

1

 请求参数



| 名称                | 类型     | 是否必需 | 描述                             |
| ----------------- | ------ | ---- | ------------------------------ |
| symbol            | STRING | YES  | 交易对                            |
| orderId           | LONG   | NO   | 系统订单号（与 origClientOrderId 二选一） |
| origClientOrderId | STRING | NO   | 原自定义订单号                        |
| recvWindow        | LONG   | NO   | 签名有效期窗口（默认 5000ms）             |
| timestamp         | LONG   | YES  | 毫秒时间戳                          |

 响应示例




{

 "clientOrderId": "myOrder1",

 "cumQty": "0",

 "cumQuote": "0",

 "executedQty": "0",

 "orderId": 283194212,

 "origQty": "11",

 "price": "0",

 "reduceOnly": false,

 "side": "BUY",

 "positionSide": "SHORT",

 "status": "CANCELED",

 "stopPrice": "9300",

 "closePosition": false,

 "symbol": "BTCUSDT",

 "timeInForce": "GTC",

 "origType": "TRAILING_STOP_MARKET",

 "type": "TRAILING_STOP_MARKET",

 "activatePrice": "9020",

 "priceRate": "0.3",

 "updateTime": 1571110484038,

 "workingType": "CONTRACT_PRICE",

 "priceProtect": false,

 "priceMatch": "NONE",

 "selfTradePreventionMode": "NONE",

 "goodTillDate": 0

}


 5.7 批量撤销订单（TRADE）

 接口描述

批量撤销最多 10 个未成交订单

 HTTP 请求

DELETE /fapi/v1/batchOrders

 请求权重

1

 请求参数



| 名称                    | 类型     | 是否必需 | 描述                                    |
| --------------------- | ------ | ---- | ------------------------------------- |
| symbol                | STRING | YES  | 交易对                                   |
| orderIdList           | LIST   | NO   | 系统订单号列表（最多 10 个，如 [1234567,2345678]） |
| origClientOrderIdList | LIST   | NO   | 自定义订单号列表（最多 10 个，需 URL 编码双引号）         |
| recvWindow            | LONG   | NO   | 签名有效期窗口（默认 5000ms）                    |
| timestamp             | LONG   | YES  | 毫秒时间戳                                 |

 特殊说明



orderIdList 与 origClientOrderIdList 二选一，不可同时传

 响应示例




[

 {

   "clientOrderId": "myOrder1",

   "cumQty": "0",

   "cumQuote": "0",

   "executedQty": "0",

   "orderId": 283194212,

   "origQty": "11",

   "price": "0",

   "reduceOnly": false,

   "side": "BUY",

   "positionSide": "SHORT",

   "status": "CANCELED",

   "stopPrice": "9300",

   "closePosition": false,

   "symbol": "BTCUSDT",

   "timeInForce": "GTC",

   "origType": "TRAILING_STOP_MARKET",

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

 {

   "code": -2011,

   "msg": "Unknown order sent."

 }

]


 5.8 撤销全部订单（TRADE）

 接口描述

撤销指定交易对的所有未成交订单

 HTTP 请求

DELETE /fapi/v1/allOpenOrders

 请求权重

1

 请求参数



| 名称         | 类型     | 是否必需 | 描述                 |
| ---------- | ------ | ---- | ------------------ |
| symbol     | STRING | YES  | 交易对                |
| recvWindow | LONG   | NO   | 签名有效期窗口（默认 5000ms） |
| timestamp  | LONG   | YES  | 毫秒时间戳              |

 响应示例




{

 "code": 200,

 "msg": "The operation of cancel all open order is done."

}


 5.9 查询订单（USER_DATA）

 接口描述

查询单个订单状态（仅保留符合条件的订单）

 不支持查询的订单条件



最终状态为 CANCELED/EXPIRED + 无成交记录 + 生成时间 + 3 天 < 当前时间

创建时间 + 90 天 < 当前时间

 HTTP 请求

GET /fapi/v1/order

 请求权重

1

 请求参数



| 名称                | 类型     | 是否必需 | 描述                             |
| ----------------- | ------ | ---- | ------------------------------ |
| symbol            | STRING | YES  | 交易对                            |
| orderId           | LONG   | NO   | 系统订单号（与 origClientOrderId 二选一） |
| origClientOrderId | STRING | NO   | 自定义订单号                         |
| recvWindow        | LONG   | NO   | 签名有效期窗口（默认 5000ms）             |
| timestamp         | LONG   | YES  | 毫秒时间戳                          |

 特殊说明



orderId 在 symbol 维度自增

 响应示例




{

 "avgPrice": "0.00000",

 "clientOrderId": "abc",

 "cumQuote": "0",

 "executedQty": "0",

 "orderId": 1573346959,

 "origQty": "0.40",

 "origType": "TRAILING_STOP_MARKET",

 "price": "0",

 "reduceOnly": false,

 "side": "BUY",

 "positionSide": "SHORT",

 "status": "NEW",

 "stopPrice": "9300",

 "closePosition": false,

 "symbol": "BTCUSDT",

 "time": 1579276756075,

 "timeInForce": "GTC",

 "type": "TRAILING_STOP_MARKET",

 "activatePrice": "9020",

 "priceRate": "0.3",

 "updateTime": 1579276756075,

 "workingType": "CONTRACT_PRICE",

 "priceProtect": false,

 "priceMatch": "NONE",

 "selfTradePreventionMode": "NONE",

 "goodTillDate": 0

}


 5.10 查看当前全部挂单（USER_DATA）

 接口描述

查询指定交易对或所有交易对的未成交挂单

 HTTP 请求

GET /fapi/v1/openOrders

 请求权重



带 symbol：1

不带 symbol：40（谨慎使用）

 请求参数



| 名称         | 类型     | 是否必需 | 描述                 |
| ---------- | ------ | ---- | ------------------ |
| symbol     | STRING | NO   | 交易对（不传返回所有交易对挂单）   |
| recvWindow | LONG   | NO   | 签名有效期窗口（默认 5000ms） |
| timestamp  | LONG   | YES  | 毫秒时间戳              |

 响应示例




[

 {

   "avgPrice": "0.00000",

   "clientOrderId": "abc",

   "cumQuote": "0",

   "executedQty": "0",

   "orderId": 1917641,

   "origQty": "0.40",

   "origType": "TRAILING_STOP_MARKET",

   "price": "0",

   "reduceOnly": false,

   "side": "BUY",

   "positionSide": "SHORT",

   "status": "NEW",

   "stopPrice": "9300",

   "closePosition": false,

   "symbol": "BTCUSDT",

   "time": 1579276756075,

   "timeInForce": "GTC",

   "type": "TRAILING_STOP_MARKET",

   "activatePrice": "9020",

   "priceRate": "0.3",

   "updateTime": 1579276756075,

   "workingType": "CONTRACT_PRICE",

   "priceProtect": false,

   "priceMatch": "NONE",

   "selfTradePreventionMode": "NONE",

   "goodTillDate": 0

 }

]


 5.11 查询当前挂单（USER_DATA）

 接口描述

查询指定交易对的单个未成交挂单

 HTTP 请求

GET /fapi/v1/openOrder

 请求权重

1

 请求参数



| 名称                | 类型     | 是否必需 | 描述                             |
| ----------------- | ------ | ---- | ------------------------------ |
| symbol            | STRING | YES  | 交易对                            |
| orderId           | LONG   | NO   | 系统订单号（与 origClientOrderId 二选一） |
| origClientOrderId | STRING | NO   | 自定义订单号                         |
| recvWindow        | LONG   | NO   | 签名有效期窗口（默认 5000ms）             |
| timestamp         | LONG   | YES  | 毫秒时间戳                          |

 特殊说明



订单已成交或取消时返回报错 "Order does not exist."

 响应示例




{

 "avgPrice": "0.00000",

 "clientOrderId": "abc",

 "cumQuote": "0",

 "executedQty": "0",

 "orderId": 1917641,

 "origQty": "0.40",

 "origType": "TRAILING_STOP_MARKET",

 "price": "0",

 "reduceOnly": false,

 "side": "BUY",

 "positionSide": "SHORT",

 "status": "NEW",

 "stopPrice": "9300",

 "closePosition": false,

 "symbol": "BTCUSDT",

 "time": 1579276756075,

 "timeInForce": "GTC",

 "type": "TRAILING_STOP_MARKET",

 "activatePrice": "9020",

 "priceRate": "0.3",

 "updateTime": 1579276756075,

 "workingType": "CONTRACT_PRICE",

 "priceProtect": false,

 "priceMatch": "NONE",

 "selfTradePreventionMode": "NONE",

 "goodTillDate": 0

}


 5.12 更改持仓模式（TRADE）

 接口描述

切换所有交易对的持仓模式（双向 / 单向）

 HTTP 请求

POST /fapi/v1/positionSide/dual

 请求权重

1

 请求参数



| 名称               | 类型     | 是否必需 | 描述                         |
| ---------------- | ------ | ---- | -------------------------- |
| dualSidePosition | STRING | YES  | "true"= 双向持仓；"false"= 单向持仓 |
| recvWindow       | LONG   | NO   | 签名有效期窗口（默认 5000ms）         |
| timestamp        | LONG   | YES  | 毫秒时间戳                      |

 响应示例




{

 "code": 200,

 "msg": "success"

}


 5.13 调整开仓杠杆（TRADE）

 接口描述

修改指定交易对的开仓杠杆倍数

 HTTP 请求

POST /fapi/v1/leverage

 请求权重

1

 请求参数



| 名称         | 类型     | 是否必需 | 描述                 |
| ---------- | ------ | ---- | ------------------ |
| symbol     | STRING | YES  | 交易对                |
| leverage   | INT    | YES  | 杠杆倍数（1-125 之间的整数）  |
| recvWindow | LONG   | NO   | 签名有效期窗口（默认 5000ms） |
| timestamp  | LONG   | YES  | 毫秒时间戳              |

 响应示例




{

 "leverage": 21,

 "maxNotionalValue": "1000000",

 "symbol": "BTCUSDT"

}






 附录：接口鉴权类型汇总



| 接口名称                                                                 | 鉴权类型       |
| -------------------------------------------------------------------- | ---------- |
| 下单 / 批量下单 / 修改订单 / 批量修改订单 / 撤销订单 / 批量撤销订单 / 撤销全部订单 / 更改持仓模式 / 调整开仓杠杆 | TRADE      |
| 查询订单修改历史 / 查询订单 / 查看当前全部挂单 / 查询当前挂单                                  | USER_DATA |

