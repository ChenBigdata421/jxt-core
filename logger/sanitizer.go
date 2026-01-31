package logger

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SanitizerConfig 脱敏配置
type SanitizerConfig struct {
	// Enabled 是否启用脱敏
	Enabled bool
	// Rules 脱敏规则列表
	Rules []SanitizerRule
	// CustomMatcher 自定义字段匹配函数（可选）
	CustomMatcher func(fieldName string) *SanitizerRule
}

// SanitizerRule 脱敏规则
type SanitizerRule struct {
	// FieldPattern 字段名匹配（精确匹配或后缀匹配）
	// 例如："phone", "idcard", "*_token"（*表示后缀匹配）
	FieldPattern string
	// Strategy 脱敏策略
	// - "mask": 掩码（保留前后，中间替换为 *）
	// - "hash": SHA256 哈希
	// - "remove": 完全删除字段
	Strategy string
	// MaskChar 掩码字符（默认 "*"）
	MaskChar string
	// KeepPrefix 保留前缀长度（mask 策略）
	KeepPrefix int
	// KeepSuffix 保留后缀长度（mask 策略）
	KeepSuffix int
}

// DefaultSanitizerRules 默认脱敏规则（生产环境推荐）
var DefaultSanitizerRules = []SanitizerRule{
	// 手机号：138****5678
	{
		FieldPattern: "phone",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   3,
		KeepSuffix:   4,
	},
	// 身份证：110***********1234
	{
		FieldPattern: "idcard",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   3,
		KeepSuffix:   4,
	},
	{
		FieldPattern: "id_card",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   3,
		KeepSuffix:   4,
	},
	// 密码：完全删除
	{
		FieldPattern: "password",
		Strategy:     "remove",
	},
	{
		FieldPattern: "passwd",
		Strategy:     "remove",
	},
	// Token：哈希
	{
		FieldPattern: "token",
		Strategy:     "hash",
	},
	{
		FieldPattern: "*_token", // 后缀匹配（access_token, refresh_token）
		Strategy:     "hash",
	},
	// API Key：哈希
	{
		FieldPattern: "api_key",
		Strategy:     "hash",
	},
	{
		FieldPattern: "apikey",
		Strategy:     "hash",
	},
	// 邮箱：部分掩码（zhang***@example.com）
	{
		FieldPattern: "email",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   4,
		KeepSuffix:   12, // 保留 @example.com
	},
}

// DefaultSanitizerConfig 默认脱敏配置
var DefaultSanitizerConfig = SanitizerConfig{
	Enabled: true,
	Rules:   DefaultSanitizerRules,
}

// sanitizerLogger 脱敏日志（自动过滤敏感字段）
type sanitizerLogger struct {
	logger  Logger
	config  SanitizerConfig
	matcher map[string]*SanitizerRule // 快速查找表（避免循环匹配）
}

// NewSanitizerLogger 创建脱敏 logger
func NewSanitizerLogger(logger Logger, config SanitizerConfig) Logger {
	if !config.Enabled || len(config.Rules) == 0 {
		// 未启用或无规则，直接返回原 logger
		return logger
	}
	
	// 构建快速查找表
	matcher := make(map[string]*SanitizerRule)
	for i := range config.Rules {
		rule := &config.Rules[i]
		// 应用默认值
		if rule.Strategy == "mask" && rule.MaskChar == "" {
			rule.MaskChar = "*"
		}
		
		// 精确匹配规则直接加入 map
		if !strings.HasPrefix(rule.FieldPattern, "*") {
			matcher[rule.FieldPattern] = rule
		}
	}
	
	return &sanitizerLogger{
		logger:  logger,
		config:  config,
		matcher: matcher,
	}
}

func (s *sanitizerLogger) Init(opts ...Option) error {
	return s.logger.Init(opts...)
}

func (s *sanitizerLogger) Options() Options {
	return s.logger.Options()
}

func (s *sanitizerLogger) Fields(fields map[string]interface{}) Logger {
	// 脱敏字段
	sanitized := s.sanitizeFields(fields)
	
	return &sanitizerLogger{
		logger:  s.logger.Fields(sanitized),
		config:  s.config,
		matcher: s.matcher,
	}
}

func (s *sanitizerLogger) Log(level Level, v ...interface{}) {
	s.logger.Log(level, v...)
}

