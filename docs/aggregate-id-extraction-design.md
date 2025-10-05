# 聚合ID（aggregateId）提取设计与最佳实践

## 目标
- 100% 可提取：任何被消费的事件都能稳定、可靠地提取到 aggregateId
- 跨中间件一致：Kafka / NATS（JetStream）统一策略
- 与顺序处理强耦合：用于 Keyed-Worker 池路由、严格顺序与背压
- 兼容演进：允许从现有系统平滑迁移，支持回退与告警

## 关键要求
- 确定性：相同消息在任何环境、任何实现中提取结果一致
- 明确来源：优先级清晰（Envelope > Header > Broker Key/Subject > 自定义规则）
- 可校验：提供格式校验（字符集/长度/格式），出现缺失或非法时有清晰处理策略（拒收/重试/DLQ）
- 性能友好：提取成本低（O(1) 访问 header / key；Envelope 解析轻量）

## 方案候选

### 方案 A：统一消息包络（Envelope）携带 aggregateId（推荐）
- 说明：定义统一消息结构（JSON / Protobuf），在 Envelope 中显式携带 `aggregate_id`、`event_version`、`event_type` 等元数据；业务负载放入 `payload` 字段。
- Kafka 对应：发布时仍将 `aggregate_id` 赋给 Kafka Key（利用分区有序与幂等），但提取以 Envelope 为准。
- NATS 对应：发布时将 `aggregate_id` 同步到 NATS Header 以便路由与观测；Subject 不强制包含 ID。
- 优点：
  - 跨中间件统一；解耦于 Kafka Key / NATS Subject 的差异
  - 易扩展（version、trace、租户、重试元数据等）
  - 便于签名、加密与兼容治理
- 缺点：
  - 需要定义/固化消息契约；需要小幅迁移成本

### 方案 B：强制 Header（X-Aggregate-ID, X-Event-Version）
- 说明：统一约定消息头 `X-Aggregate-ID`、`X-Event-Version`。发布端负责设置，消费端从 Header 提取。
- Kafka 对应：可同时设置 Kafka Key=aggregateId，Header 作为提取数据源
- NATS 对应：使用 NATS Header 原生支持
- 优点：
  - 无需解析 Envelope，提取开销更低
  - 适配现有消息体变更小
- 缺点：
  - Header 容易被不同生产者实现遗漏；治理成本需更严格
  - Header 语义不如 Envelope 丰富，未来扩展受限

### 方案 C：把 aggregateId 编入主题/科目（Topic/Subject）
- 说明：将 ID 放入 NATS Subject 的一个 segment，如 `orders.{aggregateId}.events`；Kafka 不建议在 Topic 层承载 ID（会导致主题数量爆炸）。
- 优点：
  - 对于 NATS 强 Subject 场景路由简单
- 缺点：
  - Kafka 不可取；NATS 也会存在 Subject 维度膨胀与 ACL、监控复杂度上升
  - ID 过多将影响订阅与服务端元数据开销

### 方案 D：从业务负载（Payload）中派生
- 说明：通过规则从 JSON/Proto 负载字段推断 ID（如 `order.id`）
- 优点：
  - 不改变外层结构，迁移成本最低
- 缺点：
  - 容易被不同业务模型破坏；规则繁琐且脆弱
  - 解析成本更高；与语言/序列化强耦合

### 方案 E：CloudEvents 规范
- 说明：采用 CloudEvents，使用 `subject` 或扩展属性承载 aggregateId，生态成熟
- 优点：
  - 标准化，生态工具丰富
- 缺点：
  - 对现有系统改造较大；落地复杂度高于自定义 Envelope


### 方案 F：SDK 元数据（Watermill 风格）
- 说明：在 SDK 层提供统一消息对象（如 Watermill 的 message.Message），通过 `Metadata`（map[string]string）承载 `aggregate_id`、`event_version` 等元数据；不同中间件由 SDK 适配：能映射 Header 的映射为 Header；不支持 Header 的则编码进消息体。
- 优点：
  - 对应用开发者友好：统一 API（msg.Metadata.Get/Set），无需关心不同中间件差异
  - 易测试、易扩展：可在 SDK 中集中校验/埋点/追踪；跨中间件语义一致
  - 与方案 B（Header）在落地层面高度兼容：Kafka/NATS 通常映射为原生 Header
