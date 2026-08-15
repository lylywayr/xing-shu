# 第二阶段发现

- 当前目录同步已保留 `unknown` 状态，且资源池支持用户选择模型。
- 当前 `SetAllow(true)` 会同时将外部模型变为 `active + auto_routable`，绕过能力验证。
- 当前批量探测只验证 `response_format: json_object`，并只接受已 `active` 模型。
- Tools、Vision 与上下文能力可能来自目录字段，但尚无主动验证证据。
- 当前治理 API 已提供批量、审计和撤销，可扩展为第二层 Auto 批准。
