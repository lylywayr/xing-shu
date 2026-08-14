# 星枢正式版发布与运维手册

## 产品边界

星枢是唯一产品与运行服务。正式服务只依赖自己的镜像、配置、Provider、`XING_SHU_DATA_DIR` 和运行态。

`/api/admin/*` 是唯一管理接口。`/v1/models` 与 `/v1/chat/completions` 只表示 OpenAI-compatible 标准协议。

## 发布前检查

```sh
./ops/xing-shu-preflight.sh
sh -n ops/xing-shu-preflight.sh
sh -n ops/xing-shu-watchdog.sh
GOMAXPROCS=1 go test -p 1 -vet=off ./...
node --check scripts/audit-contracts.mjs
```

标准构建环境还必须完成前端测试、类型检查、生产构建和镜像构建。发布镜像必须从 `frontend` 生成 `web-dist`，不得使用归档静态包。

## 启动与健康

```sh
docker compose up -d --build
curl -fsS http://127.0.0.1:12100/health
curl -fsS http://127.0.0.1:12100/health/ready
```

就绪状态只由星枢自身目录决定。Provider 同步失败、治理不一致和运行态异常在管理总览中分别显示，不调用历史系统健康接口。

## 故障处理

`ops/xing-shu-watchdog.sh` 只检查星枢健康。达到失败阈值后：

- 正常模式：记录故障并返回失败，交给外部告警/编排系统处理；
- `XING_SHU_WATCHDOG_DRY_RUN=1`：只写入故障标记，不重启容器，不切换端口，不操作历史系统。

恢复时先检查星枢自身日志、Provider、数据目录和镜像，再按星枢自身镜像/数据快照恢复。

## 发布验收

- 控制台标题和页面文案只出现“星枢”；
- 总览响应只包含 `health.service`，不包含 `health.v1`/`health.v2`；
- 前端使用 `/api/admin/*`；
- 只存在星枢容器、项目目录、镜像和数据；
- 数据恢复、健康检查、登录、管理接口契约、手机/桌面页面回归通过。
