# OpenVinci

[English](README.md) | [简体中文](README.zh-CN.md)

**OpenVinci — 面向 Agent 的生图工具。**

`vinci` 用一条 shell 命令调用生图接口，为 Agent 提供文生图和本地图片编辑能力。无需 SDK、MCP 或常驻服务。

```bash
vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png
# /home/you/project/assets/hero.png
```

成功时，普通模式仅在 stdout 输出绝对输出路径。`--json` 模式输出机器可解析的结果对象。所有失败均具有稳定的错误码和分层退出码。

## 快速开始

### 针对 Agent

将以下指令发送给 Agent：

```text
读取 https://raw.githubusercontent.com/BubblePtr/openvinci/main/SKILL.md，并按其中说明安装并使用 vinci。
```

Skill 文档是面向 Agent 的标准调用契约。若文档与实际行为有分歧，以 `vinci --help` 实时 CLI 契约为准。探索索引：[llms.txt](https://raw.githubusercontent.com/BubblePtr/openvinci/main/llms.txt)。

首次使用时，Agent 会集中询问默认模型、API Base URL 和 API Key，并复用已提供的信息。推荐模型为 `gpt-image-2.5-flare`；选定的配置会保存到用户配置目录，供后续调用加载。已有配置完整时不再询问，也可以选择仅用于当前会话。详见[首次使用指南](docs/first-use.md)。

### 手动使用

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
export OPENAI_API_KEY="sk-..."

vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png --json
```

若使用兼容网关，命令完全一致，只需指定 base URL 以及模型别名（若 API Key 绑定了特定别名）：

```bash
export OPENAI_BASE_URL="https://api.example.com/v1"
vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png --model gpt-image-2-official --json
```

## 安装

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
```

该脚本会将静态 `vinci` 二进制文件安装至 `~/.local/bin`（可通过 `--dir` 或环境变量 `VINCI_INSTALL_DIR` 自定义安装目录）。支持的平台：macOS arm64、Linux x64/arm64。

安装指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh -s -- --version v0.1.0
```

源码编译（需要 Go 1.24+）：

```bash
go build -o vinci ./cmd/vinci
```

## 配置

CLI 读取两个环境变量。Agent Skill 可以保存、加载用户配置文件，CLI 本身不会自动读取该文件。手动使用时可设置：

```bash
export OPENAI_API_KEY="sk-..."                             # 必填
export OPENAI_BASE_URL="https://api.openai.com"            # 可选；任何 OpenAI 兼容网关
```

| 环境变量 | 是否必填 | 说明 |
| --- | --- | --- |
| `OPENAI_API_KEY` | 是 | API Key。仅从环境变量读取，绝不通过命令行 flag 传入，避免泄漏至 shell 历史记录。 |
| `OPENAI_BASE_URL` | 否 | 任何 OpenAI 兼容网关。默认为 `https://api.openai.com`。若 URL 已以 `/v1` 结尾，会直接原样接受。 |

上游模型默认是 `gpt-image-2.5-flare`。可用 `--model` 选择当前 Key 已开通的官方模型或网关别名。Skill 将用户选定的默认模型保存为 `VINCI_MODEL`，每次调用时通过 `--model` 传入；CLI 本身不会读取 `VINCI_MODEL`。一次调用就是调一次生图接口、写出一张本地图片：上游要么立刻返回图片（`b64_json` 或 `url`），要么返回任务 ID，由 `vinci` 内部轮询到出图。轮询是等同一张图，不是失败后重试。失败不会自动重试，延迟和成本可预期。`--timeout` 覆盖提交、轮询和下载整段。

GPT Image 2.5 有两个型号：`gpt-image-2.5-flare` 适合快速日常生图，`gpt-image-2.5-sunburst` 侧重精细编辑。两者都支持生成和编辑，画质在 `low`、`medium`、`high`、`auto` 之外增加了 `xhigh` 和 `max`。可用性和参数执行情况取决于上游。调用示例、FlatRouter 配置及实测参数差异见[模型与画质指南](docs/image-models.md)。

## 使用方法

```bash
vinci [flags] "<prompt>"
echo "<prompt>" | vinci [flags]
vinci --image input.png [--mask mask.png] "<编辑要求>"
```

Prompt 来源于位置参数；当未提供位置参数时，则从 stdin 读取（便于传递包含引号和换行符的长 Prompt）。位置参数永远优先，此时不会读取 stdin，因此闲置的 pipe 不会阻塞 CLI。两者均未提供则属于用法错误。

当 Prompt 本身以破折号 `-` 开头时，使用 `--` 结束 flag 解析：

```bash
vinci -o hero.png -- "-a prompt starting with a dash"
```

```bash
# 显式指定输出路径；格式会根据文件扩展名自动推断
vinci "a lighthouse in a storm" -o ./assets/lighthouse.jpg

# 未指定 -o：根据 Prompt 派生 slug 文件名并写入当前目录
vinci "a red bicycle"            # -> ./a-red-bicycle.png

# 从 stdin 读取长 Prompt，设置透明背景，输出机器可读的 JSON
cat prompt.txt | vinci --background transparent --json -o ./assets/icon.png
```

已存在的目标文件会被静默覆盖，保证重复执行同一命令具有幂等性（类似 `curl -o`）。

## 图片编辑

```bash
vinci --image input.png "将背景改成日落海滩" -o edited.png --json
vinci --image subject.png --image reference.jpg "参考第二张图的配色修改第一张图" -o edited.webp --json
vinci --image input.png --mask mask.png "在遮罩区域加入花束" -o edited.png --json
```

`--image` 可重复，按顺序传入多张本地图片。`--mask` 必须搭配输入图片：使用带 Alpha 通道的 PNG，尺寸与第一张输入图一致，透明区域表示编辑区域。格式、大小及模型限制由上游校验。需要保留原图时，请使用不同的输出路径。

默认通过 multipart 表单上传文件至 `/v1/images/edits`。当主机名为 `api.apimart.ai` 且模型名以 `gpt-image-` 开头时，CLI 自动转用 `/v1/images/generations`，将本地图片以 Base64 Data URL 写入 `image_urls` 和可选的 `mask_url`，无需图床：

```bash
export OPENAI_BASE_URL="https://api.apimart.ai/v1"
vinci --image input.png "将背景改成蓝色" \
  --model gpt-image-2-official -o edited.png --json
```

Key 必须有对应模型权限。其他主机和非 GPT 模型保持默认上传协议；请求失败后不会切换协议重复提交。详细用法和实测记录见[图片编辑指南](docs/image-editing.md)。

## 参数选项 (Flags)

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `-o`, `--output <path>` | 根据 Prompt 派生的 slug 文件名（当前目录） | 输出路径。缺失的父目录会自动创建。 |
| `--image <path>` | 无 | 本地输入图片，可重复传入多张；启用编辑模式。 |
| `--mask <path>` | 无 | 第一张输入图的 PNG 遮罩，需要搭配 `--image`。 |
| `--model <name>` | `gpt-image-2.5-flare` | 当前 Key 已开通的官方模型或网关别名。原样透传，包括 GPT Image 2.5 型号。 |
| `--size <spec>` | `auto` | 图片尺寸，例如 `1024x1024`, `1536x1024`。直接透传给上游。 |
| `--quality <q>` | `auto` | 渲染画质：`low`、`medium`、`high`、`auto`；GPT Image 2.5 另支持 `xhigh`、`max`，需上游支持。 |
| `--background <b>` | `auto` | 背景，例如 `transparent`, `opaque`。 |
| `--format <f>` | 根据 `--output` 推断，否则为 `png` | 输出格式：`png`, `jpeg` 或 `webp`。与输出路径扩展名冲突时报错。 |
| `--moderation <m>` | `auto` | 审核敏感度，例如 `low`（在 Prompt 被过度拒绝拦截时使用）。 |
| `--timeout <dur>` | `120s` | 生成超时时间，支持秒数（`90`）或时长表达（`90s`, `2m`）。覆盖提交、轮询和下载全过程。 |
| `--json` | 默认关闭 | 成功时在 stdout 输出结果对象，失败时输出错误对象。可通过 `--json=false` 显式禁用。 |
| `-h`, `--help` | | 显示使用帮助。 |
| `--version` | | 显示版本号。 |

模型名和已有渲染选项的取值会直接透传给上游，本地不做重复校验。新增模型名和画质档位无需 CLI 发版即可传入；上游是否接受、是否实际执行，需要分别确认。

## 输出契约

普通模式（成功）：stdout 输出单行绝对输出路径。所有诊断信息输出至 stderr。

JSON 模式（成功）：

```json
{"path":"/home/you/project/assets/hero.png","size":"1024x1024","format":"png","model":"gpt-image-2.5-flare","duration_ms":8421}
```

`size` 为上游回报的图片尺寸；若上游未返回，则使用请求时的设定值（未指定时为 `auto`）。Vinci 不读取图片像素来校验尺寸。`model` 是请求的模型名或网关别名，不能用来确认实际执行的模型；此 JSON 结果未包含上游返回的 `model` 和 `quality` 字段。

JSON 模式（失败）—— 写入 stdout，并返回非零退出码：

```json
{"error":{"code":"moderation_blocked","message":"moderation blocked this prompt: ..."}}
```

## 退出码与错误契约

### 退出码分层

| 退出码 | 含义 |
| --- | --- |
| `0` | 成功 (Success) |
| `1` | 通用失败 (Generic failure) |
| `2` | 用法错误 (Usage error) |
| `3` | 上游 API 错误 (Upstream API error) |
| `4` | 本地 IO 错误 (Local IO error) |

### 错误码定义

| 错误码 (`code`) | 退出码 | 说明与建议处理动作 |
| --- | --- | --- |
| `usage_error` | 2 | 错误参数、缺失 Prompt、多余位置参数或格式冲突。应修正命令，勿原样重试。 |
| `missing_api_key` | 1 | 未设置 `OPENAI_API_KEY`。提示配置环境变量。 |
| `api_error` | 3 | 上游拒绝或执行失败；错误信息包含 HTTP 状态码。报告错误信息，勿盲目重试。 |
| `moderation_blocked` | 3 | 上游安全系统拦截了 Prompt。重写 Prompt，或尝试加 `--moderation low` 重试一次。 |
| `network_error` | 3 | 无法连接上游，或达到 `--timeout` 超时。检查 `OPENAI_BASE_URL` 和网络配置。 |
| `read_failed` | 4 | 输入图片或遮罩无法读取。检查本地路径和读取权限。 |
| `write_failed` | 4 | 图片无法写入指定的输出路径。检查目标路径写权限。 |

错误码属于公开契约的一部分：调用方（Agent）应基于错误码构建重试与错误处理分支，而非基于自然语言报错文本。

## 开发与测试

```bash
go test ./...                                # 完全离线测试（基于模拟上游）
sh install_test.sh                           # 测试安装脚本的平台检测
go vet ./... && gofmt -l .
VINCI_LIVE_TEST=1 go test ./... -run Live    # 可选：连接真实 API 进行冒烟测试
```

发版通过在 `main` 分支打 tag（如 `v0.1.0`）触发；详见 `docs/release.md`。

## 开源协议

Apache-2.0。Copyright 2026 Kieran Zhang。
