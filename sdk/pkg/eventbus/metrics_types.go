package eventbus

import "time"

// ComprehensiveMetrics 全面性能指标
type ComprehensiveMetrics struct {
	// 基本信息
	System    string
	Pressure  string
	Messages  int
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// 消息指标
	Sent        int64
	Received    int64
	Errors      int64
	SuccessRate float64

	// 性能指标
	SendRate       float64 // 发送速率 msg/s
	Throughput     float64 // 吞吐量 msg/s
	FirstLatency   time.Duration
	AverageLatency time.Duration
	P95Latency     time.Duration
	P99Latency     time.Duration

	// 资源占用
	InitialGoroutines int
	PeakGoroutines    int
	GoroutineDelta    int
	InitialMemoryMB   float64
	PeakMemoryMB      float64
	MemoryDeltaMB     float64
	CPUUsagePercent   float64

	// 网络指标
	BytesSent      int64
	BytesReceived  int64
	NetworkLatency time.Duration
}

