package eventbus

import (
	"math"
	"time"
)

// calculateOverallScore 计算综合评分 (总分100)
func calculateOverallScore(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	
	reliability := calculateReliabilityScore(results)    // 25分
	performance := calculatePerformanceScore(results)   // 35分
	efficiency := calculateEfficiencyScore(results)     // 25分
	scalability := calculateScalabilityScore(results)   // 15分
	
	return reliability + performance + efficiency + scalability
}

// calculateReliabilityScore 计算可靠性评分 (满分25)
func calculateReliabilityScore(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	
	var totalSuccessRate float64
	for _, result := range results {
		totalSuccessRate += result.SuccessRate
	}
	
	avgSuccessRate := totalSuccessRate / float64(len(results))
	
	// 成功率评分: 100%=25分, 90%=20分, 80%=15分, 以此类推
	score := avgSuccessRate * 25 / 100
	if score > 25 {
		score = 25
	}
	
	return score
}

// calculatePerformanceScore 计算性能评分 (满分35)
func calculatePerformanceScore(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	
	// 吞吐量评分 (20分)
	var totalThroughput float64
	for _, result := range results {
		totalThroughput += result.Throughput
	}
	avgThroughput := totalThroughput / float64(len(results))
	
	// 基于吞吐量的评分: 50 msg/s = 20分, 按比例计算
	throughputScore := math.Min(avgThroughput/50*20, 20)
	
	// 延迟评分 (15分)
	var totalLatency time.Duration
	validLatencies := 0
	for _, result := range results {
		if result.FirstLatency > 0 {
			totalLatency += result.FirstLatency
			validLatencies++
		}
	}
	
	latencyScore := 0.0
	if validLatencies > 0 {
		avgLatencyMs := float64(totalLatency.Nanoseconds()) / float64(validLatencies) / 1000000
		// 延迟评分: 1ms=15分, 10ms=10分, 100ms=5分, 1000ms=0分
		if avgLatencyMs <= 1 {
			latencyScore = 15
		} else if avgLatencyMs <= 10 {
			latencyScore = 15 - (avgLatencyMs-1)*5/9
		} else if avgLatencyMs <= 100 {
			latencyScore = 10 - (avgLatencyMs-10)*5/90
		} else if avgLatencyMs <= 1000 {
			latencyScore = 5 - (avgLatencyMs-100)*5/900
		}
		if latencyScore < 0 {
			latencyScore = 0
		}
	}
	
	return throughputScore + latencyScore
}

// calculateEfficiencyScore 计算资源效率评分 (满分25)
func calculateEfficiencyScore(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	
	// 内存效率评分 (15分)
	var totalMemoryPerMsg float64
	for _, result := range results {
		if result.Messages > 0 {
			memoryPerMsg := result.MemoryDeltaMB / float64(result.Messages)
			totalMemoryPerMsg += memoryPerMsg
		}
	}
	avgMemoryPerMsg := totalMemoryPerMsg / float64(len(results))
	
	// 内存效率: 0.001MB/msg=15分, 0.01MB/msg=10分, 0.1MB/msg=5分
	memoryScore := 0.0
	if avgMemoryPerMsg <= 0.001 {
		memoryScore = 15
	} else if avgMemoryPerMsg <= 0.01 {
		memoryScore = 15 - (avgMemoryPerMsg-0.001)*5/0.009
	} else if avgMemoryPerMsg <= 0.1 {
		memoryScore = 10 - (avgMemoryPerMsg-0.01)*5/0.09
	} else {
		memoryScore = 5 - math.Min((avgMemoryPerMsg-0.1)*5/0.9, 5)
	}
	if memoryScore < 0 {
		memoryScore = 0
	}
	
	// Goroutine效率评分 (10分)
	var totalGoroutinePerMsg float64
	for _, result := range results {
		if result.Messages > 0 {
			goroutinePerMsg := float64(result.GoroutineDelta) / float64(result.Messages)
			totalGoroutinePerMsg += goroutinePerMsg
		}
	}
	avgGoroutinePerMsg := totalGoroutinePerMsg / float64(len(results))
	
	// Goroutine效率: 0.001/msg=10分, 0.01/msg=7分, 0.1/msg=3分
	goroutineScore := 0.0
	if avgGoroutinePerMsg <= 0.001 {
		goroutineScore = 10
	} else if avgGoroutinePerMsg <= 0.01 {
		goroutineScore = 10 - (avgGoroutinePerMsg-0.001)*3/0.009
	} else if avgGoroutinePerMsg <= 0.1 {
		goroutineScore = 7 - (avgGoroutinePerMsg-0.01)*4/0.09
	} else {
		goroutineScore = 3 - math.Min((avgGoroutinePerMsg-0.1)*3/0.9, 3)
	}
	if goroutineScore < 0 {
		goroutineScore = 0
	}
	
	return memoryScore + goroutineScore
}

