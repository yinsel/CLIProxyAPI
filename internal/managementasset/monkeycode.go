package managementasset

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// The complete native provider editor is built from pinned sources in webui/.
//
//go:embed management.html.gz
var bundledManagementGZIP []byte

var bundledManagementOnce sync.Once
var bundledManagementHTML []byte
var bundledManagementErr error

// UseBundledManagementPanel keeps the default editor in sync with the engine.
// Explicit custom panel repositories and MANAGEMENT_STATIC_PATH remain supported.
func UseBundledManagementPanel(repository string) bool {
	repository = strings.TrimRight(strings.TrimSpace(repository), "/")
	return strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")) == "" &&
		(repository == "" || repository == config.DefaultPanelGitHubRepository)
}

func BundledManagementHTML() ([]byte, error) {
	bundledManagementOnce.Do(func() {
		reader, err := gzip.NewReader(bytes.NewReader(bundledManagementGZIP))
		if err != nil {
			bundledManagementErr = err
			return
		}
		bundledManagementHTML, bundledManagementErr = io.ReadAll(reader)
		if errClose := reader.Close(); bundledManagementErr == nil {
			bundledManagementErr = errClose
		}
	})
	return bundledManagementHTML, bundledManagementErr
}
