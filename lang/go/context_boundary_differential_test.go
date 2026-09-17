package lang_test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Context-boundary differential: does a `context set` inside a nested body
// stay inside it, and do the two engines agree?
//
// The interpreter runs every nested body by spawning a sub-engine, and
// `Engine.Run` pushes a child context layer — so interpreted, a nested body is
// a context boundary essentially for free. The VM has no such single seam: it
// reaches a body four different ways, and for a long time bracketed none of
// them, so `context set` inside a compiled `do` / `each` escaped into the
// parent scope. See design/verse-report-defects-investigation.0.md §B.
//
// Two things make this test the shape it is.
//
// First, THE DIVERGENCE WAS INVISIBLE TO THE WHOLE SUITE. Every
// compiled-differential gate the repo has passed with the bug present, and the
// two Go tests that assert the intended semantics by name
// (`TestContextSubEngineIsolation`, `TestContextSubEngineNewKeyDoesNotLeak`)
// run interpreter-only. A spec row that would have covered it was deliberately
// written interpreter-side, with a comment explaining that the compiled path
// was safe "as it currently compiles to a fallback island" — a justification
// that stopped being true when `do` gained a CallableSpec and began lowering
// to a closure, and nothing re-examined it. So the table is explicit about
// mode rather than trusting any single-engine assertion.
//
// Second, IT RECORDS HOW EACH FORM IS COVERED, because the coverage is
// two different mechanisms. Closure-unit bodies — `do`, `each`, `fold`,
// `filter`, `outer`, `scan`, and those same words nested inside a fn — are
// BRACKETED at the VM's re-entrant body entry (vmContext.enterBodyUnit). A
// body the compiler INLINES into the caller's unit — the `case` desugaring
// to a nested-`if` chain, `otherwise`'s list argument, list
// auto-evaluation, an interp-string hole — has no call to wrap, so it
// cannot be bracketed at all: instead, a context write through a handle
// read inside one REFUSES compilation (NUR054, the refusal rule (a defect the runtime absorbs))
// and the whole program runs on the interpreter, whose scoping is
// canonical. Those rows therefore agree the way the ungrouped map-slot row
// below agrees — both sides of the comparison are the interpreter — and if
// the emitter ever learns to lower them with a real context-frame opcode
// pair, they start exercising two engines instead of one.
//
// The refusal fires AT THE MINT — a `context` read inside the inline
// region refuses, because the region's layer has no compiled twin and every
// consumption that can tell it from the ambient layer (a write, an alias, an
// identity probe, a render, a baked container element) would diverge. A
// store handle bound OUTSIDE the region (`def s (context)` … `s set` in a
// case arm) is an in-place layer write that persists identically on both
// engines, and it KEEPS COMPILING — the compile-status rows in
// TestNur054InlineCtxRefusal pin both directions.
//
// If an agreeing row starts diverging, the frame (or the refusal) has
// regressed.
func TestContextBoundaryDifferential(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// wantDiverge marks a form the compiled path does NOT yet bracket.
		wantDiverge bool
		// why documents an expected divergence. Required when wantDiverge.
		why string
	}{
		// ---- closure-unit bodies: bracketed at enterBodyUnit ----
		{name: "do", src: `do [ context set y 1 5 ]
context has y/q`},
		{name: "nested do", src: `do [ do [ context set y 1 5 ] ]
context has y/q`},
		{name: "each", src: `each [ drop context set y 1 5 ] [1]
context has y/q`},
		{name: "fold", src: `fold [ drop drop context set y 1 5 ] [1]
context has y/q`},
		{name: "filter", src: `filter [ drop context set y 1 true ] [1]
context has y/q`},
		{name: "outer", src: `outer [ drop drop context set y 1 5 ] [1] [2]
context has y/q`},
		{name: "scan", src: `scan [ drop drop context set y 1 5 ] [1]
context has y/q`},
		// Nested inside a fn body: a different unit-nesting shape, and the one
		// the reverted patch's differential corpus found most reliably.
		{name: "do inside a fn body", src: `def f fn [[] [Any] [ do [ context set y 1 5 ] ]]
f
context has y/q`},
		{name: "each inside a fn body", src: `def f fn [[] [Any] [ each [ drop context set y 1 5 ] [1] ]]
f
context has y/q`},

		// ---- forms that are NOT boundaries in EITHER engine ----
		// Recorded because the note's own Blocker-2 table listed the fn-body
		// case as still leaking, and measurement says otherwise: a named fn
		// body and a called lambda leak in BOTH engines, so CALL_USER needs no
		// bracketing at all. Keeping these rows is what stops a future
		// "complete the fix" pass from bracketing CALL_USER and silently
		// changing the interpreter's answer too.
		{name: "if branch", src: `if true [ context set y 1 5 ] [ 6 ]
context has y/q`},
		{name: "for body", src: `for 1 [ context set y 1 5 ]
context has y/q`},
		{name: "named fn body", src: `def f fn [[] [Any] [ context set y 1 5 ]]
f
context has y/q`},
		{name: "called lambda", src: `def g ([] => [ context set y 1 5 ])
g
context has y/q`},
		// An `if` CODE-BODY condition is NoEval (handler-run, not list
		// auto-evaluation), so it is outside the NUR054 inline regions and
		// keeps compiling (TestNur054InlineCtxRefusal pins that); this row
		// pins that the engines also AGREE on where its write lands.
		{name: "if code-body condition", src: `if [ context set y 1 true ] [ 5 ] [ 6 ]
context has y/q`},

		// ---- INLINED bodies: not bracketable by any seam — covered by the
		// NUR054 refusal instead. Each of these programs REFUSES compilation
		// (`context` read inside the inline region hands out the region's own
		// layer; a set/del through that handle would land one scope too
		// shallow compiled), so both sides of the comparison are the
		// interpreter and the rows agree trivially. The refusal reason is
		// pinned by TestNur054InlineCtxRefusal.
		{name: "case clause body", src: `case 1 [ 1 [ context set y 1 5 ] 2 [ 6 ] ]
context has y/q`},
		{name: "otherwise list argument", src: `false otherwise [ context set y 1 5 ]
context has y/q`},
		{name: "def list auto-evaluation", src: `def b [ context set y 1 5 ]
context has y/q`},
		{name: "fn list argument never used", src: `def run fn [[b:List] [Any] [ 0 ]]
run [ context set y 1 5 ]
context has y/q`},
		// The newly-measured members of the same inline class, found while
		// closing NUR054: a collection-argument list, an interp-string hole,
		// and the `del` twin of the recorded `set` divergence.
		{name: "each collection list", src: `each [ drop 0 ] [ context set y 1 1 ]
context has y/q`},
		{name: "interp-string hole", src: "`x${context set y 1 5}`\ncontext has y/q"},
		{name: "context del in case arm", src: `context set y 9
case 1 [ 1 [ context del y 5 ] 2 [ 6 ] ]
context has y/q`},
		// The 2026-08-14 Codex review round's three confirmed escapes, closed
		// by moving the refusal to the MINT (`context` read in-region): a
		// re-IDed alias a write-site rule could not chase, an xml child hole
		// the region brackets had missed, and an identity probe through an
		// escaped handle.
		{name: "aliased write via dup in otherwise list",
			src: "false otherwise [ context dup drop set y 1 5 ]\ncontext has y/q"},
		{name: "xml child hole", src: `<p>${context set y 1 5}</p> drop
context has y/q`},
		{name: "identity eq in interp hole", src: "def s (context)\n`x${context eq s}`"},
		// Provenance precision: a store handle bound OUTSIDE the inline
		// region written inside it is an in-place layer write that persists
		// identically on both engines — it keeps compiling AND agrees.
		{name: "outside-bound store handle written in case arm", src: `def s (context)
case 1 [ 1 [ s set y 1 5 ] 2 [ 6 ] ]
context has y/q`},
		// `if` branches are not context boundaries in EITHER engine, so a
		// branch-joined `context` handle written at the top level is the
		// ambient layer on both — it keeps compiling AND agrees (measured
		// against the Codex round's contrary claim).
		{name: "branch-joined handle written at top level", src: `if true [ context ] [ context ] set y 1 5
context has y/q`},
		// PAREN-GROUPED, and FIXED — this row used to diverge in the OPPOSITE
		// direction (compiled CONTAINED the write, interpreted leaked it),
		// which is why it is worth keeping the mechanism written down. The
		// disassembly was the finding:
		//
		//	(m.f 1) drop → CALL_DYN_METHOD  ; (paren apply)/1 -> 1
		//	m.f 1 drop   → uncompilable: member fn value auto-applies
		//	               mid-expression (fn-value-call boundary, Stage 3)
		//
		// callDynMethod ISLANDS the apply — islandRun builds a sub-engine and
		// calls eng.Run — and Engine.Run is where the interpreter pushes its
		// per-body context layer. So the compiled path was MORE isolated than
		// the interpreter: it delegates the call to an interpreter island, and
		// the island's Run pushed a frame the interpreter's own inline
		// dispatch of the same tokens never pushes. Run now skips the frame
		// for an island (Engine.isIsland) — an island CONTINUES an expression,
		// it does not enter a body — which also matches the stated rule that a
		// paren form is never a boundary: the value is resolved and then made
		// available to the signature matcher.
		//
		// It was NOT the VM's enterBodyUnit frame, though that was the obvious
		// suspect. Verified by disabling that frame outright (Push and Pop both
		// removed, counted): the row still diverged. An earlier probe that
		// removed only the Push suggested otherwise and was unsound — it left a
		// Pop running against the parent's layer.
		{name: "paren-grouped map-slot lambda method", src: `def m {f: ([a:Integer] => [ context set y 1 a ])}
(m.f 1) drop
context has y/q`},
		// The ungrouped twin — and read it carefully, because it agrees
		// TRIVIALLY rather than by the same mechanism. The form does not
		// compile at all (the refusal above), so the program falls back to the
		// interpreter and both sides of the comparison ARE the interpreter.
		// Kept as its own row so that if the emitter ever learns to lower it,
		// this row starts exercising two real engines instead of one.
		{name: "ungrouped map-slot lambda method", src: `def m {f: ([a:Integer] => [ context set y 1 a ])}
m.f 1 drop
context has y/q`},
	}

	// The RATCHET. The per-row checks below already fail when a recorded
	// divergence heals (good news, move the row) or when an agreeing row
	// regresses. Neither catches the third direction: adding a NEW
	// `wantDiverge` row, which papers over a fresh regression and still
	// passes. An inventory that can grow silently is a licence, not a
	// budget — so the count is pinned, and enlarging it has to be a
	// deliberate, reviewable edit to this number with a reason.
	//
	// This may only go DOWN — and it reached 0 when the NUR054 refusal
	// landed: an inline-lowered form the compiler cannot bracket now refuses
	// compilation instead of answering differently, so no recorded
	// divergence remains. A new entry here means a fresh contract breach.
	const openDivergenceBudget = 0
	open := 0
	for _, c := range cases {
		if c.wantDiverge {
			open++
		}
	}
	if open > openDivergenceBudget {
		t.Errorf("interpreter/compiler context-boundary divergences: %d, budget %d.\n"+
			"A NEW divergence was added. The contract (design/COMPILABLE-SUBSET.md) is "+
			"that a form the compiler cannot lower faithfully must be REFUSED, not "+
			"answered differently; the refusal is contained, not fixed. If this row is genuinely "+
			"unavoidable for now, raise the budget deliberately and say why in NUR054.",
			open, openDivergenceBudget)
	}
	if open < openDivergenceBudget {
		t.Errorf("interpreter/compiler context-boundary divergences: %d, budget %d.\n"+
			"That is PROGRESS — lower the budget to %d so it cannot silently drift "+
			"back up, and update NUR054.", open, openDivergenceBudget, open)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.wantDiverge && c.why == "" {
				t.Fatal("an expected divergence must carry its reason")
			}
			a, _ := lang.New()
			compiled, cErr := a.Run(c.src)
			b, _ := lang.New()
			interp, iErr := b.RunInterp(c.src)
			if cErr != nil || iErr != nil {
				t.Fatalf("both engines must run the program — compiled: %v / interpreted: %v",
					cErr, iErr)
			}
			cs, is := fmt.Sprint(compiled), fmt.Sprint(interp)
			agree := cs == is

			switch {
			case c.wantDiverge && agree:
				t.Errorf("this form is recorded as an OPEN divergence but the engines "+
					"now agree (%s). That is progress, not a failure — move the row to "+
					"the agreeing set and drop its `why`.\n  reason on record: %s",
					cs, c.why)
			case !c.wantDiverge && !agree:
				t.Errorf("compiled %s != interpreted %s — the per-body context frame "+
					"has regressed. A `context set` inside this body escapes on one "+
					"engine and not the other, which is a silent wrong answer rather "+
					"than an error.", cs, is)
			}
		})
	}
}

