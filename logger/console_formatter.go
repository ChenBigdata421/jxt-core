package logger

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ANSI 颜色常量（与 GORM 一致）
// 设计原则：深色背景优化，清晰可读，专业简洁
const (
	// 控制符
	Reset = "\033[0m"
	
	// 基础颜色（用于日志级别、路径等）
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"        // Caller 路径、GORM 路径
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	Gray        = "\033[90m"        // 次要信息
	BrightWhite = "\033[97m"       // DEBUG 级别
	
	// 加粗颜色（用于高优先级信息）
	BlueBold    = "\033[34;1m"     // GORM rows 计数
	MagentaBold = "\033[35;1m"
	RedBold     = "\033[31;1m"     // FATAL/PANIC
	YellowBold  = "\033[33;1m"
	GreenBold   = "\033[32;1m"
	CyanBold    = "\033[36;1m"
)

// ConsoleFormatter 自定义控制台格式化器 - 简洁优雅（GORM 风格）
type ConsoleFormatter struct {
	TimestampFormat string
	ForceColors     bool
}

// Format 实现 logrus.Formatter 接口（GORM 风格）
// 性能优化：复用 entry.Buffer，避免频繁分配内存
func (f *ConsoleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer // 复用 Buffer（高性能场景）
	} else {
		b = &bytes.Buffer{}
	}

	// 时间戳格式
	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = "2006-01-02 15:04:05.000"
	}

	// 日志级别颜色（GORM 风格）
	levelColor := getLevelColor(entry.Level)
	
	// 1. 时间戳（白色 - 与 GORM 一致）
	fmt.Fprintf(b, "%s ", entry.Time.Format(timestampFormat))
	
	// 2. 日志级别（带颜色，不加粗）
	levelText := strings.ToUpper(entry.Level.String())
	if len(levelText) < 5 {
		levelText = levelText + strings.Repeat(" ", 5-len(levelText))
	}
	fmt.Fprintf(b, "%s%-5s%s ", levelColor, levelText, Reset)
	
	// 检测是否为 GORM 日志（消息以文件路径开头）
	isGormLog := strings.Contains(entry.Message, "/") && 
		(strings.Contains(entry.Message, ".go:") || strings.Contains(entry.Message, "[") && strings.Contains(entry.Message, "ms]"))
	
	// 3. Caller 和请求 ID（仅非 GORM 日志显示）
	if !isGormLog {
		if caller, ok := entry.Data["caller"].(string); ok && caller != "" {
			// Caller 路径（蓝色 - 与 GORM 一致，可点击）
			fmt.Fprintf(b, "%s%s%s ", Blue, caller, Reset)
			
			// 请求 ID（如果存在，跟在 caller 后面，黄色）
			if requestID, ok := entry.Data["x-request-id"].(string); ok && requestID != "" {
				// 截取请求 ID 前 8 位
				shortID := requestID
				if len(requestID) > 8 {
					shortID = requestID[:8]
				}
				fmt.Fprintf(b, "%sreq:%s%s ", Yellow, shortID, Reset)
			}
		}
	}
	
	// 4. 主要消息
	if isGormLog {
		// GORM 日志：直接输出，不缩进（已经带路径）
		fmt.Fprintf(b, "%s", formatGormLog(entry.Message))
	} else {
		// 普通日志：直接输出
		fmt.Fprintf(b, "%s", entry.Message)
	}
	
	// 5. 关键字段（GORM 风格 - 黄色时间 + 蓝色加粗 rows）
	var keyFields []string
	// 如果消息中已包含 method 和 path（如 "GET /api/v1/sys-api"），则跳过这两个字段避免重复
	skipMethodPath := strings.Contains(entry.Message, " /")
	fieldOrder := []string{"status", "latency", "method", "path", "ip", "user_id", "user_name"}
	
	for _, key := range fieldOrder {
		// 跳过已在消息中显示的 method 和 path
		if skipMethodPath && (key == "method" || key == "path") {
			continue
		}
		if val, ok := entry.Data[key]; ok {
			keyFields = append(keyFields, formatField(key, val))
		}
	}
	
	if len(keyFields) > 0 {
		// 关键字段用空格分隔
		fmt.Fprintf(b, " %s", strings.Join(keyFields, " "))
	}
	
	// 6. 其他字段（次要，放在新行，灰色）
	var otherFields []string
	for key, val := range entry.Data {
		// 跳过已处理的字段和内置字段
		if isKeyField(key) || key == "logger" || key == "caller" || key == "x-request-id" {
			continue
		}
		otherFields = append(otherFields, fmt.Sprintf("%s%s=%s%v", Gray, key, Reset, val))
	}
	
	if len(otherFields) > 0 {
		sort.Strings(otherFields) // 字母排序
		fmt.Fprintf(b, "\n      %s", strings.Join(otherFields, " "))
	}
	
	b.WriteByte('\n')
	return b.Bytes(), nil
}

// getLevelColor 获取日志级别对应的颜色（深色背景优化）
func getLevelColor(level logrus.Level) string {
	switch level {
	case logrus.DebugLevel:
		return BrightWhite // 亮白色 - 深色背景清晰
	case logrus.TraceLevel:
		return Gray // 灰色 - 最低优先级
	case logrus.InfoLevel:
		return Green // 绿色 - 正常状态
	case logrus.WarnLevel:
		return Yellow // 黄色 - 潜在问题
	case logrus.ErrorLevel:
		return Red // 红色 - 错误
	case logrus.FatalLevel, logrus.PanicLevel:
		return RedBold // 红色加粗 - 致命错误
	default:
		return Reset
	}
}

