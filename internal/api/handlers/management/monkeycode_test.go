package management

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMonkeyCodeManagementRoundTripAndLegacySaves(t *testing.T) {
	for _, section := range []string{"claude-api-key", "codex-api-key", "openai-compatibility"} {
		t.Run(section, func(t *testing.T) {
			cfg := &config.Config{
				ClaudeKey:           []config.ClaudeKey{{APIKey: "test", BaseURL: "https://test"}},
				CodexKey:            []config.CodexKey{{APIKey: "test", BaseURL: "https://test/v1"}},
				OpenAICompatibility: []config.OpenAICompatibility{{Name: "test", BaseURL: "https://test/v1", APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "test"}}}},
			}
			h := &Handler{cfg: cfg, configFilePath: writeTestConfigFile(t)}
			call := func(handler gin.HandlerFunc, method, body string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(method, "/v0/management/"+section, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				handler(c)
				return rec
			}
			get := func() monkeyCodeProvider {
				rec := call(h.GetMonkeyCode, "GET", "")
				var result struct {
					Providers []monkeyCodeProvider `json:"providers"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				for _, p := range result.Providers {
					if p.Section == section {
						return p
					}
				}
				t.Fatal("provider missing")
				return monkeyCodeProvider{}
			}
			before := get()
			body, _ := json.Marshal(map[string]any{"section": section, "index": 0, "revision": before.Revision, "signing_secret": "omas_test_secret"})
			if rec := call(h.PatchMonkeyCode, "PATCH", string(body)); rec.Code != 200 {
				t.Fatalf("patch: %s", rec.Body)
			}
			if get().SigningSecret != "omas_test_secret" {
				t.Fatal("secret not readable")
			}
			saved, err := os.ReadFile(h.configFilePath)
			if err != nil || !strings.Contains(string(saved), "omas_test_secret") {
				t.Fatal("secret not persisted")
			}
			if rec := call(h.PatchMonkeyCode, "PATCH", string(body)); rec.Code != 409 {
				t.Fatal("accepted stale provider revision")
			}
			var put, patch gin.HandlerFunc
			var record map[string]any
			switch section {
			case "claude-api-key":
				put = h.PutClaudeKeys
				patch = h.PatchClaudeKey
				record = map[string]any{"api-key": "test", "base-url": "https://test"}
			case "codex-api-key":
				put = h.PutCodexKeys
				patch = h.PatchCodexKey
				record = map[string]any{"api-key": "test", "base-url": "https://test/v1"}
			default:
				put = h.PutOpenAICompat
				patch = h.PatchOpenAICompat
				record = map[string]any{"name": "test", "base-url": "https://test/v1", "api-key-entries": []map[string]string{{"api-key": "test"}}}
			}
			legacy, _ := json.Marshal([]any{record})
			if rec := call(put, "PUT", string(legacy)); rec.Code != 200 {
				t.Fatalf("legacy save: %s", rec.Body)
			}
			if get().SigningSecret != "omas_test_secret" {
				t.Fatal("legacy panel erased secret")
			}
			if rec := call(patch, "PATCH", `{"index":0,"value":{"signing_secret":"omas_rotated"}}`); rec.Code != 200 {
				t.Fatal(rec.Body)
			}
			if get().SigningSecret != "omas_rotated" {
				t.Fatal("native PATCH did not rotate secret")
			}
			record["signing_secret"] = ""
			clear, _ := json.Marshal(map[string]any{"items": []any{record}})
			if rec := call(put, "PUT", string(clear)); rec.Code != 200 {
				t.Fatal(rec.Body)
			}
			if get().SigningSecret != "" {
				t.Fatal("explicit empty secret not cleared")
			}
		})
	}
}

func TestMonkeyCodePreserveDoesNotCopyToDifferentCredential(t *testing.T) {
	previous := []config.ClaudeKey{{APIKey: "old", BaseURL: "https://test", SigningSecret: "omas_old"}}
	next := []config.ClaudeKey{{APIKey: "new", BaseURL: "https://test"}}
	preserveMonkeyCodeSecrets([]byte(`[{"api-key":"new","base-url":"https://test"}]`), next, previous)
	if next[0].SigningSecret != "" {
		t.Fatal("copied secret to another credential")
	}
}
