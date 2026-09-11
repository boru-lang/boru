package compiler

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// event_kind_census_test.go is the structural guard NUR137 earned.
//
// Twice a widening has admitted a new EVENT KIND to a producer set without
// teaching the predicates keyed on kind about it, and both times the gap was
// silent rather than loud:
//
//   - NUR133: `regionReadsTheStack` did not read a user call's or a
//     fallback's own operands, so a mark opened above a value the op popped.
//   - NUR137: the same function's DEFAULT read `ev.call.ops` for a branch,
//     whose operands live in `ev.br` — a zero-value payload answering a
//     soundness question, wrongly, for four increments, on the default lane.
//
// A comment did not hold that line: NUR133's fix left one in the very
// function that then broke, saying exactly what would go wrong. What holds a
// line is a test that FAILS when the premise changes.
//
// The lists below are checked against the SOURCE, not maintained by hand,
// and that is not fastidiousness — the hand-written first cut named eight
// sites, a review caught three more, and parsing the package found NINETEEN.
// A prompt that is wrong about where to look is worse than no prompt.

// eventKindSites: keyed on EVENT kind — a new event kind changes what each
// one should do. The trailing note says what an UNNAMED kind gets today.
var eventKindSites = map[string]string{
	"regionReadsTheStack":     "emit.go  — does the region's event pop something already on the stack? (default: reads the stack — NUR137)",
	"eventPos":                "emit.go  — the event's source position (no default; an unnamed kind reports 0:0 — see NUR130 on what a wrong position costs)",
	"eventDivergesDeep":       "emit.go  — does this event never return past itself? (no default)",
	"eventsBindDynScope":      "emit.go  — does this event bind a registry-visible name? (no default)",
	"callResultPlacedIn":      "emit.go  — where a call's result lands",
	"forEachOperand":          "lower.go — every enclosing-scope operand the event references (a missing case drops values: unvisited is unreferenced, so a live producer is marked dead)",
	"forEachFragmentOperand":  "lower.go — the same walk over a fragment's own events",
	"eachClosureCap":          "lower.go — the event's CLOSURE captures (a missing case leaves captures stale)",
	"childFragments":          "lower.go — the event's nested fragments (a missing case hides a whole subtree)",
	"RewritePromotedRefs":     "lower.go — rewrites promoted operand refs (a missing case leaves them stale)",
	"collectPromotableEvents": "lower.go — which events may be promoted to frame locals",
	"planValueDefLocals":      "lower.go — the promotion plan itself",
	"lowerEvents":             "lower.go — the emission (default: REFUSES, \"unknown event kind\" — the shape worth copying)",
	"singleOutputCall":        "lower.go — is this a promotable single-result call? (default: false)",
	"fragSingleResidual":      "lower.go — does the fragment net exactly one value?",
	"fragmentOuts":            "lower.go — the fragment's out operands",
	"fragmentResultSeqs":      "lower.go — the seqs a fragment's results come from",
	"markTailCalls":           "lower.go — marks a body's tail call",
	"computeLeaverPrefix":     "lower.go — the prefix a diverging arm leaves",
}

// operandKindSites key on OPERAND kind (opConst / opLocal / opEvent / …), a
// different enum a new EVENT kind cannot reach. Listed so the completeness
// check can tell "classified as irrelevant" from "never looked at".
var operandKindSites = map[string]bool{
	"appendResidualSeqs":    true,
	"callResultRenderKnown": true,
	"closureOpShape":        true,
	"computedArmCondOK":     true,
	"deoptDeferred":         true,
	"pushOperand":           true,
	"regionSourceOf":        true,
	"residualReadHazard":    true,
}

// evKinds is every event kind, with the name a failure should print.
var evKinds = map[int]string{
	evCall:     "evCall",
	evBranch:   "evBranch",
	evLoop:     "evLoop",
	evBreak:    "evBreak",
	evContinue: "evContinue",
	evCallUser: "evCallUser",
	evFallback: "evFallback",
	evTrap:     "evTrap",
	evStore:    "evStore",
	evDynBind:  "evDynBind",
	evBindTwin: "evBindTwin",
}

// TestEventKindCensus fails when the set of event kinds changes. That is the
// whole point: the failure is the prompt to walk eventKindSites and decide,
// per site, what the new kind means there — instead of inheriting whatever
// its default happens to do.
func TestEventKindCensus(t *testing.T) {
	// evKindEnd is one past the last kind, so this count changes the moment a
	// kind is added above it — which is the whole mechanism.
	if got, want := evKindEnd-1, len(evKinds); got != want {
		t.Fatalf("event kinds changed: %d kinds, census has %d entries.\n\n"+
			"Add it to evKinds here, then teach each of these sites — every one is keyed on\n"+
			"ev.kind, and a default that merely 'works' is how NUR133 and NUR137 both shipped:\n  %s",
			got, want, joinLines(siteLines()))
	}
	for k, name := range evKinds {
		if k < 1 || k >= evKindEnd {
			t.Errorf("%s = %d is outside the iota range 1..%d", name, k, evKindEnd-1)
		}
	}
}

