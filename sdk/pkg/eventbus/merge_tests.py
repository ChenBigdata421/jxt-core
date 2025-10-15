#!/usr/bin/env python3
"""
合并多个 Go 测试文件为一个文件
"""
import re
import sys
from pathlib import Path

def merge_go_test_files(input_files, output_file):
    """合并多个 Go 测试文件"""

    # 收集所有 imports
    all_imports = set()
    # 收集所有代码（函数、变量、常量等）
    all_code = []

    for input_file in input_files:
        if not Path(input_file).exists():
            print(f"Warning: {input_file} not found, skipping")
            continue

        with open(input_file, 'r', encoding='utf-8') as f:
            lines = f.readlines()

        # 状态机：解析文件
        in_import = False
        import_lines = []
        code_lines = []
        skip_package = True

        for line in lines:
            # 跳过 package 声明
            if skip_package and line.strip().startswith('package '):
                skip_package = False
                continue

            # 处理 import 块
            if line.strip().startswith('import ('):
                in_import = True
                continue
            elif in_import:
                if line.strip() == ')':
                    in_import = False
                    continue
                else:
                    import_lines.append(line.strip())
                    continue

            # 处理单行 import
            if line.strip().startswith('import '):
                match = re.match(r'import\s+"([^"]+)"', line.strip())
                if match:
                    import_lines.append(f'"{match.group(1)}"')
                continue

            # 收集代码
            if line.strip() and not skip_package:
                code_lines.append(line.rstrip())

        # 添加 imports
        for imp in import_lines:
            if imp and not imp.startswith('//'):
                all_imports.add(imp)

        # 添加代码
        if code_lines:
            # 移除开头的空行
            while code_lines and not code_lines[0].strip():
                code_lines.pop(0)
            # 移除结尾的空行
            while code_lines and not code_lines[-1].strip():
                code_lines.pop()

            if code_lines:
                all_code.append('\n'.join(code_lines))

    # 生成输出文件
    with open(output_file, 'w', encoding='utf-8') as f:
        # Package 声明
        f.write('package eventbus\n\n')

        # Imports
        if all_imports:
            f.write('import (\n')
            for imp in sorted(all_imports):
                f.write(f'\t{imp}\n')
            f.write(')\n\n')

        # 所有代码
        for i, code in enumerate(all_code):
            f.write(code)
            if i < len(all_code) - 1:
                f.write('\n\n')
        f.write('\n')

    print(f"[OK] Merged {len(input_files)} files into {output_file}")
    print(f"   Total imports: {len(all_imports)}")
    print(f"   Total code blocks: {len(all_code)}")

if __name__ == '__main__':
    # 定义所有需要整合的文件组
    merge_groups = [
        # NATS Unit Tests (6 → 1)
        {
            'files': [
                'sdk/pkg/eventbus/nats_unit_test.go',
                'sdk/pkg/eventbus/nats_config_architecture_test.go',
                'sdk/pkg/eventbus/nats_global_worker_test.go',
                'sdk/pkg/eventbus/nats_persistence_test.go',
                'sdk/pkg/eventbus/nats_standalone_test.go',
                'sdk/pkg/eventbus/simple_nats_persistence_test.go',
            ],
            'output': 'sdk/pkg/eventbus/nats_test.go'
        },
        # NATS Benchmark (2 → 1)
        {
            'files': [
                'sdk/pkg/eventbus/nats_simple_benchmark_test.go',
                'sdk/pkg/eventbus/nats_unified_performance_benchmark_test.go',
            ],
            'output': 'sdk/pkg/eventbus/nats_benchmark_test.go'
        },
        # NATS Pressure (3 → 1)
        {
            'files': [
                'sdk/pkg/eventbus/nats_jetstream_high_pressure_test.go',
                'sdk/pkg/eventbus/nats_stage2_pressure_test.go',
                'sdk/pkg/eventbus/nats_vs_kafka_high_pressure_comparison_test.go',
            ],
            'output': 'sdk/pkg/eventbus/nats_pressure_test.go'
        },
        # Health Check Unit (7 → 1)
        {
            'files': [
                'sdk/pkg/eventbus/health_checker_advanced_test.go',
                'sdk/pkg/eventbus/health_check_subscriber_extended_test.go',
                'sdk/pkg/eventbus/health_check_message_coverage_test.go',
                'sdk/pkg/eventbus/health_check_message_extended_test.go',
                'sdk/pkg/eventbus/health_check_simple_test.go',
                'sdk/pkg/eventbus/eventbus_health_test.go',
                'sdk/pkg/eventbus/eventbus_health_check_coverage_test.go',
            ],
            'output': 'sdk/pkg/eventbus/health_check_test.go'
        },
        # Health Check Integration (3 → 1)
        {
            'files': [
                'sdk/pkg/eventbus/health_check_comprehensive_test.go',
                'sdk/pkg/eventbus/health_check_config_test.go',
                'sdk/pkg/eventbus/health_check_failure_test.go',
                'sdk/pkg/eventbus/eventbus_start_all_health_check_test.go',
            ],
            'output': 'sdk/pkg/eventbus/health_check_integration_test.go'
        },
        # Topic Config (4 → 1)
        {
            'files': [
                'sdk/pkg/eventbus/topic_config_manager_test.go',
                'sdk/pkg/eventbus/topic_config_manager_coverage_test.go',
                'sdk/pkg/eventbus/topic_persistence_test.go',
                'sdk/pkg/eventbus/eventbus_topic_config_coverage_test.go',
            ],
            'output': 'sdk/pkg/eventbus/topic_config_test.go'
        },
    ]

    for group in merge_groups:
        print(f"\n{'='*60}")
        merge_go_test_files(group['files'], group['output'])

