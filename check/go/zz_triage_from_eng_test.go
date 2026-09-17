package check

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"strconv"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

func TestDisjunctCombosTakeSig(t *testing.T) {
	r := covRegistry(t, nil)
	implAny := &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}
	implInt := &core.BoruImpl{Body: []core.Value{core.NewInteger(2)}}
	implStr := &core.BoruImpl{Body: []core.Value{core.NewInteger(3)}}
	r.RegisterNativeFunc(core.NativeFunc{Name: "ljone", Signatures: []core.Signature{ljImplSig(implAny, core.TAny)}})
	r.RegisterNativeFunc(core.NativeFunc{Name: "ljtwo", Signatures: []core.Signature{
		ljImplSig(implInt, core.TInteger), ljImplSig(implStr, core.TString),
	}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	d := strictDisjunct(core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString))
	anySig := &core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: -1, Impl: implAny}

	if disjunctCombosTakeSig(r, "lj-no-such-word", []core.Value{d}, anySig) {
		t.Error("an unresolvable word must decline")
	}
	// 5 two-alternative disjunct args = 32 combos > the partition cap.
	wide := make([]core.Value, 5)
	for i := range wide {
		wide[i] = strictDisjunct(core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString))
	}
	if disjunctCombosTakeSig(r, "ljone", wide, anySig) {
		t.Error("a combo enumeration over the cap must decline")
	}
	// The String alternative first-matches the SIBLING String arm, not the
	// committed Integer one — one baked CALL_USER would miscompile it.
	intSig := &core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: -1, Impl: implInt}
	if disjunctCombosTakeSig(r, "ljtwo", []core.Value{d}, intSig) {
		t.Error("a combo taking a sibling overload must decline")
	}
	// Every combo lands on the sole Any arm — one recorded call is faithful.
	if !disjunctCombosTakeSig(r, "ljone", []core.Value{d}, anySig) {
		t.Error("uniform combos over the committed sig must pass")
	}
}

// Per-fn analysis quota (design/legacy/checker-accuracy-review.10.ignore A9):
// past FnAnalysisQuota distinct call shapes the analyser answers
// without body re-analysis and emits exactly one analysis_truncated
// diagnostic.
func TestFnAnalysisQuota(t *testing.T) {
	r, _ := core.NewRegistry()
	done := r.Check.Begin()
	defer done()

	body := []core.Value{core.NewInteger(1)}
	// Distinct memo keys via distinct capture names — the cheap way to
	// force a fresh analysis per call without minting types.
	for i := 0; i <= FnAnalysisQuota+5; i++ {
		caps := []core.CapturedBinding{{Name: "c" + strconv.Itoa(i), Value: core.NewCarrier(core.TInteger)}}
		AnalyseFnBody(r, "poly", nil, body, nil, caps, nil, false)
	}

	// The counter is keyed by definition site (scope + name + body
	// position), not bare name — every iteration analyses the SAME body at
	// the SAME position under a fresh capture, so they share one counter.
	polyKey := fnQuotaKey(r.AnalysisScopeID(), "poly", body)
	if got := r.Check.FnAnalysisCounts[polyKey]; got <= FnAnalysisQuota {
		t.Fatalf("expected the counter past the quota, got %d", got)
	}
	count := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "analysis_truncated" {
			count++
			if d.Severity != core.SeverityInfo {
				t.Errorf("analysis_truncated severity = %s, want info", d.Severity)
			}
		}
	}
	if count != 1 {
		t.Errorf("analysis_truncated emitted %d times, want exactly 1", count)
	}

	// Under-quota fns are never truncated (negative).
	r2, _ := core.NewRegistry()
	done2 := r2.Check.Begin()
	defer done2()
	for i := 0; i < 3; i++ {
		caps := []core.CapturedBinding{{Name: "c" + strconv.Itoa(i), Value: core.NewCarrier(core.TInteger)}}
		AnalyseFnBody(r2, "small", nil, body, nil, caps, nil, false)
	}
	for _, d := range r2.Check.Diagnostics {
		if d.Code == "analysis_truncated" {
			t.Errorf("under-quota fn was truncated: %s", d.Detail)
		}
	}
}

func TestDryPassOperandsCanonicalisesNone(t *testing.T) {
	// A strict none-SHAPE arg becomes the canonical `none` sentinel, so
	// handler payload probes (IsNone, truthiness) agree with runtime;
	// concrete args pass through untouched (same backing slice when no
	// canonicalisation is needed).
	args := []core.Value{core.NewTypeLiteral(core.TNone), core.NewInteger(1)}
	got := dryPassOperands(args)
	if !core.IsNone(got[0]) {
		t.Errorf("none literal not canonicalised: %v", got[0])
	}
	if n, err := core.AsInteger(got[1]); err != nil || n != 1 {
		t.Errorf("concrete arg disturbed: %v", got[1])
	}

	allConcrete := []core.Value{core.NewInteger(2)}
	if same := dryPassOperands(allConcrete); &same[0] != &allConcrete[0] {
		t.Error("all-concrete args should pass through without copying")
	}
}

func TestDryPassConcretenessGate(t *testing.T) {
	if allConcreteArgs([]core.Value{core.NewInteger(1), core.NewCarrier(core.TInteger)}) {
		t.Error("a bare carrier arg must fail the dry-pass gate")
	}
	dynNone := core.NewCarrier(core.TNone)
	dynNone.Dynamic = true
	if allConcreteArgs([]core.Value{dynNone}) {
		t.Error("a DYNAMIC none carrier must fail the dry-pass gate")
	}
	if !allConcreteArgs([]core.Value{core.NewInteger(1), core.NewTypeLiteral(core.TNone)}) {
		t.Error("concrete + strict none-shape args must pass the gate")
	}
}

