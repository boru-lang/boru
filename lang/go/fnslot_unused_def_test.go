package lang

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// A fn handed to a Function-typed parameter is USED — `boru check` must not
// report `unused_def` for it.
//
// The slot used to take a BARE name as a value (stepWord's TFunction
// intercept), and a false positive on that path — `Sort.quick mycmp xs`,
// `filter pred xs`, every comparator and predicate API — was fixed by noting
// the use there. NUR078 retired the intercept: a bare name bound to a fn
// CALLS, universally, and the reference is spelled `/v`, so the use-recording
// re-homed with it to the `/v` read. A bare name at the slot is a call now —
// a use too, and the check reports the call's own failure.
//
// The negative half is the real contract: the note fires only on a SUCCESSFUL
// fn lookup, so a def that is genuinely never referenced must still warn.
func TestFunctionSlotArgIsNotUnused(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		wantUnused bool
		wantError  bool
		why        string
	}{
		{
			name: "explicit /v reference",
			src: "def mycmp fn [[b:Any a:Any] [Integer] [ a b cmp ]]\n" +
				"def use fn [[f:Function xs:List] [List] [ xs ]]\n" +
				"use mycmp/v [3 1 2]\n",
			wantUnused: false,
			why:        "mycmp IS used — it is the Function argument",
		},
		{
			name: "bare fn before a Function slot calls",
			src: "def mycmp fn [[b:Any a:Any] [Integer] [ a b cmp ]]\n" +
				"def use fn [[f:Function xs:List] [List] [ xs ]]\n" +
				"use mycmp [3 1 2]\n",
			wantUnused: false,
			wantError:  true,
			why:        "a bare name CALLS (NUR078) — a use, and use is left without its Function",
		},
		{
			name: "a genuinely unused def still warns",
			src: "def never-called fn [[n:Integer] [Integer] [ n ]]\n" +
				"1 add 2\n",
			wantUnused: true,
			why:        "the note must fire only on a successful fn lookup at the slot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			res, err := a.Check(tc.src)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			got, gotErr := false, false
			for _, d := range res.Diagnostics {
				if d.Code == "unused_def" {
					got = true
				}
				if d.Severity == native.SeverityError {
					gotErr = true
					if !tc.wantError {
						t.Errorf("unexpected check error: [%s] %s", d.Code, d.Detail)
					}
				}
			}
			if got != tc.wantUnused {
				t.Errorf("unused_def reported = %v, want %v — %s", got, tc.wantUnused, tc.why)
			}
			if tc.wantError && !gotErr {
				t.Errorf("no check error reported — %s", tc.why)
			}
		})
	}
}
