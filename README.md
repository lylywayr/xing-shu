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

## 客户端 API 接入

登录控制台后打开“API 接入”页面，可查看 OpenAI-compatible Base URL、模型接口、聊天接口与 SDK 示例，并创建独立客户端 API Key。

- 客户端 Key 与 `ADMIN_API_KEY` 完全隔离；管理员密钥不得分发给调用方。
- 完整客户端 Key 仅创建或轮换时显示一次，服务端只持久化 SHA-256 哈希和安全前缀。
- 新安装和升级默认使用 `optional` 兼容迁移模式；无 Key 的存量 `/v1` 调用暂时可继续工作。
- 客户端迁移完成后，应在前端切换为 `required`；`/v1/models` 需要 `models:read`，`/v1/chat/completions` 需要 `chat:write`。
- 经反向代理访问时可配置 `XING_SHU_PUBLIC_BASE_URL=https://router.example.com`，确保控制台显示正确公网地址。

```sh
curl "$XING_SHU_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $XING_SHU_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","messages":[{"role":"user","content":"hello"}]}'
```

## 可靠路由与故障切换

`model=auto` 仅从已接入、完成能力证据且获 Auto 批准的模型构建候选。排序综合基础分、能力匹配、可信路由知识、历史成功率与延迟、额度和冷却状态；请求最多尝试 3 个不同候选，并避免在同一故障 Provider 上重复请求。

网络错误、超时、429、可重试 5xx、401/403 和协议错误按类型隔离或冷却；冷却结束后必须通过最小恢复探针才能重新进入候选池。SSE 只允许在首个有效事件输出前切换，上游已经向客户端输出后不会拼接其他模型。

尝试链、错误类型、HTTP 状态、TTFB、总延迟、切换次数和客户端 Key 前缀进入只读审计；不会记录完整客户端 Key。

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
- `XING_SHU_CREDENTIAL_KEY`：动态 Provider API Key 的 AES-GCM 主密钥，必须是 Base64 编码的 32 字节随机值；丢失后动态凭证不可恢复。
- `XING_SHU_PUBLIC_BASE_URL`：可选的公开根地址，仅用于控制台生成客户端调用地址和示例。
Provider 密钥仍通过部署环境或加密注册表保存；客户端 API Key 由控制台创建，服务端只保存哈希，不保存可回读明文。星枢不会在公开 API、审计或日志中返回完整密钥。

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

### 控制台布局

控制台采用移动优先的 New API 式后台信息架构，但不复用其品牌素材：

- 手机与平板使用固定应用顶栏和分组导航抽屉，触控目标不低于 40px；
- 390px 下数据表自动转为卡片列表，输入控件保持 16px，避免 iOS 聚焦缩放；
- 1024px 以下内容全宽，指标保持合理分栏；
- 桌面使用固定分组侧栏和独立内容区；
- 所有断点禁止横向溢出，功能与 `/api/admin/*` 契约保持不变。

### 通用 Provider 接入

资源池支持从手机或桌面直接接入 OpenAI-compatible Provider：

1. 填写名称、Base URL 和 API Key；内部 Provider ID 由星枢自动生成；
2. 星枢规范化 Base URL，并真实验证网络、鉴权、`/models` 与 `/chat/completions`；
3. 四步全部通过后，API Key 使用 `XING_SHU_CREDENTIAL_KEY` 做 AES-GCM 加密并写入 `providers-xing-shu.json`；
4. 保存后立即同步模型；可继续编辑、重新验证、停用、启用或删除。

模型接入采用明确的两级策略：

- **FreeLLMAPI**：发现的模型全部自动进入可路由模型池；FreeLLMAPI 的独立生产路由开关仍然是最终调用闸门；
- **其他 Provider 的第一层目录接入**：同步后保持 `unknown`、不可路由；在资源池点击“同步模型”后选择模型，所选模型变为 `active`，可被能力验证，但仍不会进入 Auto；取消选择恢复 `unknown`。
- **其他 Provider 的第二层 Auto 批准**：能力治理会对已接入模型实际验证结构化输出、Tools 和 Vision 协议，保存带时间与来源的证据。三项验证均完成后，用户可单个或批量批准模型进入 Auto；撤销批准立即退出 Auto。同步和重启不会绕过任一层选择。

教师模型还要求已完成结构化输出协议验证且已获第二层 Auto 批准。

环境变量 Provider 在控制台中标记为只读。停用或删除动态 Provider 后，历史模型保留为 `stale/orphaned` 且立即退出 Auto。FreeLLMAPI 仍是独立模块，不纳入通用注册中心。

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
