# 第三方声明

星枢包含或依赖以下开源组件。发布前应根据锁定版本核对上游许可证文本，并在需要时补充完整许可证副本。

## Go 依赖

- `modernc.org/sqlite` 及其依赖：用于纯 Go SQLite 存储实现。
- `github.com/google/uuid`：UUID 生成。
- `github.com/dustin/go-humanize`：人类可读格式化。
- `github.com/mattn/go-isatty`：终端能力检测。
- `github.com/ncruces/go-strftime`：时间格式化。
- `golang.org/x/exp`、`golang.org/x/sys`：Go 扩展库。

精确版本由 `go.mod` 和 `go.sum` 锁定；Docker 构建通过 `go mod download` 获取依赖。发布镜像必须保留并遵循各上游组件的许可证要求。

## 前端依赖

- Vue
- Vite
- TypeScript
- vue-tsc
- Vitest
- jsdom
- `@vitejs/plugin-vue`
- `@babel/types`
- `@types/node`

精确版本由 `frontend/package.json` 和 `frontend/package-lock.json` 锁定。发布二进制或镜像时，前端依赖仅用于构建控制台静态资源。

## 代码来源与许可证边界

星枢是独立维护的路由与治理产品。任何参考实现、外部服务或部署环境都不是运行依赖；本仓库不应包含外部服务的密钥、Cookie、数据库、运行态或未确认许可证的复制代码。

仓库许可证在许可证边界完成最终法务确认后补充。若新增第三方代码，请在本文件添加来源、固定版本/commit、许可证和本地修改说明，并将许可证全文放入合适的 `NOTICE`/`LICENSES` 目录。
