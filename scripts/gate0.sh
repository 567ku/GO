#!/bin/bash
# Gate-0: 基础测试门禁
# 功能: 运行所有单元测试，确保基本功能正常

set -e

echo "========================================="
echo "Gate-0: 基础测试门禁"
echo "========================================="

# 1. 编译检查
echo "步骤 1/3: 编译检查..."
go build ./...

# 2. 运行所有单元测试
echo "步骤 2/3: 运行单元测试..."
go test ./pkg/... -timeout 30s

# 3. 简单smoke测试（快速验证核心模块）
echo "步骤 3/3: Smoke测试..."
go test ./pkg/model ./pkg/store ./pkg/engine -v -run "TestParse|TestSnapshot|TestEngine_Bootstrap"

echo "========================================="
echo "✅ Gate-0: 通过"
echo "========================================="
