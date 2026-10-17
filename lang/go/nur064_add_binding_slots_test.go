package lang

import (
	"fmt"
	"testing"
)

// nur064Svc is a service whose `create` handler reads its bound slot.
const nur064Svc = `def s (service {}) add {op:"create" text:String} ([req:Map state:Any] => [ text ]) s `

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
