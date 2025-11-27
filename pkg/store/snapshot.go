// Stage 4A: Snapshot存储 - 原子写+版本号+校验
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gridbot/pkg/model"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// SnapshotStore Snapshot存储管理器
type SnapshotStore struct {
	path string     // 文件路径（如: data/grid_state.json）
	mu   sync.Mutex // 保护并发写入
}

// NewSnapshotStore 创建Snapshot存储管理器
func NewSnapshotStore(path string) *SnapshotStore {
	return &SnapshotStore{path: path}
}

// SnapshotEnvelope Snapshot存储信封（外层包装）
// 包含版本号、校验和、元数据
type SnapshotEnvelope struct {
	Version     string                  `json:"version"`     // 快照格式版本（如"1.0"）
	Checksum    string                  `json:"checksum"`    // SHA256校验和（hex编码）
	SavedAtMs   int64                   `json:"savedAtMs"`   // 保存时间（冗余，用于快速查看）
	EngineMode  model.EngineMode        `json:"engineMode"`  // 保存时的引擎状态（冗余）
	Snapshot    model.GridStateSnapshot `json:"snapshot"`    // 实际快照数据
	MetaComment string                  `json:"metaComment"` // 元数据注释（可选，用于调试）
}

// Save 原子写入Snapshot
// 策略：写入临时文件 → 计算校验和 → 原子重命名
func (s *SnapshotStore) Save(snapshot *model.GridStateSnapshot) error {
	// 0. 加锁保护并发写入
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 确保目录存在
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	// 2. 创建存储信封
	envelope := SnapshotEnvelope{
		Version:     "1.0",
		SavedAtMs:   model.NowMs(),
		EngineMode:  snapshot.EngineMode,
		Snapshot:    *snapshot,
		MetaComment: fmt.Sprintf("Grid snapshot for %s", snapshot.Symbol),
	}

	// 3. 序列化为JSON
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	// 4. 计算校验和
	hash := sha256.Sum256(data)
	envelope.Checksum = hex.EncodeToString(hash[:])

	// 5. 重新序列化（包含校验和）
	dataWithChecksum, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot with checksum: %w", err)
	}

	// 6. 写入临时文件（原子写策略）
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, dataWithChecksum, 0644); err != nil {
		return fmt.Errorf("write temp snapshot: %w", err)
	}

	// 7. 原子重命名（覆盖旧文件）
	if err := os.Rename(tmpPath, s.path); err != nil {
		// 清理临时文件
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename snapshot: %w", err)
	}

	return nil
}

// Load 加载并校验Snapshot
func (s *SnapshotStore) Load() (*model.GridStateSnapshot, error) {
	// 1. 读取文件
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	// 2. 解析信封
	var envelope SnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	// 3. 校验版本号
	if envelope.Version != "1.0" {
		return nil, fmt.Errorf("unsupported snapshot version: %s (expected 1.0)", envelope.Version)
	}

	// 4. 重新序列化快照数据（用于计算校验和）
	envelopeForCheck := envelope
	envelopeForCheck.Checksum = "" // 清空校验和字段
	checkData, err := json.MarshalIndent(envelopeForCheck, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal for checksum: %w", err)
	}

	// 5. 验证校验和
	hash := sha256.Sum256(checkData)
	expectedChecksum := hex.EncodeToString(hash[:])
	if envelope.Checksum != expectedChecksum {
		return nil, fmt.Errorf("checksum mismatch: got %s, expected %s", envelope.Checksum, expectedChecksum)
	}

	// 6. 返回快照数据
	return &envelope.Snapshot, nil
}

// Verify 验证Snapshot文件完整性（不加载到内存）
func (s *SnapshotStore) Verify() error {
	// 1. 打开文件
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrSnapshotNotFound
		}
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer f.Close()

	// 2. 读取并解析
	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}

	var envelope SnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}

	// 3. 验证版本号
	if envelope.Version != "1.0" {
		return fmt.Errorf("unsupported version: %s", envelope.Version)
	}

	// 4. 验证校验和
	envelopeForCheck := envelope
	envelopeForCheck.Checksum = ""
	checkData, err := json.MarshalIndent(envelopeForCheck, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal for checksum: %w", err)
	}

	hash := sha256.Sum256(checkData)
	expectedChecksum := hex.EncodeToString(hash[:])
	if envelope.Checksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: file may be corrupted")
	}

	return nil
}

// Exists 检查Snapshot文件是否存在
func (s *SnapshotStore) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Delete 删除Snapshot文件（慎用）
func (s *SnapshotStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

// ErrSnapshotNotFound Snapshot文件不存在错误
var ErrSnapshotNotFound = fmt.Errorf("snapshot file not found")
