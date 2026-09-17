#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
source_dir="$(mktemp -d)"
trap 'rm -rf "$source_dir"' EXIT
revision="$(cat "$repo_dir/webui/UPSTREAM_REVISION")"
git -C "$source_dir" init --quiet
git -C "$source_dir" remote add origin https://github.com/router-for-me/Cli-Proxy-API-Management-Center.git
git -C "$source_dir" fetch --depth=1 origin "$revision"
git -C "$source_dir" checkout --detach FETCH_HEAD
git -C "$source_dir" apply "$repo_dir/webui/monkeycode.patch"
cd "$source_dir"
bun install --frozen-lockfile
VERSION="monkeycode-${revision:0:8}" bun run build
gzip -n -9 -c dist/index.html > "$repo_dir/internal/managementasset/management.html.gz"
