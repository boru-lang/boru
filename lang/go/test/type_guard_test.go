package test

import (
	"testing"
)

// --- guard: predicate-body workhorse ---
//
// `cond guard value` returns value when cond is true, None otherwise.
// Designed for the predicate-type idiom: every predicate body has
// the shape "compute a Boolean, return val on true / None on false",
// which `guard` shortens.

func TestGuard_TruePassesValue(t *testing.T) {
	got := runOne(t, `true guard 42`)
	if len(got) != 1 || got[0] != int64(42) {
		t.Errorf("true guard 42 = %v, want [42]", got)
	}
}

func TestGuard_FalseGivesNone(t *testing.T) {
	got := runOne(t, `false guard 42`)
	if len(got) != 1 || got[0] != "None" {
		t.Errorf("false guard 42 = %v, want [\"None\"]", got)
	}
}

// guard composes naturally with `and`-chains in a predicate body.
// The Bbd type ("string between 'b' and 'd' inclusive") written
// with guard is membership-equivalent to the if/None form.
func TestGuard_PredicateIdiom(t *testing.T) {
	got := runOne(t, `def Bbd fnpred x:Any [(x is String) and (x gte "b") and (x lte "d") guard x]
"a" is Bbd
"b" is Bbd
"c" is Bbd
"d" is Bbd
"e" is Bbd
99 is Bbd`)
	want := []string{"false", "true", "true", "true", "false", "false"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d results", got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// guard's BarrierPos=1 keeps it from greedily consuming the next
// expression as a second forward arg — `cond guard x; y` doesn't
// pull y in.
func TestGuard_BarrierPosDoesNotEatNextToken(t *testing.T) {
	got := runOne(t, `true guard 1
2`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != int64(1) {
		t.Errorf("first result = %v, want 1", got[0])
	}
	if got[1] != int64(2) {
		t.Errorf("second result = %v, want 2", got[1])
	}
}

// Used inside `def x:*Type body` via a transforming predicate: the
// returned value is what binds.
func TestGuard_TypedDefAcceptsTransformed(t *testing.T) {
	got := runOne(t, `def Up fnpred x:Any [(x is String) guard (x upper)]
def s:Up "hi"
s`)
	if len(got) != 1 || got[0] != "HI" {
		t.Errorf("def s:Up \"hi\"; s = %v, want [\"HI\"]", got)
	}
}
