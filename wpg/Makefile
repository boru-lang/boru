.PHONY: serve wasm test vet fmt clean e2e

# wpg (github.com/boru-lang/boru/wpg) — the boru web playground.
#
#   wasm/   boru engine compiled to WebAssembly (browser-side eval)
#   serve/  Standalone HTTP server (server-side eval; self-contained HTML)
#
# The language library lives at ../lang/go (wired in via go.mod replace).

WASM_DIR := ../docs
SQLJS_VENDOR := wasm/vendor

# Build-info stamps for the playground header (commit / tag / date).
# Fall back gracefully when not in a git checkout. GIT_TAG is allowed
# to be empty — the recipe omits the tag chip entirely in that case
# rather than printing a placeholder.
GIT_COMMIT       := $(shell git -C .. rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_COMMIT_FULL  := $(shell git -C .. rev-parse HEAD 2>/dev/null)
GIT_TAG          := $(shell git -C .. describe --tags --abbrev=0 2>/dev/null)
GIT_DATE         := $(shell git -C .. log -1 --format=%cd --date=iso-local 2>/dev/null || echo unknown)
GIT_REPO_URL     := https://github.com/boru-lang/boru

serve:
	go run ./serve

wasm:
	@mkdir -p $(WASM_DIR)
	GOOS=js GOARCH=wasm go build -o $(WASM_DIR)/boru.wasm ./wasm
	@GOROOT=$$(go env GOROOT); \
	if [ -f "$$GOROOT/misc/wasm/wasm_exec.js" ]; then \
		cp "$$GOROOT/misc/wasm/wasm_exec.js" $(WASM_DIR)/wasm_exec.js; \
	elif [ -f "$$GOROOT/lib/wasm/wasm_exec.js" ]; then \
		cp "$$GOROOT/lib/wasm/wasm_exec.js" $(WASM_DIR)/wasm_exec.js; \
	else \
		echo "error: wasm_exec.js not found in GOROOT"; exit 1; \
	fi
	cp $(SQLJS_VENDOR)/sql-wasm.js $(WASM_DIR)/sql-wasm.js
	cp $(SQLJS_VENDOR)/sql-wasm.wasm $(WASM_DIR)/sql-wasm.wasm
	@echo "Bundling into single HTML file (commit $(GIT_COMMIT), tag $(if $(GIT_TAG),$(GIT_TAG),<none>), date $(GIT_DATE))..."
	@SQL_WASM_B64=$$(base64 -w0 $(WASM_DIR)/sql-wasm.wasm); \
	BORU_WASM_B64=$$(base64 -w0 $(WASM_DIR)/boru.wasm); \
	SEP='<span class="sep">·</span>'; \
	if [ -n "$(GIT_COMMIT_FULL)" ]; then \
		COMMIT_HTML='<a href="$(GIT_REPO_URL)/commit/$(GIT_COMMIT_FULL)" target="_blank" rel="noopener">$(GIT_COMMIT)</a>'; \
	else \
		COMMIT_HTML='$(GIT_COMMIT)'; \
	fi; \
	if [ -n "$(GIT_TAG)" ]; then \
		TAG_HTML='<a href="$(GIT_REPO_URL)/releases/tag/$(GIT_TAG)" target="_blank" rel="noopener">$(GIT_TAG)</a>'; \
		BUILD_INFO="$${TAG_HTML}$${SEP}$${COMMIT_HTML}$${SEP}$(GIT_DATE)"; \
	else \
		BUILD_INFO="$${COMMIT_HTML}$${SEP}$(GIT_DATE)"; \
	fi; \
	( \
		sed -n '1,/<script src="sql-wasm.js"><\/script>/{ /<script src="sql-wasm.js"><\/script>/!p }' wasm/index.html; \
		echo '<script>'; \
		cat $(WASM_DIR)/sql-wasm.js; \
		echo '</script>'; \
		echo '<script>'; \
		cat $(WASM_DIR)/wasm_exec.js; \
		echo '</script>'; \
		echo '<script>'; \
		echo "window.__sqlWasmB64 = '$$SQL_WASM_B64';"; \
		echo "window.__boruWasmB64 = '$$BORU_WASM_B64';"; \
		echo '</script>'; \
		sed -n '/<script src="wasm_exec.js"><\/script>/,$${ /<script src="wasm_exec.js"><\/script>/!p }' wasm/index.html; \
	) | sed "s|__BUILD_INFO__|$$BUILD_INFO|" > $(WASM_DIR)/index.html
	@rm -f $(WASM_DIR)/boru.wasm $(WASM_DIR)/wasm_exec.js $(WASM_DIR)/sql-wasm.js $(WASM_DIR)/sql-wasm.wasm
	@echo "Built: $(WASM_DIR)/index.html ($$(du -h $(WASM_DIR)/index.html | cut -f1))"

test:
	go test ./...

# Browser e2e for the boru:tui playground backend. Opt-in: needs the
# bundle (make wasm), node, playwright, and a chromium. See
# e2e/tui-e2e.js for the env overrides.
e2e:
	node e2e/tui-e2e.js

vet:
	go vet -copylocks=false ./...

fmt:
	gofmt -w .

clean:
	go clean -testcache
