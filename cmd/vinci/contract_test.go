package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if got["size"] != "1536x1024" {
		t.Errorf("size = %v, want 1536x1024", got["size"])
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

func TestJSONModeSuccessReportsAutoSizeWhenNotRequested(t *testing.T) {
	f := newFakeUpstream(t)
	h := newHarnessWith(t, f)
	dir := chdirTemp(t)

	if code := h.run("a red bicycle", "-o", filepath.Join(dir, "hero.webp"), "--json"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	got := decodeJSONOutput(t, h.out())
	if got["size"] != "auto" {
		t.Errorf("size = %v, want auto", got["size"])
	}
	if got["format"] != "webp" {
		t.Errorf("format = %v, want webp", got["format"])
	}
}
