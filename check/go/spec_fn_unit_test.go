package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// CompileFnSigUnit's declines: a nil or check-less caller, an index out
// of range, a Go-implemented or fallback signature. The compile itself is
// pinned by the lang rows that dispatch the outer overload a conditional
// redefinition replaced (a module alias, a lambda-valued outer).
func TestCompileFnSigUnitDeclines(t *testing.T) {
	fd := core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}, {Fallback: true, Impl: core.Boru(nil)}}}
	if CompileFnSigUnit(nil, fd, 0) != -1 {
		t.Fatal("a nil caller compiles nothing")
	}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if CompileFnSigUnit(r, fd, -1) != -1 || CompileFnSigUnit(r, fd, 2) != -1 {
		t.Fatal("an index out of range compiles nothing")
	}
	if CompileFnSigUnit(r, fd, 1) != -1 {
		t.Fatal("a fallback signature compiles nothing")
	}
	native := core.FnDefInfo{Name: "n", Signatures: []core.Signature{{}}}
	if CompileFnSigUnit(r, native, 0) != -1 {
		t.Fatal("a Go-implemented signature compiles nothing")
	}
	// An inactive recorder declines the unit — for a declared sig, an
	// untyped-param sig and an anonymous fn alike.
	if CompileFnSigUnit(r, fd, 0) != -1 {
		t.Fatal("an inactive recorder compiles nothing")
	}
	untyped := core.FnDefInfo{Name: "u", Signatures: []core.Signature{{Params: []core.FnParam{{Name: "x"}}, Impl: core.Boru([]core.Value{core.NewWord("x")})}}}
	lambda := core.FnDefInfo{Name: "l", Anonymous: true, Signatures: []core.Signature{{Returns: []*core.Type{core.TAny}, Impl: core.Boru([]core.Value{core.NewInteger(1)})}}}
	if CompileFnSigUnit(r, untyped, 0) != -1 || CompileFnSigUnit(r, lambda, 0) != -1 {
		t.Fatal("an inactive recorder compiles nothing for an untyped param or a lambda either")
	}
}
