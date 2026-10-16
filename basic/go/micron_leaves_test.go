package basic

import (
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// TestMicronNewtypeLeaves covers the newtype (bare-refine) construction
// path for every new builtin leaf: minting a subtype and instantiating it
// constructs the base then retags, walking micronInstantiate's newtype
// loop.
func TestMicronNewtypeLeaves(t *testing.T) {
	r := newTestRegistry(t)
	cases := []struct {
		base *Type
		in   string
	}{
		{TCidron, "10.0.0.0/8"},
		{TMacon, "00:1a:2b:3c:4d:5e"},
		{TColoron, "#ff0000"},
		{TMimon, "text/html"},
		{TQion, "USD 12.50"},
		{TPhonon, "+14155552671"},
	}
	for _, c := range cases {
		newt := r.Types.MintType("W4b"+c.base.Name(), c.base)
		out, err := micronInstantiate(NewTypeLiteral(newt), NewString(c.in), r)
		if err != nil {
			t.Fatalf("%s newtype make: %v", c.base.Name(), err)
		}
		if !ParentOf(&out[0]).Equal(newt) {
			t.Errorf("%s newtype parent = %v, want %s", c.base.Name(), ParentOf(&out[0]), newt.Name())
		}
	}
}

// mkLeaf constructs a builtin Micron leaf from a string and fails the
// test on error.
func mkLeaf(t *testing.T, kind *Type, make func(Value) ([]Value, error), s string) Value {
	t.Helper()
	out, err := make(NewString(s))
	if err != nil {
		t.Fatalf("make %s %q: %v", kind.Name(), s, err)
	}
	if !ParentOf(&out[0]).Equal(kind) {
		t.Fatalf("make %s %q: parent = %v", kind.Name(), s, ParentOf(&out[0]))
	}
	return out[0]
}

func prop(t *testing.T, v Value, key string) Value {
	t.Helper()
	p, ok := MicronProperty(v, key)
	if !ok {
		t.Fatalf("%s: no property %q", ParentOf(&v).Name(), key)
	}
	return p
}

// ---- Cidron ----

func TestMakeCidron(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.0/8":         "10.0.0.0/8",
		"10.0.0.5/8":         "10.0.0.0/8",
		"192.168.1.100/24":   "192.168.1.0/24",
		"2001:0DB8::/32":     "2001:db8::/32",
		"::ffff:1.2.3.0/120": "::ffff:1.2.3.0/120",
		"0.0.0.0/0":          "0.0.0.0/0",
	} {
		v := mkLeaf(t, TCidron, makeCidron, in)
		if v.String() != want {
			t.Errorf("cidron %q → %q, want %q", in, v.String(), want)
		}
	}
	// Properties.
	v := mkLeaf(t, TCidron, makeCidron, "192.168.1.0/24")
	if n, _ := AsInteger(prop(t, v, "prefix")); n != 24 {
		t.Errorf("prefix = %d", n)
	}
	if n, _ := AsInteger(prop(t, v, "version")); n != 4 {
		t.Errorf("version = %d", n)
	}
	if n, _ := AsInteger(prop(t, v, "hostbits")); n != 8 {
		t.Errorf("hostbits = %d", n)
	}
	if n, _ := AsInteger(prop(t, v, "count")); n != 256 {
		t.Errorf("count = %d", n)
	}
	if n, _ := AsInteger(prop(t, mkLeaf(t, TCidron, makeCidron, "2001:db8::/32"), "version")); n != 6 {
		t.Errorf("v6 version = %d", n)
	}
	// count for a v6 block overflows int64 → BigInteger.
	if bc := prop(t, mkLeaf(t, TCidron, makeCidron, "2001:db8::/0"), "count"); !ParentOf(&bc).Equal(TBigInteger) {
		t.Errorf("v6 /0 count type = %v", ParentOf(&bc))
	}
	if _, ok := MicronProperty(v, "nope"); ok {
		t.Error("unknown cidron prop returned ok")
	}
	// Map form + negatives.
	if out, err := makeCidron(mapOf("addr", NewString("10.0.0.0"), "prefix", NewInteger(8))); err != nil || out[0].String() != "10.0.0.0/8" {
		t.Errorf("map cidron = %v (%v)", out, err)
	}
	if out, err := makeCidron(mapOf("addr", NewString("10.0.0.0"), "prefix", NewInteger(8), "version", NewInteger(4))); err != nil || out[0].String() != "10.0.0.0/8" {
		t.Errorf("map cidron w/version = %v (%v)", out, err)
	}
	for _, bad := range []Value{
		NewString("10.0.0.0/33"), NewString("2001:db8::/129"), NewString("10.0.0.0"),
		NewString("192.168.001.000/24"), NewString("not-a-cidr"), NewInteger(42),
		mapOf("addr", NewString("10.0.0.0"), "prefix", NewInteger(8), "extra", NewString("y")),
		mapOf("prefix", NewInteger(8)),
		mapOf("addr", NewInteger(1), "prefix", NewInteger(8)),
		mapOf("addr", NewString("10.0.0.0"), "prefix", NewString("8")),
		mapOf("addr", NewString("10.0.0.0"), "prefix", NewInteger(8), "version", NewString("4")),
		MakeValue(TMap, ListPayload{}),
	} {
		if _, err := makeCidron(bad); err == nil {
			t.Errorf("bad cidron %v accepted", bad)
		}
	}
	// Ordering: family, address, prefix.
	lt := func(a, b string) {
		if compareCidrons(mkLeaf(t, TCidron, makeCidron, a), mkLeaf(t, TCidron, makeCidron, b)) != -1 {
			t.Errorf("cidron %q < %q failed", a, b)
		}
	}
	lt("10.0.0.0/8", "10.0.0.0/16")
	lt("10.0.0.0/8", "10.0.1.0/24")
	lt("10.0.0.0/8", "2001:db8::/32")
	if compareCidrons(mkLeaf(t, TCidron, makeCidron, "10.0.0.0/8"), mkLeaf(t, TCidron, makeCidron, "10.0.0.0/8")) != 0 {
		t.Error("equal cidron cmp != 0")
	}
	if compareCidrons(NewInteger(1), NewInteger(2)) == 0 {
		t.Error("non-payload cidron fallback returned equal")
	}
}

