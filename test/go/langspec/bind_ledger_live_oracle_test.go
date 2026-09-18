// bind_ledger_live_oracle_test.go is the STRONG oracle §6.5's staging calls
// for: the ledger's final depth per name compared against the depth the check
// pass ACTUALLY left in the live registry.
//
// TestBindLedgerDepthsCompose is self-contained by construction — it replays
// each name's transitions symbolically and can only catch internal
// incoherence. This one cannot be fooled the same way: Boru.NativeRegistry()
// exposes the registry the pass ran against, so a transition the ledger
// MISSED entirely (an install with no note, a teardown with no undef) shows
// up as a final-depth disagreement even when the recorded entries compose
// perfectly among themselves.
//
// This is the assertion the inert-twin step needs (§6.5 "assert that the
// replayed binding state matches what the pass left") — a twin replaying the
// ledger onto a rolled-back registry reproduces exactly the ledger's final
// depths, so any name where ledger and live registry disagree is a name the
// replay would get wrong.
//
// NO ALLOWANCE. A gate that reports a number it tolerates teaches the next
// author to tolerate it. When this fails, fix the ledger (or, where the
// mismatch is the ORACLE's own blind spot, fix the comparison) — never the
// number.
package langspec

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// ledgerFinalDepths reduces a ledger to the last recorded depth per name, in
// first-touch order (order only matters for stable reporting).
func ledgerFinalDepths(led []core.BindTransition) (map[string]int, []string) {
	final := map[string]int{}
	var order []string
	for _, tr := range led {
		if _, seen := final[tr.Name]; !seen {
			order = append(order, tr.Name)
		}
		final[tr.Name] = tr.Depth
	}
	return final, order
}

// liveOracleRow runs one row's compile pass and reports every disagreement
// between the ledger and the live registry — in BOTH directions: a ledgered
// name whose final recorded depth differs from the live depth, and (the
// direction Codex pointed out the first cut could not see, P2 on PR #418) a
// name whose live depth CHANGED across the pass with no ledger entry at all —
// a transition the ledger missed entirely, which the ledgered-names-only walk
// reported as success. The pre-pass depths of every then-known name are
// snapshotted for that second direction; a name the pass INTRODUCES silently
// (absent before, bound after, unledgered) is caught by walking the post-pass
// name set too.
// internerBinding reports whether name is one of the kernel's INTERNAL
// interner bindings — `__const:<id>:<v>` (const singleton types) and
// `__gen:<id>:<args>` (generic instantiations). These are compile-time
// products under §6.5's front-end carve-out (its site table already rules
// generic instantiation out of the twin population: "type instantiation is a
// compile-time product"): the compiled program bakes the minted type refs,
// and the interpreter re-creates the cache entries on demand, so no twin
// replays them and the unledgered direction must not read them as gaps.
func internerBinding(name string) bool {
	return strings.HasPrefix(name, "__")
}

func liveOracleRow(src, where string) []string {
	a, err := lang.New()
	if err != nil {
		return []string{fmt.Sprintf("%s lang.New: %v", where, err)}
	}
	defs := a.NativeRegistry().Defs
	pre := map[string]int{}
	for _, name := range defs.Names() {
		pre[name] = defs.Depth(name)
	}
	prog, _, res, _ := a.CompileCheck(src)
	final, order := ledgerFinalDepths(res.BindLedger)
	s := src
	if len(s) > 90 {
		s = s[:90] + "…"
	}
	var bad []string
	for _, name := range order {
		live := defs.Depth(name)
		if live != final[name] {
			bad = append(bad, fmt.Sprintf("%s name=%q ledger=%d live=%d  %s",
				where, name, final[name], live, s))
		}
	}
	// The UNLEDGERED direction runs only for rows that COMPILED: the
	// twin-replay contract it defends exists only where a Program does. A
	// refused or erroring row legitimately leaves partial state the ledger
	// does not model — an error path abandons mid-construct exactly as the
	// interpreter does (`gen [T] gen [U] …` raises with the outer gen binder
	// still pushed, in BOTH engines) — and no twin will ever replay it.
	if prog == nil {
		return bad
	}
	seen := map[string]bool{}
	for _, name := range defs.Names() {
		seen[name] = true
		if _, ledgered := final[name]; ledgered || internerBinding(name) {
			continue
		}
		if live := defs.Depth(name); live != pre[name] {
			bad = append(bad, fmt.Sprintf("%s name=%q UNLEDGERED depth change %d -> %d  %s",
				where, name, pre[name], live, s))
		}
	}
	for name, preD := range pre {
		if seen[name] {
			continue
		}
		if _, ledgered := final[name]; ledgered || internerBinding(name) {
			continue
		}
		// Bound before the pass, gone after, never ledgered.
		bad = append(bad, fmt.Sprintf("%s name=%q UNLEDGERED depth change %d -> 0  %s",
			where, name, preD, s))
	}
	return bad
}

// TestBindLedgerLiveDepths runs the strong oracle over the whole corpus plus
// the synthetic branch-arm rows the corpus lacks.
func TestBindLedgerLiveDepths(t *testing.T) {
	t.Parallel()
	for _, src := range syntheticBranchArmSources {
		for _, bad := range liveOracleRow(src, "synthetic") {
			t.Errorf("%s", bad)
		}
	}

	var mu sync.Mutex
	checked, mismatched := 0, 0
	var worst []string
	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		bad := liveOracleRow(r.Input, fmt.Sprintf("%s:%d", r.File, r.Line))
		if bad == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		checked++
		mismatched += len(bad)
		for _, b := range bad {
			if len(worst) < 20 {
				worst = append(worst, b)
			}
		}
	})
	sort.Strings(worst) // the worklist reads the same whichever worker saw a row first

	t.Logf("bind-ledger live-depth oracle: %d rows mismatched, %d name mismatches", checked, mismatched)
	for _, w := range worst {
		t.Logf("    %s", w)
	}
	if mismatched > 0 {
		t.Errorf("%d ledger-vs-live depth mismatches: the ledger disagrees with the state "+
			"the check pass left, so a twin replay would reproduce the wrong registry (§6.5)", mismatched)
	}
}
