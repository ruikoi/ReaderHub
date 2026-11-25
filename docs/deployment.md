# 部署与维护

ReaderHub 第一版由一个常开 macOS Hub 和按需运行的 macOS/Linux CLI 组成。平板通过浏览器访问，无需安装原生应用。本文示例 IP 为 `192.168.1.10`，使用时替换为 Hub 实际地址。

## 准备构建产物

在开发电脑的项目根目录执行：

```sh
npm --prefix web ci
make build-all
```

开发环境需要 Go 1.26、Node.js 22.12+、npm 和 Make。`go.mod` 指定 Go 工具链，前端依赖由 `web/package-lock.json` 锁定。

| 产物目录 | 内容 |
| --- | --- |
| dist/darwin-arm64 | Apple Silicon Mac 的 readerhub 和 readerhub-cli |
| dist/darwin-amd64 | Intel Mac 的 readerhub 和 readerhub-cli |
| dist/linux-amd64 | x86_64 Linux 的 readerhub-cli |
| dist/linux-arm64 | ARM64 Linux 的 readerhub-cli |

目标电脑只需要对应的二进制，不需要 Go、Node.js、SQLite 命令行或单独的 Web 服务器。网页已经嵌入 Hub。当前不支持 Windows CLI，部署对象为普通电脑，不包括嵌入式设备。

## 首次启动 Hub

以下命令在 Hub Mac 上执行，以 Apple Silicon 为例。若在另一台电脑构建，先把对应产物复制到 Hub。

```sh
mkdir -p "$HOME/.local/bin" "$HOME/ReaderHubData"
install -m 755 ./dist/darwin-arm64/readerhub "$HOME/.local/bin/readerhub"

"$HOME/.local/bin/readerhub" serve \
  --listen 192.168.1.10:8848 \
  --data "$HOME/ReaderHubData"
```

`--listen` 指定本机实际持有的 IP 与端口。默认地址 `127.0.0.1:8848` 只允许本机访问；供平板阅读时应显式绑定 Hub 的内网 IP。

另开终端验证：

```sh
curl --fail http://192.168.1.10:8848/health
"$HOME/.local/bin/readerhub" token --data "$HOME/ReaderHubData"
```

健康检查返回版本、就绪状态和 `ai_enabled: false`。访问口令由首次启动生成，保存在数据目录的 `access-token` 文件中；浏览器和 CLI 均使用此口令。

## 配置 macOS 常驻运行

先停止前台启动的实例。使用项目中的配置生成器创建用户 LaunchAgent；生成器需要 Python 3，运行 Hub 本身不需要它。

```sh
mkdir -p "$HOME/ReaderHubData/logs" "$HOME/Library/LaunchAgents"

python3 scripts/render-launchagent.py \
  --binary "$HOME/.local/bin/readerhub" \
  --data "$HOME/ReaderHubData" \
  --listen 192.168.1.10:8848 \
  --output "$HOME/Library/LaunchAgents/io.readerhub.hub.plist"

plutil -lint "$HOME/Library/LaunchAgents/io.readerhub.hub.plist"
launchctl bootstrap "gui/$(id -u)" \
  "$HOME/Library/LaunchAgents/io.readerhub.hub.plist"
```

LaunchAgent 在该用户登录会话中运行，登录时启动，并在进程退出后重新拉起；这不等同于无人登录的系统服务。Hub 使用绝对数据路径，升级或改变工作目录不会生成另一份空数据。

常用管理命令：

```sh
# 状态
launchctl print "gui/$(id -u)/io.readerhub.hub"

# 重启本应用
launchctl kickstart -k "gui/$(id -u)/io.readerhub.hub"

# 停止并卸载当前会话中的服务
launchctl bootout "gui/$(id -u)/io.readerhub.hub"

# 查看启动与错误日志
tail -n 50 "$HOME/ReaderHubData/logs/stderr.log"
```

如果系统要求本地网络访问权限，按 macOS 的正常授权提示处理。程序、监听端口和服务标识都应独立于其他应用。

## 安装来源端 CLI

将对应平台的 `readerhub-cli` 复制到来源电脑，在该电脑上执行：

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 ./readerhub-cli "$HOME/.local/bin/readerhub-cli"

"$HOME/.local/bin/readerhub-cli" configure \
  --hub http://192.168.1.10:8848 \
  --name "Linux 工作站"

"$HOME/.local/bin/readerhub-cli" add ./article.md --project 我的项目
```

这里假定复制来的二进制在当前目录。可以把 `~/.local/bin` 加入自己的 PATH，或始终使用完整路径。CLI 不安装服务、不监听目录，也不维护设备心跳。

配置默认保存到 `~/.config/readerhub-cli/config.json`，包含 Hub 地址、设备标识和访问口令。更新二进制时保留此文件，避免同一电脑被识别成新的来源。更多参数见[使用指南](usage.md)。

## 升级

1. 构建新版本，按平台选择产物，并核对文件摘要。
2. 停止 Hub 写入，备份完整数据目录、原程序和 LaunchAgent 配置。
3. 替换 `~/.local/bin/readerhub`，保留原有数据目录与访问口令。
4. 重新 bootstrap 服务，核验 `/health`、文章、图片和想法记录。

若升级失败，保留日志，再恢复对应的旧程序与配置。涉及数据库版本变化时，应恢复匹配的停写备份，而不是只换回旧二进制。

## 备份与恢复

数据目录中的数据库、文章、图片、口令和配置应作为整体备份。使用 LaunchAgent 时，可以先停止服务，再复制完整目录：

```sh
launchctl bootout "gui/$(id -u)/io.readerhub.hub"
mkdir -p "$HOME/ReaderHubBackups"
ditto "$HOME/ReaderHubData" \
  "$HOME/ReaderHubBackups/ReaderHubData-$(date +%Y%m%d-%H%M%S)"
launchctl bootstrap "gui/$(id -u)" \
  "$HOME/Library/LaunchAgents/io.readerhub.hub.plist"
```

恢复同样在停写状态下进行。仅复制文章文件不能恢复完整队列、阅读位置和用户记录；仅复制数据库也不能恢复图片。

## 常见部署问题

| 现象 | 检查方式 |
| --- | --- |
| 本机能读，平板不能访问 | 确认监听的是内网 IP，两台设备处于可互通网络，再检查系统网络权限和防火墙 |
| 提示数据目录已被使用 | 检查是否还有前台实例或另一个服务，避免同目录同时运行 |
| CLI 返回未认证 | 重新核对 Hub 地址与访问口令；不要混用不同实例的配置 |
| 升级后看起来没有文章 | 核对 `--data` 是否仍指向原数据目录 |
| 服务启动失败 | 查看 stderr.log，确认程序架构、监听地址、数据路径和目录权限 |
