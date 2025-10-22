package function_regression_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelper 测试辅助工具
type TestHelper struct {
	t *testing.T
}

// NewTestHelper 创建测试辅助工具
func NewTestHelper(t *testing.T) *TestHelper {
	return &TestHelper{t: t}
}

// AssertNoError 断言无错误
func (h *TestHelper) AssertNoError(err error, msgAndArgs ...interface{}) {
	assert.NoError(h.t, err, msgAndArgs...)
}

// RequireNoError 要求无错误
func (h *TestHelper) RequireNoError(err error, msgAndArgs ...interface{}) {
	require.NoError(h.t, err, msgAndArgs...)
}

// AssertEqual 断言相等
func (h *TestHelper) AssertEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.Equal(h.t, expected, actual, msgAndArgs...)
}

// AssertNotEmpty 断言非空
func (h *TestHelper) AssertNotEmpty(obj interface{}, msgAndArgs ...interface{}) {
	assert.NotEmpty(h.t, obj, msgAndArgs...)
}

// AssertNil 断言为 nil
func (h *TestHelper) AssertNil(obj interface{}, msgAndArgs ...interface{}) {
	assert.Nil(h.t, obj, msgAndArgs...)
}

// AssertNotNil 断言不为 nil
func (h *TestHelper) AssertNotNil(obj interface{}, msgAndArgs ...interface{}) {
	assert.NotNil(h.t, obj, msgAndArgs...)
}

// AssertTrue 断言为 true
func (h *TestHelper) AssertTrue(value bool, msgAndArgs ...interface{}) {
	assert.True(h.t, value, msgAndArgs...)
}

// AssertFalse 断言为 false
func (h *TestHelper) AssertFalse(value bool, msgAndArgs ...interface{}) {
	assert.False(h.t, value, msgAndArgs...)
}

// AssertGreater 断言大于
func (h *TestHelper) AssertGreater(e1, e2 interface{}, msgAndArgs ...interface{}) {
	assert.Greater(h.t, e1, e2, msgAndArgs...)
}

// AssertContains 断言包含
func (h *TestHelper) AssertContains(s, contains interface{}, msgAndArgs ...interface{}) {
	assert.Contains(h.t, s, contains, msgAndArgs...)
}

// AssertNotEqual 断言两个值不相等
func (h *TestHelper) AssertNotEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.NotEqual(h.t, expected, actual, msgAndArgs...)
}

// AssertRegex 断言字符串匹配正则表达式
func (h *TestHelper) AssertRegex(pattern, s string, msgAndArgs ...interface{}) {
	assert.Regexp(h.t, pattern, s, msgAndArgs...)
}

// CreateTestEvent 创建测试事件
func (h *TestHelper) CreateTestEvent(tenantID, aggregateType, aggregateID, eventType string) *outbox.OutboxEvent {
	payload := map[string]interface{}{
		"test":      true,
		"timestamp": time.Now().Unix(),
	}

	payloadBytes, err := json.Marshal(payload)
	h.RequireNoError(err, "Failed to marshal payload")

	event, err := outbox.NewOutboxEvent(
		tenantID,
		aggregateType,
		aggregateID,
		eventType,
		payloadBytes,
	)
	h.RequireNoError(err, "Failed to create outbox event")

	return event
}

// CreateTestEventWithPayload 创建带自定义负载的测试事件
func (h *TestHelper) CreateTestEventWithPayload(tenantID, aggregateType, aggregateID, eventType string, payload interface{}) *outbox.OutboxEvent {
	payloadBytes, err := json.Marshal(payload)
	h.RequireNoError(err, "Failed to marshal payload")

	event, err := outbox.NewOutboxEvent(
		tenantID,
		aggregateType,
		aggregateID,
		eventType,
		payloadBytes,
	)
	h.RequireNoError(err, "Failed to create outbox event")

	return event
}

