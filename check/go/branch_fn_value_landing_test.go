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
	maybe  map[string]bool
	landed []string
}

func (m *mayBeFnEmit) Active() bool           { return true }
func (m *mayBeFnEmit) MayBeFn(id string) bool { return m.maybe[id] }
func (m *mayBeFnEmit) NoteReStepLanding(v core.Value, _ core.SrcPos) {
	m.landed = append(m.landed, v.ID)
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
