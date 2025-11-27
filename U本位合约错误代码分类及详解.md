 U 本位合约错误代码完整文档

 一、通用说明



 错误响应格式：所有错误均返回 JSON 格式，包含错误代码（`code`）和描述（`msg`），示例如下：




{

 "code": -1121,

 "msg": "Invalid symbol."

}

 代码规则：错误代码为通用标识，描述（`msg`）可能根据实际场景微调，但核心含义一致。

 分类逻辑：按错误类型分为 5 大类（10xx/11xx/20xx/40xx/50xx），便于按场景快速查询。





 二、错误代码分类详情

 2.1 10xx：常规服务器或网络问题



| 错误代码  | 英文描述                                                                             | 中文说明                        | 备注                                       |
| ----- | -------------------------------------------------------------------------------- | --------------------------- | ---------------------------------------- |
| -1000 | UNKNOWN: An unknown error occured while processing the request.                  | 未知错误：处理请求时发生未知异常            | 建议重试，若频繁出现需联系技术支持                        |
| -1001 | DISCONNECTED: Internal error; unable to process your request. Please try again.  | 连接断开：内部错误，无法处理请求            | 需重新建立连接并重试                               |
| -1002 | UNAUTHORIZED: You are not authorized to execute this request.                    | 未授权：无权执行该请求                 | 检查 API-KEY 权限、IP 白名单配置                   |
| -1003 | TOO\_MANY\_REQUESTS: Too many requests; current limit is %s requests per minute. | 请求过多：超出速率限制（当前限制每分钟 % s 次）  | 优先使用 WebSocket 减少 REST 请求，按退避策略重试        |
| -1004 | DUPLICATE\_IP: This IP is already on the white list                              | IP 重复：该 IP 已在白名单中           | 无需重复添加同一 IP                              |
| -1005 | NO\_SUCH\_IP: No such IP has been white listed                                   | IP 不存在：白名单中无此 IP            | 需先将 IP 添加至 API-KEY 白名单                   |
| -1006 | UNEXPECTED\_RESP: An unexpected response was received from the message bus.      | 响应异常：从消息总线收到意外响应            | 执行状态未知，需通过查询接口确认结果                       |
| -1007 | TIMEOUT: Timeout waiting for response from backend server.                       | 超时：等待后端响应超时                 | 发送状态和执行状态均未知，避免重复操作                      |
| -1008 | Request Throttled: Server is currently overloaded with other requests.           | 请求限流：服务器负载过高（仅减仓 / 平仓订单豁免）  | 系统级保护，降低并发并退避重试                          |
| -1014 | UNKNOWN\_ORDER\_COMPOSITION: Unsupported order combination.                      | 订单组合无效：不支持当前下单参数组合          | 检查订单类型、持仓方向、仅减仓等参数的搭配                    |
| -1015 | TOO\_MANY\_ORDERS: Too many new orders.                                          | 订单过多：新订单数量超出限制              | 单次下单 / 批量下单需符合速率限制（参考接口权重）               |
| -1016 | SERVICE\_SHUTTING\_DOWN: This service is no longer available.                    | 服务停用：该服务已不可用                | 确认接口是否已废弃，切换至替代接口                        |
| -1020 | UNSUPPORTED\_OPERATION: This operation is not supported.                         | 操作不支持：该操作暂不支持               | 检查接口方法、请求方式（GET/POST 等）是否正确              |
| -1021 | INVALID\_TIMESTAMP: Timestamp for this request is outside of the recvWindow.     | 时间戳无效：请求时间戳超出 recvWindow 范围 | 校准本地时间与服务器时间（误差≤5000ms），调整 recvWindow 参数 |
| -1022 | INVALID\_SIGNATURE: Signature for this request is not valid.                     | 签名无效：请求签名不正确                | 检查 API-Secret、参数排序、签名算法是否正确              |
| -1023 | START\_TIME\_GREATER\_THAN\_END\_TIME: Start time is greater than end time.      | 时间范围无效：开始时间晚于结束时间           | 调整 startTime 和 endTime 参数，确保前者≤后者        |

 2.2 11xx：请求参数问题