- 缺点：
  - 生产者必须统一依赖 SDK；跨语言/跨团队接入时一致性需要强治理
  - 对不支持 Header 的通道需退化为“写入载荷”或 Envelope，存在双写与一致性要求

- 发布者（Watermill 风格）示例（Go）

## Watermill 元数据方案（方案F）与 A/B 的对比与推荐

- 能力与落地方式：
  - 方案F（SDK Metadata）在 SDK 层形成统一契约与 API，底层通常映射为中间件 Header（等价于方案B），或在不支持 Header 的通道上退化为写入载荷/Envelope（与方案A协同）。
  - 方案A（Envelope）在“线上的字节序列”层面定义了强契约，跨语言/跨团队/跨中间件最稳妥；方案B（Header）在支持 Header 的通道上更轻量。
- 跨语言与治理：
  - 方案F 需要所有生产者都通过统一 SDK 才能保证一致性；若存在非 SDK 生产者（异构语言/团队），一致性风险上升。
  - 方案A 通过线上的 Envelope 契约天然跨语言，治理与合规更强；方案B 需确保不同生产者都“不漏写头”。
- 性能与复杂度：
  - 方案B/F 提取成本最低（读取 Header/Metadata）；方案A 需要解析 Envelope（轻量 JSON/Proto）。
- 顺序与版本校验：
  - 三者均能承载 aggregate_id 与 event_version。方案A 在“可观测/审计/加密签名/合规”上更直观（契约即载荷），方案B/F 更偏工程落地效率。
- 运维与可观测：
  - 方案F 可在 SDK 集中打点与校验，但依赖 SDK 覆盖度；方案A 在通道层可直接观测字段（取决于工具是否能解析载荷）；方案B 可直接在 Broker Header 侧观测。

- 推荐选择：
  - 全 Go/统一 SDK 团队：优先 方案F（SDK Metadata）+ 同步写入中间件 Header（等效方案B），获取最低改动成本与良好工程体验。
  - 跨语言/跨团队/对外开放契约：优先 方案A（Envelope）作为长期契约；短期可用 方案B/F 过渡，SDK 同时写入 Header 与 Envelope，保证一致。
  - 最佳实践（组合）：SDK 写 Metadata（F）→ 映射写 Header（B）→ 消息体使用 Envelope（A）。消费侧提取优先级：Envelope > Header/Metadata > Key/Subject。

<augment_code_snippet mode="EXCERPT">
````go
msg := message.NewMessage(uuid, payload)
msg.Metadata.Set("aggregate_id", aggID)
msg.Metadata.Set("event_version", strconv.FormatInt(ver,10))
publisher.Publish(topic, msg)
````
</augment_code_snippet>

- 订阅者提取示例（Go）
<augment_code_snippet mode="EXCERPT">
````go
aggID := msg.Metadata.Get("aggregate_id")
if aggID == "" { /* 回退到 Header/Key/Subject 或入 DLQ */ }
````
</augment_code_snippet>

## 业界最佳实践分析与推荐

### 业界主流做法
真正的业界"最佳实践"是：**以统一事件契约为基石（方案A：Envelope 或等价的标准如 CloudEvents/Avro/Protobuf schema），在消息体中明确携带 aggregate_id 与 event_version；同时在具体中间件层按需镜像关键字段**：
- **Kafka**：将 aggregate_id 放到 record key 以驱动分区与顺序
- **Kafka/NATS**：将 aggregate_id、event_version 同步到 Header 便于观测与排障

### 为什么是 A + Key + Header 组合
- **互操作与可治理**：跨语言、跨团队、跨中间件普遍依赖"消息体契约"为真实来源（CloudEvents、Confluent Schema Registry 等均倡导消息 schema 封装关键业务元数据）。这让审核、合规、签名、回放、离线处理都有统一依据。
- **顺序与扩展**：Kafka 生态的通行做法是"payload schema 携带实体标识与序号 + record key 控制分区与顺序"，并可在 Header 中镜像关键信息供可观测/排障。
- **降风险**：Header 可能在跨桥接、网关、非一致生产者时被遗漏或篡改；SDK Metadata 依赖统一 SDK 的覆盖度与纪律。把关键字段放进消息体契约是更稳妥的根基。

