package modules

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// w8Fmt is a minimal test Format: Decode returns preset output/err.
type w8Fmt struct {
	out    []native.Value
	decErr error
}

func (f w8Fmt) Decode(_ string) ([]native.Value, error) { return f.out, f.decErr }
func (f w8Fmt) Encode(_ native.Value) (string, error)   { return "", nil }

// w8OptFmt additionally implements DecodeOpter.
type w8OptFmt struct{ w8Fmt }

func (f w8OptFmt) DecodeOpts(_ string, _ map[string]any) ([]native.Value, error) {
	return f.out, f.decErr
}

// TestW8FormatParseHandler drives formatParseHandler's src (297), DecodeOpter
// (302), decode-error (307) and empty-output (312) arms.
func TestW8FormatParseHandler(t *testing.T) {
	r := mcovReg(t)
	opts := s7bMap()

	// 297: non-concrete src.
	h := formatParseHandler("x", w8Fmt{})
	if _, err := h([]native.Value{w8LitS(), opts}, nil, nil, r); err == nil {
		t.Error("formatParseHandler: non-concrete src should error")
	}

	// 302: the DecodeOpter branch (happy).
	ho := formatParseHandler("x", w8OptFmt{w8Fmt{out: []native.Value{native.NewInteger(7)}}})
	out, err := ho([]native.Value{native.NewString("s"), opts}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("DecodeOpts path: got %v, %v", out, err)
	}

	// 307: decode error.
	he := formatParseHandler("x", w8Fmt{decErr: errors.New("boom")})
	if _, err := he([]native.Value{native.NewString("s"), opts}, nil, nil, r); err == nil {
		t.Error("formatParseHandler: decode error should propagate")
	}

	// 312: empty output → None.
	hn := formatParseHandler("x", w8Fmt{out: nil})
	out, err = hn([]native.Value{native.NewString("s"), opts}, nil, nil, r)
	if err != nil || len(out) != 1 || !native.IsNoneShape(out[0]) {
		t.Errorf("formatParseHandler: empty decode should yield None, got %v (%v)", out, err)
	}
}

// TestW8InstallBuiltinParserDuplicate drives installBuiltinParser's
// duplicate-key compile failure — defensive: the fixed built-in set is disjoint, so
// only a drifted TabnasKinds could collide.
func TestW8InstallBuiltinParserDuplicate(t *testing.T) {
	r := mcovReg(t)
	exports := native.NewOrderedMap()
	spec := ParseLangSpec{Name: "dupw8", Handler: func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	}}
	if err := installBuiltinParser(exports, r, spec); err != nil {
		t.Fatalf("first install should succeed: %v", err)
	}
	if err := installBuiltinParser(exports, r, spec); err == nil {
		t.Error("second install of the same name should error")
	}
}

