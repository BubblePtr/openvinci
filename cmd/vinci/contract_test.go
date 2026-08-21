package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func decodeJSONOutput(t *testing.T, out string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, out)
	}
	return got
}

func TestDefaultOutputPathIsSlugFilenameInCurrentDirectory(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("A Minimal, Technical Illustration of an AI Agent!"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	assertGeneratedFile(t, h.out(), dir, "a-minimal-technical-illustration-of-an-ai-agent.png", fakeImage)
}

func TestSlugFilenameIsTruncatedAndUsesTheChosenFormat(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	chdirTemp(t)

	prompt := "an extremely detailed cinematic wide angle photograph of a lighthouse during a storm"
	if code := h.run(prompt, "--format", "jpeg"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}

	base := filepath.Base(strings.TrimSpace(h.out()))
	if !strings.HasSuffix(base, ".jpeg") {
		t.Errorf("output file name = %q, want a .jpeg extension", base)
	}
	stem := strings.TrimSuffix(base, ".jpeg")
	if len(stem) > 48 {
		t.Errorf("slug stem %q is %d chars, want at most 48", stem, len(stem))
	}
	if strings.HasPrefix(stem, "-") || strings.HasSuffix(stem, "-") {
		t.Errorf("slug stem %q has a dangling dash", stem)
	}
	if !strings.HasPrefix(stem, "an-extremely-detailed-cinematic") {
		t.Errorf("slug stem = %q, want it derived from the prompt", stem)
	}
}

func TestSlugFilenameFallsBackWhenPromptHasNoWordCharacters(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("!!! ??? ..."); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	assertGeneratedFile(t, h.out(), dir, "image.png", fakeImage)
}

func TestExistingOutputFileIsOverwrittenSilently(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	target := filepath.Join(dir, "hero.png")
	if err := os.WriteFile(target, []byte("stale bytes"), 0o644); err != nil {
		t.Fatalf("seeding output file: %v", err)
	}

	if code := h.run("a red bicycle", "-o", target); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if h.err() != "" {
		t.Errorf("stderr = %q, want empty (overwrite must be silent)", h.err())
	}
	assertGeneratedFile(t, h.out(), dir, "hero.png", fakeImage)
}

func TestPromptCanBeReadFromStdin(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	h.stdin = "  a lighthouse in a \"storm\"\n"

	if code := h.run("-o", filepath.Join(dir, "hero.png")); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := f.onlyCall(t).body["prompt"]; got != `a lighthouse in a "storm"` {
		t.Errorf("upstream prompt = %q, want the trimmed stdin text", got)
	}
}

// A positional prompt wins outright: stdin is never read, so an open-but-idle
// pipe handed over by an agent harness cannot hang the CLI.
func TestPositionalPromptNeverReadsStdin(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	pipe := &silentPipe{read: make(chan struct{})}
	h.stdinReader = pipe

	done := make(chan int, 1)
	go func() { done <- h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png")) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
		}
	case <-pipe.read:
		t.Fatal("run() read stdin even though a positional prompt was given")
	case <-time.After(10 * time.Second):
		t.Fatal("run() blocked, most likely on stdin")
	}
	if got := f.onlyCall(t).body["prompt"]; got != "a red bicycle" {
		t.Errorf("upstream prompt = %q, want the positional argument", got)
	}
}

func TestPositionalPromptWinsOverStdinContent(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	h.stdin = "a prompt from stdin"

	if code := h.run("a prompt from the argument", "-o", filepath.Join(dir, "hero.png")); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := f.onlyCall(t).body["prompt"]; got != "a prompt from the argument" {
		t.Errorf("upstream prompt = %q, want the positional argument", got)
	}
}

func TestFlagsMayPrecedeThePromptArgument(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("--size", "1024x1024", "-o", filepath.Join(dir, "hero.png"), "a red bicycle"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := f.onlyCall(t).body["prompt"]; got != "a red bicycle" {
		t.Errorf("upstream prompt = %q, want %q", got, "a red bicycle")
	}
}

func TestDashDashTerminatesFlagsForAPromptStartingWithADash(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("-o", filepath.Join(dir, "hero.png"), "--", "-a dash-leading prompt"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := f.onlyCall(t).body["prompt"]; got != "-a dash-leading prompt" {
		t.Errorf("upstream prompt = %q, want the argument after --", got)
	}
}

func TestJSONFlagAcceptsAnExplicitBooleanValue(t *testing.T) {
	for _, tc := range []struct {
		arg      string
		wantJSON bool
	}{
		{"--json", true},
		{"--json=true", true},
		{"--json=false", false},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			f := newFakeUpstream(t)
			h := newHarnessWith(t, f)
			dir := chdirTemp(t)

			if code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), tc.arg); code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
			}
			isJSON := strings.HasPrefix(strings.TrimSpace(h.out()), "{")
			if isJSON != tc.wantJSON {
				t.Errorf("%s produced stdout %q, json mode = %v, want %v", tc.arg, h.out(), isJSON, tc.wantJSON)
			}
		})
	}
}

