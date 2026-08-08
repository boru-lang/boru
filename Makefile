.PHONY: all build install test test-race test-ts test-ts-core test-ts-parser vet fmt fmt-docs lint vuln bench clean cover cover-gate cover-profile cover-check cover-html cover-html-open \
        spec-gen spec-test crossdiff parser-crossdiff cover-gate-eng cover-gate-check cover-gate-compiler cover-gate-parser facades \
        verify-bytecode fuzz-bytecode status \
        publish publish-eng publish-basic publish-lang publish-cmd release tags \
        viz viz-tools viz-clean viz-index \
        viz-callvis viz-callgraph viz-goda viz-godepgraph \
        viz-gomod viz-golds viz-plantuml viz-list viz-modgraph

# Top-level Makefile for the whole boru codebase.
#
# The repo is a collection of Go modules:
#
#   core/go        — the pure-interpreter kernel (values, types, dispatch, step loop)
#   parser/go      — boru source text -> []core.Value (jsonic grammar); depends
#                    on core alone, and nothing depends on it but the layers above
#   eng/go         — check + compiler + VM over core, plus the parser bridge
#   basic/go       — the base language layer (fundamental words +
#                    predefined content types; depends on eng only)
#   lang/go        — the language layer (native_* words, engine shim)
#   cmd/go         — the boru CLI command
#   calc/go        — small calculator built directly on eng (learning example)
#   wpg            — wasm playground (wasm build + serve)
#   test/go        — shared TSV spec-runner scaffolding
#   test/solardemo — standalone HTTP fixture used by API tests
#
# Each module has its own go.mod and a focused Makefile. The targets
# here fan out across the set so the whole codebase can be built,
# tested, visualised, and coverage-tracked from one place.

# Order matters for `make test`: eng must build before basic, basic
# before lang, etc.
MODULES := core/go check/go compiler/go parser/go eng/go basic/go lang/go cmd/go calc/go wpg test/go test/solardemo

all: test

# ---- boru CLI binary ----------------------------------------------------
#
# The `boru` CLI lives in cmd/go and has its own Makefile that handles
# the LDFLAGS/version stamping and the bin/boru layout. These targets
# delegate so the binary can be built from the repo root:
#
#   make build    -> cmd/go/bin/boru
#   make install  -> $GOBIN/boru

build:
	$(MAKE) -C cmd/go build

install:
	$(MAKE) -C cmd/go install

# ---- generated syntax-combination spec ---------------------------------
#
# Regenerate the two committed spec files under test/go/specgen/:
#   syntax-matrix.tsv          — the exhaustive matrix of every boru token
#                                sequence up to length 4 over a fixed
#                                alphabet, with the canonical interpreter
#                                result (or stable error class) for each.
#   syntax-matrix-passing.tsv  — the subset of those rows that pass all
#                                three pipelines (interpret + check +
#                                compile), derived from the full matrix.
#
# They are the frozen contract their sibling syntax_matrix_test.go checks
# the interpreter, compiler, and checker against; rerun this after any
# deliberate change to the alphabet or to evaluation semantics, then
# review the diff.

spec-gen:
	cd test/go && go run ./specgen -max 4 -out ./specgen/syntax-matrix.tsv
	cd test/go && go run ./specgen -extract -in ./specgen/syntax-matrix.tsv -out ./specgen/syntax-matrix-passing.tsv
	cd test/go && go run ./specgen -frontier \
	  -passing ./specgen/syntax-matrix-passing.tsv \
	  -check-out ./specgen/syntax-matrix-fail-check.tsv \
	  -compile-out ./specgen/syntax-matrix-fail-compile.tsv \
	  -runtime-out ./specgen/syntax-matrix-fail-runtime.tsv
	cd test/go && go run ./specgen -extract -max 3 -in ./specgen/syntax-matrix.tsv -out ./specgen/syntax-matrix-len123-passing.tsv
	cd test/go && go run ./specgen -frontier -max 3 \
	  -passing ./specgen/syntax-matrix-passing.tsv \
	  -check-out ./specgen/syntax-matrix-len123-fail-check.tsv \
	  -compile-out ./specgen/syntax-matrix-len123-fail-compile.tsv \
	  -runtime-out ./specgen/syntax-matrix-len123-fail-runtime.tsv
	cd test/go && go run ./specgen -extend5 \
	  -passing ./specgen/syntax-matrix-passing.tsv \
	  -len123 ./specgen/syntax-matrix-len123-passing.tsv \
	  -pass-out ./specgen/syntax-matrix-len5-passing.tsv \
	  -check-out ./specgen/syntax-matrix-len5-fail-check.tsv \
	  -compile-out ./specgen/syntax-matrix-len5-fail-compile.tsv \
	  -runtime-out ./specgen/syntax-matrix-len5-fail-runtime.tsv \
	  -mismatch-out ./specgen/syntax-matrix-len5-compiler-mismatch.tsv

# spec-test runs the generated-matrix replay suite. It is gated behind the
# `specgen` build tag so it is EXCLUDED from `make test` (the matrix-replay
# tests re-execute the interpreter/checker/compiler over tens of thousands
# of rows — ~7 min sampled — which is too slow for the normal unit run).
# Set SPECGEN_FULL=1 to verify every row of every file exhaustively
# (~24 min); the default samples the large length-5 buckets. A longer test
# timeout is supplied because the exhaustive mode exceeds the 10-min default.
spec-test:
	cd test/go && go test -tags specgen -timeout 40m ./specgen/

# ---- generated module facades ------------------------------------------
#
# eng/go/aliases_{core,check,compiler}.go re-export the lower modules'
# surface under package eng so downstream code (basic, lang, cmd, calc,
# wpg, the harnesses) compiles unchanged across the four-piece split
# (design/ENG-FOUR-PIECE.0.md). They are GENERATED — regenerate after any
# change to an exported symbol in core/check/compiler, then re-run the
# checklist. The generator also derives the "cold" set (funcs no consumer
# calls through the facade) and emits those as func-value re-exports, so
# no wrapper body sits permanently uncovered under the ADR-008 gate.
#
# piecetool lives in its own module (tools/piecetool) that is NOT in
# MODULES: it is a developer tool, so its statements stay out of the
# repo-wide 100% coverage universe the shipped modules must satisfy.
facades:
	@cd tools/piecetool && go build -o "$(abspath $(COVER_DIR))/piecetool" .
	@"$(abspath $(COVER_DIR))/piecetool" -facade core/go eng/go/aliases_core.go core
	@"$(abspath $(COVER_DIR))/piecetool" -facade check/go eng/go/aliases_check.go check
	@"$(abspath $(COVER_DIR))/piecetool" -facade compiler/go eng/go/aliases_compiler.go compiler
	@cd eng/go && gofmt -w aliases_core.go aliases_check.go aliases_compiler.go
	@echo "==> facades regenerated (run make fmt && make test)"

# ---- per-module fan-out -------------------------------------------------

test:
	@set -e; for m in $(MODULES); do \
	  echo "==> test $$m"; \
	  ( cd $$m && go test -timeout 20m ./... ); \
	done

