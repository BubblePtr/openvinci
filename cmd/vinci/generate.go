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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// pollInterval is the wait between async task status GETs. Tests may shorten it.
var pollInterval = time.Second

const maxImageBytes = 32 << 20

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
		}{outputPath, reportedSize(result.size, opts.size), opts.format, opts.model, time.Since(started).Milliseconds()}
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
		"model":         opts.model,
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
	return resolveImage(ctx, apiKey, baseURL, opts.timeout, raw)
}

// resolveImage accepts the two envelopes gateways actually return: an
// immediate image (b64_json or url) or a submitted task that must be polled.
func resolveImage(ctx context.Context, apiKey, baseURL string, timeout time.Duration, raw []byte) (generated, *cliError) {
	var decoded struct {
		Size string `json:"size"`
		Data []struct {
			B64JSON string          `json:"b64_json"`
			URL     json.RawMessage `json:"url"`
			Size    string          `json:"size"`
			TaskID  string          `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return generated{}, errorf(codeAPIError, exitAPI, "upstream returned HTTP 200 with an unreadable body: %v", err)
	}
	if len(decoded.Data) == 0 {
		return generated{}, errorf(codeAPIError, exitAPI, "upstream returned HTTP 200 without image data")
	}
	item := decoded.Data[0]
	size := reportedSize(decoded.Size, item.Size)

	if item.B64JSON != "" {
		image, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return generated{}, errorf(codeAPIError, exitAPI, "upstream returned invalid base64 image data: %v", err)
		}
		return generated{image: image, size: size}, nil
	}
	if item.TaskID != "" {
		got, err := pollTask(ctx, apiKey, baseURL, timeout, item.TaskID)
		if err != nil {
			return generated{}, err
		}
		if got.size == "" {
			got.size = size
		}
		return got, nil
	}
	if imageURL := firstURL(item.URL); imageURL != "" {
		image, err := fetchImageURL(ctx, timeout, imageURL)
		if err != nil {
			return generated{}, err
		}
		return generated{image: image, size: size}, nil
	}
	return generated{}, errorf(codeAPIError, exitAPI, "upstream returned HTTP 200 without image data")
}

func pollTask(ctx context.Context, apiKey, baseURL string, timeout time.Duration, taskID string) (generated, *cliError) {
	endpoint := taskEndpoint(baseURL, taskID)
	for {
		raw, status, err := getJSON(ctx, endpoint, apiKey)
		if err != nil {
			if timedOut(ctx, err) {
				return generated{}, errorf(codeNetworkError, exitAPI, "upstream request timed out after %s", timeout)
			}
			return generated{}, errorf(codeNetworkError, exitAPI, "cannot reach upstream %s: %v", endpoint, err)
		}
		if status != http.StatusOK {
			return generated{}, upstreamFailure(status, raw)
		}

		var decoded struct {
			Data struct {
				Status string `json:"status"`
				Error  struct {
					Message string `json:"message"`
				} `json:"error"`
				Result struct {
					URL    json.RawMessage `json:"url"`
					Images []struct {
						URL json.RawMessage `json:"url"`
					} `json:"images"`
				} `json:"result"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return generated{}, errorf(codeAPIError, exitAPI, "upstream returned an unreadable task status: %v", err)
		}

		switch strings.ToLower(strings.TrimSpace(decoded.Data.Status)) {
		case "completed", "success", "succeeded":
			imageURL := firstURL(decoded.Data.Result.URL)
			if imageURL == "" && len(decoded.Data.Result.Images) > 0 {
				imageURL = firstURL(decoded.Data.Result.Images[0].URL)
			}
			if imageURL == "" {
				return generated{}, errorf(codeAPIError, exitAPI, "upstream completed without image data")
			}
			image, ferr := fetchImageURL(ctx, timeout, imageURL)
			if ferr != nil {
				return generated{}, ferr
			}
			return generated{image: image}, nil
		case "failed", "error", "cancelled", "canceled":
			msg := strings.TrimSpace(decoded.Data.Error.Message)
			if msg == "" {
				msg = "generation failed"
			}
			return generated{}, errorf(codeAPIError, exitAPI, "upstream generation failed: %s", msg)
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return generated{}, errorf(codeNetworkError, exitAPI, "upstream request timed out after %s", timeout)
		case <-timer.C:
		}
	}
}

func fetchImageURL(ctx context.Context, timeout time.Duration, imageURL string) ([]byte, *cliError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, errorf(codeNetworkError, exitAPI, "invalid image URL %s: %v", imageURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if timedOut(ctx, err) {
			return nil, errorf(codeNetworkError, exitAPI, "upstream request timed out after %s", timeout)
		}
		return nil, errorf(codeNetworkError, exitAPI, "cannot download image %s: %v", imageURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errorf(codeAPIError, exitAPI, "image download returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		if timedOut(ctx, err) {
			return nil, errorf(codeNetworkError, exitAPI, "upstream request timed out after %s", timeout)
		}
		return nil, errorf(codeNetworkError, exitAPI, "cannot read image download: %v", err)
	}
	if int64(len(data)) > maxImageBytes {
		return nil, errorf(codeAPIError, exitAPI, "image download exceeded %d bytes", maxImageBytes)
	}
	if len(data) == 0 {
		return nil, errorf(codeAPIError, exitAPI, "image download was empty")
	}
	return data, nil
}

func getJSON(ctx context.Context, endpoint, apiKey string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, err
}

func firstURL(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, u := range arr {
			if u = strings.TrimSpace(u); u != "" {
				return u
			}
		}
	}
	return ""
}

func timedOut(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded) || (err != nil && errors.Is(err, context.DeadlineExceeded))
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

// apiRoot tolerates both base URL conventions: bare origins and gateways
// whose OPENAI_BASE_URL already carries the /v1 prefix.
func apiRoot(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func generationsEndpoint(baseURL string) string {
	return apiRoot(baseURL) + "/images/generations"
}

func taskEndpoint(baseURL, taskID string) string {
	return apiRoot(baseURL) + "/tasks/" + url.PathEscape(taskID)
}
