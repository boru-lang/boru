package native

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The entry rule at the handlers (design/IMMUTABLE-DEF.1.md §5 phase 1, the
// callback protocol): a Function callback over a map is handed the entry's
// VALUE, or the whole entry as a KeyVal {k v i n} when its entry param — the
// last one — is typed KeyVal; filter's Function form over a list is handed
// the ELEMENT. Positive and negative shapes side by side, at the seams the
// words share (newMapBody / mapBody.entry, filterHandler).

func TestEntryRuleMapBody(t *testing.T) {
	r := b2Reg(t)
	for _, c := range []struct {
		src    string
		keyVal bool
	}{
		{"([v:Integer] => [v])", false},
		{"([v:Any] => [v])", false},
		{"([m:Map] => [m])", false},
		{"([kv:KeyVal] => [kv.v])", true},
		{"([a:Integer kv:KeyVal] => [a])", true},
		{"([kv:KeyVal a:Integer] => [a])", false},
	} {
		mb, err := newMapBody(r, b2Lambda(t, r, c.src), "each")
		if err != nil {
			t.Fatal(err)
		}
		if mb.keyVal != c.keyVal {
			t.Errorf("%s: keyVal = %v, want %v", c.src, mb.keyVal, c.keyVal)
		}
		want := TInteger
		if c.keyVal {
			want = TKeyVal
		}
		if e := mb.entry("k", NewInteger(7), 1, 3); !e.Parent.Equal(want) {
			t.Errorf("%s: entry = %v, want a %s", c.src, e, want.Name())
		}
	}
	// A quotation never asks for the entry.
	mb, err := newMapBody(r, NewList([]Value{NewWord("dup")}), "each")
	if err != nil || mb.keyVal {
		t.Errorf("a quotation body asked for the entry: %+v / %v", mb, err)
	}
}

func TestEntryRuleFilterFunctionForm(t *testing.T) {
	r := b2Reg(t)
	for _, c := range []struct{ src, want string }{
		{"filter ([p:Integer] => [p gt 1]) [1 2 3]", "[2 3]"},
		{"filter ([p:Any] => [p gt 1]) [1 2 3]", "[2 3]"},
		{"filter ([v:Integer] => [v gt 1]) {a:1 b:2}", "{b:2}"},
		{"filter ([kv:KeyVal] => [kv.k eq 'a']) {a:1 b:2}", "{a:1}"},
		{"filter ([kv:KeyVal] => [kv.i eq 1]) {a:1 b:2}", "{b:2}"},
		{"def ok fn [[p:Integer][Boolean][p gt 1]] filter ok/v [1 2 3]", "[2 3]"},
	} {
		out, err := b2Run(r, c.src)
		if err != nil || len(out) != 1 || out[0].String() != c.want {
			t.Errorf("%s = %v / %v, want %s", c.src, out, err, c.want)
		}
	}
	for _, c := range []struct{ src, code, detail string }{
		{"filter ([kv:KeyVal] => [true]) [1 2]", "signature_error", "element 0: "},
		{"filter ([p:String] => [true]) {a:1}", "signature_error", "key \"a\": "},
		{"filter ([p:Any] => [1]) [1 2]", "filter_error", "element 0: predicate must produce a Boolean, got Integer"},
		{"filter ([v:Integer] => [v add 1]) {a:1}", "filter_error", "key \"a\": predicate must produce a Boolean, got Integer"},
		{"filter ([p:Any] => [true]) 5", "filter_error", "expects a concrete list or map"},
		{"filter ([p:Any] => [true]) Map", "filter_error", "expects a concrete list or map"},
	} {
		_, err := b2Run(r, c.src)
		if err == nil || !strings.Contains(err.Error(), c.code) || !strings.Contains(err.Error(), c.detail) {
			t.Errorf("%s: err = %v, want %s / %q", c.src, err, c.code, c.detail)
		}
	}
}

// A compiled closure met outside a VM run (no Invoker) is not bridged to a
// signature: it is handed the element unmatched and runs through the token
// seam, where it is data — a non-Boolean verdict, never a panic.
func TestEntryRuleClosureOutsideVM(t *testing.T) {
	r := b2Reg(t)
	prog := &compiler.Program{Fns: []compiler.CompiledFn{{
		Name: "filter$body", NParams: 1, NArgs: 1, NLocals: 1, Params: []*core.Type{core.TInteger},
		Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}},
	}}}
	cl := compiler.NewClosure(prog, 0, nil)
	fc := newFilterCallback(r, cl)
	if fc.sigFn.Data != nil || fc.keyVal {
		t.Fatalf("outside a VM run a closure is not bridged: %+v", fc)
	}
	if _, err := fc.keep(r, []Value{NewInteger(1)}, "element 0"); err == nil || !strings.Contains(err.Error(), "element 0") {
		t.Errorf("the unmatched closure's verdict: err = %v", err)
	}
	mb, err := newMapBody(r, cl, "each")
	if err != nil || mb.sigFn.Data != nil || mb.keyVal || !mb.closure {
		t.Errorf("newMapBody over an unbridged closure: %+v / %v", mb, err)
	}
}
