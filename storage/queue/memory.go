package queue

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ChenBigdata421/jxt-core/storage"
)

type queue chan storage.Messager

// NewMemory 内存模式
func NewMemory(poolNum uint) *Memory {
	ctx, cancel := context.WithCancel(context.Background())
	return &Memory{
		queue:   new(sync.Map),
		ctx:     ctx,
		cancel:  cancel,
		PoolNum: poolNum,
	}
}

type Memory struct {
	queue   *sync.Map
	ctx     context.Context
	cancel  context.CancelFunc
	PoolNum uint
}

func (*Memory) String() string {
	return "memory"
}

func (m *Memory) makeQueue() queue {
	if m.PoolNum <= 0 {
		return make(queue)
	}
	return make(queue, m.PoolNum)
}

func (m *Memory) Append(message storage.Messager) error {
	memoryMessage := new(Message)
	memoryMessage.SetID(message.GetID())
	memoryMessage.SetStream(message.GetStream())
	memoryMessage.SetValues(message.GetValues())

	actual, _ := m.queue.LoadOrStore(message.GetStream(), m.makeQueue())
	q := actual.(queue)

	go func(gm storage.Messager, gq queue) {
		gm.SetID(uuid.New().String())
		select {
		case gq <- gm:
		case <-m.ctx.Done():
		}
	}(memoryMessage, q)
	return nil
}

func (m *Memory) Register(name string, f storage.ConsumerFunc) {
	actual, _ := m.queue.LoadOrStore(name, m.makeQueue())
	q := actual.(queue)

	go func(out queue, gf storage.ConsumerFunc) {
		var err error
		for message := range q {
			err = gf(message)
			if err != nil {
				if message.GetErrorCount() < 3 {
					message.SetErrorCount(message.GetErrorCount() + 1)
					// 每次间隔时长放大
					i := time.Second * time.Duration(message.GetErrorCount())
					select {
					case out <- message:
					case <-m.ctx.Done():
						return
					}
					time.Sleep(i)
				}
				err = nil
			}
		}
	}(q, f)
}

func (m *Memory) Run() {
	<-m.ctx.Done()
}

func (m *Memory) Shutdown() {
	m.cancel()
}
