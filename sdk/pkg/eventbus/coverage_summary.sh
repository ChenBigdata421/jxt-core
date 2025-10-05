#!/bin/bash
echo "=== EventBus 测试覆盖率报告 ==="
echo ""
echo "总体覆盖率:"
go tool cover -func=coverage.out | grep "total:"
echo ""
echo "各文件覆盖率详情:"
go tool cover -func=coverage.out | awk '
/^github/ {
    split($1, parts, ":")
    file = parts[1]
    gsub(/.*\//, "", file)
    if (file != last_file) {
        if (last_file != "") {
            printf "%-40s %6.1f%%\n", last_file, total/count
        }
        last_file = file
        total = 0
        count = 0
    }
    gsub(/%/, "", $NF)
    total += $NF
    count++
}
END {
    if (last_file != "") {
        printf "%-40s %6.1f%%\n", last_file, total/count
    }
}' | sort -t% -k2 -rn
