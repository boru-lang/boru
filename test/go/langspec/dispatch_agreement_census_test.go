// dispatch_agreement_census_test.go — the admission-agreement census
// (design/FULL-COMPILATION.0.md §10, the Tier-1 instrument its correction
// note names).
//
// boru has TWO signature matchers. The interpreter plans and selects with
// Engine.MatchSignature (forward collection, barrier splits, /q word
// preference, the deferred-atom fallback); the VM re-matches at runtime
// with the kernel window matcher core.MatchSignature (signature.go), and
// §6.5 specifies OpDispatchGeneric as THAT matcher over the claimed
// window. Today a disagreement between them is at worst a defer; when the
// generic op EXECUTES on match, a disagreement is a wrong answer. This
// census measures the agreement surface over every dispatch the corpus
// COMMITS, from the probe seam (core.InstallDispatchProbe), and holds it
// to a COMMITTED ledger — the bind-ledger discipline, exact in both
// directions: an observed divergence shape with no ledger entry fails,
// and a ledgered shape that stops occurring fails as stale. Shapes are
// keyed class/word (Codex P1, PR #420): a class-level key would let a
// NEWLY diverging word fold silently into an existing class.
//
// THE WINDOW MAPPING IS THE POINT, so it is explicit: the two matchers
// consume OPPOSITE conventions. The planner returns positions[] as
// TAPE-ABSOLUTE slots in signature order; the kernel matcher reads
// window[i] as sig position i (exactly how the VM builds its windows:
// window[i] = stack[len-1-i], eng/go/vm.go). So the census rebuilds the
// claimed window as window[i] = Tape.At(positions[i]) and hands it to the
// kernel with the VM's own exact-arity WordInfo. Windows the plan filled
// with material that only resolves at ARRIVAL — a /q word, a paren
// group, a token NESTED inside a concrete container shell like
// {a:word(true)} (Codex P2) — are not comparable at plan time: skipped
// and COUNTED, never compared or silently dropped.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// agreementLedger is the committed divergence ledger, keyed class/word.
// Measured 2026-08-31 over the whole corpus (see the census log for the
// exact windows): 23 divergent dispatches across these shapes, each a
// §10 T2 reconciliation candidate — one commit and one spec row per
// closed shape — and none reachable by today's compiled lanes (no
// generic dispatch executes yet; the VM's kernel-matcher calls are poly
// re-matches over recorder-proven windows, none of these shapes).
//
// The two MECHANISMS behind the shapes:
//   - split-sensitive selection: for a window two overloads accept, the
//     planner's choice depends on the CALL FORM (forward-phase candidate
//     order) — the two-map Store shapes and emit's Function+map — or on
//     bare type-node operands tripping the planner-vs-kernel
//     rejectsTypeLiteral scope difference (gt/lt). The generic lane must
//     re-create the planner's selection from OpCollect's descriptor
//     (which carries the split), never from the bare window.
//   - pattern-admission difference: /q- and pattern-heavy sigs (def,
//     use, 1-arg remove) whose QuoteArgs/pattern admission the kernel's
//     pattern loop answers differently from the planner's patternsOk.
var agreementLedger = map[string]string{
	"kernel-different-overload/create": "split-sensitive selection (two-map Store shape)",
	"kernel-different-overload/emit":   "split-sensitive selection (Function+map service shape)",
	"kernel-different-overload/gt":     "rejectsTypeLiteral scope difference on bare type-node operands",
	"kernel-different-overload/load":   "split-sensitive selection (two-map Store shape)",
	"kernel-different-overload/lt":     "rejectsTypeLiteral scope difference on bare type-node operands",
	"kernel-different-overload/remove": "split-sensitive selection (two-map Store shape)",
	"kernel-different-overload/update": "split-sensitive selection (two-map Store shape)",
	"kernel-no-match/def":              "pattern-admission difference (/q + type-operand sig)",
	"kernel-no-match/remove":           "pattern-admission difference (1-arg Store form)",
	"kernel-no-match/use":              "pattern-admission difference (pattern sig over a map operand)",
}

// timingDependentShapes marks ledger entries whose witness rows are
// MACHINE-SPEED-DEPENDENT (interval/await-flavored corpus rows iterate a
// different number of times per host — the probed-dispatch total itself
// varies by ~0.5% between this container and CI), so their occurrence
// count can legitimately reach zero on a fast host. The STALE direction
// of the gate skips exactly these (measured: CI saw 18 of the 20 local
// divergences, with kernel-no-match/remove absent and use at ×1); the
// UNLEDGERED direction — the safety one — still fails everywhere, for
// every shape.
var timingDependentShapes = map[string]bool{
	"kernel-no-match/remove": true,
	"kernel-no-match/use":    true,
}

// censusGoid parses the current goroutine's id from runtime.Stack's
// header — a census-only cost (~1µs per probed dispatch), and the price
// of suppressing EXACTLY the census's own artifacts: the kernel rematch
// below evaluates predicate slots (RunPredicate), whose boru bodies
// dispatch — on the SAME goroutine, by construction — re-entering this
// probe (the first census build deadlocked on its own mutex there). A
// goroutine-blind busy flag also swallowed dispatches from CONCURRENT
// corpus branches (TimeUtil.await rows — Codex P2, PR #420); with the
// goid check, those block briefly on the mutex and are processed, never
// dropped.
func censusGoid() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	if i := strings.IndexByte(s, ' '); i > 0 {
		if id, err := strconv.ParseInt(s[:i], 10, 64); err == nil {
			return id
		}
	}
	return -1
}