# test-race is the data-race gate (design/TEST-SEAMS.10.md). `make test` does
# NOT run under -race — the detector inflates CPU/alloc ~5-10x, which breaks
# the perf/alloc-ceiling tests and would make the whole suite too slow. So the
# race lane runs -race -short: -short skips the perf gates (they guard
# testing.Short()), while every concurrency test — anything spawning goroutines
# or forking a registry — runs under the detector.
#
# RACE_MODULES run their WHOLE package tree under -race, so a NEW concurrency
# test is covered automatically with no hand-maintained name list (the gap that
# let the ForkConcurrent cf.Reg race reach CI). test/go/langspec is the one
# exception: its 5941-row differential is single-threaded per row (race adds no
# value) and times out under the detector, so only its concurrency rows run
# here.
RACE_MODULES := core/go check/go compiler/go parser/go eng/go basic/go lang/go
test-race:
	@set -e; for m in $(RACE_MODULES); do \
	  echo "==> test-race $$m"; \
	  ( cd $$m && go test -race -short -timeout 25m ./... ); \
	done
	@echo "==> test-race test/go/langspec (concurrency rows)"
	cd test/go && go test -race -short -timeout 15m ./langspec/ -run 'Concurrent|RaceFree|Race'

vet:
	@set -e; for m in $(MODULES); do \
	  echo "==> vet $$m"; \
	  ( cd $$m && go vet ./... ); \
	done

fmt:
	@set -e; for m in $(MODULES); do \
	  echo "==> fmt $$m"; \
	  ( cd $$m && gofmt -w . ); \
	done

# fmt-docs applies `boru fmt` to the user-facing docs whose ```boru fenced
# blocks are pinned fmt-clean by test/go/docexamples (DOC_FILES mirrors
# its docFiles list). `boru fmt` rewrites only the fences and
# <!-- borufmt --> regions, leaving the prose untouched — run this when
# the fmt-clean gate flags a drifted block, or after editing examples.
DOC_FILES := README.md REFERENCE.md TUTORIAL.md HOWTO.md EXPLANATION.md
fmt-docs:
	@echo "==> fmt-docs $(DOC_FILES)"
	@cd cmd/go && go run ./boru fmt $(addprefix ../../,$(DOC_FILES))

lint:
	@set -e; for m in $(MODULES); do \
	  echo "==> lint $$m"; \
	  ( cd $$m && golangci-lint run ./... ); \
	done

vuln:
	@set -e; for m in $(MODULES); do \
	  echo "==> vuln $$m"; \
	  ( cd $$m && govulncheck ./... ); \
	done

# ---- performance baseline ----------------------------------------------
#
# Run the performance-baseline benchmark suites: kernel primitives
# (eng/go), parser shapes (eng/go/parser), dispatch shapes + interpreter
# vs compiled + word families + check/compile cost (lang/go). Compare
# before/after an engine change with benchstat:
#
#   make bench > after.txt     # (and once on the base commit > before.txt)
#   benchstat before.txt after.txt
#
# BENCH_TIME tunes -benchtime (default 1s per benchmark). The
# deterministic regression *gates* (allocation ceilings) run in the
# normal `make test`: TestCompiledAllocCeilings and TestInterpAllocCeilings
# in lang/go.
BENCH_TIME ?= 1s
bench:
	@echo "==> bench eng/go (kernel primitives)"
	cd eng/go && go test -run '^$$' -bench 'BenchmarkKernel|BenchmarkTape' -benchmem -benchtime $(BENCH_TIME) .
	@echo "==> bench parser/go (parse shapes)"
	cd parser/go && go test -run '^$$' -bench 'BenchmarkParse' -benchmem -benchtime $(BENCH_TIME) .
	@echo "==> bench lang/go (dispatch, exec, words, check, compile)"
	cd lang/go && go test -run '^$$' -bench 'BenchmarkBytecodeBaseline|BenchmarkStage6|BenchmarkParens|BenchmarkPerf' -benchmem -benchtime $(BENCH_TIME) .

# ---- TypeScript engine port (eng/ts) -----------------------------------
#
# @boru-lang/eng mirrors the Go kernel and must stay row-for-row green on
# the SAME eng/spec/*.tsv corpus as the Go engspec runner. Runs the
# typechecker then the node:test suite (Node >= 24, type-stripping).
# The line-coverage threshold is the TS half of the standalone-parity
# ratchet (design/ENG-COVERAGE-PARITY.0.md; Go statements ≡ TS lines,
# both measured by the engine's OWN suite): raise TS_GATE_LINES as
# coverage grows towards the 100% target; never lower it.
#
# The denominator is SOURCE ONLY. node:test instruments every file it loads,
# which includes the *.test.ts files themselves; counting those made the gate
# measure its own test code. That is not the Go metric — `go test -coverpkg`
# never counts _test.go statements — so the parity equivalence (Go statements
# ≡ TS lines) only holds with the exclusion below.
#
# RE-BASED 97 -> 96 when the exclusion landed, for the same reason
# ENG_GATE_FLOOR re-based at the four-piece cut: the measurement UNIVERSE
# changed, so the old number is not comparable. Measured both ways on the same
# commit: 97.06% with test files counted (which is what let the 97 floor pass),
# 96.80% source-only. The floor tracks the source-only figure from here and
# ratchets up as before — never down.
# RE-BASED 96 -> 97 when --test-coverage-include landed, and this one was a
# CORRECTNESS fix, not just a denominator change. @boru-lang/core (and now
# @boru-lang/parser) are `file:` dependencies, which npm installs as symlinks;
# node resolves through the symlink to the real path, so node:test was
# instrumenting core/ts's and parser/ts's sources and folding them into eng's
# figure. That is precisely the cross-suite coverage the standalone gates exist
# to forbid — the TS analogue of measuring core/go's statements from eng/go's
# suite. --test-coverage-include='src/**' scopes the gate to eng/ts's OWN
# source, the same universe `go test -coverpkg` gives the Go half.
#
# Measured both ways on the same commit: 90.00% with the symlinked deps folded
# in, 97.25% eng-only. The floor tracks the eng-only figure from here.
TS_GATE_LINES ?= 97
test-ts:
	@echo "==> typecheck eng/ts"
	cd eng/ts && npx tsc
	@echo "==> test eng/ts (source line-coverage floor $(TS_GATE_LINES)%)"
	cd eng/ts && node --test --experimental-strip-types --no-warnings \
	  --experimental-test-coverage --test-coverage-lines=$(TS_GATE_LINES) \
	  --test-coverage-exclude='**/*.test.ts' \
	  --test-coverage-include='src/**' \
	  'src/**/*.test.ts'

# ---- TypeScript interpreter core (core/ts) -----------------------------
#
# @boru-lang/core is the TS twin of the core/go module — values, types,
# signatures, matching, the registry, and the step loop, with NO check pass,
# NO compiler, NO parser and no dependencies at all (core/go at least needs
# apd; the TS core needs nothing). It is the fourth gate in the standalone
# set, the direct counterpart of `make cover-gate-core`:
#
#   cover-gate-core    core/go by its own suite   floor 100
#   test-ts-core       core/ts by its own suite   floor $(TS_CORE_GATE_LINES)
#
# Same source-only denominator as test-ts, and the same ratchet discipline:
# raise the floor in the change that raises coverage, never lower it.
#
# The no-upward-imports rule (core/go/CLAUDE.md) is what makes this gate
# meaningful, and it is STRUCTURALLY enforced here rather than by convention:
# core/ts has no dependency on @boru-lang/eng, so a core file that reached
# for the check pass or the compiler would fail to resolve. The check piece
# reaches core only through the seam tables core owns — AnalysisImpl
# (analysis-hooks.ts) and EmitRecorder (emit-recorder.ts) — each with NAMED
# inactive defaults pinned by a core-side test, exactly as core/go requires.
# Floor 62, RE-BASED DOWN from the stage-1 71 — read the reason before
# treating this as a regression, because it is the opposite of one.
#
# node:test only instruments files a test actually loads. At stage 1 the suite
# loaded seven of core/ts's seventeen files (the seam tables, canon, type,
# signature and their dependencies), so 71.76% was 71.76% *of those seven*.
# The core/spec corpus loads the whole package — engine, match, resolve, make,
# sugar, coretype, check-state — and those arrive largely uncovered, so the
# honest figure over the FULL core surface is 62.06%.
#
# Same re-base as ENG_GATE_FLOOR at the four-piece cut and TS_GATE_LINES at
# the source-only correction: the measurement universe changed, so the old
# number is not comparable to the new one. Nothing became less tested — the
# denominator got honest. From here the ratchet only rises, and the corpus is
# the instrument: rows added to core/spec lift both engines at once.
#
# Current per-file, worst first: engine 68, match 85, resolve 96, value 96,
# registry 97, coretype 94, canon 99 — the corpus is what moved them, and
# rows added to core/spec lift both engines at once.
TS_CORE_GATE_LINES ?= 88
test-ts-core:
	@echo "==> typecheck core/ts"
	cd core/ts && npx tsc
	@echo "==> test core/ts (source line-coverage floor $(TS_CORE_GATE_LINES)%)"
	cd core/ts && node --test --experimental-strip-types --no-warnings \
	  --experimental-test-coverage --test-coverage-lines=$(TS_CORE_GATE_LINES) \
	  --test-coverage-exclude='**/*.test.ts' \
	  --test-coverage-include='src/**' \
	  'src/**/*.test.ts'