| 错误代码  | 英文描述                                                                        | 中文说明                              | 备注                                                |
| ----- | --------------------------------------------------------------------------- | --------------------------------- | ------------------------------------------------- |
| -1100 | ILLEGAL\_CHARS: Illegal characters found in a parameter.                    | 参数含非法字符：参数中存在不允许的字符               | 检查参数格式（如 symbol、clientOrderId）是否符合正则规则            |
| -1101 | TOO\_MANY\_PARAMETERS: Too many parameters sent for this endpoint.          | 参数过多：发送的参数数量超出接口限制                | 移除接口不需要的参数，避免重复传参                                 |
| -1102 | MANDATORY\_PARAM\_EMPTY\_OR\_MALFORMED: A mandatory parameter was not sent. | 必选参数缺失 / 格式错误：未发送必选参数或格式无效        | 检查接口文档，确保所有必填参数完整且格式正确（如数值类型、长度）                  |
| -1103 | UNKNOWN\_PARAM: An unknown parameter was sent.                              | 未知参数：发送了接口不支持的参数                  | 核对参数名称，移除多余的未知参数                                  |
| -1104 | UNREAD\_PARAMETERS: Not all sent parameters were read.                      | 参数未读取：部分发送的参数未被接口解析               | 检查参数传递方式（Query String/Request Body）是否符合接口要求       |
| -1105 | PARAM\_EMPTY: A parameter was empty.                                        | 参数为空：必填参数值为空或 null                | 确保参数值非空（如 quantity、price 等核心参数）                   |
| -1106 | PARAM\_NOT\_REQUIRED: A parameter was sent when not required.               | 多余参数：发送了非必需的参数                    | 仅保留接口文档中指定的可选 / 必选参数                              |
| -1108 | BAD\_ASSET: Invalid asset.                                                  | 资产无效：指定的资产不正确                     | 检查资产名称（如 USDT、BTC）是否支持                            |
| -1109 | BAD\_ACCOUNT: Invalid account.                                              | 账户无效：非有效合约账户                      | 确认账户已开通 U 本位合约权限，且状态正常                            |
| -1110 | BAD\_INSTRUMENT\_TYPE: Invalid symbolType.                                  | 交易对类型无效：symbolType 参数错误           | 检查交易对类型是否为 FUTURE（期货）                             |
| -1111 | BAD\_PRECISION: Precision is over the maximum defined for this asset.       | 精度超限：参数精度超出资产定义的最大值               | 参考 exchangeInfo 接口的价格 / 数量精度限制（tickSize/stepSize） |
| -1112 | NO\_DEPTH: No orders on book for symbol.                                    | 无挂单深度：该交易对当前无挂单                   | 确认交易对状态为 TRADING，或稍后再试                            |
| -1113 | WITHDRAW\_NOT\_NEGATIVE: Withdrawal amount must be negative.                | 提现金额无效：提现金额需为负数                   | 调整提现金额参数为负值                                       |
| -1114 | TIF\_NOT\_REQUIRED: TimeInForce parameter sent when not required.           | 有效方法多余：发送了不需要的 timeInForce 参数     | 部分订单类型无需 timeInForce，按接口要求移除                      |
| -1115 | INVALID\_TIF: Invalid timeInForce.                                          | 有效方法无效：timeInForce 参数值不正确         | 可选值：GTC/IOC/FOK/GTX/GTD，需与订单类型匹配                  |
| -1116 | INVALID\_ORDER\_TYPE: Invalid orderType.                                    | 订单类型无效：orderType 参数错误             | 可选值：LIMIT/MARKET/STOP 等（参考枚举定义）                   |
| -1117 | INVALID\_SIDE: Invalid side.                                                | 买卖方向无效：side 参数错误                  | 可选值：BUY/SELL，需与持仓方向匹配                             |
| -1118 | EMPTY\_NEW\_CL\_ORD\_ID: New client order ID was empty.                     | 自定义订单号为空：newClientOrderId 参数为空    | 若指定该参数，需符合正则规则且不重复                                |
| -1119 | EMPTY\_ORG\_CL\_ORD\_ID: Original client order ID was empty.                | 原始订单号为空：origClientOrderId 参数为空    | 查询 / 修改 / 撤销订单时，需指定有效订单号                          |
| -1120 | BAD\_INTERVAL: Invalid interval.                                            | 时间间隔无效：K 线间隔参数错误                  | 可选值：1m/3m/1h/1d 等（参考枚举定义）                         |
| -1121 | BAD\_SYMBOL: Invalid symbol.                                                | 交易对无效：symbol 参数错误                 | 检查交易对是否存在（如 BTCUSDT，区分大小写）                        |
| -1122 | INVALID\_SYMBOL\_STATUS: Invalid symbol status.                             | 交易对状态无效：该交易对状态不支持当前操作             | 仅 TRADING 状态的交易对可执行下单等操作                          |
| -1125 | INVALID\_LISTEN\_KEY: This listenKey does not exist.                        | listenKey 无效：该 listenKey 不存在      | 重新调用 POST /fapi/v1/listenKey 生成新的 listenKey       |
| -1126 | ASSET\_NOT\_SUPPORTED: This asset is not supported.                         | 资产不支持：该资产暂不支持合约交易                 | 确认资产是否已上线 U 本位合约                                  |
| -1127 | MORE\_THAN\_XX\_HOURS: Lookup interval is too big.                          | 查询间隔过大：startTime 与 endTime 间隔超出限制 | 缩短查询时间范围（通常不超过 30 天）                              |
| -1128 | OPTIONAL\_PARAMS\_BAD\_COMBO: Combination of optional parameters invalid.   | 可选参数组合无效：可选参数搭配错误                 | 检查参数间依赖关系（如 price 与 priceMatch 不可同时传）             |
| -1130 | INVALID\_PARAMETER: Invalid data sent for a parameter.                      | 参数数据无效：参数值不符合要求                   | 检查参数取值范围（如 leverage 为 1-125 的整数）                  |
| -1136 | INVALID\_NEW\_ORDER\_RESP\_TYPE: Invalid newOrderRespType.                  | 响应类型无效：newOrderRespType 参数错误      | 可选值：ACK/RESULT                                    |

 2.3 20xx：订单处理问题



