package log

import "time"

// Option used by the logger
// Option 用于日志记录器
type Option func(*Options)

// Options are logger options
// Options 是日志记录器选项
type Options struct {
	// Name of the log
	// 日志的名称
	Name string
	// Size is the size of ring buffer
	// Size 是环形缓冲区的大小
	Size int
	// Format specifies the output format
	// Format 指定输出格式
	Format FormatFunc
}

// Name of the log
// 设置日志的名称
func Name(n string) Option {
	return func(o *Options) {
		o.Name = n
	}
}

// Size sets the size of the ring buffer
// 设置环形缓冲区的大小
func Size(s int) Option {
	return func(o *Options) {
		o.Size = s
	}
}

// Format sets the output format
// 设置输出格式
func Format(f FormatFunc) Option {
	return func(o *Options) {
		o.Format = f
	}
}

// ReadOptions for querying the logs
// ReadOptions 用于查询日志
type ReadOptions struct {
	// Since what time in past to return the logs
	// 从过去的哪个时间点开始返回日志
	Since time.Time
	// Count specifies number of logs to return
	// Count 指定要返回的日志数量
	Count int
	// Stream requests continuous log stream
	// Stream 请求连续的日志流
	Stream bool
}

// ReadOption used for reading the logs
// ReadOption 用于读取日志
type ReadOption func(*ReadOptions)

// Since sets the time since which to return the log records
// 设置从哪个时间点开始返回日志记录
func Since(s time.Time) ReadOption {
	return func(o *ReadOptions) {
		o.Since = s
	}
}

// Count sets the number of log records to return
// 设置要返回的日志记录数量
func Count(c int) ReadOption {
	return func(o *ReadOptions) {
		o.Count = c
	}
}
