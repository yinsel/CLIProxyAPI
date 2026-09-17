package management

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// Legacy management panels do not serialize new fields. Retain an omitted secret
// only for the same provider and credentials; an explicit empty string clears it.
func preserveMonkeyCodeSecrets[T config.ClaudeKey | config.CodexKey | config.OpenAICompatibility](data []byte, entries, previous []T) {
	var raw []map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		var wrapper struct {
			Items []map[string]json.RawMessage `json:"items"`
		}
		if json.Unmarshal(data, &wrapper) != nil {
			return
		}
		raw = wrapper.Items
	}
	explicit := make(map[string]bool)
	for _, entry := range raw {
		if _, exists := entry["signing_secret"]; exists {
			explicit[monkeyCodeIdentity(entry)] = true
		}
	}
	secrets := make(map[string]string)
	counts := make(map[string]int)
	for _, entry := range previous {
		fields := monkeyCodeFields(entry)
		id := monkeyCodeIdentity(fields)
		counts[id]++
		var secret string
		_ = json.Unmarshal(fields["signing_secret"], &secret)
		secrets[id] = secret
	}
	for i := range entries {
		id := monkeyCodeIdentity(monkeyCodeFields(entries[i]))
		if explicit[id] || counts[id] != 1 {
			continue
		}
		switch entry := any(&entries[i]).(type) {
		case *config.ClaudeKey:
			entry.SigningSecret = secrets[id]
		case *config.CodexKey:
			entry.SigningSecret = secrets[id]
		case *config.OpenAICompatibility:
			entry.SigningSecret = secrets[id]
		}
	}
}

func monkeyCodeFields(value any) map[string]json.RawMessage {
	data, _ := json.Marshal(value)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	return fields
}

func monkeyCodeIdentity(fields map[string]json.RawMessage) string {
	parts := make([]string, 0, 4)
	for _, name := range []string{"name", "api-key", "base-url"} {
		var value string
		_ = json.Unmarshal(fields[name], &value)
		parts = append(parts, strings.TrimSpace(value))
	}
	var keys []struct {
		APIKey string `json:"api-key"`
	}
	_ = json.Unmarshal(fields["api-key-entries"], &keys)
	var values []string
	for _, key := range keys {
		values = append(values, strings.TrimSpace(key.APIKey))
	}
	sort.Strings(values)
	parts = append(parts, values...)
	id, _ := json.Marshal(parts)
	return string(id)
}
