package modules

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// mcovReg builds a registry with the module resolver wired.
func mcovReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	return r
}

func mcovRun(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := native.NewTop(r).Run(vals)
	if err != nil {
		t.Fatalf("run: %v\n--- src ---\n%s", err, src)
	}
	return out
}

func mcovErr(t *testing.T, r *native.Registry, src string) error {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, rerr := native.NewTop(r).Run(vals)
	return rerr
}

const mcovImp = `import "boru:minilang"  `

// TestMiniCovHexBytes drives the hb kind: grouping, size, and the loud
// odd-length / non-hex rejections.
func TestMiniCovHexBytes(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`mini hb 'de_ad be_ef' size`)
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 4 {
		t.Errorf("hb size = %v, want 4 (err %v)", out[0], err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini hb 'xyz'`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("non-hex source should be mini_parse_error, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini hb 'abc'`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("odd-length source should be mini_parse_error, got %v", err)
	}
}

// TestMiniCovBinBytes drives the bb kind: value equivalence with hb plus
// the bit-count and bad-digit rejections.
func TestMiniCovBinBytes(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`(mini bb '01001100') (mini hb '4c') eq`)
	if b, err := out[0].AsConcreteBoolean(); err != nil || !b {
		t.Errorf("bb '01001100' should equal hb '4c', got %v (err %v)", out[0], err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini bb '0100110'`); err == nil ||
		!strings.Contains(err.Error(), "not a multiple of 8") {
		t.Errorf("7 bits should be rejected, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini bb '01001102'`); err == nil ||
		!strings.Contains(err.Error(), "not a binary digit") {
		t.Errorf("'2' should be rejected, got %v", err)
	}
}

// TestMiniCovRe drives both re paths — the expansion-time compile hook
// (mini re → run-re) and the plain transducer (MiniLang.lang_re) — plus
// opts, capture groups, and the no-match shape.
func TestMiniCovRe(t *testing.T) {
	r := mcovReg(t)

	// Hook path (mini re) and transducer path (lang_re) agree.
	hook := mcovRun(t, r, mcovImp+`('AbcD' mini re '[a-z]+').fst.m`)
	if s, _ := hook[0].AsConcreteString(); s != "bc" {
		t.Errorf("mini re fst.m = %v, want bc", hook[0])
	}
	trans := mcovRun(t, r, mcovImp+`('AbcD' MiniLang.lang_re '[a-z]+' {} end).fst.m`)
	if s, _ := trans[0].AsConcreteString(); s != "bc" {
		t.Errorf("lang_re fst.m = %v, want bc", trans[0])
	}

	// opts.limit caps matches on both paths.
	lim := mcovRun(t, r, mcovImp+`('a1b2c3' mini re '\\d' {limit:2}).n`)
	if n, _ := lim[0].AsConcreteInteger(); n != 2 {
		t.Errorf("limit:2 n = %v, want 2", lim[0])
	}

	// Unmatched capture groups render as none.
	grp := mcovRun(t, r, mcovImp+`('ab' mini re '(a)(x)?(b)').fst.g`)
	gl, glErr := native.AsList(grp[0])
	if glErr != nil || gl.Len() != 3 || !native.IsNone(gl.Get(1)) {
		t.Errorf("groups = %v, want ['a' none 'b'] (err %v)", grp[0], glErr)
	}

	// No match: ok=false, n=0, no fst.
	nm := mcovRun(t, r, mcovImp+`('abc' mini re 'z').ok`)
	if b, _ := nm[0].AsConcreteBoolean(); b {
		t.Errorf("no-match ok = %v, want false", nm[0])
	}
}

