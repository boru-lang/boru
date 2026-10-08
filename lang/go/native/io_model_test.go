package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Direct arm coverage for the io read/write check-mode models
// (readReturns / writeReturns in native_misc.go). The corpus drives the
// mainline shapes end to end (a made Pathon routing text / bytes /
// positioned reads); these tests pin the decline arms and the format
// table the rows cannot reach.

// modelPathon builds a concrete Pathon the way the runtime does —
// through the scalar make handler — so the model sees the same payload
// shape check mode surfaces.
func modelPathon(t *testing.T, r *Registry, path string) Value {
	t.Helper()
	out, err := core.MakeScalarHandler([]Value{NewTypeLiteral(TPathon), NewString(path)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("make Pathon %q = (%v, %v)", path, out, err)
	}
	return out[0]
}

// wantOneCarrier asserts the residual is exactly one non-dynamic
// carrier of typ.
func wantOneCarrier(t *testing.T, out []Value, typ *Type, label string) {
	t.Helper()
	if len(out) != 1 || out[0].Dynamic || !out[0].Parent.Equal(typ) {
		t.Fatalf("%s residual = %v, want one %s carrier", label, out, typ.Leaf())
	}
}

// wantDynAny asserts the model declined to the declared dynamic Any.
func wantDynAny(t *testing.T, out []Value, label string) {
	t.Helper()
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Fatalf("%s residual = %v, want one dynamic Any carrier", label, out)
	}
}

func TestReadReturnsRouting(t *testing.T) {
	r := mirrorReg(t)
	rf1 := readReturns(false)
	rf2 := readReturns(true)

	// 1-arg: extension-routed formats. txt maps to text (String);
	// an unmapped extension falls back to text; csv/tsv decode to a
	// table-shaped List; a payload-shaped format (json) declines.
	wantOneCarrier(t, rf1([]Value{modelPathon(t, r, "mem://a.txt")}, r), TString, "txt read")
	wantOneCarrier(t, rf1([]Value{modelPathon(t, r, "mem://a.bin")}, r), TString, "unmapped-ext read")
	wantOneCarrier(t, rf1([]Value{modelPathon(t, r, "mem://a.csv")}, r), TList, "csv read")
	wantDynAny(t, rf1([]Value{modelPathon(t, r, "mem://a.json")}, r), "json read")

	// 1-arg declines: nil registry, a Pathon carrier (computed target),
	// a concrete non-Pathon (reachable through union-combo expansion).
	wantDynAny(t, rf1([]Value{modelPathon(t, r, "mem://a.txt")}, nil), "nil-registry read")
	wantDynAny(t, rf1([]Value{NewCarrier(TPathon)}, r), "carrier-target read")
	wantDynAny(t, rf1([]Value{NewInteger(1)}, r), "non-Pathon read")
	wantDynAny(t, rf1(nil, r), "no-args read")

	// Opts: the binary bypass ({enc:'bytes'} or 'binary' reads Bytes),
	// the positioned slice ({offset}/{length} reads the decoded text),
	// an explicit fmt override, and the extension fallback.
	p := modelPathon(t, r, "mem://a.txt")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("enc", NewString("bytes"))}, r), TBytes, "enc bytes")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("enc", NewString("binary"))}, r), TBytes, "enc binary")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("offset", NewInteger(1))}, r), TString, "positioned offset")
	wantOneCarrier(t, rf2([]Value{p, mapOfKV("length", NewInteger(3))}, r), TString, "positioned length")
	wantOneCarrier(t, rf2([]Value{modelPathon(t, r, "mem://a.json"), mapOfKV("fmt", NewString("lines"))}, r), TList, "explicit fmt beats ext")
	wantOneCarrier(t, rf2([]Value{modelPathon(t, r, "mem://a.tsv"), NewMap(NewOrderedMap())}, r), TList, "tsv ext with empty opts")

	// Opts declines: a computed opts map (the runtime value decides the
	// format, e.g. a carrier {enc:…}), a short arg slice.
	wantDynAny(t, rf2([]Value{p, NewCarrier(TMap)}, r), "carrier opts")
	wantDynAny(t, rf2([]Value{p}, r), "missing opts")

	// The SHALLOW-CONCRETENESS decline, and the reason this model gates on
	// DeepConcrete: a map literal holding a computed field is IsConcrete,
	// but the field accessors read that field as ABSENT, so the model
	// would take the utf8 default and claim String where the run produces
	// Bytes. Caught as a live soundness violation:
	//   IO.read p {enc:e}   check: String   runtime: Bytes
	computedEnc := mapOfKV("enc", NewCarrier(TString))
	if !core.IsConcrete(computedEnc) {
		t.Fatal("fixture: a map with a carrier field must still be IsConcrete — that is the trap being guarded")
	}
	wantDynAny(t, rf2([]Value{p, computedEnc}, r), "computed enc field")
	wantDynAny(t, rf2([]Value{p, mapOfKV("offset", NewCarrier(TInteger))}, r), "computed offset field")
	wantDynAny(t, rf2([]Value{p, mapOfKV("fmt", NewDynamicCarrier(TString))}, r), "computed fmt field")
}