### 其他方案的适用场景
- **方案B（Header-only）**：仅建议在强一致的内部、同质化链路里做"短期最小改动落地"；仍应规划升级为"消息体契约为准，Header 为镜像"。
- **方案F（SDK Metadata）**：在全 Go/统一 SDK 的组织内非常高效；但对外或多语言生态，仍建议将 Metadata 映射为 Header 并写入消息体契约，避免被 SDK 覆盖度限制。

### 推荐方案与理由
选择“方案 A：统一消息包络（Envelope）”，并配合以下映射确保端到端一致：
- 发布侧：
  - Kafka：`Kafka Key = aggregate_id`；Envelope.payload 装载业务数据；Headers 附带 `X-Aggregate-ID`、`X-Event-Version`
  - NATS：NATS Header 附带 `X-Aggregate-ID`、`X-Event-Version`；Subject 不强制编码 ID（避免 Subject 爆炸）
- 消费侧提取优先级：Envelope > Header > Broker Key/Subject > 自定义规则（最后兜底）
- 理由：
  - **符合业界最佳实践**：消息体契约为唯一真实来源，传输层镜像为工程便捷
  - 跨 Kafka/NATS 一致与可移植；契约清晰、可治理
  - 与顺序处理（Keyed-Worker）天然匹配，支持将来引入 version gap 校验
  - 兼具性能（轻量解析）与可扩展性（元数据演进/合规/可观测）

### 落地建议
- **长期契约**：采用方案A（统一 Envelope/schema，含 aggregate_id + event_version），并在 Kafka 设置 key、在 Kafka/NATS 写 Header
- **过渡期**：可同时启用 B/F，保持向后兼容；消费侧以"Envelope > Header/Metadata > Key/Subject"的优先级提取
- **最终形态**：A（消息体契约）为唯一真实来源，B/F 为镜像与工程便捷。这样既符合业界最佳实践，也与当前 Keyed-Worker 顺序处理天然契合


## 采用方案A/B时，与“无需提取”基线方式的差异

对比基线（发布者仅发送原始业务负载，订阅者直接处理负载，不关心 aggregateId）时，采用方案A（Envelope）与方案B（Header）在发布与订阅侧的差异如下：

### 方案 A（Envelope）
- 发布者侧：
  - 必做：
    - 将业务负载放入 Envelope.payload，补齐 `aggregate_id`、`event_version`、`event_type`、`timestamp` 等必需元数据。
    - Kafka：同时将 `aggregate_id` 设置为 Kafka Key；可同步写入 `X-Aggregate-ID`/`X-Event-Version` Header 便于观测。
    - NATS：将 `X-Aggregate-ID`/`X-Event-Version` 写入 NATS Header（Subject 无需包含 ID）。
  - 可选：
    - 使用 SDK 的 MessageFormatter 统一完成编码/校验/补全（减少重复代码与漏配风险）。
  - 相对基线的额外开销/变化：
    - 需要序列化 Envelope（JSON/Proto）；消息体体积略增（少量元数据）。
    - 需要在发布路径做基础校验（ID/版本/长度/字符集），校验失败走重试/拒收/告警。
    - 需要在发布代码或网关统一注入 Formatter（一次性改动）。

- 订阅者侧：
  - 必做：
    - 在业务处理前先解析 Envelope 并提取 `aggregate_id`；缺失/非法按策略（重试/DLQ）。
    - 使用提取到的 `aggregate_id` 路由到 Keyed-Worker 池，按 key 串行处理；处理完成再 ack/commit。
  - 可选：
    - 校验 `event_version` 单调递增（为后续严格不越过前驱打基础）。
  - 相对基线的额外开销/变化：
    - 在处理前多一步轻量解析与校验；
    - ack 时机从“接收后立即”改为“业务处理完成后”（严格顺序+幂等更安全）。

