package main

import (
	"bytes"
	"strings"
	"testing"
)

// harness drives the single seam run() the way a shell would, so every test
// asserts only externally observable behaviour: exit code, stdout, stderr.
type harness struct {
	env    map[string]string
	stdin  string
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newHarness() *harness {
	return &harness{env: map[string]string{}}
}

func (h *harness) run(args ...string) int {
	lookup := func(key string) string { return h.env[key] }
	return run(args, lookup, strings.NewReader(h.stdin), &h.stdout, &h.stderr)
}

func (h *harness) out() string { return h.stdout.String() }
func (h *harness) err() string { return h.stderr.String() }

func TestVersionPrintsVersionToStdout(t *testing.T) {
	h := newHarness()
	if code := h.run("--version"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, h.err())
	}
	if strings.TrimSpace(h.out()) == "" {
		t.Errorf("stdout is empty, want a version string")
	}
	if h.err() != "" {
		t.Errorf("stderr = %q, want empty", h.err())
	}
}

func TestHelpPrintsUsageToStdout(t *testing.T) {
	h := newHarness()
	if code := h.run("--help"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, want := range []string{"vinci", "--output", "--size", "--quality", "--background", "--format", "--moderation", "--timeout", "--json", "OPENAI_API_KEY", "OPENAI_BASE_URL"} {
		if !strings.Contains(h.out(), want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestNoPromptIsUsageError(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"
	if code := h.run(); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if h.out() != "" {
		t.Errorf("stdout = %q, want empty", h.out())
	}
	if !strings.Contains(h.err(), "prompt") {
		t.Errorf("stderr = %q, want it to mention the missing prompt", h.err())
	}
}

func TestPromptFromBothArgAndStdinIsUsageError(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"
	h.stdin = "a prompt from stdin"
	if code := h.run("a prompt from the argument"); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(h.err(), "stdin") {
		t.Errorf("stderr = %q, want it to mention stdin", h.err())
	}
}

func TestTwoPositionalArgsIsUsageError(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"
	if code := h.run("first", "second"); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "test-key"
	if code := h.run("--nope", "a prompt"); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(h.err(), "--nope") {
		t.Errorf("stderr = %q, want it to name the unknown flag", h.err())
	}
}
