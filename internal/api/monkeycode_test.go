package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestMonkeyCodeManagementHTML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MANAGEMENT_STATIC_PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "management.html"), []byte("<!doctype html><html><head></head><body>upstream panel</body></html>"), 0600); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: &config.Config{}, configFilePath: filepath.Join(dir, "config.yaml")}
	engine := gin.New()
	engine.GET("/management.html", server.serveManagementControlPanel)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/management.html", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "upstream panel") {
		t.Fatal("custom management panel not preserved")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("management panel may be stale")
	}
	t.Setenv("MANAGEMENT_STATIC_PATH", "")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/management.html", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "signing_secret") || strings.Contains(response.Body.String(), "upstream panel") {
		t.Fatal("bundled native editor not served")
	}
	server.cfg.RemoteManagement.DisableControlPanel = true
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/management.html", nil))
	if response.Code != 404 {
		t.Fatal("disabled panel still served")
	}
}
