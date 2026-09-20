package lang

import (
	"fmt"
	"testing"
)

// TestGetOverDisjunctReceiver pins the Stage-D discriminator widening: a get
// over a UNION (Disjunct) receiver — `nd: Map | Any` from an if-branch join, the
// trie/tst node-vs-none shape — fails matchSignature against any single concrete
// overload, but at run time holds ONE concrete alternative the same first-match
// the interpreter takes dispatches. checkModeAssumeSig's recovery used to fire
// only for a bare Any carrier (anyAnyCarrier); a Disjunct carrier (Parent=Disjunct,
// no payload) missed it and hit the hard decline. Now it records a
// runtime-re-matching OpCallNativePoly. Pinned compiled==interpreter across
// present-key / other-member / absent-key (the off-corpus recursive-node shape
// the langspec differential does not cover).
func TestGetOverDisjunctReceiver(t *testing.T) {
	const prelude = `def pluck fn [[nd:Any] [Any] [(nd "ch" get)]] ` +
		`def ins fn [[x:Any] [Any] [ def nd (if (x eq none) [{ch:1}] [x]) (nd pluck) ]] `
	for _, c := range []struct{ name, arg, want string }{
		{"present key over the then-arm Map", "none", "[1]"},
		{"present key over the else-arm Map", "{ch:9}", "[9]"},
		{"absent key -> None", "{other:1}", "[None]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := prelude + "(" + c.arg + " ins)"
			a, _ := New()
			got, err := a.RunCompiledStrict(src)
			if err != nil {
				t.Fatalf("get over a union receiver must compile, declined: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(src)
			if werr != nil {
				t.Fatalf("interpreter error: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}

// TestGetOverConcreteNonContainerFailsToCompile guards the widening: a get over a
// CONCRETE non-container receiver (Integer) is a genuine type error, not a union
// dispatch, and must still DECLINE strict compile (falling back to the
// interpreter, which raises) — the widening must not swallow real type errors.
func TestGetOverConcreteNonContainerFailsToCompile(t *testing.T) {
	const src = `def f fn [[x:Integer] [Any] [(x "k" get)]] (5 f)`
	a, _ := New()
	if _, err := a.RunCompiledStrict(src); err == nil {
		t.Fatal("get over a concrete Integer must decline strict compile (genuine type error), not poly-recover")
	}
	// And it must still error the same way under the interpreter / fallback.
	b, _ := New()
	if _, err := b.RunInterp(src); err == nil {
		t.Fatal("get over a concrete Integer must error under the interpreter too")
	}
}
