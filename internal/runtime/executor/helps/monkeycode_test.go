package helps

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	auth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestMonkeyCodeAuthRecognition(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs map[string]string
		want  bool
	}{
		{"native", map[string]string{"signing_secret": "omas_test"}, true},
		{"bridge", map[string]string{"base_url": "http://127.0.0.1:1234/monkeycode/route/v1", "header:X-EasyCLI-MonkeyCode": "metadata"}, true},
		{"bridge-ipv6", map[string]string{"base_url": "http://[::1]:1234/monkeycode/route/v1", "header:x-easycli-monkeycode": "metadata"}, true},
		{"ordinary", map[string]string{}, false},
		{"remote-marker", map[string]string{"base_url": "https://api.example/monkeycode/route/v1", "header:X-EasyCLI-MonkeyCode": "metadata"}, false},
		{"unmarked-local", map[string]string{"base_url": "http://127.0.0.1:1234/monkeycode/route/v1"}, false},
		{"other-path", map[string]string{"base_url": "http://127.0.0.1:1234/v1", "header:X-EasyCLI-MonkeyCode": "metadata"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsMonkeyCodeAuth(&auth.Auth{Attributes: tc.attrs}); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if IsMonkeyCodeAuth(nil) {
		t.Fatal("nil auth recognized")
	}
}

func TestMonkeyCodeResponsesNormalizationPreservesHistory(t *testing.T) {
	const body = `{"instructions":"Keep the prompt.","input":[{"role":"user","content":"hi"},{"type":"reasoning","id":"rs_1","content":null,"summary":[{"type":"summary_text","text":"Plan"}],"encrypted_content":"opaque"},{"type":"reasoning","content":[{"type":"reasoning_text","text":"Keep"}]},{"type":"reasoning","summary":[]},{"type":"function_call","call_id":"c1","name":"read_file","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":"file","content":null}]}`
	want := strings.Replace(body, `"content":null`, `"content":[]`, 1)
	for _, path := range []string{"/v1/responses", "/custom/v1/responses/compact"} {
		got, err := prepareMonkeyCodeBody(path, []byte(body))
		if err != nil || string(got) != want {
			t.Fatalf("normalization = %s, %v; want %s", got, err, want)
		}
		before, _ := MonkeyCodeSignature([]byte(body), "omas_test")
		after, _ := MonkeyCodeSignature(got, "omas_test")
		if before != after {
			t.Fatal("system prompt changed")
		}
		second, err := prepareMonkeyCodeBody(path, got)
		if err != nil || !bytes.Equal(second, got) {
			t.Fatal("normalization is not idempotent")
		}
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/models"} {
		got, err := prepareMonkeyCodeBody(path, []byte(body))
		if err != nil || string(got) != body {
			t.Fatalf("changed non-Responses body for %s", path)
		}
	}
	for _, raw := range []string{
		`{ "input":[] }`, `{"input":"text"}`,
		`{"input":[{"type":"reasoning","summary":[]}]}`,
		`{"input":[{"type":"reasoning","content":[]}]}`,
		`{"input":[{"type":"reasoning","content":"invalid but not null"}]}`,
		`{"input":[null,{"role":"assistant","content":null}]}`,
	} {
		got, err := prepareMonkeyCodeBody("/v1/responses", []byte(raw))
		if err != nil || string(got) != raw {
			t.Fatalf("changed unrelated body %s", raw)
		}
	}
	if _, err := prepareMonkeyCodeBody("/v1/responses", []byte("not json")); err == nil {
		t.Fatal("accepted invalid JSON")
	}
}

func TestMonkeyCodeCodexSecondTurn(t *testing.T) {
	for name, makeClient := range map[string]func(context.Context, *config.Config, *auth.Auth, time.Duration) *http.Client{
		"openai": NewProxyAwareHTTPClient, "claude-codex": NewUtlsHTTPClient,
	} {
		t.Run(name, func(t *testing.T) {
			const output = `[{"type":"reasoning","id":"rs_1","content":null,"summary":[],"encrypted_content":"opaque"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]`
			const event = `{"type":"response.completed","response":{"output":` + output + `}}`
			const sse = "data: " + event + "\n\n"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.URL.String() != "/v1/responses" || r.ContentLength != int64(len(body)) {
					t.Errorf("unexpected URL/length: %s, %d vs %d", r.URL, r.ContentLength, len(body))
				}
				expected, _ := MonkeyCodeSignature(body, "omas_test")
				if r.Header.Get(monkeyCodeSignatureHeader) != expected || r.Header.Get("X-Api-Key") != "oma_test" {
					t.Error("invalid final body signature or paired key")
				}
				var payload struct {
					Input []struct {
						Type      string          `json:"type"`
						Content   json.RawMessage `json:"content"`
						Encrypted string          `json:"encrypted_content"`
					} `json:"input"`
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Error(err)
				}
				for _, item := range payload.Input {
					if item.Type == "reasoning" {
						var content []json.RawMessage
						if err := json.Unmarshal(item.Content, &content); err != nil || content == nil {
							http.Error(w, "content: invalid type: null, expected a sequence", 422)
							return
						}
						if item.Encrypted != "opaque" {
							t.Error("lost encrypted reasoning")
						}
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, sse)
			}))
			defer upstream.Close()
			client := makeClient(context.Background(), nil, &auth.Auth{Attributes: map[string]string{"signing_secret": "omas_test", "api_key": "oma_test"}}, 0)
			input := []json.RawMessage{json.RawMessage(`{"role":"user","content":"Hello"}`)}
			for turn := 0; turn < 2; turn++ {
				body, _ := json.Marshal(map[string]any{"instructions": "You are a helpful assistant.", "input": input, "stream": true})
				req, _ := http.NewRequest(http.MethodPost, upstream.URL+"/v1/responses?beta=true", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(res.Body)
				_ = res.Body.Close()
				if err != nil || res.StatusCode != http.StatusOK || string(data) != sse {
					t.Fatalf("turn %d: status %d, body %s, error %v", turn+1, res.StatusCode, data, err)
				}
				if turn == 0 {
					var completion struct {
						Response struct {
							Output []json.RawMessage `json:"output"`
						} `json:"response"`
					}
					if err := json.Unmarshal(bytes.TrimSpace(bytes.TrimPrefix(data, []byte("data: "))), &completion); err != nil {
						t.Fatal(err)
					}
					input = append(input, completion.Response.Output...)
					input = append(input, json.RawMessage(`{"role":"user","content":"Continue"}`))
				}
			}
		})
	}
}

func TestMonkeyCodeSignature(t *testing.T) {
	const secret = "omas_test_secret"
	const want = "v1=b529b56a975dabea4b14956e84dbce9dd6c00dc3b6017e41a961899a9fa47a85"
	for _, body := range []string{
		`{"instructions":"你好\n  world "}`,
		`{"system":[{"text":"你好\n  world "},{"text":"ignored"}],"instructions":"ignored"}`,
		`{"messages":[null,{"role":"system","content":"你好\n  world "}]}`,
		`{"input":[{"role":"developer","content":[{"text":"你好\n  world "}]}]}`,
		`{"system":[{"text":""},{"text":"ignored"}],"instructions":"你好\n  world "}`,
	} {
		got, err := MonkeyCodeSignature([]byte(body), secret)
		if err != nil || got != want {
			t.Fatalf("signature = %q, %v; want %q", got, err, want)
		}
	}
	for _, body := range []string{`{}`, `{"messages":[{"role":"user","content":"hi"}]}`, `{"input":"invalid","instructions":"hi"}`, `{"messages":[{"role":"system","content":""},{"role":"system","content":"ignored"}]}`} {
		if _, err := MonkeyCodeSignature([]byte(body), secret); err == nil {
			t.Fatalf("accepted missing or malformed prompt: %s", body)
		}
	}
	if _, err := MonkeyCodeSignature([]byte(`{"system":"hi"}`), ""); err == nil {
		t.Fatal("accepted empty secret")
	}
}

func TestMonkeyCodeClientsSignFinalBodyAndStripAllQueries(t *testing.T) {
	for name, makeClient := range map[string]func(context.Context, *config.Config, *auth.Auth, time.Duration) *http.Client{
		"openai": NewProxyAwareHTTPClient, "claude-codex": NewUtlsHTTPClient,
	} {
		t.Run(name, func(t *testing.T) {
			const body = `{ "model":"test", "system":"hello", "stream":true }`
			expected, _ := MonkeyCodeSignature([]byte(body), "omas_test_secret")
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Path != "/custom/v1/messages" {
					t.Errorf("unexpected URL %s", r.URL)
				}
				if r.Header.Get(monkeyCodeSignatureHeader) != expected {
					t.Error("invalid signature")
				}
				if r.Header.Get("X-Api-Key") != "test-key" {
					t.Error("missing provider key")
				}
				data, _ := io.ReadAll(r.Body)
				if string(data) != body {
					t.Error("body changed")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: ok\n\n")
			}))
			defer upstream.Close()
			client := makeClient(context.Background(), nil, &auth.Auth{Attributes: map[string]string{"signing_secret": "omas_test_secret", "api_key": "test-key"}}, 0)
			req, _ := http.NewRequest(http.MethodPost, upstream.URL+"/custom/v1/messages?beta=true&trace=1&tag=a&tag=b&encoded=a%26b", strings.NewReader(body))
			req.Header.Set(monkeyCodeSignatureHeader, "stale")
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			data, _ := io.ReadAll(res.Body)
			if string(data) != "data: ok\n\n" {
				t.Error("stream changed")
			}
			if req.URL.Query().Get("beta") != "true" {
				t.Error("mutated original URL")
			}
		})
	}
}

func TestMonkeyCodeOptInAndRedirects(t *testing.T) {
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	client := &http.Client{Transport: base}
	ConfigureMonkeyCodeClient(client, &auth.Auth{})
	if _, ok := client.Transport.(roundTripperFunc); !ok {
		t.Fatal("changed unsigned transport")
	}
	ConfigureMonkeyCodeClient(client, &auth.Auth{Attributes: map[string]string{"signing_secret": "test"}})
	if client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("signed redirects enabled")
	}
	req, _ := http.NewRequest(http.MethodPost, "http://test/v1/messages?beta=true", strings.NewReader(`{}`))
	if _, err := client.Do(req); err == nil {
		t.Fatal("sent unsigned prompt")
	}
	req, _ = http.NewRequest(http.MethodGet, "http://test/v1/models?beta=true", nil)
	if res, err := client.Do(req); err != nil {
		t.Fatal(err)
	} else {
		res.Body.Close()
	}
}
