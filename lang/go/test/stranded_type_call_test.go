package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// §5.1 of design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore — "a capitalised name bound to
// a function never calls, silently". `def I x:Integer => [add 1 x] end I 5`
// prints `I 5` and exits 0: the capitalised name minted a TYPE, so writing
// it in call position placed the lattice node and left the 5 unconsumed.
// `stranded_type_call` is the hint that shape now costs (recommendation 2).
//
// The gate is deliberately narrow, so the negatives below are the contract:
// a hint that fires on legal type-as-data code is worse than one that misses
// a spelling. Positives and negatives are paired per lang/go/CLAUDE.md's test
// discipline.

func strandedTypeCallDiags(t *testing.T, src string) []lang.CheckDiagnostic {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check %q: %v", src, err)
	}
	var out []lang.CheckDiagnostic
	for _, d := range res.Diagnostics {
		if d.Code == "stranded_type_call" {
			out = append(out, d)
		}
	}
	return out
}

// TestStrandedTypeCallFires — every shape that IS §5.1: a fn-bodied type node
// placed where a call was written, with its operands left behind it.
func TestStrandedTypeCallFires(t *testing.T) {
	cases := []struct {
		src   string
		name  string
		unpos bool // the placement carries no source token
	}{
		// The audit's own repro: an arrow lambda under a capitalised name.
		{`def I x:Integer => [add 1 x] end I 5`, "I", false},
		// The combinator that matters most — K is 2-arg, so its minted node is
		// plainly Function-parented rather than a PREDICATE type. Keying the
		// gate on the node's fn CONTENT (not on predicate-ness) is what makes
		// the multi-argument half of the S/K/I/B/C/W/Y set visible.
		{`def K fn [[a:Any b:Any][Any][a]] end K 1 2`, "K", false},
		// The verbose fn spelling, 1-arg (this one DOES mint a predicate type).
		{`def Succ fn [[n:Integer][Integer][n add 1]] end Succ 5`, "Succ", false},
		// Inside a fn body: the same defect, the same hint. A fn body,
		// an `if` arm, a `for` body, a `do` region and a paren group all
		// surface their residual into the top-level one, so judging that
		// one list reaches every place the stranded pair can still show up.
		{`def I x:Integer => [add 1 x] end  def g y:Integer => [I y]  g 5`, "I", false},
		{`def I x:Integer => [add 1 x] end  (I 5)`, "I", false},
		{`def I x:Integer => [add 1 x] end  do [I 5]`, "I", false},
		{`def I x:Integer => [add 1 x] end  for [1 2] [I 5]`, "I", false},
		// An ALIAS writes one name for another's node: the message follows
		// what was WRITTEN (`J`), which is also where the caret lands.
		{`def I x:Integer => [add 1 x] end  def J I  J 5`, "J", false},
		// A COMPUTED placement carries no source token at all, so there the
		// node's own name is the only thing left to name it by.
		{`def I x:Integer => [add 1 x] end  (valof I) 5`, "I", true},
	}
	for _, c := range cases {
		diags := strandedTypeCallDiags(t, c.src)
		if len(diags) != 1 {
			t.Errorf("%q: want 1 stranded_type_call, got %d", c.src, len(diags))
			continue
		}
		d := diags[0]
		if d.Severity != lang.SeverityWarning {
			t.Errorf("%q: severity = %q, want warning — the program runs and exits 0, so this is a suspicion, not a guaranteed failure", c.src, d.Severity)
		}
		if d.Word != c.name {
			t.Errorf("%q: Word = %q, want %q", c.src, d.Word, c.name)
		}
		if positioned := d.Row != 0 && d.Col != 0; positioned == c.unpos {
			t.Errorf("%q: positioned=%v, want %v (row=%d col=%d)", c.src, positioned, !c.unpos, d.Row, d.Col)
		}
		// One suggestion, naming the lowercase spelling — and NO Replacement.
		// The fix is a coordinated rename (the declaration and every
		// reference); this diagnostic points at one use site, so a
		// single-token code action applied there would leave the capitalised
		// def standing and turn the call into an undefined_word.
		if len(d.Suggestions) != 1 || d.Suggestions[0].Replacement != nil {
			t.Errorf("%q: want exactly one suggestion and no mechanical replacement, got %+v", c.src, d.Suggestions)
			continue
		}
		if !strings.Contains(d.Suggestions[0].Message, strings.ToLower(c.name)) {
			t.Errorf("%q: suggestion must name the lowercase spelling %q, got %q",
				c.src, strings.ToLower(c.name), d.Suggestions[0].Message)
		}
	}
}

