# spec — the Lean specification of the boru kernel.
#
#   make check             everything below (what CI should run)
#   make build             type-check the model and every theorem
#   make corpus            run the model over core/spec/*.tsv (the third runner)
#   make verify-typetable  the generated lattice still matches core/go
#   make fuzz              differential-test the model against core/go (a search, not a gate)
#
# Needs a Lean 4 toolchain (`lean-toolchain` pins it). With elan installed
# `lake` picks it up; otherwise put a matching `lake` on PATH or set LAKE.

LAKE ?= lake
SEEDS ?= 1 2 3
N ?= 4000

.PHONY: check build corpus verify-typetable typetable fuzz clean

check: verify-typetable build corpus

build:
	@echo "==> spec: build (model + theorems)"
	@$(LAKE) build Boru corespec

corpus: build
	@echo "==> spec: corpus (core/spec on the Lean model)"
	@$(LAKE) exe corespec ../core/spec

verify-typetable:
	@./tools/gen-typetable.sh --check

typetable:
	@./tools/gen-typetable.sh

fuzz: build
	@for s in $(SEEDS); do python3 tools/diff-fuzz.py --n $(N) --seed $$s || exit 1; done

clean:
	@rm -rf .lake
