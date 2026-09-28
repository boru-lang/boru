package basic

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// consumingSig is a user extension that binds its arguments and does
// nothing else: named params, no body, no declared return.
func consumingSig(barrier int, names ...string) Signature {
	ps := make([]FnParam, len(names))
	for i, n := range names {
		ps[i] = FnParam{Name: n, Type: TInteger}
	}
	return Signature{Params: ps, Impl: &core.BoruImpl{}, Returns: []*Type{}, BarrierPos: barrier}
}

// TestShuffleDispatchSafe pins the do-body raise-free screen's dispatch
// half: a stack shuffle's dispatch must hold exactly one registered all-Any
// stack-only native (the total fallback), and any other overload must be a
// user extension that only consumes its arguments with the shuffle's effect.
func TestShuffleDispatchSafe(t *testing.T) {
	drop := shuffleEffect["drop"]
	native := Signature{Args: []*Type{TAny}, Impl: Go(dropHandler), Returns: []*Type{}, BarrierPos: 0}
	fd := func(sigs ...Signature) *FnDefInfo { return &FnDefInfo{Name: "drop", Signatures: sigs} }

	if !shuffleDispatchSafe(fd(native), drop, false) {
		t.Error("the lone all-Any native is safe")
	}
	if !shuffleDispatchSafe(fd(consumingSig(0, "x"), native), drop, false) {
		t.Error("a stack-only consuming extension beside the native is safe")
	}
	if !shuffleDispatchSafe(fd(consumingSig(1, "x"), native), drop, true) {
		t.Error("a forward-collecting consuming extension is safe as the body's last token")
	}
	macro := fd(native)
	macro.Macro = true
	stackNative := native
	stackNative.BarrierPos = 1
	wide := native
	wide.Args = []*Type{TAny, TAny}
	narrow := native
	narrow.Args = []*Type{TString}
	for _, c := range []struct {
		fd   *FnDefInfo
		last bool
		what string
	}{
		{nil, true, "an unbound word"},
		{macro, true, "a macro"},
		{fd(stackNative), true, "a forward-collecting native"},
		{fd(wide), true, "a native of another arity"},
		{fd(narrow), true, "a narrowed native"},
		{fd(native, native), true, "two natives"},
		{fd(consumingSig(0, "x")), true, "no native fallback"},
		{fd(consumingSig(1, "x"), native), false, "a forward-collecting extension with tokens after it"},
	} {
		if shuffleDispatchSafe(c.fd, drop, c.last) {
			t.Errorf("%s must not be proven raise-free", c.what)
		}
	}
}

// TestConsumingNoOpSig pins the extension half: every way a user overload
// can raise or leave a value refuses it.
func TestConsumingNoOpSig(t *testing.T) {
	drop, dup := shuffleEffect["drop"], shuffleEffect["dup"]
	good := consumingSig(0, "x")
	if !consumingNoOpSig(&good, drop, false) {
		t.Fatal("a named, bodiless, return-less one-param overload consumes like drop")
	}
	mut := func(f func(s *Signature)) *Signature {
		s := consumingSig(0, "x")
		f(&s)
		return &s
	}
	for _, c := range []struct {
		sig  *Signature
		eff  [2]int
		what string
	}{
		{mut(func(s *Signature) { s.Impl = Go(dropHandler) }), drop, "a Go handler"},
		{mut(func(s *Signature) { s.Fallback = true }), drop, "a synthetic fallback"},
		{mut(func(s *Signature) { s.Impl = &core.BoruImpl{Body: []Value{NewWord("x")}} }), drop, "a body"},
		{mut(func(s *Signature) { s.Returns = []*Type{TInteger} }), drop, "a declared return"},
		{mut(func(s *Signature) {}), dup, "a shuffle that leaves values"},
		{mut(func(s *Signature) { s.Params = append(s.Params, FnParam{Name: "y", Type: TInteger}) }), drop, "another arity"},
		{mut(func(s *Signature) { s.BarrierPos = 1 }), drop, "forward collection past its position"},
		{mut(func(s *Signature) { s.Params[0].Name = "" }), drop, "an unnamed param"},
		{mut(func(s *Signature) { s.Params[0].Optional = true }), drop, "an optional param"},
		{mut(func(s *Signature) { s.Params[0].Quote = true }), drop, "a /q param"},
	} {
		if consumingNoOpSig(c.sig, c.eff, false) {
			t.Errorf("%s must not count as a consuming no-op", c.what)
		}
	}
}

// TestShuffleOnlyBodyOverAConsumingExtension runs the whole screen: `[1
// drop]` stays raise-free with a consuming extension registered beside the
// native, and `[drop]` still takes a value the body never pushed.
func TestShuffleOnlyBodyOverAConsumingExtension(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("drop", Signature{Args: []*Type{TAny}, Impl: Go(dropHandler), Returns: []*Type{}, BarrierPos: 0},
		consumingSig(1, "x"))
	if fd := r.Lookup("drop"); fd == nil || len(fd.Signatures) != 2 {
		t.Fatalf("want a two-signature drop, got %+v", fd)
	}
	if !shuffleOnlyBody([]Value{NewInteger(1), NewWord("drop")}, r) {
		t.Error("[1 drop] over a consuming extension is shuffle-only")
	}
	if shuffleOnlyBody([]Value{NewWord("drop")}, r) {
		t.Error("[drop] takes a value the body never pushed")
	}
	if shuffleOnlyBody([]Value{NewInteger(1), NewInteger(2), NewWord("drop"), NewWord("drop")}, r) {
		t.Error("a forward-collecting extension before another token is not proven")
	}
	if shuffleOnlyBody([]Value{NewInteger(1), NewWord("add")}, r) {
		t.Error("a word outside the shuffle set is not proven")
	}
}
