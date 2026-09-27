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
	words   []string
}

func (m *mayBeFnEmit) Active() bool           { return true }
func (m *mayBeFnEmit) MayBeFn(id string) bool { return m.maybe[id] }
func (m *mayBeFnEmit) NoteReStepLanding(v core.Value, _ core.SrcPos) {
	m.landed = append(m.landed, v.ID)
}
func (m *mayBeFnEmit) NoteLandingNext(_ core.Value, next core.LandingNext, beneath bool, word core.Value) {
	m.next = append(m.next, next)
	m.beneath = append(m.beneath, beneath)
	name := ""
	if w, err := core.AsWord(word); err == nil {
		name = w.Name
	}
	m.words = append(m.words, name)
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
// token after it leaves it unrecorded, except in a def's operand group,
// where it is a COLLECTING landing (NUR298) unless the token is a dispatch
// modifier.
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

	group := func(after core.Value) []core.Value {
		return []core.Value{core.NewWord("def"), core.NewWord("j"), core.NewOpenParen(), core.NewInteger(5), branch, after, core.NewCloseParen()}
	}
	e, rec, fin = mayBeFnEngine(t, group(core.NewInteger(7)), map[string]bool{"br-land": true})
	defer fin()
	noteReStepLanding(e, 4)
	if len(rec.landed) != 1 || len(rec.next) != 1 || rec.next[0] != core.LandingNextCollect {
		t.Errorf("in a def's group a collectable token makes a collecting landing: %v %v", rec.landed, rec.next)
	}
	e, rec, fin = mayBeFnEngine(t, group(core.NewDispatchMod(core.DispatchModInfo{Val: true})), map[string]bool{"br-land": true})
	defer fin()
	noteReStepLanding(e, 4)
	if len(rec.landed) != 0 {
		t.Errorf("a dispatch modifier after the value is data intent: %v", rec.landed)
	}
}

// TestInDefGroup pins where a collecting landing is noted (NUR298): directly
// in the paren group written as a def's operand, which runs before the def
// dispatches.
func TestInDefGroup(t *testing.T) {
	r := covRegistry(t, nil)
	v := core.NewInteger(1)
	for _, c := range []struct {
		name string
		tape []core.Value
		at   int
		want bool
	}{
		{"a def's group", []core.Value{core.NewWord("def"), core.NewWord("j"), core.NewOpenParen(), v}, 3, true},
		{"past a closed inner group", []core.Value{core.NewWord("def"), core.NewWord("j"), core.NewOpenParen(), core.NewOpenParen(), v, core.NewCloseParen(), v}, 6, true},
		{"inside an inner group", []core.Value{core.NewWord("def"), core.NewWord("j"), core.NewOpenParen(), core.NewOpenParen(), v}, 4, false},
		{"another word's group", []core.Value{core.NewWord("add"), core.NewWord("j"), core.NewOpenParen(), v}, 3, false},
		{"a value before the group", []core.Value{core.NewWord("def"), v, core.NewOpenParen(), v}, 3, false},
		{"a group at the tape's start", []core.Value{core.NewOpenParen(), v}, 1, false},
		{"no group", []core.Value{core.NewWord("def"), core.NewWord("j"), v}, 2, false},
	} {
		e := core.NewTop(r)
		e.Tape = core.NewTape(c.tape, core.StackHeadroom)
		if got := inDefGroup(e, c.at); got != c.want {
			t.Errorf("%s: inDefGroup = %v, want %v", c.name, got, c.want)
		}
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
		// The word itself rides with the note: a function word is what the
		// VM's landing walks the run-time fn's overloads over (NUR190), and a
		// collected one is what a `/q` slot captures in place of the value
		// the pass folds it to (NUR219). A boundary or the tape's end carries
		// none.
		wantWord := ""
		if tc.next == core.LandingNextWord || tc.next == core.LandingNextValue {
			w, _ := core.AsWord(tc.tape[1])
			wantWord = w.Name
		}
		if rec.words[0] != wantWord {
			t.Errorf("%s: word = %q, want %q", tc.name, rec.words[0], wantWord)
		}
	}
}
