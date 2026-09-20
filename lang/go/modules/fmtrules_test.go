package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/formatter"
	"github.com/boru-lang/boru/lang/go/native"
)

// ruleMap builds a boru map value from alternating key/value pairs.
func ruleMap(pairs ...any) native.Value {
	m := native.NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(native.Value))
	}
	return native.NewMap(m)
}

func strList(ss ...string) native.Value {
	out := make([]native.Value, len(ss))
	for i, s := range ss {
		out[i] = native.NewString(s)
	}
	return native.NewList(out)
}

// TestFmtRulesAuthorityRoundTrip is the authority pin: the boru rendering of
// the canonical rule table, read back through valueToRules, reproduces
// Fmt.format byte-for-byte across representative shapes. The table IS the
// formatter's configuration, not documentation of it.
func TestFmtRulesAuthorityRoundTrip(t *testing.T) {
	out, err := fmtRulesHandler(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Fmt.rules: %v", err)
	}
	ru, verr := valueToRules(out[0])
	if verr != nil {
		t.Fatalf("valueToRules(rulesToValue(default)): %v", verr)
	}
	srcs := []string{
		"def x 1",
		"{a:1 # note\nb:2}",
		"def handler fn [[request:Map response:Map] [Map] [request merge response merge {status:200}]]",
		"[def alpha 100 def bravo 200 def charlie 300 def delta 400 def echo 500 def fox 600]",
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen",
		"input.(foo) a, b x ? y",
	}
	for _, src := range srcs {
		if got, want := formatter.FormatRules(src, ru), formatter.Format(src); got != want {
			t.Errorf("round-tripped rules diverge for %q:\n got: %q\nwant: %q", src, got, want)
		}
	}
}

// TestFmtRulesValueShape pins the rendered table's keys and a few load-
// bearing values, including the attach-class derivation (prev/both) and
// the next-only branch.
func TestFmtRulesValueShape(t *testing.T) {
	v := rulesToValue(formatter.DefaultRules())
	m, err := native.AsMap(v)
	if err != nil {
		t.Fatalf("rules not a map: %v", err)
	}
	wantKeys := []string{"width", "indent", "fn-word", "statement-starts", "attach", "attach-dot-suffix", "templates", "strategies"}
	keys := m.Keys()
	if len(keys) != len(wantKeys) {
		t.Fatalf("rules keys = %v, want %v", keys, wantKeys)
	}
	for i, k := range wantKeys {
		if keys[i] != k {
			t.Errorf("rules key %d = %q, want %q", i, keys[i], k)
		}
	}
	if wv, _ := m.Get("width"); func() bool { n, _ := wv.AsConcreteInteger(); return n != 72 }() {
		t.Error("width != 72")
	}
	av, _ := m.Get("attach")
	am, _ := native.AsMap(av)
	if cv, _ := am.Get("comma"); func() bool { s, _ := cv.AsConcreteString(); return s != "prev" }() {
		t.Error("attach.comma != prev")
	}
	if cv, _ := am.Get("colon"); func() bool { s, _ := cv.AsConcreteString(); return s != "both" }() {
		t.Error("attach.colon != both")
	}

	// A next-only kind renders as 'next' (not derivable from the default
	// table, whose next kinds are all also prev).
	custom := formatter.DefaultRules()
	custom.AttachPrev = nil
	custom.AttachNext = []formatter.NodeKind{formatter.NdColon}
	cv := rulesToValue(custom)
	cm, _ := native.AsMap(cv)
	cam, _ := native.AsMap(func() native.Value { x, _ := cm.Get("attach"); return x }())
	if s, _ := func() native.Value { x, _ := cam.Get("colon"); return x }().AsConcreteString(); s != "next" {
		t.Errorf("next-only colon rendered as %q, want next", s)
	}

	// A kind DUPLICATED in a Go-built AttachPrev list renders by set
	// membership — colon (also in AttachNext) stays 'both', never demoted to
	// 'prev' by the duplicate — so the behaviour round-trip contract holds
	// for duplicate-bearing tables too.
	dup := formatter.DefaultRules()
	dup.AttachPrev = append(dup.AttachPrev, formatter.NdColon)
	dm, _ := native.AsMap(rulesToValue(dup))
	dam, _ := native.AsMap(func() native.Value { x, _ := dm.Get("attach"); return x }())
	if s, _ := func() native.Value { x, _ := dam.Get("colon"); return x }().AsConcreteString(); s != "both" {
		t.Errorf("duplicated colon rendered as %q, want both", s)
	}
}