// TestStrandedTypeCallDedupesOneSourceDefect — one source defect costs one
// diagnostic. A fn body is analysed once per call shape, so a body that
// strands the pair surfaces the same tokens into the residual once per
// analysed call; without the dedupe the CLI and the LSP both show the
// warning twice at the identical row and column.
func TestStrandedTypeCallDedupesOneSourceDefect(t *testing.T) {
	const src = `def I x:Integer => [add 1 x] end def g y:Integer => [I y] g 1 g 2`
	if diags := strandedTypeCallDiags(t, src); len(diags) != 1 {
		t.Errorf("two calls into one defective body must yield 1 diagnostic, got %d", len(diags))
	}
}

// TestStrandedTypeCallMissesConsumedPair records the diagnostic's RECALL
// limit as a fact, not a hope: judging the finished residual cannot see a
// stranded pair that a later word or an enclosing construct consumed. Each
// row below is the §5.1 defect and each goes unreported. Pinning it here
// means the limit is visible to the next reader and any fix that closes it
// fails this test loudly instead of drifting past it — see
// design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore §5.1 "What it still misses".
func TestStrandedTypeCallMissesConsumedPair(t *testing.T) {
	for _, src := range []string{
		`def I x:Integer => [add 1 x] end I 5 drop`,        // a later word takes the operand
		`def I x:Integer => [add 1 x] end I 5 print`,       // an output word takes it
		`def I x:Integer => [add 1 x] end  def r (I 5)  r`, // def binds one of the pair
		`def I x:Integer => [add 1 x] end  size (I 5)`,     // an enclosing call consumes the node
	} {
		if diags := strandedTypeCallDiags(t, src); len(diags) != 0 {
			t.Errorf("%q: NOW REPORTED (%d) — the residual scan's recall limit is closed; "+
				"delete this row and update §5.1's \"What it still misses\"", src, len(diags))
		}
	}
}

// TestStrandedTypeCallStaysQuiet — the negatives. Each is legal code that
// leaves a type node next to a value, and each would be noise.
func TestStrandedTypeCallStaysQuiet(t *testing.T) {
	cases := []struct {
		src string
		why string
	}{
		// The DELIBERATE use of a predicate type: `is` consumes it.
		{`def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end 4 is Even`,
			"the predicate type used as one — `is` consumed the node"},
		// A statement that merely NAMES a type after an earlier one produced a
		// value: the node is last, so nothing was being called.
		{`def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end  4 is Even  Even`,
			"the node is stranded LAST — no operands follow it"},
		{`def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end  def xs [1 2 3]  xs Even`,
			"stack-form residual: the node is last, the list precedes it"},
		// Two adjacent nodes — nothing was being called.
		{`def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end def Odd fn n:Integer Boolean [eq 1 (mod 2 n)] end Even Odd`,
			"two type nodes side by side"},
		// A node whose content is NOT a function: an alias, a builtin, a
		// signature type, a class. Naming one beside a value is ordinary.
		{`def Foo Integer  Foo 5`, "an alias to a plain type"},
		{`Integer 5`, "a builtin type name beside a value"},
		{`def F fnsig [[Integer][Integer]]  F 5`, "a fnsig type is a SIGNATURE, not a function"},
		{`def C class {a:Integer}  C 5`, "a class node is constructed with make, not called"},
		// Types as DATA inside a container: boru carries types first-class, so
		// a node beside a value in a list or map is not a failed call.
		{`def I x:Integer => [add 1 x] end  [I 5]`, "a list literal holding a type and a value"},
		{`def I x:Integer => [add 1 x] end  {a:I b:5}`, "a map holding a type and a value"},
		// The fix itself: the lowercase name binds a callable function.
		{`def i x:Integer => [add 1 x] end i 5`, "the lowercase spelling calls"},
	}
	for _, c := range cases {
		if diags := strandedTypeCallDiags(t, c.src); len(diags) != 0 {
			t.Errorf("%q: want no stranded_type_call (%s), got %d: %s",
				c.src, c.why, len(diags), diags[0].Detail)
		}
	}
}
