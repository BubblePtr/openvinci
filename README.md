# OpenVinci

[English](README.md) | [简体中文](README.zh-CN.md)

**OpenVinci — image generation for agents.**

`vinci` calls an image generation API in one shell call, giving agents text-to-image and local image editing. No SDK, no MCP, no daemon.

```bash
vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png
# /home/you/project/assets/hero.png
```

On success, plain mode prints only the absolute output path on stdout. `--json` prints a machine-readable result object. Every failure has a stable error code and a layered exit code.

## Quick start

### For agents

Paste this to the agent:

```text
Read https://raw.githubusercontent.com/BubblePtr/openvinci/main/SKILL.md and follow the instructions to install and use vinci.
```

The skill is the agent-facing contract. `vinci --help` is the live CLI contract if the two ever disagree. Discovery index: [llms.txt](https://raw.githubusercontent.com/BubblePtr/openvinci/main/llms.txt).

### Manual

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
export OPENAI_API_KEY="sk-..."

vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png --json
```

A gateway uses the same command, plus a base URL and a model alias if the key is scoped to one:

```bash
export OPENAI_BASE_URL="https://api.example.com/v1"
vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png --model gpt-image-2-official --json
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
```

That puts a static `vinci` binary in `~/.local/bin` (override with `--dir` or `VINCI_INSTALL_DIR`). Supported platforms: macOS arm64, Linux x64/arm64.

Pin a release:

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh -s -- --version v0.1.0
```

From source (Go 1.24+):

```bash
go build -o vinci ./cmd/vinci
```

## Configuration

Two environment variables, no config file. Copy and fill in:

```bash
export OPENAI_API_KEY="sk-..."                             # required
export OPENAI_BASE_URL="https://api.openai.com"            # optional; any OpenAI-compatible gateway
```

| Variable | Required | Description |
| --- | --- | --- |
| `OPENAI_API_KEY` | yes | The API key. Read from the environment only, never from a flag, so it stays out of shell history. |
| `OPENAI_BASE_URL` | no | Any OpenAI-compatible gateway. Defaults to `https://api.openai.com`. A URL that already ends in `/v1` is accepted as-is. |

The upstream model defaults to `gpt-image-2`. Override it with `--model` for a gateway alias. One call is one image API request and one local file: either the upstream returns the image immediately (`b64_json` or `url`), or it returns a task id and `vinci` polls until the image is ready. Polling is waiting, not a retry. There is no automatic retry of a failed generation, so latency and cost stay predictable. The `--timeout` covers the whole generation, including polls and the image download.

## Usage

```bash
vinci [flags] "<prompt>"
echo "<prompt>" | vinci [flags]
vinci --image input.png [--mask mask.png] "<edit prompt>"
```

The prompt comes from the positional argument, or from stdin when there is no positional argument (handy for long prompts full of quotes and newlines). A positional argument always wins and stdin is then never read, so an idle pipe can never stall the CLI. Passing neither is a usage error.

Use `--` to end flag parsing when the prompt itself starts with a dash:

```bash
vinci -o hero.png -- "-a prompt starting with a dash"
```

```bash
# Explicit output path; the format is inferred from the extension.
vinci "a lighthouse in a storm" -o ./assets/lighthouse.jpg

# No -o: a slug filename derived from the prompt, written to the current directory.
vinci "a red bicycle"            # -> ./a-red-bicycle.png

# Long prompt from stdin, transparent background, machine-readable output.
cat prompt.txt | vinci --background transparent --json -o ./assets/icon.png
```

An existing target file is overwritten silently, so rerunning the same command is idempotent, like `curl -o`.

## Image editing

```bash
vinci --image input.png "Change the background to a sunset beach" -o edited.png --json
vinci --image subject.png --image reference.jpg "Use the second image as a color reference" -o edited.webp --json
vinci --image input.png --mask mask.png "Add flowers in the masked area" -o edited.png --json
```

Repeat `--image` for multiple local inputs, in order. `--mask` requires an input image; use a PNG with an alpha channel and the same dimensions as the first input. Transparent areas indicate where to edit. Format, size and model limits are checked by the upstream. Use a different output path to preserve the original image.

By default, edits upload files to `/v1/images/edits` using multipart form data. For `api.apimart.ai` and model names starting with `gpt-image-`, Vinci instead sends local images as Base64 Data URLs in `image_urls` and optional `mask_url` to `/v1/images/generations`. No image hosting is needed:

```bash
export OPENAI_BASE_URL="https://api.apimart.ai/v1"
vinci --image input.png "Change the background to blue" \
  --model gpt-image-2-official -o edited.png --json
```

The model must be enabled for your API key. Other hosts and non-GPT models retain the default upload protocol. Failed requests are not resubmitted through another protocol. See the [image editing guide](docs/image-editing.md) for details and live verification notes.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-o`, `--output <path>` | slug filename from the prompt, in the current directory | Output path. Missing parent directories are created. |
| `--image <path>` | none | Local input image. Repeat for multiple inputs; enables editing. |
| `--mask <path>` | none | PNG mask for the first input image; requires `--image`. |
| `--model <name>` | `gpt-image-2` | Upstream model. Passed through unchanged, so a gateway alias such as `gpt-image-2-official` works. |
| `--size <spec>` | `auto` | Image size, e.g. `1024x1024`, `1536x1024`. Passed straight through to the upstream. |
| `--quality <q>` | `auto` | Rendering quality, e.g. `low`, `medium`, `high`. |
| `--background <b>` | `auto` | Background, e.g. `transparent`, `opaque`. |
| `--format <f>` | inferred from `--output`, else `png` | `png`, `jpeg` or `webp`. Contradicting the output extension is a usage error. |
| `--moderation <m>` | `auto` | Moderation sensitivity, e.g. `low`, for when a prompt is refused unfairly. |
| `--timeout <dur>` | `120s` | Generation timeout, as seconds (`90`) or a duration (`90s`, `2m`). Covers submit, polling, and download. |
| `--json` | off | Print a result object on success, an error object on failure. `--json=false` explicitly disables it. |
| `-h`, `--help` | | Show usage. |
| `--version` | | Show the version. |

Model rendering options are passed through to the upstream without being re-validated locally, so upstream additions work without a new release.

## Output

Plain mode, success: one line on stdout, the absolute output path. All diagnostics go to stderr.

JSON mode, success:

```json
{"path":"/home/you/project/assets/hero.png","size":"1024x1024","format":"png","model":"gpt-image-2","duration_ms":8421}
```

`size` is the size the upstream reports for the generated image; if the upstream omits it, the requested value is used (`auto` when you did not ask for one).

JSON mode, failure — written to stdout, with a non-zero exit code:

```json
{"error":{"code":"moderation_blocked","message":"moderation blocked this prompt: ..."}}
```

## Exit codes and error codes

| Exit code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Generic failure |
| `2` | Usage error |
| `3` | Upstream API error |
| `4` | Local IO error |

| Error code | Exit code | Meaning |
| --- | --- | --- |
| `usage_error` | 2 | Bad flags, a missing prompt, too many positional arguments, or a format conflict. |
| `missing_api_key` | 1 | `OPENAI_API_KEY` is not set. |
| `api_error` | 3 | The upstream rejected or failed the request; the message includes the HTTP status. |
| `moderation_blocked` | 3 | The upstream safety system rejected the prompt. Rewrite it, or retry with `--moderation low`. |
| `network_error` | 3 | The upstream was unreachable, or the request hit `--timeout`. |
| `read_failed` | 4 | An input image or mask could not be read. Check the local path and read permissions. |
| `write_failed` | 4 | The image could not be written to the output path. |

Error codes are part of the public contract: build retry logic on them, not on message text.

## Development

```bash
go test ./...            # fully offline, against a fake upstream
sh install_test.sh       # platform detection for the install script
go vet ./... && gofmt -l .
VINCI_LIVE_TEST=1 go test ./... -run Live   # opt-in smoke test against the real API
```

Releases are cut by tagging `main` (`v0.1.0`); see `docs/release.md`.

## License

Apache-2.0. Copyright 2026 Kieran Zhang.
