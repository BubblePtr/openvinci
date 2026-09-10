# 图片模型与画质

Vinci 默认使用 `gpt-image-2.5-flare`。`--model` 可以选择当前上游支持、且 API Key 已开通的官方模型或网关别名；模型名和 `--quality` 等已有选项的取值会原样发送给上游。新增模型名或画质档位无需改动 CLI，但参数传入成功不代表上游一定按要求执行。

首次使用时，Agent 可通过现有 Skill 引导设置默认模型、API Base URL 和 API Key，并保存后续可复用的配置。详见[首次使用指南](first-use.md)。

## GPT Image 2.5

| 模型 ID | 官方定位 |
| --- | --- |
| `gpt-image-2.5-flare` | 快速日常生图 |
| `gpt-image-2.5-sunburst` | 图像生成和精细编辑 |

两者都支持图片生成和编辑，画质选项为 `low`、`medium`、`high`、`xhigh`、`max`、`auto`。其中 `xhigh`、`max` 是 2.5 新增的档位，不能据此假定旧模型或所有网关都支持。Vinci 的默认画质仍为 `auto`。

```sh
vinci "白色背景上的产品海报" \
  --model gpt-image-2.5-flare --quality xhigh -o poster.png --json

vinci --image input.png "调整光照，保留人物细节" \
  --model gpt-image-2.5-sunburst --quality max -o edited.png --json
```

官方文档：[Flare](https://developers.openai.com/api/docs/models/gpt-image-2.5-flare)、[Sunburst](https://developers.openai.com/api/docs/models/gpt-image-2.5-sunburst)、[图片生成与参数说明](https://developers.openai.com/api/docs/guides/image-generation)。

## FlatRouter

先将 FlatRouter 的 API Key 设置到 `OPENAI_API_KEY`，再指定接口地址。Key 所属分组需开启生图权限并包含所选模型，具体要求见 [FlatRouter 生图文档](https://flatrouter.com/docs/images)。

```sh
export OPENAI_BASE_URL="https://api.flatrouter.com/v1"

vinci "白色背景上，左侧一个红色方块，右侧一个蓝色圆形" \
  --model gpt-image-2.5-flare --quality low -o shapes.png --json

vinci --image shapes.png "仅将红色方块改成绿色，保留蓝色圆形" \
  --model gpt-image-2.5-sunburst --quality max -o edited.png --json
```

生成使用 `/v1/images/generations`，编辑使用 `/v1/images/edits` 的 multipart 文件上传协议，无需 APIMart 专用的 Base64 JSON 路径。实际测试发现了尺寸和画质回报差异，见下方记录。

先运行 `vinci --help` 确认安装版本包含 `--image`。`v0.1.0` 尚不支持编辑，可按 [README 的源码编译步骤](../README.zh-CN.md#安装) 构建，并使用 `./vinci` 调用编译后的程序。

### 实测记录（2026-09-10）

使用当前源码 `6dcd47a` 编译的 CLI 和一个已开通两个 2.5 型号的 Key，分别执行生成与编辑。四次均成功写出 PNG；视觉检查确认编辑将红色方块改为绿色，并保留蓝色圆形。

| 请求模型 | 操作 | 请求画质 | CLI 耗时 |
| --- | --- | --- | --- |
| `gpt-image-2.5-flare` | 生成 | `low` | 26.2 秒 |
| `gpt-image-2.5-sunburst` | 生成 | `low` | 25.2 秒 |
| `gpt-image-2.5-flare` | 编辑 | `xhigh` | 26.2 秒 |
| `gpt-image-2.5-sunburst` | 编辑 | `max` | 32.9 秒 |

四次均请求 `size=1024x1024`，实际 PNG 像素尺寸均为 `1536x1024`。两次编辑捕获的上游原始响应还存在以下差异：

| 字段 | Flare 请求 | Sunburst 请求 | 两次上游响应 |
| --- | --- | --- | --- |
| `model` | `gpt-image-2.5-flare` | `gpt-image-2.5-sunburst` | `gpt-image-2-codex` |
| `quality` | `xhigh` | `max` | `auto` |
| `size` | `1024x1024` | `1024x1024` | `1536x1024` |

边界记录确认 CLI 已正确发送上述参数。尺寸差异发生在 CLI 之后；尚未定位是网关转发还是底层模型执行导致。仅凭响应模型标识不能断定发生了降级，也不能凭调用成功确认两个 2.5 型号及 `xhigh/max` 实际生效。这份记录反映的是当日所测 Key 与上游链路，不代表其他分组或后续版本的行为。

## 如何解读 CLI 输出

`--json` 的成功结果包含 `path`、`size`、`format`、`model`、`duration_ms`：

- `model` 是请求的模型名或网关别名。CLI 没有输出上游返回的模型标识。
- `size` 优先采用上游回报值，缺失时回退到请求值；两者都没有具体尺寸时为 `auto`。CLI 不读取图片像素来验证尺寸。
- 上游回报的 `quality` 不包含在 CLI JSON 中。

需要精确尺寸时检查输出文件的像素尺寸；需要核实实际模型或画质时，应查看上游原始响应和服务端记录。Vinci 负责传参和保存图片，当前输出契约不提供这些额外验证。
