# 第三阶段进度

- 2026-08-15：用户提出前端缺少 API 地址展示和密钥生成；已纳入第三阶段正式交付。
- 已确认不能暴露 `ADMIN_API_KEY`，需要独立客户端 API Key 体系和 `/v1/*` 鉴权。
- 2026-08-15：用户授权开始完整第三阶段；按 L3 流程在 `feat/phase3-reliable-routing` 独立 worktree 实施。
- 当前状态：`deployed`；第三阶段已提交、合并、部署到 NAS 生产并完成健康与契约验收。
- 后端 Go 全量测试、NAS 标准前端 17 个测试文件/41 项测试、TypeScript 类型检查、32 项管理 API 契约、客户端 Key 生命周期、Chromium 手机/桌面无溢出验收均通过。
- 生产镜像：`xing-shu:v0.8.0`，摘要 `sha256:40f210fccc436ea4df551eebd2b298a06f52de115de2574f9af337afdfa0a96b`；容器 healthy，287 models，`/health/ready` 返回 ready=true。
- 生产回滚备份已保存，权限 600；临时候选容器和构建目录已清理。生产契约审计通过 32 个管理接口，唯一 warning 是未指定真实教师模型而跳过 reviewer 上游探测。
