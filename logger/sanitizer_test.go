package logger

import (
	"bytes"
	"strings"
	"testing"
)

// TestSanitizerLogger_Basic 测试基础脱敏功能
func TestSanitizerLogger_Basic(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, SanitizerConfig{
		Enabled: true,
		Rules:   DefaultSanitizerRules,
	})
	
	// 记录包含敏感字段的日志
	sanitizer.Fields(map[string]interface{}{
		"phone":    "13812345678",
		"password": "secret123",
		"username": "testuser",
	}).Log(InfoLevel, "user login")
	
	output := buf.String()
	
	// 验证手机号被脱敏
	if strings.Contains(output, "13812345678") {
		t.Error("Phone number should be masked")
	}
	if !strings.Contains(output, "138****5678") {
		t.Errorf("Expected masked phone '138****5678', got: %s", output)
	}
	
	// 验证密码被删除
	if strings.Contains(output, "secret123") {
		t.Error("Password should be removed")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Expected '[REDACTED]' for password")
	}
	
	// 验证普通字段不受影响
	if !strings.Contains(output, "testuser") {
		t.Error("Username should not be sanitized")
	}
}

// TestSanitizerLogger_PhoneMask 测试手机号脱敏
func TestSanitizerLogger_PhoneMask(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	testCases := []struct {
		input    string
		expected string
	}{
		{"13812345678", "138****5678"},
		{"18900001111", "189****1111"},
		{"1234567", "*******"}, // 太短，全部掩码
	}
	
	for _, tc := range testCases {
		buf.Reset()
		
		sanitizer.Fields(map[string]interface{}{
			"phone": tc.input,
		}).Log(InfoLevel, "test")
		
		output := buf.String()
		
		// 验证原始值不存在（除非太短）
		if len(tc.input) > 7 && strings.Contains(output, tc.input) {
			t.Errorf("Original phone '%s' should be masked", tc.input)
		}
		
		// 验证掩码值存在
		if !strings.Contains(output, tc.expected) {
			t.Errorf("Expected masked phone '%s', got: %s", tc.expected, output)
		}
	}
}

