package core

import "testing"

// recordParenReStep is fnReturnPark's negative twin: where the park says "this
// collapse PLACED a Function", this says "this collapse is about to RE-STEP one
// into a call". The two together are the paren re-step rule
// (design/PAREN-RESTEP-RULE.0.md), and the compiler needs BOTH because the
// residual it later lowers is byte-identical for the two spellings —
// `(mk 1) 2` places, `((mk 1) 2)` applies, and after the collapse there is
// nothing left to tell them apart.
//
// Tested here for the same reason fnReturnPark is (fn_return_park_test.go's
// header): core/go is gated by its OWN suite at a 100% floor, so a decision
// function reached only from above reads as uncovered there. Every negative
// carries its reason — a record one case too wide makes the compiler APPLY a
// value the interpreter places, which is a wrong answer rather than a compile failure.
func TestRecordParenReStep(t *testing.T) {
	carrier := func() Value {
		v := NewCarrier(TFunction)
		v.ID = "fnc-1"
		return v
	}
	dynAny := func() Value {
		v := NewCarrier(TAny)
		v.Dynamic = true
		v.ID = "dyn-1"
		return v
	}

	cases := []struct {
		name     string
		tape     []Value
		closeIdx int
		park     int
		reach    bool
		noCheck  bool
		want     bool
		why      string
	}{
		{
			name: "carrier leading two survivors is re-stepped",
			tape: []Value{carrier(), NewInteger(2)}, closeIdx: 3, want: true,
			why: "the park declined, so the rewind lands on the carrier and stepLiteral dispatches it",
		},
		{
			name: "dynamic maybe-fn leading two survivors is re-stepped",
			tape: []Value{dynAny(), NewInteger(2)}, closeIdx: 3, want: true,
			why: "dynamic(Any) does not exclude Function, and the rewind cannot know either — " +
				"the same not-disjoint test the auto-dispatch guard uses",
		},
		{
			name: "a park of 1 records nothing",
			tape: []Value{carrier()}, closeIdx: 2, park: 1, want: false,
			why: "the park PLACED it: the pointer steps past, and no rewind ever reaches the value",
		},
		{
			name: "one survivor records nothing",
			tape: []Value{carrier()}, closeIdx: 2, want: false,
			why: "the survivor count is the question — one survivor is the placement case",
		},
		{
			name: "a reach group records nothing",
			tape: []Value{carrier(), NewInteger(2)}, closeIdx: 3, reach: true, want: false,
			why: "a reach-lowered group's re-step is its own dispatch, not a user paren's apply",
		},
		{
			name:     "a quoted lead records nothing",
			tape:     []Value{func() Value { v := carrier(); v.Quoted = true; return v }(), NewInteger(2)},
			closeIdx: 3, want: false,
			why: "a quoted value is inert on both lanes — the rewind pushes it, it does not call it",
		},
		{
			name:     "an ID-less lead records nothing",
			tape:     []Value{func() Value { v := carrier(); v.ID = ""; return v }(), NewInteger(2)},
			closeIdx: 3, want: false,
			why: "the record is keyed by value ID; without one there is nothing the compiler could read back",
		},
		{
			name: "a non-callable lead records nothing",
			tape: []Value{NewInteger(1), NewInteger(2)}, closeIdx: 3, want: false,
			why: "an Integer leading the survivors is not something the rewind could call",
		},
		{
			name: "a lead past the tape end records nothing",
			tape: []Value{}, closeIdx: 3, want: false,
			why: "the bounds guard: an empty collapse leaves openIdx past the end",
		},
		{
			name: "no check state records nothing",
			tape: []Value{carrier(), NewInteger(2)}, closeIdx: 3, noCheck: true, want: false,
			why: "the interpreter runs with no analysis state at all; the record is a check-pass artefact",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{Tape: NewTape(tc.tape, 4)}
			if !tc.noCheck {
				e.Registry = &Registry{Check: &CheckState{}}
			} else {
				e.Registry = &Registry{}
			}
			e.recordParenReStep(0, tc.closeIdx, tc.park, tc.reach)
			got := e.Registry.Check != nil && len(e.Registry.Check.ParenReSteppedFnIDs) > 0
			if got && len(tc.tape) > 0 && !e.Registry.Check.ParenReSteppedFnIDs[tc.tape[0].ID] {
				t.Errorf("recorded an ID other than the lead's: %v", e.Registry.Check.ParenReSteppedFnIDs)
			}
			if got != tc.want {
				t.Errorf("recorded = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}

	// The map is created lazily and REUSED: a second re-step in the same pass
	// must not drop the first, or the compiler forgets an apply it was told
	// about and silently places it.
	e := &Engine{Tape: NewTape([]Value{carrier(), NewInteger(2)}, 4), Registry: &Registry{Check: &CheckState{}}}
	e.recordParenReStep(0, 3, 0, false)
	second := dynAny()
	e.Tape = NewTape([]Value{second, NewInteger(2)}, 4)
	e.recordParenReStep(0, 3, 0, false)
	if !e.Registry.Check.ParenReSteppedFnIDs["fnc-1"] || !e.Registry.Check.ParenReSteppedFnIDs["dyn-1"] {
		t.Errorf("second record replaced the first: %v", e.Registry.Check.ParenReSteppedFnIDs)
	}
}
