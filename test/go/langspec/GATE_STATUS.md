| armed-only diagnostics | 16 | 0 | 16 | open | programs `boru check` calls clean and compiling FAILS — a user cannot diagnose them (NUR103) |
| compile failures | 113 | 0 | 113 | open | corpus rows that FAIL to compile — every one a BUG, not a policy (design/COMPILABLE-SUBSET.md §5) |
| compute gaps | 104 | 0 | 104 | open | real-compute rows that refuse |
| correct-error compile failures | 1 | 0 | 1 | open | a known-to-error row must compile an OpTrap / RET error path; failing to compile it is a bug |
| diagnostic parity divergences | 358 | 0 | 358 | open | rows whose findings differ between the plain and the compile-armed check — the checker's verdict depends on who is asking (NUR103); top shapes:   95x  plain=unreachable_branch/if armed=;   32x  plain=no_signature/add armed=;   29x  plain=no_signature/g armed= |
| engine entries | 366 | 0 | 366 | open | unattributed interpreter runs on the compiled path, by seam: Engine.Run×366, CallBoru×250, RunResolved×40, runPooledSub×26, vm:island×21, InvokeCallback:callboru×10, vm:island-resolved×9 |
| interp-entry census rows | 52 | 0 | 52 | open | corpus rows that run compiled and still enter the interpreter through an unattributed seam — the OpFallback island ceiling cannot see this (it counts disassembly spans, not a CallBoru inside a handler) |
| interpreter islands | 10 | 0 | 10 | open | compiled programs with an OpFallback span — an uncompiled region inside something called compiled |
| interpreter-only rows | 0 | 0 | 3 | at end state | rows a word claims are irreducible — justify or compile |
| locally-resolved defers | 1 | 0 | 1 | open | VM bails a caller's own fallback absorbed, the program staying compiled: vm:poly-no-match×1 |
| reducible (tier-2) rows | 17 | 0 | 17 | open | rows refused by a word-class gap the compiler does not model |
| runtime defers | 8 | 0 | 8 | open | vmDefer activations on the corpus walk — the VM bailing to the whole-program interpreter re-run, by site: vm:poly-nout-drift×3, vm:rematch-matched×3, vm:poly-no-match×2 |
| type-soundness violations | 5 | 0 | 5 | open | clean value rows whose checked residual type does not cover the actual — a wrong-TYPE checker finding |
