# CI/CD 配置说明

## GitHub Actions Workflow

### 文件位置
`.github/workflows/go-test.yml`

### 触发条件
- **Push**: `main`, `dev`, `feature/*` 分支
- **Pull Request**: 针对 `main`, `dev` 分支

### 测试矩阵
- Go 1.21
- Go 1.22

### 执行步骤

#### 1. 环境准备
```bash
# 安装build-essential（包含gcc，race detector所需）
sudo apt-get update
sudo apt-get install -y build-essential

# 验证gcc安装
gcc --version

# 下载Go依赖
go mod download
```

#### 2. 常规测试
```bash
go test -v -count=1 ./pkg/...
```

#### 3. Race Detector测试（P0-CI-02门禁）
```bash
CGO_ENABLED=1 go test -race -v -count=1 ./pkg/...
```

#### 4. 静态检查
```bash
# go vet
go vet ./pkg/...

# gofmt检查
gofmt -s -l ./pkg
```

### 关键配置说明

#### CGO_ENABLED=1
- **必须**：race detector依赖CGO
- **Linux CI**：预装gcc，直接可用
- **Windows本地**：不强制要求（需额外安装MinGW-w64，成本高）

#### -count=1
- 禁用测试缓存，确保每次运行都是真实测试

#### -race
- 启用race detector，检测并发竞态条件
- 性能开销：约10x（仅CI环境，可接受）

### 本地开发建议

#### Linux/macOS
```bash
# 常规测试（快速）
go test ./pkg/...

# race测试（完整）
CGO_ENABLED=1 go test -race ./pkg/...
```

#### Windows
```bash
# 常规测试（推荐）
go test ./pkg/...

# race测试（可选，需安装gcc）
# 不强制要求，CI会执行
```

### 故障排查

#### 错误：`cgo: C compiler "gcc" not found`
**原因**：缺少C编译器

**解决**（Linux/Ubuntu）：
```bash
sudo apt-get update
sudo apt-get install -y build-essential
```

**解决**（macOS）：
```bash
xcode-select --install
```

**解决**（Windows，可选）：
1. 安装 [MinGW-w64](https://www.mingw-w64.org/)
2. 或安装 [TDM-GCC](https://jmeubank.github.io/tdm-gcc/)
3. 配置PATH环境变量

#### 错误：Race detector报告竞态
**示例输出**：
```
==================
WARNING: DATA RACE
Write at 0x00c000012345 by goroutine 7:
  ...
Previous read at 0x00c000012345 by goroutine 6:
  ...
==================
```

**修复方向**：
1. 使用 `sync.Mutex` 保护共享状态
2. 使用 `atomic` 包进行原子操作
3. 使用 channel 传递数据（避免共享）
4. 检查是否误用了非线程安全的全局变量

### CI徽章（可选）

在项目 `README.md` 中添加：

```markdown
![Go Tests](https://github.com/YOUR_USERNAME/YOUR_REPO/actions/workflows/go-test.yml/badge.svg)
```

### 验收标准

✅ **CI绿**：所有步骤通过
- 常规测试：`go test ./pkg/...` ✅
- Race测试：`go test -race ./pkg/...` ✅
- 静态检查：`go vet ./pkg/...` ✅
- 格式检查：`gofmt -s -l ./pkg` 无输出 ✅

---

## 相关文档

- [Go Race Detector官方文档](https://go.dev/doc/articles/race_detector)
- [GitHub Actions Go Setup](https://github.com/actions/setup-go)
- [CGO环境配置](https://pkg.go.dev/cmd/cgo)