// TestFormatWithOverrides drives Fmt.format-with through the handler for
// each table dimension and checks it matches the equivalent Go-built Rules
// — the boru surface controls the same processor.
func TestFormatWithOverrides(t *testing.T) {
	run := func(rules native.Value, src string) string {
		t.Helper()
		out, err := fmtFormatWithHandler([]native.Value{rules, native.NewString(src)}, nil, nil, nil)
		if err != nil {
			t.Fatalf("format-with: %v", err)
		}
		s, _ := out[0].AsConcreteString()
		return s
	}

	// Empty table = the canonical style.
	if got, want := run(ruleMap(), "def  x  1"), formatter.Format("def  x  1"); got != want {
		t.Errorf("empty table: %q, want %q", got, want)
	}

	longFn := "def handler fn [[request:Map response:Map] [Map] [request merge response merge {status:200}]]"
	cases := []struct {
		name  string
		rules native.Value
		src   string
		want  func() string
	}{
		{"width", ruleMap("width", native.NewInteger(20)), "[alpha bravo charlie delta]", func() string {
			ru := formatter.DefaultRules()
			ru.Width = 20
			return formatter.FormatRules("[alpha bravo charlie delta]", ru)
		}},
		{"indent", ruleMap("indent", native.NewInteger(4)), "{a:1 # note\nb:2}", func() string {
			ru := formatter.DefaultRules()
			ru.Indent = 4
			return formatter.FormatRules("{a:1 # note\nb:2}", ru)
		}},
		{"fn-word", ruleMap("fn-word", native.NewString("lambda")), longFn, func() string {
			ru := formatter.DefaultRules()
			ru.FnWord = "lambda"
			return formatter.FormatRules(longFn, ru)
		}},
		{"statement-starts", ruleMap("statement-starts", strList()), "[def a 1 def b 2]", func() string {
			ru := formatter.DefaultRules()
			ru.StmtStartWords = nil
			return formatter.FormatRules("[def a 1 def b 2]", ru)
		}},
		// The src includes `x ? y`: question is ABSENT from this table, so
		// under the documented REPLACE semantics it detaches (`x ? y`), which
		// a merge-with-defaults implementation would render `x? y` — the
		// replace contract is observable, not decorative.
		{"attach", ruleMap("attach", ruleMap("colon", native.NewString("both"), "dot", native.NewString("both"), "semicolon", native.NewString("prev"), "comma", native.NewString("none"))), "a, b; c x ? y", func() string {
			ru := formatter.DefaultRules()
			ru.AttachPrev = []formatter.NodeKind{formatter.NdColon, formatter.NdDot, formatter.NdSemicolon}
			ru.AttachNext = []formatter.NodeKind{formatter.NdColon, formatter.NdDot}
			return formatter.FormatRules("a, b; c x ? y", ru)
		}},
		// The 'next' class alone: the following token glues, the kind itself
		// does not (`m : k` → `m :k`).
		{"attach next-only", ruleMap("attach", ruleMap("colon", native.NewString("next"))), "m : k", func() string {
			ru := formatter.DefaultRules()
			ru.AttachPrev = nil
			ru.AttachNext = []formatter.NodeKind{formatter.NdColon}
			return formatter.FormatRules("m : k", ru)
		}},
		{"attach-dot-suffix", ruleMap("attach-dot-suffix", native.NewBoolean(false)), "input.(foo)", func() string {
			ru := formatter.DefaultRules()
			ru.AttachDotSuffix = false
			return formatter.FormatRules("input.(foo)", ru)
		}},
		// A template override: the list template's literals become the
		// brackets; the semicolon template's literal becomes its glyph.
		{"templates", ruleMap("templates", ruleMap(
			"list", native.NewList([]native.Value{native.NewString("<"), native.NewAtom("apply"), native.NewString(">")}),
			"semicolon", native.NewList([]native.Value{native.NewString("!!")}),
		)), "[1 2] ; x", func() string {
			ru := formatter.DefaultRules()
			ru.ListOpen, ru.ListClose = "<", ">"
			g := map[formatter.NodeKind]string{}
			for k, v := range ru.Glyphs {
				g[k] = v
			}
			g[formatter.NdSemicolon] = "!!"
			ru.Glyphs = g
			return formatter.FormatRules("[1 2] ; x", ru)
		}},
		{"strategies", ruleMap("strategies", strList("inline")), "one two three four five six seven eight nine ten eleven twelve thirteen fourteen", func() string {
			ru := formatter.DefaultRules()
			ru.Strategies = []string{"inline"}
			return formatter.FormatRules("one two three four five six seven eight nine ten eleven twelve thirteen fourteen", ru)
		}},
	}
	for _, c := range cases {
		if got, want := run(c.rules, c.src), c.want(); got != want {
			t.Errorf("%s override: %q, want %q", c.name, got, want)
		}
	}
}

