.PHONY: test vet fmt lint vuln lint-assertions clean publish tags

# This module (github.com/boru-lang/boru/lang/go) is the boru language
# library. The boru CLI lives in the sibling cmd/go module — build it
# with `make -C ../../cmd/go build`. The web playground (wasm + serve)
# lives in ../../wpg.
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

# Known-vulnerability scan (reachability-aware).
# Install: go install golang.org/x/vuln/cmd/govulncheck@latest
vuln:
	govulncheck ./...

# Fail if any non-test runtime handler uses a single-value .Data.(Type)
# assertion (panics on mismatch). Two-value (v, ok := ...) is safe.
# Scans native/native_*.go (the language-layer primitives, post engine→native
# consolidation in May 2026).
lint-assertions:
	@echo "Checking for unsafe .Data.(Type) assertions in runtime handlers..."
	@HITS=$$(find native -name 'native_*.go' ! -name '*_test.go' \
		-exec grep -Hn '\.Data\.(' {} \; \
		| grep -v ',.*:=' | grep -v '\.(type)' \
		| grep -v '// lint:allow-assert'); \
	if [ -n "$$HITS" ]; then \
		echo "FAIL: unsafe .Data.(Type) assertions found:"; \
		echo "$$HITS"; \
		echo "Use the two-value form: v, ok := x.Data.(Type)"; \
		exit 1; \
	fi
	@echo "OK: no unsafe assertions found"

clean:
	go clean -testcache

# Publish a new tagged release.
#   make publish V=0.1.0                        # eng+basic pinned to v0.1.0 too
#   make publish V=0.1.0 ENG=0.1.0 BASIC=0.1.0  # explicit sibling versions
#
# Updates the `require github.com/boru-lang/boru/eng/go` and
# `…/basic/go` lines to the pinned sibling versions (so consumers
# resolve real tags), commits, tags `lang/go/v$(V)`, pushes. The
# `replace` directives keep local development resolving to the in-tree
# siblings regardless of the require versions.
publish: test
	@test -n "$(V)" || (echo "Usage: make publish V=x.y.z [ENG=x.y.z] [BASIC=x.y.z]" && exit 1)
	@ENG_VER=$${ENG:-$(V)}; BASIC_VER=$${BASIC:-$(V)}; \
	  perl -i -pe 's{(github\.com/boru-lang/boru/eng/go) v\S+}{$$1 v'$$ENG_VER'}' go.mod; \
	  perl -i -pe 's{(github\.com/boru-lang/boru/basic/go) v\S+}{$$1 v'$$BASIC_VER'}' go.mod; \
	  cd ../.. && git add lang/go/go.mod && \
	  (git diff --cached --quiet || git commit -m "lang/go: v$(V) (eng/go v'$$ENG_VER', basic/go v'$$BASIC_VER')") && \
	  git tag lang/go/v$(V) && \
	  git push origin main lang/go/v$(V)
	@if command -v gh >/dev/null 2>&1; then \
		cd ../.. && gh release create lang/go/v$(V) --title "lang/go v$(V)" --notes "boru language layer v$(V)"; \
	fi

# List recent lang/go tags (newest first).
tags:
	@git -C ../.. tag -l 'lang/go/v*' --sort=-version:refname
