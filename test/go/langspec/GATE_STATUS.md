| type-soundness violations | 5 | 0 | 5 | open | clean value rows whose checked residual type does not cover the actual — a wrong-TYPE checker finding |
| correct-error refusals | 1 | 0 | 1 | open | a known-to-error row must compile an OpTrap / RET error path, not refuse |
| compile refusals | 113 | 0 | 113 | open | corpus rows the compiler refuses — every one an open defect (design/COMPILABLE-SUBSET.md §5) |
| interpreter islands | 12 | 0 | 12 | open | compiled programs with an OpFallback span — an uncompiled region inside something called compiled |
| engine entries | 379 | 0 | 379 | open | unattributed interpreter runs on the compiled path, by seam: Engine.Run×379, CallBoru×253, RunResolved×40, runPooledSub×34, vm:island×23, InvokeCallback:callboru×13, vm:island-resolved×9 |
| runtime defers | 8 | 0 | 8 | open | vmDefer activations on the corpus walk — the VM bailing to the whole-program interpreter re-run, by site: vm:poly-nout-drift×3, vm:rematch-matched×3, vm:poly-no-match×2 |
| locally-resolved defers | 1 | 0 | 1 | open | VM bails a caller's own fallback absorbed, the program staying compiled: vm:poly-no-match×1 |
| interpreter-only rows | 0 | 0 | 3 | at end state | rows a word claims are irreducible — justify or compile |
| reducible (tier-2) rows | 17 | 0 | 17 | open | rows refused by a word-class gap the compiler does not model |
| compute gaps | 107 | 0 | 107 | open | real-compute rows that refuse |
| armed-only diagnostics | 16 | 0 | 16 | open | programs `boru check` calls clean and the compiler refuses — a user cannot diagnose them (NUR103) |
| diagnostic parity divergences | 358 | 0 | 358 | open | rows whose findings differ between the plain and the compile-armed check — the checker's verdict depends on who is asking (NUR103); top shapes:   95x  plain=unreachable_branch/if armed=;   32x  plain=no_signature/add armed=;   29x  plain=no_signature/g armed= |
| interp-entry census rows | 54 | 0 | 54 | open | corpus rows that run compiled and still enter the interpreter through an unattributed seam — the OpFallback island ceiling cannot see this (it counts disassembly spans, not a CallBoru inside a handler) |
