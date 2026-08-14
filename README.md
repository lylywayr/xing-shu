# 星枢（Xing Shu）

星枢是一个独立的大模型智能路由与治理控制面。它把多个可验证的 Provider 统一为 OpenAI-compatible 接口，维护模型能力目录，并根据能力、治理规则、运营状态和历史证据进行候选排序、失败切换与冷却恢复。

## 产品边界

星枢只依赖自己的镜像、配置和 `XING_SHU_DATA_DIR`。它不读取历史项目目录、不挂载历史数据库、不探测或切换其他服务，也不把平台账号、签到、余额管理或密钥托管作为核心能力。

- `/v1/models`、`/v1/chat/completions`：标准协议兼容入口。
- `/api/admin/*`：唯一管理接口。
- FreeLLMAPI：唯一保留的可选主动适配；授权前不探测、不读取本地数据库、不参与调用。
- 其他 Provider：只按通用 OpenAI-compatible `/models` 和 `/chat/completions` 接入。

## 主要能力

- Provider 模型目录同步与能力证据
- 能力筛选、治理 allow/deny、快照与撤销
- 多候选排序、失败切换、Provider 冷却与恢复
- 请求级 Explain、Knowledge 证据和 Shadow/复盘闭环
- 额度事实与告警（未知或未授权状态保持 `unavailable`，不伪造数字）
- 审计、运行态观测和响应式管理控制台

## 快速启动

```sh
cp .env.example .env
# 编辑 .env，至少设置 ADMIN_API_KEY；Provider 按需配置
mkdir -p data
docker compose up -d --build
curl -fsS http://127.0.0.1:12100/health
curl -fsS http://127.0.0.1:12100/health/ready
```

打开 `http://127.0.0.1:12100/` 进入控制台。生产环境请使用随机长管理员密钥，不要把 `.env`、`data/` 或任何 Provider Key 提交到 Git。

## 配置

复制 `.env.example` 后配置：

- `ADMIN_API_KEY`、`XING_SHU_ADMIN_USER`、`XING_SHU_ADMIN_PASSWORD`：管理认证。
- `PROVIDER_A_URL/KEY`、`PROVIDER_B_URL/KEY`、`PROVIDER_C_URL/KEY`：通用 Provider 槽位。URL 应指向兼容 API 的 `/v1` 基地址。
- `FREELLMAPI_URL/KEY`：可选 FreeLLMAPI 集成；需要在控制台显式授权。
- `XING_SHU_DATA_PATH`：宿主机唯一持久化目录，默认 `./data`。
所有密钥只通过部署环境注入。星枢不会在公开 API、审计或日志中返回完整密钥。

## 开发与验证

后端：

```sh
GOMAXPROCS=1 go test -p 1 -vet=off ./...
```

前端（在 `frontend/`）：

```sh
npm ci
npm run test
npm run typecheck
npm run build
```

运维脚本：

```sh
sh -n ops/xing-shu-preflight.sh
sh -n ops/xing-shu-watchdog.sh
sh -n scripts/browser-smoke.sh
node --check scripts/audit-contracts.mjs
```

Dockerfile 使用标准 Node/Go 多阶段构建；iSH ARM64 上的本地 Vitest/esbuild 可能受原生执行权限影响，发布门禁应使用 Docker 构建器完成。

## 发布与恢复

发布前请先执行敏感信息扫描、测试门禁、Compose 配置检查和数据目录备份。正式发布只包含源码、文档和锁文件，不包含 `.env`、`data/`、数据库、运行日志、归档包或 NAS 运行态。

NAS 或其他主机恢复时：

1. 备份现有项目目录和 `data/`，保留时间戳和回滚路径。
2. 从正式 Git tag 获取源码；在部署机本地生成 `.env`，不要提交。
3. 执行 `docker compose config`，确认只挂载星枢自己的数据目录。
4. `docker compose up -d --build --force-recreate`。
5. 验证容器 healthy、`/health`、`/health/ready`、登录和核心管理接口。
6. 异常时停止新容器并从备份恢复；不要修改无关服务。

详细清单见 [`docs/xing-shu-release-runbook.md`](docs/xing-shu-release-runbook.md) 和 [`docs/ai-handoff.md`](docs/ai-handoff.md)。

## 已知限制

- 能力未知不会被推断为支持；`semantic_match` 是确定性 token 重叠比较，不等价于 LLM 语义裁判。
- Tools 目前是 Schema/调用结构校验，不执行真实外部工具。
- 额度依赖上游可验证字段；401、接口不存在或字段不足时保持 unavailable。
- FreeLLMAPI 的本地额度读取为只读、可选和显式授权，不读取下游 Key、Cookie、Prompt 或 Response。
- iSH ARM64 可能无法提供本地前端原生测试证据，应以标准 Docker 构建结果为准。

## 许可证与来源

本仓库的许可证和第三方来源声明见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。发布前请确认所有新加入代码和依赖的许可证边界。
