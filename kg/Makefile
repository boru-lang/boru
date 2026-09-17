# Makefile for the kg/ knowledge-graph pipeline.
#
# Everything runs FROM THIS DIRECTORY — boru imports and fixture paths
# resolve against the working directory. The `boru` binary comes from
# cmd/go (make -C ../cmd/go build).

BORU ?= ../cmd/go/bin/boru

.PHONY: all build check fmt test graph verify

all: fmt check test graph verify

# Ensure the boru binary exists.
build:
	$(MAKE) -C ../cmd/go build

# Static type-check every pipeline module and every test.
# --base . : a test under tests/ imports its siblings' PARENT
# (`import "./util.boru"`), which `boru tests/x.boru` resolves against the
# cwd and `boru check tests/x.boru` against the file's own directory —
# where the import silently found nothing and every KgX word came back
# undefined (28 phantom errors per test file). The flag anchors check where
# run anchors.
check:
	@for f in *.boru tests/*.boru; do \
	  echo "check $$f"; \
	  $(BORU) check --base . $$f || exit 1; \
	done

# Canonical formatting, in place.
#
# Part of `all` so the sources cannot drift out of canonical form: the
# formatter is idempotent, so once they are canonical this is a no-op on
# the tree and `make all` leaves nothing to commit.
#
# This target was a guarded no-op until 2026-07-29. It was disabled because
# `boru fmt` corrupted template interpolations (`hi ${name}` re-parsed the
# `${expr}` hole as a map literal) and because formatting these sources took
# ~10 minutes. Both are fixed: the formatter scans backtick literals
# verbatim (lang/go/formatter/format.go scanBacktick / scanInterpHole), and
# emitNode is memoised, which took kg/validate.boru from 53s to 40ms — the
# whole kg tree now formats in well under a second.
fmt:
	@for f in *.boru tests/*.boru; do \
	  $(BORU) fmt $$f || exit 1; \
	done

# Run the whole kg test suite (each file exits non-zero on failure).
test:
	@for f in tests/*_test.boru; do \
	  echo "== $$f"; \
	  $(BORU) $$f || exit 1; \
	done

# Rebuild the committed project knowledge graph
# (out/graph.json + .sql + .md).
graph:
	$(BORU) main.boru

# Is the COMMITTED graph still true of this working tree? Recomputes a
# content digest of every input file (go.work, every go.mod, the project
# candidate file, and each document it cites) and compares it with the
# digest stored in out/graph.json. Exits non-zero when they differ, so
# this can gate a commit.
#
# This is the check the `generated_at` stamp cannot do: that stamp is
# pinned to a constant so rebuilds stay byte-identical, which makes it
# useless as a freshness signal and actively misleading as one.
verify:
	$(BORU) verify.boru
