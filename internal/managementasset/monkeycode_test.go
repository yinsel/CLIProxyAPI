package managementasset

import (
	"bytes"
	"testing"
)

func TestMonkeyCodePanelSurvivesAssetUpdates(t *testing.T) {
	for _, page := range []string{"<!doctype html><html><head><script>original()</script></head><body>hello</body></html>", "<HTML><HEAD lang='en'></HEAD><BODY>new release</BODY></HTML>", "<body>fallback</body>"} {
		got := WithMonkeyCodePanel([]byte(page))
		if !bytes.Contains(got, []byte("data-cpa-monkeycode")) || !bytes.Contains(got, []byte("signing_secret")) {
			t.Fatal("missing settings panel")
		}
		if p := bytes.Index(got, []byte("original()")); p >= 0 && bytes.Index(got, []byte("data-cpa-monkeycode")) > p {
			t.Fatal("authentication observer loaded too late")
		}
		if bytes.Contains(monkeyCodePanel, []byte("</script")) {
			t.Fatal("embedded script closes its own tag")
		}
	}
}