# ---- TypeScript parser (parser/ts) -------------------------------------
#
# @boru-lang/parser is the TS twin of the parser/go module — source text to
# Value[], the front end and nothing else. Cut out of eng/ts/src/parser so the
# TS side mirrors the Go module graph: a leaf over @boru-lang/core that the
# engine depends on, rather than a directory inside the engine.
#
# It is the fifth gate in the standalone set:
#
#   cover-gate-parser  parser/go by its own suite   floor 100
#   test-ts-parser     parser/ts by its own suite   floor $(TS_PARSER_GATE_LINES)
#
# Same source-only, own-module denominator as the other two, and the same
# ratchet discipline: raise the floor in the change that raises coverage,
# never lower it. parser/go's gate has sat at 100 since the module was cut
# (parser/go/CLAUDE.md: a leaf over core has no other suite that could be
# covering it), and on 2026-08-08 parser/ts reached it too — the two halves
# of the module are now gated identically.
TS_PARSER_GATE_LINES ?= 100
test-ts-parser:
	@echo "==> typecheck parser/ts"
	cd parser/ts && npx tsc
	@echo "==> test parser/ts (source line-coverage floor $(TS_PARSER_GATE_LINES)%)"
	cd parser/ts && node --test --experimental-strip-types --no-warnings \
	  --experimental-test-coverage --test-coverage-lines=$(TS_PARSER_GATE_LINES) \
	  --test-coverage-exclude='**/*.test.ts' \
	  --test-coverage-include='src/**' \
	  'src/**/*.test.ts'

# ---- TypeScript base layer (basic/ts) ----------------------------------
#
# @boru-lang/basic is the TS twin of the basic/go module. It is the sixth
# gate in the standalone set:
#
#   cover-gate-core    core/go by its own suite     floor 100
#   test-ts-core       core/ts by its own suite     floor $(TS_CORE_GATE_LINES)
#   cover-gate-parser  parser/go by its own suite   floor 100
#   test-ts-parser     parser/ts by its own suite   floor $(TS_PARSER_GATE_LINES)
#   cover-gate         basic/go via the merged gate floor 100
#   test-ts-basic      basic/ts by its own suite    floor $(TS_BASIC_GATE_LINES)
#
# The floor starts at 100 rather than on a ratchet, and can: the module is
# being built increment by increment against basic/spec, so every line that
# exists is a line the shared corpus already reaches. It is a ratchet in
# the other direction — the floor holds while the SURFACE grows, so a new
# word cannot land without corpus rows that exercise it.
TS_BASIC_GATE_LINES ?= 100
test-ts-basic:
	@echo "==> typecheck basic/ts"
	cd basic/ts && npx tsc
	@echo "==> test basic/ts (source line-coverage floor $(TS_BASIC_GATE_LINES)%)"
	cd basic/ts && node --test --experimental-strip-types --no-warnings \
	  --experimental-test-coverage --test-coverage-lines=$(TS_BASIC_GATE_LINES) \
	  --test-coverage-exclude='**/*.test.ts' \
	  --test-coverage-include='src/**' \
	  'src/**/*.test.ts'

# ---- cross-engine differential -----------------------------------------
#
# Run the shared corpus (eng/spec value mode + eng/spec/check check mode)
# through BOTH the Go kernel and the TS engine and diff the two result streams
# row-by-row. This target drives the Go half (it execs the TS dumper
# eng/ts/src/crossdump.ts); the TS half is the mirror (eng/ts/src/crossdiff.test.ts
# execs the Go dumper) and runs under `make test-ts`. Reports agreements,
# error-code differences, and functionality gaps; hard-fails only on a true
# divergence (both engines produce a value but the values differ). Requires `node`.
crossdiff:
	@echo "==> cross-engine differential (Go kernel vs TS engine; value + check)"
	cd test/go && go test ./engspec/ -run TestCrossEngineDifferential -v

# ---- parser-level cross-engine differential -----------------------------
#
# The PARSER twins, diffed row-for-row over eng/spec. Both dumpers have
# existed since the TS port — parser/go/streamdump_test.go and
# parser/ts/src/streamdump.ts, each emitting `<file>:<line> OK|ERR <render>`
# — and NOTHING ever ran the comparison. With STREAMDUMP_FILE unset the Go
# side dumps to a temp dir and discards it, so the corpus only proved every
# row parses.
#
# That gap is not covered by `crossdiff` above: it compares the two ENGINES
# and hard-fails only when both produce a value and the values differ, so a
# parser-level render difference that still evaluates alike is invisible to
# it. Three real defects were living in that blind spot — a disjunction
# rendering as the literal '[object Object]', the None type literal
# rendering as the none value, and every type literal rendering by full path
# instead of leaf name (design/TS-PARITY-AUDIT.0.md).
#
# parser/spec is the curated contract; this is the breadth sweep over the
# 1765 rows of eng/spec that the contract does not enumerate.
parser-crossdiff:
	@echo "==> parser differential (parser/go vs parser/ts over eng/spec)"
	@mkdir -p "$(abspath $(COVER_DIR))"
	@cd parser/go && STREAMDUMP_FILE="$(abspath $(COVER_DIR))/go-streams.tsv" \
	  go test -run TestStreamDump >/dev/null
	@cd parser/ts && node --experimental-strip-types --no-warnings src/streamdump.ts \
	  > "$(abspath $(COVER_DIR))/ts-streams.tsv"
	@diff -u "$(abspath $(COVER_DIR))/go-streams.tsv" "$(abspath $(COVER_DIR))/ts-streams.tsv" \
	  && echo "==> parser-crossdiff: IDENTICAL ($$(wc -l < "$(abspath $(COVER_DIR))/go-streams.tsv") rows)" \
	  || { echo "==> parser-crossdiff FAILED: the parser twins disagree (see the diff above)"; exit 1; }

# ---- compiled-coverage status surface ----------------------------------
#
# Regenerate test/go/langspec/COMPILED_STATUS.md from the live spec corpus.
# Run after any change that moves compiled coverage; TestCompiledStatus fails
# if the committed surface is stale.
status:
	cd test/go && BORU_WRITE_STATUS=1 go test ./langspec/ -run TestCompiledStatus
	@echo "==> wrote test/go/langspec/COMPILED_STATUS.md"

