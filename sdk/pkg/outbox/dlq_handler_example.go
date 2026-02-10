package outbox

import (
	"context"
	"fmt"
	"log"
)

// LoggingDLQHandler 日志记录 DLQ 处理器（示例实现）
// 将死信事件记录到日志中
type LoggingDLQHandler struct {
	// 可以注入自定义的 logger（如 zap.Logger）
	logger interface {
		Error(msg string, fields ...interface{})
	}
}

// NewLoggingDLQHandler 创建日志记录 DLQ 处理器
func NewLoggingDLQHandler() *LoggingDLQHandler {
	return &LoggingDLQHandler{}
}

// Handle 处理死信事件
func (h *LoggingDLQHandler) Handle(ctx context.Context, event *OutboxEvent) error {
	// 记录详细的错误信息
	log.Printf("[DLQ] Event moved to dead letter queue: ID=%s, Type=%s, AggregateID=%s, RetryCount=%d, Error=%s",
		event.ID,
		event.EventType,
		event.AggregateID,
		event.RetryCount,
		event.LastError,
	)

	// 可以在这里添加更多处理逻辑：
	// 1. 保存到专门的死信队列表
	// 2. 发送到死信队列 Topic
	// 3. 记录到监控系统
	// 4. 触发人工介入流程

	return nil
}

// EmailAlertHandler 邮件告警处理器（示例实现）
// 发送邮件告警
type EmailAlertHandler struct {
	// 邮件服务配置
	smtpHost     string
	smtpPort     int
	from         string
	to           []string
	alertEnabled bool
}

// NewEmailAlertHandler 创建邮件告警处理器
func NewEmailAlertHandler(smtpHost string, smtpPort int, from string, to []string) *EmailAlertHandler {
	return &EmailAlertHandler{
		smtpHost:     smtpHost,
		smtpPort:     smtpPort,
		from:         from,
		to:           to,
		alertEnabled: true,
	}
}

// Alert 发送告警
func (h *EmailAlertHandler) Alert(ctx context.Context, event *OutboxEvent) error {
	if !h.alertEnabled {
		return nil
	}

	// 构造邮件内容
	subject := fmt.Sprintf("[Outbox DLQ Alert] Event Failed: %s", event.EventType)
	body := fmt.Sprintf(`
Outbox Event Failed and Moved to Dead Letter Queue

Event Details:
- Event ID: %s
- Event Type: %s
- Aggregate Type: %s
- Aggregate ID: %s
- Tenant ID: %d
- Retry Count: %d
- Max Retries: %d
- Last Error: %s
- Created At: %s
- Last Retry At: %s

Please investigate and take appropriate action.
`,
		event.ID,
		event.EventType,
		event.AggregateType,
		event.AggregateID,
		event.TenantID,
		event.RetryCount,
		event.MaxRetries,
		event.LastError,
		event.CreatedAt.Format("2006-01-02 15:04:05"),
		func() string {
			if event.LastRetryAt != nil {
				return event.LastRetryAt.Format("2006-01-02 15:04:05")
			}
			return "N/A"
		}(),
	)

	// 这里应该调用实际的邮件发送服务
	// 示例中只是打印日志
	log.Printf("[Email Alert] Subject: %s\nBody: %s", subject, body)

	// 实际实现应该类似：
	// return h.emailService.Send(h.from, h.to, subject, body)

	return nil
}

// CompositeDLQHandler 组合 DLQ 处理器（示例实现）
// 允许同时使用多个 DLQ 处理器
type CompositeDLQHandler struct {
	handlers []DLQHandler
}

// NewCompositeDLQHandler 创建组合 DLQ 处理器
func NewCompositeDLQHandler(handlers ...DLQHandler) *CompositeDLQHandler {
	return &CompositeDLQHandler{
		handlers: handlers,
	}
}

// Handle 处理死信事件（依次调用所有处理器）
func (h *CompositeDLQHandler) Handle(ctx context.Context, event *OutboxEvent) error {
	var lastErr error
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, event); err != nil {
			lastErr = err
			// 继续执行其他处理器，不中断
		}
	}
	return lastErr
}

// 使用示例：
//
// // 1. 创建日志处理器
// loggingHandler := outbox.NewLoggingDLQHandler()
//
// // 2. 创建邮件告警处理器
// emailHandler := outbox.NewEmailAlertHandler(
//     "smtp.example.com",
//     587,
//     "noreply@example.com",
//     []string{"admin@example.com"},
// )
//
// // 3. 创建组合处理器
// dlqHandler := outbox.NewCompositeDLQHandler(loggingHandler)
//
// // 4. 配置调度器
// scheduler := outbox.NewScheduler(
//     outbox.WithRepository(repo),
//     outbox.WithEventPublisher(eventPublisher),
//     outbox.WithTopicMapper(topicMapper),
//     outbox.WithSchedulerConfig(&outbox.SchedulerConfig{
//         EnableDLQ:       true,
//         DLQInterval:     5 * time.Minute,
//         DLQHandler:      dlqHandler,
//         DLQAlertHandler: emailHandler,
//     }),
// )

