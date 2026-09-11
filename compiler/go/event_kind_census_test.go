package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// event_kind_census_test.go is the structural guard NUR137 earned.
//
// Twice now a widening has admitted a new EVENT KIND to a producer set
// without teaching the predicates keyed on kind about it, and both times the
// gap was silent rather than loud:
//
//   - NUR133: `regionReadsTheStack` did not read a user call's or a
//     fallback's own operands, so a mark opened above a value the op popped.
//   - NUR137: the same function's DEFAULT read `ev.call.ops` for a branch,
//     whose operands live in `ev.br` — a zero-value payload answering a
//     soundness question, wrongly, for four increments, on the default lane.
//
// A comment did not hold that line: NUR133's fix left one in the very
// function that then broke, saying exactly what would go wrong. What holds a
// line is a test that FAILS when the premise changes. So this file asserts
// the kind SET, and names — in the failure message — every site that must
// learn about a new member. A new kind cannot be added without reading that
// list.
//
// Keep the list below in sync with the sites, not with the constants: if a
// kind-keyed switch is added or deleted anywhere in this package, edit
// kindKeyedSites here in the same commit.
var kindKeyedSites = []string{
	"emit.go regionReadsTheStack   — does the region's event pop something already on the stack? (default: reads the stack)",
	"emit.go eventPos              — the event's source position",
	"emit.go eventDivergesDeep     — does this event never return past itself?",
	"emit.go eventsBindDynScope    — does this event bind a registry-visible name?",
	"lower.go forEachOperand       — every enclosing-scope operand the event references",
	"lower.go lowerEvent           — the emission itself (default: refuses, \"unknown event kind\")",
	"lower.go singleOutputCall     — is this a promotable single-result call?",
	"lower.go collectPromotableEvents / the promotion walks (default: skip)",
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
// whole point: the failure is the prompt to walk kindKeyedSites and decide,
// per site, what the new kind means there — instead of inheriting whatever
// its default happens to do.
func TestEventKindCensus(t *testing.T) {
	// evKindEnd is one past the last kind, so this count changes the moment a
	// kind is added above it — which is the whole mechanism.
	if got, want := evKindEnd-1, len(evKinds); got != want {
		t.Fatalf("event kinds changed: highest constant %d, census has %d entries.\n\n"+
			"Add it to evKinds here, then teach each of these sites — every one is keyed on\n"+
			"ev.kind, and a default that merely 'works' is how NUR133 and NUR137 both shipped:\n  %s",
			got, want, joinLines(kindKeyedSites))
	}
	for k, name := range evKinds {
		if k < 1 || k >= evKindEnd {
			t.Errorf("%s = %d is outside the iota range 1..%d", name, k, evKindEnd-1)
		}
	}
}

// TestForEachOperandHandlesEveryKind walks a well-formed event of EVERY kind
// through the operand walker. Its `default` arm contributes nothing and used
// to dereference ev.br, so a kind added without a case would either drop
// operands (a live producer marked dead, and its value dropped) or panic —
// and ADR-005 forbids the second outright.
func TestForEachOperandHandlesEveryKind(t *testing.T) {
	for kind, name := range evKinds {
		t.Run(name, func(t *testing.T) {
			ev := wellFormedEvent(kind)
			seen := 0
			forEachOperand(ev, func(EmitOperand) { seen++ })
			// evBindTwin marks a position and references nothing, and the
			// two flow terminators carry no operands either. Every OTHER
			// kind has at least one operand slot, so visiting none means a
			// missing case — which is exactly what this catches.
			if kind != evBindTwin && kind != evBreak && kind != evContinue && seen == 0 {
				t.Errorf("%s contributed no operands — is it missing a case?", name)
			}
		})
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

// TestEventPosHandlesEveryKind — the same census applied to the position
// reader, which every diagnostic and every mark plan consults. A kind it does
// not name silently reports 0:0, and NUR130 already records what a wrong
// position costs a user.
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
		default:
			continue // break / continue carry no position, by construction
		}
		if got := eventPos(*ev); got != pos {
			t.Errorf("eventPos(%s) = %v, want the payload's own %v", name, got, pos)
		}
	}
}

func joinLines(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "\n  "
		}
		out += s
	}
	return out
}