// MockRepository 模拟仓储
type MockRepository struct {
	events                 map[string]*outbox.OutboxEvent
	idempotencyKeyIndex    map[string]*outbox.OutboxEvent
	saveError              error
	updateError            error
	findPendingError       error
	findByIDError          error
	findByIdempotencyError error
	mu                     sync.RWMutex
}

// NewMockRepository 创建模拟仓储
func NewMockRepository() *MockRepository {
	return &MockRepository{
		events:              make(map[string]*outbox.OutboxEvent),
		idempotencyKeyIndex: make(map[string]*outbox.OutboxEvent),
	}
}

// Save 保存事件
func (m *MockRepository) Save(ctx context.Context, event *outbox.OutboxEvent) error {
	if m.saveError != nil {
		return m.saveError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.events[event.ID] = event
	if event.IdempotencyKey != "" {
		m.idempotencyKeyIndex[event.IdempotencyKey] = event
	}

	return nil
}

// SaveBatch 批量保存事件
func (m *MockRepository) SaveBatch(ctx context.Context, events []*outbox.OutboxEvent) error {
	for _, event := range events {
		if err := m.Save(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Update 更新事件
func (m *MockRepository) Update(ctx context.Context, event *outbox.OutboxEvent) error {
	if m.updateError != nil {
		return m.updateError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.events[event.ID] = event
	return nil
}

// BatchUpdate 批量更新事件
func (m *MockRepository) BatchUpdate(ctx context.Context, events []*outbox.OutboxEvent) error {
	for _, event := range events {
		if err := m.Update(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// FindPendingEvents 查找待发布事件
func (m *MockRepository) FindPendingEvents(ctx context.Context, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	if m.findPendingError != nil {
		return nil, m.findPendingError
	}

	var pending []*outbox.OutboxEvent
	for _, event := range m.events {
		if event.Status == outbox.EventStatusPending {
			if tenantID == "" || event.TenantID == tenantID {
				pending = append(pending, event)
				if len(pending) >= limit {
					break
				}
			}
		}
	}

	return pending, nil
}

// FindPendingEventsWithDelay 查找创建时间超过指定延迟的待发布事件
func (m *MockRepository) FindPendingEventsWithDelay(ctx context.Context, tenantID string, delaySeconds int, limit int) ([]*outbox.OutboxEvent, error) {
	if m.findPendingError != nil {
		return nil, m.findPendingError
	}

	var pending []*outbox.OutboxEvent
	now := time.Now()
	delayDuration := time.Duration(delaySeconds) * time.Second

	for _, event := range m.events {
		if event.Status == outbox.EventStatusPending {
			// 检查事件创建时间是否超过延迟
			if now.Sub(event.CreatedAt) >= delayDuration {
				if tenantID == "" || event.TenantID == tenantID {
					pending = append(pending, event)
					if len(pending) >= limit {
						break
					}
				}
			}
		}
	}

	return pending, nil
}

// FindByID 根据 ID 查找事件
func (m *MockRepository) FindByID(ctx context.Context, id string) (*outbox.OutboxEvent, error) {
	if m.findByIDError != nil {
		return nil, m.findByIDError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	event, exists := m.events[id]
	if !exists {
		return nil, nil
	}

	return event, nil
}

// FindByAggregateID 根据聚合根 ID 查找事件
func (m *MockRepository) FindByAggregateID(ctx context.Context, aggregateID string, tenantID string) ([]*outbox.OutboxEvent, error) {
	var result []*outbox.OutboxEvent
	for _, event := range m.events {
		if event.AggregateID == aggregateID {
			if tenantID == "" || event.TenantID == tenantID {
				result = append(result, event)
			}
		}
	}
	return result, nil
}

// FindByIdempotencyKey 根据幂等性键查找事件
func (m *MockRepository) FindByIdempotencyKey(ctx context.Context, key string) (*outbox.OutboxEvent, error) {
	if m.findByIdempotencyError != nil {
		return nil, m.findByIdempotencyError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	event, exists := m.idempotencyKeyIndex[key]
	if !exists {
		return nil, nil
	}

	return event, nil
}

// ExistsByIdempotencyKey 检查幂等性键是否已存在
func (m *MockRepository) ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
	_, exists := m.idempotencyKeyIndex[idempotencyKey]
	return exists, nil
}

// MarkAsPublished 标记事件为已发布
func (m *MockRepository) MarkAsPublished(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	event, exists := m.events[id]
	if !exists {
		return fmt.Errorf("event not found: %s", id)
	}
	event.MarkAsPublished()
	return nil
}

// MarkAsFailed 标记事件为失败
func (m *MockRepository) MarkAsFailed(ctx context.Context, id string, err error) error {
	event, exists := m.events[id]
	if !exists {
		return fmt.Errorf("event not found: %s", id)
	}
	event.MarkAsFailed(err)
	return nil
}

// IncrementRetry 增加重试次数
func (m *MockRepository) IncrementRetry(ctx context.Context, id string, errorMsg string) error {
	event, exists := m.events[id]
	if !exists {
		return fmt.Errorf("event not found: %s", id)
	}
	event.IncrementRetry(errorMsg)
	return nil
}

// MarkAsMaxRetry 标记事件为超过最大重试次数
func (m *MockRepository) MarkAsMaxRetry(ctx context.Context, id string, errorMsg string) error {
	event, exists := m.events[id]
	if !exists {
		return fmt.Errorf("event not found: %s", id)
	}
	event.MarkAsMaxRetry(errorMsg)
	return nil
}

// IncrementRetryCount 增加重试次数（已废弃）
func (m *MockRepository) IncrementRetryCount(ctx context.Context, id string) error {
	return m.IncrementRetry(ctx, id, "")
}

// Delete 删除事件
func (m *MockRepository) Delete(ctx context.Context, id string) error {
	delete(m.events, id)
	return nil
}

// DeleteBatch 批量删除事件
func (m *MockRepository) DeleteBatch(ctx context.Context, ids []string) error {
	for _, id := range ids {
		delete(m.events, id)
	}
	return nil
}

// FindFailedEvents 查找失败的事件
func (m *MockRepository) FindFailedEvents(ctx context.Context, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	var failed []*outbox.OutboxEvent
	for _, event := range m.events {
		if event.Status == outbox.EventStatusFailed {
			if tenantID == "" || event.TenantID == tenantID {
				failed = append(failed, event)
				if len(failed) >= limit {
					break
				}
			}
		}
	}
	return failed, nil
}

// FindMaxRetryEvents 查找超过最大重试次数的事件
func (m *MockRepository) FindMaxRetryEvents(ctx context.Context, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	var maxRetry []*outbox.OutboxEvent
	for _, event := range m.events {
		if event.Status == outbox.EventStatusMaxRetry {
			if tenantID == "" || event.TenantID == tenantID {
				maxRetry = append(maxRetry, event)
				if len(maxRetry) >= limit {
					break
				}
			}
		}
	}
	return maxRetry, nil
}

// FindScheduledEvents 查找计划发布的事件
func (m *MockRepository) FindScheduledEvents(ctx context.Context, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	var scheduled []*outbox.OutboxEvent
	now := time.Now()
	for _, event := range m.events {
		if event.ScheduledAt != nil && event.ScheduledAt.Before(now) && event.Status == outbox.EventStatusPending {
			if tenantID == "" || event.TenantID == tenantID {
				scheduled = append(scheduled, event)
				if len(scheduled) >= limit {
					break
				}
			}
		}
	}
	return scheduled, nil
}

// CountPendingEvents 统计待发布事件数量
func (m *MockRepository) CountPendingEvents(ctx context.Context, tenantID string) (int64, error) {
	count := int64(0)
	for _, event := range m.events {
		if event.Status == outbox.EventStatusPending {
			if tenantID == "" || event.TenantID == tenantID {
				count++
			}
		}
	}
	return count, nil
}

// CountFailedEvents 统计失败事件数量
func (m *MockRepository) CountFailedEvents(ctx context.Context, tenantID string) (int64, error) {
	count := int64(0)
	for _, event := range m.events {
		if event.Status == outbox.EventStatusFailed {
			if tenantID == "" || event.TenantID == tenantID {
				count++
			}
		}
	}
	return count, nil
}

// Count 统计事件数量
func (m *MockRepository) Count(ctx context.Context, status outbox.EventStatus, tenantID string) (int64, error) {
	count := int64(0)
	for _, event := range m.events {
		statusMatch := status == "" || event.Status == status
		tenantMatch := tenantID == "" || event.TenantID == tenantID
		if statusMatch && tenantMatch {
			count++
		}
	}
	return count, nil
}

// CountByStatus 按状态统计事件数量
func (m *MockRepository) CountByStatus(ctx context.Context, tenantID string) (map[outbox.EventStatus]int64, error) {
	counts := make(map[outbox.EventStatus]int64)
	for _, event := range m.events {
		if tenantID == "" || event.TenantID == tenantID {
			counts[event.Status]++
		}
	}
	return counts, nil
}

// DeletePublishedBefore 删除指定时间之前已发布的事件
func (m *MockRepository) DeletePublishedBefore(ctx context.Context, before time.Time, tenantID string) (int64, error) {
	deleted := int64(0)
	for id, event := range m.events {
		if event.Status == outbox.EventStatusPublished && event.PublishedAt != nil && event.PublishedAt.Before(before) {
			if tenantID == "" || event.TenantID == tenantID {
				delete(m.events, id)
				deleted++
			}
		}
	}
	return deleted, nil
}

// DeleteFailedBefore 删除指定时间之前失败的事件
func (m *MockRepository) DeleteFailedBefore(ctx context.Context, before time.Time, tenantID string) (int64, error) {
	deleted := int64(0)
	for id, event := range m.events {
		if event.Status == outbox.EventStatusFailed && event.CreatedAt.Before(before) {
			if tenantID == "" || event.TenantID == tenantID {
				delete(m.events, id)
				deleted++
			}
		}
	}
	return deleted, nil
}

// FindByAggregateType 根据聚合类型查找待发布事件
func (m *MockRepository) FindByAggregateType(ctx context.Context, aggregateType string, limit int) ([]*outbox.OutboxEvent, error) {
	var result []*outbox.OutboxEvent
	for _, event := range m.events {
		if event.AggregateType == aggregateType && event.Status == outbox.EventStatusPending {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

// FindEventsForRetry 查找需要重试的事件
func (m *MockRepository) FindEventsForRetry(ctx context.Context, maxRetries int, limit int) ([]*outbox.OutboxEvent, error) {
	var result []*outbox.OutboxEvent
	for _, event := range m.events {
		if event.Status == outbox.EventStatusFailed && event.RetryCount < maxRetries {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

// SetSaveError 设置保存错误
func (m *MockRepository) SetSaveError(err error) {
	m.saveError = err
}

// SetUpdateError 设置更新错误
func (m *MockRepository) SetUpdateError(err error) {
	m.updateError = err
}

// SetFindPendingError 设置查找待发布事件错误
func (m *MockRepository) SetFindPendingError(err error) {
	m.findPendingError = err
}

// GetEventCount 获取事件数量
func (m *MockRepository) GetEventCount() int {
	return len(m.events)
}

// GetEventByID 获取事件
func (m *MockRepository) GetEventByID(id string) *outbox.OutboxEvent {
	return m.events[id]
}

// Clear 清空所有事件
func (m *MockRepository) Clear() {
	m.events = make(map[string]*outbox.OutboxEvent)
	m.idempotencyKeyIndex = make(map[string]*outbox.OutboxEvent)
}

// MockEventPublisher 模拟事件发布器（旧版，已废弃，使用 MockAsyncEventPublisher）
type MockEventPublisher struct {
	publishedData   [][]byte
	publishedTopics []string
	publishError    error
	publishDelay    time.Duration
	resultChan      chan *outbox.PublishResult
}

// NewMockEventPublisher 创建模拟事件发布器
func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		publishedData:   make([][]byte, 0),
		publishedTopics: make([]string, 0),
		resultChan:      make(chan *outbox.PublishResult, 100),
	}
}

// Publish 发布事件（符合 EventPublisher 接口）
func (m *MockEventPublisher) Publish(ctx context.Context, topic string, data []byte) error {
	if m.publishDelay > 0 {
		time.Sleep(m.publishDelay)
	}

	if m.publishError != nil {
		return m.publishError
	}

	m.publishedData = append(m.publishedData, data)
	m.publishedTopics = append(m.publishedTopics, topic)
	return nil
}

// PublishEnvelope 发布 Envelope（符合 EventPublisher 接口）
func (m *MockEventPublisher) PublishEnvelope(ctx context.Context, topic string, envelope *outbox.Envelope) error {
	// 简单实现：将 Envelope 序列化为 JSON
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return m.Publish(ctx, topic, data)
}

// GetPublishResultChannel 获取发布结果通道（符合 EventPublisher 接口）
func (m *MockEventPublisher) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return m.resultChan
}

// SetPublishError 设置发布错误
func (m *MockEventPublisher) SetPublishError(err error) {
	m.publishError = err
}

// SetPublishDelay 设置发布延迟
func (m *MockEventPublisher) SetPublishDelay(delay time.Duration) {
	m.publishDelay = delay
}

// GetPublishedData 获取已发布数据
func (m *MockEventPublisher) GetPublishedData() [][]byte {
	return m.publishedData
}

// GetPublishedTopics 获取已发布主题
func (m *MockEventPublisher) GetPublishedTopics() []string {
	return m.publishedTopics
}

// GetPublishedCount 获取已发布事件数量
func (m *MockEventPublisher) GetPublishedCount() int {
	return len(m.publishedData)
}

// Clear 清空已发布事件
func (m *MockEventPublisher) Clear() {
	m.publishedData = make([][]byte, 0)
	m.publishedTopics = make([]string, 0)
}

// MockTopicMapper 模拟 Topic 映射器
type MockTopicMapper struct {
	topicMap map[string]string
}

// NewMockTopicMapper 创建模拟 Topic 映射器
func NewMockTopicMapper() *MockTopicMapper {
	return &MockTopicMapper{
		topicMap: make(map[string]string),
	}
}

// GetTopic 获取 Topic（符合 TopicMapper 接口）
func (m *MockTopicMapper) GetTopic(aggregateType string) string {
	if topic, exists := m.topicMap[aggregateType]; exists {
		return topic
	}
	return aggregateType + "-events"
}

// SetTopicMapping 设置 Topic 映射
func (m *MockTopicMapper) SetTopicMapping(aggregateType, topic string) {
	m.topicMap[aggregateType] = topic
}

// GetDefaultPublisherConfig 获取默认发布器配置
func GetDefaultPublisherConfig() *outbox.PublisherConfig {
	return &outbox.PublisherConfig{
		MaxRetries:     3,
		RetryDelay:     5 * time.Second,
		PublishTimeout: 30 * time.Second,
		EnableMetrics:  false,
	}
}

// MockAsyncEventPublisher 模拟异步事件发布器（支持 PublishEnvelope 和 ACK）
type MockAsyncEventPublisher struct {
	publishedEnvelopes []*outbox.Envelope
	publishedTopics    []string
	resultChan         chan *outbox.PublishResult
	publishError       error
	publishDelay       time.Duration
	mu                 sync.Mutex
}

// NewMockAsyncEventPublisher 创建模拟异步事件发布器
func NewMockAsyncEventPublisher() *MockAsyncEventPublisher {
	return &MockAsyncEventPublisher{
		publishedEnvelopes: make([]*outbox.Envelope, 0),
		publishedTopics:    make([]string, 0),
		resultChan:         make(chan *outbox.PublishResult, 100),
	}
}

// PublishEnvelope 发布 Envelope（符合 EventPublisher 接口）
func (m *MockAsyncEventPublisher) PublishEnvelope(ctx context.Context, topic string, envelope *outbox.Envelope) error {
	if m.publishDelay > 0 {
		time.Sleep(m.publishDelay)
	}

	if m.publishError != nil {
		return m.publishError
	}

	m.mu.Lock()
	m.publishedEnvelopes = append(m.publishedEnvelopes, envelope)
	m.publishedTopics = append(m.publishedTopics, topic)
	m.mu.Unlock()

	return nil
}

// GetPublishResultChannel 获取发布结果通道（符合 EventPublisher 接口）
func (m *MockAsyncEventPublisher) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return m.resultChan
}

// SendACKSuccess 发送 ACK 成功结果
func (m *MockAsyncEventPublisher) SendACKSuccess(eventID, topic, aggregateID, eventType string) {
	result := &outbox.PublishResult{
		EventID:     eventID,
		Topic:       topic,
		Success:     true,
		Timestamp:   time.Now(),
		AggregateID: aggregateID,
		EventType:   eventType,
	}

	select {
	case m.resultChan <- result:
		// 成功发送
	default:
		// 通道满，丢弃（测试中不应该发生）
	}
}

// SendACKFailure 发送 ACK 失败结果
func (m *MockAsyncEventPublisher) SendACKFailure(eventID, topic, aggregateID, eventType string, err error) {
	result := &outbox.PublishResult{
		EventID:     eventID,
		Topic:       topic,
		Success:     false,
		Error:       err,
		Timestamp:   time.Now(),
		AggregateID: aggregateID,
		EventType:   eventType,
	}

	select {
	case m.resultChan <- result:
		// 成功发送
	default:
		// 通道满，丢弃（测试中不应该发生）
	}
}

// SetPublishError 设置发布错误
func (m *MockAsyncEventPublisher) SetPublishError(err error) {
	m.publishError = err
}

// SetPublishDelay 设置发布延迟
func (m *MockAsyncEventPublisher) SetPublishDelay(delay time.Duration) {
	m.publishDelay = delay
}

// GetPublishedEnvelopes 获取已发布的 Envelope
func (m *MockAsyncEventPublisher) GetPublishedEnvelopes() []*outbox.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedEnvelopes
}

// GetPublishedTopics 获取已发布主题
func (m *MockAsyncEventPublisher) GetPublishedTopics() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedTopics
}

// GetPublishedEnvelopeCount 获取已发布 Envelope 数量
func (m *MockAsyncEventPublisher) GetPublishedEnvelopeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishedEnvelopes)
}

// Clear 清空已发布事件
func (m *MockAsyncEventPublisher) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEnvelopes = make([]*outbox.Envelope, 0)
	m.publishedTopics = make([]string, 0)
}

// Close 关闭发布器
func (m *MockAsyncEventPublisher) Close() {
	close(m.resultChan)
}

// ========== MockEventBusForAdapter ==========

// MockEventBusForAdapter 用于测试 EventBus 适配器的 Mock EventBus
// 实现完整的 eventbus.EventBus 接口
type MockEventBusForAdapter struct {
	publishedEnvelopes []*eventbus.Envelope
	publishedTopics    []string
	resultChan         chan *eventbus.PublishResult
	publishError       error
	mu                 sync.Mutex
}

// NewMockEventBusForAdapter 创建用于适配器测试的 Mock EventBus
func NewMockEventBusForAdapter() *MockEventBusForAdapter {
	return &MockEventBusForAdapter{
		publishedEnvelopes: make([]*eventbus.Envelope, 0),
		publishedTopics:    make([]string, 0),
		resultChan:         make(chan *eventbus.PublishResult, 100),
	}
}

// PublishEnvelope 发布 Envelope（实现 eventbus.EventBus 接口）
func (m *MockEventBusForAdapter) PublishEnvelope(ctx context.Context, topic string, envelope *eventbus.Envelope) error {
	if m.publishError != nil {
		return m.publishError
	}

	m.mu.Lock()
	m.publishedEnvelopes = append(m.publishedEnvelopes, envelope)
	m.publishedTopics = append(m.publishedTopics, topic)
	m.mu.Unlock()

	return nil
}

// GetPublishResultChannel 获取发布结果通道（实现 eventbus.EventBus 接口）
func (m *MockEventBusForAdapter) GetPublishResultChannel() <-chan *eventbus.PublishResult {
	return m.resultChan
}

// SendACKSuccess 发送 ACK 成功结果
func (m *MockEventBusForAdapter) SendACKSuccess(eventID, topic, aggregateID, eventType string) {
	result := &eventbus.PublishResult{
		EventID:     eventID,
		Topic:       topic,
		Success:     true,
		Timestamp:   time.Now(),
		AggregateID: aggregateID,
		EventType:   eventType,
	}

	select {
	case m.resultChan <- result:
		// 成功发送
	default:
		// 通道满，丢弃
	}
}

// SendACKFailure 发送 ACK 失败结果
func (m *MockEventBusForAdapter) SendACKFailure(eventID, topic, aggregateID, eventType string, err error) {
	result := &eventbus.PublishResult{
		EventID:     eventID,
		Topic:       topic,
		Success:     false,
		Error:       err,
		Timestamp:   time.Now(),
		AggregateID: aggregateID,
		EventType:   eventType,
	}

	select {
	case m.resultChan <- result:
		// 成功发送
	default:
		// 通道满，丢弃
	}
}

// GetPublishedEnvelopes 获取已发布的 Envelope
func (m *MockEventBusForAdapter) GetPublishedEnvelopes() []*eventbus.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedEnvelopes
}

// GetPublishedEnvelopeCount 获取已发布 Envelope 数量
func (m *MockEventBusForAdapter) GetPublishedEnvelopeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishedEnvelopes)
}

// SetPublishError 设置发布错误
func (m *MockEventBusForAdapter) SetPublishError(err error) {
	m.publishError = err
}

// 实现 eventbus.EventBus 接口的其他方法（空实现）
func (m *MockEventBusForAdapter) Publish(ctx context.Context, topic string, message []byte) error {
	return nil
}
func (m *MockEventBusForAdapter) Subscribe(ctx context.Context, topic string, handler eventbus.MessageHandler) error {
	return nil
}
func (m *MockEventBusForAdapter) Close() error { return nil }
func (m *MockEventBusForAdapter) RegisterReconnectCallback(callback eventbus.ReconnectCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) Start(ctx context.Context) error { return nil }
func (m *MockEventBusForAdapter) Stop() error                     { return nil }
func (m *MockEventBusForAdapter) PublishWithOptions(ctx context.Context, topic string, message []byte, opts eventbus.PublishOptions) error {
	return nil
}
func (m *MockEventBusForAdapter) SetMessageFormatter(formatter eventbus.MessageFormatter) error {
	return nil
}
func (m *MockEventBusForAdapter) RegisterPublishCallback(callback eventbus.PublishCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) SubscribeWithOptions(ctx context.Context, topic string, handler eventbus.MessageHandler, opts eventbus.SubscribeOptions) error {
	return nil
}
func (m *MockEventBusForAdapter) RegisterSubscriberBacklogCallback(callback eventbus.BacklogStateCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) StartSubscriberBacklogMonitoring(ctx context.Context) error {
	return nil
}
func (m *MockEventBusForAdapter) StopSubscriberBacklogMonitoring() error { return nil }
func (m *MockEventBusForAdapter) RegisterPublisherBacklogCallback(callback eventbus.PublisherBacklogCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) StartPublisherBacklogMonitoring(ctx context.Context) error {
	return nil
}
func (m *MockEventBusForAdapter) StopPublisherBacklogMonitoring() error { return nil }
func (m *MockEventBusForAdapter) StartAllBacklogMonitoring(ctx context.Context) error {
	return nil
}
func (m *MockEventBusForAdapter) StopAllBacklogMonitoring() error { return nil }
func (m *MockEventBusForAdapter) SetMessageRouter(router eventbus.MessageRouter) error {
	return nil
}
func (m *MockEventBusForAdapter) SetErrorHandler(handler eventbus.ErrorHandler) error {
	return nil
}
func (m *MockEventBusForAdapter) RegisterSubscriptionCallback(callback eventbus.SubscriptionCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) StartHealthCheckPublisher(ctx context.Context) error {
	return nil
}
func (m *MockEventBusForAdapter) StopHealthCheckPublisher() error { return nil }
func (m *MockEventBusForAdapter) GetHealthCheckPublisherStatus() eventbus.HealthCheckStatus {
	return eventbus.HealthCheckStatus{}
}
func (m *MockEventBusForAdapter) RegisterHealthCheckPublisherCallback(callback eventbus.HealthCheckCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) StartHealthCheckSubscriber(ctx context.Context) error {
	return nil
}
func (m *MockEventBusForAdapter) StopHealthCheckSubscriber() error { return nil }
func (m *MockEventBusForAdapter) GetHealthCheckSubscriberStats() eventbus.HealthCheckSubscriberStats {
	return eventbus.HealthCheckSubscriberStats{}
}
func (m *MockEventBusForAdapter) RegisterHealthCheckSubscriberCallback(callback eventbus.HealthCheckAlertCallback) error {
	return nil
}
func (m *MockEventBusForAdapter) StartAllHealthCheck(ctx context.Context) error { return nil }
func (m *MockEventBusForAdapter) StopAllHealthCheck() error                     { return nil }
func (m *MockEventBusForAdapter) StartHealthCheck(ctx context.Context) error    { return nil }
func (m *MockEventBusForAdapter) StopHealthCheck() error                        { return nil }
func (m *MockEventBusForAdapter) GetHealthStatus() eventbus.HealthCheckStatus {
	return eventbus.HealthCheckStatus{}
}
func (m *MockEventBusForAdapter) GetConnectionState() eventbus.ConnectionState {
	return eventbus.ConnectionState{}
}
func (m *MockEventBusForAdapter) GetMetrics() eventbus.Metrics {
	return eventbus.Metrics{}
}
func (m *MockEventBusForAdapter) ConfigureTopic(ctx context.Context, topic string, options eventbus.TopicOptions) error {
	return nil
}
func (m *MockEventBusForAdapter) SetTopicPersistence(ctx context.Context, topic string, persistent bool) error {
	return nil
}
func (m *MockEventBusForAdapter) GetTopicConfig(topic string) (eventbus.TopicOptions, error) {
	return eventbus.TopicOptions{}, nil
}
func (m *MockEventBusForAdapter) ListConfiguredTopics() []string {
	return nil
}
func (m *MockEventBusForAdapter) RemoveTopicConfig(topic string) error {
	return nil
}
func (m *MockEventBusForAdapter) SetTopicConfigStrategy(strategy eventbus.TopicConfigStrategy) {}
func (m *MockEventBusForAdapter) GetTopicConfigStrategy() eventbus.TopicConfigStrategy {
	return eventbus.StrategySkip
}
func (m *MockEventBusForAdapter) SubscribeEnvelope(ctx context.Context, topic string, handler eventbus.EnvelopeHandler) error {
	return nil
}
