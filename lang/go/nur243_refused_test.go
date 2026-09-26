package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR243ConstTakenArmWithNoValueDeclines pins one of NUR243's refused
// programs and covers the guard that refuses it — a guard that carried a
// covergate pragma whose proof was false: a constant-true branch whose
// taken arm leaves no value is valid code the lowering declines ("if:
// branch produces no value"), where the interpreter answers [1]. When
// NUR243 closes, the row compiles and agrees, and this test flips.
func TestNUR243ConstTakenArmWithNoValueDeclines(t *testing.T) {
	const src = `def x 0 if [true] [def x 1] [2] end x`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck: %v", cerr)
	}
	if prog != nil || !strings.Contains(reason, "branch produces no value") {
		t.Fatalf("want the NUR243 decline, got prog=%v reason=%q", prog != nil, reason)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, ierr := b.RunInterp(src)
	if ierr != nil || fmt.Sprint(got) != "[1]" {
		t.Fatalf("interpreter: %v / %v, want [1]", got, ierr)
	}
}
