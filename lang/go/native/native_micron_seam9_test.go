package native

import (
	"strings"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for native_micron.go: the Micron
// property-access handlers (get/getr/has on type-literal receivers) and
// the check-mode narrowing ReturnsFn getMicronReturns, driven directly
// with crafted carrier/concrete receivers.

func TestW9MicronHandlerTypeLiteralReceiver(t *testing.T) {
	r := seam5Reg(t)
	lit := NewTypeLiteral(TMicron)

	// getMicronHandler: type-literal receiver rejected (24.23,26.3).
	if _, err := getMicronHandler([]Value{NewString("x"), lit}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "type literal") {
		t.Fatalf("getMicronHandler: expected type-literal error, got %v", err)
	}
	// getrMicronHandler: type-literal receiver rejected (39.23,41.3).
	if _, err := getrMicronHandler([]Value{NewString("x"), lit}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "type literal") {
		t.Fatalf("getrMicronHandler: expected type-literal error, got %v", err)
	}
	// hasMicronHandler: type-literal receiver → false (52.23,54.3).
	out, err := hasMicronHandler([]Value{NewString("x"), lit}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("hasMicronHandler: %v / %v", out, err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Fatalf("hasMicronHandler type-literal: expected false, got true")
	}
}

func TestW9GetMicronReturnsBadArgs(t *testing.T) {
	r := seam5Reg(t)
	// Wrong arity → dynamic fallback (114.69,116.3).
	out := getMicronReturns([]Value{NewString("only-one")}, r)
	if len(out) != 1 || !IsDynamic(&out[0]) {
		t.Fatalf("getMicronReturns bad args: expected dynamic carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsConcreteReceiver(t *testing.T) {
	r := seam5Reg(t)
	// A concrete Emailon instance.
	res, err := seam5Run(r, `make Emailon 'alice@example.com'`)
	if err != nil || len(res) == 0 {
		t.Fatalf("make Emailon: %v / %v", res, err)
	}
	email := res[len(res)-1]

	// Present property "user": narrows to the field's carrier type
	// (119.22 → 121.39 → NewCarrier(ft)).
	out := getMicronReturns([]Value{NewString("user"), email}, r)
	if len(out) != 1 || IsDynamic(&out[0]) {
		t.Fatalf("getMicronReturns user: expected concrete carrier, got %v", out)
	}

	// Absent property → NewCarrier(TNone) (126.3,126.36).
	out = getMicronReturns([]Value{NewString("nope"), email}, r)
	if len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("getMicronReturns absent: expected None carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsCarrierDefaults(t *testing.T) {
	r := seam5Reg(t)

	// Pathon carrier, key that is neither "parts" nor "abs" → None (137.3,137.36).
	out := getMicronReturns([]Value{NewString("zzz"), NewCarrier(TPathon)}, r)
	if len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("pathon carrier default: expected None carrier, got %v", out)
	}

	// Urlon carrier, unknown key → None (152.3,152.36).
	out = getMicronReturns([]Value{NewString("zzz"), NewCarrier(TUrlon)}, r)
	if len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("urlon carrier default: expected None carrier, got %v", out)
	}

	// Ipon carrier: typed fields narrow, unknown key → None (160.3,160.36).
	if out = getMicronReturns([]Value{NewString("addr"), NewCarrier(TIpon)}, r); len(out) != 1 || !out[0].Parent().Equal(TString) {
		t.Fatalf("ipon addr: expected String carrier, got %v", out)
	}
	if out = getMicronReturns([]Value{NewString("version"), NewCarrier(TIpon)}, r); len(out) != 1 || !out[0].Parent().Equal(TInteger) {
		t.Fatalf("ipon version: expected Integer carrier, got %v", out)
	}
	if out = getMicronReturns([]Value{NewString("zzz"), NewCarrier(TIpon)}, r); len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("ipon carrier default: expected None carrier, got %v", out)
	}

	// Hoston carrier: host/authority narrow to String, port stays dynamic,
	// unknown key → None (168.3,168.36).
	if out = getMicronReturns([]Value{NewString("authority"), NewCarrier(THoston)}, r); len(out) != 1 || !out[0].Parent().Equal(TString) {
		t.Fatalf("hoston authority: expected String carrier, got %v", out)
	}
	if out = getMicronReturns([]Value{NewString("port"), NewCarrier(THoston)}, r); len(out) != 1 || !IsDynamic(&out[0]) {
		t.Fatalf("hoston port: expected dynamic carrier, got %v", out)
	}
	if out = getMicronReturns([]Value{NewString("zzz"), NewCarrier(THoston)}, r); len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("hoston carrier default: expected None carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsSemveronCarrier(t *testing.T) {
	r := seam5Reg(t)
	sv := NewCarrier(TSemveron)
	// Each key narrows from the static per-kind field table.
	intKeys := []string{"major", "minor", "patch"}
	for _, k := range intKeys {
		out := getMicronReturns([]Value{NewString(k), sv}, r)
		if len(out) != 1 || !out[0].Parent().Equal(TInteger) {
			t.Fatalf("semveron carrier %q: expected Integer carrier, got %v", k, out)
		}
	}
	for _, k := range []string{"release", "version"} {
		out := getMicronReturns([]Value{NewString(k), sv}, r)
		if len(out) != 1 || !out[0].Parent().Equal(TString) {
			t.Fatalf("semveron carrier %q: expected String carrier, got %v", k, out)
		}
	}
	if out := getMicronReturns([]Value{NewString("stable"), sv}, r); len(out) != 1 || !out[0].Parent().Equal(TBoolean) {
		t.Fatalf("semveron carrier stable: expected Boolean carrier, got %v", out)
	}
	for _, k := range []string{"prereleaseParts", "buildParts"} {
		out := getMicronReturns([]Value{NewString(k), sv}, r)
		if len(out) != 1 || !out[0].Parent().ConformsTo(TList) {
			t.Fatalf("semveron carrier %q: expected List carrier, got %v", k, out)
		}
	}
	// Optional per-instance fields stay dynamic.
	for _, k := range []string{"prerelease", "build"} {
		out := getMicronReturns([]Value{NewString(k), sv}, r)
		if len(out) != 1 || !IsDynamic(&out[0]) {
			t.Fatalf("semveron carrier %q: expected dynamic carrier, got %v", k, out)
		}
	}
	// Unknown key → None.
	if out := getMicronReturns([]Value{NewString("zzz"), sv}, r); len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("semveron carrier default: expected None carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsNewLeafCarriers(t *testing.T) {
	r := seam5Reg(t)
	// Each new leaf's carrier receiver narrows typed keys and defaults an
	// unknown key to None (covers the per-leaf carrier switch arms).
	check := func(kind *Value, key string, want *Value) {
		out := getMicronReturns([]Value{NewString(key), NewCarrier(kind)}, r)
		if len(out) != 1 || !out[0].Parent().Equal(want) {
			t.Fatalf("%s.%s: expected %s carrier, got %v", kind.Name(), key, want.Name(), out)
		}
	}
	checkDyn := func(kind *Value, key string) {
		out := getMicronReturns([]Value{NewString(key), NewCarrier(kind)}, r)
		if len(out) != 1 || !IsDynamic(&out[0]) {
			t.Fatalf("%s.%s: expected dynamic carrier, got %v", kind.Name(), key, out)
		}
	}
	checkNone := func(kind *Value) {
		out := getMicronReturns([]Value{NewString("zzz"), NewCarrier(kind)}, r)
		if len(out) != 1 || !out[0].Parent().Equal(TNone) {
			t.Fatalf("%s default: expected None carrier, got %v", kind.Name(), out)
		}
	}

	check(TCidron, "cidr", TString)
	check(TCidron, "prefix", TInteger)
	checkDyn(TCidron, "count")
	checkNone(TCidron)

	check(TMacon, "oui", TString)
	check(TMacon, "bits", TInteger)
	check(TMacon, "eui64", TBoolean)
	checkNone(TMacon)

	check(TColoron, "r", TInteger)
	check(TColoron, "hex", TString)
	check(TColoron, "opaque", TBoolean)
	check(TColoron, "alpha", TFloat)
	checkNone(TColoron)

	check(TMimon, "essence", TString)
	checkDyn(TMimon, "params")
	checkNone(TMimon)

	check(TQion, "code", TString)
	check(TQion, "amount", TBigDecimal)
	check(TQion, "minor", TInteger)
	check(TQion, "negative", TBoolean)
	checkDyn(TQion, "units")
	checkNone(TQion)

	check(TPhonon, "e164", TString)
	check(TPhonon, "length", TInteger)
	checkNone(TPhonon)
}

func TestW9GetMicronReturnsUserSchema(t *testing.T) {
	r := seam5Reg(t)
	// Define a user Micron kind carrying a schema, then obtain its type
	// literal so we can build a carrier receiver.
	res, err := seam5Run(r, `def Baron refine Micron {foo:String}
Baron`)
	if err != nil || len(res) == 0 {
		t.Fatalf("def Baron: %v / %v", res, err)
	}
	// The name evaluates to the minted NODE (the Stage 2 flip), so the
	// carrier's type is the node itself, not the result's Parent.
	baronLit := res[len(res)-1]
	baronCarrier := NewCarrier(NodeOf(&baronLit))

	// Key not in the schema → None (162.3,162.36).
	out := getMicronReturns([]Value{NewString("missing"), baronCarrier}, r)
	if len(out) != 1 || !out[0].Parent().Equal(TNone) {
		t.Fatalf("user schema key-miss: expected None carrier, got %v", out)
	}

	// A schema field typed Function narrows to dynamic (157.70,159.5):
	// callable fields can't be statically carried.
	res, err = seam5Run(r, `def Funcon refine Micron {doit:Function}
Funcon`)
	if err != nil || len(res) == 0 {
		t.Fatalf("def Funcon: %v / %v", res, err)
	}
	funLit := res[len(res)-1]
	funCarrier := NewCarrier(NodeOf(&funLit))
	out = getMicronReturns([]Value{NewString("doit"), funCarrier}, r)
	if len(out) != 1 || !IsDynamic(&out[0]) {
		t.Fatalf("user schema function-field: expected dynamic carrier, got %v", out)
	}
}