func (s *sanitizerLogger) Logf(level Level, format string, v ...interface{}) {
	s.logger.Logf(level, format, v...)
}

func (s *sanitizerLogger) String() string {
	return s.logger.String() + "-sanitizer"
}

// sanitizeFields 脱敏字段映射
func (s *sanitizerLogger) sanitizeFields(fields map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return fields
	}
	
	// 复制字段（避免修改原始数据）
	result := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		result[k] = v
	}
	
	// 遍历字段，应用脱敏规则
	for key, value := range result {
		rule := s.matchRule(key)
		if rule == nil {
			continue
		}
		
		// 应用脱敏策略
		result[key] = s.applySanitizer(value, rule)
	}
	
	return result
}

// matchRule 匹配脱敏规则（性能优化：先精确匹配，再模糊匹配）
func (s *sanitizerLogger) matchRule(fieldName string) *SanitizerRule {
	// 1. 精确匹配（O(1)）
	if rule, ok := s.matcher[fieldName]; ok {
		return rule
	}
	
	// 2. 精确匹配（小写）
	lowerName := strings.ToLower(fieldName)
	if rule, ok := s.matcher[lowerName]; ok {
		return rule
	}
	
	// 3. 自定义匹配器
	if s.config.CustomMatcher != nil {
		if rule := s.config.CustomMatcher(fieldName); rule != nil {
			return rule
		}
	}
	
	// 4. 后缀匹配（遍历规则）
	for i := range s.config.Rules {
		rule := &s.config.Rules[i]
		if strings.HasPrefix(rule.FieldPattern, "*") {
			suffix := strings.TrimPrefix(rule.FieldPattern, "*")
			if strings.HasSuffix(lowerName, suffix) {
				return rule
			}
		}
	}
	
	return nil
}

// applySanitizer 应用脱敏策略
func (s *sanitizerLogger) applySanitizer(value interface{}, rule *SanitizerRule) interface{} {
	// 仅处理字符串类型
	str, ok := value.(string)
	if !ok {
		return value
	}
	
	switch rule.Strategy {
	case "mask":
		return s.maskString(str, rule)
	case "hash":
		return s.hashString(str)
	case "remove":
		return "[REDACTED]"
	default:
		return value
	}
}

// maskString 掩码字符串（保留前后，中间替换）
func (s *sanitizerLogger) maskString(str string, rule *SanitizerRule) string {
	length := len(str)
	
	// 字符串太短，不脱敏
	if length <= rule.KeepPrefix+rule.KeepSuffix {
		return strings.Repeat(rule.MaskChar, length)
	}
	
	// 构造掩码字符串
	prefix := str[:rule.KeepPrefix]
	suffix := str[length-rule.KeepSuffix:]
	maskLen := length - rule.KeepPrefix - rule.KeepSuffix
	
	return prefix + strings.Repeat(rule.MaskChar, maskLen) + suffix
}

// hashString 哈希字符串（SHA256）
func (s *sanitizerLogger) hashString(str string) string {
	hash := sha256.Sum256([]byte(str))
	return "sha256:" + hex.EncodeToString(hash[:8]) // 仅保留前 8 字节（16 字符）
}

// 扩展接口透传

func (s *sanitizerLogger) Info(msg string, fields ...Field) {
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Info(msg, fields...)
	}
}

func (s *sanitizerLogger) Debug(msg string, fields ...Field) {
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Debug(msg, fields...)
	}
}

func (s *sanitizerLogger) Warn(msg string, fields ...Field) {
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Warn(msg, fields...)
	}
}

func (s *sanitizerLogger) Error(msg string, fields ...Field) {
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Error(msg, fields...)
	}
}

func (s *sanitizerLogger) WithContext(ctx interface{}) Logger {
	if ext, ok := s.logger.(extendedLogger); ok {
		return &sanitizerLogger{
			logger:  ext.WithContext(ctx),
			config:  s.config,
			matcher: s.matcher,
		}
	}
	return s
}

func (s *sanitizerLogger) With(fields ...Field) Logger {
	if ext, ok := s.logger.(extendedLogger); ok {
		return &sanitizerLogger{
			logger:  ext.With(fields...),
			config:  s.config,
			matcher: s.matcher,
		}
	}
	return s
}

func (s *sanitizerLogger) Sync() error {
	if ext, ok := s.logger.(extendedLogger); ok {
		return ext.Sync()
	}
	return nil
}
