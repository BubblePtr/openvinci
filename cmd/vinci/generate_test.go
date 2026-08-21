package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var fakeImage = []byte("\x89PNG\r\n\x1a\nfake image bytes")

// fakeUpstream is the minimal subset of the OpenAI Images API the CLI needs.
// Tests reach it the same way a real deployment reaches a gateway: through
// OPENAI_BASE_URL. There is no test-only injection point.
type fakeUpstream struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []upstreamRequest

	// Response knobs, read by the handler.
	status int
	body   string
	delay  time.Duration

	// asyncTaskID, if set, makes POST return a submitted task and GET
	// /v1/tasks/{id} complete (or fail) according to the knobs below.
	asyncTaskID      string
	asyncPolls       int
	asyncFailed      bool
	asyncNeverDone   bool
	asyncFailMessage string
	polls            int
}

type upstreamRequest struct {
	method string
	path   string
	auth   string
	ctype  string
	body   map[string]any
	raw    string
}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{
		status: http.StatusOK,
		body:   successBody(fakeImage),
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		parsed := map[string]any{}
		_ = json.Unmarshal(raw, &parsed)

		f.mu.Lock()
		f.requests = append(f.requests, upstreamRequest{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			ctype:  r.Header.Get("Content-Type"),
			body:   parsed,
			raw:    string(raw),
		})
		delay, status, body := f.delay, f.status, f.body
		taskID, pollsBefore, failed, neverDone, failMsg := f.asyncTaskID, f.asyncPolls, f.asyncFailed, f.asyncNeverDone, f.asyncFailMessage
		path := r.URL.Path
		if r.Method == http.MethodGet && isTaskPath(path) {
			f.polls++
			polls := f.polls
			f.mu.Unlock()
			f.writeTask(w, r, polls, pollsBefore, neverDone, failed, failMsg)
			return
		}
		f.mu.Unlock()

		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		if r.Method == http.MethodGet && path == "/generated.png" {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fakeImage)
			return
		}
		if taskID != "" && r.Method == http.MethodPost {
			body = `{"code":200,"data":[{"status":"submitted","task_id":"` + taskID + `"}]}`
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func isTaskPath(path string) bool {
	return strings.HasPrefix(path, "/v1/tasks/") || strings.HasPrefix(path, "/tasks/")
}

func (f *fakeUpstream) writeTask(w http.ResponseWriter, r *http.Request, polls, pollsBefore int, neverDone, failed bool, failMsg string) {
	if delay := f.delay; delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if neverDone || polls <= pollsBefore {
		_, _ = io.WriteString(w, `{"code":200,"data":{"id":"`+f.asyncTaskID+`","status":"processing"}}`)
		return
	}
	if failed {
		if failMsg == "" {
			failMsg = "upstream exploded"
		}
		_, _ = io.WriteString(w, `{"code":200,"data":{"id":"`+f.asyncTaskID+`","status":"failed","error":{"message":"`+failMsg+`"}}}`)
		return
	}
	url := f.server.URL + "/generated.png"
	_, _ = io.WriteString(w, `{"code":200,"data":{"id":"`+f.asyncTaskID+`","status":"completed","result":{"images":[{"url":["`+url+`"]}]}}}`)
}

func successBody(img []byte) string {
	return `{"created":1,"size":"1024x1024","quality":"high","output_format":"png","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(img) + `"}]}`
}

// successBodyWithoutSize mimics a gateway that drops the size echo.
func successBodyWithoutSize(img []byte) string {
	return `{"created":1,"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(img) + `"}]}`
}

func (f *fakeUpstream) calls() []upstreamRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]upstreamRequest(nil), f.requests...)
}

func (f *fakeUpstream) onlyCall(t *testing.T) upstreamRequest {
	t.Helper()
	calls := f.calls()
	if len(calls) != 1 {
		t.Fatalf("upstream request count = %d, want exactly 1", len(calls))
	}
	return calls[0]
}

// newHarnessWith wires a harness to a fake upstream plus a working API key.
func newHarnessWith(t *testing.T, f *fakeUpstream) *harness {
	t.Helper()
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"
	h.env["OPENAI_BASE_URL"] = f.server.URL
	return h
}

// chdirTemp moves the process into an empty directory so tests that exercise
// the default Output Path do not litter the repository.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

