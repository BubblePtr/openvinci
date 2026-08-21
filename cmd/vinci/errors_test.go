package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// errorObject decodes the JSON Mode failure contract and fails the test if the
// payload is not exactly {"error":{"code":...,"message":...}}.
func errorObject(t *testing.T, out string) map[string]any {
	t.Helper()
	got := decodeJSONOutput(t, out)
	if len(got) != 1 {
		t.Fatalf("failure payload = %v, want only an \"error\" key", got)
	}
	obj, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want an object", got["error"])
	}
	if len(obj) != 2 {
		t.Errorf("error object keys = %v, want exactly code and message", obj)
	}
	if msg, _ := obj["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("error message = %v, want a non-empty string", obj["message"])
	}
	return obj
}

func TestMissingAPIKeyIsGenericFailure(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	delete(h.env, "OPENAI_API_KEY")

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(h.err(), "OPENAI_API_KEY") {
		t.Errorf("stderr = %q, want it to name OPENAI_API_KEY", h.err())
	}
	if n := len(f.calls()); n != 0 {
		t.Errorf("upstream request count = %d, want 0 without an API key", n)
	}
}

func TestMissingAPIKeyJSONErrorCode(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	delete(h.env, "OPENAI_API_KEY")

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if h.err() != "" {
		t.Errorf("stderr = %q, want empty in JSON mode", h.err())
	}
	if got := errorObject(t, h.out())["code"]; got != "missing_api_key" {
		t.Errorf("error code = %v, want missing_api_key", got)
	}
}

func TestUpstreamServerErrorIsAPIError(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.status = http.StatusInternalServerError
	f.body = `{"error":{"message":"upstream exploded","type":"server_error"}}`

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	obj := errorObject(t, h.out())
	if obj["code"] != "api_error" {
		t.Errorf("error code = %v, want api_error", obj["code"])
	}
	if msg, _ := obj["message"].(string); !strings.Contains(msg, "500") {
		t.Errorf("error message = %q, want it to include the upstream HTTP status", msg)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero.png")); !os.IsNotExist(err) {
		t.Errorf("output file exists, want no file written on an upstream failure")
	}
}

func TestUpstreamBadRequestIsAPIErrorWhenNotModeration(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.status = http.StatusBadRequest
	f.body = `{"error":{"code":"invalid_size","message":"unsupported size"}}`

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	obj := errorObject(t, h.out())
	if obj["code"] != "api_error" {
		t.Errorf("error code = %v, want api_error", obj["code"])
	}
	if msg, _ := obj["message"].(string); !strings.Contains(msg, "400") {
		t.Errorf("error message = %q, want it to include the upstream HTTP status", msg)
	}
}

func TestModerationBlockIsItsOwnErrorCode(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.status = http.StatusBadRequest
	f.body = `{"error":{"code":"moderation_blocked","message":"Your request was rejected by our safety system."}}`

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if got := errorObject(t, h.out())["code"]; got != "moderation_blocked" {
		t.Errorf("error code = %v, want moderation_blocked", got)
	}
	if n := len(f.calls()); n != 1 {
		t.Errorf("upstream request count = %d, want exactly 1 (no auto retry)", n)
	}
}

func TestAsyncTaskFailureIsAPIError(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.asyncTaskID = "task_fail"
	f.asyncFailed = true
	f.asyncFailMessage = "safety filter"

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 3 {
		t.Fatalf("exit code = %d, want 3 (stdout: %s, stderr: %s)", code, h.out(), h.err())
	}
	obj := errorObject(t, h.out())
	if obj["code"] != "api_error" {
		t.Errorf("error code = %v, want api_error", obj["code"])
	}
	if msg, _ := obj["message"].(string); !strings.Contains(msg, "safety filter") {
		t.Errorf("error message = %q, want it to mention the upstream failure", msg)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero.png")); !os.IsNotExist(err) {
		t.Errorf("output file exists, want no file written on a failed generation")
	}
}

func TestAsyncTaskTimeoutIsNetworkError(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.asyncTaskID = "task_hang"
	f.asyncNeverDone = true

	started := time.Now()
	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--timeout", "0.2", "--json")
	elapsed := time.Since(started)

	if code != 3 {
		t.Fatalf("exit code = %d, want 3 (stdout: %s)", code, h.out())
	}
	if elapsed > 3*time.Second {
		t.Errorf("run took %s, want it to give up at the timeout", elapsed)
	}
	obj := errorObject(t, h.out())
	if obj["code"] != "network_error" {
		t.Errorf("error code = %v, want network_error", obj["code"])
	}
	if msg, _ := obj["message"].(string); !strings.Contains(msg, "timed out") {
		t.Errorf("error message = %q, want it to mention the timeout", msg)
	}
}

func TestSuccessStatusWithoutImageDataIsAPIError(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.body = `{"created":1,"data":[]}`

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if got := errorObject(t, h.out())["code"]; got != "api_error" {
		t.Errorf("error code = %v, want api_error", got)
	}
}

func TestUnreachableUpstreamIsNetworkError(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"
	h.env["OPENAI_BASE_URL"] = deadURL
	dir := chdirTemp(t)

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json")
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if got := errorObject(t, h.out())["code"]; got != "network_error" {
		t.Errorf("error code = %v, want network_error", got)
	}
}

func TestSlowUpstreamHitsTheTimeout(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.delay = 5 * time.Second

	started := time.Now()
	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--timeout", "0.2", "--json")
	elapsed := time.Since(started)

	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if elapsed > 3*time.Second {
		t.Errorf("run took %s, want it to give up at the timeout", elapsed)
	}
	obj := errorObject(t, h.out())
	if obj["code"] != "network_error" {
		t.Errorf("error code = %v, want network_error", obj["code"])
	}
	if msg, _ := obj["message"].(string); !strings.Contains(msg, "timed out") {
		t.Errorf("error message = %q, want it to mention the timeout", msg)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero.png")); !os.IsNotExist(err) {
		t.Errorf("output file exists, want no file written on a timeout")
	}
}

func TestUnwritableOutputPathIsWriteFailed(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	// An existing directory can never be replaced by a file: a local IO failure
	// that happens only after a successful upstream generation.
	blocked := filepath.Join(dir, "hero.png")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("seeding directory: %v", err)
	}

	code := h.run("a red bicycle", "-o", blocked, "--json")
	if code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
	if got := errorObject(t, h.out())["code"]; got != "write_failed" {
		t.Errorf("error code = %v, want write_failed", got)
	}
}

func TestUsageErrorInJSONModeGoesToStdout(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"

	code := h.run("--json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if h.err() != "" {
		t.Errorf("stderr = %q, want empty in JSON mode", h.err())
	}
	if got := errorObject(t, h.out())["code"]; got != "usage_error" {
		t.Errorf("error code = %v, want usage_error", got)
	}
}

func TestUsageErrorOnAnUnknownFlagIsReportedAsJSON(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"

	code := h.run("--json", "--nope", "a red bicycle")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got := errorObject(t, h.out())["code"]; got != "usage_error" {
		t.Errorf("error code = %v, want usage_error", got)
	}
}

func TestInvalidTimeoutIsUsageError(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"

	if code := h.run("a red bicycle", "--timeout", "soon"); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(h.err(), "--timeout") {
		t.Errorf("stderr = %q, want it to name --timeout", h.err())
	}
}
