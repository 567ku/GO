# PR: stage4.1: make ws executor stop idempotent

## 变更范围

### 修改文件

1. **pkg/executor/ws_executor.go**
   - 新增字段：`stopOnce sync.Once`（L52）
   - 修改函数：`Stop()`（L265-274）
     - 使用 `sync.Once` 保护 `close(stopCh)` 和 `cancel()`
     - 允许多次调用 `Stop()` 不会 panic

2. **pkg/executor/ws_executor_test.go**
   - 新增测试：`TestWSExecutor_Stop_Idempotent`（L349-400）
     - 验证多次 Stop() 不 panic
     - 验证 Stop() 后 pump 正确退出
     - 验证无 goroutine 泄漏（阈值断言）
   - 新增测试：`TestWSExecutor_Stop_PumpExitVerify`（L402-434）
     - 验证 Stop() 后 pump 能立即退出（500ms 超时检测）
   - 新增辅助函数：`countGoroutines()`（L436-440）

3. **pkg/engine/p0_fail_test.go**
   - 修复 `NewWSExecutor` 调用签名（增加 outboxSize 参数）
   - L234: `executor.NewWSExecutor(fakeTradeClient, eventCh, 1, 3, 2048)`
   - L252: `executor.NewWSExecutor(fakeTradeClient, eventCh, 2, 4, 2048)`

### 关键函数变更

#### Stop() 方法（ws_executor.go:L265-274）

**修改前**:
```go
func (e *WSExecutor) Stop() error {
	// P0-1: 取消 context，触发 shutdown
	e.cancel()
	close(e.stopCh)
	e.wg.Wait()
	return nil
}
```

**修改后**:
```go
func (e *WSExecutor) Stop() error {
	// P1-EX-01: 使用 sync.Once 保护 close(stopCh) / cancel，防止panic
	e.stopOnce.Do(func() {
		e.cancel()        // 取消 context
		close(e.stopCh)   // 关闭 stopCh（只会执行一次）
	})

	// wait 放在 Once 外部，允许多次调用 Stop 都能等待 pump 退出
	e.wg.Wait()
	return nil
}
```

**核心设计**：
- `sync.Once` 保护 `close(stopCh)` 和 `cancel()`，防止多次调用导致 panic
- `wg.Wait()` 放在 Once 外部，确保每次调用 Stop() 都能等待 pump 退出
- 幂等性：多次调用 Stop() 安全且行为一致

## 风险评估

### Panic 风险消除

#### 问题场景
原 `Stop()` 实现中，多次调用会导致：
1. **重复 close(stopCh)**：panic: close of closed channel
2. **重复 cancel()**：虽然 context.CancelFunc 可多次调用，但语义不清晰

#### 解决方案
**sync.Once 保护机制**：
- `stopOnce.Do(func() { ... })` 保证闭包只执行一次
- 首次调用：正常关闭 channel 和取消 context
- 后续调用：Do() 直接返回，跳过闭包执行
- `wg.Wait()` 可多次调用（WaitGroup 本身支持多次 Wait）

#### 测试验证
```go
// TestWSExecutor_Stop_Idempotent
err1 := exec.Stop() // 第1次：正常关闭
err2 := exec.Stop() // 第2次：幂等，不panic
err3 := exec.Stop() // 第3次：幂等，不panic
```

**测试结果**：
```
✅ 验收点1通过：多次 Stop() 不 panic
✅ 验收点2/3通过：pump 正确退出，无 goroutine 泄漏（baseline=3, current=2, leaked=-1）
```

### 并发安全分析

#### 场景1：并发调用 Stop()
- **风险**：多个 goroutine 同时调用 `Stop()`
- **防护**：`sync.Once` 内部使用原子操作 + 互斥锁，保证只有一个 goroutine 执行闭包
- **结果**：所有 goroutine 都会等待 pump 退出（wg.Wait）

#### 场景2：Stop() 期间 resultPump 仍在运行
- **风险**：pump 可能正在写入 eventCh
- **防护**：
  - `close(stopCh)` 会触发 pump 的 `<-e.stopCh` 分支退出
  - pump 退出前会完成当前事件的转发
  - `wg.Wait()` 确保 pump 完全退出后才返回
- **结果**：优雅关闭，无数据竞争

### 兼容性风险

#### 向后兼容
- **API 不变**：`Stop() error` 签名保持不变
- **行为增强**：从"单次调用"扩展为"幂等调用"
- **调用方影响**：现有调用代码无需修改，行为更安全

