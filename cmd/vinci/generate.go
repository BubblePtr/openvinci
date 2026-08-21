package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// resolveFormat implements the format rule: an explicit --format wins, an
// omitted one is inferred from the output extension, and a contradiction
// between the two is a usage error rather than a silently wrong file.
func resolveFormat(opts *options) *cliError {
	inferred := formatFromExtension(opts.output)
	if !opts.formatExplicit {
		if inferred == "" {
			opts.format = defaultFormat
		} else {
			opts.format = inferred
		}
		return nil
	}
	if opts.format == "" {
		return usageErrorf("flag --format needs a value")
	}
	if inferred != "" && normalizeFormat(opts.format) != inferred {
		return usageErrorf("--format %s conflicts with the %s extension of %s", opts.format, filepath.Ext(opts.output), opts.output)
	}
	return nil
}

func formatFromExtension(output string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(output), ".")) {
	case "png":
		return "png"
	case "jpg", "jpeg":
		return "jpeg"
	case "webp":
		return "webp"
	default:
		return ""
	}
}

func normalizeFormat(format string) string {
	if strings.EqualFold(format, "jpg") {
		return "jpeg"
	}
	return strings.ToLower(format)
}

// generate performs exactly one upstream request and writes exactly one file.
// There is no retry: cost and latency stay predictable for the calling Agent.
func generate(opts *options, env func(string) string, stdout, stderr io.Writer) *cliError {
	started := time.Now()

	apiKey := strings.TrimSpace(env("OPENAI_API_KEY"))
	if apiKey == "" {
		return errorf(codeMissingAPIKey, exitGeneric,
			"OPENAI_API_KEY is not set; export it first, e.g. export OPENAI_API_KEY=sk-...")
	}

	outputPath, cerr := resolveOutputPath(opts)
	if cerr != nil {
		return cerr
	}

	result, cerr := requestImage(opts, apiKey, env("OPENAI_BASE_URL"))
	if cerr != nil {
		return cerr
	}

	if err := writeImage(outputPath, result.image); err != nil {
		return errorf(codeWriteFailed, exitIO, "cannot write %s: %v", outputPath, err)
	}

	if opts.jsonMode {
		result := struct {
			Path       string `json:"path"`
			Size       string `json:"size"`
			Format     string `json:"format"`
			Model      string `json:"model"`
			DurationMS int64  `json:"duration_ms"`
		}{outputPath, reportedSize(result.size, opts.size), opts.format, defaultModel, time.Since(started).Milliseconds()}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(result)
		return nil
	}
	fmt.Fprintln(stdout, outputPath)
	return nil
}

func resolveOutputPath(opts *options) (string, *cliError) {
	target := opts.output
	if target == "" {
		target = slugFilename(opts.prompt) + "." + extensionForFormat(opts.format)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", errorf(codeWriteFailed, exitIO, "cannot resolve output path %s: %v", target, err)
	}
	return abs, nil
}

func extensionForFormat(format string) string {
	if f := normalizeFormat(format); f != "" {
		return f
	}
	return defaultFormat
}

// slugFilename derives the default Output Path from the Prompt so that a call
// without -o still succeeds with a name a human can recognise.
func slugFilename(prompt string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(prompt) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")

	runes := []rune(slug)
	if len(runes) > maxSlugFilenameRunes {
		slug = strings.Trim(string(runes[:maxSlugFilenameRunes]), "-")
	}
	if slug == "" {
		return "image"
	}
	return slug
}

func writeImage(path string, data []byte) error {
	// Creating missing parent directories keeps `-o ./assets/hero.png` working
	// on a fresh checkout, which is the shape agents write most often.
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// Overwriting silently matches `curl -o`: rerunning the same command is
	// idempotent from the caller's point of view.
	return os.WriteFile(path, data, 0o644)
}

// reportedSize prefers the size the upstream says it rendered; a gateway that
// omits it leaves the requested value ("auto" when none was asked for) as the
// only honest answer.
func reportedSize(fromResponse, requested string) string {
	if s := strings.TrimSpace(fromResponse); s != "" {
		return s
	}
	return requested
}

// generated is one Generation's result: the decoded image plus what the
// upstream reported about it.
type generated struct {
	image []byte
	size  string
}

type upstreamError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func requestImage(opts *options, apiKey, baseURL string) (generated, *cliError) {
	payload := map[string]any{
		"model":         defaultModel,
		"prompt":        opts.prompt,
		"n":             1,
		"size":          opts.size,
		"quality":       opts.quality,
		"background":    opts.background,
		"moderation":    opts.moderation,
		"output_format": opts.format,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return generated{}, errorf(codeAPIError, exitAPI, "cannot encode request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	endpoint := generationsEndpoint(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return generated{}, errorf(codeNetworkError, exitAPI, "invalid upstream URL %s: %v", endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return generated{}, errorf(codeNetworkError, exitAPI, "upstream request timed out after %s", opts.timeout)
		}
		return generated{}, errorf(codeNetworkError, exitAPI, "cannot reach upstream %s: %v", endpoint, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return generated{}, errorf(codeNetworkError, exitAPI, "upstream request timed out after %s", opts.timeout)
		}
		return generated{}, errorf(codeNetworkError, exitAPI, "cannot read upstream response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return generated{}, upstreamFailure(resp.StatusCode, raw)
	}

	var decoded struct {
		Size string `json:"size"`
		Data []struct {
			B64JSON string `json:"b64_json"`
			Size    string `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return generated{}, errorf(codeAPIError, exitAPI, "upstream returned HTTP 200 with an unreadable body: %v", err)
	}
	if len(decoded.Data) == 0 || decoded.Data[0].B64JSON == "" {
		return generated{}, errorf(codeAPIError, exitAPI, "upstream returned HTTP 200 without image data")
	}
	image, err := base64.StdEncoding.DecodeString(decoded.Data[0].B64JSON)
	if err != nil {
		return generated{}, errorf(codeAPIError, exitAPI, "upstream returned invalid base64 image data: %v", err)
	}
	return generated{image: image, size: reportedSize(decoded.Size, decoded.Data[0].Size)}, nil
}

// upstreamFailure maps an upstream error body onto the Error Contract, lifting
// moderation blocks to a first-class category so agents can rewrite the prompt
// instead of blindly retrying.
func upstreamFailure(status int, raw []byte) *cliError {
	var body upstreamError
	_ = json.Unmarshal(raw, &body)
	message := strings.TrimSpace(body.Error.Message)

	if status == http.StatusBadRequest && body.Error.Code == codeModerationBlock {
		if message == "" {
			message = "the prompt was blocked by the upstream moderation system"
		}
		return errorf(codeModerationBlock, exitAPI, "moderation blocked this prompt: %s", message)
	}
	if message == "" {
		message = strings.TrimSpace(string(raw))
	}
	if message == "" {
		message = "no error message in the response body"
	}
	return errorf(codeAPIError, exitAPI, "upstream returned HTTP %d: %s", status, message)
}

// generationsEndpoint tolerates both base URL conventions: bare origins and
// gateways whose OPENAI_BASE_URL already carries the /v1 prefix.
func generationsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/images/generations"
	}
	return base + "/v1/images/generations"
}
