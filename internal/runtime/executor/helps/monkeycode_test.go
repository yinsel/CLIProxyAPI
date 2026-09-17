package helps

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	auth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

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