// TestKindKeyedSiteCensus parses this package and proves the two lists above
// are COMPLETE — every `switch X.kind` in a non-test file sits in a function
// one of them classifies. The census is only as good as the prompt it
// prints, so the prompt is checked against the source rather than against
// anyone's memory of it.
func TestKindKeyedSiteCensus(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}
	var unclassified []string
	for _, pkg := range pkgs {
		for path, f := range pkg.Files {
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					sw, ok := n.(*ast.SwitchStmt)
					if !ok || sw.Tag == nil {
						return true
					}
					se, ok := sw.Tag.(*ast.SelectorExpr)
					if !ok || se.Sel.Name != "kind" {
						return true
					}
					if _, known := eventKindSites[fd.Name.Name]; known || operandKindSites[fd.Name.Name] {
						return true
					}
					unclassified = append(unclassified, fmt.Sprintf("%s (%s:%d)",
						fd.Name.Name, strings.TrimPrefix(path, "./"), fset.Position(sw.Pos()).Line))
					return true
				})
			}
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("%d kind-keyed switch(es) in functions neither list classifies:\n  %s\n\n"+
			"Decide which enum each keys on and add it: eventKindSites if a new EVENT kind\n"+
			"changes what it should do (then say what its default gives an unnamed kind), or\n"+
			"operandKindSites if it keys on operand kind and a new event kind cannot reach it.",
			len(unclassified), joinLines(unclassified))
	}
}

// TestForEachOperandHandlesEveryKind walks a well-formed event of EVERY kind
// through the operand walker. Its `default` arm contributes nothing and used
// to dereference ev.br, so a kind added without a case would either drop
// operands or panic — and ADR-005 forbids the second outright.
func TestForEachOperandHandlesEveryKind(t *testing.T) {
	for kind, name := range evKinds {
		t.Run(name, func(t *testing.T) {
			seen := 0
			forEachOperand(wellFormedEvent(kind), func(EmitOperand) { seen++ })
			// evBindTwin marks a position and references nothing, and the two
			// flow terminators carry no operands either. Every OTHER kind has
			// at least one operand slot, so visiting none means a missing
			// case — which is exactly what this catches.
			if kind != evBindTwin && kind != evBreak && kind != evContinue && seen == 0 {
				t.Errorf("%s contributed no operands — is it missing a case?", name)
			}
		})
	}
}

// TestEventPosHandlesEveryKind — the same census applied to the position
// reader, which every diagnostic and every mark plan consults.
//
// The default arm FAILS rather than skipping. Its first cut said
// `default: continue`, which is the very defect this file exists to remove:
// a default that quietly does nothing while looking like coverage. A review
// caught it, and it is kept as a case study rather than quietly corrected.
func TestEventPosHandlesEveryKind(t *testing.T) {
	pos := core.SrcPos{Row: 3, Col: 4}
	for kind, name := range evKinds {
		ev := wellFormedEvent(kind)
		switch kind {
		case evCall:
			ev.call.pos = pos
		case evBranch:
			ev.br.pos = pos
		case evLoop:
			ev.loop.pos = pos
		case evCallUser:
			ev.uc.pos = pos
		case evFallback:
			ev.fb.pos = pos
		case evTrap:
			ev.trap.pos = pos
		case evStore:
			ev.store.pos = pos
		case evDynBind:
			ev.dyn.pos = pos
		case evBindTwin:
			ev.twin.pos = pos
		case evBreak, evContinue:
			// Positionless BY CONSTRUCTION, and asserted rather than skipped.
			if got := eventPos(*ev); got != (core.SrcPos{}) {
				t.Errorf("eventPos(%s) = %v, want the zero position", name, got)
			}
			continue
		default:
			t.Errorf("%s is unclassified here, so this test proves nothing about it: "+
				"give it a case that stamps its payload position, or list it above as positionless", name)
			continue
		}
		if got := eventPos(*ev); got != pos {
			t.Errorf("eventPos(%s) = %v, want the payload's own %v", name, got, pos)
		}
	}
}

// wellFormedEvent builds an event of one kind with its payload present, so a
// walk over it exercises the real case rather than a nil deref.
func wellFormedEvent(kind int) *EmitEvent {
	ev := &EmitEvent{kind: kind}
	op := ConstOperand(1)
	switch kind {
	case evCall:
		ev.call = emitCall{ops: []EmitOperand{op}}
	case evBranch:
		ev.br = &emitBranch{cond: op}
	case evLoop:
		ev.loop = &emitLoop{end: op}
	case evCallUser:
		ev.uc = emitUserCall{ops: []EmitOperand{op}}
	case evFallback:
		ev.fb = emitFallback{ins: []EmitOperand{op}}
	case evTrap:
		// A trap's operands are its REMATCH window, when it has one; a plain
		// terminal trap carries none. The window is what must be visited, so
		// that is what a well-formed fixture has.
		ev.trap = EmitTrap{rematchWord: "w", rematchOps: []EmitOperand{op}, rematchNWritten: 1}
	case evStore:
		ev.store = &emitStore{src: op}
	case evDynBind:
		ev.dyn = &emitDynBind{src: op, srcSeq: -1, residentTwin: -1}
	case evBindTwin:
		ev.twin = &emitBindTwin{}
	}
	return ev
}

// siteLines renders eventKindSites in a stable order for a failure message.
func siteLines() []string {
	names := make([]string, 0, len(eventKindSites))
	for n := range eventKindSites {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, n+"  "+eventKindSites[n])
	}
	return out
}

func joinLines(ss []string) string {
	return strings.Join(ss, "\n  ")
}
