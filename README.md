# OpenVinci

**OpenVinci — visual tools for agents.**

`vinci` is a single static binary that turns a prompt into a local image file in one shell call. No SDK, no MCP server, no daemon — the kind of tool an AI coding agent can call in the middle of a task and parse the result of.

```bash
vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png
# /home/you/project/assets/hero.png
```

On success, plain mode prints only the absolute output path on stdout. `--json` prints a machine-readable result object. Every failure has a stable error code and a layered exit code.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
```

That puts a static `vinci` binary in `~/.local/bin` (override with `--dir` or `VINCI_INSTALL_DIR`). Supported platforms: macOS arm64, Linux x64/arm64. Pin a release with `sh -s -- --version v0.1.0`.

From source (Go 1.24+):

```bash
go build -o vinci ./cmd/vinci
```

## Configuration

Two environment variables, no config file:

| Variable | Required | Description |
| --- | --- | --- |
| `OPENAI_API_KEY` | yes | The API key. Read from the environment only, never from a flag, so it stays out of shell history. |
| `OPENAI_BASE_URL` | no | Any OpenAI-compatible gateway. Defaults to `https://api.openai.com`. A URL that already ends in `/v1` is accepted as-is. |

The upstream model defaults to `gpt-image-2`. Override it with `--model` for a gateway alias. A generation is one prompt to one local file: either the upstream returns the image immediately (`b64_json` or `url`), or it returns a task id and `vinci` polls until the image is ready. Polling is waiting, not a retry. There is no automatic retry of a failed generation, so latency and cost stay predictable. The `--timeout` covers the whole generation, including polls and the image download.

## Usage

```bash
vinci [flags] "<prompt>"
echo "<prompt>" | vinci [flags]
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

## For AI agents

`vinci --help` is the single source of truth for the calling contract. To make an agent reach for the tool on its own, drop this block into your project's `CLAUDE.md` or `AGENTS.md`:

```markdown
## Generating images

Use `vinci` when a task needs an illustration, icon, hero image or other visual asset: it turns a text prompt into a local image file in one shell call, with no SDK or MCP server. Requires `OPENAI_API_KEY` in the environment. If `vinci` is not on PATH:

    curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh

    vinci "minimal technical illustration of an AI agent" -o ./assets/hero.png --json

Always pass `--json`: it prints `{"path","size","format","model","duration_ms"}` on success, or `{"error":{"code","message"}}` with a non-zero exit code on failure. Run `vinci --help` for the full contract.
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-o`, `--output <path>` | slug filename from the prompt, in the current directory | Output path. Missing parent directories are created. |
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

Flag values are passed through to the upstream without being re-validated locally, so upstream additions work without a new release.

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
