# 星枢正式版发布与运维手册

## 产品边界

星枢是完全独立版本。正式服务只依赖自己的镜像、配置、Provider、`STARCORE_DATA_DIR` 和运行态；不读取、挂载、探测、停止或启动历史系统。

`/api/admin/*` 是正式管理接口。`/v2/admin/*` 仅为短期兼容别名，服务会返回弃用标记。`/v1/models` 与 `/v1/chat/completions` 只表示标准协议兼容。

## 发布前检查

```sh
./ops/starcore-preflight.sh
sh -n ops/starcore-preflight.sh
sh -n ops/starcore-watchdog.sh
GOMAXPROCS=1 go test -p 1 -vet=off ./...
node --check scripts/audit-contracts.mjs
```

标准构建环境还必须完成前端测试、类型检查、生产构建和镜像构建。发布镜像必须从 `frontend` 生成 `web-dist`，不得使用归档静态包。

## 数据迁移

历史数据迁移只能离线执行，源目录只读、目标目录必须是新的星枢目录：

```sh
/app/starcore-migrate --source /path/to/historical-data --target /path/to/starcore-data
```

迁移命令拒绝覆盖已有 `catalog-starcore.json`。确认迁移报告、数量和目标文件后，正式服务只读取目标目录。

## 启动与健康

```sh
docker compose up -d --build
curl -fsS http://127.0.0.1:12100/health
curl -fsS http://127.0.0.1:12100/health/ready
```

就绪状态只由星枢自身目录决定。Provider 同步失败、治理不一致和运行态异常在管理总览中分别显示，不调用历史系统健康接口。

## 故障处理

`ops/starcore-watchdog.sh` 只检查星枢健康。达到失败阈值后：

- 正常模式：记录故障并返回失败，交给外部告警/编排系统处理；
- `STARCORE_WATCHDOG_DRY_RUN=1`：只写入故障标记，不重启容器，不切换端口，不操作历史系统。

恢复时先检查星枢自身日志、Provider、数据目录和镜像，再按星枢自身镜像/数据快照恢复。

## 发布验收

- 控制台标题和页面文案只出现“星枢”；
- 总览响应只包含 `health.service`，不包含 `health.v1`/`health.v2`；
- 前端使用 `/api/admin/*`；
- 无历史目录、历史地址和历史容器时仍可启动；
- 数据恢复、健康检查、登录、管理接口契约、手机/桌面页面回归通过；
- 未执行任何历史系统切流操作。
