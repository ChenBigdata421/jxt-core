package runtime

import "github.com/ChenBigdata421/jxt-core/storage"

// NewQueue 创建对应上下文队列
func NewQueue(prefix string, queue storage.AdapterQueue) storage.AdapterQueue {
	return &Queue{
		prefix: prefix,
		queue:  queue,
	}
}

type Queue struct {
	prefix string
	queue  storage.AdapterQueue
}

func (e *Queue) String() string {
	if e.queue == nil {
		return ""
	}
	return e.queue.String()
}

// Register 注册消费者
func (e *Queue) Register(name string, f storage.ConsumerFunc) {
	streamName := name
	if e.prefix != "" {
		streamName = e.prefix + ":" + name
	}
	e.queue.Register(streamName, f)
}

// Append 增加数据到生产者
func (e *Queue) Append(message storage.Messager) error {
	if e.prefix != "" {
		message.SetStream(e.prefix + ":" + message.GetStream())
	}
	// Stamp the tenant tag via the mutator. GetValues() returns a shallow copy,
	// so mutating it (as the prior code did) never reached the message's
	// internal map — PrefixKey was silently left unset on every prefixed Append.
	message.SetPrefix(e.prefix)
	return e.queue.Append(message)
}

// Run 运行
func (e *Queue) Run() {
	e.queue.Run()
}

// Shutdown 停止
func (e *Queue) Shutdown() {
	if e.queue != nil {
		e.queue.Shutdown()
	}
}
