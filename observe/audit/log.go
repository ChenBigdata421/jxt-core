package log

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var (
	// DefaultSize Default buffer size if any 默认缓冲区大小
	DefaultSize = 256
	// DefaultFormat Default formatter 默认格式化函数
	DefaultFormat = TextFormat
)

// Log is debug log interface for reading and writing logs
// Log 是用于读写日志的调试日志接口
type Log interface {
	// Read reads log entries from the logger
	// Read 从日志记录器中读取日志条目
	Read(...ReadOption) ([]Record, error)
	// Write writes records to log
	// Write 将记录写入日志
	Write(Record) error
	// Stream log records
	// Stream 日志记录流
	Stream() (Stream, error)
}

// Record is log record entry
// Record 是日志记录条目
type Record struct {
	// Timestamp of logged event
	// 日志事件的时间戳
	Timestamp time.Time `json:"timestamp"`
	// Metadata to enrich log record
	// 用于丰富日志记录的元数据
	Metadata map[string]string `json:"metadata"`
	// Value contains log entry
	// 包含日志条目的值
	Message interface{} `json:"message"`
}

// Stream returns a log stream
// 返回日志流
type Stream interface {
	Chan() <-chan Record
	Stop() error
}

// FormatFunc is a function which formats the output
// FormatFunc 是格式化输出的函数
type FormatFunc func(Record) string

// TextFormat returns text format
// TextFormat 返回文本格式
func TextFormat(r Record) string {
	var sb strings.Builder
	sb.WriteString(r.Timestamp.Format("2006-01-02 15:04:05"))
	sb.WriteString(" ")
	sb.WriteString(fmt.Sprintf("%v", r.Message))
	sb.WriteString(" ")
	return sb.String()
}

// JSONFormat is a json format func
// JSONFormat 是 JSON 格式化函数
func JSONFormat(r Record) string {
	b, _ := json.Marshal(r)
	return string(b) + " "
}