### 方案 B（Header）
- 发布者侧：
  - 必做：
    - 统一写入 Header：`X-Aggregate-ID`、`X-Event-Version`。
    - Kafka：建议同时设置 Kafka Key=aggregateId（分区有序、幂等/Exactly-Once 架构友好）。
  - 可选：
    - 可继续发送原有业务负载格式（无需 Envelope），改动最小。
  - 相对基线的额外开销/变化：
    - 需要在发布路径补齐 Header；
    - 不解析/改造负载本身，体积与序列化成本与基线近似。

- 订阅者侧：
  - 必做：
    - 先从 Header 提取 `X-Aggregate-ID`；缺失时可回退到 Kafka Key / 其它来源；最终缺失走重试/DLQ。
    - 使用提取到的 `aggregate_id` 路由至 Keyed-Worker 池，处理完成后再 ack/commit。
  - 可选：
    - 校验 `X-Event-Version` 单调递增。
  - 相对基线的额外开销/变化：
    - 无需解析 Envelope，读取 Header 成本极低；
    - 同样需要延后 ack 到处理完成阶段以保证严格顺序。

### 两种方案共同的订阅侧变化点（相对“无需提取”基线）
- 引入统一的 ExtractAggregateID 步骤与校验逻辑（A：解析 Envelope；B：读取 Header；均可回退到 Kafka Key/NATS Subject）。
- 将消息提交/确认延后到“业务处理完成”之后，避免乱序可见性（严格顺序要求）。
- 接入 Keyed-Worker 池：同 key 串行、不同 key 并发；当某 worker 队列满时对上游实施背压（Kafka Pause/Resume 或 NATS 降速/Flow Control）。
- 错误治理：缺失/非法 ID、解析失败、重试超限进入 DLQ，并携带 `aggregateId`、`eventVersion`、offset/sequence、错误栈以便排障。

### 实践选择建议
- 若可以接受轻量契约升级并希望获得更强的可治理性/可扩展性：优先方案 A（Envelope）。
- 若短期以“最小改动成本”落地为目标：优先方案 B（Header），并在中期演进至方案 A（两者可共存一段时间）。


### 代码对比：发布者（Kafka / NATS）

- 基线（无需提取 aggregateId）

````go
// Kafka（基线）
msg := &sarama.ProducerMessage{Topic: topic, Value: sarama.ByteEncoder(payload)}
_, _, _ = producer.SendMessage(msg)

// NATS（基线）
_ = nc.Publish(subject, payload)
````

- 方案 A（Envelope）

````go
// 定义 Envelope（JSON 举例）
type Envelope struct {
    AggregateID  string          `json:"aggregate_id"`
    EventType    string          `json:"event_type"`
    EventVersion int64           `json:"event_version"`
    Timestamp    time.Time       `json:"timestamp"`
    Payload      json.RawMessage `json:"payload"`
}

// Kafka：Envelope + Key + Headers
envBytes, _ := json.Marshal(Envelope{AggregateID: aggID, EventType: et, EventVersion: ver, Timestamp: time.Now(), Payload: payload})
msg := &sarama.ProducerMessage{
    Topic:   topic,
    Key:     sarama.StringEncoder(aggID), // 分区有序
    Value:   sarama.ByteEncoder(envBytes),
    Headers: []sarama.RecordHeader{{Key: []byte("X-Aggregate-ID"), Value: []byte(aggID)}, {Key: []byte("X-Event-Version"), Value: []byte(strconv.FormatInt(ver,10))}},
}
_, _, _ = producer.SendMessage(msg)

// NATS：Envelope + Headers（Subject 无需 ID）
msgN := &nats.Msg{Subject: subject, Data: envBytes, Header: nats.Header{}}
msgN.Header.Set("X-Aggregate-ID", aggID)
msgN.Header.Set("X-Event-Version", strconv.FormatInt(ver, 10))
_ = nc.PublishMsg(msgN)
````

- 方案 B（Header）

````go
// Kafka：原有负载 + Key + Headers（无需 Envelope）
msg := &sarama.ProducerMessage{
    Topic:   topic,
    Key:     sarama.StringEncoder(aggID),
    Value:   sarama.ByteEncoder(payload),
    Headers: []sarama.RecordHeader{{Key: []byte("X-Aggregate-ID"), Value: []byte(aggID)}, {Key: []byte("X-Event-Version"), Value: []byte(strconv.FormatInt(ver,10))}},
}
_, _, _ = producer.SendMessage(msg)

