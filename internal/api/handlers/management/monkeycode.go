package management

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type monkeyCodeProvider struct {
	Section       string `json:"section"`
	Index         int    `json:"index"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	SigningSecret string `json:"signing_secret"`
	Revision      string `json:"revision"`
}

func monkeyCodeRevision(entry any) string {
	data, _ := json.Marshal(entry)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// GetMonkeyCode exposes the same provider settings as the desktop client.
func (h *Handler) GetMonkeyCode(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	providers := make([]monkeyCodeProvider, 0)
	for i, entry := range h.cfg.OpenAICompatibility {
		providers = append(providers, monkeyCodeProvider{"openai-compatibility", i, entry.Name, entry.BaseURL, entry.SigningSecret, monkeyCodeRevision(entry)})
	}
	for i, entry := range h.cfg.CodexKey {
		providers = append(providers, monkeyCodeProvider{"codex-api-key", i, "Codex", entry.BaseURL, entry.SigningSecret, monkeyCodeRevision(entry)})
	}
	for i, entry := range h.cfg.ClaudeKey {
		providers = append(providers, monkeyCodeProvider{"claude-api-key", i, "Claude Messages", entry.BaseURL, entry.SigningSecret, monkeyCodeRevision(entry)})
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

// PatchMonkeyCode checks the record revision before updating, so concurrent edits
// or reordering cannot attach a secret to a different credential.
func (h *Handler) PatchMonkeyCode(c *gin.Context) {
	var body struct {
		Section       string  `json:"section"`
		Index         *int    `json:"index"`
		Revision      string  `json:"revision"`
		SigningSecret *string `json:"signing_secret"`
	}
	if c.ShouldBindJSON(&body) != nil || body.Index == nil || *body.Index < 0 || body.SigningSecret == nil || len(*body.SigningSecret) > 4096 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid MonkeyCode settings"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	i := *body.Index
	var entry any
	var secret *string
	switch body.Section {
	case "openai-compatibility":
		if i < len(h.cfg.OpenAICompatibility) {
			entry = h.cfg.OpenAICompatibility[i]
			secret = &h.cfg.OpenAICompatibility[i].SigningSecret
		}
	case "codex-api-key":
		if i < len(h.cfg.CodexKey) {
			entry = h.cfg.CodexKey[i]
			secret = &h.cfg.CodexKey[i].SigningSecret
		}
	case "claude-api-key":
		if i < len(h.cfg.ClaudeKey) {
			entry = h.cfg.ClaudeKey[i]
			secret = &h.cfg.ClaudeKey[i].SigningSecret
		}
	}
	if secret == nil || body.Revision != monkeyCodeRevision(entry) {
		c.JSON(http.StatusConflict, gin.H{"error": "Provider changed; reload before saving"})
		return
	}
	previous := *secret
	*secret = *body.SigningSecret
	if !h.persistLocked(c) {
		*secret = previous
	}
}
