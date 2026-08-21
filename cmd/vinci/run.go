package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// Exit code tiers. Agents branch on these without parsing any text.
const (
	exitOK      = 0
	exitGeneric = 1
	exitUsage   = 2
	exitAPI     = 3
	exitIO      = 4
)

// Stable error-code vocabulary of the Error Contract. These strings are public
// API: they must stay stable even when the human-readable message changes.
const (
	codeUsageError       = "usage_error"
	codeMissingAPIKey    = "missing_api_key"
	codeAPIError         = "api_error"
	codeModerationBlock  = "moderation_blocked"
	codeNetworkError     = "network_error"
	codeWriteFailed      = "write_failed"
	defaultModel         = "gpt-image-2"
	defaultBaseURL       = "https://api.openai.com"
	defaultTimeout       = 120 * time.Second
	defaultFormat        = "png"
	defaultAuto          = "auto"
	maxSlugFilenameRunes = 48
)

// cliError carries both halves of the Error Contract: the machine-readable
// code plus the exit tier it maps to.
type cliError struct {
	code    string
	exit    int
	message string
}

func (e *cliError) Error() string { return e.message }

func usageErrorf(format string, args ...any) *cliError {
	return &cliError{code: codeUsageError, exit: exitUsage, message: fmt.Sprintf(format, args...)}
}

func errorf(code string, exit int, format string, args ...any) *cliError {
	return &cliError{code: code, exit: exit, message: fmt.Sprintf(format, args...)}
}

// run is the only seam of the CLI. Everything observable — exit code, stdout,
// stderr, files on disk — is produced from here.
func run(args []string, env func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	// The reporting mode must be known even when parsing itself fails, so that
	// a JSON-mode caller never gets a bare text error on a malformed command.
	jsonMode := hasJSONFlag(args)

	err := execute(args, env, stdin, stdout, stderr)
	if err == nil {
		return exitOK
	}
	return report(err, jsonMode, stdout, stderr)
}

func report(err *cliError, jsonMode bool, stdout, stderr io.Writer) int {
	if jsonMode {
		payload := map[string]any{
			"error": map[string]string{"code": err.code, "message": err.message},
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(payload)
		return err.exit
	}
	fmt.Fprintf(stderr, "vinci: %s\n", err.message)
	return err.exit
}

// hasJSONFlag pre-scans for --json so that failures are reported in the mode
// the caller asked for even when parsing itself fails. It must agree with
// parseArgs: same terminator, same last-one-wins rule, and an unparseable
// value counts as enabled so that its own usage error arrives as JSON.
func hasJSONFlag(args []string) bool {
	enabled := false
	for _, a := range args {
		if a == "--" {
			break
		}
		name, value, hasValue := splitFlag(a)
		if name != "json" {
			continue
		}
		if !hasValue {
			enabled = true
			continue
		}
		parsed, err := strconv.ParseBool(value)
		enabled = err != nil || parsed
	}
	return enabled
}

// splitFlag decomposes "--flag=value" into its parts. A non-flag argument
// yields an empty name.
func splitFlag(arg string) (name, value string, hasValue bool) {
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return "", "", false
	}
	name = strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		return name[:idx], name[idx+1:], true
	}
	return name, "", false
}

type options struct {
	prompt         string
	output         string
	size           string
	quality        string
	background     string
	format         string
	formatExplicit bool
	moderation     string
	timeout        time.Duration
	jsonMode       bool
	help           bool
	version        bool
}

func execute(args []string, env func(string) string, stdin io.Reader, stdout, stderr io.Writer) *cliError {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opts.help {
		fmt.Fprint(stdout, usageText())
		return nil
	}
	if opts.version {
		fmt.Fprintf(stdout, "vinci %s\n", version)
		return nil
	}
	if err := resolvePrompt(opts, stdin); err != nil {
		return err
	}
	if err := resolveFormat(opts); err != nil {
		return err
	}
	return generate(opts, env, stdout, stderr)
}

