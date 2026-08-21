# OpenVinci

面向 AI Agent 的极简图片生成 CLI：Agent 通过一条 shell 命令（`vinci`）调用上游图片模型生成本地图片文件，无需 SDK、MCP 或常驻服务。定位一句话：**OpenVinci — visual tools for agents.**

## Language

**Agent（调用方）**:
通过 shell 调用 `vinci` 的 AI 编码代理（Claude Code、Codex、Cursor 等）。本项目的第一用户，人类用户是次要角色。
_Avoid_: 用户、client

**Generation（生成）**:
一次从 Prompt 到本地图片文件的完整调用，恰好对应一次上游 API 请求——不含自动重试、不含批量。
_Avoid_: 任务、job、batch

**Prompt**:
描述目标图片的自然语言文本，经位置参数或 stdin 传入。
_Avoid_: 提示语、query

**Upstream（上游）**:
实际执行图片生成的 OpenAI 兼容 API 服务，可以是 OpenAI 官方或任意兼容网关（经 `OPENAI_BASE_URL` 指定）。
_Avoid_: 后端、服务端

**Output Path（输出路径）**:
生成图片最终写入的本地文件路径；成功时以绝对路径形式返回给 Agent，是普通模式下唯一的 stdout 输出。
_Avoid_: 保存位置、目标文件

**Slug Filename（slug 文件名）**:
未指定输出路径时，由 Prompt 派生的默认文件名（写入当前目录）。
_Avoid_: 随机文件名、临时文件名

**JSON Mode（JSON 模式）**:
`--json` 开启的机器可解析输出契约：成功输出结果对象，失败输出错误对象，供 Agent 稳定解析。
_Avoid_: api 模式

**Error Contract（错误契约）**:
失败路径的稳定约定：分层退出码、stderr 人类可读信息、JSON 模式下的结构化错误对象。与成功路径同等级别的公开接口。
_Avoid_: 报错处理

**Moderation Block（审核拦截）**:
上游安全系统拒绝 Prompt 导致的生成失败，含误伤（过度拒绝）情形；是错误契约中的一等错误类别，Agent 可据此调整审核敏感度后重试。
_Avoid_: 违规、封禁