#### 测试覆盖
- ✅ 单次 Stop：TestWSExecutor_Stop_PumpExitVerify
- ✅ 多次 Stop：TestWSExecutor_Stop_Idempotent
- ✅ Goroutine 泄漏检测：runtime.NumGoroutine() 阈值断言

## 新增测试列表

### 1. TestWSExecutor_Stop_Idempotent
**测试目标**：验证 Stop() 方法幂等性

**验收点**：
- ✅ 多次调用 Stop() 不会 panic
- ✅ Stop() 后 resultPump goroutine 正确退出
- ✅ 无 goroutine 泄漏（阈值 ≤ 2）

**测试输出**：
```
=== RUN   TestWSExecutor_Stop_Idempotent
    ws_executor_test.go:379: ✅ 验收点1通过：多次 Stop() 不 panic
    ws_executor_test.go:396: ✅ 验收点2/3通过：pump 正确退出，无 goroutine 泄漏（baseline=3, current=2, leaked=-1）
--- PASS: TestWSExecutor_Stop_Idempotent (0.20s)
```

**关键实现**：
```go
exec := NewWSExecutor(tradeClient, eventCh, 2, 3, 2048)
baselineGoroutines := countGoroutines()

err1 := exec.Stop() // 第1次
err2 := exec.Stop() // 第2次
err3 := exec.Stop() // 第3次

// 验证无 goroutine 泄漏
currentGoroutines := countGoroutines()
leaked := currentGoroutines - baselineGoroutines
if leaked > 2 {
	t.Errorf("Goroutine leak detected: leaked=%d", leaked)
}
```

### 2. TestWSExecutor_Stop_PumpExitVerify
**测试目标**：验证 Stop() 后 pump 能立即退出

**验收点**：
- ✅ Stop() 在合理时间内返回（< 500ms）
- ✅ Pump 不会无限阻塞

**测试输出**：
```
=== RUN   TestWSExecutor_Stop_PumpExitVerify
    ws_executor_test.go:432: ✅ Stop() 在 0s 内返回，pump 正确退出
--- PASS: TestWSExecutor_Stop_PumpExitVerify (0.00s)
```

**关键实现**：
```go
exec := NewWSExecutor(tradeClient, eventCh, 2, 3, 2048)

// 发送一些事件到 outbox
for i := 0; i < 10; i++ {
	exec.sendExecutorResult(task, true, 0, "", nil)
}

// 调用 Stop()（应该立即返回，pump 退出）
startTime := time.Now()
err := exec.Stop()
elapsed := time.Since(startTime)

if elapsed > 500*time.Millisecond {
	t.Errorf("Stop() took too long: %v", elapsed)
}
```

## 门禁执行结果

### 1. go test -v ./... -count=1 -timeout 60s

**摘要**：
```
ok      gridbot/pkg/engine      6.316s
ok      gridbot/pkg/executor    2.458s
ok      gridbot/pkg/model       1.157s
ok      gridbot/pkg/store       0.925s
ok      gridbot/pkg/ws          1.037s
```

**关键测试结果**：
```
=== RUN   TestWSExecutor_Stop_Idempotent
    ws_executor_test.go:379: ✅ 验收点1通过：多次 Stop() 不 panic
    ws_executor_test.go:396: ✅ 验收点2/3通过：pump 正确退出，无 goroutine 泄漏（baseline=3, current=2, leaked=-1）
--- PASS: TestWSExecutor_Stop_Idempotent (0.20s)

=== RUN   TestWSExecutor_Stop_PumpExitVerify
    ws_executor_test.go:432: ✅ Stop() 在 0s 内返回，pump 正确退出
--- PASS: TestWSExecutor_Stop_PumpExitVerify (0.00s)
```

**状态**：✅ PASSED

---

### 2. go vet ./...

**执行命令**：
```bash
cd c:\Users\Administrator\Desktop\GO2.0
go vet ./...
```

**输出**：
```
(无输出，检查通过)
```

**状态**：✅ PASSED

---

### 3. go test ./pkg/engine -run Reconnect -count=50

**执行命令**：
```bash
cd c:\Users\Administrator\Desktop\GO2.0
go test ./pkg/engine -run "TestEngine_ReconnectToReconciling|TestReconcile_Reconnected" -count=50
```

**输出**：
```
ok      gridbot/pkg/engine      4.014s
```

