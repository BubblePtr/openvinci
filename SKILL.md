---
name: vinci
description: Generate a local image file from a text prompt with one `vinci` shell call. Use when a task needs an illustration, icon, hero image, or other visual asset. No SDK, no MCP. Requires OPENAI_API_KEY in the environment.
---

# vinci

**Canonical URL**: `https://raw.githubusercontent.com/BubblePtr/openvinci/main/SKILL.md`
**Audience**: AI coding agents (Claude Code, Codex, Cursor, Grok Build, and other shell-capable agents)
**Purpose**: Install `vinci` if needed, then turn a prompt into a local image file with a stable JSON contract.

## What this document is

When a human says to read this URL and follow it, run the steps below. Do the safe work yourself. Do not paste a human install guide back at them.

## Agent rules

1. **One command, one file.** Call `vinci` once per image. Never poll `/v1/tasks` yourself. Whether the upstream returns `b64_json` or a task id is handled inside `vinci`.
2. **Always pass `--json`.** Success is `{"path","size","format","model","duration_ms"}`. Failure is `{"error":{"code","message"}}` plus a non-zero exit. Branch on `error.code`, never on message text.
3. **The key stays in the environment.** Use `OPENAI_API_KEY`. Never pass it as a flag, never write it into the repo, never echo it.
4. **Do not invent the model name.** Default is `gpt-image-2`. Add `--model` only when the human (or the environment they already set) named a gateway alias such as `gpt-image-2-official`.
5. **If the binary or the key is missing, stop and ask.** Do not guess `OPENAI_BASE_URL` or a model alias.

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

## Step 2 — credentials

Required: `OPENAI_API_KEY` already in the environment.

Optional: `OPENAI_BASE_URL` for an OpenAI-compatible gateway. A URL that already ends in `/v1` is accepted as-is.

If `OPENAI_API_KEY` is unset, ask the human to export it. Do not have them paste it into a flag.

## Step 3 — generate

```bash
vinci "PROMPT" -o ./path/to/out.png --json
```

- Pass `-o` when you know the destination. Missing parent directories are created. An existing file is overwritten.
- Omit `-o` only when a slug filename in the current directory is acceptable.
- Long prompts with quotes or newlines go on stdin. Do not also pass a positional prompt; a positional argument wins and stdin is then never read.
- A prompt that starts with `-` needs `--`: `vinci -o out.png -- "-leading dash"`.
- Gateway alias: add `--model NAME`.
- `--size`, `--quality`, `--background`, `--format`, `--moderation`, `--timeout` pass through unchanged. Run `vinci --help` for the live list.

## Step 4 — read the result

Exit `0`: use `path`. That is the absolute file that was written.

Non-zero: parse `error.code`:

| code | exit | what you do |
| --- | --- | --- |
| `missing_api_key` | 1 | ask the human to export `OPENAI_API_KEY` |
| `usage_error` | 2 | fix the command; do not retry as-is |
| `api_error` | 3 | report the message; do not blindly retry |
| `moderation_blocked` | 3 | rewrite the prompt, or retry once with `--moderation low` |
| `network_error` | 3 | check `OPENAI_BASE_URL` and `--timeout`; retry only if the human asks |
| `write_failed` | 4 | pick a writable `-o` path |

`--timeout` covers submit, async polling, and download (default 120s). Do not wrap a failed generation in your own retry loop.

## Help

`vinci --help` is the live calling contract. If this file and `--help` disagree, `--help` wins.
