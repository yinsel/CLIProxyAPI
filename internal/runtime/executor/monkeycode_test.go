package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	auth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	executor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	translator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

func TestMonkeyCodeExecutorsSignFinalTranslatedRequests(t *testing.T) {
	for _, protocol := range []string{"claude", "codex", "openai"} {
		for _, stream := range []bool{false, true} {
			t.Run(protocol+map[bool]string{false: "/json", true: "/stream"}[stream], func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					signature, err := helps.MonkeyCodeSignature(body, "omas_test_secret")
					if err != nil || r.Header.Get("X-OhMyAgent-Signature") != signature {
						t.Errorf("final body signature mismatch: %v", err)
					}
					if r.URL.RawQuery != "" {
						t.Errorf("query leaked: %s", r.URL.RawQuery)
					}
					if r.Header.Get("X-Api-Key") != "test" {
						t.Error("provider key missing")
					}
					switch protocol {
					case "codex":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}\n\n")
					case "claude":
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
						}
					default:
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
						}
					}
				}))
				defer server.Close()
				cfg := &config.Config{}
				credential := &auth.Auth{ID: "monkeycode-test", Provider: protocol, Attributes: map[string]string{"api_key": "test", "base_url": server.URL, "signing_secret": "omas_test_secret"}}
				var engine auth.ProviderExecutor
				var body string
				format := protocol
				switch protocol {
				case "claude":
					engine = NewClaudeExecutor(cfg)
					body = `{"model":"test-model","system":"original system","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
				case "codex":
					engine = NewCodexExecutor(cfg)
					format = "openai-response"
					body = `{"model":"test-model","instructions":"original system","input":[{"role":"user","content":"hi"}]}`
				default:
					engine = NewOpenAICompatExecutor("openai", cfg)
					body = `{"model":"test-model","messages":[{"role":"system","content":"original system"},{"role":"user","content":"hi"}]}`
				}
				opts := executor.Options{SourceFormat: translator.FromString(format), Stream: stream}
				request := executor.Request{Model: "test-model", Payload: []byte(body)}
				if stream {
					result, err := engine.ExecuteStream(context.Background(), credential, request, opts)
					if err != nil {
						t.Fatal(err)
					}
					for chunk := range result.Chunks {
						if chunk.Err != nil {
							t.Fatal(chunk.Err)
						}
					}
				} else {
					if _, err := engine.Execute(context.Background(), credential, request, opts); err != nil {
						t.Fatal(err)
					}
				}
				if calls != 1 {
					t.Fatalf("upstream calls = %d", calls)
				}
			})
		}
	}
}

func TestMonkeyCodeDisablesCodexWebsockets(t *testing.T) {
	credential := &auth.Auth{Attributes: map[string]string{"websockets": "true", "signing_secret": "omas_test"}}
	if codexWebsocketsEnabled(credential) {
		t.Fatal("signed websocket enabled")
	}
	delete(credential.Attributes, "signing_secret")
	if !codexWebsocketsEnabled(credential) {
		t.Fatal("unsigned websocket disabled")
	}
}
