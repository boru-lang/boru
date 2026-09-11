package langspec

import "testing"

// TestWhileSpreadElementCoversEveryResidualValue pins the check model behind
// the fifty-first increment's graduated row.
//
// A while's plain-check residual is a VARIADIC SPREAD, and a spread carries
// ONE element type. A body that nets more than one value per round leaves one
// of EACH on every trip, interleaved, so that single element has to cover all
// of them. The model took stk's LAST value alone, which is a false claim
// about the other N-1 — invisible for as long as that last value happened to
// be a dynamic carrier, which covers anything (control.tsv §7's counter
// loop), and false the moment it is a concrete Integer over a body that also
// leaves the flex container.
//
// TestCheckTypeSoundness would catch a regression too, as a nameless "+1
// violation" over 6,475 rows. This says which row and why.
func TestWhileSpreadElementCoversEveryResidualValue(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"mixed residual types", `def zc (flex {n:0}) end while [(zc get 'n') lt 2] [ set 'n' ((zc get 'n') add 1) zc end (zc get 'n') ]`},
		{"the graduated row", `def c (flex {n:0}) end while [(c get 'n') lt 3] [ set 'n' ((c get 'n') add 1) c end if ((c get 'n') eq 2) [continue] end (c get 'n') ]`},
		{"single-typed body", `def zn 0 end while [zn lt 2] [def zn (zn add 1) zn zn] end 'z'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checked, flagged := checkRow(t, tc.src)
			if flagged {
				t.Fatal("the row must check clean — a flagged row is skipped by the soundness oracle")
			}
			actual, ok := runRow(t, tc.src)
			if !ok {
				t.Fatal("the row must run")
			}
			if len(actual) < 2 {
				t.Fatalf("the witness must leave several values, got %s", stackTypes(actual))
			}
			if !stackTypeCovered(checked, actual) {
				t.Errorf("the spread element does not cover the run:\n  checked=%s\n  actual=%s",
					stackTypes(checked), stackTypes(actual))
			}
		})
	}
}