// ---- Macon ----

func TestMakeMacon(t *testing.T) {
	for _, in := range []string{"00:1A:2B:3C:4D:5E", "00-1a-2b-3c-4d-5e", "001A.2B3C.4D5E", "001a2b3c4d5e"} {
		if v := mkLeaf(t, TMacon, makeMacon, in); v.String() != "00:1a:2b:3c:4d:5e" {
			t.Errorf("macon %q → %q", in, v.String())
		}
	}
	v := mkLeaf(t, TMacon, makeMacon, "00:1a:2b:3c:4d:5e")
	if n, _ := AsInteger(prop(t, v, "bits")); n != 48 {
		t.Errorf("bits = %d", n)
	}
	if bo, _ := AsBoolean(prop(t, v, "eui64")); bo {
		t.Error("48-bit reported eui64")
	}
	if s, _ := AsString(prop(t, v, "oui")); s != "00:1a:2b" {
		t.Errorf("oui = %q", s)
	}
	if bo, _ := AsBoolean(prop(t, v, "local")); bo {
		t.Error("local should be false")
	}
	e := mkLeaf(t, TMacon, makeMacon, "00:1a:2b:3c:4d:5e:6f:70")
	if bo, _ := AsBoolean(prop(t, e, "eui64")); !bo {
		t.Error("8-octet not eui64")
	}
	bc := mkLeaf(t, TMacon, makeMacon, "ff:ff:ff:ff:ff:ff")
	if bo, _ := AsBoolean(prop(t, bc, "broadcast")); !bo {
		t.Error("ff... not broadcast")
	}
	if bo, _ := AsBoolean(prop(t, bc, "multicast")); !bo {
		t.Error("ff... not multicast")
	}
	if bo, _ := AsBoolean(prop(t, v, "broadcast")); bo {
		t.Error("non-ff reported broadcast")
	}
	if _, ok := MicronProperty(v, "nope"); ok {
		t.Error("unknown macon prop ok")
	}
	if out, err := makeMacon(mapOf("addr", NewString("00:1a:2b:3c:4d:5e"))); err != nil || out[0].String() != "00:1a:2b:3c:4d:5e" {
		t.Errorf("map macon = %v (%v)", out, err)
	}
	for _, bad := range []Value{
		NewString("00:1A:2B:3C:4D"), NewString("0:1:2:3:4:5"), NewString("GG:1A:2B:3C:4D:5E"),
		NewString("00gg2b3c4d5e"), // 12 chars, non-hex → not normalized, ParseMAC rejects
		NewString("00:00:00:00:fe:80:00:00:00:00:00:00:02:00:5e:10:00:00:00:01"), // 20-octet InfiniBand
		NewInteger(42), mapOf("x", NewString("y")), mapOf("addr", NewInteger(1)),
		mapOf(), MakeValue(TMap, ListPayload{}),
	} {
		if _, err := makeMacon(bad); err == nil {
			t.Errorf("bad macon %v accepted", bad)
		}
	}
	if maconAllOnes(nil) {
		t.Error("empty hw reported all-ones")
	}
}