// formatField 格式化字段（GORM 风格）
func formatField(key string, val interface{}) string {
	switch key {
	case "status":
		status := fmt.Sprintf("%v", val)
		// 状态码着色（2xx绿/4xx黄/5xx红）
		if strings.HasPrefix(status, "2") {
			return fmt.Sprintf("%s%s%s", Green, status, Reset)
		} else if strings.HasPrefix(status, "4") {
			return fmt.Sprintf("%s%s%s", Yellow, status, Reset)
		} else if strings.HasPrefix(status, "5") {
			return fmt.Sprintf("%s%s%s", Red, status, Reset)
		}
		return status
	case "latency":
		// 耗时格式化（黄色 - GORM 风格）
		if duration, ok := val.(time.Duration); ok {
			return formatDuration(duration)
		}
		return fmt.Sprintf("%s[%v]%s", Yellow, val, Reset)
	case "method":
		// HTTP 方法（不着色，保持简洁）
		return fmt.Sprintf("%v", val)
	case "path":
		// 路径（不着色）
		return fmt.Sprintf("%v", val)
	case "ip":
		// IP 地址（灰色）
		return fmt.Sprintf("%sip:%v%s", Gray, val, Reset)
	case "user_id", "user_name":
		// 用户信息（灰色）
		return fmt.Sprintf("%s%s:%v%s", Gray, key, val, Reset)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// formatDuration 格式化耗时（GORM 风格 - 黄色）
// 性能分级：< 1s 黄色（正常），>= 1s 红色（警示）
func formatDuration(d time.Duration) string {
	ms := float64(d.Nanoseconds()) / 1e6
	if ms < 1 {
		return fmt.Sprintf("%s[%.3fms]%s", Yellow, ms, Reset)
	} else if ms < 1000 {
		return fmt.Sprintf("%s[%.3fms]%s", Yellow, ms, Reset)
	} else {
		// 超过 1 秒用红色警示（不加粗）
		return fmt.Sprintf("%s[%.3fs]%s", Red, ms/1000, Reset)
	}
}

// isKeyField 判断是否为关键字段
func isKeyField(key string) bool {
	keyFields := map[string]bool{
		"status":    true,
		"latency":   true,
		"method":    true,
		"path":      true,
		"ip":        true,
		"user_id":   true,
		"user_name": true,
	}
	return keyFields[key]
}

// formatGormLog 格式化 GORM 日志（彩色分段）
func formatGormLog(msg string) string {
	// 先清除 GORM 自带的 ANSI 转义码
	msg = stripAnsiCodes(msg)
	
	// GORM 格式：/path/file.go:123 [12.5ms] [rows:10] SELECT ...
	// 优化为：路径(蓝色) [耗时](黄色) [rows](蓝色) SQL(白色)
	
	// 1. 提取文件路径（到 .go:行号）
	pathEnd := strings.Index(msg, ".go:")
	if pathEnd == -1 {
		return msg // 不是标准 GORM 格式，直接返回
	}
	
	// 找到行号结束位置
	lineEnd := pathEnd + 4 // ".go:"
	for lineEnd < len(msg) && msg[lineEnd] >= '0' && msg[lineEnd] <= '9' {
		lineEnd++
	}
	
	path := msg[:lineEnd]
	rest := msg[lineEnd:]
	
	// 2. 提取 [耗时]
	var latency string
	if idx := strings.Index(rest, "["); idx != -1 {
		if endIdx := strings.Index(rest[idx:], "]"); endIdx != -1 {
			latency = rest[idx : idx+endIdx+1]
			rest = rest[idx+endIdx+1:]
		}
	}
	
	// 3. 提取 [rows:N]
	var rows string
	if idx := strings.Index(rest, "[rows:"); idx != -1 {
		if endIdx := strings.Index(rest[idx:], "]"); endIdx != -1 {
			rows = rest[idx : idx+endIdx+1]
			rest = rest[idx+endIdx+1:]
		}
	}
	
	// 4. 组装彩色输出
	var result strings.Builder
	result.WriteString(Blue) // 与 GORM caller 颜色一致
	result.WriteString(path)
	result.WriteString(Reset)
	
	if latency != "" {
		result.WriteString(" ")
		result.WriteString(Yellow)
		result.WriteString(latency)
		result.WriteString(Reset)
	}
	
	if rows != "" {
		result.WriteString(" ")
		result.WriteString(BlueBold)
		result.WriteString(rows)
		result.WriteString(Reset)
	}
	
	if rest != "" {
		result.WriteString(rest) // SQL 语句保持原色
	}
	
	return result.String()
}

// stripAnsiCodes 移除字符串中的 ANSI 转义码
func stripAnsiCodes(s string) string {
	// 匹配 ANSI 转义序列：ESC [ ... m
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i < len(s)-1 && s[i] == '\033' && s[i+1] == '[' {
			// 找到 'm' 结束符
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1 // 跳过整个转义序列
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}