func TestWriteReturnsIdentity(t *testing.T) {
	r := mirrorReg(t)
	rf := writeReturns()

	// A concrete Pathon target is handed back VERBATIM — the runtime's
	// returnPath contract — so the construction stays concrete through
	// a write-then-read chain.
	p := modelPathon(t, r, "mem://w.txt")
	out := rf([]Value{p, NewString("hello")}, r)
	if len(out) != 1 || !core.IsConcrete(out[0]) {
		t.Fatalf("concrete write residual = %v, want the Pathon back", out)
	}
	if !core.ValuesEqual(out[0], p) {
		t.Fatalf("write residual = %v, want the input Pathon %v", out[0], p)
	}

	// Declines keep the declared fresh carrier: a computed target, a
	// concrete non-Pathon, a short arg slice.
	wantOneCarrier(t, rf([]Value{NewCarrier(TPathon), NewString("x")}, r), TPathon, "carrier-target write")
	wantOneCarrier(t, rf([]Value{NewInteger(1), NewString("x")}, r), TPathon, "non-Pathon write")
	wantOneCarrier(t, rf(nil, r), TPathon, "no-args write")
}

// mapOfKV builds a one-entry concrete map value.
func mapOfKV(k string, v Value) Value {
	m := NewOrderedMap()
	m.Set(k, v)
	return NewMap(m)
}

func TestStatSpaceShapeReturns(t *testing.T) {
	r := mirrorReg(t)

	// stat: a dynamic Record∪None union whose record alternative carries
	// the full buildStatRecord field set.
	out := statShapeReturns(TAtom)(nil, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("stat shape = %v, want one dynamic carrier", out)
	}
	di, err := AsDisjunct(out[0])
	if err != nil || len(di.Alternatives) != 2 {
		t.Fatalf("stat bound = (%v, %v), want a two-alternative disjunct", di, err)
	}
	rt, ok := di.Alternatives[0].Data.(core.RecordTypeInfo)
	if !ok {
		t.Fatalf("stat map alternative = %T, want a record schema", di.Alternatives[0].Data)
	}
	for _, field := range []string{"name", "path", "type", "size", "mode", "mtime", "owner", "group", "target", "xattr"} {
		if _, ok := rt.Fields.Get(field); !ok {
			t.Errorf("stat schema lacks %q", field)
		}
	}
	if n := core.TypeNodeOf(di.Alternatives[1]); n == nil || !n.Equal(TNone) {
		t.Errorf("stat second alternative = %v, want the None node", di.Alternatives[1])
	}

	// Identity discipline: two invocations mint DISTINCT values (the
	// def-slot aliasing lesson — a shared hoisted bound aliased three
	// reads onto one compiled slot).
	again := statShapeReturns(TAtom)(nil, r)
	if out[0].ID != "" && out[0].ID == again[0].ID {
		t.Error("stat shape reuses one value identity across invocations")
	}

	// space: a dynamic record schema of the filesystem-stats fields.
	sout := spaceShapeReturns()(nil, r)
	if len(sout) != 1 || !sout[0].Dynamic {
		t.Fatalf("space shape = %v, want one dynamic carrier", sout)
	}
	srt, ok := sout[0].Data.(core.RecordTypeInfo)
	if !ok {
		t.Fatalf("space bound = %T, want a record schema", sout[0].Data)
	}
	for _, field := range []string{"total", "free", "available", "bsize", "type"} {
		if _, ok := srt.Fields.Get(field); !ok {
			t.Errorf("space schema lacks %q", field)
		}
	}
}