// ---- Coloron ----

func TestMakeColoron(t *testing.T) {
	for in, want := range map[string]string{
		"#f00":                   "#ff0000",
		"#FF0000":                "#ff0000",
		"#abc8":                  "#aabbcc88",
		"#11223344":              "#11223344",
		"rgb(255, 0, 0)":         "#ff0000",
		"rgb(255 0 0 / 0.5)":     "#ff000080",
		"rgba(0, 128, 255, 0.8)": "#0080ffcc",
		"rgb(50%, 100%, 0%)":     "#80ff00",
		"rgb(none 0 0)":          "#000000",
		"rgb(0 0 0 / none)":      "#00000000", // none alpha → 0 → 8-digit
		"rgb(300 -5 0)":          "#ff0000",   // clamped
		"rgba(0 0 0 / 50%)":      "#00000080",
		"rgb(255 0 0 / 50%)":     "#ff000080", // percentage alpha
		"  #f00  ":               "#ff0000",   // trimmed
	} {
		if v := mkLeaf(t, TColoron, makeColoron, in); v.String() != want {
			t.Errorf("coloron %q → %q, want %q", in, v.String(), want)
		}
	}
	v := mkLeaf(t, TColoron, makeColoron, "#00000080")
	if n, _ := AsInteger(prop(t, v, "a")); n != 128 {
		t.Errorf("a = %d", n)
	}
	if bo, _ := AsBoolean(prop(t, v, "opaque")); bo {
		t.Error("alpha<255 reported opaque")
	}
	if f, _ := AsFloat(prop(t, v, "alpha")); f < 0.50 || f > 0.51 {
		t.Errorf("alpha = %v", f)
	}
	if s, _ := AsString(prop(t, v, "css")); s != "rgb(0 0 0 / 0.5019607843137255)" {
		t.Errorf("css = %q", s)
	}
	op := mkLeaf(t, TColoron, makeColoron, "#ff0000")
	if s, _ := AsString(prop(t, op, "css")); s != "rgb(255 0 0)" {
		t.Errorf("opaque css = %q", s)
	}
	if s, _ := AsString(prop(t, op, "hex")); s != "#ff0000" {
		t.Errorf("hex = %q", s)
	}
	if _, ok := MicronProperty(v, "nope"); ok {
		t.Error("unknown coloron prop ok")
	}
	// Map forms.
	if out, err := makeColoron(mapOf("r", NewInteger(255), "g", NewInteger(0), "b", NewInteger(0))); err != nil || out[0].String() != "#ff0000" {
		t.Errorf("map coloron = %v (%v)", out, err)
	}
	if out, err := makeColoron(mapOf("r", NewInteger(17), "g", NewInteger(34), "b", NewInteger(51), "a", NewInteger(68))); err != nil || out[0].String() != "#11223344" {
		t.Errorf("map coloron alpha = %v (%v)", out, err)
	}
	for _, bad := range []Value{
		NewString("#12345"), NewString("#gg0000"), NewString("ff0000"), NewString("#zz"),
		NewString("rgb(255, 0, 0 / 0.5)"), NewString("rgb(255, 0)"), NewString("rgb(1 2 3 4 5)"),
		NewString("rgb(a b c)"), NewString("rgb(1 2 / 3)"), NewString("hsl(0,0,0)"),
		NewString("rgb 1 2 3"), NewInteger(42),
		mapOf("r", NewInteger(0), "g", NewInteger(0)),
		mapOf("r", NewInteger(0), "g", NewInteger(0), "b", NewInteger(0), "x", NewInteger(1)),
		mapOf("r", NewInteger(300), "g", NewInteger(0), "b", NewInteger(0)),
		mapOf("r", NewString("x"), "g", NewInteger(0), "b", NewInteger(0)),
		MakeValue(TMap, ListPayload{}),
	} {
		if _, err := makeColoron(bad); err == nil {
			t.Errorf("bad coloron %v accepted", bad)
		}
	}
	// Bad alpha / percent / channel / unterminated-paren tokens (each of the
	// three channel positions and the alpha position must reject).
	for _, bad := range []string{
		"rgb(0 0 0 / xx)", "rgb(0 0 0 / 5x%)", "rgb(50x% 0 0)",
		"rgb(1 b 3)", "rgb(1 2 c)", "rgb(1 2 3",
	} {
		if _, err := makeColoron(NewString(bad)); err == nil {
			t.Errorf("bad colour %q accepted", bad)
		}
	}
	// Ordering by channel tuple.
	lt := func(a, b string) {
		if compareColorons(mkLeaf(t, TColoron, makeColoron, a), mkLeaf(t, TColoron, makeColoron, b)) != -1 {
			t.Errorf("coloron %q < %q failed", a, b)
		}
	}
	lt("#ff0000", "#ff0001")
	lt("#00000010", "#ff000000")
	if compareColorons(mkLeaf(t, TColoron, makeColoron, "#112233"), mkLeaf(t, TColoron, makeColoron, "#112233")) != 0 {
		t.Error("equal coloron cmp != 0")
	}
	if compareColorons(NewInteger(1), NewInteger(2)) == 0 {
		t.Error("non-payload coloron fallback equal")
	}
	// Helper edge cases.
	if clampFloat(-1, 0, 10) != 0 || clampFloat(20, 0, 10) != 10 || clampFloat(5, 0, 10) != 5 {
		t.Error("clampFloat wrong")
	}
}

