package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditUploadsImagesAndMask(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "source.png")
			if err := os.WriteFile(input, fakeImage, 0600); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/images/edits" || r.Method != "POST" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("missing auth")
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("prompt") != "change the sky" || r.FormValue("output_format") != "webp" {
					t.Errorf("fields: %v", r.MultipartForm.Value)
				}
				if r.FormValue("moderation") != "auto" {
					t.Error("moderation not forwarded")
				}
				field := "image"
				if count > 1 {
					field = "image[]"
				}
				if len(r.MultipartForm.File[field]) != count || len(r.MultipartForm.File["mask"]) != 1 {
					t.Errorf("files: %v", r.MultipartForm.File)
				}
				for _, files := range r.MultipartForm.File {
					for _, file := range files {
						f, err := file.Open()
						if err != nil {
							t.Error(err)
							continue
						}
						data, err := io.ReadAll(f)
						f.Close()
						if err != nil || !bytes.Equal(data, fakeImage) {
							t.Error("upload bytes differ")
						}
					}
				}
				io.WriteString(w, successBody(fakeImage))
			}))
			defer server.Close()
			h := newHarness()
			h.env["OPENAI_API_KEY"] = "test-key"
			h.env["OPENAI_BASE_URL"] = server.URL + "/v1/"
			out := filepath.Join(dir, "out.webp")
			args := []string{"change the sky", "--mask", input, "-o", out, "--json"}
			for i := 0; i < count; i++ {
				args = append(args, "--image="+input)
			}
			if code := h.run(args...); code != 0 {
				t.Fatalf("exit %d: %s %s", code, h.out(), h.err())
			}
			data, err := os.ReadFile(out)
			if err != nil || !bytes.Equal(data, fakeImage) {
				t.Fatalf("output: %q, %v", data, err)
			}
		})
	}
}

func TestEditRejectsInvalidInputsBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		code      int
		errorCode string
	}{
		{"mask without image", []string{"--mask", "mask.png"}, 2, "usage_error"},
		{"empty image", []string{"--image="}, 2, "usage_error"},
		{"empty mask", []string{"--image", "source.png", "--mask="}, 2, "usage_error"},
		{"missing image", []string{"--image", filepath.Join(t.TempDir(), "missing.png")}, 4, "read_failed"},
		{"directory image", []string{"--image", t.TempDir()}, 4, "read_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeUpstream(t)
			h := newHarness()
			h.env["OPENAI_API_KEY"] = "key"
			h.env["OPENAI_BASE_URL"] = f.server.URL
			args := append([]string{"edit sky", "--json", "-o", filepath.Join(t.TempDir(), "out.png")}, tc.args...)
			if code := h.run(args...); code != tc.code {
				t.Fatalf("exit %d: %s", code, h.out())
			}
			if !strings.Contains(h.out(), `"code":"`+tc.errorCode+`"`) {
				t.Fatal(h.out())
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.requests) != 0 {
				t.Error("invalid input reached upstream")
			}
		})
	}
}

func TestEditResolvesAsyncResult(t *testing.T) {
	f := newFakeUpstream(t)
	f.asyncTaskID = "edit-task"
	dir := t.TempDir()
	input := filepath.Join(dir, "input.png")
	out := filepath.Join(dir, "result.png")
	if err := os.WriteFile(input, fakeImage, 0600); err != nil {
		t.Fatal(err)
	}
	h := newHarness()
	h.env["OPENAI_API_KEY"] = "key"
	h.env["OPENAI_BASE_URL"] = f.server.URL
	h.stdin = "change sky"
	if code := h.run("--image", input, "-o", out); code != 0 {
		t.Fatalf("exit %d: %s", code, h.err())
	}
	data, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(data, fakeImage) {
		t.Fatalf("output %q: %v", data, err)
	}
}

func TestAPIMartGPTEditUsesBase64AndPolls(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.png")
	mask := filepath.Join(dir, "mask.png")
	for path, data := range map[string][]byte{input: fakeImage, mask: append(append([]byte{}, fakeImage...), 1)} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		base, model string
		json        bool
	}{
		{"https://api.apimart.ai/v1/", "gpt-image-2-official", true},
		{"https://api.apimart.ai", "gpt-image-2", true},
		{"https://api.apimart.ai.example.com/v1", "gpt-image-2-official", false},
		{"https://api.apimart.ai/v1", "grok-imagine-image", false},
	} {
		t.Run(tc.base+tc.model, func(t *testing.T) {
			submitted := false
			polled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					polled = true
					io.WriteString(w, `{"data":{"status":"failed","error":{"message":"test rejection"}}}`)
					return
				}
				submitted = true
				if tc.json {
					if r.URL.Path != "/v1/images/generations" || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("wrong protocol: %s %s", r.URL.Path, r.Header.Get("Content-Type"))
					}
					var body struct {
						Images []string `json:"image_urls"`
						Mask   string   `json:"mask_url"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if len(body.Images) != 2 {
						t.Errorf("images: %v", body.Images)
					} else {
						for _, value := range body.Images {
							data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "data:image/png;base64,"))
							if err != nil || !bytes.Equal(data, fakeImage) {
								t.Error("image data mismatch")
							}
						}
					}
					data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(body.Mask, "data:image/png;base64,"))
					if err != nil || !bytes.Equal(data, append(append([]byte{}, fakeImage...), 1)) {
						t.Error("mask data mismatch")
					}
				} else if r.URL.Path != "/v1/images/edits" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
					t.Error("non-APIMart GPT request changed")
				}
				io.WriteString(w, `{"data":[{"task_id":"edit-task"}]}`)
			}))
			defer server.Close()
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.Proxy = nil
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			// Keep production URL routing visible while serving all traffic locally.
			original := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				r = r.Clone(r.Context())
				u := *r.URL
				u.Scheme = "http"
				r.URL = &u
				return transport.RoundTrip(r)
			})}
			defer func() { http.DefaultClient = original; transport.CloseIdleConnections() }()
			h := newHarness()
			h.env["OPENAI_API_KEY"] = "test-key"
			h.env["OPENAI_BASE_URL"] = tc.base
			code := h.run("edit", "--model", tc.model, "--image", input, "--image", input, "--mask", mask, "--json", "-o", filepath.Join(dir, "output.png"))
			if code != 3 || !strings.Contains(h.out(), "test rejection") || !submitted || !polled {
				t.Fatalf("exit %d: %s submitted=%v polled=%v", code, h.out(), submitted, polled)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
