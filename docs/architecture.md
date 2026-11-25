# ReaderHub 第一版架构与接口

来源电脑通过 `readerhub-cli` 在本地读取文章及图片；`bundle` 基于 Markdown AST 区分真实图片、引用图片与代码示例，通过 `client` 主动上传。来源端无守护进程、设备发现或在线检测。

Hub 接收完整清单和资源，逐一验证大小、摘要和媒体类型，在独立 incoming 目录准备文章包，再提交到 SQLite。来源设备、路径、文本与资源共同形成去重指纹。重复或并发提交同一版本不会创建多个条目，变化的内容形成新版本，不覆盖已有阅读材料。

## 存储

```text
ReaderHubData/
  .lock                      单实例进程锁
  access-token               单用户访问口令，0600
  hub.db                     SQLite 状态、搜索文本和想法
  incoming/                  上传准备目录
  articles/<id>/
    source.md                提交的原始文本
    article.md               图像引用本地化的可迁移文本
    metadata.json            来源、标题、标签、状态及资源清单
    assets/<sha256>.<ext>    图片
  trash/                     删除提交期间使用的中间目录
  logs/                      常驻服务日志
```

`source.md` 与 `article.md` 是同一材料的两种文本表示，共用一套图片。正文中的代码示例和普通文本不会因为图片路径重写而改变。未收藏与收藏不创建两套文章包；`queued` 和 `saved` 分别表示留存关系，`reading_state` 独立表示用户手动状态。

数据库是可变元数据及用户记录的权威来源，metadata.json 是随文章迁移的镜像。启动时恢复已落盘但未登记的提交，撤回未提交完成的删除，清理已提交删除的中间文件，并同步镜像。源文章和资源仍需完整备份；全文索引不等同于图片备份。

源文档的正文版本保持不变，摘录随文章绑定。修改标题和标签只调整 Hub 展示，不写回来源项目。真正永久移除文章时，其未被其他材料使用的记录也随文章删除。

## HTTP API

除健康检查、登录页面与登录接口外，文章 API 使用 Bearer 口令或 HttpOnly 会话 cookie 认证。网页不将口令保存在 localStorage；未保存的想法草稿保存在当前浏览器。跨来源写请求被拒绝，正文按不可信内容清洗，图片由 Hub 的受认证接口提供。

| 接口 | 功能 |
| --- | --- |
| GET /health | 进程就绪、版本与 AI 禁用状态 |
| POST /api/login | 验证访问口令并设置浏览器会话 |
| POST /api/logout | 清除浏览器会话 |
| GET /api/session | 会话及功能状态 |
| POST /api/submissions | multipart：manifest JSON + 以资源 ID 命名的图片文件 |
| GET /api/items?scope=queue 或 library&q=关键词 | 浏览与搜索正文、元数据、用户想法 |
| GET /api/items/{id} | 元数据、原文和想法 |
| PATCH /api/items/{id} | 标题、标签、手动状态、阅读位置或收藏关系 |
| DELETE /api/items/{id}?scope=queue 或 library | 移除一个留存关系 |
| GET /api/items/{id}/assets/{asset} | 本文清单中的图片 |
| GET /api/items/{id}/download | 自包含 Markdown、图片、来源与想法 ZIP |
| POST /api/items/{id}/notes | 保存摘录及想法 |
| PATCH /api/items/{id}/notes/{note} | 编辑已有想法，保留原记录 ID |
| DELETE /api/items/{id}/notes/{note} | 删除指定想法 |
| POST /api/items/{id}/research | 返回 501，AI 尚未实现 |

单次上传 64 MiB，单张图片 24 MiB，Markdown 4 MiB；这些是第一版的接收上限，不包含公网配额或运营机制。CLI 和服务端都进行校验，上传失败不展示半篇文章。

## 运行

Hub 使用 `readerhub serve --listen <地址:端口> --data <绝对路径>`，启动后保持数据目录独占锁，收到退出信号后关闭 HTTP 服务及 SQLite。网页构建产物嵌入同一个二进制；目标 Mac 不需要 Node.js。

`scripts/render-launchagent.py` 仅生成用户 LaunchAgent plist，不安装或启动服务。实际安装、更新与运行验证见[部署与维护](deployment.md)。真实主机和运行日志分别放在忽略的 `deploy/hosts.local.json` 与 `.local/deploy/`，不写入通用文档。

第一版为家庭内网单用户。PDF、Windows CLI、AI 分析、公网与多用户流程均未实现。
