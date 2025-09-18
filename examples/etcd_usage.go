//go:build ignore
// +build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/tools/etcd"
)

func runETCDExample() {
	// 1. 加载配置
	err := config.Setup("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. 初始化etcd客户端
	if config.AppConfig.Etcd.IsEnabled() {
		client, err := etcd.NewClientFromConfig(config.AppConfig.Etcd)
		if err != nil {
			log.Fatalf("Failed to create etcd client: %v", err)
		}

		// 设置全局客户端
		etcd.SetClient(client)
		defer client.Close()

		fmt.Println("ETCD client initialized successfully")

		// 3. 演示基本操作
		demonstrateBasicOperations()

		// 4. 演示服务发现
		demonstrateServiceDiscovery()
	} else {
		fmt.Println("ETCD is disabled in configuration")
	}
}

func demonstrateBasicOperations() {
	fmt.Println("\n=== 基本操作演示 ===")

	client := etcd.GetClient()
	ctx := context.Background()

	// 存储配置
	err := client.Put(ctx, "app/database/host", "localhost")
	if err != nil {
		log.Printf("Failed to put key: %v", err)
		return
	}
	fmt.Println("✓ 存储配置: app/database/host = localhost")

	// 读取配置
	value, err := client.Get(ctx, "app/database/host")
	if err != nil {
		log.Printf("Failed to get key: %v", err)
		return
	}
	fmt.Printf("✓ 读取配置: app/database/host = %s\n", value)

	// 列出所有app配置
	configs, err := client.List(ctx, "app/")
	if err != nil {
		log.Printf("Failed to list keys: %v", err)
		return
	}
	fmt.Printf("✓ 所有app配置: %v\n", configs)
}

func demonstrateServiceDiscovery() {
	fmt.Println("\n=== 服务发现演示 ===")

	client := etcd.GetClient()
	ctx := context.Background()

	// 注册服务
	serviceEndpoint := etcd.ServiceEndpoint{
		Address: "localhost",
		Port:    9000,
		Meta: map[string]string{
			"version": "1.0.0",
			"region":  "us-east-1",
		},
	}

	serviceData, _ := json.Marshal(serviceEndpoint)
	err := client.Register(ctx, "services/grpc/user-service", string(serviceData), 30)
	if err != nil {
		log.Printf("Failed to register service: %v", err)
		return
	}
	fmt.Println("✓ 服务注册成功: services/grpc/user-service")

	// 发现服务
	time.Sleep(1 * time.Second) // 等待注册完成
	endpoints, err := client.Discover(ctx, "services/grpc/")
	if err != nil {
		log.Printf("Failed to discover services: %v", err)
		return
	}

	fmt.Printf("✓ 发现 %d 个服务:\n", len(endpoints))
	for i, endpoint := range endpoints {
		fmt.Printf("  服务 %d: %s:%d (版本: %s)\n",
			i+1, endpoint.Address, endpoint.Port, endpoint.Meta["version"])
	}

	// 监听服务变化
	fmt.Println("✓ 开始监听服务变化 (5秒后停止)...")
	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	watchChan, err := client.Watch(watchCtx, "services/grpc/")
	if err != nil {
		log.Printf("Failed to watch services: %v", err)
		return
	}

	go func() {
		for event := range watchChan {
			switch event.Type {
			case etcd.WatchEventTypePut:
				fmt.Printf("  → 服务更新: %s\n", event.Key)
			case etcd.WatchEventTypeDelete:
				fmt.Printf("  → 服务下线: %s\n", event.Key)
			}
		}
	}()

	// 等待监听结束
	<-watchCtx.Done()
	fmt.Println("✓ 监听结束")
}