| 错误代码  | 英文描述                                                                                        | 中文说明                                 | 备注                                    |
| ----- | ------------------------------------------------------------------------------------------- | ------------------------------------ | ------------------------------------- |
| -2010 | NEW\_ORDER\_REJECTED: NEW\_ORDER\_REJECTED                                                  | 新订单被拒绝：订单未提交成功                       | 检查参数合法性、账户余额、持仓状态等                    |
| -2011 | CANCEL\_REJECTED: CANCEL\_REJECTED                                                          | 取消订单被拒绝：订单无法撤销                       | 常见原因：订单已成交、已撤销、处于触发中                  |
| -2012 | CANCEL\_ALL\_FAIL: Batch cancel failure.                                                    | 批量取消失败：批量撤销订单操作失败                    | 可能存在部分订单撤销成功，需单独核对订单状态                |
| -2013 | NO\_SUCH\_ORDER: Order does not exist.                                                      | 订单不存在：查询的订单不存在                       | 检查订单号、交易对是否正确，订单是否已过期（超过 90 天）        |
| -2014 | BAD\_API\_KEY\_FMT: API-key format invalid.                                                 | API-KEY 格式无效：API-KEY 格式错误            | 检查 API-KEY 是否完整，无多余空格或字符              |
| -2015 | REJECTED\_MBX\_KEY: Invalid API-key, IP, or permissions for action.                         | 权限 / IP/API-KEY 无效：API-KEY、IP 或权限不匹配 | 检查 API-KEY 权限（如 TRADE 权限）、IP 是否在白名单   |
| -2016 | NO\_TRADING\_WINDOW: No trading window could be found for the symbol.                       | 无交易窗口：该交易对无匹配的交易窗口                   | 切换至 ticker/24hrs 接口查询市场数据             |
| -2017 | API\_KEYS\_LOCKED: API Keys are locked on this account.                                     | API-KEY 被锁定：账户 API-KEY 已上锁           | 前往币安官网解锁 API-KEY，或重新创建                |
| -2018 | BALANCE\_NOT\_SUFFICIENT: Balance is insufficient.                                          | 余额不足：账户余额不足以支持当前操作                   | 检查钱包余额、可用保证金是否充足                      |
| -2019 | MARGIN\_NOT\_SUFFICIEN: Margin is insufficient.                                             | 保证金不足：杠杆账户保证金不足                      | 增加保证金或降低杠杆倍数                          |
| -2020 | UNABLE\_TO\_FILL: Unable to fill.                                                           | 无法成交：订单无法匹配成交                        | 检查订单价格、数量是否符合市场深度，或调整订单类型             |
| -2021 | ORDER\_WOULD\_IMMEDIATELY\_TRIGGER: Order would immediately trigger.                        | 订单将立即触发：条件单触发条件已满足                   | 调整触发价（stopPrice）或激活价（activationPrice） |
| -2022 | REDUCE\_ONLY\_REJECT: ReduceOnly Order is rejected.                                         | 仅减仓订单被拒绝：与当前未成交同向订单冲突                | 先取消现有同向未成交订单，再提交仅减仓订单                 |
| -2023 | USER\_IN\_LIQUIDATION: User in liquidation mode now.                                        | 用户处于强平模式：账户正在被强平                     | 无法进行开仓操作，需处理强平风险                      |
| -2024 | POSITION\_NOT\_SUFFICIENT: Position is not sufficient.                                      | 持仓不足：当前持仓数量不足以支持操作                   | 仅减仓 / 平仓订单需确保持仓数量≥订单数量                |
| -2025 | MAX\_OPEN\_ORDER\_EXCEEDED: Reach max open order limit.                                     | 挂单上限：挂单数量已达上限                        | 取消未成交订单后再提交新订单（默认单交易对 200 单）          |
| -2026 | REDUCE\_ONLY\_ORDER\_TYPE\_NOT\_SUPPORTED: This OrderType is not supported when reduceOnly. | 订单类型不支持仅减仓：当前订单类型无法搭配 reduceOnly     | 更换支持仅减仓的订单类型（如 LIMIT、MARKET）          |
| -2027 | MAX\_LEVERAGE\_RATIO: Exceeded the maximum allowable position at current leverage.          | 杠杆持仓超限：超出当前杠杆下的最大持仓 / 挂单量            | 降低杠杆倍数或减少订单 / 持仓数量                    |
| -2028 | MIN\_LEVERAGE\_RATIO: Leverage is smaller than permitted.                                   | 杠杆过低：调整的杠杆低于允许范围（保证金不足）              | 提高杠杆倍数或增加保证金                          |

 2.4 40xx：过滤器与规则限制问题



