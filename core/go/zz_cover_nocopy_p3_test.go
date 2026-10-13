package core

import (
	"strings"
	"testing"
)

// The no-copy phase 3 arms core's own suite reaches (cover-gate-core): each
// is reached across the repository by the other modules' suites, and is
// pinned here on core's side of the seam.

// p3Cloner is a host body that can clone itself (DeepCloner).
type p3Cloner struct{}

func (p3Cloner) DeepClone() any { return p3Cloner{} }

// TestP3IsMutablePayload: the containers a literal's evaluation constructs
// afresh, a host payload only when it can clone itself, and never a scalar.
func TestP3IsMutablePayload(t *testing.T) {
	for _, p := range []Payload{ListPayload{}, MapPayload{}, &FlexListData{}, ErrorInfo{}} {
		if !IsMutablePayload(p) {
			t.Errorf("%T is a mutable container's payload", p)
		}
	}
	if !IsMutablePayload(ExtensionPayload{Body: p3Cloner{}}) {
		t.Error("a host body that clones itself is mutable")
	}
	if IsMutablePayload(ExtensionPayload{Body: 7}) {
		t.Error("a host body that cannot clone itself is shared")
	}
	if IsMutablePayload(IntPayload{N: 1}) {
		t.Error("a scalar is shared")
	}
}

// TestP3OccurrenceIdentity: a holder's identity is its own — a second
// holder clears it without touching the first — and a type node's copy can
// clear the node's own identity and take it back.
func TestP3OccurrenceIdentity(t *testing.T) {
	v := NewInteger(1)
	SetID(&v, "a")
	w := v
	SetID(&w, "")
	if IDOf(&w) != "" || IDOf(&v) != "a" {
		t.Errorf("clearing one holder's identity: %q / %q", IDOf(&w), IDOf(&v))
	}
	lit := NewTypeLiteral(TInteger)
	own := IDOf(&lit)
	if own == "" {
		t.Fatal("a type node's copy answers the node's identity")
	}
	SetID(&lit, "")
	if IDOf(&lit) != "" {
		t.Errorf("a cleared type literal still answers %q", IDOf(&lit))
	}
	SetID(&lit, own)
	if IDOf(&lit) != own {
		t.Errorf("the node's own identity back: %q, want %q", IDOf(&lit), own)
	}
	if node := NewTypeLiteral(TInteger); IDOf(&node) != own {
		t.Error("clearing a copy reached the node")
	}
}

// TestP3FnNames: a binding names its holder (SetFnName), never the payload;
// a holder no binding named answers the payload's name — a definition's, a
// compiled closure's — and a value that is no fn answers none.
func TestP3FnNames(t *testing.T) {
	raw := Value{Parent: TFunction, Data: &FnDefInfo{Name: "f"}}
	if HolderFnName(&raw) != "" || FnNameOf(&raw) != "f" || !NamedFnValue(&raw) {
		t.Errorf("an unnamed holder: holder %q, name %q, named %v", HolderFnName(&raw), FnNameOf(&raw), NamedFnValue(&raw))
	}
	held := raw
	SetFnName(&held, "g")
	SetFnName(&held, "g") // the same name again changes nothing
	if FnNameOf(&held) != "g" || HolderFnName(&held) != "g" || !NamedFnValue(&held) {
		t.Errorf("a named holder: %q / %q", FnNameOf(&held), HolderFnName(&held))
	}
	if FnNameOf(&raw) != "f" || held.Data.(*FnDefInfo).Name != "f" {
		t.Error("naming one holder reached the other holder or the payload")
	}
	anon := Value{Parent: TFunction, Data: &FnDefInfo{Anonymous: true}}
	if NamedFnValue(&anon) {
		t.Error("an anonymous fn under no binding's name is not a named definition")
	}
	cl := Value{Parent: TFunction, Data: ClosurePayload{RetName: "c"}}
	if FnNameOf(&cl) != "c" {
		t.Errorf("a compiled closure answers its unit's name: %q", FnNameOf(&cl))
	}
	n := NewInteger(1)
	if FnNameOf(&n) != "" || NamedFnValue(&n) {
		t.Error("a value that is no fn has no name")
	}
	if _, ok := FnDefCopy(IntPayload{N: 1}); ok {
		t.Error("FnDefCopy over a payload that is no fn definition")
	}
}