// ---- Mimon ----

func TestMakeMimon(t *testing.T) {
	for in, want := range map[string]string{
		`text/html; charset=utf-8`:    `text/html; charset=utf-8`,
		`Text/HTML; Charset="utf-8"`:  `text/html; charset=utf-8`,
		`text/html ; charset = utf-8`: `text/html; charset=utf-8`,
		`application/vnd.api+json`:    `application/vnd.api+json`,
		`a/b; z=1; a=2; m=3`:          `a/b; a=2; m=3; z=1`,
		`image/svg+xml`:               `image/svg+xml`,
	} {
		if v := mkLeaf(t, TMimon, makeMimon, in); v.String() != want {
			t.Errorf("mimon %q → %q, want %q", in, v.String(), want)
		}
	}
	v := mkLeaf(t, TMimon, makeMimon, "text/html; charset=utf-8")
	if s, _ := AsString(prop(t, v, "type")); s != "text" {
		t.Errorf("type = %q", s)
	}
	if s, _ := AsString(prop(t, v, "essence")); s != "text/html" {
		t.Errorf("essence = %q", s)
	}
	if s, _ := AsString(prop(t, v, "mime")); s != "text/html; charset=utf-8" {
		t.Errorf("mime = %q", s)
	}
	if s, _ := AsString(prop(t, mkLeaf(t, TMimon, makeMimon, "image/svg+xml"), "suffix")); s != "xml" {
		t.Errorf("suffix = %q", s)
	}
	if s, _ := AsString(prop(t, mkLeaf(t, TMimon, makeMimon, "application/vnd.api+json"), "facet")); s != "vnd" {
		t.Errorf("facet = %q", s)
	}
	// No suffix / no facet → miss.
	plain := mkLeaf(t, TMimon, makeMimon, "text/html")
	if _, ok := MicronProperty(plain, "suffix"); ok {
		t.Error("text/html has a suffix?")
	}
	if _, ok := MicronProperty(plain, "facet"); ok {
		t.Error("text/html has a facet?")
	}
	if _, ok := MicronProperty(plain, "nope"); ok {
		t.Error("unknown mimon prop ok")
	}
	// Map form.
	if out, err := makeMimon(mapOf("type", NewString("text"), "subtype", NewString("html"),
		"params", mapOf("charset", NewString("utf-8")))); err != nil || out[0].String() != "text/html; charset=utf-8" {
		t.Errorf("map mimon = %v (%v)", out, err)
	}
	if out, err := makeMimon(mapOf("type", NewString("text"), "subtype", NewString("plain"))); err != nil || out[0].String() != "text/plain" {
		t.Errorf("map mimon no params = %v (%v)", out, err)
	}
	for _, bad := range []Value{
		NewString("text"), NewString("*/*"), NewString("text/*"),
		NewString(`text/html; charset=""`), NewString("text/html; charset=utf-8; charset=ascii"),
		NewString("text/ html"), NewInteger(42),
		mapOf("type", NewString("text"), "subtype", NewString("html"), "extra", NewString("x")),
		mapOf("type", NewString("text")),
		mapOf("type", NewInteger(1), "subtype", NewString("html")),
		mapOf("type", NewString("text"), "subtype", NewInteger(1)),
		mapOf("type", NewString("text"), "subtype", NewString("html"), "params", NewInteger(1)),
		mapOf("type", NewString("text"), "subtype", NewString("html"), "params", mapOf("k", NewInteger(1))),
		mapOf("type", NewString("*"), "subtype", NewString("*")),
		mapOf("type", NewString("a/b"), "subtype", NewString("c")), // FormatMediaType rejects a slashed type
		MakeValue(TMap, ListPayload{}),
	} {
		if _, err := makeMimon(bad); err == nil {
			t.Errorf("bad mimon %v accepted", bad)
		}
	}
	if validRestrictedName("") || validRestrictedName("*x") || validRestrictedName("a b") {
		t.Error("validRestrictedName too lax")
	}
	if !validRestrictedName("vnd.api+json") {
		t.Error("validRestrictedName too strict")
	}
}