// TestMiniCovReRuneOffsets pins NUR047's resolution on both re paths:
// each match's i/e are RUNE indices into the subject — Go's regexp
// reports byte offsets and reMatchResult converts them in one incremental
// pass — so they compose with `slice` directly. Covers a match after
// multi-byte runes, a multi-byte match, an astral rune, and the
// zero-width case (i == e, one match per rune boundary).
func TestMiniCovReRuneOffsets(t *testing.T) {
	r := mcovReg(t)

	// Transducer path (lang_re): a match after three 3-byte runes.
	li := mcovRun(t, r, mcovImp+`('日本語c' MiniLang.lang_re 'c' {} end).fst.i`)
	if n, _ := li[0].AsConcreteInteger(); n != 3 {
		t.Errorf("lang_re fst.i = %v, want rune index 3 (bytes said 9)", li[0])
	}
	le := mcovRun(t, r, mcovImp+`('日本語c' MiniLang.lang_re 'c' {} end).fst.e`)
	if n, _ := le[0].AsConcreteInteger(); n != 4 {
		t.Errorf("lang_re fst.e = %v, want rune index 4 (bytes said 10)", le[0])
	}

	// Compiled path (mini re → run-re) agrees, and the offsets compose
	// with the rune-counting slice — the composition NUR047 recorded as
	// broken (it returned '' under byte offsets).
	comp := mcovRun(t, r, mcovImp+`def r ('日本語c' mini re 'c') end  slice (r.fst.i) (r.fst.e) '日本語c'`)
	if s, _ := comp[0].AsConcreteString(); s != "c" {
		t.Errorf("slice by rune offsets = %v, want 'c'", comp[0])
	}

	// A multi-byte MATCH spans runes 1..3, never bytes 1..7.
	me := mcovRun(t, r, mcovImp+`('a日本b' mini re '日本').fst.e`)
	if n, _ := me[0].AsConcreteInteger(); n != 3 {
		t.Errorf("multi-byte match fst.e = %v, want 3 (bytes said 7)", me[0])
	}

	// An astral (4-byte) rune is ONE index step.
	ae := mcovRun(t, r, mcovImp+`('x🎉y' mini re '🎉').fst.e`)
	if n, _ := ae[0].AsConcreteInteger(); n != 2 {
		t.Errorf("astral match fst.e = %v, want 2 (bytes said 5)", ae[0])
	}

	// Zero-width matches: one per rune boundary, i == e at each.
	zn := mcovRun(t, r, mcovImp+`('日a' mini re '').n`)
	if n, _ := zn[0].AsConcreteInteger(); n != 3 {
		t.Errorf("zero-width n = %v, want 3 (boundaries 0 1 2)", zn[0])
	}
	zi := mcovRun(t, r, mcovImp+`('日a' mini re '').lst.i`)
	if n, _ := zi[0].AsConcreteInteger(); n != 2 {
		t.Errorf("zero-width lst.i = %v, want 2 (byte 4)", zi[0])
	}
	ze := mcovRun(t, r, mcovImp+`('日a' mini re '').lst.e`)
	if n, _ := ze[0].AsConcreteInteger(); n != 2 {
		t.Errorf("zero-width lst.e = %v, want 2 — i == e", ze[0])
	}
}

// TestMiniCovRuneOffsetScanner drives the converter directly: ascending
// offsets convert in one forward scan, a repeated offset (a zero-width
// match, or a match abutting its predecessor) advances nothing, and a
// multi-byte / astral rune counts as one index step.
func TestMiniCovRuneOffsetScanner(t *testing.T) {
	sc := &runeOffsetScanner{subject: "a日🎉b"}
	// byte layout: a=0, 日=1..3, 🎉=4..7, b=8, end=9
	for _, c := range []struct{ byteOff, want int }{
		{0, 0}, {0, 0}, // zero-width at the start: repeated, no advance
		{1, 1}, {4, 2}, {4, 2}, {8, 3}, {9, 4},
	} {
		if got := sc.runeIdx(c.byteOff); got != c.want {
			t.Errorf("runeIdx(%d) = %d, want %d", c.byteOff, got, c.want)
		}
	}
}