// TestP3FnDefAtPointerView: a holder a binding named dispatches through a
// view under that name; the payload keeps its own and is handed out as it
// is to a holder no binding renamed.
func TestP3FnDefAtPointerView(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	fd := &FnDefInfo{Name: "f"}
	plain := Value{Parent: TFunction, Data: fd}
	if _, got, ok := e.fnDefAtPointer(plain); !ok || got != fd {
		t.Errorf("an unrenamed holder reads the payload itself: %v %v", got, ok)
	}
	held := plain
	SetFnName(&held, "g")
	_, view, ok := e.fnDefAtPointer(held)
	if !ok || view == fd || view.Name != "g" || fd.Name != "f" {
		t.Errorf("a renamed holder's view: %+v (payload %q)", view, fd.Name)
	}
}

// p3Render is a compiled runtime that renders a closure under a holder's
// name.
type p3Render struct{ noCompiledRuntime }

func (p3Render) ClosureRender(_ *ClosurePayload, name string) (string, bool) {
	return "rendered " + name, true
}

// TestP3ClosureRender: a closure a binding named renders through the
// compiled runtime under that name; under the default runtime, which
// declines, its own rendering stands, and a closure with none renders by
// its parts.
func TestP3ClosureRender(t *testing.T) {
	cl := Value{Parent: TFunction, Data: ClosurePayload{Render: "<src>"}}
	SetFnName(&cl, "h")
	prev := InstallCompiledRuntime(p3Render{})
	got := cl.String()
	InstallCompiledRuntime(noCompiledRuntime{})
	declined := cl.String()
	bare := Value{Parent: TFunction, Data: ClosurePayload{}}
	parts := bare.String()
	InstallCompiledRuntime(prev)
	if got != "rendered h" {
		t.Errorf("the runtime's rendering under the holder's name: %q", got)
	}
	if declined != "<src>" {
		t.Errorf("the closure's own rendering when the runtime declines: %q", declined)
	}
	if parts == "" || parts == "<src>" {
		t.Errorf("a closure with no rendering renders by its parts: %q", parts)
	}
}

// TestP3NamedSurfaceInfo: an installed surface's conformance set is its own,
// seeded from the declaration's, which is never written.
func TestP3NamedSurfaceInfo(t *testing.T) {
	decl := &SurfaceInfo{Name: "S", Conform: map[string]bool{"T1": true}}
	got := namedSurfaceInfo(decl, "m.S", nil)
	if got.Name != "m.S" || !got.Conform["T1"] {
		t.Fatalf("the installed surface: %+v", got)
	}
	got.Conform["T2"] = true
	if decl.Conform["T2"] || decl.Name != "S" {
		t.Error("the installed surface wrote its declaration")
	}
}

// TestP3AtomReferentsIn: an atom takes what its name is bound to in a run
// of the tape's own; an unbound name takes nothing; a nested list whose atom
// takes one is a new list; and the program's arrays are never written.
func TestP3AtomReferentsIn(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("a", NewInteger(7))
	inner := NewList([]Value{NewAtom("a")})
	elems := []Value{NewAtom("a"), NewAtom("p3unbound"), inner, NewInteger(1)}
	out := atomReferentsIn(r, elems)
	if len(out) != len(elems) {
		t.Fatalf("a run with a bound atom is rebuilt: %v", out)
	}
	if ref, ok := AtomReferent(out[0]); !ok || ref.String() != "7" {
		t.Errorf("the bound atom's referent: %v %v", ref, ok)
	}
	if _, ok := AtomReferent(out[1]); ok {
		t.Error("an unbound name takes no referent")
	}
	nl, err := AsList(out[2])
	if err != nil || nl.Len() != 1 {
		t.Fatalf("the nested list: %v", out[2])
	}
	if ref, ok := AtomReferent(*nl.Ref(0)); !ok || ref.String() != "7" {
		t.Errorf("the nested atom's referent: %v %v", ref, ok)
	}
	if _, ok := AtomReferent(elems[0]); ok {
		t.Error("the program's own atom was written")
	}
	il, _ := AsList(inner)
	if _, ok := AtomReferent(*il.Ref(0)); ok {
		t.Error("the program's nested array was written")
	}
	if atomReferentsIn(r, []Value{NewAtom("p3unbound"), NewInteger(2)}) != nil {
		t.Error("a run no atom of which is bound keeps its own array")
	}
}

