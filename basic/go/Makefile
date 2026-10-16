.PHONY: test vet fmt lint vuln lint-assertions clean publish tags

# This module (github.com/boru-lang/boru/basic/go) is the boru base
# language layer: the fundamental words (stack, definition, control,
# type-generics) and the predefined global content types (the
# Scalar/Time family, Resource/Entity), registered against the core
# kernel. It depends on the sibling core/go and parser/go modules ONLY
# (ADR-013); the full word library lives one layer up in lang/go.
#
# This Makefile runs targets for this module only. Use the top-level
# Makefile at ../../Makefile to fan out across all modules.

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

# Fail if any non-test runtime handler uses a single-value .Data.(Type)
# assertion (panics on mismatch). Two-value (v, ok := ...) is safe.
# Mirrors lang/go's lint-assertions over this module's word files.
lint-assertions:
	@echo "Checking for unsafe .Data().(Type) assertions in runtime handlers..."
	@HITS=$$(find . -name '*.go' ! -name '*_test.go' \
		-exec grep -Hn '\.Data\(()\)\?\.(' {} \; \
		| grep -v ',.*:=' | grep -v '\.(type)' \
		| grep -v '// lint:allow-assert'); \
	if [ -n "$$HITS" ]; then \
		echo "FAIL: unsafe .Data().(Type) assertions found:"; \
		echo "$$HITS"; \
		echo "Use the two-value form: v, ok := x.Data().(Type)"; \
		exit 1; \
	fi
	@echo "OK: no unsafe assertions found"

clean:
	go clean -testcache

# Publish a new tagged release.
#   make publish V=0.1.0                          # core/go and parser/go pinned to v0.1.0 as well
#   make publish V=0.1.0 CORE=0.1.0 PARSER=0.1.0  # explicit sibling versions
#
# Updates the `require github.com/boru-lang/boru/core/go` and
# `…/parser/go` lines to the pinned sibling versions (so consumers
# resolve real tags), commits, tags `basic/go/v$(V)`, pushes. The
# `replace` directives keep local development resolving to the in-tree
# siblings regardless of the require versions.
publish: test
	@test -n "$(V)" || (echo "Usage: make publish V=x.y.z [CORE=x.y.z] [PARSER=x.y.z]" && exit 1)
	@CORE_VER=$${CORE:-$(V)}; PARSER_VER=$${PARSER:-$(V)}; \
	  perl -i -pe 's{(github\.com/boru-lang/boru/core/go) v\S+}{$$1 v'$$CORE_VER'}' go.mod; \
	  perl -i -pe 's{(github\.com/boru-lang/boru/parser/go) v\S+}{$$1 v'$$PARSER_VER'}' go.mod; \
	  cd ../.. && git add basic/go/go.mod && \
	  (git diff --cached --quiet || git commit -m "basic/go: v$(V) (core/go v'$$CORE_VER', parser/go v'$$PARSER_VER')") && \
	  git tag basic/go/v$(V) && \
	  git push origin main basic/go/v$(V)
	@if command -v gh >/dev/null 2>&1; then \
		cd ../.. && gh release create basic/go/v$(V) --title "basic/go v$(V)" --notes "boru base language layer v$(V)"; \
	fi

# List recent basic/go tags (newest first).
tags:
	@git -C ../.. tag -l 'basic/go/v*' --sort=-version:refname