// TestContextBoundaryAccumulates covers the property a single end-of-body
// check misses: the interpreter's boundary is per INVOCATION, so a compiled
// path that pushed one frame for the whole loop instead of one per element
// would pass every row above and still be wrong.
//
// The note's own repro: each element reads the counter, adds one, writes it
// back. Contained per element, every element sees 0 and the result is [1 1];
// leaking, the writes accumulate and it is [1 2].
func TestContextBoundaryAccumulates(t *testing.T) {
	const src = `context set 'z' 0
each [ drop context set 'z' ((context get 'z') add 1) end (context get 'z') ] [1 2]`
	a, _ := lang.New()
	compiled, cErr := a.Run(src)
	b, _ := lang.New()
	interp, iErr := b.RunInterp(src)
	if cErr != nil || iErr != nil {
		t.Fatalf("compiled: %v / interpreted: %v", cErr, iErr)
	}
	if fmt.Sprint(compiled) != fmt.Sprint(interp) {
		t.Errorf("per-element accumulation diverges: compiled %v != interpreted %v — "+
			"the frame is being pushed once for the loop rather than once per "+
			"element invocation", compiled, interp)
	}
	// And the frame must actually contain the write, not merely agree: two
	// engines that both leaked would pass the comparison above.
	if strings.Contains(fmt.Sprint(compiled), "2") {
		t.Errorf("got %v — the second element saw the first element's write, so the "+
			"body is not a boundary on either engine", compiled)
	}
}
