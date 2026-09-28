package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur064Svc is a service whose `create` handler reads its bound slot.
const nur064Svc = `def s (service {}) add {op:"create" text:String} ([req:Map state:Any] => [ text ]) s `

// TestNUR064AddPatternBindsItsSlots pins NUR064: a service `add` pattern is
// the clause pattern a `receive` clause is — scalar fields route, `name:Type`
// fields are binding slots the handler's run sees by name — on both lanes.
// A layered handler binds its own slots, a slot declining the request falls
// back to a slot-free catch-all, a fn the handler builds reads the slot
// through the run's scope, and `req` still carries the whole request.
func TestNUR064AddPatternBindsItsSlots(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nur064Svc + `call {op:"create" text:"hi"} s`, "[hi]"},
		{`def s (service {}) add {op:"inc" n:Integer} ([req:Map state:Any] => [ n add 1 ]) s ` +
			`add {op:"inc" n:Integer} ([req:Map state:Any prior:Any] => [ (prior req) mul 10 ]) s call {op:"inc" n:4} s`, "[50]"},
		{`def s (service {}) add {} ([req:Map state:Any] => [ "caught" ]) s ` +
			`add {op:"create" text:String} ([req:Map state:Any] => [ text ]) s call {op:"create" text:1} s`, "[caught]"},
		{`def s (service {}) add {op:"inc" n:Integer} ([req:Map state:Any] => [ (fn [[] [Any] [n]]) ]) s call {op:"inc" n:4} s`, "[4]"},
		{`def s (service {}) add {op:"create" text:String} ([req:Map state:Any] => [ req.text ]) s call {op:"create" text:"hi"} s`, "[hi]"},
		{`send {op:"create" text:"hi"} (self) receive [ {op:"create" text:String} [ text ] ]`, "[hi]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
}

// TestNUR064DecliningSlotsRaiseNoMatch is the paired negative: a request the
// routed handler's slots decline, with no slot-free catch-all to fall to,
// raises no_match on both lanes — as the `receive` clause twin does; a
// catch-all whose own handler has slots is no fallback; and a request `prior`
// passes on without a lower handler's slot field reaches no handler.
func TestNUR064DecliningSlotsRaiseNoMatch(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{nur064Svc + `call {op:"create" text:1} s`, "failed its typed binding slots"},
		{nur064Svc + `call {op:"create"} s`, "failed its typed binding slots"},
		{`def s (service {}) add {n:Integer} ([req:Map state:Any] => [ n ]) s call {op:"x"} s`, "failed its typed binding slots"},
		{`def s (service {}) add {op:"x" n:Integer} ([req:Map state:Any] => [ n ]) s ` +
			`add {op:"x"} ([req:Map state:Any prior:Any] => [ prior {op:"x"} ]) s call {op:"x" n:1} s`, "passed on by prior fails"},
		{`send {op:"create" text:1} (self) receive [ {op:"create" text:String} [ text ] ]`, "failed its typed binding slots"},
	} {
		_, _, errC, _, errI := runBothEngines(t, c.src)
		for lane, err := range map[string]error{"interpreter": errI, "compiled": errC} {
			if err == nil || !strings.Contains(err.Error(), "no_match") || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %s error %v, want no_match %q", c.src, lane, err, c.want)
			}
		}
	}
}

// TestNUR064SlotReadsPassTheCheck pins the check half: the handler's read of
// a slot's name is the slot's, not an undefined word — while a misspelt
// read beside it, and the same name read outside the handler, stay flagged.
func TestNUR064SlotReadsPassTheCheck(t *testing.T) {
	undefined := func(src string) []string {
		t.Helper()
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		res, err := b.Check(src)
		if err != nil {
			t.Fatalf("%s: check: %v", src, err)
		}
		var words []string
		for _, d := range res.Diagnostics {
			if d.Code == "undefined_word" {
				words = append(words, d.Word)
			}
		}
		return words
	}
	if got := undefined(nur064Svc); len(got) != 0 {
		t.Errorf("a slot read must pass the check, got undefined %v", got)
	}
	typo := `def s (service {}) add {op:"create" text:String} ([req:Map state:Any] => [ texx ]) s`
	if got := undefined(typo); fmt.Sprint(got) != "[texx]" {
		t.Errorf("a misspelt read must stay flagged, got %v", got)
	}
	outside := `def g ([req:Map state:Any] => [ text ]) def s (service {}) add {op:"create"} g/v s`
	if got := undefined(outside); fmt.Sprint(got) != "[text]" {
		t.Errorf("a read no slot binds must stay flagged, got %v", got)
	}
}
