// P0-1: TaskID 唯一性保证（ULID实现）
package model

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync/atomic"
)

// ULID 生成器（简化版，基于时间戳 + 随机数 + 原子计数器）
// 格式: <timestamp_ms>_<random>_<counter>
// 唯一性保证：同一毫秒内最多生成 2^16 个不同ID

var (
	ulidCounter uint64 // 原子计数器
)

// GenerateTaskID 生成全局唯一的 TaskID
// 格式: task_<timestamp>_<random16>_<counter16>
// 示例: task_1732672800000_a3f2_0001
func GenerateTaskID() string {
	// 时间戳（毫秒）
	ts := NowMs()

	// 随机数（16位十六进制，64bit）
	var randBytes [8]byte
	_, _ = rand.Read(randBytes[:])
	randomPart := binary.BigEndian.Uint64(randBytes[:])

	// 原子计数器（递增）
	counter := atomic.AddUint64(&ulidCounter, 1)

	// 组合: task_<ts>_<random16>_<counter4>
	return fmt.Sprintf("task_%d_%016x_%04x", ts, randomPart, counter&0xFFFF)
}

// GenerateReconcileTaskID 生成对账任务专用 TaskID
// 格式: reconcile_<timestamp>_<random16>_<counter16>
func GenerateReconcileTaskID() string {
	ts := NowMs()
	var randBytes [8]byte
	_, _ = rand.Read(randBytes[:])
	randomPart := binary.BigEndian.Uint64(randBytes[:])
	counter := atomic.AddUint64(&ulidCounter, 1)
	return fmt.Sprintf("reconcile_%d_%016x_%04x", ts, randomPart, counter&0xFFFF)
}