// TestMiniCovReNegatives pins the loud failures of both re paths: a
// malformed pattern (hook defers to the transducer's error) and a
// non-Integer limit.
func TestMiniCovReNegatives(t *testing.T) {
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' mini re '('`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("bad pattern via mini re should be mini_parse_error, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' MiniLang.lang_re '(' {} end`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("bad pattern via lang_re should be mini_parse_error, got %v", err)
	}
	// run-re path with a bad opts.limit.
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' mini re 'x' {limit:'z'}`); err == nil ||
		!strings.Contains(err.Error(), "opts.limit") {
		t.Errorf("bad limit (run-re) should name opts.limit, got %v", err)
	}
	// lang_re path with a bad opts.limit.
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' MiniLang.lang_re 'x' {limit:'z'} end`); err == nil ||
		!strings.Contains(err.Error(), "opts.limit") {
		t.Errorf("bad limit (lang_re) should name opts.limit, got %v", err)
	}
}

// TestMiniCovBf drives the brainfuck kind: filter form (stack input),
// generator form (opts.in), the `,`-at-EOF zero, and a loop program.
func TestMiniCovBf(t *testing.T) {
	r := mcovReg(t)
	gen := mcovRun(t, r, mcovImp+`mini bf ',+.' {in:'A'}`)
	if s, _ := gen[0].AsConcreteString(); s != "B" {
		t.Errorf("generator form = %v, want B", gen[0])
	}
	fil := mcovRun(t, r, mcovImp+`'A' mini bf ',+.'`)
	if s, _ := fil[0].AsConcreteString(); s != "B" {
		t.Errorf("filter form = %v, want B", fil[0])
	}
	eof := mcovRun(t, r, mcovImp+`(mini bf ',+.') size`)
	if n, _ := eof[0].AsConcreteInteger(); n != 1 {
		t.Errorf("EOF read should still emit one byte, got %v", eof[0])
	}
	loop := mcovRun(t, r, mcovImp+`mini bf '++++++++[>++++++++<-]>+.'`)
	if s, _ := loop[0].AsConcreteString(); s != "A" {
		t.Errorf("loop program = %v, want A (8*8+1=65)", loop[0])
	}
}