// TestIndexProvablyOOB pins the soundness boundary: an index is flagged
// only when every value it could take is outside [0, n). Concrete
// in-bounds indices and value-less carriers must never be flagged.
func TestIndexProvablyOOB(t *testing.T) {
	const n = 2
	cases := []struct {
		name    string
		idx     core.Value
		n       int
		wantOOB bool
	}{
		// Concrete integers.
		{"past end", core.NewInteger(5), n, true},
		{"at length boundary", core.NewInteger(2), n, true},
		{"last valid", core.NewInteger(1), n, false},
		{"first", core.NewInteger(0), n, false},
		{"negative", core.NewInteger(-1), n, true},
		{"empty list, index 0", core.NewInteger(0), 0, true},
		// DepScalar lower bound → too-high detection.
		{"gte past end", core.NewDepScalar(core.DepGTE, core.NewInteger(5)), n, true},
		{"gt to boundary", core.NewDepScalar(core.DepGT, core.NewInteger(1)), n, true}, // min 2 >= 2
		{"gte in range", core.NewDepScalar(core.DepGTE, core.NewInteger(1)), n, false}, // min 1 < 2
		// DepScalar upper bound → negative detection.
		{"lt zero is negative", core.NewDepScalar(core.DepLT, core.NewInteger(0)), 5, true}, // max -1 < 0
		{"lte zero not negative", core.NewDepScalar(core.DepLTE, core.NewInteger(0)), 5, false},
		// Unknown index (value-less carrier) is never provably OOB.
		{"value-less carrier", core.NewCarrier(core.TInteger), n, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail, oob := indexProvablyOOB(tc.idx, tc.n)
			if oob != tc.wantOOB {
				t.Errorf("indexProvablyOOB(%v, %d) = %v (%q), want %v", tc.idx, tc.n, oob, detail, tc.wantOOB)
			}
			if oob && detail == "" {
				t.Errorf("flagged index produced an empty detail message")
			}
		})
	}
}

// TestValueCarriesCarrierMapArm pins the MapPayload arm of the
// interpolation bake probe's recursive walk: a plain map value whose
// ENTRY is a check-mode carrier must report true even though the map's
// own Carrier flag is false. The list arm and the top-level flag are
// exercised by the corpus lanes; a map hole reaches the walk through
// the KeyVal-list shape, so this arm needs a direct MapPayload value.
func TestValueCarriesCarrierMapArm(t *testing.T) {
	withCarrier := core.NewOrderedMap()
	withCarrier.Set("a", core.NewCarrier(core.TInteger))
	if !valueCarriesCarrier(core.NewValueRaw(core.TMap, core.MapPayload{M: withCarrier})) {
		t.Error("map holding a carrier entry must report true")
	}

	concrete := core.NewOrderedMap()
	concrete.Set("a", core.NewInteger(1))
	if valueCarriesCarrier(core.NewValueRaw(core.TMap, core.MapPayload{M: concrete})) {
		t.Error("map of concrete entries must report false")
	}
}

func TestQuoteParamCarrierBind(t *testing.T) {
	atomCarrier := core.NewCarrier(core.TAtom)
	concreteAtom := core.NewAtom("meta")

	// A carrier bound to an Atom-typed param: the diverging shape.
	if !quoteParamCarrierBind([]core.FnParam{{Name: "k", Type: core.TAtom}}, []core.Value{atomCarrier}) {
		t.Error("a carrier at an Atom-typed param must report the bind")
	}
	// The explicit Quote flag reports too.
	if !quoteParamCarrierBind([]core.FnParam{{Name: "k", Type: core.TInteger, Quote: true}}, []core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("a carrier at a /q param must report the bind")
	}
	// A CONCRETE atom (a bare-word collection's legitimate arrival) does not.
	if quoteParamCarrierBind([]core.FnParam{{Name: "k", Type: core.TAtom}}, []core.Value{concreteAtom}) {
		t.Error("a concrete atom arrival is the legitimate /q bind — no report")
	}
	// A value-typed param never reports.
	if quoteParamCarrierBind([]core.FnParam{{Name: "n", Type: core.TInteger}}, []core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("a value-typed param must not report")
	}
	// Fewer args than params: the missing slot is not a bind.
	if quoteParamCarrierBind([]core.FnParam{{Name: "k", Type: core.TAtom}}, nil) {
		t.Error("an absent arg is not a bind")
	}
}

func TestW9MintFlexShapeCarrierDeclines(t *testing.T) {
	// Depth over the cap declines.
	if _, ok := MintFlexShapeCarrier(core.NewMap(core.NewOrderedMap()), flexShapeMaxDepth+1); ok {
		t.Error("depth over the cap must decline")
	}
	// A TMap-tagged value whose payload is NOT a map payload passes the
	// concrete/conforms guards but declines at AsMap (err arm).
	badMap := core.Value{Parent: core.TMap, Data: core.ListPayload{}}
	if _, ok := MintFlexShapeCarrier(badMap, 0); ok {
		t.Error("a non-map payload must decline at AsMap")
	}
	// A non-map input declines.
	if _, ok := MintFlexShapeCarrier(core.NewInteger(1), 0); ok {
		t.Error("a non-map input must decline")
	}
}