# ---- bytecode verification gate ----------------------------------------
#
# The strict, runnable regression gate for the bytecode compiler
# (design/boru-bytecode-plan.0.md). It is the single command to validate a
# change to the compiler/VM and catch regressions:
#
#   1. fmt / vet / lint across every module.
#   2. The dual-mode differential gate (>= minCompiledRows compile) and
#      the whole-corpus compile-or-fallback gate (0 divergences in values
#      AND error taxonomy over the full spec corpus + the curated
#      bytecode-combinations matrix).
#   3. The Go-driven combination matrix (parity + compilation-path pins).
#   4. The emitter goldens, return-check, isolation, and Tape.Reload pins.
#   5. The deterministic compiled-mode allocation ceilings (catches a
#      per-dispatch or island-reuse allocation regression).
#   6. The -race concurrency gates (shared immutable Program across forks;
#      island sub-engine reuse with no state leak; concurrent spec rows).
#   7. The same parity gates under -tags borudebug (a fresh args slice per
#      CALL_NATIVE), so a compiled-reachable native that retains its args
#      slice — silently corrupting a later dispatch under the release build's
#      buffer reuse — instead diverges cleanly here. Mirrors the CI lane.
#
# Any divergence, race, or allocation regression fails the gate.
verify-bytecode: fmt vet lint
	@echo "==> bytecode: differential + whole-corpus + combination matrix + property fuzz"
	cd test/go && go test ./langspec/ -run 'TestSpecCompiledDifferential|TestSpecCompiledOrFallback|TestCompiledCombination|TestPropertyDifferential'
	@echo "==> bytecode: emitter / return-check / step-budget / isolation / reuse / alloc pins"
	cd lang/go && go test . -run 'TestEmit|TestRunCompiled|TestCompiled|TestStepBudget|TestTapeReload'
	@echo "==> bytecode: const-bake mutation-safety + VM pins"
	cd eng/go  && go test . -run 'TestTapeReload|TestVM|TestIsInertConst'
	@echo "==> bytecode: -race concurrency gates"
	cd lang/go && go test . -run 'TestCompiledConcurrencyRaceFree|TestCompiledIslandReuseNoStateLeak' -race
	cd test/go && go test ./langspec/ -run 'TestSpecCompiledConcurrentRowsRaceFree' -race
	@echo "==> bytecode: args-aliasing gate (-tags borudebug, fresh args slice per CALL_NATIVE)"
	cd lang/go && go test -tags borudebug . -run 'TestEmit|TestRunCompiled|TestCompiled|TestStepBudget'
	cd test/go && go test -tags borudebug ./langspec/ -run 'TestSpecCompiledDifferential|TestSpecCompiledOrFallback|TestCompiledCombinationParity|TestPropertyDifferential'
	@echo "==> bytecode: VERIFY PASSED"

# Nightly / on-demand DEEP property fuzz of the compilable subset. The standard
# verify-bytecode runs the lean deterministic default; this cranks the seed and
# iteration budget (override on the command line) and re-runs the generated
# corpus under BOTH the release build and the -tags borudebug build (fresh args
# slice per CALL_NATIVE), so a deep run also exercises the args-aliasing invariant.
#   make fuzz-bytecode                       # the cranked default below
#   make fuzz-bytecode FUZZ_SEEDS=40 FUZZ_ITERS=20000
FUZZ_SEEDS ?= 20
FUZZ_ITERS ?= 5000
fuzz-bytecode:
	@echo "==> bytecode: property fuzz ($(FUZZ_SEEDS) seeds x $(FUZZ_ITERS) iters), release build"
	cd test/go && BORU_FUZZ_SEEDS=$(FUZZ_SEEDS) BORU_FUZZ_ITERS=$(FUZZ_ITERS) \
	  go test ./langspec/ -run 'TestPropertyDifferential' -timeout 60m -v
	@echo "==> bytecode: property fuzz, -tags borudebug (args-aliasing build)"
	cd test/go && BORU_FUZZ_SEEDS=$(FUZZ_SEEDS) BORU_FUZZ_ITERS=$(FUZZ_ITERS) \
	  go test -tags borudebug ./langspec/ -run 'TestPropertyDifferential' -timeout 60m
	@echo "==> bytecode: FUZZ PASSED"

clean:
	@set -e; for m in $(MODULES); do \
	  echo "==> clean $$m"; \
	  ( cd $$m && go clean -testcache ); \
	done
	rm -rf $(VIZ_DIR) $(COVER_DIR)

# ---- publishing --------------------------------------------------------
#
# Each Go library/CLI module publishes via its own subdir Makefile (the
# tag prefix has to match the repo subpath per Go's submodule rules).
# Targets here orchestrate them in dependency order so a coordinated
# release uses one matched version:
#
#   make publish V=0.2.0
#     -> tags eng/go/v0.2.0, basic/go/v0.2.0, lang/go/v0.2.0, cmd/go/v0.2.0
#     -> bumps basic/go's eng require, lang/go's eng+basic requires,
#        cmd/go's eng+lang requires
#
# Per-module publish (independent versions):
#   make publish-eng   V=0.2.0
#   make publish-basic V=0.2.0 ENG=0.1.0
#   make publish-lang  V=0.2.0 ENG=0.1.0 BASIC=0.2.0
#   make publish-cmd   V=0.2.0 ENG=0.1.0 LANG=0.2.0
#
# After publishing, consumers install with:
#   go install github.com/boru-lang/boru/cmd/go/boru@v0.2.0   # boru CLI
#   go get     github.com/boru-lang/boru/lang/go@v0.2.0  # lang library
#   go get     github.com/boru-lang/boru/basic/go@v0.2.0 # basic layer
#   go get     github.com/boru-lang/boru/eng/go@v0.2.0   # eng kernel

publish:
	@test -n "$(V)" || (echo "Usage: make publish V=x.y.z" && exit 1)
	$(MAKE) -C eng/go   publish V=$(V)
	$(MAKE) -C basic/go publish V=$(V) ENG=$(V)
	$(MAKE) -C lang/go  publish V=$(V) ENG=$(V) BASIC=$(V)
	$(MAKE) -C cmd/go   publish V=$(V) ENG=$(V) LANG=$(V)

# release — the versioned release flow (see RELEASING.md and scripts/release.sh):
# runs the full test suite, then auto-bumps the PATCH of eng/go, lang/go and
# cmd/go, strips the local sibling `replace` directives, pins real versions,
# and tags/pushes each in dependency order so `go install …@latest` works.
# Use `DRY_RUN=1 make release` to preview without tagging or pushing.
release:
	@scripts/release.sh

publish-eng:
	$(MAKE) -C eng/go publish V=$(V)

publish-basic:
	$(MAKE) -C basic/go publish V=$(V) ENG=$(ENG)

publish-lang:
	$(MAKE) -C lang/go publish V=$(V) ENG=$(ENG) BASIC=$(BASIC)

publish-cmd:
	$(MAKE) -C cmd/go publish V=$(V) ENG=$(ENG) LANG=$(LANG)

# Show recent tags for every published module (newest first).
tags:
	@echo "==> eng/go";   git tag -l 'eng/go/v*'   --sort=-version:refname | head
	@echo "==> basic/go"; git tag -l 'basic/go/v*' --sort=-version:refname | head
	@echo "==> lang/go";  git tag -l 'lang/go/v*'  --sort=-version:refname | head
	@echo "==> cmd/go";   git tag -l 'cmd/go/v*'   --sort=-version:refname | head

# ---- coverage ----------------------------------------------------------
#
# Per-module coverage profiles land in $(COVER_DIR)/<module>.out. The
# `cover` target prints each module's totals plus an aggregate. The HTML
# variants render one report per module under $(COVER_DIR)/html/.

COVER_DIR := coverage

