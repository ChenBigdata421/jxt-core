package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使用方式:")
		fmt.Println("  go run examples/*.go etcd    # 运行ETCD示例")
		fmt.Println("  go run examples/*.go grpc    # 运行gRPC配置示例")
		return
	}

	switch os.Args[1] {
	case "etcd":
		fmt.Println("=== 运行 ETCD 示例 ===")
		runETCDExample()
	case "grpc":
		fmt.Println("=== 运行 gRPC 配置示例 ===")
		runGRPCConfigExample()
	default:
		fmt.Printf("未知示例: %s\n", os.Args[1])
		fmt.Println("可用示例: etcd, grpc")
	}
}
