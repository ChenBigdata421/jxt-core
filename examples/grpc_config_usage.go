//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"log"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/spf13/viper"
)

// Config 应用配置
type Config struct {
	Application struct {
		Name    string `mapstructure:"name"`
		Mode    string `mapstructure:"mode"`
		Version string `mapstructure:"version"`
	} `mapstructure:"application"`

	ETCD config.ETCDConfig `mapstructure:"etcd"`
	GRPC config.GRPCConfig `mapstructure:"grpc"`
	HTTP config.HTTPConfig `mapstructure:"http"`
}

func runGRPCConfigExample() {
	// 加载配置
	cfg := loadConfig()

	// 演示如何使用配置
	demonstrateGRPCConfig(cfg)
}

func loadConfig() *Config {
	viper.SetConfigName("config.example")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}

	return &cfg
}

func demonstrateGRPCConfig(cfg *Config) {
	fmt.Println("=== gRPC 配置演示 ===")

	// 1. 显示原始配置
	fmt.Printf("服务端配置:\n")
	fmt.Printf("  监听地址: %s\n", cfg.GRPC.Server.ListenOn)
	fmt.Printf("  服务键名: %s\n", cfg.GRPC.Server.ServiceKey)
	fmt.Printf("  是否启用: %t\n", cfg.GRPC.Server.Enabled)
	fmt.Printf("  超时时间: %dms\n", cfg.GRPC.Server.Timeout)

	fmt.Printf("\n客户端配置:\n")
	fmt.Printf("  服务键名: %s\n", cfg.GRPC.Client.ServiceKey)
	fmt.Printf("  是否启用: %t\n", cfg.GRPC.Client.Enabled)
	fmt.Printf("  使用服务发现: %t\n", cfg.GRPC.Client.UseDiscovery)
	fmt.Printf("  Keepalive时间: %v\n", cfg.GRPC.Client.KeepaliveTime)

	// 2. 转换为go-zero配置
	fmt.Printf("\n=== 转换为 go-zero 配置 ===\n")

	// 服务端配置转换
	serverConf := cfg.GRPC.Server.ToGoZeroRpcServerConf(&cfg.ETCD)
	fmt.Printf("go-zero 服务端配置: %+v\n", serverConf)

	// 客户端配置转换
	clientConf := cfg.GRPC.Client.ToGoZeroRpcClientConf(&cfg.ETCD)
	fmt.Printf("go-zero 客户端配置: %+v\n", clientConf)

	// 3. 演示不同连接模式
	fmt.Printf("\n=== 不同连接模式演示 ===\n")

	// 直连模式
	directClient := cfg.GRPC.Client
	directClient.UseDiscovery = false
	directClient.Endpoints = []string{"localhost:9000", "localhost:9001"}
	directConf := directClient.ToGoZeroRpcClientConf(&cfg.ETCD)
	fmt.Printf("直连模式配置: %+v\n", directConf)

	// Target模式
	targetClient := cfg.GRPC.Client
	targetClient.UseDiscovery = false
	targetClient.Target = "dns:///localhost:9000"
	targetConf := targetClient.ToGoZeroRpcClientConf(&cfg.ETCD)
	fmt.Printf("Target模式配置: %+v\n", targetConf)

	// 4. 演示中间件配置
	fmt.Printf("\n=== 中间件配置 ===\n")
	fmt.Printf("服务端中间件:\n")
	fmt.Printf("  链路跟踪: %t\n", cfg.GRPC.Server.Middlewares.Trace)
	fmt.Printf("  异常恢复: %t\n", cfg.GRPC.Server.Middlewares.Recover)
	fmt.Printf("  统计: %t\n", cfg.GRPC.Server.Middlewares.Stat)
	fmt.Printf("  Prometheus: %t\n", cfg.GRPC.Server.Middlewares.Prometheus)
	fmt.Printf("  熔断器: %t\n", cfg.GRPC.Server.Middlewares.Breaker)

	fmt.Printf("\n客户端中间件:\n")
	fmt.Printf("  链路跟踪: %t\n", cfg.GRPC.Client.Middlewares.Trace)
	fmt.Printf("  持续时间: %t\n", cfg.GRPC.Client.Middlewares.Duration)
	fmt.Printf("  Prometheus: %t\n", cfg.GRPC.Client.Middlewares.Prometheus)
	fmt.Printf("  熔断器: %t\n", cfg.GRPC.Client.Middlewares.Breaker)
}

// 演示如何在实际项目中使用
func createGoZeroRpcServer(cfg *Config) error {
	if !cfg.GRPC.Server.Enabled {
		return fmt.Errorf("gRPC服务未启用")
	}

	// 转换配置
	rpcConf := cfg.GRPC.Server.ToGoZeroRpcServerConf(&cfg.ETCD)

	// 这里可以使用转换后的配置创建go-zero的RPC服务
	fmt.Printf("创建 gRPC 服务，配置: %+v\n", rpcConf)

	// 示例：
	// server := zrpc.MustNewServer(rpcConf, func(s *grpc.Server) {
	//     // 注册服务
	// })
	// server.Start()

	return nil
}

func createGoZeroRpcClient(cfg *Config) error {
	if !cfg.GRPC.Client.Enabled {
		return fmt.Errorf("gRPC客户端未启用")
	}

	// 转换配置
	rpcConf := cfg.GRPC.Client.ToGoZeroRpcClientConf(&cfg.ETCD)

	// 这里可以使用转换后的配置创建go-zero的RPC客户端
	fmt.Printf("创建 gRPC 客户端，配置: %+v\n", rpcConf)

	// 示例：
	// client := zrpc.MustNewClient(rpcConf)
	// conn := client.Conn()

	return nil
}
