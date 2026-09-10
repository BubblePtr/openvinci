---
name: vinci
description: Install and configure Vinci on first use, then generate or edit local images with one `vinci` shell call. Use for Vinci setup or when a task needs an illustration, icon, hero image, or other visual asset. Collect the model, API Base URL, and API key when configuration is missing.
---

# vinci

**Canonical URL**: `https://raw.githubusercontent.com/BubblePtr/openvinci/main/SKILL.md`
**Audience**: AI coding agents (Claude Code, Codex, Cursor, Grok Build, and other shell-capable agents)
**Purpose**: Install `vinci`, guide first-time configuration, and reuse it to generate or edit local image files with a stable JSON contract.

## What this document is

When a human says to read this URL and follow it, run the steps below. Do the safe work yourself. Do not paste a human install guide back at them.

## Agent rules

1. **One command, one file.** Call `vinci` once per image. Never poll `/v1/tasks` yourself. Whether the upstream returns `b64_json` or a task id is handled inside `vinci`.
2. **Always pass `--json`.** Success is `{"path","size","format","model","duration_ms"}`. Failure is `{"error":{"code","message"}}` plus a non-zero exit. Branch on `error.code`, never on message text.
3. **Pass the key through the environment.** Use `OPENAI_API_KEY`. The setup profile described below may store it locally. Never put it in CLI flags, the repo, output, or memory.
4. **Use the selected model.** The CLI default is `gpt-image-2.5-flare`. Respect a model the human already chose, including an official model or gateway alias enabled for their key. Pass the selected value explicitly with `--model`.
5. **Resolve missing setup through Steps 1 and 2.** Reuse values already supplied or configured. Ask for missing information together; do not repeatedly ask a returning user for the same settings.

## Step 1 — make sure vinci is on PATH

```bash
vinci --version
```

If that fails:

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
vinci --version
```

Supported platforms: macOS arm64, Linux x64/arm64. If the installer rejects the OS or arch, tell the human and stop.

## Step 2 — load configuration or guide first use

Resolve these three values before generating or editing:

| Value | Invocation | First-use suggestion |
| --- | --- | --- |
| Default model | Pass `VINCI_MODEL` as `--model` | `gpt-image-2.5-flare` |
| API Base URL | `OPENAI_BASE_URL` | `https://api.openai.com/v1` for OpenAI; use the provider's URL for a gateway |
| API key | `OPENAI_API_KEY` | Required; no default |

Check the current request, exported environment, and `${XDG_CONFIG_HOME:-$HOME/.config}/openvinci/env`, in that order. Read only that profile or another credential source the human names; never print its key. Keep the key paired with its provider URL. A newly selected URL does not authorize sending a previous provider's key there.

If the values are complete, use them without asking again. If anything is missing, ask **one bundled question in the human's language** for the missing fields. Show the model and URL suggestions above, and explain that OpenAI and a gateway require their own keys. A key may come from an environment variable, a local file path, or secure input when available; if the human already supplied it, do not ask again or echo it.

For a fresh setup, the question should cover:

> Which default model and API Base URL should Vinci use, and where can I read the API key? Suggested model: gpt-image-2.5-flare. For OpenAI, the URL is https://api.openai.com/v1; for a gateway, use its address and key. I will save these settings in your user configuration directory for later calls. Tell me if they should apply only to this session.

State the resolved profile path when collecting the settings. After the human supplies them, save `OPENAI_API_KEY`, `OPENAI_BASE_URL`, and `VINCI_MODEL` there as shell-quoted assignments with `export`. Use `umask 077` before creating the directory or file, set directory permissions to `700` and file permissions to `600`, and preserve unrelated existing entries. Quote values as data, for example with Python's `shlex.quote`; do not interpolate credentials into executable shell text. Honor session-only setup and do not overwrite the saved defaults for a one-off model override.

When current environment or request values are used instead of the saved profile, retain those overrides in the invocation. For a saved profile, load it and call Vinci **in the same shell execution**:

```bash
(
  set -a
  . "${XDG_CONFIG_HOME:-$HOME/.config}/openvinci/env" || exit
  set +a
  vinci "PROMPT" --model "$VINCI_MODEL" -o ./path/to/out.png --json
)
```

CLI tools launched in a later shell do not inherit an earlier tool call's exports. Load the profile again on each call that uses it. Vinci itself does not read this file or `VINCI_MODEL`; the agent passes the selected model via `--model`.

