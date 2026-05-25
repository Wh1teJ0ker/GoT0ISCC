# GoT0ISCC Desktop

`GoT0ISCC` 是新一代桌面端重构目录，目标是把当前 `Auto/` 与 `AutoGo/` 的能力收敛到一个完整的 `Go + Wails + React` 架构里，并内置 `Python` 沙盒执行能力。

## 目标

- 使用 `Wails` 提供桌面壳、窗口生命周期和前后端绑定
- 使用 `Go` 承担核心业务、任务编排、状态管理和运行时控制
- 使用 `React` 构建桌面控制台 UI
- 内置 `Python` 沙盒，方便执行解题脚本、临时代码和后续求解器插件
- 为后续逐步迁移 `Auto/` 和 `AutoGo/` 预留清晰边界

## 当前目录

```text
GoT0ISCC/
  main.go
  go.mod
  wails.json
  README.md
  docs/
    architecture.md
    migration-plan.md
  build/
  frontend/
  internal/
    application/
    bootstrap/
    platform/
    presentation/
  data/
    got0iscc.init.sql          # 首次启动种子 SQL
    got0iscc.init.example.yaml # 初始化配置示例
    清洗后的题库*.json
    ISCC题库(1)(1).docx
    got0iscc.db                # 运行库，不提交
```

## 本阶段完成内容

- 建立新的 `Wails` 工程目录
- 梳理出桌面端推荐分层
- 提供 Python 沙盒接口与一个可运行的本地隔离执行器
- 提供桌面首页骨架，用于展示架构信息和试跑 Python 沙盒

## 开发命令

```bash
npm --prefix frontend install
wails generate module
npm --prefix frontend run build
go test ./...
```

如果本机已安装并配置好 Wails，可直接开发：

```bash
wails dev
```

## 数据落点

- 账户库、运行日志、沙盒临时目录、附件目录统一放在 `data/`
- 首次启动如果没有 `data/got0iscc.db`，会自动初始化本地 SQLite
- 如果检测到旧数据目录，会自动迁移到 `data/`
- 首次导入所需静态数据统一来自 `data/got0iscc.init.sql`
- 运行配置写入 SQLite `meta`
- `data/got0iscc.init.example.yaml` 仅作为配置示例保留，不参与首启导入
- `data/` 下的种子 SQL、初始化示例和题库源文件可以随仓库同步
- `data/*.db`、`data/runtime/`、`data/python/` 属于本地运行态，不提交

## 初始化

- 首启种子文件：`data/got0iscc.init.sql`
- 示例配置文件：`data/got0iscc.init.example.yaml`
- 首次启动时导入静态默认值与题库数据，导入后运行态以 SQLite 为准

## 迁移原则

- 旧系统先保留：上级工作区里的 `Auto/`
- 现有 Go 控制层先保留：上级工作区里的 `AutoGo/`
- 新系统先搭骨架、定边界、补沙盒，再逐步迁移具体业务能力

详细设计见：

- [architecture.md](docs/architecture.md)
- [migration-plan.md](docs/migration-plan.md)
