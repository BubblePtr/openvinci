# 兼容同步出图与异步轮询两种上游信封

OpenAI Images API 在同一次 HTTP 响应里返回 `b64_json`（或 `url`）。部分兼容网关（如 APIMart 的 `gpt-image-2` / `gpt-image-2-official`）虽然路径仍是 `/v1/images/generations`，但只返回 `task_id`，需要再 `GET /v1/tasks/{id}` 轮询，完成后用 URL 取图。

Generation 仍是一次 CLI 调用、一张本地图片。轮询是等待同一张图就绪，不是失败后的自动重试；`--timeout` 覆盖提交、轮询和下载整段。不引入第二种命令、不引入 Agent 可见的任务 ID。

## Consequences

- 对 OpenAI 官方路径行为不变，现有离线测试仍只打一次 POST。
- 异步网关会打出多次 HTTP：提交、状态、下载；失败仍不自动重试。
- 不在本地复刻各网关的额外字段（如 `resolution`）；模型名继续由 `--model` 透传。
