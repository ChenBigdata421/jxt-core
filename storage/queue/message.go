package queue

import (
	"sync"

	"github.com/ChenBigdata421/jxt-core/storage"
)

// Message is a standalone queue message with no external dependencies.
// Fields are unexported to enforce mutex-protected access.
type Message struct {
	id         string
	stream     string
	values     map[string]interface{}
	errorCount int
	mux        sync.RWMutex
}

func (m *Message) GetID() string {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.id
}

func (m *Message) GetStream() string {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.stream
}

// GetValues returns a shallow copy of the values map so callers cannot
// mutate the Message's internal state without going through setters.
func (m *Message) GetValues() map[string]interface{} {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if m.values == nil {
		return nil
	}
	cp := make(map[string]interface{}, len(m.values))
	for k, v := range m.values {
		cp[k] = v
	}
	return cp
}

func (m *Message) SetID(id string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.id = id
}

func (m *Message) SetStream(stream string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.stream = stream
}

func (m *Message) SetValues(values map[string]interface{}) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.values = values
}

func (m *Message) GetPrefix() (prefix string) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if m.values == nil {
		return
	}
	v, _ := m.values[storage.PrefixKey]
	prefix, _ = v.(string)
	return
}

func (m *Message) SetPrefix(prefix string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.values == nil {
		m.values = make(map[string]interface{})
	}
	m.values[storage.PrefixKey] = prefix
}

func (m *Message) SetErrorCount(count int) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.errorCount = count
}

func (m *Message) GetErrorCount() int {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.errorCount
}