// assertGeneratedFile checks the plain-mode contract: stdout is exactly one
// absolute path, and it names the expected file with the expected bytes.
// Identity is compared with os.SameFile because a temp dir can be reached
// through several equivalent paths (/var vs /private/var on macOS).
func assertGeneratedFile(t *testing.T, printed, dir, base string, want []byte) {
	t.Helper()
	path := strings.TrimSuffix(printed, "\n")
	if strings.Contains(path, "\n") {
		t.Fatalf("stdout = %q, want a single line", printed)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("stdout path = %q, want an absolute path", path)
	}
	if got := filepath.Base(path); got != base {
		t.Errorf("output file name = %q, want %q", got, base)
	}
	printedInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat printed path: %v", err)
	}
	expectedInfo, err := os.Stat(filepath.Join(dir, base))
	if err != nil {
		t.Fatalf("stat expected path: %v", err)
	}
	if !os.SameFile(printedInfo, expectedInfo) {
		t.Errorf("printed path %q is not the file written at %q", path, filepath.Join(dir, base))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("file bytes = %q, want %q", got, want)
	}
}

func TestGenerateWritesFileAndPrintsAbsolutePath(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("a red bicycle", "-o", "hero.png"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}

	if h.err() != "" {
		t.Errorf("stderr = %q, want empty", h.err())
	}
	assertGeneratedFile(t, h.out(), dir, "hero.png", fakeImage)
}

func TestGenerateSendsOneUpstreamRequestWithDefaults(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png")); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}

	call := f.onlyCall(t)
	if call.method != http.MethodPost {
		t.Errorf("method = %s, want POST", call.method)
	}
	if call.path != "/v1/images/generations" {
		t.Errorf("path = %s, want /v1/images/generations", call.path)
	}
	if call.auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", call.auth, "Bearer test-key")
	}
	if call.ctype != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", call.ctype)
	}
	want := map[string]any{
		"model":         "gpt-image-2",
		"prompt":        "a red bicycle",
		"size":          "auto",
		"quality":       "auto",
		"background":    "auto",
		"moderation":    "auto",
		"output_format": "png",
	}
	for k, v := range want {
		if call.body[k] != v {
			t.Errorf("request body %q = %v, want %v (raw: %s)", k, call.body[k], v, call.raw)
		}
	}
	if n, ok := call.body["n"].(float64); ok && n != 1 {
		t.Errorf("request body n = %v, want 1", n)
	}
}

func TestGeneratePassesFlagValuesThroughUnchanged(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	code := h.run("a red bicycle",
		"-o", filepath.Join(dir, "hero.webp"),
		"--size", "1536x1024",
		"--quality", "low",
		"--background", "transparent",
		"--moderation", "low",
		"--model", "gpt-image-2-official",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}

	call := f.onlyCall(t)
	want := map[string]any{
		"model":         "gpt-image-2-official",
		"size":          "1536x1024",
		"quality":       "low",
		"background":    "transparent",
		"moderation":    "low",
		"output_format": "webp",
	}
	for k, v := range want {
		if call.body[k] != v {
			t.Errorf("request body %q = %v, want %v (raw: %s)", k, call.body[k], v, call.raw)
		}
	}
}

func TestGenerateAcceptsBaseURLThatAlreadyEndsInV1(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	h.env["OPENAI_BASE_URL"] = f.server.URL + "/v1/"

	if code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png")); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := f.onlyCall(t).path; got != "/v1/images/generations" {
		t.Errorf("path = %s, want /v1/images/generations", got)
	}
}

func TestGenerateDownloadsASynchronousImageURL(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.body = `{"created":1,"size":"1024x1024","data":[{"url":"` + f.server.URL + `/generated.png"}]}`

	if code := h.run("a red bicycle", "-o", "hero.png"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	assertGeneratedFile(t, h.out(), dir, "hero.png", fakeImage)
}

func TestGeneratePollsAnAsyncTaskAndDownloadsTheImage(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.asyncTaskID = "task_test"

	if code := h.run("a red bicycle", "-o", "hero.png"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	assertGeneratedFile(t, h.out(), dir, "hero.png", fakeImage)

	var methods []string
	for _, c := range f.calls() {
		methods = append(methods, c.method+" "+c.path)
	}
	if len(methods) < 3 {
		t.Fatalf("upstream calls = %v, want POST generations, GET task, GET image", methods)
	}
	if methods[0] != "POST /v1/images/generations" {
		t.Errorf("first call = %q, want POST /v1/images/generations", methods[0])
	}
	if methods[1] != "GET /v1/tasks/task_test" {
		t.Errorf("second call = %q, want GET /v1/tasks/task_test", methods[1])
	}
	if methods[2] != "GET /generated.png" {
		t.Errorf("third call = %q, want GET /generated.png", methods[2])
	}
}
