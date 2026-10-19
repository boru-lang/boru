#!/usr/bin/env bash
# scripts/release-verify.sh — install the CLI just released from outside the
# repository, the way a user does, and run it (RELEASING.md, "After a
# release").
#
#   scripts/release-verify.sh [VERSION]    default: the newest cmd/go tag
#
# GOWORK=off and an empty directory, so nothing in the checkout can stand in
# for the published modules: a go.mod that still carries a replace, or pins
# a sibling version that was never tagged, fails here as it would for a
# user. The module comes straight from GitHub (GOPROXY=direct, no checksum
# database): the proxy and the database each fetch a new version on its
# first request, and a just-pushed tag may not be there yet.
set -euo pipefail
cd "$(dirname "$0")/.."

git fetch --tags --quiet origin || true
v=${1:-$(git tag -l 'cmd/go/v*' --sort=-version:refname | sed -n '1s|^cmd/go/||p')}
[ -n "$v" ] || { echo "release-verify: no cmd/go tag to verify" >&2; exit 1; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
( cd "$tmp" && GOWORK=off GOFLAGS= GOPROXY=direct GONOSUMDB=github.com/boru-lang/boru GOBIN="$tmp/bin" \
    go install "github.com/boru-lang/boru/cmd/go/boru@$v" )
"$tmp/bin/boru" -version
"$tmp/bin/boru" help > /dev/null
echo "release-verify: boru $v installs from outside the repository and runs"
