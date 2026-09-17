package helps

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

const monkeyCodeSignatureHeader = "X-OhMyAgent-Signature"

type monkeyCodeError string

func (e monkeyCodeError) Error() string   { return string(e) }
func (e monkeyCodeError) StatusCode() int { return http.StatusBadRequest }

// MonkeyCodeSignature signs the final protocol payload, using the full secret as raw bytes.
func MonkeyCodeSignature(body []byte, secret string) (string, error) {
	if secret == "" {
		return "", monkeyCodeError("MonkeyCode signing_secret is empty")
	}
	var payload struct {
		System       json.RawMessage `json:"system"`
		Instructions json.RawMessage `json:"instructions"`
		Messages     []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Input []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", monkeyCodeError("Invalid MonkeyCode prompt fields")
	}
	prompt := monkeyCodePromptText(payload.System, true)
	if prompt == "" {
		prompt = monkeyCodePromptText(payload.Instructions, false)
	}
	if prompt == "" {
		for _, message := range payload.Messages {
			if message.Role == "system" {
				prompt = monkeyCodePromptText(message.Content, false)
				break
			}
		}
	}
	if prompt == "" {
		for _, message := range payload.Input {
			if message.Role == "system" || message.Role == "developer" {
				prompt = monkeyCodePromptText(message.Content, false)
				break
			}
		}
	}
	if prompt == "" {
		return "", monkeyCodeError("MonkeyCode requires a non-empty system prompt")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(prompt))
	return "v1=" + hex.EncodeToString(mac.Sum(nil)), nil
}

func monkeyCodePromptText(raw json.RawMessage, firstOnly bool) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil || len(blocks) == 0 {
		return ""
	}
	if firstOnly {
		return blocks[0].Text
	}
	var texts []string
	for _, block := range blocks {
		if block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// ConfigureMonkeyCodeClient signs final requests only for explicitly configured credentials.
func ConfigureMonkeyCodeClient(client *http.Client, auth *cliproxyauth.Auth) {
	if auth == nil || auth.Attributes["signing_secret"] == "" {
		return
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = &monkeyCodeTransport{base: base, secret: auth.Attributes["signing_secret"], apiKey: auth.Attributes["api_key"]}
	// A redirected request must not disclose the provider key or prompt signature.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
}

type monkeyCodeTransport struct {
	base           http.RoundTripper
	secret, apiKey string
}

func (t *monkeyCodeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	request.URL.RawQuery = ""
	request.URL.ForceQuery = false
	request.URL.Fragment = ""
	for name := range request.Header {
		if strings.EqualFold(name, monkeyCodeSignatureHeader) {
			delete(request.Header, name)
		}
	}
	if request.Method == http.MethodPost && monkeyCodePromptEndpoint(request.URL.Path) {
		if encoding := request.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
			return nil, monkeyCodeError("Compressed MonkeyCode prompt bodies are not supported")
		}
		if req.Body == nil {
			return nil, monkeyCodeError("MonkeyCode requires a request body")
		}
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read MonkeyCode request body: %w", err)
		}
		signature, err := MonkeyCodeSignature(body, t.secret)
		if err != nil {
			return nil, err
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.Header.Set(monkeyCodeSignatureHeader, signature)
	}
	if t.apiKey != "" {
		for name := range request.Header {
			if strings.EqualFold(name, "X-Api-Key") {
				delete(request.Header, name)
			}
		}
		request.Header.Set("X-Api-Key", t.apiKey)
	}
	return t.base.RoundTrip(request)
}

func monkeyCodePromptEndpoint(path string) bool {
	for _, suffix := range []string{"/chat/completions", "/responses", "/responses/compact", "/messages", "/messages/count_tokens"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