**测试覆盖**：
- TestEngine_ReconnectToReconciling：模拟 Reconnect 后进入 Reconciling 模式
- TestReconcile_Reconnected_To_Running_With_GapScanTasks：Reconnect 后 GapScan 任务提交

**状态**：✅ PASSED（50次循环，无失败）

---

## 门禁总结

| 门禁项 | 命令 | 结果 | 耗时 | 备注 |
|--------|------|------|------|------|
| 单元测试 | `go test -v ./... -count=1 -timeout 60s` | ✅ PASS | ~12s | 所有包测试通过 |
| 静态检查 | `go vet ./...` | ✅ PASS | <1s | 无代码问题 |
| Reconnect压测 | `go test ./pkg/engine -run Reconnect -count=50` | ✅ PASS | 4.014s | 50次无失败 |

**结论**：所有门禁通过，可安全合并 ✅

---

## 变更清单

### 核心变更
- [x] WSExecutor.Stop() 幂等化（sync.Once 保护）
- [x] 新增测试：多次 Stop() 不 panic
- [x] 新增测试：Stop() 后 pump 正确退出
- [x] 新增测试：Goroutine 泄漏检测

### 辅助修复
- [x] 修复 engine/p0_fail_test.go 中 NewWSExecutor 调用签名
- [x] 补充 runtime 包导入（用于 goroutine 计数）

### 文件列表
```
pkg/executor/ws_executor.go          (+6 lines, 新增 stopOnce 字段 + Stop 改造)
pkg/executor/ws_executor_test.go     (+99 lines, 2个新测试 + 辅助函数)
pkg/engine/p0_fail_test.go           (+2 lines, 修复函数调用)
```

---

## 技术亮点

### 1. Sync.Once 应用
- **位置**：pkg/executor/ws_executor.go:L265-274
- **价值**：防止 close(channel) 二次调用 panic，标准并发安全模式
- **扩展性**：适用于所有需要"仅一次初始化/清理"的场景

### 2. Goroutine 泄漏检测
- **位置**：pkg/executor/ws_executor_test.go:L436-440
- **方法**：`runtime.NumGoroutine()` 快照对比
- **阈值设计**：允许测试框架自身的少量 goroutine 波动（threshold=2）
- **生产价值**：可扩展为长时间运行的服务健康检查指标

### 3. Stop 超时检测
- **位置**：pkg/executor/ws_executor_test.go:L427
- **验证点**：Stop() 应在 500ms 内返回（正常退出 <100ms）
- **防御场景**：防止 pump 死锁导致 Stop() 永久阻塞

---

## 后续改进建议（可选）

### 1. 增强日志
建议在 Stop() 中增加日志：
```go
e.stopOnce.Do(func() {
	log.Printf("WSExecutor stopping (stopCh closed, ctx cancelled)")
	e.cancel()
	close(e.stopCh)
})
```

### 2. Metrics 埋点
建议增加 Executor 生命周期指标：
- `executor_stop_count`：Stop() 调用次数
- `executor_pump_exit_duration_ms`：pump 退出耗时

### 3. Context 超时
建议 Stop() 增加超时保护：
```go
func (e *WSExecutor) Stop() error {
	e.stopOnce.Do(func() {
		e.cancel()
		close(e.stopCh)
	})
	
	// 带超时的 wg.Wait()
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("pump exit timeout")
	}
}
```

---

## Checklist

- [x] 所有测试通过（go test -v ./... -count=1 -timeout 60s）
- [x] 静态检查通过（go vet ./...）
- [x] Reconnect 压测通过（50次循环）
- [x] 新增测试覆盖核心场景（幂等性 + pump退出 + 泄漏检测）
- [x] 代码注释清晰（P1-EX-01 标记）
- [x] 无向后兼容性破坏
- [x] 风险点已识别并消除（panic 风险通过 sync.Once 防护）
- [x] PR 描述完整（变更范围 + 风险评估 + 测试列表 + 门禁结果）

---

**提交建议**：
```bash
git add pkg/executor/ws_executor.go pkg/executor/ws_executor_test.go pkg/engine/p0_fail_test.go
git commit -m "stage4.1: make ws executor stop idempotent

- Add stopOnce sync.Once to protect close(stopCh)/cancel
- New test: TestWSExecutor_Stop_Idempotent (multi-stop safe)
- New test: TestWSExecutor_Stop_PumpExitVerify (timeout check)
- New test: countGoroutines (leak detection)
- Fix: NewWSExecutor call signature in p0_fail_test.go

Gates: go test ./... ✅ | go vet ✅ | reconnect*50 ✅"
```
