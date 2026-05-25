# GoT0ISCC

`GoT0ISCC` 是一个基于 `Go + Wails + React` 的 ISCC 桌面工作台，用于把账号管理、赛道快照、理论题自动化、WP 同步、桌面辅助能力和受管 Python 执行环境收敛到一个桌面应用中。

## 主要能力

- 账号管理：本地保存账号、密码、代理和登录重试配置
- 练武题 / 擂台题：基于本地同步数据展示赛道进度
- 理论题：题库检索、人工提交、AI 设置、自动答题状态跟踪
- 实战题：题目快照、提交入口和附件工作区衔接
- WP 管理：本地快照、远端同步、缺交项识别
- Python 环境：受管解释器初始化、依赖安装、沙盒执行
- 迁移导出：导出本地运行时数据库和工作目录快照

## 技术栈

- Go `1.24.x`
- Wails `v2.10.2`
- React `18`
- Vite `6`
- SQLite

## 仓库结构

```text
GoT0ISCC/
  .github/                  # GitHub Actions、模板、仓库元数据
  cmd/                      # 辅助 CLI 工具
  extensions/               # 浏览器扩展和附属工具
  frontend/                 # React 前端和 Wails 生成绑定
  internal/                 # 应用层、领域层、平台层、桌面 API
  scripts/                  # 本地打包和导出脚本
  tools/                    # 维护和数据整理脚本
  main.go                   # Wails 应用入口
  wails.json                # Wails 配置
```

## 开发环境要求

- Go `1.24.x`
- Node.js `22+`
- Wails CLI `v2.10.2`

安装 Wails CLI：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

## 本地开发

安装前端依赖：

```bash
cd frontend
npm ci
```

在 Go API 签名变更后重新生成 Wails 绑定：

```bash
wails generate module
```

启动开发模式：

```bash
wails dev
```

验证仓库状态：

```bash
go test ./...
cd frontend && npm run ci
```

## 数据目录策略

运行时数据位于 `data/` 下，并且不进入 Git：

- `data/got0iscc.db`
- `data/runtime/`
- `data/challenges/`
- `data/python/`

以下内容也应始终排除在 Git 之外：

- `build/`
- `runtime/`
- `frontend/node_modules/`
- 本地虚拟环境

## GitHub 自动化

仓库内置了跨平台工作流：

- Ubuntu：前端构建 + `go test ./...`
- macOS：构建 Wails 桌面应用并上传 artifact
- Windows：构建 Wails 桌面应用并上传 artifact
- Release：推送 `v*` tag 时自动构建并发布 GitHub Release

相关文件：

- [`.github/workflows/build.yml`](.github/workflows/build.yml)
- [`.github/workflows/release.yml`](.github/workflows/release.yml)
- [`CONTRIBUTING.md`](CONTRIBUTING.md)
- [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md)
- [`.github/dependabot.yml`](.github/dependabot.yml)

## 发布说明

本地打包脚本保留在：

- [`scripts/package_release.sh`](scripts/package_release.sh)

这份脚本偏向本机 macOS 环境使用。CI 和 GitHub Release 使用独立的 workflow 处理打包与发布。

## 维护说明

- `frontend/wailsjs/` 需要纳入版本库，因为前端源码直接依赖 Wails 生成绑定
- `frontend/dist/index.html` 保留最小占位文件，避免干净克隆后 `go test` 因 `embed` 失败
- 当桌面 API 签名变更时，提交前需要重新执行 `wails generate module`
