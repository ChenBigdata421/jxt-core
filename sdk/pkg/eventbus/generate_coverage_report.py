#!/usr/bin/env python3
"""生成 EventBus 测试覆盖率报告"""

import subprocess
from collections import defaultdict

def main():
    # 运行 go tool cover
    result = subprocess.run(
        ['go', 'tool', 'cover', '-func=coverage.out'],
        capture_output=True,
        text=True
    )
    
    lines = result.stdout.strip().split('\n')
    
    # 按文件分组统计
    file_coverage = defaultdict(list)
    total_coverage = ''
    
    for line in lines:
        if 'total:' in line:
            total_coverage = line.split()[-1]
        elif line.startswith('github.com'):
            parts = line.split()
            if len(parts) >= 3:
                file_name = parts[0].split(':')[0].split('/')[-1]
                try:
                    coverage = float(parts[-1].replace('%', ''))
                    file_coverage[file_name].append(coverage)
                except:
                    pass
    
    # 计算每个文件的平均覆盖率
    file_avg = {}
    for file_name, coverages in file_coverage.items():
        if coverages:
            file_avg[file_name] = sum(coverages) / len(coverages)
    
    # 按覆盖率排序
    sorted_files = sorted(file_avg.items(), key=lambda x: x[1], reverse=True)
    
    # 打印报告
    print('=' * 80)
    print('EventBus 组件测试覆盖率报告')
    print('=' * 80)
    print()
    print(f'📊 总体覆盖率: {total_coverage}')
    print()
    print('📁 各文件覆盖率详情:')
    print('-' * 80)
    print(f"{'文件名':<45} {'覆盖率':>10}")
    print('-' * 80)
    
    for file_name, avg_coverage in sorted_files:
        if avg_coverage >= 70:
            status = '✅'
        elif avg_coverage >= 40:
            status = '⚠️ '
        else:
            status = '❌'
        print(f'{status} {file_name:<42} {avg_coverage:>8.1f}%')
    
    print('-' * 80)
    print()
    
    # 统计
    high = sum(1 for _, cov in sorted_files if cov >= 70)
    medium = sum(1 for _, cov in sorted_files if 40 <= cov < 70)
    low = sum(1 for _, cov in sorted_files if cov < 40)
    
    print(f'📈 覆盖率分布:')
    print(f'   ✅ 高覆盖率 (≥70%): {high} 个文件')
    print(f'   ⚠️  中覆盖率 (40-70%): {medium} 个文件')
    print(f'   ❌ 低覆盖率 (<40%): {low} 个文件')
    print()
    
    # 重点关注的低覆盖率文件
    print('🔍 需要重点关注的低覆盖率文件 (<40%):')
    print('-' * 80)
    low_coverage_files = [(f, c) for f, c in sorted_files if c < 40]
    if low_coverage_files:
        for file_name, coverage in low_coverage_files:
            print(f'   ❌ {file_name:<42} {coverage:>8.1f}%')
    else:
        print('   🎉 没有低覆盖率文件！')
    print()

if __name__ == '__main__':
    main()

