.PHONY: build run install vet fmt test lint vuln clean publish tags

# This module (github.com/boru-lang/boru/cmd/go) is the boru command-line
# tool. The CLI implementation lives in this directory as a library
# (package `boru`); the thin `main` package that produces the `boru`
# binary lives at ./boru so `go install` picks the right binary name.
# The language library lives in ../../lang/go and the kernel pieces in
# ../../core/go, ../../check/go, ../../parser/go and ../../basic/go (all
# wired in via replace directives in go.mod).

BINARY := bin/boru
VERSION := 0.1.0-dev
LDFLAGS := -ldflags "-X github.com/boru-lang/boru/cmd/go.Version=$(VERSION)"
CMD := ./boru

build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

install:
	go install $(LDFLAGS) $(CMD)

test:
	go test ./...

vet:
	go vet -copylocks=false ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml) and govulncheck.
lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

clean:
	rm -rf bin/
	go clean -testcache

# Publish a new tagged release of the boru CLI.
#   make publish V=0.1.0                          # lang+parser pinned to v0.1.0
#   make publish V=0.1.0 LANG=0.1.0 PARSER=0.1.0  # explicit versions
#
# Updates require lines for lang/go and parser/go to the pinned versions
# (so consumers resolve real tags), embeds Version into main.go,
# commits, tags `cmd/go/v$(V)`, pushes. Wildcard `replace` directives
# keep local development resolving to the workspace siblings.
#
# After publish, users install with:
#   go install github.com/boru-lang/boru/cmd/go/boru@v$(V)
# (the binary is named `boru`; the thin main package at ./boru is what
# makes that work — installing the bare module produces a binary
# named `go`, which is not what we want).
publish: test
	@test -n "$(V)" || (echo "Usage: make publish V=x.y.z [LANG=x.y.z PARSER=x.y.z]" && exit 1)
	@LANG_VER=$${LANG:-$(V)}; \
	 PARSER_VER=$${PARSER:-$(V)}; \
	  perl -i -pe 's{(github\.com/boru-lang/boru/lang/go) v\S+}{$$1 v'$$LANG_VER'}' go.mod; \
	  perl -i -pe 's{(github\.com/boru-lang/boru/parser/go) v\S+}{$$1 v'$$PARSER_VER'}' go.mod; \
	  perl -i -pe 's{(^var Version = )"[^"]*"}{$$1"$(V)"}' main.go; \
	  cd ../.. && git add cmd/go/go.mod cmd/go/main.go && \
	  (git diff --cached --quiet || git commit -m "cmd/go: v$(V) (lang/go v'$$LANG_VER', parser/go v'$$PARSER_VER')") && \
	  git tag cmd/go/v$(V) && \
	  git push origin main cmd/go/v$(V)
	@if command -v gh >/dev/null 2>&1; then \
		cd ../.. && gh release create cmd/go/v$(V) --title "cmd/go (boru CLI) v$(V)" --notes "boru CLI v$(V)"; \
	fi

# List recent cmd/go tags (newest first).
tags:
	@git -C ../.. tag -l 'cmd/go/v*' --sort=-version:refname
