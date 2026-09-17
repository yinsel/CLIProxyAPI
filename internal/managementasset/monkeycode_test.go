package managementasset

import (
	"bytes"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"testing"
)

func TestMonkeyCodeBundledNativePanel(t *testing.T) {
	page, err := BundledManagementHTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"signing_secret", "monkeycode", "<html", "omas_"} {
		if !bytes.Contains(page, []byte(marker)) {
			t.Fatalf("native editor missing %s", marker)
		}
	}
	if bytes.Contains(page, []byte("cpa-monkeycode-settings")) {
		t.Fatal("legacy floating panel remains")
	}
}

func TestMonkeyCodeBundledPanelSelection(t *testing.T) {
	t.Setenv("MANAGEMENT_STATIC_PATH", "")
	for _, repo := range []string{"", config.DefaultPanelGitHubRepository} {
		if !UseBundledManagementPanel(repo) {
			t.Fatal("default editor not bundled")
		}
	}
	if UseBundledManagementPanel("https://github.com/example/custom") {
		t.Fatal("custom repository overridden")
	}
	t.Setenv("MANAGEMENT_STATIC_PATH", t.TempDir())
	if UseBundledManagementPanel("") {
		t.Fatal("custom static path overridden")
	}
}
