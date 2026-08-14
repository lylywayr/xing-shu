# 星枢 AI 交接文档

本文面向继续维护、审查、发布或恢复星枢的 AI/工程师。

## 1. 当前产品定义

星枢是唯一产品与运行服务。运行时只读取 `XING_SHU_DATA_DIR`（容器内默认 `/data`）和部署环境变量。`/v1/models` 与 `/v1/chat/completions` 仅是 OpenAI-compatible 标准协议路径。

核心边界：FreeLLMAPI 保留为唯一主动适配的可选外部集成；其他 Provider 只能使用 OpenAI-compatible `/models` 和 `/chat/completions`。星枢负责目录、能力证据、治理、选路、失败切换、冷却、审计和学习闭环，不负责平台账号、签到、Key 自动获取或外部平台运营。

## 2. 代码地图

- `cmd/router`：服务启动、配置装配、健康与协议入口。
- `internal/catalog`：模型目录、同步、生命周期。
- `internal/provider`：通用 Provider HTTP 客户端与错误分类。
- `internal/integration`：FreeLLMAPI 状态、模型同步和只读本地额度。
- `internal/routing`：候选排序、会话亲和、失败切换和 Provider gate。
- `internal/governance`：allow/deny、快照、审计和撤销。
- `internal/api`：管理 API、协议转发、观测与复盘接口。
- `frontend`：Vue 控制台源码。应用壳采用移动优先布局：手机/平板为顶栏与分组抽屉，桌面为固定分组侧栏；列表在手机端卡片化。
- `frontend/src/styles/workspace.css`：New API 风格的浅色实体面板、响应式断点与移动触控规范；作为 `app.css` 后加载的重构层。
- `ops`、`scripts`：发布前置、watchdog、契约审计和浏览器冒烟。

## 3. 数据与安全

公开仓库不得包含 `.env`、`data/`、数据库、JSONL 运行日志、Cookie、完整 API Key、管理员密码、NAS 地址或备份路径。`.gitignore` 与 `.dockerignore` 是第一道门禁，发布前还要执行 `git grep` 和正则扫描。

唯一正式持久化挂载是宿主机配置的 `XING_SHU_DATA_PATH:/data`。FreeLLMAPI 数据卷不属于默认公开 Compose；如确需使用集成，必须由部署者明确配置只读挂载、显式授权并确认风险。

## 4. 本地验证顺序

1. `GOMAXPROCS=1 go test -p 1 -vet=off ./...`
2. `cd frontend && npm ci && npm run test && npm run typecheck && npm run build`
3. `node --check scripts/audit-contracts.mjs`
4. `sh -n ops/xing-shu-preflight.sh ops/xing-shu-watchdog.sh scripts/browser-smoke.sh`
5. `docker compose config`
6. 生成镜像后验证 `/health`、`/health/ready`、认证、`/api/admin/*`、`/v1/models` 和最小聊天契约。

构建前测试全部通过后再生成正式镜像。iSH ARM64 的 esbuild/Vitest 原生执行失败不能被当成源码失败，也不能被包装成测试通过；使用标准 Docker 构建器补证据。

## 5. NAS 恢复流程

以下是通用流程，不写入任何真实主机地址或凭证：

```sh
# 在部署主机执行
stamp=$(date +%Y%m%d-%H%M%S)
cp -a "$PROJECT" "$PROJECT.backup-$stamp"
cp -a "$PROJECT/data" "$PROJECT.backup-$stamp/data"
git clone --branch <release-tag> <repository-url> "$PROJECT.new"
cp "$PROJECT/.env" "$PROJECT.new/.env"
cd "$PROJECT.new"
docker compose config
docker compose up -d --build --force-recreate
curl -fsS http://127.0.0.1:<port>/health
curl -fsS http://127.0.0.1:<port>/health/ready
```

实际执行时必须通过 Minis 全局环境变量引用 SSH 密码和 GitHub 凭证，禁止 echo/cat 变量值。恢复前确认旧项目与 `data` 备份成功；恢复后保留旧目录，直到健康、登录、管理接口和关键页面验收结束。

## 6. GitHub 发布流程

- 分支：`main`。
- 当前正式发布线从 `v0.1.0` 起；前端移动优先重构版本为 `v0.3.0`。
- 发布前检查 staged diff、`git grep`、`.env`/`data` 是否被忽略、Compose 是否只挂载 `/data`。
- 使用 `GH_TOKEN` 调 GitHub API 创建公开仓库、配置 origin、push main、创建 tag 和 Release；绝不输出 token。
- Release notes 应说明：独立产品边界、Docker 启动、通用 Provider、FreeLLMAPI 可选适配、路由能力、测试限制和已知限制。

## 7. 回滚原则

回滚只针对星枢自身镜像、源码和数据备份。停止新容器前记录镜像、配置摘要和健康状态；恢复备份后重新执行 `/health`、`/health/ready` 和管理接口验收。不得通过 watchdog 切换历史服务，不得修改无关容器或数据卷。

## 8. 继续任务模板

接手后先检查：

```sh
pwd
find . -maxdepth 2 -type f | sort
git status --short
git log --oneline -5
```

然后按“验证 → 安全扫描 → 提交 → GitHub 发布 → 备份 → NAS 部署 → 健康验收 → 回写发布记录”的顺序推进。发现凭证、内网地址或运行态时，立即从 staged 内容和公开文件中移除，并重新扫描；不要把秘密写入记忆、交接文档或 Release notes。