func parseArgs(args []string) (*options, *cliError) {
	opts := &options{
		size:       defaultAuto,
		quality:    defaultAuto,
		background: defaultAuto,
		moderation: defaultAuto,
		timeout:    defaultTimeout,
	}

	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		name, value, hasValue := splitFlag(arg)

		// takeValue pulls the value from "--flag=value" or "--flag value".
		takeValue := func() (string, *cliError) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", usageErrorf("flag %s needs a value", arg)
			}
			i++
			return args[i], nil
		}

		var perr *cliError
		switch name {
		case "h", "help":
			opts.help = true
		case "version":
			opts.version = true
		case "json":
			opts.jsonMode = true
			if hasValue {
				enabled, err := strconv.ParseBool(value)
				if err != nil {
					perr = usageErrorf("invalid value %q for --json: want true or false", value)
					break
				}
				opts.jsonMode = enabled
			}
		case "o", "output":
			opts.output, perr = takeValue()
		case "size":
			opts.size, perr = takeValue()
		case "quality":
			opts.quality, perr = takeValue()
		case "background":
			opts.background, perr = takeValue()
		case "format":
			opts.format, perr = takeValue()
			opts.formatExplicit = true
		case "moderation":
			opts.moderation, perr = takeValue()
		case "timeout":
			var raw string
			if raw, perr = takeValue(); perr == nil {
				opts.timeout, perr = parseTimeout(raw)
			}
		default:
			perr = usageErrorf("unknown flag %s (see vinci --help)", arg)
		}
		if perr != nil {
			return opts, perr
		}
	}

	if opts.help || opts.version {
		return opts, nil
	}
	if len(positional) > 1 {
		return opts, usageErrorf("expected at most one prompt argument, got %d", len(positional))
	}
	if len(positional) == 1 {
		opts.prompt = positional[0]
	}
	return opts, nil
}

func parseTimeout(raw string) (time.Duration, *cliError) {
	// A bare number is the common case for agents, and means seconds.
	if secs, err := strconv.ParseFloat(raw, 64); err == nil {
		d := time.Duration(secs * float64(time.Second))
		if d <= 0 {
			return 0, usageErrorf("invalid --timeout %q: must be greater than zero", raw)
		}
		return d, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, usageErrorf("invalid --timeout %q: want seconds (90) or a duration (90s, 2m)", raw)
	}
	if d <= 0 {
		return 0, usageErrorf("invalid --timeout %q: must be greater than zero", raw)
	}
	return d, nil
}

// resolvePrompt implements the Prompt entry rule: a positional argument wins
// outright and stdin is then never read at all. Probing stdin to detect a
// second prompt source would block forever on the open-but-idle pipe that
// agent harnesses hand their child processes.
func resolvePrompt(opts *options, stdin io.Reader) *cliError {
	if opts.prompt != "" {
		return nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return errorf(codeUsageError, exitUsage, "cannot read prompt from stdin: %v", err)
	}
	opts.prompt = strings.TrimSpace(string(data))
	if opts.prompt == "" {
		return usageErrorf("no prompt: pass it as an argument or pipe it on stdin")
	}
	return nil
}

func usageText() string {
	return `vinci — visual tools for agents. Generate an image from a prompt.

Usage:
  vinci [flags] "<prompt>"
  echo "<prompt>" | vinci [flags]

The prompt is the positional argument; stdin is read only when no positional
argument is given. Use -- to end flag parsing for a prompt starting with "-":
  vinci -o hero.png -- "-a prompt starting with a dash"

Flags:
  -o, --output <path>   output file path (default: slug filename from the prompt, in the current directory)
      --size <spec>     image size, e.g. 1024x1024 (default: auto)
      --quality <q>     rendering quality, e.g. low|medium|high (default: auto)
      --background <b>  background, e.g. transparent|opaque (default: auto)
      --format <f>      output format: png|jpeg|webp (default: inferred from --output, else png)
      --moderation <m>  moderation sensitivity, e.g. low (default: auto)
      --timeout <dur>   upstream timeout in seconds or as a duration (default: 120s)
      --json            print a machine-readable result or error object on stdout
  -h, --help            show this help
      --version         show the version

Environment:
  OPENAI_API_KEY   required; the only place the API key is read from
  OPENAI_BASE_URL  optional; any OpenAI-compatible gateway (default: https://api.openai.com)

Output:
  Plain mode prints only the absolute output path on stdout; diagnostics go to stderr.
  --json prints {"path","size","format","model","duration_ms"}, or {"error":{"code","message"}} on failure.

Exit codes:
  0 success   1 generic   2 usage   3 upstream API   4 local IO
`
}