// calculateScalabilityScore 计算扩展性评分 (满分15)
func calculateScalabilityScore(results []*ComprehensiveMetrics) float64 {
	if len(results) < 2 {
		return 7.5 // 默认中等分数
	}
	
	// 吞吐量扩展性 (10分)
	lightThroughput := results[0].Throughput
	heavyThroughput := results[len(results)-1].Throughput
	
	throughputScaling := 0.0
	if lightThroughput > 0 {
		scalingFactor := heavyThroughput / lightThroughput
		// 理想扩展: 2x=10分, 1.5x=7分, 1x=5分, 0.5x=2分
		if scalingFactor >= 2.0 {
			throughputScaling = 10
		} else if scalingFactor >= 1.5 {
			throughputScaling = 7 + (scalingFactor-1.5)*3/0.5
		} else if scalingFactor >= 1.0 {
			throughputScaling = 5 + (scalingFactor-1.0)*2/0.5
		} else if scalingFactor >= 0.5 {
			throughputScaling = 2 + (scalingFactor-0.5)*3/0.5
		} else {
			throughputScaling = scalingFactor * 2 / 0.5
		}
	}
	
	// 延迟稳定性 (5分)
	var latencyVariance float64
	var avgLatency float64
	validCount := 0
	
	for _, result := range results {
		if result.FirstLatency > 0 {
			latencyMs := float64(result.FirstLatency.Nanoseconds()) / 1000000
			avgLatency += latencyMs
			validCount++
		}
	}
	
	latencyStability := 0.0
	if validCount > 1 {
		avgLatency /= float64(validCount)
		
		for _, result := range results {
			if result.FirstLatency > 0 {
				latencyMs := float64(result.FirstLatency.Nanoseconds()) / 1000000
				diff := latencyMs - avgLatency
				latencyVariance += diff * diff
			}
		}
		latencyVariance /= float64(validCount)
		
		// 延迟稳定性: 方差越小分数越高
		stdDev := math.Sqrt(latencyVariance)
		if stdDev <= 10 {
			latencyStability = 5
		} else if stdDev <= 50 {
			latencyStability = 5 - (stdDev-10)*3/40
		} else {
			latencyStability = 2 - math.Min((stdDev-50)*2/100, 2)
		}
		if latencyStability < 0 {
			latencyStability = 0
		}
	} else {
		latencyStability = 2.5 // 默认分数
	}
	
	return throughputScaling + latencyStability
}

// 辅助函数：计算平均值和范围
func calculateAverageSuccessRate(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	var total float64
	for _, result := range results {
		total += result.SuccessRate
	}
	return total / float64(len(results))
}

func calculateAverageThroughput(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	var total float64
	for _, result := range results {
		total += result.Throughput
	}
	return total / float64(len(results))
}

func calculateAverageLatency(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	var total time.Duration
	validCount := 0
	for _, result := range results {
		if result.FirstLatency > 0 {
			total += result.FirstLatency
			validCount++
		}
	}
	if validCount == 0 {
		return 0
	}
	return float64(total.Nanoseconds()) / float64(validCount) / 1000000 // ms
}

func getMinThroughput(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	min := results[0].Throughput
	for _, result := range results {
		if result.Throughput < min {
			min = result.Throughput
		}
	}
	return min
}

func getMaxThroughput(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	max := results[0].Throughput
	for _, result := range results {
		if result.Throughput > max {
			max = result.Throughput
		}
	}
	return max
}

func getMinLatency(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	min := float64(results[0].FirstLatency.Nanoseconds()) / 1000000
	for _, result := range results {
		if result.FirstLatency > 0 {
			latency := float64(result.FirstLatency.Nanoseconds()) / 1000000
			if latency < min {
				min = latency
			}
		}
	}
	return min
}

func getMaxLatency(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	max := float64(results[0].FirstLatency.Nanoseconds()) / 1000000
	for _, result := range results {
		if result.FirstLatency > 0 {
			latency := float64(result.FirstLatency.Nanoseconds()) / 1000000
			if latency > max {
				max = latency
			}
		}
	}
	return max
}

func getMinMemory(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	min := results[0].MemoryDeltaMB
	for _, result := range results {
		if result.MemoryDeltaMB < min {
			min = result.MemoryDeltaMB
		}
	}
	return min
}

func getMaxMemory(results []*ComprehensiveMetrics) float64 {
	if len(results) == 0 {
		return 0
	}
	max := results[0].MemoryDeltaMB
	for _, result := range results {
		if result.MemoryDeltaMB > max {
			max = result.MemoryDeltaMB
		}
	}
	return max
}