// ---- Qion ----

func TestMakeQion(t *testing.T) {
	for in, want := range map[string]string{
		"USD 12.50":        "USD 12.50",
		"usd 12.5":         "USD 12.50",
		"JPY 500":          "JPY 500",
		"BHD 1.250":        "BHD 1.250",
		"USD -3.05":        "USD -3.05",
		"USD -0.00":        "USD 0.00",
		"CLF 0.0001":       "CLF 0.0001",
		"EUR   1000000.00": "EUR 1000000.00",
	} {
		if v := mkLeaf(t, TQion, makeQion, in); v.String() != want {
			t.Errorf("qion %q → %q, want %q", in, v.String(), want)
		}
	}
	v := mkLeaf(t, TQion, makeQion, "USD 12.50")
	if s, _ := AsString(prop(t, v, "code")); s != "USD" {
		t.Errorf("code = %q", s)
	}
	if n, _ := AsInteger(prop(t, v, "minor")); n != 2 {
		t.Errorf("minor = %d", n)
	}
	if n, _ := AsInteger(prop(t, v, "units")); n != 1250 {
		t.Errorf("units = %d", n)
	}
	if n, _ := AsInteger(prop(t, mkLeaf(t, TQion, makeQion, "JPY 500"), "units")); n != 500 {
		t.Errorf("jpy units = %d", n)
	}
	if n, _ := AsInteger(prop(t, mkLeaf(t, TQion, makeQion, "USD -3.05"), "units")); n != -305 {
		t.Errorf("negative units = %d", n)
	}
	if bo, _ := AsBoolean(prop(t, mkLeaf(t, TQion, makeQion, "USD -3.05"), "negative")); !bo {
		t.Error("negative amount not negative")
	}
	if bo, _ := AsBoolean(prop(t, mkLeaf(t, TQion, makeQion, "USD 0.00"), "negative")); bo {
		t.Error("zero reported negative")
	}
	if s, _ := AsString(prop(t, v, "display")); s != "USD 12.50" {
		t.Errorf("display = %q", s)
	}
	if _, ok := MicronProperty(v, "nope"); ok {
		t.Error("unknown qion prop ok")
	}
	// Map form: string, integer, decimal amounts.
	if out, err := makeQion(mapOf("code", NewString("USD"), "amount", NewString("12.5"))); err != nil || out[0].String() != "USD 12.50" {
		t.Errorf("map qion string = %v (%v)", out, err)
	}
	if out, err := makeQion(mapOf("code", NewString("JPY"), "amount", NewInteger(500))); err != nil || out[0].String() != "JPY 500" {
		t.Errorf("map qion int = %v (%v)", out, err)
	}
	decAmt := qionAmountValue(t, "3.14")
	if out, err := makeQion(mapOf("code", NewString("USD"), "amount", decAmt)); err != nil || out[0].String() != "USD 3.14" {
		t.Errorf("map qion decimal = %v (%v)", out, err)
	}
	for _, bad := range []Value{
		NewString("USD 12.999"), NewString("JPY 5.0"), NewString("ZZZ 5.00"), NewString("US 5.00"),
		NewString("USD 1,000.00"), NewString("USD 1e2"), NewString("USD +5.00"), NewString("$12.50"),
		NewString("USD"), NewString("USD 1 2"), NewInteger(42),
		mapOf("code", NewString("USD"), "amount", NewString("1"), "extra", NewString("y")),
		mapOf("code", NewString("USD")),
		mapOf("code", NewInteger(1), "amount", NewString("1")),
		mapOf("code", NewString("USD"), "amount", NewBoolean(true)),
		mapOf("code", NewString("USD"), "amount", MakeValue(TBigDecimal, ListPayload{})),
		MakeValue(TMap, ListPayload{}),
	} {
		if _, err := makeQion(bad); err == nil {
			t.Errorf("bad qion %v accepted", bad)
		}
	}
	// units overflowing int64 → absent.
	huge := mkLeaf(t, TQion, makeQion, "USD 999999999999999999.99")
	if _, ok := MicronProperty(huge, "units"); ok {
		t.Error("overflowing units not absent")
	}
	// Ordering: code then magnitude.
	lt := func(a, b string) {
		if compareQions(mkLeaf(t, TQion, makeQion, a), mkLeaf(t, TQion, makeQion, b)) != -1 {
			t.Errorf("qion %q < %q failed", a, b)
		}
	}
	lt("USD 9.00", "USD 100.00")
	lt("USD -3.05", "USD 0.00")
	lt("EUR 100.00", "USD 1.00")
	if compareQions(mkLeaf(t, TQion, makeQion, "USD 1.00"), mkLeaf(t, TQion, makeQion, "USD 1.00")) != 0 {
		t.Error("equal qion cmp != 0")
	}
	if compareQions(NewInteger(1), NewInteger(2)) == 0 {
		t.Error("non-payload qion fallback equal")
	}
	if isThreeLetters("us") || isThreeLetters("USDD") || isThreeLetters("US1") {
		t.Error("isThreeLetters too lax")
	}
}

