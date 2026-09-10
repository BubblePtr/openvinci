# 图片编辑

默认情况下，传入 `--image` 时，Vinci 使用 `multipart/form-data` 调用 `/v1/images/edits`；未传入时继续调用图片生成接口。使用现有 `OPENAI_API_KEY`、`OPENAI_BASE_URL` 和默认模型，也可通过 `--model` 指定上游支持的模型。

```sh
vinci --image input.png "将背景改成日落海滩" -o edited.png --json
vinci --image subject.png --image reference.jpg "参考第二张图的配色修改第一张图" -o edited.webp
vinci --image input.png --mask mask.png "在遮罩区域加入一束花" -o edited.png
printf '将画面改成水彩风格' | vinci --image input.png -o edited.png
```

`--image` 可重复，按传入顺序上传本地文件；`--mask` 需要同时传入 `--image`，多图时用于第一张图。遮罩使用 PNG，透明区域指示编辑区域，尺寸应与第一张输入图一致。具体格式、大小、数量及模型支持由上游校验。

编辑支持现有 `--size`、`--quality`、`--background`、`--format`、`--moderation`、`--timeout` 和 `--json`。一次调用输出一张图片，支持即时 base64、图片 URL 及兼容网关的异步任务返回。输入文件在内存中组装为上传请求，适合常规图片文件。

使用 GPT Image 2.5 时，可通过 `--model` 选择 `gpt-image-2.5-flare` 或 `gpt-image-2.5-sunburst`。两个型号的画质选项、FlatRouter 调用示例和实测限制见[模型与画质指南](image-models.md)。

空路径或单独传入遮罩返回退出码 `2`、`usage_error`；本地文件读取失败返回退出码 `4`、`read_failed`，不会提交上游请求。成功输出、上游错误、输出格式推断和覆盖行为与图片生成一致。需要保留原图时，请使用不同的输出路径。

接口参考：[OpenAI 图片编辑文档](https://developers.openai.com/api/reference/resources/images/methods/edit)。

## APIMart

当 `OPENAI_BASE_URL` 的主机名为 `api.apimart.ai`，且模型名以 `gpt-image-` 开头时，CLI 自动使用 APIMart 的 `/v1/images/generations` JSON 协议，将本地图片编码为 Base64 Data URL，分别传入 `image_urls` 和可选的 `mask_url`。无需图床或手动上传。其他上游及 APIMart 的非 GPT 图片模型保持默认文件上传方式。

```sh
set -a
source ~/.config/openvinci/env
set +a
vinci --image input.png "将背景改成蓝色" \
  --model gpt-image-2-official -o edited.png --json
vinci --image input.png --mask mask.png "修改遮罩区域" \
  --model gpt-image-2-official -o masked.png --json
```

CLI 不会自动读取该配置文件，也不会自动切换模型；请显式传入 `--model`。上游 Key 必须具备相应模型权限。

APIMart 仍然复用现有异步轮询、结果下载和错误输出。不会在失败后自动切换协议再次提交，避免重复计费。

### 实测记录（2026-09-08）

使用 `gpt-image-2-official`，CLI 将本地图片与 PNG 遮罩都编码为 Base64，完整跑通提交、轮询和下载，约 45 秒输出 1024×1024 PNG。目标红色方块变为绿色，但模型增加了黑色描边；接口接受遮罩不代表输出能严格保持遮罩外像素不变。

此前独立请求已验证不带遮罩的 Base64 编辑成功。普通 `gpt-image-2` 在当前 Key 下返回模型权限错误，不能将其视为该模型不支持编辑。
