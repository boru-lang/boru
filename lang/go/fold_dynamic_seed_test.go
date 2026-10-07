package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// A fold whose SEED is gradual (a `(do {…})` result, a get-result threaded as
// the init) must produce a gradual accumulator, so a body word over the
// accumulator (`acc … get`, `add`) poly-matches optimistically instead of
// failing no_signature against a strict Any. foldAccCarrier preserves the
// seed's dynamic modality. This is radix's `common-prefix-len`, whose seed is a
// `(do {n: …, ok: …})` map.
func TestFoldDynamicSeedAccumulator(t *testing.T) {
	errCount := func(src string) int {
		a, _ := lang.New()
		cr, _ := a.Check(src)
		n := 0
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				n++
			}
		}
		return n
	}

	// `(do {n:0})` seed → gradual accumulator → `acc "n" get` + `add` clean.
	clean := `def f fn [[a:String] [Any] [
  (do {n: 0}) (iota 3) [ ([i acc] => [do {n: [(acc "n" get) 1 add]}]) apply ] fold
]] end`
	if n := errCount(clean); n != 0 {
		t.Errorf("fold over a (do {…}) seed: expected 0 errors, got %d", n)
	}

	// A plain map-literal seed already worked (the accumulator is a concrete
	// Map); guard it stays clean.
	clean2 := `def f fn [[a:String] [Any] [
  {n: 0} (iota 3) [ ([i acc] => [(acc "n" get)]) apply ] fold
]] end`
	if n := errCount(clean2); n != 0 {
		t.Errorf("fold over a map-literal seed: expected 0 errors, got %d", n)
	}
}