// ---- Phonon ----

func TestMakePhonon(t *testing.T) {
	for in, want := range map[string]string{
		"+14155552671":      "+14155552671",
		"+1 (415) 555-2671": "+14155552671",
		"+44 20 7183 8750":  "+442071838750",
		"+81-3-1234-5678":   "+81312345678",
		"+1.415.555.2671":   "+14155552671",
		"+3906698":          "+3906698",
	} {
		if v := mkLeaf(t, TPhonon, makePhonon, in); v.String() != want {
			t.Errorf("phonon %q → %q, want %q", in, v.String(), want)
		}
	}
	v := mkLeaf(t, TPhonon, makePhonon, "+14155552671")
	if s, _ := AsString(prop(t, v, "digits")); s != "14155552671" {
		t.Errorf("digits = %q", s)
	}
	if s, _ := AsString(prop(t, v, "e164")); s != "+14155552671" {
		t.Errorf("e164 = %q", s)
	}
	if n, _ := AsInteger(prop(t, v, "length")); n != 11 {
		t.Errorf("length = %d", n)
	}
	if _, ok := MicronProperty(v, "nope"); ok {
		t.Error("unknown phonon prop ok")
	}
	if out, err := makePhonon(mapOf("cc", NewString("1"), "subscriber", NewString("4155552671"))); err != nil || out[0].String() != "+14155552671" {
		t.Errorf("map phonon = %v (%v)", out, err)
	}
	for _, bad := range []Value{
		NewString("14155552671"), NewString("+0415552671"), NewString("+1"),
		NewString("+1234567890123456"), NewString("+1800FLOWERS"), NewString("004415552671"),
		NewString("+14155552671;ext=101"), NewInteger(42),
		mapOf("cc", NewString("1")),
		mapOf("cc", NewString("1"), "subscriber", NewString("4155552671"), "extra", NewString("x")),
		mapOf("cc", NewInteger(1), "subscriber", NewString("x")),
		mapOf("cc", NewString("1"), "subscriber", NewInteger(1)),
		MakeValue(TMap, ListPayload{}),
	} {
		if _, err := makePhonon(bad); err == nil {
			t.Errorf("bad phonon %v accepted", bad)
		}
	}
}

