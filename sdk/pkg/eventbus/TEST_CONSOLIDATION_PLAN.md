# EventBus 测试文件整合计划

## 阶段 1: 清理 ✅ 完成

已删除 8 个无意义文件：
- ✅ kafka_vs_nats_battle_test.go
- ✅ nats_disk_vs_kafka_simple_test.go  
- ✅ naming_verification_test.go
- ✅ health_check_debug_test.go
- ✅ nats_routing_debug_test.go
- ✅ optimization_implementation_test.go
- ✅ optimization_verification_test.go
- ✅ eventbus_extended_test.go

## 阶段 2: 整合核心组件

### 2.1 EventBus Manager (13 → 4)

#### 保留文件：
1. ✅ `eventbus_integration_test.go` - 保留不变
2. ✅ `eventbus_types_test.go` - 保留不变
3. ✅ `eventbus_performance_test.go` - 保留不变

#### 整合为 `eventbus_test.go`：
合并以下文件（93 个测试）：
- `eventbus_core_test.go` (15 tests)
- `eventbus_advanced_features_test.go` (19 tests)
- `eventbus_coverage_boost_test.go` (15 tests)
- `eventbus_edge_cases_test.go` (10 tests)
- `eventbus_hotpath_advanced_test.go` (16 tests)
- `eventbus_wrappedhandler_test.go` (10 tests)
- `eventbus_metrics_test.go` (8 tests)
- `eventbus_manager_advanced_test.go` (11 tests) - 需要添加
- `eventbus_health_test.go` (14 tests) - 移到 health_check_test.go
- `eventbus_health_check_coverage_test.go` (17 tests) - 移到 health_check_test.go
- `eventbus_start_all_health_check_test.go` (6 tests) - 移到 health_check_test.go
- `eventbus_topic_config_coverage_test.go` (15 tests) - 移到 topic_config_test.go

### 2.2 Health Check (10 → 3)

#### 整合为 `health_check_test.go`：
合并以下文件：
- `health_checker_advanced_test.go` (14 tests)
- `health_check_subscriber_extended_test.go` (13 tests)
- `health_check_message_coverage_test.go` (16 tests)
- `health_check_message_extended_test.go` (13 tests)
- `health_check_simple_test.go` (? tests)
- `eventbus_health_test.go` (14 tests)
- `eventbus_health_check_coverage_test.go` (17 tests)

#### 整合为 `health_check_integration_test.go`：
合并以下文件：
- `health_check_comprehensive_test.go` (? tests)
- `health_check_config_test.go` (? tests)
- `health_check_failure_test.go` (? tests)
- `eventbus_start_all_health_check_test.go` (6 tests)

#### 保留：
- ✅ `health_check_deadlock_test.go` - 重要的死锁测试

### 2.3 Factory (2 → 1)

#### 整合为 `factory_test.go`：
合并以下文件：
- `factory_test.go` (22 tests) - 保留为基础
- `factory_coverage_test.go` (17 tests) - 合并进去

### 2.4 Config (3 → 1)

#### 整合为 `config_test.go`：
合并以下文件：
- `json_config_test.go` (18 tests)
- `config_conversion_test.go` (? tests)
- `enterprise_config_layering_test.go` (? tests)

#### 保留：
- ✅ `json_performance_test.go` - 性能测试

## 阶段 3: 整合后端实现

### 3.1 Kafka (3 → 2)

#### 整合为 `kafka_test.go`：
合并以下文件：
- `kafka_unit_test.go` (15 tests)
- `kafka_connection_test.go` (? tests)

#### 保留：
- ✅ `kafka_integration_test.go` (7 tests)

### 3.2 NATS (11 → 4)

#### 整合为 `nats_test.go`：
合并以下文件：
- `nats_unit_test.go` (14 tests)
- `nats_config_architecture_test.go` (? tests)
- `nats_global_worker_test.go` (? tests)
- `nats_persistence_test.go` (? tests)
- `nats_standalone_test.go` (? tests)
- `simple_nats_persistence_test.go` (? tests)

#### 整合为 `nats_benchmark_test.go`：
合并以下文件：
- `nats_simple_benchmark_test.go` (? tests)
- `nats_unified_performance_benchmark_test.go` (? tests)

#### 整合为 `nats_pressure_test.go`：
合并以下文件：
- `nats_jetstream_high_pressure_test.go` (? tests)
- `nats_stage2_pressure_test.go` (? tests)
- `nats_vs_kafka_high_pressure_comparison_test.go` (? tests)

#### 保留：
- ✅ `nats_integration_test.go` (7 tests)

### 3.3 Memory (2 → 1)

#### 整合为 `memory_test.go`：
合并以下文件：
- `memory_advanced_test.go` (11 tests)
- `memory_integration_test.go` (7 tests)

## 阶段 4: 整合辅助组件

### 4.1 Topic Config (4 → 1)

#### 整合为 `topic_config_test.go`：
合并以下文件：
- `topic_config_manager_test.go` (8 tests)
- `topic_config_manager_coverage_test.go` (15 tests)
- `topic_persistence_test.go` (? tests)
- `eventbus_topic_config_coverage_test.go` (15 tests)

### 4.2 Backlog (3 → 2)

#### 整合为 `backlog_detector_test.go`：
合并以下文件：
- `backlog_detector_test.go` (9 tests)
- `backlog_detector_mock_test.go` (12 tests)

#### 保留：
- ✅ `publisher_backlog_detector_test.go` (14 tests)

### 4.3 Internal (2 → 1)

#### 整合为 `internal_test.go`：
合并以下文件：
- `internal_test.go` (? tests)
- `extract_aggregate_id_test.go` (? tests)

### 4.4 其他组件

#### 保留不变：
- ✅ `envelope_advanced_test.go` (16 tests)
- ✅ `message_formatter_test.go` (20 tests)
- ✅ `rate_limiter_test.go` (13 tests)
- ✅ `init_test.go` (12 tests)
- ✅ `e2e_integration_test.go` (6 tests)
- ✅ `pre_subscription_test.go` (? tests)
- ✅ `pre_subscription_pressure_test.go` (? tests)
- ✅ `production_readiness_test.go` (? tests)

## 整合后的文件列表（预期 ~29 个）

### 核心测试
1. eventbus_test.go
2. eventbus_integration_test.go
3. eventbus_types_test.go
4. eventbus_performance_test.go

### 后端实现
5. kafka_test.go
6. kafka_integration_test.go
7. nats_test.go
8. nats_integration_test.go
9. nats_benchmark_test.go
10. nats_pressure_test.go
11. memory_test.go

### 健康检查
12. health_check_test.go
13. health_check_integration_test.go
14. health_check_deadlock_test.go

### 辅助组件
15. backlog_detector_test.go
16. publisher_backlog_detector_test.go
17. topic_config_test.go
18. factory_test.go
19. config_test.go
20. envelope_advanced_test.go
21. message_formatter_test.go
22. rate_limiter_test.go
23. internal_test.go
24. init_test.go

### 集成和性能
25. e2e_integration_test.go
26. pre_subscription_test.go
27. pre_subscription_pressure_test.go
28. production_readiness_test.go
29. json_performance_test.go

**总计**: 29 个文件（从 71 个减少到 29 个，减少 59%）

