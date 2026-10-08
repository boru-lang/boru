package lang

import "testing"

// TestNUR323TypeValueIsNoValueOfItsParent pins NUR323's close. A type
// literal is the one value whose Parent is not its type — the Integer node's
// Parent is Number — and every check-mode widening of a value to "a carrier
// of its Parent" claimed a Number where the run holds a type: a member read
// (`sub m.e 3` over `{e: Integer}` answered [3] compiled), a fn's argument
// (`f Integer`), a branch join, a list element. The check pass now widens a
// type literal to a Type carrier (core.ValueCarrier), and the VM's re-match
// refuses a type at a value slot as the interpreter's plan does
// (rejectsTypeLiteral in positionalMatch). `is` and `typeof`, which read the
// node's own place in the lattice, still answer the interpreter's value.
func TestNUR323TypeValueIsNoValueOfItsParent(t *testing.T) {
	const noSub, noAdd = "ERROR:cannot call `sub`", "ERROR:cannot call `add`"
	for _, c := range []struct{ src, want string }{
		{`def m {e: Integer} end sub m.e 3`, noSub},
		{`def m {e: Integer} end m.e add 1`, noAdd},
		{`def m {e: String} end m.e add "x"`, noAdd},
		{`def m {e: Integer} end [m.e] get 0 add 1`, noAdd},
		{`def m {e: {a: Integer}} end sub m.e.a 3`, noSub},
		{`def m {e: Integer} end def k m.e k add 1`, noAdd},
		{`def f fn [[x:Any][Any][x add 1]] end f Integer`, noAdd},
		{`def f fn [[x:Any][Any][x add 1]] end f 5 f Integer`, noAdd},
		{`def f fn [[x:Type][Any][x add 1]] end f Integer`, noAdd},
		{`def f fn [[x:Any y:Any][Any][x add y]] end f Integer 1`, noAdd},
		{`def c true end def t (if c [Integer] [5]) t add 1`, noAdd},
		// The type's own reads keep the interpreter's answers.
		{`def m {e: Integer} end m.e is Number`, "[true]"},
		{`def m {e: Integer} end m.e is Type`, "[true]"},
		{`def m {e: Integer} end typeof m.e`, "[Number]"},
		{`def f fn [[x:Type][Any][x is Number]] end f Integer`, "[true]"},
		{`def f fn [[x:Type][Any][x]] end f Integer`, "[Integer]"},
		// A stored None reads as the None value (it crashed the check pass).
		{`def m {e: None} end m.e`, "[None]"},
		// Plain data is unchanged.
		{`def m {e: 5} end sub m.e 3`, "[-2]"},
		{`def f fn [[x:Any][Any][x add 1]] end f 5`, "[6]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A poly in a fn unit whose operand is a type at run time meets its
	// no-match — it answered the handler over the type literal ([1]), then
	// deferred loudly for want of a raise plan. The gradual operand's
	// dispatch now publishes its layout (core optimisticLayout's gradual
	// arm), so the run raises the interpreter's signature_error itself.
	for _, src := range []string{
		`def f fn [[][Any][Integer]] end (f) add 1`,
		`def f fn [[m:Map][Any][m.e add 1]] end f {e: Integer}`,
		`each ([e:Any] => [e add 1]) [Integer]`,
	} {
		agreeOnBothLanes(t, src, noAdd)
	}
}

// TestNUR311TupleStopsAtATypeOperand pins NUR311's close. The interpreter's
// no-match report walks the operands written after the word and stops at the
// first that is no concrete value — a type literal, None — then falls to the
// stack prefix beneath; the check pass held a carrier there and rendered on
// past it. The VM rebuilds the interpreter's tuple from the recorded prefix
// (DispatchSpec.Prefix, and the poly plan's StackTuple), so the reports
// agree byte for byte.
func TestNUR311TupleStopsAtATypeOperand(t *testing.T) {
	const mini = `import "boru:minilang" end `
	for _, src := range []string{
		mini + `def m {e: Function} end mini m.e 'ab'`,
		mini + `def m {e: Function} end mini (m.e) 'ab'`,
		mini + `mini (typeof 5) 'ab'`,
		`def m {e: 5} end sub m.x 3`,
		`def m {e: 5} end 4 sub m.x`,
		`def m {e: Integer} end add m.e 3`,
		`def m {e: None} end sub m.e 3`,
		`def m {e: Integer} end def f fn [[x:Integer][Any][x]] end f m.e`,
	} {
		agreeOnBothLanes(t, src, "ERROR:no signature matches")
	}
}