// NATS：原有负载 + Headers（无需 Envelope）
msgN := &nats.Msg{Subject: subject, Data: payload, Header: nats.Header{}}
msgN.Header.Set("X-Aggregate-ID", aggID)
msgN.Header.Set("X-Event-Version", strconv.FormatInt(ver, 10))
_ = nc.PublishMsg(msgN)
````

### 代码对比：订阅者（Kafka / NATS）

- 基线（无需提取 aggregateId）

````go
// Kafka：直接处理负载，常见做法是处理后 MarkMessage（此处演示差异对比）
for msg := range claim.Messages() {
    _ = handler(ctx, msg.Value) // 无序约束
    session.MarkMessage(msg, "")
}

// NATS：直接处理并 Ack
sub, _ := js.PullSubscribe(subject, durable)
msgs, _ := sub.Fetch(1, nats.MaxWait(time.Second))
for _, m := range msgs {
    _ = handler(ctx, m.Data) // 无序约束
    _ = m.Ack()
}
````

- 方案 A（Envelope）：解析 Envelope 提取 aggregateId，路由 Keyed-Worker，处理完成后再 ack/commit

````go
// Kafka
for msg := range claim.Messages() {
    var env Envelope
    if err := json.Unmarshal(msg.Value, &env); err != nil { /* 重试/入DLQ */ continue }
    aggID := env.AggregateID
    aggMsg := &AggregateMessage{AggregateID: aggID, Value: env.Payload, Context: ctx, Done: make(chan error, 1)}
    if err := keyedPool.ProcessMessage(ctx, aggMsg); err != nil { /* backpressure: 暂停分区等 */ continue }
    if err := <-aggMsg.Done; err != nil { /* 重试/入DLQ */ continue }
    session.MarkMessage(msg, "") // 处理完成后再提交
}

// NATS
msgs, _ := sub.Fetch(1, nats.MaxWait(time.Second))
for _, m := range msgs {
    var env Envelope
    if err := json.Unmarshal(m.Data, &env); err != nil { /* 重试/入DLQ */ continue }
    aggMsg := &AggregateMessage{AggregateID: env.AggregateID, Value: env.Payload, Context: ctx, Done: make(chan error, 1)}
    if err := keyedPool.ProcessMessage(ctx, aggMsg); err != nil { /* 降低拉取速率/Flow Control */ continue }
    if err := <-aggMsg.Done; err != nil { /* 重试/入DLQ */ continue }
    _ = m.Ack() // 处理完成后再 Ack
}
````

- 方案 B（Header）：从 Header（缺失时回退到 Key/Subject）提取 aggregateId，路由 Keyed-Worker，处理完成后再 ack/commit

````go
// Kafka（优先 Header，其次 Key）
extractID := func(headers []*sarama.RecordHeader, key []byte) string {
    for _, h := range headers { if string(h.Key) == "X-Aggregate-ID" { return string(h.Value) } }
    if len(key) > 0 { return string(key) }
    return ""
}
for msg := range claim.Messages() {
    aggID := extractID(msg.Headers, msg.Key)
    if aggID == "" { /* 重试/入DLQ */ continue }
    aggMsg := &AggregateMessage{AggregateID: aggID, Value: msg.Value, Context: ctx, Done: make(chan error, 1)}
    if err := keyedPool.ProcessMessage(ctx, aggMsg); err != nil { /* backpressure */ continue }
    if err := <-aggMsg.Done; err != nil { /* 重试/入DLQ */ continue }
    session.MarkMessage(msg, "")
}

// NATS（Header 提取）
aggID := m.Header.Get("X-Aggregate-ID")
if aggID == "" { /* 重试/入DLQ */ }
aggMsg := &AggregateMessage{AggregateID: aggID, Value: m.Data, Context: ctx, Done: make(chan error, 1)}
if err := keyedPool.ProcessMessage(ctx, aggMsg); err == nil { if err2 := <-aggMsg.Done; err2 == nil { _ = m.Ack() } }
````

