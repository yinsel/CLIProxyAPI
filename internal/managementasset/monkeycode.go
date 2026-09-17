package managementasset

import (
	"bytes"
	_ "embed"
)

//go:embed monkeycode.js
var monkeyCodePanel []byte

// WithMonkeyCodePanel augments the downloaded panel at serve time so asset
// updates do not remove the native MonkeyCode settings entry point.
func WithMonkeyCodePanel(page []byte) []byte {
	script := append([]byte("<script data-cpa-monkeycode>\n"), monkeyCodePanel...)
	script = append(script, []byte("\n</script>")...)
	lower := bytes.ToLower(page)
	if start := bytes.Index(lower, []byte("<head")); start >= 0 {
		if end := bytes.IndexByte(page[start:], '>'); end >= 0 {
			pos := start + end + 1
			out := make([]byte, 0, len(page)+len(script))
			out = append(out, page[:pos]...)
			out = append(out, script...)
			return append(out, page[pos:]...)
		}
	}
	return append(script, page...)
}