// windowComparable reports whether a matched operand is a RUNTIME value
// the kernel matcher can be handed at plan time. Token kinds resolve at
// ARRIVAL — and that includes tokens nested inside a concrete List/Map
// literal shell ({a:word(true)}), so exact-List/Map containers are
// walked recursively (depth-bounded; exotic container kinds are not
// materialized here — a token hiding in one would surface as a ledgered
// divergence, visible rather than silent).
func windowComparable(v core.Value, depth int) bool {
	if depth > 32 {
		return false
	}
	if core.IsWord(v) || core.IsForward(v) || core.IsMark(v) || core.IsMove(v) {
		return false
	}
	if !core.IsConcrete(v) && !core.IsBareTypeNode(v) {
		return false
	}
	if v.Parent != nil && v.Parent.Equal(core.TList) {
		if rl, err := core.AsList(v); err == nil {
			for i := 0; i < rl.Len(); i++ {
				if ev, ok := rl.GetOk(i); ok && !windowComparable(ev, depth+1) {
					return false
				}
			}
		}
	}
	if v.Parent != nil && v.Parent.Equal(core.TMap) {
		if rm, err := core.AsMap(v); err == nil && rm != nil {
			for _, k := range rm.Keys() {
				if mv, ok := rm.Get(k); ok && !windowComparable(mv, depth+1) {
					return false
				}
			}
		}
	}
	return true
}

func TestDispatchAdmissionAgreementCensus(t *testing.T) {
	type stats struct {
		agreed, engineNoMatch, speculative int
		skipped                            map[string]int
		diverged                           map[string]int
		examples                           map[string]string
	}
	s := stats{skipped: map[string]int{}, diverged: map[string]int{}, examples: map[string]string{}}
	var mu sync.Mutex
	var rematchGoid atomic.Int64
	var artifactSkips atomic.Int64 // atomic, NOT mu: the rematching call holds mu

	uninstall := core.InstallDispatchProbe(func(e *core.Engine, fn *core.FnDefInfo, w core.WordInfo, sig *core.Signature, positions []int, specAt int) {
		g := censusGoid()
		if rematchGoid.Load() == g {
			artifactSkips.Add(1) // the census's own rematch dispatching (predicate bodies)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if sig == nil {
			// The dispatch failed — the error/recovery path follows, not a
			// generic-lane execution. Counted, not compared: the kernel's
			// view of failures is OpDispatchRematch's territory.
			s.engineNoMatch++
			return
		}
		if specAt >= 0 {
			// A forward slot filled with a WORD that dispatches at runtime
			// (the speculation marker): the plan-time window is not the
			// runtime window by the plan's own admission.
			s.speculative++
			return
		}
		if sig.Fallback {
			// A 0-arg catch-all firing to raise "no matching signature" —
			// an error path, not a surface the generic lane executes.
			s.skipped["fallback-sig"]++
			return
		}
		window := make([]core.Value, len(positions))
		for i, p := range positions {
			if p < 0 || p >= e.Tape.Len() {
				s.skipped["position-out-of-tape"]++
				return
			}
			v := e.Tape.At(p)
			if !windowComparable(v, 0) {
				s.skipped["not-comparable-at-plan-time"]++
				return
			}
			window[i] = v
		}
		rematchGoid.Store(g)
		mr := core.MatchSignature(fn.Signatures, window, core.WordInfo{ArgCount: len(window)})
		rematchGoid.Store(0)
		class := ""
		switch {
		case mr == nil || mr.Sig == nil:
			class = "kernel-no-match"
		case mr.Sig != sig:
			class = "kernel-different-overload"
		default:
			s.agreed++
			return
		}
		key := class + "/" + fn.Name
		s.diverged[key]++
		if _, ok := s.examples[key]; !ok {
			s.examples[key] = fmt.Sprintf("arity=%d window=%s", len(window), renderWindow(window))
		}
	})
	defer uninstall()

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, ferr := os.Open(filepath.Join(specDir, e.Name()))
		if ferr != nil {
			t.Fatal(ferr)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			a, aerr := lang.New()
			if aerr != nil {
				continue
			}
			a.SetClock(specClock)
			_, _ = a.RunInterp(strings.TrimSpace(parts[0]))
			rows++
		}
		_ = f.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	s.skipped["census-rematch-artifact"] = int(artifactSkips.Load())
	total := s.agreed + s.engineNoMatch + s.speculative
	for _, n := range s.skipped {
		total += n
	}
	divergedTotal := 0
	for _, n := range s.diverged {
		total += n
		divergedTotal += n
	}
	t.Logf("dispatch admission agreement: %d rows, %d dispatches probed — %d agreed, %d diverged, %d engine-no-match, %d speculative, skipped %v",
		rows, total, s.agreed, divergedTotal, s.engineNoMatch, s.speculative, s.skipped)
	var keys []string
	for k := range s.diverged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("    %s ×%d  %s  (ledgered: %q)", k, s.diverged[k], s.examples[k], agreementLedger[k])
	}

	if s.agreed == 0 {
		t.Fatal("no dispatch was compared — the probe or the eligibility rules are broken, not the corpus")
	}
	for k := range s.diverged {
		if _, ok := agreementLedger[k]; !ok {
			t.Errorf("UNLEDGERED divergence %q (×%d, %s): the planner and the kernel matcher disagree on an admission the generic lane will inherit — ledger it with its mechanism, or reconcile it (one commit, one spec row, per §10's T2)",
				k, s.diverged[k], s.examples[k])
		}
	}
	for k, why := range agreementLedger {
		if s.diverged[k] == 0 && !timingDependentShapes[k] {
			t.Errorf("stale ledger entry %q (%s): the divergence no longer occurs — delete the entry so the ledger stays exact", k, why)
		}
	}
}

func renderWindow(window []core.Value) string {
	parts := make([]string, 0, len(window))
	for _, v := range window {
		s := v.String()
		if len(s) > 24 {
			s = s[:24] + "…"
		}
		parts = append(parts, s)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