// micronFields builds a MicronPayload field map for the defensive-branch
// tests that inject deliberately-malformed payloads.
func micronFields(pairs ...Value) *OrderedMap {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		s, _ := AsString(pairs[i])
		m.Set(s, pairs[i+1])
	}
	return m
}

// TestMicronLeafDefensivePayloads exercises the defensive fallbacks that
// fire only on a malformed stored payload — unreachable through normal
// construction, so injected here directly.
func TestMicronLeafDefensivePayloads(t *testing.T) {
	// Cidron: a payload whose stored cidr no longer parses falls back in
	// compareCidrons (cidronPrefixOf → netip failure).
	badCidron := NewValueRaw(TCidron, MicronPayload{Fields: micronFields(NewString("cidr"), NewString("garbage"))})
	if _, ok := cidronPrefixOf(badCidron); ok {
		t.Error("garbage cidr parsed")
	}
	_ = compareCidrons(badCidron, badCidron) // exercises the fallback path

	// Macon: a payload whose stored addr no longer parses → maconBytes
	// fails → the derived properties miss.
	badMacon := NewValueRaw(TMacon, MicronPayload{Fields: micronFields(NewString("addr"), NewString("zzz"))})
	if _, ok := MicronProperty(badMacon, "bits"); ok {
		t.Error("garbage macon addr yielded bits")
	}

	// Mimon: params stored as a non-Map → mimonParams returns nil.
	badMimon := NewValueRaw(TMimon, MicronPayload{Fields: micronFields(
		NewString("type"), NewString("text"), NewString("subtype"), NewString("html"),
		NewString("params"), NewString("not-a-map"))})
	if mimonParams(fieldsOf(t, badMimon)) != nil {
		t.Error("non-map params parsed")
	}
	if micronMimonRender(fieldsOf(t, badMimon)) != "text/html" {
		t.Error("bad-params render not essence")
	}
	// A stored essence FormatMediaType can't render (slashed type) with
	// non-empty params falls back to the raw essence.
	badRender := NewValueRaw(TMimon, MicronPayload{Fields: micronFields(
		NewString("type"), NewString("a/b"), NewString("subtype"), NewString("c"),
		NewString("params"), NewMap(micronFields(NewString("x"), NewString("y"))))})
	if micronMimonRender(fieldsOf(t, badRender)) != "a/b/c" {
		t.Error("unrenderable mimon did not fall back to essence")
	}

	// Qion: amount stored as a non-Decimal → render/units/compare fall back.
	badQion := NewValueRaw(TQion, MicronPayload{Fields: micronFields(
		NewString("code"), NewString("USD"), NewString("amount"), NewString("x"),
		NewString("minor"), NewInteger(2))})
	if micronQionRender(fieldsOf(t, badQion)) != "USD" {
		t.Error("bad-amount qion render not code-only")
	}
	if _, ok := qionUnits(fieldsOf(t, badQion)); ok {
		t.Error("bad-amount qion units present")
	}
	good := mkLeaf(t, TQion, makeQion, "USD 1.00")
	_ = compareQions(badQion, good) // same code, bad amount → CompareValues fallback

	// Qion payload missing its amount entirely → render is code-only,
	// units/negative miss.
	noAmt := NewValueRaw(TQion, MicronPayload{Fields: micronFields(NewString("code"), NewString("USD"))})
	if micronQionRender(fieldsOf(t, noAmt)) != "USD" {
		t.Error("no-amount qion render not code-only")
	}
	if _, ok := qionUnits(fieldsOf(t, noAmt)); ok {
		t.Error("no-amount qion units present")
	}
	if _, ok := MicronProperty(noAmt, "negative"); ok {
		t.Error("no-amount qion negative present")
	}
	_ = compareQions(noAmt, good) // one side missing amount → fallback
}