| 错误代码  | 英文描述                                                                                                       | 中文说明                                         | 备注                                        |
| ----- | ---------------------------------------------------------------------------------------------------------- | -------------------------------------------- | ----------------------------------------- |
| -4000 | INVALID\_ORDER\_STATUS: Invalid order status.                                                              | 订单状态无效：订单状态不符合操作要求                           | 检查订单状态（如仅对 NEW 状态的订单进行修改 / 撤销）            |
| -4001 | PRICE\_LESS\_THAN\_ZERO: Price less than 0.                                                                | 价格小于 0：价格参数为负数                               | 价格需≥0（实际需符合 PRICE\_FILTER 的 minPrice）     |
| -4002 | PRICE\_GREATER\_THAN\_MAX\_PRICE: Price greater than max price.                                            | 价格超上限：价格超出 PRICE\_FILTER 的 maxPrice          | 参考 exchangeInfo 接口的 PRICE\_FILTER 限制，调整价格 |
| -4003 | QTY\_LESS\_THAN\_ZERO: Quantity less than zero.                                                            | 数量小于 0：数量参数为负数                               | 数量需≥0（实际需符合 LOT\_SIZE 的 minQty）           |
| -4004 | QTY\_LESS\_THAN\_MIN\_QTY: Quantity less than min quantity.                                                | 数量低于最小值：数量小于 LOT\_SIZE 的 minQty              | 参考 exchangeInfo 接口的 LOT\_SIZE 限制，调整数量     |
| -4005 | QTY\_GREATER\_THAN\_MAX\_QTY: Quantity greater than max quantity.                                          | 数量超上限：数量超出 LOT\_SIZE 的 maxQty                | 拆分订单或调整数量至允许范围                            |
| -4006 | STOP\_PRICE\_LESS\_THAN\_ZERO: Stop price less than zero.                                                  | 触发价小于 0：stopPrice 参数为负数                      | 触发价需≥0（实际需符合 PRICE\_FILTER 的 minPrice）    |
| -4007 | STOP\_PRICE\_GREATER\_THAN\_MAX\_PRICE: Stop price greater than max price.                                 | 触发价超上限：stopPrice 超出 PRICE\_FILTER 的 maxPrice | 调整触发价至允许范围                                |
| -4013 | PRICE\_LESS\_THAN\_MIN\_PRICE: Price less than min price.                                                  | 价格低于最小值：价格小于 PRICE\_FILTER 的 minPrice        | 参考 exchangeInfo 接口的 PRICE\_FILTER，提高价格    |
| -4014 | PRICE\_NOT\_INCREASED\_BY\_TICK\_SIZE: Price not increased by tick size.                                   | 价格精度不符：价格不是 tickSize 的整数倍                    | 价格需满足（price - minPrice）% tickSize == 0    |
| -4015 | INVALID\_CL\_ORD\_ID\_LEN: Client order id length should not be more than 36 chars.                        | 自定义订单号过长：长度超过 36 字符                          | 缩短 newClientOrderId 至 36 字符内，符合正则规则       |
| -4016 | PRICE\_HIGHTER\_THAN\_MULTIPLIER\_UP: Price is higher than mark price multiplier cap.                      | 价格超振幅上限：价格高于标记价格 ×multiplierUp               | 参考 PERCENT\_PRICE 过滤器，降低委托价格              |
| -4023 | QTY\_NOT\_INCREASED\_BY\_STEP\_SIZE: Qty not increased by step size.                                       | 数量精度不符：数量不是 stepSize 的整数倍                    | 数量需满足（quantity - minQty）% stepSize == 0   |
| -4024 | PRICE\_LOWER\_THAN\_MULTIPLIER\_DOWN: Price is lower than mark price multiplier floor.                     | 价格超低振幅下限：价格低于标记价格 ×multiplierDown            | 参考 PERCENT\_PRICE 过滤器，提高委托价格              |
| -4028 | INVALID\_LEVERAGE: Leverage %s is not valid.                                                               | 杠杆无效：杠杆倍数不正确                                 | 杠杆需为 1-125 的整数，且符合交易对杠杆限制                 |
| -4059 | NO\_NEED\_TO\_CHANGE\_POSITION\_SIDE: No need to change position side.                                     | 无需变更持仓方向：当前已为目标持仓模式                          | 检查当前持仓模式（单向 / 双向），无需重复操作                  |
| -4060 | INVALID\_POSITION\_SIDE: Invalid position side.                                                            | 持仓方向无效：positionSide 参数错误                     | 单向模式仅支持 BOTH，双向模式支持 LONG/SHORT            |
| -4061 | POSITION\_SIDE\_NOT\_MATCH: Order's position side does not match user's setting.                           | 持仓方向不匹配：订单持仓方向与用户设置冲突                        | 调整订单的 positionSide 与账户持仓模式一致              |
| -4062 | REDUCE\_ONLY\_CONFLICT: Invalid or improper reduceOnly value.                                              | 仅减仓设置无效：reduceOnly 参数配置错误                    | 双开模式不支持 reduceOnly，仅减仓需与持仓方向匹配            |
| -4067 | POSITION\_SIDE\_CHANGE\_EXISTS\_OPEN\_ORDERS: Position side cannot be changed if there exists open orders. | 持仓方向变更失败：存在未成交订单                             | 先取消所有未成交订单，再变更持仓模式                        |
| -4068 | POSITION\_SIDE\_CHANGE\_EXISTS\_QUANTITY: Position side cannot be changed if there exists position.        | 持仓方向变更失败：存在持仓                                | 平掉所有持仓后，再变更持仓模式                           |
| -4164 | MIN\_NOTIONAL: Order's notional must be no smaller than %s.                                                | 最小名义价值不足：订单成交额低于 min\_notional               | 提高订单数量或价格，确保成交额≥最小名义价值（市价单用标记价格计算）        |
| -4167 | ISOLATED\_REJECT\_WITH\_JOINT\_MARGIN: Unable to adjust to Multi-Assets mode with isolated-margin symbols. | 多资产模式切换失败：存在逐仓模式交易对                          | 先将所有交易对切换为全仓模式，再切换多资产模式                   |
| -4168 | JOINT\_MARGIN\_REJECT\_WITH\_ISOLATED: Unable to adjust to isolated-margin mode under Multi-Assets mode.   | 逐仓模式切换失败：当前为多资产模式                            | 先关闭多资产模式，再切换逐仓模式                          |
| -4169 | JOINT\_MARGIN\_REJECT\_WITH\_MB: Unable to adjust Multi-Assets Mode with insufficient margin.              | 多资产模式切换失败：保证金不足                              | 增加保证金至要求金额后再切换                            |
| -4170 | JOINT\_MARGIN\_REJECT\_WITH\_OPEN\_ORDER: Unable to adjust Multi-Assets Mode with open orders.             | 多资产模式切换失败：存在未成交订单                            | 取消所有未成交订单后再切换                             |

 2.5 50xx：订单执行问题