After setup, report the model, URL, profile path (or session-only scope), and that the key is configured, with no key value. Continue the original image request. If the human only requested setup, finish after configuration checks without making a paid image request. See the [first-use guide](https://raw.githubusercontent.com/BubblePtr/openvinci/main/docs/first-use.md) (Chinese) for the user-facing flow and profile format.

## Step 3 — generate or edit

```bash
vinci "PROMPT" --model "$VINCI_MODEL" -o ./path/to/out.png --json
```

- Pass `-o` when you know the destination. Missing parent directories are created. An existing file is overwritten.
- Omit `-o` only when a slug filename in the current directory is acceptable.
- Long prompts with quotes or newlines go on stdin. Do not also pass a positional prompt; a positional argument wins and stdin is then never read.
- A prompt that starts with `-` needs `--`: `vinci -o out.png --model "$VINCI_MODEL" -- "-leading dash"`.
- Official model or gateway alias: add `--model NAME`.
- Load or inject the resolved configuration from Step 2 in the same shell call. The examples below illustrate flags; keep the user's selected model unless they ask to override it.
- `--size`, `--quality`, `--background`, `--format`, `--moderation`, `--timeout` pass through unchanged. Run `vinci --help` for the live list.

### GPT Image 2.5

- `gpt-image-2.5-flare`: fast everyday generation.
- `gpt-image-2.5-sunburst`: generation and precise editing.
- Both support `low`, `medium`, `high`, `xhigh`, `max`, and `auto` quality settings in the official API. Check support on the configured gateway before relying on a setting.

```bash
vinci "PROMPT" --model gpt-image-2.5-flare --quality xhigh -o out.png --json
vinci --image input.png "EDIT PROMPT" --model gpt-image-2.5-sunburst --quality max -o edited.png --json
```

For FlatRouter setup and observed differences between requested parameters and returned images, read the [model and quality guide](https://raw.githubusercontent.com/BubblePtr/openvinci/main/docs/image-models.md) (Chinese). A successful call alone does not verify the requested dimensions, quality, or model selection.

### Edit local images

```bash
vinci --image input.png "EDIT PROMPT" --model "$VINCI_MODEL" -o edited.png --json
vinci --image input.png --image reference.jpg "EDIT PROMPT" --model "$VINCI_MODEL" -o edited.png --json
vinci --image input.png --mask mask.png "EDIT PROMPT" --model "$VINCI_MODEL" -o edited.png --json
```

- Repeat `--image` to supply multiple local images in order. Keep a separate `-o` path to preserve the original.
- `--mask` requires `--image`: use a PNG with an alpha channel, matching the first input's dimensions. Transparent areas indicate where to edit.
- Default uploads use multipart `/v1/images/edits`. For host `api.apimart.ai` with a `gpt-image-` model, Vinci automatically uses JSON `/v1/images/generations` with Base64 `image_urls` and optional `mask_url`. Do not upload local images to a separate hosting service or build the Base64 payload yourself.
- Keep the human's configured model or gateway alias. Vinci does not read `VINCI_MODEL` automatically.
- Run `vinci --help` to check that the installed version supports `--image`; an older release may require updating or building from source.

## Step 4 — read the result

Exit `0`: use `path`. That is the absolute file that was written.

`model` is the requested name. `size` comes from the upstream response, with the requested value as a fallback. Vinci does not verify pixel dimensions or expose the upstream's model and quality fields. When exact dimensions or model identity matter, inspect the saved image or upstream evidence rather than treating the CLI's JSON result as verification.

Non-zero: parse `error.code`:

| code | exit | what you do |
| --- | --- | --- |
| `missing_api_key` | 1 | load the configured key in this shell, or collect the missing value through Step 2 |
| `usage_error` | 2 | fix the command; do not retry as-is |
| `api_error` | 3 | report the message; do not blindly retry |
| `moderation_blocked` | 3 | rewrite the prompt, or retry once with `--moderation low` |
| `network_error` | 3 | check `OPENAI_BASE_URL` and `--timeout`; retry only if the human asks |
| `read_failed` | 4 | check the input image or mask path and read permissions |
| `write_failed` | 4 | pick a writable `-o` path |

`--timeout` covers submit, async polling, and download (default 120s). Do not wrap a failed generation in your own retry loop.

## Help

`vinci --help` is the live calling contract. If this file and `--help` disagree, `--help` wins.
