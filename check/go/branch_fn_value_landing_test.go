package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// branch_fn_value_landing_test.go pins the check-side halves of NUR159
// (2026-09-23): the re-step landing's note admits a BRANCH result whose fn
// arm the merge's widened type hides (the recorder's MayBeFn), and the
// park's check-side twin records such a survivor's placement.

// mayBeFnEmit is TheInactiveEmit with Active() on, a chosen MayBeFn set and
// a record of every landing it is told about.
type mayBeFnEmit struct {
	core.EmitRecorder
	maybe   map[string]bool
	landed  []string
	next    []core.LandingNext
	beneath []bool
}

func (m *mayBeFnEmit) Active() bool           { return true }
func (m *mayBeFnEmit) MayBeFn(id string) bool { return m.maybe[id] }
func (m *mayBeFnEmit) NoteReStepLanding(v core.Value, _ core.SrcPos) {
	m.landed = append(m.landed, v.ID)
}
func (m *mayBeFnEmit) NoteLandingNext(_ core.Value, next core.LandingNext, beneath bool) {
	m.next = append(m.next, next)
	m.beneath = append(m.beneath, beneath)
}

func mayBeFnEngine(t *testing.T, tape []core.Value, maybe map[string]bool) (*core.Engine, *mayBeFnEmit, func()) {
	t.Helper()
	r := covRegistry(t, nil)
	fin := r.Check.Begin()
	rec := &mayBeFnEmit{EmitRecorder: core.TheInactiveEmit, maybe: maybe}
	r.Check.Emit = rec
	e := core.NewTop(r)
	e.Tape = core.NewTape(tape, core.StackHeadroom)
	return e, rec, fin
}

// TestNoteReStepLandingAdmitsMayBeFn: a non-callable carrier lands only when
// the recorder says its branch may have produced a fn (`if true one/v [2]`
// is 1 interpreted); the other gates still hold over it — a collectable
// token after it leaves it unrecorded.
func TestNoteReStepLandingAdmitsMayBeFn(t *testing.T) {
	branch := core.NewCarrier(core.TAny)
	branch.ID = "br-land"
	e, rec, fin := mayBeFnEngine(t, []core.Value{branch}, map[string]bool{"br-land": true})
	defer fin()
	noteReStepLanding(e, 0)
	if len(rec.landed) != 1 || rec.landed[0] != "br-land" {
		t.Errorf("a may-be-fn branch result at the tape's end lands: %v", rec.landed)
	}

	e, rec, fin = mayBeFnEngine(t, []core.Value{branch}, nil)
	defer fin()
	noteReStepLanding(e, 0)
	if len(rec.landed) != 0 {
		t.Errorf("a plain carrier the recorder does not vouch for is not callable: %v", rec.landed)
	}

	e, rec, fin = mayBeFnEngine(t, []core.Value{branch, core.NewInteger(5)}, map[string]bool{"br-land": true})
	defer fin()
	noteReStepLanding(e, 0)
	if len(rec.landed) != 0 {
		t.Errorf("a collectable token after the value leaves it unrecorded: %v", rec.landed)
	}
}

// TestParenPlacedFnCarrierAdmitsMayBeFn: a user paren collapsing to a lone
// may-be-fn branch result records the placement (`7 (if true inc/v [2])` is
// `[7 fn inc]` on both lanes); a plain carrier is not one of the shapes
// that place.
func TestParenPlacedFnCarrierAdmitsMayBeFn(t *testing.T) {
	branch := core.NewCarrier(core.TAny)
	branch.ID = "br-park"
	e, _, fin := mayBeFnEngine(t, []core.Value{branch}, map[string]bool{"br-park": true})
	defer fin()
	if !parenPlacedFnCarrier(e, 0) || !e.Registry.Check.ParenPlacedFnIDs["br-park"] {
		t.Errorf("a may-be-fn survivor is placed and recorded: %v", e.Registry.Check.ParenPlacedFnIDs)
	}
	e, _, fin = mayBeFnEngine(t, []core.Value{branch}, nil)
	defer fin()
	if parenPlacedFnCarrier(e, 0) || e.Registry.Check.ParenPlacedFnIDs["br-park"] {
		t.Errorf("a plain carrier is not placed: %v", e.Registry.Check.ParenPlacedFnIDs)
	}
}

// TestNoteReStepLandingNotesNext: beside the landing, the note says what
// follows the value — the tape's end, a word, a boundary — and whether
// values sit beneath it in its frame (NUR186: a named fn's no-match raises
// only with a candidate after it and nothing beneath).
func TestNoteReStepLandingNotesNext(t *testing.T) {
	branch := core.NewCarrier(core.TAny)
	branch.ID = "br-next"
	maybe := map[string]bool{"br-next": true}
	for _, tc := range []struct {
		name    string
		tape    []core.Value
		at      int
		next    core.LandingNext
		beneath bool
	}{
		{"the tape ends", []core.Value{branch}, 0, core.LandingNextEnd, false},
		{"a registered word follows", []core.Value{branch, core.NewWord("cadd")}, 0, core.LandingNextWord, false},
		{"a boundary follows", []core.Value{branch, core.NewCloseParen()}, 0, core.LandingNextBoundary, false},
		{"a value beneath", []core.Value{core.NewInteger(7), branch}, 1, core.LandingNextEnd, true},
		// The forward phase's word arm: a word bound to a VALUE is collected
		// (`m.f k` with `def k 2` is g over 2), a binding that dispatches
		// and a registered word stop it, a name it resolves to a literal —
		// `true`, a type name, an undefined name's atom — is collected.
		{"a value-bound word follows", []core.Value{branch, core.NewWord("k")}, 0, core.LandingNextValue, false},
		{"a fn-bound word follows", []core.Value{branch, core.NewWord("g")}, 0, core.LandingNextWord, false},
		{"a literal name follows", []core.Value{branch, core.NewWord("true")}, 0, core.LandingNextValue, false},
		{"a type name follows", []core.Value{branch, core.NewWord("Integer")}, 0, core.LandingNextValue, false},
		{"an undefined name follows", []core.Value{branch, core.NewWord("zzz-undefined")}, 0, core.LandingNextValue, false},
	} {
		e, rec, fin := mayBeFnEngine(t, tc.tape, maybe)
		e.Registry.Defs.Push("k", core.NewInteger(2))
		e.Registry.Defs.Push("g", core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{BarrierPos: 0}}}))
		e.Pointer = tc.at
		noteReStepLanding(e, tc.at)
		fin()
		if len(rec.landed) != 1 || len(rec.next) != 1 {
			t.Errorf("%s: one landing, one note, got %v %v", tc.name, rec.landed, rec.next)
			continue
		}
		if rec.next[0] != tc.next || rec.beneath[0] != tc.beneath {
			t.Errorf("%s: next = %v beneath = %v, want %v %v", tc.name, rec.next[0], rec.beneath[0], tc.next, tc.beneath)
		}
	}
}