| 错误代码  | 英文描述                                                                                           | 中文说明                          | 备注                                           |
| ----- | ---------------------------------------------------------------------------------------------- | ----------------------------- | -------------------------------------------- |
| -5021 | FOK\_ORDER\_REJECT: Due to the order could not be filled immediately.                          | FOK 订单被拒绝：无法立即全部成交            | FOK 订单需一次性全部成交，否则拒绝，可更换为 IOC 或 GTC 订单        |
| -5022 | GTX\_ORDER\_REJECT: Due to the order could not be executed as maker.                           | GTX 订单被拒绝：无法作为挂单方成交           | GTX 订单仅能作为挂单方，若将立即吃单则拒绝，调整价格后重试              |
| -5025 | LIMIT\_ORDER\_ONLY: Only limit order is supported.                                             | 仅支持限价单：当前操作仅支持 LIMIT 订单       | 更换订单类型为 LIMIT，再执行修改等操作                       |
| -5026 | Exceed\_Maximum\_Modify\_Order\_Limit: Exceed maximum modify order limit.                      | 改单次数超限：单个订单修改次数超出上限           | 同一订单最多修改 10000 次，需重新下单                       |
| -5027 | SAME\_ORDER: No need to modify the order.                                                      | 订单无变更：修改后的参数与原订单一致            | 无需重复提交相同参数的改单请求                              |
| -5037 | INVALID\_PRICE\_MATCH: Invalid price match.                                                    | 价格匹配模式无效：priceMatch 参数错误      | 可选值：OPPONENT/QUEUE 系列（参考枚举定义）                |
| -5038 | UNSUPPORTED\_ORDER\_TYPE\_PRICE\_MATCH: Price match only supports LIMIT/STOP/TAKE\_PROFIT.     | 订单类型不支持价格匹配：仅限价 / 止损 / 止盈单支持  | 更换订单类型为支持的类型，或移除 priceMatch 参数               |
| -5039 | INVALID\_SELF\_TRADE\_PREVENTION\_MODE: Invalid self trade prevention mode.                    | 自成交保护模式无效：参数错误                | 可选值：EXPIRE\_TAKER/EXPIRE\_MAKER/EXPIRE\_BOTH |
| -5040 | FUTURE\_GOOD\_TILL\_DATE: The goodTillDate timestamp must be greater than current time + 600s. | GTD 订单时间无效：goodTillDate 不符合要求 | 时间戳需 > 当前时间 + 600s 且 < 253402300799000（秒级精度） |
| -5043 | Existing\_Pending\_Modification: A pending modification already exists for this order.         | 改单请求处理中：该订单已有改单请求在执行          | 等待前一次改单完成后，再提交新的改单请求                         |