# cover-gate enforces ADR-008: 100% Go unit coverage of all reachable
# statements, at all times. Every module's tests run with -coverpkg
# spanning the whole repo (so a statement counts as covered when ANY
# suite reaches it — lang's tests legitimately cover eng, the spec corpus
# covers both), the per-module profiles are merged block-by-block by
# test/go/covergate, and the gate fails below GATE_FLOOR. The sole
# exclusions are provably-unreachable defensive guards, each marked with a
# //covergate:allow <reason> comment on its opening line (covergate reads
# them from source via -root); the gate fails if such a guard becomes
# covered (graduate it) or its pragma goes stale. Seam conventions for
# reaching the hard edges are in design/TEST-SEAMS.10.md; the exclusion
# policy is in design/COVERAGE-ALLOWLIST.10.md and ADR-008.
#
# The per-module runs carry an explicit -timeout because coverage
# instrumentation slows the corpus suites well past go test's 10m default —
# `make test` already passes 20m for the same reason, and without it here the
# gate fails at the MODULE step (not the threshold check) on a loaded machine,
# which reads as a coverage failure and is not one.
#
# The gate runs in SMALLER STEPS, individually addressable for iteration:
#
#   make cover-profile             profile every module (the slow loop)
#   make cover-profile m=eng/go    refresh ONE module's .xout profile
#   make cover-check               analysis only, over the .xout files
#                                  already in $(COVER_DIR) (fast)
#   make cover-gate                full re-profile + check — the
#                                  authoritative ADR-008 gate
#
# After a change confined to one module's tests, `make cover-profile
# m=<module> cover-check` re-verifies in that module's time instead of the
# whole loop. CAVEAT: cover-check merges whatever profiles are on disk, so
# mixing vintages is only sound while the measured files' line numbers are
# unchanged since the older profiles were written (coverage blocks are
# keyed by position); after edits to covered source, re-profile every
# module whose suites reach it — when in doubt, run the full cover-gate.
GATE_FLOOR ?= 100
GATE_PKGS := github.com/boru-lang/boru/...
COVER_MODS = $(if $(m),$(m),$(MODULES))
cover-profile:
	@mkdir -p $(COVER_DIR)
	@set -e; t0=$$(date +%s); n=0; total=$$(echo "$(COVER_MODS)" | wc -w); \
	for m in $(COVER_MODS); do \
	  n=$$((n + 1)); tm=$$(date +%s); \
	  echo "==> cover-profile $$m [$$n/$$total, $$((100 * (n - 1) / total))% done, $$((tm - t0))s elapsed]"; \
	  out="$(abspath $(COVER_DIR))/$$(echo $$m | tr '/' '_').xout"; \
	  ( cd $$m && go test -timeout 25m -coverpkg="$(GATE_PKGS)" -coverprofile=$$out ./... > "$$out.log" 2>&1 ) \
	    || { echo "==> cover-profile $$m FAILED — last lines of $$out.log:"; tail -40 "$$out.log"; exit 1; }; \
	  te=$$(date +%s); \
	  echo "    $$m profiled in $$((te - tm))s [$$((100 * n / total))% done, $$((te - t0))s elapsed]"; \
	done