## Envelope 契约（JSON 示例）

- 字段建议：
  - aggregate_id: string（必填）
  - event_type: string（必填）
  - event_version: int64（必填，单聚合内单调递增）
  - timestamp: RFC3339/epoch
  - payload: 任意对象

示例：
<augment_code_snippet mode="EXCERPT">
````json
{
  "aggregate_id": "order-8f1a2c",
  "event_type": "OrderPaid",
  "event_version": 42,
  "timestamp": "2025-09-20T10:20:30Z",
  "payload": {"orderId":"8f1a2c", "amount": 100}
}
````
</augment_code_snippet>

## 提取顺序与伪代码

优先级：Envelope > Header > Broker Key/Subject > Fallback

<augment_code_snippet mode="EXCERPT">
````go
func ExtractAggregateID(msg []byte, headers map[string][]byte, kafkaKey []byte, subject string) (string, error) {
    if id := tryFromEnvelope(msg); id != "" { return id, nil }
    if id := getHeader(headers, "X-Aggregate-ID"); id != "" { return id, nil }
    if len(kafkaKey) > 0 { return string(kafkaKey), nil }
    if id := tryFromSubject(subject); id != "" { return id, nil }
    return "", errors.New("aggregateId not found")
}
````
</augment_code_snippet>

## 校验规则
- 允许字符：`[A-Za-z0-9:_-]`（可按需扩展）；长度 ≤ 256；去除首尾空白
- 非法/缺失处理：
  - 重试预算内：记录告警并重试（指数退避）
  - 超过预算：入 DLQ，携带原始头、offset/sequence、错误信息
- 观测：
  - 指标：`eventbus.aggregate_id.missing.count`、`invalid.count`、`source.breakdown`（envelope/header/key/subject）
  - 日志：聚合 ID 缺失/非法，伴随 topic/partition/offset 或 stream/consumer/sequence

## 与 Keyed-Worker 的协作
- 路由：`workerIndex = hash(aggregate_id) % M`
- 顺序：同 key 串行；不同 key 并发
- 背压：当某 worker 队列满，消费侧可 Pause/Resume（Kafka）或降低拉取速率/收紧 MaxAckPending（NATS）
- 版本：建议事件携带 `event_version`，后续可在 Worker 内维护 `lastVersion` 与 gap 缓冲，严格不越过前驱

## 发布侧对齐要求
- Kafka Publisher：
  - 将 `aggregate_id` 作为 Kafka Key
  - Headers 设置 `X-Aggregate-ID`、`X-Event-Version`
  - 消息体使用 Envelope（推荐）
- NATS Publisher：
  - Headers 设置 `X-Aggregate-ID`、`X-Event-Version`
  - 消息体使用 Envelope（推荐）

## 兼容与迁移
- Phase 0：消费侧按优先级提取（Envelope > Header > Key/Subject），开启观测与告警，统计缺失/来源
- Phase 1：生产侧逐步统一发送 Envelope，并同步写入 Header 与（Kafka）Key
- Phase 2：启用强校验（缺失/非法直接拒收或入 DLQ），引入严格 version gap 校验

## 风险与边界
- 多生产者不一致：通过网关/SDK 强约束（MessageFormatter）与 CI 契约校验解决
- 超长/非法 ID：强校验+告警+DLQ；必要时采用规范化/截断+原值保留头字段
- NATS Subject 爆炸：不建议将 aggregateId 编入 Subject；若必须，限制模式与粒度

## 实施计划（最小变更）
1) 在 SDK 定义/强化 MessageFormatter：支持 Envelope 编解码、Header/Key 同步、ExtractAggregateID 标准实现
2) Kafka/NATS Publish：填充 Envelope + Header；Kafka 同步设置 Key
3) Kafka/NATS Subscribe：按“提取顺序”统一提取 aggregateId，路由到 Keyed-Worker 池
4) 增加单测/集成测试：缺失/非法/多来源混合、回退路径、性能回归

---
如需，我可以直接提交 MessageFormatter 的最小实现（含提取函数与单测），并将 Kafka/NATS 的订阅路径切换为使用该标准提取逻辑。