// TestValueToRulesRejects pins every validation branch: each malformed
// table is declined with a message naming the offending field — never a
// silent no-op, never a panic.
func TestValueToRulesRejects(t *testing.T) {
	bad := []struct {
		name string
		v    native.Value
		want string
	}{
		{"non-map", native.NewInteger(5), "fmt format-with"},
		{"type-literal map", native.NewTypeLiteral(native.TMap), "fmt format-with"},
		{"width not integer", ruleMap("width", native.NewString("x")), "width must be an Integer"},
		{"indent not integer", ruleMap("indent", native.NewString("x")), "indent must be an Integer"},
		{"width out of range", ruleMap("width", native.NewInteger(1<<40)), "width 1099511627776 out of range"},
		{"indent out of range", ruleMap("indent", native.NewInteger(-99999)), "indent -99999 out of range"},
		{"fn-word not string", ruleMap("fn-word", native.NewInteger(1)), "fn-word must be a String"},
		{"statement-starts not list", ruleMap("statement-starts", native.NewInteger(1)), "statement-starts must be a List"},
		{"statement-starts elem", ruleMap("statement-starts", native.NewList([]native.Value{native.NewInteger(1)})), "statement-starts[0] must be a String"},
		{"attach not map", ruleMap("attach", native.NewInteger(1)), "attach must be a Map"},
		{"attach unknown kind", ruleMap("attach", ruleMap("zzz", native.NewString("prev"))), "unknown node kind"},
		{"attach class not string", ruleMap("attach", ruleMap("comma", native.NewInteger(1))), "attach.comma must be a String"},
		{"attach unknown class", ruleMap("attach", ruleMap("comma", native.NewString("glue"))), "unknown class"},
		{"attach-dot-suffix not bool", ruleMap("attach-dot-suffix", native.NewInteger(1)), "attach-dot-suffix must be a Boolean"},
		{"templates not map", ruleMap("templates", native.NewInteger(1)), "templates must be a Map"},
		{"templates unknown kind", ruleMap("templates", ruleMap("tuple", native.NewList([]native.Value{}))), "unknown node kind"},
		{"template body not list", ruleMap("templates", ruleMap("list", native.NewInteger(1))), "templates.list must be a List"},
		{"template wrong op", ruleMap("templates", ruleMap("list", native.NewList([]native.Value{native.NewAtom("entries")}))), "templates.list needs the apply/q op"},
		{"template part not literal", ruleMap("templates", ruleMap("comma", native.NewList([]native.Value{native.NewInteger(1)}))), "must be a String literal or an op atom"},
		{"template part atom literal", ruleMap("templates", ruleMap("list", native.NewList([]native.Value{native.NewTypeLiteral(native.TAtom)}))), "templates.list[0]"},
		{"strategies not list", ruleMap("strategies", native.NewInteger(1)), "strategies must be a List"},
		{"strategies elem", ruleMap("strategies", native.NewList([]native.Value{native.NewInteger(1)})), "strategies[0] must be a String"},
		{"unknown strategy", ruleMap("strategies", strList("bogus")), "unknown strategy"},
		{"unknown key", ruleMap("widht", native.NewInteger(72)), "unknown rule key"},
	}
	for _, c := range bad {
		_, err := valueToRules(c.v)
		if err == nil {
			t.Errorf("%s: accepted, want error containing %q", c.name, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not mention %q", c.name, err.Error(), c.want)
		}
	}
}

// TestBuildFmtModuleStylesheetError pins the surfaced-at-construction
// path: a recorded stylesheet init error (the embedded fmt-rules.boru
// failing to parse — a build defect) makes BuildFmtModule decline loudly
// instead of formatting with a zero table. Uses the formatter's test seam
// (SwapDefaultRulesErr), mirroring the native.TypeInitError pattern.
func TestBuildFmtModuleStylesheetError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	prev := formatter.SwapDefaultRulesErr(errStylesheet)
	defer formatter.SwapDefaultRulesErr(prev)
	if _, berr := BuildFmtModule(reg); berr == nil {
		t.Fatal("BuildFmtModule accepted a broken stylesheet; want error")
	}
}

var errStylesheet = &stylesheetErr{}

type stylesheetErr struct{}

func (*stylesheetErr) Error() string { return "stylesheet parse failed" }

// TestFormatWithHandlerErrors pins the handler-level rejections: a bad rule
// table and a non-concrete source string both error rather than panic.
func TestFormatWithHandlerErrors(t *testing.T) {
	if _, err := fmtFormatWithHandler([]native.Value{ruleMap("widht", native.NewInteger(1)), native.NewString("a")}, nil, nil, nil); err == nil {
		t.Error("bad rule table accepted")
	}
	if _, err := fmtFormatWithHandler([]native.Value{ruleMap(), native.NewTypeLiteral(native.TString)}, nil, nil, nil); err == nil {
		t.Error("non-concrete source accepted")
	}
}