// TestMiniCovBfNegatives pins runBrainfuck's rejections: unbalanced
// brackets (both directions), the step budget, tape underrun, and the
// opts type errors.
func TestMiniCovBfNegatives(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"unbalanced close", `mini bf ']'`, "unbalanced ']'"},
		{"unbalanced open", `mini bf '['`, "unbalanced '['"},
		{"step budget", `mini bf '+[]' {steps:10}`, "step budget"},
		{"tape underrun", `mini bf '<'`, "pointer out of range"},
		{"bad steps opt", `mini bf '+.' {steps:'x'}`, "opts.steps"},
		{"bad in opt", `mini bf ',.' {in:5}`, "opts.in"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mcovErr(t, mcovReg(t), mcovImp+c.src)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestMiniCovMicronBuiltin drives the no-registrations micron fast path
// (Emailon / Pathon dispatch) and its parse failure.
func TestMiniCovMicronBuiltin(t *testing.T) {
	r := mcovReg(t)
	em := mcovRun(t, r, mcovImp+`mini m 'alice@example.com' typeof`)
	if em[0].String() != "Emailon" {
		t.Errorf("email literal typeof = %s, want Emailon", em[0].String())
	}
	pa := mcovRun(t, r, mcovImp+`mini micron 'a/b' typeof`)
	if pa[0].String() != "Pathon" {
		t.Errorf("path literal typeof = %s, want Pathon", pa[0].String())
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini m 'a b'`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("whitespace source should be mini_parse_error, got %v", err)
	}
}

// TestMiniCovMicronTombstone pins the frozen +m literal grammar: the
// former MiniLang.micron user-shape hook raises mini_registry_frozen
// unconditionally — with and without legacy args — and a user Micron
// TYPE still works through make (only the literal sugar is builtin-only).
func TestMiniCovMicronTombstone(t *testing.T) {
	if err := mcovErr(t, mcovReg(t), mcovImp+`MiniLang.micron`); err == nil ||
		!strings.Contains(err.Error(), "mini_registry_frozen") {
		t.Errorf("micron tombstone must raise mini_registry_frozen, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`def Ticketon refine Micron {id:String}  `+
		`MiniLang.micron Ticketon {options:{match:{token:{'#TK':'@/T-[0-9]+/'}}}} ([s:String] => [make Ticketon {id:s}])`); err == nil ||
		!strings.Contains(err.Error(), "mini_registry_frozen") {
		t.Errorf("a legacy micron registration must raise mini_registry_frozen, got %v", err)
	}
	// The user TYPE itself is untouched; +m never claims its spans (Pathon
	// catch-all), and make still constructs instances.
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`def Ticketon refine Micron {id:String}  (make Ticketon {id:'T-1'}) typeof`)
	if out[0].String() != "Ticketon" {
		t.Errorf("make Ticketon typeof = %s, want Ticketon", out[0].String())
	}
	out2 := mcovRun(t, r, `mini m 'T-123' typeof`)
	if out2[0].String() != "Pathon" {
		t.Errorf("an unclaimed span stays a Pathon, got %s", out2[0].String())
	}
}

// TestMiniCovValueForms drives the def-bound value forms that replaced
// registration: a generator fn (named params via opts) and a filter fn
// (stack subject), both dispatched through `mini <name>`; kinds stays the
// fixed built-in list.
func TestMiniCovValueForms(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+
		`def poly (fn [[src:String opts:Map] [Integer] [((opts.x pow 2) add (3 mul opts.y))]])  `+
		`mini poly 'x^2 + 3*y' {x:10, y:2}`)
	if n, _ := out[0].AsConcreteInteger(); n != 106 {
		t.Errorf("generator value = %v, want 106", out[0])
	}

	r2 := mcovReg(t)
	out2 := mcovRun(t, r2, mcovImp+
		`def count (fn [[src:String opts:Map subject:String] [Integer] [def m (subject mini re src)  m.n]])  `+
		`"banana" mini count 'an'`)
	if n, _ := out2[0].AsConcreteInteger(); n != 2 {
		t.Errorf("filter value = %v, want 2", out2[0])
	}
	// kinds is the FIXED built-in list — a def-bound value never appears.
	kinds := mcovRun(t, r2, `MiniLang.kinds`)
	if strings.Contains(kinds[0].String(), "count") || !strings.Contains(kinds[0].String(), "re") {
		t.Errorf("kinds = %s, want the built-ins only", kinds[0].String())
	}
}

// TestMiniCovRegisterTombstones pins the frozen registry: both register
// words raise mini_registry_frozen unconditionally, and the fn-value
// contract failure is loud on the value form.
func TestMiniCovRegisterTombstones(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"register tombstone", mcovImp + `MiniLang.register`, "mini_registry_frozen"},
		{"register with legacy args", mcovImp +
			`MiniLang.register re (fn [[s:String o:Map] [Integer] [1]])`, "mini_registry_frozen"},
		{"register-compiled tombstone", mcovImp + `MiniLang.register-compiled`, "mini_registry_frozen"},
		{"value form bad signature", mcovImp +
			`def bad (fn [[a:Integer] [Integer] [a]])  mini bad 'x'`, "mini_bad_signature"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mcovErr(t, mcovReg(t), c.src)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// mcovHostSpec builds a Go mini-language spec with one stack input: it
// returns the subject's length plus the length of src.
func mcovHostSpec(name string) MiniLangSpec {
	return MiniLangSpec{
		Name:    name,
		Inputs:  []native.FnParam{{Type: native.TString}},
		Returns: []*native.Type{native.TInteger},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString()
			if err != nil {
				return nil, err
			}
			subj, err := args[2].AsConcreteString()
			if err != nil {
				return nil, err
			}
			return []native.Value{native.NewInteger(int64(len(src) + len(subj)))}, nil
		},
	}
}

// TestMiniCovNewMiniLangFn drives the value constructor: a def-bound value
// with a stack input dispatches through `mini <name>` (import-free), the
// constructor's compile failures are loud, and a binding never shadows a built-in
// kind.
func TestMiniCovNewMiniLangFn(t *testing.T) {
	r := mcovReg(t)
	v, err := NewMiniLangFn(mcovHostSpec("hlen"))
	if err != nil {
		t.Fatalf("NewMiniLangFn: %v", err)
	}
	native.InstallDef(r, "hlen", v)
	out := mcovRun(t, r, `'hello' mini hlen 'xy'`)
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Errorf("bound value = %v, want 7 (2+5)", out[0])
	}

	// Constructor compile failures.
	if _, err := NewMiniLangFn(MiniLangSpec{Name: "", Handler: mcovHostSpec("x").Handler}); err == nil {
		t.Error("empty name must be declined")
	}
	if _, err := NewMiniLangFn(MiniLangSpec{Name: "nohandler"}); err == nil {
		t.Error("nil handler must be declined")
	}
	if _, err := NewMiniLangFn(MiniLangSpec{Name: "not a word", Handler: mcovHostSpec("x").Handler}); err == nil {
		t.Error("an invalid word name must be declined")
	}

	// A built-in kind wins over a same-named binding: `re` still
	// regex-matches (the bound value would return an Integer length).
	r2 := mcovReg(t)
	v2, err := NewMiniLangFn(mcovHostSpec("re"))
	if err != nil {
		t.Fatalf("NewMiniLangFn(re): %v", err)
	}
	native.InstallDef(r2, "re", v2)
	out2 := mcovRun(t, r2, mcovImp+`def m ("AbcD" mini re '[a-z]+')  m.fst.m`)
	if s, _ := out2[0].AsConcreteString(); s != "bc" {
		t.Errorf("built-in re should win over the binding: got %v, want bc", out2[0])
	}
}

// TestMiniCovNewMiniLangFnQuotedInput pins the FnParam.Quote contract on a
// constructed value: a Quote:true Input rides as QuoteArgs on the inner
// native, so the trivial-delegation dispatch /q-captures a bare word into
// the quoted slot instead of evaluating it (which would be undefined_word).
func TestMiniCovNewMiniLangFnQuotedInput(t *testing.T) {
	r := mcovReg(t)
	v, err := NewMiniLangFn(MiniLangSpec{
		Name:    "ql",
		Inputs:  []native.FnParam{{Type: native.TAtom, Quote: true}},
		Returns: []*native.Type{native.TString},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			a, err := args[2].AsConcreteAtom()
			if err != nil {
				return nil, r.BoruError("mini_error", "ql: input: "+err.Error(), "ql")
			}
			return []native.Value{native.NewString("q:" + a)}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewMiniLangFn: %v", err)
	}
	native.InstallDef(r, "ql", v)
	// The direct call forward-collects the bare word into the quoted slot.
	out := mcovRun(t, r, `ql 'src' {} someword end`)
	if s, _ := out[0].AsConcreteString(); s != "q:someword" {
		t.Errorf("quoted input = %v, want q:someword (the bare word as an atom)", out[0])
	}
}

// TestMiniCovOptHelpersDirect pins miniOptInt / miniOptString /
// miniDropGrouping / miniCompiledPattern directly.
func TestMiniCovOptHelpersDirect(t *testing.T) {
	// Non-map opts fall back to the default.
	if n, err := miniOptInt(native.NewInteger(3), "k", 42); err != nil || n != 42 {
		t.Errorf("miniOptInt(non-map) = %d,%v want 42,nil", n, err)
	}
	if s, err := miniOptString(native.NewInteger(3), "k"); err != nil || s != "" {
		t.Errorf("miniOptString(non-map) = %q,%v want \"\",nil", s, err)
	}
	m := native.NewOrderedMap()
	m.Set("i", native.NewInteger(7))
	m.Set("s", native.NewString("v"))
	m.Set("bad", native.NewBoolean(true))
	opts := native.NewMap(m)
	if n, err := miniOptInt(opts, "i", 0); err != nil || n != 7 {
		t.Errorf("miniOptInt present = %d,%v want 7,nil", n, err)
	}
	if n, err := miniOptInt(opts, "absent", 9); err != nil || n != 9 {
		t.Errorf("miniOptInt absent = %d,%v want 9,nil", n, err)
	}
	if _, err := miniOptInt(opts, "bad", 0); err == nil {
		t.Error("miniOptInt should decline a non-Integer value")
	}
	if s, err := miniOptString(opts, "s"); err != nil || s != "v" {
		t.Errorf("miniOptString present = %q,%v want v,nil", s, err)
	}
	if s, err := miniOptString(opts, "absent"); err != nil || s != "" {
		t.Errorf("miniOptString absent = %q,%v", s, err)
	}
	if _, err := miniOptString(opts, "bad"); err == nil {
		t.Error("miniOptString should decline a non-String value")
	}

	if got := miniDropGrouping("de_ad be\tef\r\n"); got != "deadbeef" {
		t.Errorf("miniDropGrouping = %q, want deadbeef", got)
	}

	re1, err := miniCompiledPattern("cov-[a-z]+")
	if err != nil {
		t.Fatalf("miniCompiledPattern: %v", err)
	}
	re2, _ := miniCompiledPattern("cov-[a-z]+")
	if re1 != re2 {
		t.Error("miniCompiledPattern should memoize by source")
	}
	if _, err := miniCompiledPattern("("); err == nil {
		t.Error("miniCompiledPattern should surface a compile error")
	}
}

// TestMiniCovShapeReturnsDirect covers the check-mode shape hooks for re
// and gex.
func TestMiniCovShapeReturnsDirect(t *testing.T) {
	if out := miniReShapeReturns(nil, nil); len(out) != 1 {
		t.Errorf("miniReShapeReturns returned %d values, want 1", len(out))
	}
	dummy := native.NewInteger(0)
	cases := []struct {
		name    string
		subject native.Value
	}{
		{"list subject", native.NewList([]native.Value{native.NewInteger(1)})},
		{"map subject", native.NewMap(native.NewOrderedMap())},
		{"scalar subject", native.NewString("x")},
		{"any literal subject", native.NewTypeLiteral(native.TAny)},
	}
	for _, c := range cases {
		out := miniGexShapeReturns([]native.Value{dummy, dummy, c.subject}, nil)
		if len(out) != 1 {
			t.Errorf("%s: returned %d values, want 1", c.name, len(out))
		}
	}
	// Arity guard.
	if out := miniGexShapeReturns([]native.Value{dummy}, nil); len(out) != 1 {
		t.Errorf("short args: returned %d values, want 1", len(out))
	}
}

// mcovFilterFn builds a Function value with the standard 3-param filter
// signature (src, opts, subject) for the check-mode register hooks.
func mcovFilterFn() native.Value {
	return native.NewFunction(native.FnDefInfo{
		Name: "covfn",
		Signatures: []native.FnSig{{
			Params: []native.FnParam{
				{Type: native.TString}, {Type: native.TMap}, {Type: native.TString},
			},
		}},
	})
}

// TestMiniCovRegisterTombstonesDirect drives the two frozen-registry
// tombstone handlers directly: unconditional mini_registry_frozen raises
// with the migration hints.
func TestMiniCovRegisterTombstonesDirect(t *testing.T) {
	r := mcovReg(t)
	if _, err := miniRegisterFrozenHandler(nil, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "mini_registry_frozen") {
		t.Errorf("register tombstone must raise mini_registry_frozen, got %v", err)
	}
	if _, err := miniRegisterCompiledFrozenHandler(nil, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "mini_registry_frozen") {
		t.Errorf("register-compiled tombstone must raise mini_registry_frozen, got %v", err)
	}
}

// TestMiniCovRunReDirect pins run-re's carrier guards: a non-extension
// value and an extension wrapping the wrong body are both declined.
func TestMiniCovRunReDirect(t *testing.T) {
	r := mcovReg(t)
	tMini := r.Types.MintType("CovCompiled", native.TIdeal)
	args := []native.Value{native.NewInteger(5), native.NewMap(native.NewOrderedMap()), native.NewString("x")}
	if _, err := miniRunReHandler(args, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not a compiled pattern") {
		t.Errorf("a non-extension carrier should be declined, got %v", err)
	}
	args[0] = core.NewExtension(tMini, "not-a-regexp")
	if _, err := miniRunReHandler(args, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not a compiled pattern") {
		t.Errorf("a wrong-body extension should be declined, got %v", err)
	}
}

// (The micron-literal state accessor, grammar-map validator and
// check-mode dry-pass tests died with the MiniLang.micron tombstone —
// TestMiniCovMicronTombstone pins the frozen surface.)
