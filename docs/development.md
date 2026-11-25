# 开发与测试

## 环境与构建

需要 Go 1.26、Node.js 22.12+、npm 和 Make。Go 工具链及前端依赖由仓库中的 go.mod、go.sum 和 package-lock.json 确定。SQLite 使用纯 Go 驱动，发布构建关闭 CGO。

```sh
npm --prefix web ci
make build
```

`make build` 先检查并构建 Web，再构建当前平台程序。macOS 输出 Hub 与 CLI；Linux 输出 CLI。`make build-all` 构建 macOS/Linux 的 amd64 与 arm64 发布产物。

产物位于 `dist/`，Go 缓存位于 `.cache/`；它们不加入版本控制。Hub 通过 Go embed 携带 web/dist，因此在直接运行 go test 或 go run 前，应先生成前端产物。

## 本地开发

先构建前端，再启动仅供本机访问的 Hub：

```sh
npm --prefix web run build

go run ./cmd/readerhub serve \
  --listen 127.0.0.1:8848 \
  --data "$HOME/ReaderHubDevData"
```

另开终端启动前端：

```sh
npm --prefix web run dev
```

Vite 会将 `/api` 与 `/health` 代理到本机 Hub。使用开发数据目录的访问口令登录，避免将真实阅读资料混入测试环境。

## 检查与测试

```sh
# TypeScript、前端构建、Go 测试与静态检查
make check

# 使用项目缓存进行 Go 并发检测
GOCACHE="$PWD/.cache/go-build" \
GOMODCACHE="$PWD/.cache/go-mod" \
GOTOOLCHAIN=auto go test -race ./...

# 构建并运行浏览器端到端测试
make test-e2e
```

浏览器测试使用本机 Google Chrome，测试数据、日志和失败截图位于忽略的 `.local/`。脚本创建隔离 Hub，使用真实 CLI 上传图文样例，并在退出时停止测试服务。测试端口为 `127.0.0.1:18848`。

可对已有构建产物运行指定项目：

```sh
bash scripts/test-e2e.sh --project=desktop
bash scripts/test-e2e.sh --project=ipad-portrait
bash scripts/test-e2e.sh --project=android-tablet
```

目前浏览器测试包含三种尺寸下的图文阅读、收藏、想法编辑、手动状态与搜索、不可信 Markdown 展示、取消响应回归，共 9 项。Go 测试覆盖素材收集、校验、并发去重、数据恢复、认证、收藏关系与导出。

iPad 和安卓项目使用 Chrome 的视口及触摸模拟，不是实际 iPad Safari 或安卓设备验收。生产目标中的 Web 不需要安装 Chrome 测试工具。

## 代码结构

```text
cmd/readerhub/          Hub 入口与生命周期
cmd/readerhub-cli/      配置与提交命令
internal/bundle/       Markdown AST、图片收集与可迁移文本
internal/client/       HTTP 上传与重试
internal/config/       配置与访问口令
internal/document/     文章、来源和想法数据
internal/protocol/     传输清单与回执
internal/hub/          提交校验与接收用例
internal/httpapi/      路由、认证、图片与 ZIP 导出
internal/storage/      SQLite、文件、迁移与中断恢复
internal/research/     禁用的 AI 集成接口
web/src/               React 界面与 Markdown 阅读器
scripts/               构建、端到端测试、LaunchAgent 生成
```
