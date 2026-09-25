package lang

import (
	"fmt"
	"testing"
)

// TestMiniLanguageValueIsTheFn pins NUR163's close. A fn value is the same
// fn however it is reached: through the module member in place (`M.dbl`),
// through a paren (`(M.dbl)`), through `/v`, or through a local def of the
// member. The mini / emit / parse contracts read the value's OWN signatures
// now (FnDefInfo.OwnSigs): a value read through `/v` or a member carries the
// dispatch aggregate, whose synthesized 0-arg fallback signature is not a
// declaration — it failed every aggregate-bearing spelling on the prefix
// check where the def-bound spelling passed.
func TestMiniLanguageValueIsTheFn(t *testing.T) {
	const modM = `import "boru:minilang" import module [ def dbl fn [[src:String opts:Map] [String] [src add src]] export "M" {dbl: dbl/v} ] end `
	const modE = `import "boru:emitlang" import module [ def up fn [[value:Any opts:Map] [String] ['UP']] export "M" {up: up/v} ] end `
	const modP = `import "boru:parselang" import module [ def p fn [[source:String opts:Map] [Any] [source]] export "M" {p: p/v} ] end `
	for _, src := range []string{
		modM + "mini M.dbl 'ab'",
		modM + "mini (M.dbl) 'ab'",
		modM + "def g M.dbl/v end mini g 'ab'",
		`import "boru:minilang" def dbl fn [[src:String opts:Map] [String] [src add src]] end mini (dbl/v) 'ab'`,
		`import "boru:minilang" def dbl fn [[src:String opts:Map] [String] [src add src]] end mini dbl 'ab'`,
		`import "boru:minilang" def f3 fn [[src:String opts:Map subject:Any] [Any] [subject]] end 'zz' mini (f3/v) 'ab'`,
		modE + "emit M.up {a:1}",
		modE + "emit (M.up) {a:1}",
		modE + "def g (M.up) end emit g {a:1}",
		modP + "parse M.p 'xy'",
		`import "boru:parselang" def p fn [[source:String opts:Map] [Any] [source]] end parse (p/v) 'xy'`,
	} {
		requireSameVerdict(t, src)
	}
	for _, c := range []struct{ src, want string }{
		{modM + "mini M.dbl 'ab'", "[abab]"},
		{modM + "mini (M.dbl) 'ab'", "[abab]"},
		{`import "boru:minilang" def dbl fn [[src:String opts:Map] [String] [src add src]] end mini (dbl/v) 'ab'`, "[abab]"},
		{modE + "emit M.up {a:1}", "[UP]"},
		{modP + "parse M.p 'xy'", "[xy]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestZeroArgTailApplyNeverCrashes pins NUR162's close. A `word` bound to a
// 0-arg fn VALUE, spliced at a paren's tail, is data on the interpreter
// (`5 fn`); the recorder used to record an apply over NO argument, which
// lowered as a native call under a signature entry carrying no signature —
// the disassembler and the VM both dereferenced it. The trailing apply's
// window trim declines a 0-arg callee now (the recorder's one failure
// site), the lowering keeps a belt against a signature-less apply, and the
// disassembler names such an entry instead of crashing. The rows decline
// loudly and the fallback answers as the interpreter does.
func TestZeroArgTailApplyNeverCrashes(t *testing.T) {
	for _, src := range []string{
		"(def dbl word ([] => [1]) end 5 dbl)",
		`import module [ def zzvmod fn [[] [] [def dbl word ([] => [1]) end 5 dbl]] export "ZZV" {run: zzvmod/v} ] end ZZV.run`,
	} {
		requireEngineParity(t, src, false)
		got, err := mustNew(t).RunInterp(src)
		if err != nil || fmt.Sprint(got) != "[5 fn]" {
			t.Errorf("%s: interpreter = %v / %v, want [5 fn]", src, got, err)
		}
	}
	// The unbracketed twin compiles: the spliced value is data.
	requireSameVerdict(t, "def dbl word ([] => [1]) end 5 dbl")
}
