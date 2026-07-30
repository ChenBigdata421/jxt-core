package eventbus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/IBM/sarama"
)

// errInvalidEnvelope 标识 Delivery 路径下 Envelope 解码失败（区别于业务 handler 返回的 error）。
var errInvalidEnvelope = errors.New("envelope decode failed for delivery")

// RawMeta 记录消费端实际收到的原始 broker record 完整性指纹（M15）。
// 所有字段来自消费方当时的 ConsumerMessage。PayloadHash 仅证明「后续读取的原始 record
// 与消费方当时记录一致」，不构成生产端 attestation——端到端证明须另立安全/合规项目。
type RawMeta struct {
	RawValue    []byte // 原始 ConsumerMessage.Value，未经解码
	RawKey      []byte
	Headers     []MessageHeader // 有序，允许重复 key；不是 map
	Topic       string
	Partition   int32
	Offset      int64
	Timestamp   time.Time
	PayloadHash string // sha256(RawValue) 十六进制
}

// EnvelopeDelivery 把解码后的 Envelope 与原始 record 指纹一起交付给 handler。
type EnvelopeDelivery struct {
	Envelope *Envelope
	Raw      RawMeta
}

// EnvelopeDeliveryHandler 是可靠消费装饰器唯一接受的 handler 形态（M15）。
// 旧 EnvelopeHandler 保留给非可靠调用，但可靠路径不静默降级。
type EnvelopeDeliveryHandler func(context.Context, EnvelopeDelivery) error

// toRawMeta 把 *sarama.ConsumerMessage 转成 RawMeta。Kafka 填满全部字段（M15）。
func toRawMeta(msg *sarama.ConsumerMessage) RawMeta {
	sum := sha256.Sum256(msg.Value)
	return RawMeta{
		RawValue:    append([]byte(nil), msg.Value...),
		RawKey:      append([]byte(nil), msg.Key...),
		Headers:     saramaToMessageHeaders(msg.Headers),
		Topic:       msg.Topic,
		Partition:   msg.Partition,
		Offset:      msg.Offset,
		Timestamp:   msg.Timestamp,
		PayloadHash: hex.EncodeToString(sum[:]),
	}
}

// invokeDelivery 解码 Envelope 并连同 RawMeta 交付给 delivery handler。
// 从 actor pool 的 Receive 抽出便于单测（不必拉起 Hollywood engine）。
func invokeDelivery(ctx context.Context, value []byte, raw RawMeta, h EnvelopeDeliveryHandler) error {
	env, err := FromBytes(value)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidEnvelope, err)
	}
	return h(ctx, EnvelopeDelivery{Envelope: env, Raw: raw})
}
