.PHONY: test vet fmt lint vuln clean cover publish tags

# This module (github.com/boru-lang/boru/eng/go) is the boru engine
# kernel + jsonic parser + kernel spec runner. It is standalone — it has
# no dependency on the lang or cmd/go modules.
#
# Code-structure visualisation targets (viz, viz-callgraph, viz-goda,
# viz-gomod, viz-plantuml, viz-list, viz-modgraph, viz-index, viz-clean,
# viz-tools) live in the top-level Makefile (../../Makefile) so the
# whole codebase — every module — can be reviewed from one place. Run
# `make viz` from the repo root to generate them.

test:
	go test ./...

vet:
	go vet -copylocks=false ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml, found by walking up).
lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

# Per-module coverage profile. The repo-root `make cover` aggregates
# across all modules.
cover:
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

clean:
	go clean -testcache
	rm -f coverage.out

# Publish a new tagged release of this module.
#   make publish V=0.1.0
# Runs tests, then tags `eng/go/v$(V)` and pushes. Tag prefix must match
# the module's repo subpath (Go's submodule convention).
publish: test
	@test -n "$(V)" || (echo "Usage: make publish V=x.y.z" && exit 1)
	@cd ../.. && git diff --quiet HEAD -- eng/go || (echo "ERROR: uncommitted changes in eng/go" && exit 1)
	cd ../.. && git tag eng/go/v$(V)
	cd ../.. && git push origin eng/go/v$(V)
	@if command -v gh >/dev/null 2>&1; then \
		cd ../.. && gh release create eng/go/v$(V) --title "eng/go v$(V)" --notes "boru engine kernel v$(V)"; \
	fi

# List recent eng/go tags (newest first).
tags:
	@git -C ../.. tag -l 'eng/go/v*' --sort=-version:refname
