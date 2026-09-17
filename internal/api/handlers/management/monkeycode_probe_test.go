package management

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestMonkeyCodeDraftProbe(t *testing.T) {
	for _, protocol := range []struct{ name, path, body string }{
		{"openai", "/v1/chat/completions", `{"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"hi"}],"stream":true}`},
		{"codex", "/v1/responses", `{"instructions":"You are a helpful assistant.","input":[{"role":"user","content":"hi"}],"stream":true}`},
		{"codex-history", "/v1/responses", `{"instructions":"You are a helpful assistant.","input":[{"role":"user","content":"hi"},{"type":"reasoning","content":null,"summary":[],"encrypted_content":"opaque"},{"role":"user","content":"continue"}],"stream":true}`},
		{"claude", "/v1/messages", `{"system":"You are a helpful assistant.","messages":[{"role":"user","content":"hi"}],"stream":true}`},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			manager := coreauth.NewManager(nil, nil, nil)
			saved, err := manager.Register(context.Background(), &coreauth.Auth{ID: "probe-key", Provider: protocol.name, Attributes: map[string]string{"api_key": "saved-key", "signing_secret": "omas_saved"}})
			if err != nil {
				t.Fatal(err)
			}
			saved.EnsureIndex()
			for _, scenario := range []struct {
				name, secret    string
				draft, selected bool
			}{
				{"unsaved", "omas_draft_原样", true, false},
				{"rotated", "omas_rotated", true, true},
				{"cleared", "", true, true},
				{"saved", "omas_saved", false, true},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					key := "draft-key"
					if !scenario.draft {
						key = "saved-key"
					}
					calls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						data, _ := io.ReadAll(r.Body)
						wantBody := protocol.body
						if scenario.secret != "" && protocol.name == "codex-history" {
							wantBody = strings.Replace(wantBody, `"content":null`, `"content":[]`, 1)
						}
						if string(data) != wantBody {
							t.Error("probe body changed")
						}
						signature := r.Header.Get("X-OhMyAgent-Signature")
						if scenario.secret != "" {
							mac := hmac.New(sha256.New, []byte(scenario.secret))
							_, _ = mac.Write([]byte("You are a helpful assistant."))
							if signature != "v1="+hex.EncodeToString(mac.Sum(nil)) {
								t.Error("wrong signature")
							}
							if r.URL.RawQuery != "" || r.Header.Get("X-Api-Key") != key {
								t.Error("query or paired key incorrect")
							}
						} else if signature != "" || r.URL.RawQuery != "beta=true&other=1" {
							t.Error("cleared probe still modified")
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = w.Write([]byte("data: {\"ok\":true}\n\ndata: [DONE]\n\n"))
					}))
					defer upstream.Close()
					headers := map[string]string{"Authorization": "Bearer " + key}
					if protocol.name == "claude" {
						headers = map[string]string{"x-api-key": key}
					}
					request := map[string]any{"method": "POST", "url": upstream.URL + protocol.path + "?beta=true&other=1", "header": headers, "data": protocol.body}
					if scenario.selected {
						request["auth_index"] = saved.Index
					}
					if scenario.draft {
						request["signing_secret"] = scenario.secret
					}
					body, _ := json.Marshal(request)
					h := &Handler{cfg: &config.Config{}, authManager: manager}
					router := gin.New()
					router.POST("/", h.APICall)
					response := httptest.NewRecorder()
					req := httptest.NewRequest("POST", "/", strings.NewReader(string(body)))
					req.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(response, req)
					if response.Code != 200 || calls != 1 {
						t.Fatalf("probe failed: %d %s", response.Code, response.Body.String())
					}
					if auth := h.authByIndex(saved.Index); auth == nil || auth.Attributes["signing_secret"] != "omas_saved" || auth.Attributes["api_key"] != "saved-key" {
						t.Fatal("draft modified saved auth")
					}
				})
			}
		})
	}
}

func TestMonkeyCodeDraftProbeRejectsOtherRequests(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) }))
	defer upstream.Close()
	for _, tc := range []struct {
		name, method, path, body string
		host                     bool
	}{
		{"get", "GET", "/v1/responses", `{"instructions":"prompt"}`, false},
		{"other-endpoint", "POST", "/admin/delete", `{"instructions":"prompt"}`, false},
		{"missing-prompt", "POST", "/v1/responses", `{"input":[]}`, false},
		{"invalid-json", "POST", "/v1/responses", `{`, false},
		{"host-override", "POST", "/v1/responses", `{"instructions":"prompt"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{"Authorization": "Bearer paired-key"}
			if tc.host {
				headers["Host"] = "other.invalid"
			}
			body, _ := json.Marshal(map[string]any{"method": tc.method, "url": upstream.URL + tc.path, "header": headers, "data": tc.body, "signing_secret": "omas_test"})
			h := &Handler{cfg: &config.Config{}}
			router := gin.New()
			router.POST("/", h.APICall)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			if rec.Code != 400 {
				t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	if calls != 0 {
		t.Fatal("invalid model tests reached upstream")
	}
}