func TestGetNodeReturnsDynamicDisjunctSchema(t *testing.T) {
	r := mirrorReg(t)

	// A dynamic Record∪None receiver resolves a schema field GRADUALLY.
	recv := statShapeReturns(TAtom)(nil, r)[0]
	out := getNodeReturns([]Value{NewString("size"), recv}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TInteger) {
		t.Fatalf("schema field through the union = %v, want dynamic Integer", out)
	}
	// A field outside the schema keeps dynamic Any (an open map at run
	// time may carry it).
	out = getNodeReturns([]Value{NewString("bogus"), recv}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Fatalf("unknown field through the union = %v, want dynamic Any", out)
	}
	// A dynamic union with NO record alternative (env's String∪None)
	// falls through unchanged.
	strUnion := NewDynamicCarrierValue(NewDisjunct([]Value{
		NewTypeLiteral(TString), NewTypeLiteral(TNone),
	}))
	out = getNodeReturns([]Value{NewString("size"), strUnion}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Fatalf("recordless union = %v, want dynamic Any", out)
	}
	// A dynamic NON-disjunct receiver (plain dynamic Any) declines the
	// probe and keeps the fallback.
	out = getNodeReturns([]Value{NewString("size"), NewDynamicCarrier(TAny)}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
		t.Fatalf("plain dynamic receiver = %v, want dynamic Any", out)
	}
}

