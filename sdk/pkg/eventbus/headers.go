package eventbus

import "github.com/IBM/sarama"

// MessageHeader 是一条 broker header 的保真表示：有序、允许重复 key。
// PoisonMessage.Headers（C6）与 RawMeta.Headers（M15）共用此类型。
// 不用 map[string]string：map 既丢顺序又会合并重复 key——正是 C6 要修的根因。
type MessageHeader struct {
	Key   string
	Value []byte
}

// saramaToMessageHeaders 把 []*sarama.RecordHeader 原样转换为 []MessageHeader，
// 保留顺序与重复 key。toPoisonMessage 与 toRawMeta 共用（DRY）。
func saramaToMessageHeaders(in []*sarama.RecordHeader) []MessageHeader {
	out := make([]MessageHeader, 0, len(in))
	for _, h := range in {
		if h == nil {
			continue
		}
		out = append(out, MessageHeader{
			Key:   string(h.Key),
			Value: append([]byte(nil), h.Value...), // 防御性拷贝，避免别名源切片
		})
	}
	return out
}
