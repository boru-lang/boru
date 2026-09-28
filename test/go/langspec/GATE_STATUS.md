| armed-only diagnostics | 4 | 0 | 4 | open | programs `boru check` calls clean and compiling FAILS — a user cannot diagnose them (NUR103) |
| compile failures | 0 | 0 | 0 | at end state | corpus rows that FAIL to compile — every one a BUG, not a policy (design/COMPILABLE-SUBSET.md §5); the sum of compile_failures.tsv |
| compute gaps | 0 | 0 | 5 | at end state | real-compute rows that fail to compile |
| correct-error compile failures | 0 | 0 | 1 | at end state | a known-to-error row must compile an OpTrap / RET error path; failing to compile it is a bug |
| diagnostic parity divergences | 30 | 0 | 30 | open | rows whose findings differ between the plain and the compile-armed check — the checker's verdict depends on who is asking (NUR103); top shapes:   2x  plain= armed=fn_body_error/f|type_error/f;   2x  plain=no_signature/add armed=;   2x  plain=no_signature/convert armed= |
| engine entries | 171 | 0 | 171 | open | unattributed interpreter runs on the compiled path, by seam: Engine.Run×171, CallBoru×132, RunResolved×19, vm:island-resolved×7, InvokeCallback:callboru×5, runPooledSub×5, vm:island×5 |
| interp-entry census rows | 27 | 0 | 27 | open | corpus rows that run compiled and still enter the interpreter through an unattributed seam — the OpFallback island ceiling cannot see this (it counts disassembly spans, not a CallBoru inside a handler) |
| interpreter islands | 0 | 0 | 0 | at end state | compiled programs with an OpFallback span — an uncompiled region inside something called compiled |
| interpreter-only rows | 0 | 0 | 3 | at end state | rows a word claims are irreducible — justify or compile |
| locally-resolved defers | 1 | 0 | 1 | open | VM bails a caller's own fallback absorbed, the program staying compiled: vm:poly-no-match×1 |
| reducible (tier-2) rows | 0 | 0 | 4 | at end state | rows that fail to compile because of a word-class gap the compiler does not model |
| runtime defers | 0 | 0 | 0 | at end state | vmDefer activations on the corpus walk — the VM abandoning the run, by site: none |
| sweep call-form crashes | 2 | 0 | 2 | open | call-form variants of passing seeds an engine PANICS on or never answers |
| sweep call-form failures | 302 | 0 | 302 | open | call-form variants of passing seeds that fail to compile or island |
| sweep compile failures | 0 | 0 | 0 | at end state | valid seed programs that FAIL to compile — every one a BUG; SWEEP_STATUS.md lists them |
| sweep crashes | 0 | 0 | 0 | at end state | valid seed programs an engine PANICS on or never answers — recovered or abandoned by the classifier so the sweep goes on |
| sweep empty cells | 0 | 0 | 0 | at end state | word × operand-kind cells of the generated sweep with no seed program — holes in the instrument (test/go/sweep/seeds.tsv) |
| sweep invalid seeds | 0 | 0 | 0 | at end state | seed programs the interpreter rejects — fix the seed, or claim n/a with a probe |
| sweep islands | 2 | 0 | 2 | open | valid seed programs that compile with an interpreter island |
| type-soundness violations | 3 | 0 | 3 | open | clean value rows whose checked residual type does not cover the actual — a wrong-TYPE checker finding |