func fieldsOf(t *testing.T, v Value) *OrderedMap {
	t.Helper()
	f, err := AsMicronFields(v)
	if err != nil {
		t.Fatalf("fieldsOf: %v", err)
	}
	return f
}

// qionAmountValue extracts a BigDecimal amount Value (via a constructed
// Qion) so the map-form decimal-amount path can be exercised without
// importing apd into the test file.
func qionAmountValue(t *testing.T, s string) Value {
	t.Helper()
	out, err := makeQion(NewString("USD " + s))
	if err != nil {
		t.Fatalf("qionAmountValue %q: %v", s, err)
	}
	f, _ := AsMicronFields(out[0])
	amt, _ := f.Get("amount")
	return amt
}

// TestQionInfiniteAmountHasNoUnits — a hand-built Qion payload whose stored
// amount is an INFINITE Decimal (constructible only by hand: the parser
// mints finite Decimals exclusively) defeats the units derivation — the
// Text('f') render is "Infinity", which the big.Int parse rejects.
func TestQionInfiniteAmountHasNoUnits(t *testing.T) {
	inf := NewValueRaw(TQion, MicronPayload{Fields: micronFields(
		NewString("code"), NewString("USD"),
		NewString("amount"), NewBigDecimal(&apd.Decimal{Form: apd.Infinite}),
		NewString("minor"), NewInteger(2))})
	if _, ok := qionUnits(fieldsOf(t, inf)); ok {
		t.Error("infinite-amount qion must have no units")
	}
	// POSITIVE pair: the finite path still derives units and orders by
	// magnitude.
	if n, _ := AsInteger(prop(t, mkLeaf(t, TQion, makeQion, "USD 1.50"), "units")); n != 150 {
		t.Errorf("finite qion units = %d, want 150", n)
	}
	if compareQions(mkLeaf(t, TQion, makeQion, "USD 1.00"), mkLeaf(t, TQion, makeQion, "USD 2.00")) != -1 {
		t.Error("finite same-code qions must order by amount")
	}
}
