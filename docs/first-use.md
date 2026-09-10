# 首次使用与配置复用

Vinci 已有一个供 Agent 使用的 [SKILL.md](../SKILL.md)，负责安装、首次配置和图片调用。可以把下面这句话交给 Agent：

```text
读取 https://raw.githubusercontent.com/BubblePtr/openvinci/main/SKILL.md，帮我安装并配置 Vinci。缺少配置时，集中询问默认模型、API Base URL 和 API Key，配置好后供后续生图使用。
```

## 首次需要哪些信息

| 配置 | 建议值 | 说明 |
| --- | --- | --- |
| 默认模型 | `gpt-image-2.5-flare` | 也可选择当前 Key 已开通的其他官方型号或网关别名。 |
| API Base URL | `https://api.openai.com/v1` | OpenAI 官方地址。使用网关时填写网关给出的地址，例如 `https://api.flatrouter.com/v1`。 |
| API Key | 无默认值 | 必须属于所选服务。可以指定保存 Key 的本地文件或环境变量，也可使用 Agent 提供的安全输入方式。 |

Agent 先检查当前对话、已导出的环境变量和保存的配置，已有的信息不再询问。若信息不完整，将缺少的字段放在同一个问题中，并给出模型、URL 的建议值。全新用户会看到类似的问题：

> Vinci 默认模型用 `gpt-image-2.5-flare` 可以吗？API Base URL 用 OpenAI 官方地址 `https://api.openai.com/v1`，还是你的网关地址？API Key 可以从哪个本地文件或环境变量读取？配置将保存到 `~/.config/openvinci/env`，之后复用；如果只用于当前会话，请一起说明。

若设置了 `XDG_CONFIG_HOME`，问题中的路径应改为该目录下的 `openvinci/env`。已经给出的模型、URL 或 Key 不应再次询问；完整配置可以直接进入原来的生图或编辑任务。

## 配置保存在哪里

默认路径为 `~/.config/openvinci/env`；设置了 `XDG_CONFIG_HOME` 时，使用 `$XDG_CONFIG_HOME/openvinci/env`。文件内容如下，示例 Key 仅为占位符：

```sh
export OPENAI_API_KEY='YOUR_API_KEY'
export OPENAI_BASE_URL='https://api.openai.com/v1'
export VINCI_MODEL='gpt-image-2.5-flare'
```

Agent 在首次收集配置时说明保存路径，收到用户选择后写入。目录权限为 `700`，文件权限为 `600`；创建前设置 `umask 077`。值应按 shell 字符串正确引用，例如使用 Python 的 `shlex.quote`，更新时保留文件中不相关的设置。Key 只存入这个用户配置文件，不写进项目、回复或记忆。

选择“仅当前会话”时，Agent 只在本次调用进程中设置环境变量，不保存文件。用户只要求配置工具时，配置检查后即可结束；用户原本要求生图时，则继续完成原来的请求，无需另做一轮付费测试。

## 后续如何复用

Agent 在每次调用所用的同一个 shell 中加载配置，再传入选定模型：

```sh
(
  set -a
  . "${XDG_CONFIG_HOME:-$HOME/.config}/openvinci/env" || exit
  set +a
  vinci "一座海边灯塔的水彩插画" --model "$VINCI_MODEL" -o lighthouse.png --json
)
```

不能假定前一次工具调用导出的环境变量会保留到下一次。CLI 仍只从环境变量读取 `OPENAI_API_KEY`、`OPENAI_BASE_URL`；配置文件和 `VINCI_MODEL` 由 Agent 加载，后者通过 `--model` 传入。手动调用 CLI 时，不传 `--model` 使用内置默认值 `gpt-image-2.5-flare`。

本次请求明确指定的配置优先，其次是当前环境，最后是保存的配置。使用前两者时，应保留这些值，避免加载旧文件后覆盖它们。临时换一次模型只调整该次的 `--model`，不改保存的默认值。切换服务地址时，需要确认使用的是该服务的 Key。

首次配置完成后，Agent 只报告选定模型、Base URL、保存路径或会话范围，以及“Key 已配置”，不回显 Key。模型的功能差异、画质档位及网关实测限制见[模型与画质指南](image-models.md)。