cover-check:
	@echo "==> cover-check ($(abspath $(COVER_DIR))/*.xout)"
	@cd test/go && go run ./covergate -threshold $(GATE_FLOOR) -root $(CURDIR) $(abspath $(COVER_DIR))/*.xout

cover-gate:
	@$(MAKE) --no-print-directory cover-profile
	@$(MAKE) --no-print-directory cover-check
	@echo "==> cover-gate done"

# cover-gate-eng — the STANDALONE kernel gate (design/ENG-COVERAGE-
# PARITY.0.md): eng/go profiled by ITS OWN suite only (the standalone
# corpus lanes in eng/go/corpus_standalone_test.go plus the unit/seam
# tests), no other module's tests contributing. The floor is a RATCHET
# towards the 100% target: raise it as standalone coverage grows;
# never lower it. The TS twin's ratchet lives in `make test-ts`
# (node --test line-coverage threshold) — the two gates are the parity
# pair (Go statements ≡ TS lines).
#
# RE-BASED at the four-piece Stage 4 cut (design/ENG-FOUR-PIECE.0.md):
# the interpreter core's statements and ~120 kernel test files moved to
# core/go, taking their incidental eng-side coverage with them. The
# measurement universe changed — the pre-cut floor of 89 is not
# comparable — so the floor restarts at the post-cut measured value
# (84.6%) and the pair (cover-gate-eng, cover-gate-core) together
# supersedes the old single gate. Both ratchet independently to 100.
ENG_GATE_FLOOR ?= 84
# The standalone profile deliberately uses the .engout extension so the
# merged gate's cover-check (which globs $(COVER_DIR)/*.xout) NEVER
# merges it: the two gates run on different schedules, and a stale
# standalone profile carrying old line addresses poisons the merged
# view with phantom uncovered blocks after any source edit.
cover-gate-eng:
	@mkdir -p $(COVER_DIR)
	@rm -f $(COVER_DIR)/eng_standalone.xout
	@echo "==> cover-gate-eng (standalone, floor $(ENG_GATE_FLOOR)%)"
	@( cd eng/go && go test -timeout 25m \
	  -coverpkg=github.com/boru-lang/boru/eng/go/... \
	  -coverprofile=$(abspath $(COVER_DIR))/eng_standalone.engout ./... > $(abspath $(COVER_DIR))/eng_standalone.log 2>&1 ) \
	  || { echo "==> cover-gate-eng test run FAILED:"; tail -30 $(abspath $(COVER_DIR))/eng_standalone.log; exit 1; }
	@cd test/go && go run ./covergate -threshold $(ENG_GATE_FLOOR) -root $(CURDIR) $(abspath $(COVER_DIR))/eng_standalone.engout


# cover-gate-core — the CORE kernel's own gate (design/ENG-FOUR-PIECE.0.md
# Stage 5): core/go profiled by ITS OWN suite alone. The floor is a
# RATCHET towards 100%: raise it as core-standalone coverage grows;
# never lower it. Same .engout-family isolation as the eng gate so the
# merged cover-check never merges a stale standalone profile.
CORE_GATE_FLOOR ?= 100
cover-gate-core:
	@mkdir -p $(COVER_DIR)
	@rm -f $(COVER_DIR)/core_standalone.engout
	@echo "==> cover-gate-core (standalone, floor $(CORE_GATE_FLOOR)%)"
	@( cd core/go && go test -timeout 25m \
	  -coverpkg=github.com/boru-lang/boru/core/go/... \
	  -coverprofile=$(abspath $(COVER_DIR))/core_standalone.engout ./... > $(abspath $(COVER_DIR))/core_standalone.log 2>&1 ) \
	  || { echo "==> cover-gate-core test run FAILED:"; tail -30 $(abspath $(COVER_DIR))/core_standalone.log; exit 1; }
	@cd test/go && go run ./covergate -threshold $(CORE_GATE_FLOOR) -root $(CURDIR) $(abspath $(COVER_DIR))/core_standalone.engout

# cover-gate-check / cover-gate-compiler — the standalone gates for the
# two middle pieces (design/ENG-FOUR-PIECE.0.md Stage 6), the twins of
# cover-gate-core and cover-gate-eng: each module profiled by ITS OWN
# suite alone. Both floors are RATCHETS toward 100 — raise them in the
# same change that raises coverage, never lower them. The merged
# repo-wide ADR-008 gate (make cover-gate) stays the 100% contract; these
# measure how much each piece proves on its own.
#
# CHECK_GATE_FLOOR was RE-BASED at the 2026-08-08 carrier-lattice move
# (ADR-013's third amendment), on exactly the precedent ENG_GATE_FLOOR
# set at the Stage 4 cut: the measurement universe changed, so the
# pre-move floor is not comparable to the post-move one.
#
# What happened is worth spelling out, because the number went DOWN and
# that normally means a regression. It does not here. 492 statements left
# check for core (the join lattice, the body runners, guard narrowing,
# the carrier constructors, dead-overload detection), and their tests
# went with them — a MOVE, not a copy, so the merged ADR-008 gate is
# unaffected and not one covered statement was lost. But those 492 were
# better covered by check's own suite than check's average (377/492 =
# 77%, against 56% overall), so removing them lowered the RATIO:
#
#     before   1499/2672 = 56.1%
#     after    1122/2180 = 51.5%     (= (1499-377)/(2672-492))
#
# Re-basing rather than buying the difference back with ~130 statements
# of unrelated new check tests is the honest read: padding the gate to
# preserve a number that is measuring a different set of statements would
# make the ratchet a fiction. From 51 it ratchets UP as before — raise it
# in the same change that raises coverage, and never lower it again
# without a comparable structural reason recorded here.
CHECK_GATE_FLOOR ?= 51
COMPILER_GATE_FLOOR ?= 62

# cover-gate-parser — the parser's own gate. The parser is a LEAF over core
# (it uses 109 core symbols and nothing else from the repo), which is what
# makes a 100% standalone floor reasonable here from day one rather than as a
# ratchet: there is no other module's suite that could be covering it, and no
# seam whose far side lives elsewhere. Same .engout-family isolation as the
# other standalone gates so the merged cover-check never merges a stale
# profile.
PARSER_GATE_FLOOR ?= 100
cover-gate-parser:
	@mkdir -p $(COVER_DIR)
	@rm -f $(COVER_DIR)/parser_standalone.engout
	@echo "==> cover-gate-parser (standalone, floor $(PARSER_GATE_FLOOR)%)"
	@( cd parser/go && go test -timeout 25m \
	  -coverpkg=github.com/boru-lang/boru/parser/go/... \
	  -coverprofile=$(abspath $(COVER_DIR))/parser_standalone.engout ./... > $(abspath $(COVER_DIR))/parser_standalone.log 2>&1 ) \
	  || { echo "==> cover-gate-parser test run FAILED:"; tail -30 $(abspath $(COVER_DIR))/parser_standalone.log; exit 1; }
	@cd test/go && go run ./covergate -threshold $(PARSER_GATE_FLOOR) -root $(CURDIR) $(abspath $(COVER_DIR))/parser_standalone.engout

cover-gate-check:
	@mkdir -p $(COVER_DIR)
	@rm -f $(COVER_DIR)/check_standalone.engout
	@echo "==> cover-gate-check (standalone, floor $(CHECK_GATE_FLOOR)%)"
	@( cd check/go && go test -timeout 25m \
	  -coverpkg=github.com/boru-lang/boru/check/go/... \
	  -coverprofile=$(abspath $(COVER_DIR))/check_standalone.engout ./... > $(abspath $(COVER_DIR))/check_standalone.log 2>&1 ) \
	  || { echo "==> cover-gate-check test run FAILED:"; tail -30 $(abspath $(COVER_DIR))/check_standalone.log; exit 1; }
	@cd test/go && go run ./covergate -threshold $(CHECK_GATE_FLOOR) -root $(CURDIR) $(abspath $(COVER_DIR))/check_standalone.engout

cover-gate-compiler:
	@mkdir -p $(COVER_DIR)
	@rm -f $(COVER_DIR)/compiler_standalone.engout
	@echo "==> cover-gate-compiler (standalone, floor $(COMPILER_GATE_FLOOR)%)"
	@( cd compiler/go && go test -timeout 25m \
	  -coverpkg=github.com/boru-lang/boru/compiler/go/... \
	  -coverprofile=$(abspath $(COVER_DIR))/compiler_standalone.engout ./... > $(abspath $(COVER_DIR))/compiler_standalone.log 2>&1 ) \
	  || { echo "==> cover-gate-compiler test run FAILED:"; tail -30 $(abspath $(COVER_DIR))/compiler_standalone.log; exit 1; }
	@cd test/go && go run ./covergate -threshold $(COMPILER_GATE_FLOOR) -root $(CURDIR) $(abspath $(COVER_DIR))/compiler_standalone.engout

cover:
	@mkdir -p $(COVER_DIR)
	@set -e; for m in $(MODULES); do \
	  echo "==> cover $$m"; \
	  out="$(abspath $(COVER_DIR))/$$(echo $$m | tr '/' '_').out"; \
	  ( cd $$m && go test -coverprofile=$$out ./... ); \
	  ( cd $$m && go tool cover -func=$$out 2>/dev/null | tail -1 ) \
	    > "$(abspath $(COVER_DIR))/$$(echo $$m | tr '/' '_').total" 2>/dev/null || true; \
	done
	@echo
	@echo "==> per-module totals:"
	@for f in $(COVER_DIR)/*.out; do \
	  name=$$(basename $$f .out); \
	  total_file="$$(dirname $$f)/$$name.total"; \
	  if [ -s "$$total_file" ]; then \
	    total=$$(awk '/^total:/ {print $$3}' "$$total_file"); \
	  else total=N/A; fi; \
	  printf "  %-20s %s\n" "$$name" "$$total"; \
	done

cover-html: cover
	@mkdir -p $(COVER_DIR)/html
	@set -e; for m in $(MODULES); do \
	  name=$$(echo $$m | tr '/' '_'); \
	  f="$(abspath $(COVER_DIR))/$$name.out"; \
	  out="$(abspath $(COVER_DIR))/html/$$name.html"; \
	  [ -f "$$f" ] || continue; \
	  ( cd $$m && go tool cover -html=$$f -o $$out ) || true; \
	done
	@{ \
	  printf '<!doctype html>\n<html><head><meta charset="utf-8"><title>boru coverage</title>'; \
	  printf '<style>body{font:14px system-ui;margin:2em;max-width:1000px}h1{margin-bottom:.4em}'; \
	  printf 'table{border-collapse:collapse;margin-top:1em}td,th{border:1px solid #ddd;padding:6px 12px;text-align:left}'; \
	  printf 'a{color:#06c;text-decoration:none}a:hover{text-decoration:underline}</style></head><body>'; \
	  printf '<h1>boru coverage</h1>'; \
	  printf '<p>Generated %s</p>' "$$(date '+%Y-%m-%d %H:%M:%S')"; \
	  printf '<table><tr><th>Module</th><th>Coverage</th><th>Report</th></tr>'; \
	  for f in $(COVER_DIR)/*.out; do \
	    name=$$(basename $$f .out); \
	    total_file="$$(dirname $$f)/$$name.total"; \
	    if [ -s "$$total_file" ]; then \
	      total=$$(awk '/^total:/ {print $$3}' "$$total_file"); \
	    else total=N/A; fi; \
	    printf '<tr><td>%s</td><td>%s</td><td><a href="html/%s.html">view</a></td></tr>' "$$name" "$$total" "$$name"; \
	  done; \
	  printf '</table></body></html>'; \
	} > $(COVER_DIR)/index.html
	@echo "==> wrote $(COVER_DIR)/index.html"

# Open the combined coverage report. Tries `open` (macOS) then `xdg-open`.
cover-html-open: cover-html
	@(open $(COVER_DIR)/index.html 2>/dev/null || xdg-open $(COVER_DIR)/index.html 2>/dev/null || \
	  echo "open $(COVER_DIR)/index.html manually")

# ---- visualisation ----------------------------------------------------
#
# All viz output lands under $(VIZ_DIR)/<module>/. The viz targets run
# each Go-tool variant against every module so the whole codebase can be
# reviewed from one place. A top-level $(VIZ_DIR)/index.html aggregates
# every per-module index.html.

VIZ_DIR := viz

# Resolve GOBIN (where `go install` drops binaries). Honors $GOBIN, then
# $GOPATH/bin. Using absolute paths means we don't depend on $PATH.
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

CALLVIS    := $(GOBIN)/go-callvis
CALLGRAPH  := $(GOBIN)/callgraph
GODA       := $(GOBIN)/goda
GODEPGRAPH := $(GOBIN)/godepgraph
GOMOD      := $(GOBIN)/gomod
GOLDS      := $(GOBIN)/golds
GOPLANTUML := $(GOBIN)/goplantuml

# PlantUML renders the goplantuml-generated .puml to SVG. It's a Java jar,
# fetched once into ~/.cache/boru-viz and cached.
PLANTUML_JAR := $(HOME)/.cache/boru-viz/plantuml.jar
PLANTUML_URL := https://github.com/plantuml/plantuml/releases/latest/download/plantuml.jar

$(CALLVIS):
	go install github.com/ofabry/go-callvis@latest
$(CALLGRAPH):
	go install golang.org/x/tools/cmd/callgraph@latest
$(GODA):
	go install github.com/loov/goda@latest
$(GODEPGRAPH):
	go install github.com/kisielk/godepgraph@latest
$(GOMOD):
	go install github.com/Helcaraxan/gomod@latest
$(GOLDS):
	go install go101.org/golds@latest
$(GOPLANTUML):
	go install github.com/jfeliu007/goplantuml/cmd/goplantuml@latest
$(PLANTUML_JAR):
	@mkdir -p $(dir $(PLANTUML_JAR))
	curl -fsSL $(PLANTUML_URL) -o $(PLANTUML_JAR)

# Install every viz tool up front. Individual viz targets install on demand.
viz-tools: $(CALLVIS) $(CALLGRAPH) $(GODA) $(GODEPGRAPH) $(GOMOD) $(GOLDS) $(GOPLANTUML) $(PLANTUML_JAR)

# Per-module viz output directory.
mod_viz_dir = $(VIZ_DIR)/$(subst /,_,$1)

# Static call-graph (`-algo static`). Cluster + per-package detail SVGs.
# See VIZ-CALLGRAPH.md (if present) for the full pipeline; the awk
# functions are shared with the per-package split.
define CALLGRAPH_AWK_FUNCS
function pkg_of(name,    s, i, last_slash, dot_pos) {
  s = name; sub(/^\(\*?/, "", s); last_slash = 0;
  for (i = 1; i <= length(s); i++) if (substr(s, i, 1) == "/") last_slash = i;
  dot_pos = index(substr(s, last_slash + 1), ".");
  if (dot_pos == 0) return s;
  return substr(s, 1, last_slash + dot_pos - 1);
}
function leaf_of(name,    pkg, idx, p_len) {
  pkg = pkg_of(name); p_len = length(pkg) + 1;
  idx = index(name, pkg); if (idx == 0) return name;
  return substr(name, 1, idx - 1) substr(name, idx + p_len);
}
function pkg_leaf(p,    i, last_slash) {
  last_slash = 0;
  for (i = 1; i <= length(p); i++) if (substr(p, i, 1) == "/") last_slash = i;
  return (last_slash == 0) ? p : substr(p, last_slash + 1);
}
function is_exported(name,    leaf, t, j) {
  leaf = leaf_of(name);
  if (substr(leaf, 1, 1) == "(") {
    t = (substr(leaf, 2, 1) == "*") ? substr(leaf, 3) : substr(leaf, 2);
    if (substr(t, 1, 1) !~ /[A-Z]/) return 0;
    j = index(t, ").");
    if (j == 0) return 0;
    leaf = substr(t, j + 2);
  }
  return substr(leaf, 1, 1) ~ /[A-Z]/;
}
endef
export CALLGRAPH_AWK_FUNCS

define CALLGRAPH_MAIN_AWK
$(CALLGRAPH_AWK_FUNCS)
/^digraph/ || /^}/ { next }
{
  n_nodes = 0; copy = $$0;
  while (match(copy, /"[^"]+"/)) {
    name = substr(copy, RSTART + 1, RLENGTH - 2);
    n_nodes++; ln[n_nodes] = name;
    copy = substr(copy, RSTART + RLENGTH);
  }
  for (i = 1; i <= n_nodes; i++) {
    name = ln[i];
    if (!(name in seen) && is_exported(name)) {
      seen[name] = 1; p = pkg_of(name);
      pkgs[p] = pkgs[p] "    \"" name "\" [label=\"" leaf_of(name) "\" href=\"pkg_" pkg_leaf(p) ".svg\"];\n";
    }
  }
  if (/->/ && n_nodes == 2 && is_exported(ln[1]) && is_exported(ln[2])) {
    edges[++bn] = $$0;
  }
}
END {
  print "digraph callgraph {";
  print "  graph [compound=true, splines=true, rankdir=LR];";
  print "  node  [shape=box, fontsize=9, style=filled, fillcolor=\"#f8f8f8\"];";
  print "  edge  [color=\"#888\", arrowsize=0.6];";
  ci = 0;
  for (p in pkgs) {
    ci++;
    printf "  subgraph cluster_%d {\n    label=\"%s\";\n    style=\"rounded,filled\"; color=\"#bbb\"; fillcolor=\"#fafafa\";\n    href=\"pkg_%s.svg\";\n%s  }\n", ci, p, pkg_leaf(p), pkgs[p];
  }
  for (i = 1; i <= bn; i++) print "  " edges[i];
  print "}";
}
endef
export CALLGRAPH_MAIN_AWK

define CALLGRAPH_PERPKG_AWK
$(CALLGRAPH_AWK_FUNCS)
/^digraph/ || /^}/ { next }
{
  n_nodes = 0; copy = $$0;
  while (match(copy, /"[^"]+"/)) {
    name = substr(copy, RSTART + 1, RLENGTH - 2);
    n_nodes++; ln[n_nodes] = name;
    copy = substr(copy, RSTART + RLENGTH);
  }
  for (i = 1; i <= n_nodes; i++) {
    name = ln[i];
    if (!(name in seen)) {
      seen[name] = 1; p = pkg_of(name);
      pkg_nodes[p] = pkg_nodes[p] "    \"" name "\" [label=\"" leaf_of(name) "\"];\n";
    }
  }
  if (/->/ && n_nodes == 2 && pkg_of(ln[1]) == pkg_of(ln[2])) {
    p = pkg_of(ln[1]);
    pkg_edges[p] = pkg_edges[p] "  " $$0 "\n";
  }
}
END {
  for (p in pkg_nodes) {
    out = out_dir "/pkg_" pkg_leaf(p) ".dot";
    print "digraph pkg {" > out;
    print "  graph [splines=true, rankdir=LR, labelloc=t];" > out;
    print "  label=\"" p "\";" > out;
    print "  node  [shape=box, fontsize=9, style=filled, fillcolor=\"#f8f8f8\"];" > out;
    print "  edge  [color=\"#888\", arrowsize=0.6];" > out;
    printf "%s", pkg_nodes[p] > out;
    if (p in pkg_edges) printf "%s", pkg_edges[p] > out;
    print "}" > out;
    close(out);
  }
}
endef
export CALLGRAPH_PERPKG_AWK

CALLGRAPH_LAYOUT ?= dot

viz-callgraph: $(CALLGRAPH)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); \
	  mkdir -p $$d; rm -f $$d/pkg_*.dot $$d/pkg_*.svg; \
	  mod=$$(cd $$m && go list -m); \
	  echo "==> callgraph $$m ($$mod)"; \
	  ( cd $$m && $(CALLGRAPH) -algo static -format=graphviz ./... ) > $$d/callgraph.full.dot || \
	    { echo "  (callgraph skipped for $$m)"; continue; }; \
	  awk -v m="$$mod" ' \
	    { n = gsub(m, "&") } \
	    /^digraph/ || /^}/ { print; next } \
	    /->/ && n >= 2 { print; next } \
	    !/->/ && n >= 1 { print } \
	  ' $$d/callgraph.full.dot > $$d/callgraph.filtered.dot; \
	  awk "$$CALLGRAPH_MAIN_AWK" $$d/callgraph.filtered.dot > $$d/callgraph.dot; \
	  awk -v out_dir=$$d "$$CALLGRAPH_PERPKG_AWK" $$d/callgraph.filtered.dot; \
	  for f in $$d/callgraph.dot $$d/pkg_*.dot; do \
	    [ -f "$$f" ] || continue; \
	    $(CALLGRAPH_LAYOUT) -Tsvg "$$f" -o "$${f%.dot}.svg" 2>/dev/null || echo "  (dot missing for $$f)"; \
	  done; \
	done

viz-goda: $(GODA)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  echo "==> goda $$m"; \
	  ( cd $$m && $(GODA) graph ./... ) > $$d/goda.dot || continue; \
	  dot -Tsvg $$d/goda.dot -o $$d/goda.svg 2>/dev/null || true; \
	done

viz-godepgraph: $(GODEPGRAPH)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  mod=$$(cd $$m && go list -m); \
	  echo "==> godepgraph $$m ($$mod)"; \
	  ( cd $$m && $(GODEPGRAPH) -s $$mod ) > $$d/godepgraph.dot || continue; \
	  dot -Tsvg $$d/godepgraph.dot -o $$d/godepgraph.svg 2>/dev/null || true; \
	done

viz-gomod: $(GOMOD)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  echo "==> gomod $$m"; \
	  ( cd $$m && $(GOMOD) graph ) > $$d/gomod.dot || continue; \
	  dot -Tsvg $$d/gomod.dot -o $$d/gomod.svg 2>/dev/null || true; \
	done

viz-plantuml: $(GOPLANTUML) $(PLANTUML_JAR)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  echo "==> plantuml $$m"; \
	  ( cd $$m && $(GOPLANTUML) -recursive \
	    -show-aggregations -show-compositions -show-implementations \
	    -show-aliases -show-connection-labels \
	    -aggregate-private-members . ) > $$d/uml.puml 2>/dev/null || continue; \
	  java -jar $(PLANTUML_JAR) -tsvg -o $$(realpath $$d) $$d/uml.puml 2>/dev/null || true; \
	done

viz-list:
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  ( cd $$m && go list -deps -json ./... ) > $$d/deps.json; \
	done

viz-modgraph:
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  ( cd $$m && go mod graph ) > $$d/mod.graph; \
	done

# Build a top-level index.html that embeds every SVG under $(VIZ_DIR).
# SVGs are inlined (stripped of their XML prolog) and wired through
# svg-pan-zoom for drag-pan + scroll-zoom inside their 80vh frames.
# A nav at the top groups SVGs by module.
viz-index:
	@mkdir -p $(VIZ_DIR)
	@{ \
	  printf '<!doctype html>\n<html><head><meta charset="utf-8">'; \
	  printf '<title>boru — code structure</title>'; \
	  printf '<style>'; \
	  printf 'body{font:14px system-ui;margin:2em;max-width:1400px;color:#222}'; \
	  printf 'h1{margin-bottom:.2em}'; \
	  printf 'h2{margin-top:2em;padding-top:1em;border-top:1px solid #ddd}'; \
	  printf 'h3{margin-top:1.5em;color:#555}'; \
	  printf 'nav{margin:1em 0}nav details{margin-bottom:.4em}nav a{display:inline-block;margin-right:1em}'; \
	  printf '.frame{height:80vh;border:1px solid #ddd;background:#fff;overflow:hidden;position:relative;touch-action:none}'; \
	  printf '.frame > svg{width:100%%;height:100%%;display:block;cursor:grab}'; \
	  printf '.frame > svg:active{cursor:grabbing}'; \
	  printf '.meta{color:#888;font-size:.9em}'; \
	  printf 'a{color:#06c}'; \
	  printf '</style>'; \
	  printf '<script src="https://cdn.jsdelivr.net/npm/svg-pan-zoom@3.6.1/dist/svg-pan-zoom.min.js"></script>'; \
	  printf '</head><body>'; \
	  printf '<h1>boru — code structure</h1>'; \
	  printf '<p class="meta">Generated %s · drag to pan, scroll/pinch to zoom, double-click to reset</p>' "$$(date '+%Y-%m-%d %H:%M:%S')"; \
	  printf '<nav>'; \
	  for d in $(VIZ_DIR)/*/; do \
	    [ -d "$$d" ] || continue; \
	    name=$$(basename $$d); \
	    printf '<details><summary>%s</summary>' "$$name"; \
	    for f in $$d*.svg; do \
	      [ -f "$$f" ] || continue; \
	      svg=$$(basename "$$f"); \
	      printf '<a href="#%s-%s">%s</a>' "$$name" "$$svg" "$$svg"; \
	    done; \
	    printf '</details>'; \
	  done; \
	  printf '</nav>'; \
	  for d in $(VIZ_DIR)/*/; do \
	    [ -d "$$d" ] || continue; \
	    name=$$(basename $$d); \
	    printf '<h2>%s</h2>' "$$name"; \
	    for f in $$d*.svg; do \
	      [ -f "$$f" ] || continue; \
	      svg=$$(basename "$$f"); \
	      printf '<section id="%s-%s"><h3><a href="%s">%s</a></h3><div class="frame">' "$$name" "$$svg" "$$f" "$$svg"; \
	      sed -n '/<svg/,$$p' "$$f"; \
	      printf '</div></section>'; \
	    done; \
	  done; \
	  printf '<script>'; \
	  printf 'window.addEventListener("load", function () {'; \
	  printf '  document.querySelectorAll(".frame > svg").forEach(function (svg) {'; \
	  printf '    svg.removeAttribute("width"); svg.removeAttribute("height");'; \
	  printf '    svg.setAttribute("width", "100%%"); svg.setAttribute("height", "100%%");'; \
	  printf '    var pz = svgPanZoom(svg, {zoomEnabled:true, controlIconsEnabled:true, fit:true, center:true, minZoom:0.05, maxZoom:50});'; \
	  printf '    svg.addEventListener("dblclick", function () { pz.reset(); });'; \
	  printf '  });'; \
	  printf '});'; \
	  printf '</script></body></html>'; \
	} > $(VIZ_DIR)/index.html
	@echo "Wrote $(VIZ_DIR)/index.html — open with: open $(VIZ_DIR)/index.html"

# Run every viz target that works for library modules (skips viz-callvis
# which needs a single main package, and viz-golds which blocks on a server).
viz: viz-callgraph viz-goda viz-godepgraph viz-gomod viz-plantuml viz-list viz-modgraph viz-index
	@echo "Wrote artifacts under $(VIZ_DIR)/"

viz-clean:
	rm -rf $(VIZ_DIR)

# Single-package interactive call viewer; needs CALLVIS_PKG pointing at
# a directory with a main package. Default: cmd/go.
CALLVIS_PKG ?= ./cmd/go
viz-callvis: $(CALLVIS)
	@mkdir -p $(VIZ_DIR)
	$(CALLVIS) -file $(VIZ_DIR)/callvis -format svg $(CALLVIS_PKG)

# Local code browser. Blocks; open http://localhost:56789 in a browser.
GOLDS_PKG ?= ./...
viz-golds: $(GOLDS)
	@cd lang && $(GOLDS) -port=56789 $(GOLDS_PKG)
