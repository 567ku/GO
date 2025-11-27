#!/bin/bash
# Gate-WS-Reconnect: WS重连稳定性门禁
# 功能: 验证WS重连50次后无goroutine泄漏、无资源泄漏

set -e

echo "========================================="
echo "Gate-WS-Reconnect: WS重连稳定性门禁"
echo "========================================="

# 1. WS重连50次压测
echo "步骤 1/2: WS重连50次压测..."
go test ./pkg/ws -run "TestReconnect" -count=50 -timeout 10m

# 2. WS健康检查测试（Freeze机制）
echo "步骤 2/2: WS健康检查测试..."
go test ./pkg/ws -run "TestManager|TestWSState" -v -timeout 2m

echo "========================================="
echo "✅ Gate-WS-Reconnect: 通过（50次重连稳定）"
echo "========================================="
