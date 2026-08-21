package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestLiveSmokeGeneration is the only test that spends money and touches the
// network. It exists to catch upstream drift that the fake server cannot see,
// so it is opt-in: VINCI_LIVE_TEST=1 go test ./... -run Live
func TestLiveSmokeGeneration(t *testing.T) {
	if os.Getenv("VINCI_LIVE_TEST") != "1" {
		t.Skip("live smoke test is opt-in; set VINCI_LIVE_TEST=1 to run it")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Fatal("VINCI_LIVE_TEST=1 requires OPENAI_API_KEY")
	}

	h := newHarness()
	h.env["OPENAI_API_KEY"] = apiKey
	h.env["OPENAI_BASE_URL"] = os.Getenv("OPENAI_BASE_URL")
	dir := chdirTemp(t)
	target := filepath.Join(dir, "live.png")

	code := h.run("a single flat red circle centred on a white background",
		"-o", target, "--size", "1024x1024", "--quality", "low", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout: %s, stderr: %s)", code, h.out(), h.err())
	}

	got := decodeJSONOutput(t, h.out())
	if got["model"] != "gpt-image-2" {
		t.Errorf("model = %v, want gpt-image-2", got["model"])
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading generated image: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Errorf("generated file is not a PNG (%d bytes)", len(data))
	}
}