// TestW8PureParseFoldFallback drives pureParseFoldReturns's fallback arm (378):
// a non-2-arg call yields declared-Returns carriers.
func TestW8PureParseFoldFallback(t *testing.T) {
	r := mcovReg(t)
	fn := pureParseFoldReturns([]*native.Type{native.TAny}, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	out := fn([]native.Value{native.NewInteger(1)}, r) // wrong arity → fallback
	if len(out) != 1 {
		t.Fatalf("fallback should carry one declared return, got %v", out)
	}
}

// TestW8ParseFoldableValue drives parseFoldableValue's carrier / list / map arms.
func TestW8ParseFoldableValue(t *testing.T) {
	// 399: a carrier is never foldable.
	if parseFoldableValue(native.NewCarrier(native.TAny)) {
		t.Error("a carrier is not foldable")
	}
	// list happy (403/408/409/413).
	if !parseFoldableValue(native.NewList([]native.Value{native.NewInteger(1), native.NewString("x")})) {
		t.Error("a concrete scalar list is foldable")
	}
	// list with a non-foldable element (409 → return false).
	if parseFoldableValue(native.NewList([]native.Value{native.NewCarrier(native.TAny)})) {
		t.Error("a list with a carrier element is not foldable")
	}
	// map happy (416 → loop → 425 return true).
	mOK := native.NewOrderedMap()
	mOK.Set("a", native.NewInteger(1))
	if !parseFoldableValue(native.NewMap(mOK)) {
		t.Error("a concrete scalar map is foldable")
	}
	// map with a non-foldable value (421 → return false).
	mBad := native.NewOrderedMap()
	mBad.Set("a", native.NewCarrier(native.TAny))
	if parseFoldableValue(native.NewMap(mBad)) {
		t.Error("a map with a carrier value is not foldable")
	}
	// list-typed value whose payload AsList declines (405 → return false).
	if parseFoldableValue(core.NewExtension(native.TList, "w8")) {
		t.Error("a list value AsList cannot read is not foldable")
	}
	// map-typed value whose payload AsMap declines (416 → return false).
	if parseFoldableValue(core.NewExtension(native.TMap, "w8")) {
		t.Error("a map value AsMap cannot read is not foldable")
	}
}

// TestW8ResolveParseSourceBad drives resolveParseSource's non-String/non-map
// compile failure (456) and its empty-map arm (441): a TMap-typed value AsMap declines.
func TestW8ResolveParseSourceBad(t *testing.T) {
	r := mcovReg(t)
	if _, err := resolveParseSource(native.NewInteger(5), r); err == nil {
		t.Error("resolveParseSource: an Integer source should error")
	}
	// 441: a concrete TMap-typed value AsMap reads as nil.
	if _, err := resolveParseSource(core.NewExtension(native.TMap, "w8"), r); err == nil {
		t.Error("resolveParseSource: an unreadable source map should error")
	}
}

// TestW8NewParseLangFnValidation drives the value constructor's validation
// arms: empty name, nil handler, and a name RegisterNativeFunc declines.
func TestW8NewParseLangFnValidation(t *testing.T) {
	if _, err := NewParseLangFn(ParseLangSpec{Name: "", Handler: w9NopParser}); err == nil {
		t.Error("NewParseLangFn: empty Name should error")
	}
	if _, err := NewParseLangFn(ParseLangSpec{Name: "x"}); err == nil {
		t.Error("NewParseLangFn: nil Handler should error")
	}
	if _, err := NewParseLangFn(ParseLangSpec{Name: "not a word", Handler: w9NopParser}); err == nil {
		t.Error("NewParseLangFn: an invalid word name should error")
	}
	if _, err := NewParseLangFn(ParseLangSpec{Name: "multi", Handler: w9NopParser,
		Returns: []*native.Type{native.TAny, native.TAny}}); err == nil {
		t.Error("NewParseLangFn: a multi-return spec should error (a parser yields one result)")
	}
	if v, err := NewFormatParserFn("w8fmt", w8Fmt{}); err != nil || !native.IsConcrete(v) {
		t.Errorf("NewFormatParserFn should build a concrete fn value, got %v (err %v)", v, err)
	}
	if v, err := NewParseLangFn(ParseLangSpec{Name: "purew8", Handler: w9NopParser, Pure: true}); err != nil || !native.IsConcrete(v) {
		t.Errorf("NewParseLangFn Pure should build a concrete fn value, got %v (err %v)", v, err)
	}
}

// TestW8NewParseLangFnRegistryError drives both value constructors'
// sub-registry construction arms through the newDefaultRegistry seam.
func TestW8NewParseLangFnRegistryError(t *testing.T) {
	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	newDefaultRegistry = func(_ ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errors.New("w8 registry boom")
	}
	if _, err := NewParseLangFn(ParseLangSpec{Name: "x", Handler: w9NopParser}); err == nil {
		t.Error("NewParseLangFn: a registry construction error should propagate")
	}
	if _, err := NewMiniLangFn(MiniLangSpec{Name: "x", Handler: w9NopParser}); err == nil {
		t.Error("NewMiniLangFn: a registry construction error should propagate")
	}
}

// TestW8RegisterTombstone drives the ParseLang.register tombstone directly:
// an unconditional parse_registry_frozen raise with the migration hint.
func TestW8RegisterTombstone(t *testing.T) {
	r := mcovReg(t)
	_, err := parseRegisterFrozenHandler(nil, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "parse_registry_frozen") {
		t.Errorf("register tombstone must raise parse_registry_frozen, got %v", err)
	}
}

// (parselang-deferred-dispatch was deleted with Parse.register: grammar
// parsers are Parse.parser Function VALUES dispatched through
// parselang-fn-dispatch, whose arms TestFnDispatchArms drives.)

// TestW8AontuAndTabnasSrcErrors drives the src decline arms of the aontu (666)
// and tabnas (687) built-in parser handlers.
func TestW8AontuAndTabnasSrcErrors(t *testing.T) {
	r := mcovReg(t)
	opts := s7bMap()

	aontu := aontuParserSpec()
	if _, err := aontu.Handler([]native.Value{w8LitS(), opts}, nil, nil, r); err == nil {
		t.Error("aontu handler: non-concrete src should error")
	}

	specs := tabnasParserSpecs()
	if len(specs) == 0 {
		t.Fatal("expected built-in tabnas parser specs")
	}
	if _, err := specs[0].Handler([]native.Value{w8LitS(), opts}, nil, nil, r); err == nil {
		t.Error("tabnas handler: non-concrete src should error")
	}
}
