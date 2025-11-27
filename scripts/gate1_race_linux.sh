#!/bin/bash
# Gate-1: Linux -race 竞态检测门禁
# 功能: 在Linux环境下运行带-race的测试，检测并发竞态问题
# 注意: 必须在Linux环境执行，需要CGO支持

set -e

echo "========================================="
echo "Gate-1: Linux -race 竞态检测门禁"
echo "========================================="

# 检查操作系统
if [[ "$OSTYPE" != "linux-gnu"* ]]; then
    echo "⚠️  警告: 当前系统不是Linux ($OSTYPE)"
    echo "    -race测试需要在Linux环境执行"
    echo "    建议使用Docker容器或Linux虚拟机"
    exit 1
fi

# 检查CGO是否启用
if [ "$CGO_ENABLED" != "1" ]; then
    echo "启用CGO..."
    export CGO_ENABLED=1
fi

# 1. 运行-race测试（所有pkg包）
echo "步骤 1/2: 运行-race测试..."
go test -race ./pkg/... -timeout 5m

# 2. 重点测试并发模块（Engine/WS）
echo "步骤 2/2: 并发模块重点测试..."
go test -race ./pkg/engine ./pkg/ws -v -timeout 3m

echo "========================================="
echo "✅ Gate-1: 通过（无竞态问题）"
echo "========================================="
