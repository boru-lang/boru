package calc

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// newCalc constructs a Calc with a buffer-backed output writer so
// tests can assert on what print / show wrote.
func newCalc(t *testing.T) (*Calc, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	c, err := New(buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, buf
}

// asInt extracts an int64 from a single-element stack, failing the
// test with an informative message if the shape is wrong.
func asInt(t *testing.T, stk []core.Value, label string) int64 {
	t.Helper()
	if len(stk) != 1 {
		t.Fatalf("%s: want 1 result, got %d (%v)", label, len(stk), stk)
	}
	n, err := core.AsInteger(stk[0])
	if err != nil {
		t.Fatalf("%s: AsInteger: %v (value=%s)", label, err, stk[0].String())
	}
	return n
}

// asDec extracts a float64 from a single-element stack.
func asDec(t *testing.T, stk []core.Value, label string) float64 {
	t.Helper()
	if len(stk) != 1 {
		t.Fatalf("%s: want 1 result, got %d (%v)", label, len(stk), stk)
	}
	f, err := core.AsNumber(stk[0])
	if err != nil {
		t.Fatalf("%s: AsNumber: %v", label, err)
	}
	return f
}

// --- arithmetic ----------------------------------------------------

func TestArithBasic(t *testing.T) {
	// boru convention (see lang/CLAUDE.md "Non-commutative two-arg sanity
	// check"): binary handlers compute args[1] op args[0]. So `10 sub 3`
	// reads naturally as "10 minus 3 = 7"; the prefix form `sub 10 3`
	// inverts to args=[10,3] → 3-10 = -7. Commutative ops behave the
	// same either way.
	cases := []struct {
		src  string
		want int64
	}{
		{"add 2 3", 5},     // commutative
		{"10 sub 3", 7},    // infix form: 10 - 3
		{"10 3 sub", 7},    // RPN form: top=3, next=10, compute 10-3
		{"mul 4 5", 20},    // commutative
		{"10 mod 3", 1},    // infix form: 10 mod 3
		{"2 pow 10", 1024}, // infix form: 2^10
		{"neg 5", -5},
		{"neg 7 abs", 7},
		// Forward + stack mixing — parens are required where a word
		// would otherwise be a forward-collection boundary.
		{"2 3 add", 5},
		{"add 1 (add 2 (add 3 4))", 10},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			c, _ := newCalc(t)
			stk, err := c.Eval(tc.src)
			if err != nil {
				t.Fatalf("Eval(%q): %v", tc.src, err)
			}
			got := asInt(t, stk, tc.src)
			if got != tc.want {
				t.Errorf("Eval(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

func TestArithFloats(t *testing.T) {
	c, _ := newCalc(t)
	stk, err := c.Eval("add 1.5 2.5")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got := asDec(t, stk, "add 1.5 2.5"); got != 4.0 {
		t.Errorf("want 4.0, got %v", got)
	}

	// div always produces Float, even on integer inputs. Infix `1 div 2`
	// reads naturally as 1/2 = 0.5.
	c, _ = newCalc(t)
	stk, err = c.Eval("1 div 2")
	if err != nil {
		t.Fatal(err)
	}
	if !core.ParentOf(&stk[0]).ConformsTo(core.TFloat) {
		t.Errorf("1 div 2: want Float result, got %s", core.ParentOf(&stk[0]).String())
	}
	if got := asDec(t, stk, "1 div 2"); got != 0.5 {
		t.Errorf("1 div 2 = %v, want 0.5", got)
	}
}

func TestArithErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"1 div 0", "division by zero"},
		{"1 mod 0", "modulo by zero"},
		{"sqrt (neg 1)", "negative input"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			c, _ := newCalc(t)
			_, err := c.Eval(tc.src)
			if err == nil {
				t.Fatalf("Eval(%q) want error, got nil", tc.src)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Eval(%q) error = %v, want substring %q", tc.src, err, tc.want)
			}
		})
	}
}

// --- constants -----------------------------------------------------

func TestConstants(t *testing.T) {
	c, _ := newCalc(t)
	stk, err := c.Eval("pi")
	if err != nil {
		t.Fatal(err)
	}
	if got := asDec(t, stk, "pi"); math.Abs(got-math.Pi) > 1e-12 {
		t.Errorf("pi = %v, want %v", got, math.Pi)
	}

	c, _ = newCalc(t)
	stk, err = c.Eval("e")
	if err != nil {
		t.Fatal(err)
	}
	if got := asDec(t, stk, "e"); math.Abs(got-math.E) > 1e-12 {
		t.Errorf("e = %v, want %v", got, math.E)
	}
}

func TestPiTimesTwo(t *testing.T) {
	c, _ := newCalc(t)
	stk, err := c.Eval("2 pi mul")
	if err != nil {
		t.Fatal(err)
	}
	got := asDec(t, stk, "2 pi mul")
	if math.Abs(got-2*math.Pi) > 1e-12 {
		t.Errorf("2*pi = %v, want %v", got, 2*math.Pi)
	}
}

// --- stack persistence + ops ---------------------------------------

func TestStackPersistsAcrossEval(t *testing.T) {
	c, _ := newCalc(t)
	if _, err := c.Eval("1 2"); err != nil {
		t.Fatal(err)
	}
	stk, err := c.Eval("add")
	if err != nil {
		t.Fatal(err)
	}
	if got := asInt(t, stk, "1 2 then add"); got != 3 {
		t.Errorf("stack-carry add = %d, want 3", got)
	}
}

func TestStackOps(t *testing.T) {
	c, _ := newCalc(t)
	stk, err := c.Eval("1 2 3 dup")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 4 {
		t.Fatalf("dup: want 4 items, got %d", len(stk))
	}
	n3, _ := core.AsInteger(stk[3])
	if n3 != 3 {
		t.Errorf("dup: top = %d, want 3", n3)
	}

	c, _ = newCalc(t)
	stk, err = c.Eval("1 2 swap")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 2 {
		t.Fatalf("swap: want 2 items, got %d", len(stk))
	}
	a, _ := core.AsInteger(stk[0])
	b, _ := core.AsInteger(stk[1])
	if a != 2 || b != 1 {
		t.Errorf("swap: got [%d %d], want [2 1]", a, b)
	}

	c, _ = newCalc(t)
	stk, err = c.Eval("7 drop")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 0 {
		t.Errorf("drop: want empty stack, got %v", stk)
	}

	c, _ = newCalc(t)
	stk, err = c.Eval("1 2 3 over")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 4 {
		t.Fatalf("over: want 4 items, got %d", len(stk))
	}
	top, _ := core.AsInteger(stk[3])
	if top != 2 {
		t.Errorf("over: top = %d, want 2", top)
	}

	c, _ = newCalc(t)
	stk, err = c.Eval("1 2 3 4 clear")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 0 {
		t.Errorf("clear: want empty stack, got %v", stk)
	}
}

func TestDepth(t *testing.T) {
	c, _ := newCalc(t)
	stk, err := c.Eval("1 2 3 depth")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 4 {
		t.Fatalf("depth: want 4 items, got %d", len(stk))
	}
	n, _ := core.AsInteger(stk[3])
	if n != 3 {
		t.Errorf("depth: top = %d, want 3", n)
	}
}

func TestStackUnderflow(t *testing.T) {
	c, _ := newCalc(t)
	_, err := c.Eval("dup")
	if err == nil {
		t.Fatal("dup on empty stack should error")
	}
	if !strings.Contains(err.Error(), "underflow") &&
		!strings.Contains(err.Error(), "stack has 0") {
		t.Errorf("expected stack-underflow message, got %v", err)
	}
}

// --- display words -------------------------------------------------

func TestPrintWritesToConfiguredWriter(t *testing.T) {
	c, buf := newCalc(t)
	_, err := c.Eval("add 2 3 print")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "5" {
		t.Errorf("print output = %q, want %q", buf.String(), "5\n")
	}
}

func TestShowDoesNotConsume(t *testing.T) {
	c, buf := newCalc(t)
	stk, err := c.Eval("1 2 3 show")
	if err != nil {
		t.Fatal(err)
	}
	if len(stk) != 3 {
		t.Errorf("show: want stack length 3, got %d", len(stk))
	}
	if got := strings.TrimSpace(buf.String()); got != "1 2 3" {
		t.Errorf("show output = %q, want %q", buf.String(), "1 2 3\n")
	}
}

func TestShowEmpty(t *testing.T) {
	c, buf := newCalc(t)
	_, err := c.Eval("show")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "(empty)" {
		t.Errorf("show on empty stack: got %q", buf.String())
	}
}

// --- Calc API ------------------------------------------------------

func TestReset(t *testing.T) {
	c, _ := newCalc(t)
	if _, err := c.Eval("1 2 3"); err != nil {
		t.Fatal(err)
	}
	c.Reset()
	stk := c.Stack()
	if len(stk) != 0 {
		t.Errorf("Reset: want empty stack, got %v", stk)
	}
}

func TestStackReturnsCopy(t *testing.T) {
	c, _ := newCalc(t)
	if _, err := c.Eval("1 2 3"); err != nil {
		t.Fatal(err)
	}
	stk := c.Stack()
	stk[0] = core.NewInteger(99)
	stk2 := c.Stack()
	n, _ := core.AsInteger(stk2[0])
	if n != 1 {
		t.Errorf("Stack() returned a live reference; mutating the copy changed the internal stack to %d", n)
	}
}

func TestFailedEvalLeavesStackIntact(t *testing.T) {
	c, _ := newCalc(t)
	if _, err := c.Eval("1 2"); err != nil {
		t.Fatal(err)
	}
	// Try to divide by zero — the stack already has [1 2] from the
	// previous Eval; an extra 0 makes `1 div 0` apply, which errors.
	if _, err := c.Eval("0 div"); err == nil {
		t.Fatal("0 div on [1,2] should error (division by zero)")
	}
	stk := c.Stack()
	if len(stk) != 2 {
		t.Errorf("after failed Eval: want stack=[1 2], got %v", stk)
	}
}

func TestParseError(t *testing.T) {
	c, _ := newCalc(t)
	_, err := c.Eval("((")
	if err == nil {
		t.Fatal("unmatched paren should error")
	}
	if !strings.Contains(err.Error(), "parse") &&
		!strings.Contains(err.Error(), "unmatched") {
		t.Errorf("want parse / unmatched in error, got %v", err)
	}
}

func TestUndefinedWordIsAnError(t *testing.T) {
	c, _ := newCalc(t)
	_, err := c.Eval("frobnicate")
	if err == nil {
		t.Fatal("undefined word should error")
	}
}

// --- FormatStack ---------------------------------------------------

func TestFormatStackEmpty(t *testing.T) {
	if got := FormatStack(nil); got != "(empty)" {
		t.Errorf("FormatStack(nil) = %q, want (empty)", got)
	}
}

func TestFormatStackOne(t *testing.T) {
	v := core.NewInteger(42)
	if got := FormatStack([]core.Value{v}); got != "42" {
		t.Errorf("FormatStack([42]) = %q, want 42", got)
	}
}

func TestFormatStackMany(t *testing.T) {
	stk := []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}
	if got := FormatStack(stk); got != "1 2 3" {
		t.Errorf("FormatStack([1 2 3]) = %q, want 1 2 3", got)
	}
}

// --- New() with nil writer -----------------------------------------

func TestNewWithNilWriter(t *testing.T) {
	c, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	// print to discard — no panic, no observable output.
	if _, err := c.Eval("42 print"); err != nil {
		t.Fatalf("print with nil writer: %v", err)
	}
}

// --- Smoke test: end-to-end -----------------------------------------

func TestEndToEnd(t *testing.T) {
	// A small program mirroring REPL usage:
	//   compute pi, double it twice, show 4π.
	c, buf := newCalc(t)
	if _, err := c.Eval("2 pi mul"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Eval("2 mul"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Eval("show"); err != nil {
		t.Fatal(err)
	}
	stk := c.Stack()
	got := asDec(t, stk, "4 * pi")
	if math.Abs(got-4*math.Pi) > 1e-9 {
		t.Errorf("end-to-end stack: got %v, want %v", got, 4*math.Pi)
	}
	if !strings.Contains(buf.String(), fmt.Sprintf("%g", 4*math.Pi)) {
		t.Errorf("end-to-end: show output missing 4π, got %q", buf.String())
	}
}