// TestP3RunPartsUnderAnalysis: the analysis pass strips the whole program
// to carriers, a new run of its own, so a prefix and a body are joined for
// it.
func TestP3RunPartsUnderAnalysis(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	prefix := []Value{NewInteger(1)}
	body := []Value{NewInteger(2)}
	out, err := NewTop(r).RunParts(prefix, body)
	if err != nil || len(out) != 2 {
		t.Fatalf("RunParts under analysis: %v %v", out, err)
	}
	if renderAll(prefix) != "1" || renderAll(body) != "2" {
		t.Error("the parts were written")
	}
}

// TestP3RunSealedGroupOnSubEngine: an engine with no tape to seal a region
// on runs a paren group's items on a container sub-engine, in its paren
// markers.
func TestP3RunSealedGroupOnSubEngine(t *testing.T) {
	r := covRegistry(t, nil)
	e := New(r)
	if e.sealsLiterals() {
		t.Fatal("an engine with no tape does not seal")
	}
	out, err := e.runSealed([]Value{NewInteger(1), NewInteger(2)}, "paren", false, true)
	if err != nil || renderAll(out) != "1 | 2" {
		t.Errorf("the group's run: %v %v", out, err)
	}
}

// TestP3AutoEvalResidualKeeps: the residual sweep's one-value form leaves a
// quoted list as it is, resolves an inert typed shape in its holder, and
// passes any other value through.
func TestP3AutoEvalResidualKeeps(t *testing.T) {
	r := covRegistry(t, nil)
	e := s5aEng(t, r, nil, 0)
	q := NewEvalList([]Value{NewInteger(1)})
	SetQuoted(&q, true)
	if err := e.autoEvalResidual(&q); err != nil || !IsQuoted(&q) || q.String() != "[1]" {
		t.Errorf("a quoted list stays as it is: %v %v", q, err)
	}
	tl := NewTypedList(NewWord("Integer"))
	if err := e.autoEvalResidual(&tl); err != nil {
		t.Fatal(err)
	}
	if ci, _ := AsChildType(tl); IsWord(&ci.Child) {
		t.Error("the typed shape was not resolved in its holder")
	}
	n := NewInteger(3)
	if err := e.autoEvalResidual(&n); err != nil || n.String() != "3" {
		t.Errorf("a scalar passes through: %v %v", n, err)
	}
}

// p3ListEmit is an armed recorder that records a list member's assembly
// and stamps the holder it hands back.
type p3ListEmit struct {
	EmitRecorder
	recorded int
}

func (s *p3ListEmit) Active() bool { return true }
func (s *p3ListEmit) Armed() bool  { return true }
func (s *p3ListEmit) RecordMakeListInner(_ *Registry, _ []Value, out *Value, _ SrcPos) bool {
	s.recorded++
	SetID(out, "L_p3member")
	return true
}

// TestP3MapMemberListRecorded: a consumed map's list-valued member that a
// recording pass assembles is stored as the holder the record stamped (the
// member's own evaluation records its list too, so the stub counts both).
func TestP3MapMemberListRecorded(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	stub := &p3ListEmit{EmitRecorder: TheInactiveEmit}
	saved := r.Check.Emit
	r.Check.Emit = stub
	defer func() { r.Check.Emit = saved }()
	om := NewOrderedMap()
	om.Set("n", NewEvalList([]Value{NewDynamicCarrier(TAny)}))
	got, err := NewTop(r).AutoEvalMap(NewMap(om), false, true)
	if err != nil {
		t.Fatal(err)
	}
	m, merr := AsMap(got)
	if merr != nil {
		t.Fatalf("the evaluated map: %v", got)
	}
	member, _ := m.Get("n")
	if stub.recorded == 0 || IDOf(&member) != "L_p3member" {
		t.Errorf("the member is the record's holder: recorded %d, id %q (%s)", stub.recorded, IDOf(&member), strings.TrimSpace(member.String()))
	}
}