// The reporting mode is decided before parsing, so a rejected --json value
// must be reported the same way parseArgs would have reported it.
func TestJSONFlagWithANonBooleanValueIsUsageError(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"

	if code := h.run("a red bicycle", "--json=maybe"); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got := errorObject(t, h.out())["code"]; got != "usage_error" {
		t.Errorf("error code = %v, want usage_error", got)
	}
	if h.err() != "" {
		t.Errorf("stderr = %q, want empty: an invalid --json value still reports as JSON", h.err())
	}
}

func TestJSONFalseReportsFailuresOnStderr(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"

	if code := h.run("--json=false"); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if h.out() != "" {
		t.Errorf("stdout = %q, want empty with --json=false", h.out())
	}
	if !strings.Contains(h.err(), "prompt") {
		t.Errorf("stderr = %q, want the plain-text diagnostic", h.err())
	}
}

func TestFormatIsInferredFromOutputExtension(t *testing.T) {
	for _, tc := range []struct{ file, format string }{
		{"hero.jpg", "jpeg"},
		{"hero.JPEG", "jpeg"},
		{"hero.webp", "webp"},
		{"hero.png", "png"},
		{"hero.bin", "png"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			f := newFakeUpstream(t)
			h := newHarnessWith(t, f)
			dir := chdirTemp(t)

			if code := h.run("a red bicycle", "-o", filepath.Join(dir, tc.file)); code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
			}
			if got := f.onlyCall(t).body["output_format"]; got != tc.format {
				t.Errorf("output_format = %v, want %v", got, tc.format)
			}
		})
	}
}

func TestExplicitFormatConflictingWithExtensionIsUsageError(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--format", "jpeg")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(h.err(), "format") {
		t.Errorf("stderr = %q, want it to explain the format conflict", h.err())
	}
	if n := len(f.calls()); n != 0 {
		t.Errorf("upstream request count = %d, want 0 for a usage error", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "hero.png")); !os.IsNotExist(err) {
		t.Errorf("output file exists, want no file written on a usage error")
	}
}

func TestExplicitFormatAgreeingWithExtensionIsAccepted(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.jpg"), "--format", "jpeg"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := f.onlyCall(t).body["output_format"]; got != "jpeg" {
		t.Errorf("output_format = %v, want jpeg", got)
	}
}

func TestJSONModeSuccessShape(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	target := filepath.Join(dir, "hero.png")

	if code := h.run("a red bicycle", "-o", target, "--size", "1536x1024", "--json"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if h.err() != "" {
		t.Errorf("stderr = %q, want empty", h.err())
	}

	got := decodeJSONOutput(t, h.out())
	if len(got) != 5 {
		t.Errorf("result keys = %v, want exactly path, size, format, model, duration_ms", got)
	}
	path, _ := got["path"].(string)
	if !filepath.IsAbs(path) {
		t.Errorf("path = %v, want an absolute path", got["path"])
	}
	if filepath.Base(path) != "hero.png" {
		t.Errorf("path = %v, want it to name hero.png", got["path"])
	}
	// The upstream is the authority on the size that was actually rendered.
	if got["size"] != "1024x1024" {
		t.Errorf("size = %v, want the size reported by the upstream (1024x1024)", got["size"])
	}
	if got["format"] != "png" {
		t.Errorf("format = %v, want png", got["format"])
	}
	if got["model"] != "gpt-image-2" {
		t.Errorf("model = %v, want gpt-image-2", got["model"])
	}
	if _, ok := got["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", got["duration_ms"])
	}
}

// A gateway that omits size leaves the requested value as the best answer.
func TestJSONModeFallsBackToTheRequestedSize(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.body = successBodyWithoutSize(fakeImage)

	code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.webp"), "--size", "1536x1024", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	got := decodeJSONOutput(t, h.out())
	if got["size"] != "1536x1024" {
		t.Errorf("size = %v, want the requested size as a fallback", got["size"])
	}
	if got["format"] != "webp" {
		t.Errorf("format = %v, want webp", got["format"])
	}
}

func TestJSONModeReportsAutoWhenNeitherSideKnowsTheSize(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)
	f.body = successBodyWithoutSize(fakeImage)

	if code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.png"), "--json"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if got := decodeJSONOutput(t, h.out())["size"]; got != "auto" {
		t.Errorf("size = %v, want auto", got)
	}
}
