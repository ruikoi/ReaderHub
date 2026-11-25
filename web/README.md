# ReaderHub Web

TypeScript/React 阅读界面，服务于 iPad、安卓平板和电脑浏览器。`app/` 组装页面，`features/` 放待读与阅读功能，`components/` 放共享组件，`lib/` 放 API 封装，`styles/` 放阅读和响应式样式。

```sh
npm --prefix web ci
npm --prefix web run build
```

构建输出到 dist，由 Go embed 打包进 Hub，目标设备无需单独运行前端服务。

本地开发与浏览器测试见[开发文档](../docs/development.md)。