// TestSanitizerLogger_IDCardMask 测试身份证脱敏
func TestSanitizerLogger_IDCardMask(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	idcard := "110101199001011234"
	expected := "110***********1234"
	
	sanitizer.Fields(map[string]interface{}{
		"idcard": idcard,
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证原始值不存在
	if strings.Contains(output, idcard) {
		t.Error("Original ID card should be masked")
	}
	
	// 验证掩码值存在
	if !strings.Contains(output, expected) {
		t.Errorf("Expected masked idcard '%s', got: %s", expected, output)
	}
}

// TestSanitizerLogger_TokenHash 测试 Token 哈希
func TestSanitizerLogger_TokenHash(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	
	sanitizer.Fields(map[string]interface{}{
		"token": token,
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证原始 token 不存在
	if strings.Contains(output, token) {
		t.Error("Original token should be hashed")
	}
	
	// 验证哈希前缀存在
	if !strings.Contains(output, "sha256:") {
		t.Error("Expected 'sha256:' prefix for hashed token")
	}
}

// TestSanitizerLogger_PasswordRemove 测试密码删除
func TestSanitizerLogger_PasswordRemove(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	sanitizer.Fields(map[string]interface{}{
		"password": "MySecretPassword123!",
		"passwd":   "AnotherSecret",
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证密码被删除
	if strings.Contains(output, "MySecretPassword123!") {
		t.Error("Password should be removed")
	}
	if strings.Contains(output, "AnotherSecret") {
		t.Error("Passwd should be removed")
	}
	
	// 验证替换为 [REDACTED]
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Expected '[REDACTED]' for passwords")
	}
}

// TestSanitizerLogger_CaseInsensitive 测试大小写不敏感
func TestSanitizerLogger_CaseInsensitive(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	// 测试不同大小写
	testFields := map[string]interface{}{
		"Phone":    "13800001111",
		"PHONE":    "13800002222",
		"PhoneNum": "13800003333", // 不匹配（仅精确匹配 "phone"）
	}
	
	sanitizer.Fields(testFields).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// Phone 和 PHONE 应该被脱敏
	if strings.Contains(output, "13800001111") || strings.Contains(output, "13800002222") {
		t.Error("Phone fields should be masked (case-insensitive)")
	}
	
	// PhoneNum 不应该被脱敏（仅精确匹配）
	if !strings.Contains(output, "13800003333") {
		t.Error("PhoneNum should not be masked (not exact match)")
	}
}

// TestSanitizerLogger_SuffixMatch 测试后缀匹配
func TestSanitizerLogger_SuffixMatch(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	// 测试后缀匹配（*_token）
	sanitizer.Fields(map[string]interface{}{
		"access_token":  "abc123",
		"refresh_token": "def456",
		"my_token":      "ghi789",
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 所有 *_token 字段都应该被哈希
	if strings.Contains(output, "abc123") || strings.Contains(output, "def456") || strings.Contains(output, "ghi789") {
		t.Error("All *_token fields should be hashed")
	}
	
	if !strings.Contains(output, "sha256:") {
		t.Error("Expected 'sha256:' prefix for hashed tokens")
	}
}

// TestSanitizerLogger_CustomRule 测试自定义规则
func TestSanitizerLogger_CustomRule(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 自定义规则：脱敏 credit_card
	customRules := append(DefaultSanitizerRules, SanitizerRule{
		FieldPattern: "credit_card",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   4,
		KeepSuffix:   4,
	})
	
	sanitizer := NewSanitizerLogger(baseLogger, SanitizerConfig{
		Enabled: true,
		Rules:   customRules,
	})
	
	sanitizer.Fields(map[string]interface{}{
		"credit_card": "1234567812345678",
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证信用卡号被脱敏
	if strings.Contains(output, "1234567812345678") {
		t.Error("Credit card should be masked")
	}
	if !strings.Contains(output, "1234********5678") {
		t.Errorf("Expected masked credit card '1234********5678', got: %s", output)
	}
}

// TestSanitizerLogger_NoSanitization 测试禁用脱敏
func TestSanitizerLogger_NoSanitization(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 禁用脱敏
	sanitizer := NewSanitizerLogger(baseLogger, SanitizerConfig{
		Enabled: false,
		Rules:   DefaultSanitizerRules,
	})
	
	phone := "13812345678"
	sanitizer.Fields(map[string]interface{}{
		"phone": phone,
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证原始值未被脱敏
	if !strings.Contains(output, phone) {
		t.Error("Phone should not be sanitized when disabled")
	}
}

// TestSanitizerLogger_MultipleFields 测试多个敏感字段
func TestSanitizerLogger_MultipleFields(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	sanitizer.Fields(map[string]interface{}{
		"username":      "john_doe",
		"phone":         "13812345678",
		"idcard":        "110101199001011234",
		"password":      "MyPassword123",
		"access_token":  "abc123xyz",
		"email":         "john@example.com",
		"order_id":      "ORD-12345",
	}).Log(InfoLevel, "user operation")
	
	output := buf.String()
	
	// 验证普通字段不变
	if !strings.Contains(output, "john_doe") {
		t.Error("Username should not be sanitized")
	}
	if !strings.Contains(output, "ORD-12345") {
		t.Error("Order ID should not be sanitized")
	}
	
	// 验证敏感字段被脱敏
	if strings.Contains(output, "13812345678") {
		t.Error("Phone should be masked")
	}
	if strings.Contains(output, "110101199001011234") {
		t.Error("ID card should be masked")
	}
	if strings.Contains(output, "MyPassword123") {
		t.Error("Password should be removed")
	}
	if strings.Contains(output, "abc123xyz") {
		t.Error("Token should be hashed")
	}
}

// TestSanitizerLogger_FieldsChaining 测试 Fields 链式调用
func TestSanitizerLogger_FieldsChaining(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	// 链式调用 Fields
	sanitizer.
		Fields(map[string]interface{}{"phone": "13800001111"}).
		Fields(map[string]interface{}{"password": "secret"}).
		Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证所有字段都被脱敏
	if strings.Contains(output, "13800001111") {
		t.Error("Phone should be masked in chained Fields()")
	}
	if strings.Contains(output, "secret") {
		t.Error("Password should be removed in chained Fields()")
	}
}

// TestSanitizerLogger_NonStringValue 测试非字符串值
func TestSanitizerLogger_NonStringValue(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	// 包含非字符串值
	sanitizer.Fields(map[string]interface{}{
		"phone":   123456789,   // int（不脱敏）
		"password": true,        // bool（不脱敏）
		"token":    nil,         // nil（不脱敏）
		"username": "testuser",  // string（不匹配规则）
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证非字符串值不处理
	if !strings.Contains(output, "123456789") {
		t.Error("Int value should not be sanitized")
	}
	if !strings.Contains(output, "testuser") {
		t.Error("Username should not be sanitized")
	}
}

// BenchmarkSanitizerLogger 性能测试
func BenchmarkSanitizerLogger(b *testing.B) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	fields := map[string]interface{}{
		"phone":    "13812345678",
		"password": "secret123",
		"username": "testuser",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizer.Fields(fields).Log(InfoLevel, "benchmark")
	}
}

// BenchmarkSanitizerLogger_vs_NoSanitize 对比性能
func BenchmarkSanitizerLogger_vs_NoSanitize(b *testing.B) {
	buf := &bytes.Buffer{}
	
	b.Run("NoSanitize", func(b *testing.B) {
		logger := NewLogrusLogger(
			WithOutput(buf),
			WithLevel(InfoLevel),
		)
		
		fields := map[string]interface{}{
			"phone":    "13812345678",
			"username": "testuser",
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			logger.Fields(fields).Log(InfoLevel, "test")
		}
	})
	
	b.Run("WithSanitize", func(b *testing.B) {
		baseLogger := NewLogrusLogger(
			WithOutput(buf),
			WithLevel(InfoLevel),
		)
		sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
		
		fields := map[string]interface{}{
			"phone":    "13812345678",
			"username": "testuser",
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sanitizer.Fields(fields).Log(InfoLevel, "test")
		}
	})
}

// TestSanitizerLogger_EmailMask 测试邮箱脱敏
func TestSanitizerLogger_EmailMask(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	sanitizer := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	email := "zhang@example.com"
	
	sanitizer.Fields(map[string]interface{}{
		"email": email,
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证原始邮箱不存在
	if strings.Contains(output, email) {
		t.Error("Original email should be masked")
	}
	
	// 验证部分掩码
	if !strings.Contains(output, "zhan***") && !strings.Contains(output, "example.com") {
		t.Errorf("Expected masked email, got: %s", output)
	}
}

// TestSanitizerLogger_CustomMatcher 测试自定义匹配器
func TestSanitizerLogger_CustomMatcher(t *testing.T) {
	buf := &bytes.Buffer{}
	
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 自定义匹配器：所有以 "secret_" 开头的字段都删除
	sanitizer := NewSanitizerLogger(baseLogger, SanitizerConfig{
		Enabled: true,
		Rules:   DefaultSanitizerRules,
		CustomMatcher: func(fieldName string) *SanitizerRule {
			if strings.HasPrefix(strings.ToLower(fieldName), "secret_") {
				return &SanitizerRule{
					FieldPattern: fieldName,
					Strategy:     "remove",
				}
			}
			return nil
		},
	})
	
	sanitizer.Fields(map[string]interface{}{
		"secret_key":   "my_secret",
		"secret_value": "confidential",
		"public_info":  "visible",
	}).Log(InfoLevel, "test")
	
	output := buf.String()
	
	// 验证 secret_ 字段被删除
	if strings.Contains(output, "my_secret") || strings.Contains(output, "confidential") {
		t.Error("Secret fields should be removed")
	}
	
	// 验证普通字段不变
	if !strings.Contains(output, "visible") {
		t.Error("Public info should not be sanitized")
	}
}
