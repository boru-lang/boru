package lang

import (
	"fmt"
	"testing"
)

// placed_in_body_test.go pins the placed value inside a code body
// (2026-09-23, the interp-entry census's placed-in-body rows): a value a user
// paren PLACES inside an each / fold body — a lambda literal, a factory's
// returned closure — is data on both lanes (the park rule), so it is no
// unapplied apply and the body compiles into its unit; the native used to be
// handed the raw list and ran the whole body on the interpreter
// (closureResidualHasUnappliedFn skips a placed value).

const pibMk = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end `

var placedInBodyRows = []struct {
	label, src, want string
	native           bool
}{
	{"a factory's closure placed in an each body", pibMk + `each [(mk 2)] [1 2 3]`, "[[fn (Integer) fn (Integer) fn (Integer)]]", true},
	{"and counted", pibMk + `size (each [(mk 2)] [1 2 3])`, "[3]", true},
	{"a lambda literal placed in an each body", `each [([n:Integer] => [n mul 2])] [1 2 3]`, "[[fn (Integer) fn (Integer) fn (Integer)]]", true},
	{"a lambda literal placed in a fold body", `0 fold [([a:Integer e:Integer] => [a add e])] [1 2 3]`, "[fn (Integer, Integer)]", true},
	{"placed with a capture", `def k 5 end 0 fold [([a:Integer e:Integer] => [a add e add k])] [1 2]`, "[fn (Integer, Integer)]", true},
	{"placed over a map accumulator", `{} fold [([acc:Map e:String] => [set e (size e) acc])] ['a' 'bb']`, "[fn (Map, String)]", true},
	// The handler reads the body's TOP result: the placed value under a
	// later literal is not what each collects, on either lane.
	{"placed, then a literal", pibMk + `each [(mk 2) 9] [1 2]`, "[[9 9]]", true},
	// A placed CALL of a def-bound closure (`(f 1)`, f a factory's result)
	// is its result on both lanes, but the body's compile still declines
	// on the closure's shaped read inside the paren (a def-bound computed
	// fn's window), so the native runs it on the interpreter — open.
	{"a placed call of a def-bound closure (open)", pibMk + `def f (mk 1) end each [(f 1)] [1 2 3]`, "[[2 2 2]]", false},
	// A fn-typed CARRIER read as the body over the element — a captured
	// comparator, a def-bound computed fn (`each [a5] xs`) — is the
	// interpreter's word dispatch over the value beneath, which the closure
	// body does not lower yet (the trailing apply in a closure body is the
	// follow-on); the native runs the body on the interpreter, as before.
	{"a def-bound computed fn applied over the element (open)", pibMk + `def a5 (mk 5) end each [a5] [1 2 3]`, "[[6 7 8]]", false},
}

func TestPlacedInBodyParityAndNoEntry(t *testing.T) {
	for _, row := range placedInBodyRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if noteCompileDefect(t, row.src, gotC, errC) {
			t.Errorf("%s: must compile\n  %s", row.label, row.src)
			continue
		}
		if !compiled {
			t.Errorf("%s: must take the compiled lane\n  %s", row.label, row.src)
		}
		if errI != nil || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v interp %v/%v, want %s\n  %s", row.label, gotC, errC, gotI, errI, row.want, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}
