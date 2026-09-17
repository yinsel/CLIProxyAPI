package synthesizer

import (
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"testing"
	"time"
)

func TestMonkeyCodeConfigSynthesis(t *testing.T) {
	cfg := &config.Config{
		ClaudeKey:           []config.ClaudeKey{{APIKey: "claude", BaseURL: "https://test", SigningSecret: "omas_claude"}},
		CodexKey:            []config.CodexKey{{APIKey: "codex", BaseURL: "https://test/v1", SigningSecret: "omas_codex", Websockets: true}},
		OpenAICompatibility: []config.OpenAICompatibility{{Name: "custom", BaseURL: "https://test/v1", SigningSecret: "omas_openai", APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "openai"}}}},
	}
	auths, err := NewConfigSynthesizer().Synthesize(&SynthesisContext{Config: cfg, Now: time.Now(), IDGenerator: NewStableIDGenerator()})
	if err != nil {
		t.Fatal(err)
	}
	if len(auths) != 3 {
		t.Fatalf("got %d credentials", len(auths))
	}
	for _, a := range auths {
		if a.Attributes["signing_secret"] != "omas_"+a.Attributes["api_key"] {
			t.Fatal("secret lost during synthesis")
		}
		if a.Provider == "codex" && a.Attributes["websockets"] == "true" {
			t.Fatal("signed Codex must use HTTP")
		}
	}
}