// The boru:io guaranteed-error mirrors (exit / open / write-encoding).
// The corpus drives the flagged shapes; these own the silent arms and
// the gate declines that keep the false-positive pin at zero.
func TestIOValidationMirrors(t *testing.T) {
	r := mirrorReg(t)

	// exit: the RANGE compile failure is mirrored; a valid code is NOT — its
	// runtime raise is the boru/exit control error, which is how the
	// word works, not a program fault.
	ex := exitCodeMirror()
	ex([]Value{NewInteger(126)}, r)
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "exit_error" {
		t.Fatalf("an out-of-range exit code must flag exit_error, got %+v", r.Check.Diagnostics)
	}
	r2 := mirrorReg(t)
	ex([]Value{NewInteger(3)}, r2)
	if len(r2.Check.Diagnostics) != 0 {
		t.Fatalf("a valid exit code must stay silent, got %+v", r2.Check.Diagnostics)
	}
	// A computed code closes the gate.
	r3 := mirrorReg(t)
	ex([]Value{NewCarrier(TInteger)}, r3)
	if len(r3.Check.Diagnostics) != 0 {
		t.Fatalf("a computed exit code must not be flagged, got %+v", r3.Check.Diagnostics)
	}

	// open: an unknown mode flags; a known one and a missing opts map
	// (the 1-arg sig's shape) stay silent.
	op := openModeMirror(TAny)
	r4 := mirrorReg(t)
	op([]Value{modelPathon(t, r4, "mem://o.txt"), mapOfKV("mode", NewString("bogus"))}, r4)
	if len(r4.Check.Diagnostics) != 1 || r4.Check.Diagnostics[0].Code != "open_error" {
		t.Fatalf("an unknown open mode must flag open_error, got %+v", r4.Check.Diagnostics)
	}
	// The gate reads the OPTIONS argument, so a computed path with a
	// literal bad mode still flags — doOpenWord rejects the mode before
	// it touches the path (PR #348 review).
	rc := mirrorReg(t)
	op([]Value{NewCarrier(TPathon), mapOfKV("mode", NewString("bogus"))}, rc)
	if len(rc.Check.Diagnostics) != 1 {
		t.Fatalf("a computed path with a literal bad mode must still flag, got %+v", rc.Check.Diagnostics)
	}
	// A DYNAMIC operand anywhere closes the gate: the overload is not fixed.
	rd := mirrorReg(t)
	op([]Value{NewDynamicCarrier(TPathon), mapOfKV("mode", NewString("bogus"))}, rd)
	if len(rd.Check.Diagnostics) != 0 {
		t.Fatalf("a dynamic target must close the gate, got %+v", rd.Check.Diagnostics)
	}

	r5 := mirrorReg(t)
	op([]Value{modelPathon(t, r5, "mem://o.txt"), mapOfKV("mode", NewString("append"))}, r5)
	op([]Value{modelPathon(t, r5, "mem://o.txt")}, r5) // one arg: the gate declines before the validator
	op([]Value{modelPathon(t, r5, "mem://o.txt"), mapOfKV("mode", NewCarrier(TString))}, r5)
	if len(r5.Check.Diagnostics) != 0 {
		t.Fatalf("valid/absent/computed modes must stay silent, got %+v", r5.Check.Diagnostics)
	}

	// write: an unencodable character and an unknown encoding flag; a
	// representable one is silent, and a computed {enc:} closes the gate
	// (reading it as absent would default to utf8 and flag nothing —
	// or worse, flag a correct program).
	w := writeEncMirror(2)
	r6 := mirrorReg(t)
	p := modelPathon(t, r6, "mem://l.txt")
	w([]Value{p, NewString("€"), mapOfKV("enc", NewString("latin1"))}, r6)
	if len(r6.Check.Diagnostics) != 1 || r6.Check.Diagnostics[0].Code != "write_error" {
		t.Fatalf("an unencodable character must flag write_error, got %+v", r6.Check.Diagnostics)
	}
	r7 := mirrorReg(t)
	w([]Value{p, NewString("x"), mapOfKV("enc", NewString("bogus"))}, r7)
	if len(r7.Check.Diagnostics) != 1 {
		t.Fatalf("an unknown encoding must flag, got %+v", r7.Check.Diagnostics)
	}
	r8 := mirrorReg(t)
	w([]Value{p, NewString("hi"), mapOfKV("enc", NewString("utf16le"))}, r8)
	w([]Value{p, NewString("€"), mapOfKV("enc", NewCarrier(TString))}, r8)
	w([]Value{p, NewCarrier(TString), mapOfKV("enc", NewString("latin1"))}, r8)
	if len(r8.Check.Diagnostics) != 0 {
		t.Fatalf("representable/computed/carrier writes must stay silent, got %+v", r8.Check.Diagnostics)
	}
	// A RESERVED STREAM target declines: doWrite prints to the stream
	// and returns before the encoder, so the program runs — flagging it
	// was a false positive (PR #348 review, caught before merge).
	r9 := mirrorReg(t)
	w([]Value{modelPathon(t, r9, "<stdout>"), NewString("\u20ac"), mapOfKV("enc", NewString("latin1"))}, r9)
	w([]Value{modelPathon(t, r9, "<stderr>"), NewString("\u20ac"), mapOfKV("enc", NewString("latin1"))}, r9)
	if len(r9.Check.Diagnostics) != 0 {
		t.Fatalf("a stream-sentinel target must not be encoded-checked, got %+v", r9.Check.Diagnostics)
	}
	// A DepScalar CONSTRAINT in the content slot declines: it matches
	// TString at dispatch but carries bounds, not text, so there is
	// nothing to encode (AsConcreteString rejects it by design).
	r11 := mirrorReg(t)
	dep := core.NewDepScalar(core.DepGT, NewInteger(3))
	if !core.IsConcrete(dep) {
		t.Fatal("fixture: a DepScalar constraint must be concrete — otherwise the gate, not this arm, declines")
	}
	w([]Value{p, dep, mapOfKV("enc", NewString("latin1"))}, r11)
	if len(r11.Check.Diagnostics) != 0 {
		t.Fatalf("a constraint in the content slot must decline, got %+v", r11.Check.Diagnostics)
	}

	// A DYNAMIC target declines too: the match was optimistic, so the
	// runtime value may be a stream handle taking the other overload.
	r10 := mirrorReg(t)
	dynTarget := NewDynamicCarrier(TPathon)
	w([]Value{dynTarget, NewString("\u20ac"), mapOfKV("enc", NewString("latin1"))}, r10)
	if len(r10.Check.Diagnostics) != 0 {
		t.Fatalf("a dynamic target must close the gate, got %+v", r10.Check.Diagnostics)
	}

	// The residual still comes from writeReturns: a concrete Pathon
	// target is handed back verbatim.
	out := w([]Value{p, NewString("hi"), mapOfKV("enc", NewString("utf8"))}, r8)
	if len(out) != 1 || !core.IsConcrete(out[0]) {
		t.Fatalf("write residual = %v, want the concrete Pathon back", out)
	}
}
