# Bundled management editor

The engine serves a native React management editor at `/management.html`. OpenAI compatibility, Codex and Claude provider forms contain **Advanced settings → MonkeyCode 支持**. The field uses the existing provider save operation; clearing it disables signing. It requires one paired API key and an explicit Base URL. Codex WebSockets are disabled while signing is enabled.

Model connectivity tests send the current draft secret to the authenticated management API, which signs the final protocol body using the same transport as normal execution. Tests add a system prompt and use HTTP/SSE. Unsaved changes, key rotation and explicit clearing are honored without persisting the draft. GET model discovery has no prompt signature. The existing management authentication remains required; draft signing is limited to HTTP(S) POST model endpoints, rejects Host overrides, and validates the prompt before sending. It does not create a new outbound proxy endpoint.

The panel is embedded to prevent an upstream asset refresh from replacing the editor with a version missing these fields. `MANAGEMENT_STATIC_PATH` or an explicit third-party `panel-github-repository` still selects a custom panel, which must supply its own UI support.

## Rebuild

Install Git, gzip, Node.js 24 and Bun 1.3.14, then run `bash webui/build.sh` from the repository. The script checks out the exact `UPSTREAM_REVISION`, applies `monkeycode.patch`, installs locked dependencies and builds the single-file frontend. Commit the patch, revision and generated `internal/managementasset/management.html.gz` together. Ordinary Go builds require no frontend tools or network downloads.

To update the frontend, apply the patch to the new upstream checkout, resolve conflicts, run its tests/lint/build, regenerate the patch against that revision, and rebuild. The upstream editor is MIT-licensed; its license is retained here.

## Validation of this change

- Frontend type checking, ESLint and the production single-file build passed with versions from the pinned Bun lockfile.
- Eight provider serialization and draft-probe regression tests passed (`tests/monkeycode.test.ts`).
- Go tests cover all three protocol probes with unsaved, rotated, saved and explicitly cleared secrets, actual HMAC verification, paired keys and query removal.
- A compiled engine plus the compiled React page passed a DOM interaction check for the advanced field, Codex WebSocket interlock, unsaved model tests, save/reopen and clearing. A live proxy request also verified config hot reload.
- The local environment could not run Bun's test runner; the targeted frontend tests used Node's test runner instead. Chromium was unavailable, so DOM interactions were checked with Happy DOM, not a visual browser test. No real upstream credentials were used.
