| armed-only diagnostics | 11 | 0 | 11 | open | programs `boru check` calls clean and compiling FAILS — a user cannot diagnose them (NUR103) |
| compile failures | 53 | 0 | 53 | open | corpus rows that FAIL to compile — every one a BUG, not a policy (design/COMPILABLE-SUBSET.md §5); the sum of compile_failures.tsv |
| compute gaps | 49 | 0 | 49 | open | real-compute rows that fail to compile |
| correct-error compile failures | 1 | 0 | 1 | open | a known-to-error row must compile an OpTrap / RET error path; failing to compile it is a bug |
| diagnostic parity divergences | 351 | 0 | 351 | open | rows whose findings differ between the plain and the compile-armed check — the checker's verdict depends on who is asking (NUR103); top shapes:   95x  plain=unreachable_branch/if armed=;   32x  plain=no_signature/add armed=;   29x  plain=no_signature/g armed= |
| engine entries | 422 | 0 | 420 | REGRESSION | unattributed interpreter runs on the compiled path, by seam: Engine.Run×422, CallBoru×241, RunResolved×112, runPooledSub×26, vm:island×13, vm:island-resolved×9, InvokeCallback:callboru×7 |
| interp-entry census rows | 80 | 0 | 78 | REGRESSION | corpus rows that run compiled and still enter the interpreter through an unattributed seam — the OpFallback island ceiling cannot see this (it counts disassembly spans, not a CallBoru inside a handler) |
| interpreter islands | 0 | 0 | 0 | at end state | compiled programs with an OpFallback span — an uncompiled region inside something called compiled |
| interpreter-only rows | 0 | 0 | 3 | at end state | rows a word claims are irreducible — justify or compile |
| locally-resolved defers | 1 | 0 | 1 | open | VM bails a caller's own fallback absorbed, the program staying compiled: vm:poly-no-match×1 |
| reducible (tier-2) rows | 3 | 0 | 3 | open | rows that fail to compile because of a word-class gap the compiler does not model |
| runtime defers | 8 | 0 | 8 | open | vmDefer activations on the corpus walk — the VM abandoning the run, by site: vm:poly-nout-drift×3, vm:rematch-matched×3, vm:poly-no-match×2 |
| type-soundness violations | 5 | 0 | 5 | open | clean value rows whose checked residual type does not cover the actual — a wrong-TYPE checker finding |
