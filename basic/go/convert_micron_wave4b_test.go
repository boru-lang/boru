package basic

import (
	"strings"
	"testing"
)

// Wave 4b coverage: the IdealConverter capability walk and the concrete
// converters (convert_ideal.go), plus the Scalar/Micron family —
// construction, rendering, properties, comparison, check-mode mirror
// (micron.go) — and the Micron branches of InstallType (core_type.go).

// --- convert_ideal.go ----------------------------------------------------------

func TestRequireMicronName(t *testing.T) {
	if err := requireMicronName("Fooon"); err != nil {
		t.Errorf("Fooon rejected: %v", err)
	}
	if err := requireMicronName("Scalar/Micron/Bazon"); err != nil {
		t.Errorf("path name rejected: %v", err)
	}
	if err := requireMicronName("Bar"); err == nil {
		t.Error("Bar accepted")
	}
	if err := requireMicronName("On"); err == nil {
		t.Error("bare On accepted")
	}
	if err := requireMicronName("BARON"); err == nil {
		t.Error("BARON accepted (case-sensitive rule)")
	}
}

func TestMakeEmailon(t *testing.T) {
	out, err := makeEmailon(NewString("alice@example.com"))
	if err != nil {
		t.Fatalf("makeEmailon: %v", err)
	}
	v := out[0]
	if !IsMicronValue(v) || !v.Parent.Equal(TEmailon) {
		t.Fatalf("emailon = %v", v)
	}
	if got := v.String(); got != "alice@example.com" {
		t.Errorf("render = %q", got)
	}
	fields, _ := AsMicronFields(v)
	if u := micronFieldString(fields, "user"); u != "alice" {
		t.Errorf("user = %q", u)
	}
	if micronFieldString(fields, "nope") != "" {
		t.Error("missing field non-empty")
	}

	// Map form re-validates through the string path.
	out, err = makeEmailon(mapOf("user", NewString("bob"), "host", NewString("x.org")))
	if err != nil {
		t.Fatalf("map emailon: %v", err)
	}
	if got := out[0].String(); got != "bob@x.org" {
		t.Errorf("map render = %q", got)
	}

	// Negative space.
	if _, err := makeEmailon(NewString("not-an-address")); err == nil {
		t.Error("bad address accepted")
	}
	if _, err := makeEmailon(NewString("Alice <a@x.com>")); err == nil {
		t.Error("display-name form accepted")
	}
	if _, err := makeEmailon(mapOf("user", NewString("u"))); err == nil {
		t.Error("missing host accepted")
	}
	if _, err := makeEmailon(mapOf("user", NewString("u"), "bogus", NewString("v"))); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := makeEmailon(mapOf("user", NewInteger(1), "host", NewString("h"))); err == nil {
		t.Error("non-string field accepted")
	}
	if _, err := makeEmailon(NewInteger(5)); err == nil {
		t.Error("integer source accepted")
	}
}

func TestMakeUrlon(t *testing.T) {
	out, err := makeUrlon(NewString("https://example.com:8080/p?q=1#frag"))
	if err != nil {
		t.Fatalf("makeUrlon: %v", err)
	}
	v := out[0]
	if !v.Parent.Equal(TUrlon) {
		t.Fatalf("urlon = %v", v)
	}
	if got := v.String(); got != "https://example.com:8080/p?q=1#frag" {
		t.Errorf("href = %q", got)
	}

	// Map form with the full field set.
	out, err = makeUrlon(mapOf(
		"scheme", NewString("http"),
		"host", NewString("h.io"),
		"port", NewInteger(81),
		"path", NewString("/x"),
		"query", NewString("a=b"),
		"fragment", NewString("top"),
	))
	if err != nil {
		t.Fatalf("map urlon: %v", err)
	}
	if got := out[0].String(); got != "http://h.io:81/x?a=b#top" {
		t.Errorf("map href = %q", got)
	}

	// Negative space.
	if _, err := makeUrlon(NewString("/relative/only")); err == nil {
		t.Error("relative URL accepted")
	}
	if _, err := makeUrlon(NewString("://")); err == nil {
		t.Error("junk URL accepted")
	}
	if _, err := makeUrlon(mapOf("host", NewString("h"))); err == nil {
		t.Error("missing scheme accepted")
	}
	if _, err := makeUrlon(mapOf("scheme", NewString("s"), "host", NewString("h"), "zzz", NewString("v"))); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := makeUrlon(mapOf("scheme", NewString("s"), "host", NewString("h"), "port", NewString("81"))); err == nil {
		t.Error("string port accepted")
	}
	if _, err := makeUrlon(mapOf("scheme", NewString("s"), "host", NewInteger(1))); err == nil {
		t.Error("integer host accepted")
	}
	if _, err := makeUrlon(NewBoolean(true)); err == nil {
		t.Error("boolean source accepted")
	}
}

func TestMakeIpon(t *testing.T) {
	out, err := makeIpon(NewString("203.0.113.7"))
	if err != nil {
		t.Fatalf("makeIpon: %v", err)
	}
	v := out[0]
	if !v.Parent.Equal(TIpon) {
		t.Fatalf("ipon = %v", v)
	}
	if got := v.String(); got != "203.0.113.7" {
		t.Errorf("render = %q", got)
	}
	fields, _ := AsMicronFields(v)
	if a := micronFieldString(fields, "addr"); a != "203.0.113.7" {
		t.Errorf("addr = %q", a)
	}

	// IPv6 canonicalizes (lowercase, zero-compressed).
	out, err = makeIpon(NewString("2001:0DB8::1"))
	if err != nil {
		t.Fatalf("ipv6 ipon: %v", err)
	}
	if got := out[0].String(); got != "2001:db8::1" {
		t.Errorf("ipv6 render = %q", got)
	}

	// Map form re-validates through the string path.
	out, err = makeIpon(mapOf("addr", NewString("10.0.0.1")))
	if err != nil {
		t.Fatalf("map ipon: %v", err)
	}
	if got := out[0].String(); got != "10.0.0.1" {
		t.Errorf("map render = %q", got)
	}

	// Negative space.
	if _, err := makeIpon(NewString("not-an-ip")); err == nil {
		t.Error("bad address accepted")
	}
	if _, err := makeIpon(NewString("10.0.0")); err == nil {
		t.Error("short IPv4 accepted")
	}
	if _, err := makeIpon(mapOf("addr", NewString("1.2.3.4"), "extra", NewString("x"))); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := makeIpon(mapOf("addr", NewInteger(1))); err == nil {
		t.Error("non-string addr accepted")
	}
	if _, err := makeIpon(mapOf()); err == nil {
		t.Error("empty map (missing addr) accepted")
	}
	if _, err := makeIpon(NewBoolean(true)); err == nil {
		t.Error("boolean source accepted")
	}
}

func TestMakeHoston(t *testing.T) {
	mkVal := func(s string) Value {
		out, err := makeHoston(NewString(s))
		if err != nil {
			t.Fatalf("makeHoston(%q): %v", s, err)
		}
		return out[0]
	}
	host := func(v Value) string { h, _ := MicronProperty(v, "host"); s, _ := AsString(h); return s }
	port := func(v Value) (int64, bool) {
		p, ok := MicronProperty(v, "port")
		if !ok {
			return 0, false
		}
		n, _ := AsInteger(p)
		return n, true
	}

	// host:port, host-only, IPv4:port.
	v := mkVal("example.com:8080")
	if host(v) != "example.com" || v.String() != "example.com:8080" {
		t.Errorf("host:port = %+v (%q)", v, v.String())
	}
	if n, ok := port(v); !ok || n != 8080 {
		t.Errorf("port = %d %v", n, ok)
	}
	if v = mkVal("example.com"); v.String() != "example.com" {
		t.Errorf("host-only render = %q", v.String())
	}
	if _, ok := port(v); ok {
		t.Error("host-only carries a port")
	}
	if host(mkVal("127.0.0.1:80")) != "127.0.0.1" {
		t.Error("ipv4 host wrong")
	}
	// IPv6: bracketed (with/without port), and bare (multi-colon, no port).
	if v = mkVal("[::1]:8080"); host(v) != "::1" || v.String() != "[::1]:8080" {
		t.Errorf("bracketed ipv6 = %q", v.String())
	}
	if v = mkVal("[2001:db8::1]"); host(v) != "2001:db8::1" || v.String() != "2001:db8::1" {
		t.Errorf("bracketed no-port = %q", v.String())
	}
	if v = mkVal("2001:db8::1"); host(v) != "2001:db8::1" {
		t.Errorf("bare ipv6 host = %q", host(v))
	}
	if _, ok := port(mkVal("2001:db8::1")); ok {
		t.Error("bare ipv6 carries a port")
	}

	// Map form: host+port, host-only, IPv6 re-brackets.
	mval := func(m Value) (Value, error) {
		out, err := makeHoston(m)
		if err != nil {
			return Value{}, err
		}
		return out[0], nil
	}
	if got, _ := mval(mapOf("host", NewString("a.com"), "port", NewInteger(443))); got.String() != "a.com:443" {
		t.Errorf("map host+port = %q", got.String())
	}
	if got, _ := mval(mapOf("host", NewString("a.com"))); got.String() != "a.com" {
		t.Errorf("map host-only = %q", got.String())
	}
	if got, _ := mval(mapOf("host", NewString("::1"), "port", NewInteger(22))); got.String() != "[::1]:22" {
		t.Errorf("map ipv6 re-bracket = %q", got.String())
	}
	// Derived authority property.
	a, _ := MicronProperty(mkVal("a.com:8080"), "authority")
	if s, _ := AsString(a); s != "a.com:8080" {
		t.Errorf("authority = %q", s)
	}

	// Negative space — string forms. A host is not a URL, so authority
	// delimiters (/ ? #) are rejected rather than swallowed into the host.
	for _, bad := range []string{
		"", "   ", "a b:80", ":80", "host:99999", "host:-1", "host:notaport",
		"[::1", "[not-an-ip]:80", "[::1]x", "[::1]:99999", "1:2:3",
		"example.com/path", "example.com/path:443", "a?b", "a#b",
	} {
		if _, err := makeHoston(NewString(bad)); err == nil {
			t.Errorf("bad authority %q accepted", bad)
		}
	}
	// Negative space — map + source.
	if _, err := mval(mapOf("port", NewInteger(80))); err == nil {
		t.Error("missing host accepted")
	}
	// The host field is a host, not an authority: a "host:port" string there
	// is rejected (not silently split into the optional port), as are URL
	// delimiters and an out-of-range explicit port.
	if _, err := mval(mapOf("host", NewString("a.com:80"))); err == nil {
		t.Error("host field with a :port accepted")
	}
	if _, err := mval(mapOf("host", NewString("a.com/p"))); err == nil {
		t.Error("host field with a / accepted")
	}
	if _, err := mval(mapOf("host", NewString("a.com"), "port", NewInteger(99999))); err == nil {
		t.Error("out-of-range map port accepted")
	}
	if _, err := mval(mapOf("host", NewString("a"), "extra", NewString("y"))); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := mval(mapOf("host", NewInteger(1))); err == nil {
		t.Error("non-string host accepted")
	}
	if _, err := mval(mapOf("host", NewString("a"), "port", NewString("80"))); err == nil {
		t.Error("non-integer port accepted")
	}
	if _, err := makeHoston(NewInteger(42)); err == nil {
		t.Error("integer source accepted")
	}
	if _, err := makeHoston(Value{Parent: TMap, Data: ListPayload{}}); err == nil {
		t.Error("unreadable map payload accepted")
	}
}

func TestMicronTypePredicates(t *testing.T) {
	if _, err := AsMicronType(NewInteger(1)); err == nil {
		t.Error("AsMicronType accepted an integer")
	}
	if _, err := AsMicronFields(NewInteger(1)); err == nil {
		t.Error("AsMicronFields accepted an integer")
	}
	body, err := micronConstruct(NewTypeLiteral(TMicron), mapOf("tag", NewTypeLiteral(TString)), nil)
	if err != nil {
		t.Fatalf("micronConstruct: %v", err)
	}
	if !IsMicronType(body[0]) {
		t.Error("construct result not a micron type body")
	}
	info, err := AsMicronType(body[0])
	if err != nil || info.Fields.Len() != 1 {
		t.Errorf("info = %+v (%v)", info, err)
	}
	// micronAccepts: type bodies and bare family literals only.
	if !micronAccepts(body[0]) || !micronAccepts(NewTypeLiteral(TEmailon)) {
		t.Error("micronAccepts underclaims")
	}
	if micronAccepts(NewInteger(1)) || micronAccepts(NewTypeLiteral(TInteger)) {
		t.Error("micronAccepts overclaims")
	}
}

func TestMicronConstructNegatives(t *testing.T) {
	// Refining a leaf is declined.
	if _, err := micronConstruct(NewTypeLiteral(TEmailon), mapOf("x", NewTypeLiteral(TString)), nil); err == nil {
		t.Error("leaf refinement accepted")
	}
	// Refining an existing user type body is declined, naming the kind.
	body, _ := micronConstruct(NewTypeLiteral(TMicron), mapOf("t", NewTypeLiteral(TString)), nil)
	if _, err := micronConstruct(body[0], mapOf("u", NewTypeLiteral(TString)), nil); err == nil {
		t.Error("type-body refinement accepted")
	}
	// A non-map argument is declined.
	if _, err := micronConstruct(NewTypeLiteral(TMicron), NewInteger(1), nil); err == nil {
		t.Error("non-map field spec accepted")
	}
	// A field that is neither a type nor a concrete default is declined.
	if _, err := micronConstruct(NewTypeLiteral(TMicron), mapOf("f", NewCarrier(TString)), nil); err == nil {
		t.Error("carrier field accepted")
	}
}

func TestInstallMicronTypeAndMake(t *testing.T) {
	r := newTestRegistry(t)
	body, err := micronConstruct(NewTypeLiteral(TMicron),
		mapOf("tag", NewTypeLiteral(TString), "level", NewInteger(3)), r)
	if err != nil {
		t.Fatalf("micronConstruct: %v", err)
	}
	// The naming rule holds at install time.
	if err := InstallType(r, "W4bBadName", body[0]); err == nil {
		t.Error("non-on Micron name accepted")
	}
	if err := InstallType(r, "W4bTagon", body[0]); err != nil {
		t.Fatalf("InstallType W4bTagon: %v", err)
	}
	node := r.LookupTypeName("W4bTagon")
	if node == nil {
		t.Fatal("W4bTagon not bound")
	}
	// The installed schema is discoverable through the parent walk.
	schema, ok := MicronSchemaFor(node)
	if !ok || schema.Len() != 2 {
		t.Errorf("MicronSchemaFor = %v, %v", schema, ok)
	}
	if _, ok := MicronSchemaFor(TEmailon); ok {
		t.Error("builtin leaf claims a user schema")
	}

	// make with a full field map.
	tb, _ := r.TopTypeBody("W4bTagon")
	out, err := micronInstantiate(tb, mapOf("tag", NewString("x"), "level", NewInteger(9)), r)
	if err != nil {
		t.Fatalf("micronInstantiate: %v", err)
	}
	inst := out[0]
	if !inst.Parent.Equal(node) {
		t.Errorf("instance parent = %v", inst.Parent)
	}
	// Defaults fill omitted fields.
	out, err = micronInstantiate(tb, mapOf("tag", NewString("y")), r)
	if err != nil {
		t.Fatalf("defaulted make: %v", err)
	}
	fields, _ := AsMicronFields(out[0])
	lv, _ := fields.Get("level")
	if n, _ := AsInteger(lv); n != 3 {
		t.Errorf("default level = %v", lv)
	}
	// The instance renders as its field map and equals a same-content twin.
	if got := out[0].String(); !strings.Contains(got, "tag") {
		t.Errorf("user micron render = %q", got)
	}
	twin, _ := micronInstantiate(tb, mapOf("tag", NewString("y")), r)
	if !ValuesEqual(out[0], twin[0]) {
		t.Error("equal-content user microns unequal")
	}

	// Negative space.
	if _, err := micronInstantiate(tb, mapOf("bogus", NewString("z")), r); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := micronInstantiate(tb, mapOf("level", NewInteger(1)), r); err == nil {
		t.Error("missing required field accepted")
	}
	if _, err := micronInstantiate(tb, mapOf("tag", NewInteger(1)), r); err == nil {
		t.Error("type-violating field accepted")
	}
	if _, err := micronInstantiate(tb, NewInteger(1), r); err == nil {
		t.Error("non-map data accepted")
	}
}

func TestMicronInstantiateKinds(t *testing.T) {
	r := newTestRegistry(t)
	// The abstract root declines.
	if _, err := micronInstantiate(NewTypeLiteral(TMicron), NewString("x"), r); err == nil {
		t.Error("abstract Micron make accepted")
	}
	// Builtin leaves construct.
	out, err := micronInstantiate(NewTypeLiteral(TEmailon), NewString("a@b.io"), r)
	if err != nil || !out[0].Parent.Equal(TEmailon) {
		t.Errorf("Emailon make = %v (%v)", out, err)
	}
	out, err = micronInstantiate(NewTypeLiteral(TUrlon), NewString("https://a.io"), r)
	if err != nil || !out[0].Parent.Equal(TUrlon) {
		t.Errorf("Urlon make = %v (%v)", out, err)
	}
	out, err = micronInstantiate(NewTypeLiteral(TPathon), NewString("a/b"), r)
	if err != nil || !IsPathon(out[0]) {
		t.Errorf("Pathon make = %v (%v)", out, err)
	}
	out, err = micronInstantiate(NewTypeLiteral(TIpon), NewString("203.0.113.7"), r)
	if err != nil || !out[0].Parent.Equal(TIpon) {
		t.Errorf("Ipon make = %v (%v)", out, err)
	}
	out, err = micronInstantiate(NewTypeLiteral(THoston), NewString("example.com:8080"), r)
	if err != nil || !out[0].Parent.Equal(THoston) {
		t.Errorf("Hoston make = %v (%v)", out, err)
	}
	// A newtype of a builtin leaf constructs the base then retags.
	newt := r.Types.MintType("W4bMailon", TEmailon)
	out, err = micronInstantiate(NewTypeLiteral(newt), NewString("c@d.io"), r)
	if err != nil {
		t.Fatalf("newtype make: %v", err)
	}
	if !out[0].Parent.Equal(newt) {
		t.Errorf("newtype instance parent = %v, want W4bMailon", out[0].Parent)
	}
	// A newtype of Ipon likewise constructs the base then retags.
	newIp := r.Types.MintType("W4bIpon", TIpon)
	out, err = micronInstantiate(NewTypeLiteral(newIp), NewString("203.0.113.7"), r)
	if err != nil {
		t.Fatalf("ipon newtype make: %v", err)
	}
	if !out[0].Parent.Equal(newIp) {
		t.Errorf("ipon newtype instance parent = %v, want W4bIpon", out[0].Parent)
	}
	// A newtype of Hoston likewise constructs the base then retags.
	newH := r.Types.MintType("W4bHoston", THoston)
	out, err = micronInstantiate(NewTypeLiteral(newH), NewString("example.com:8080"), r)
	if err != nil {
		t.Fatalf("hoston newtype make: %v", err)
	}
	if !out[0].Parent.Equal(newH) {
		t.Errorf("hoston newtype instance parent = %v, want W4bHoston", out[0].Parent)
	}
	// A kind with no schema anywhere on its chain errors.
	bare := r.Types.MintType("W4bBareon", TMicron)
	if _, err := micronInstantiate(NewTypeLiteral(bare), mapOf("a", NewString("b")), r); err == nil {
		t.Error("schema-less kind constructed")
	}
}

func TestMicronPropertiesAndOrdering(t *testing.T) {
	p := NewPathon([]string{"a", "b"}, true)
	parts, ok := MicronProperty(p, "parts")
	if !ok {
		t.Fatal("no parts property")
	}
	pl, _ := AsList(parts)
	if pl.Len() != 2 {
		t.Errorf("parts = %v", parts)
	}
	abs, ok := MicronProperty(p, "abs")
	if !ok {
		t.Fatal("no abs property")
	}
	if b, _ := AsBoolean(abs); !b {
		t.Error("abs = false")
	}
	if _, ok := MicronProperty(p, "zzz"); ok {
		t.Error("pathon miss returned ok")
	}

	e, _ := makeEmailon(NewString("u@h.io"))
	if u, ok := MicronProperty(e[0], "user"); !ok {
		t.Error("no user property")
	} else if s, _ := AsString(u); s != "u" {
		t.Errorf("user = %q", s)
	}
	addr, ok := MicronProperty(e[0], "address")
	if !ok {
		t.Fatal("no derived address")
	}
	if s, _ := AsString(addr); s != "u@h.io" {
		t.Errorf("address = %q", s)
	}
	u, _ := makeUrlon(NewString("https://x.io/p"))
	href, ok := MicronProperty(u[0], "href")
	if !ok {
		t.Fatal("no derived href")
	}
	if s, _ := AsString(href); s != "https://x.io/p" {
		t.Errorf("href = %q", s)
	}
	if _, ok := MicronProperty(u[0], "missing"); ok {
		t.Error("urlon miss returned ok")
	}
	if _, ok := MicronProperty(NewInteger(1), "x"); ok {
		t.Error("integer property returned ok")
	}

	// Same-kind ordering via the family Comparer.
	e2, _ := makeEmailon(NewString("z@h.io"))
	c, err := CompareValues(e[0], e2[0])
	if err != nil || c != -1 {
		t.Errorf("emailon order = %d (%v)", c, err)
	}
	if c, _ := CompareValues(e[0], e[0]); c != 0 {
		t.Error("emailon not equal itself")
	}
	// Cross-kind pairs opt out of the family order and fall to Rank
	// (the raising `cmp` wrapper is eng-internal — its incomparable
	// arm over micron payloads is pinned by eng's
	// TestOrderedCompareCrossKindMicron).
	if _, err := CompareValues(p, e[0]); err != nil {
		t.Errorf("cross-kind tcmp errored: %v", err)
	}
	// The literal-first rule inside the family.
	if c, _ := CompareValues(NewTypeLiteral(TEmailon), e[0]); c != -1 {
		t.Error("Emailon literal not below a concrete emailon")
	}
	// Equality: same fields equal, kind mismatch unequal.
	e3, _ := makeEmailon(NewString("u@h.io"))
	if !ValuesEqual(e[0], e3[0]) {
		t.Error("equal emailons unequal")
	}
	if ValuesEqual(e[0], u[0]) {
		t.Error("emailon equals urlon")
	}
	if ValuesEqual(p, e[0]) {
		t.Error("pathon equals emailon")
	}
	if !ValuesEqual(p, NewPathon([]string{"a", "b"}, true)) {
		t.Error("equal pathons unequal")
	}
	if ValuesEqual(p, NewPathon([]string{"a", "b"}, false)) {
		t.Error("abs/rel pathons equal")
	}
	if ValuesEqual(p, NewPathon([]string{"a", "c"}, true)) {
		t.Error("different segments equal")
	}
}

func TestMicronTypeBodyRendering(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("tag", NewTypeLiteral(TString))
	anon := NewValueRaw(TMicron, MicronTypeInfo{Fields: fields})
	if got := anon.String(); !strings.HasPrefix(got, "Micron ") {
		t.Errorf("anonymous body render = %q", got)
	}
	named := NewValueRaw(TMicron, MicronTypeInfo{Name: "W4bNamedon", Fields: fields})
	if got := named.String(); got != "W4bNamedon" {
		t.Errorf("named body render = %q", got)
	}
}

func TestCheckMicronConstruction(t *testing.T) {
	r := newTestRegistry(t)
	pos := SrcPos{Row: 3, Col: 4}

	// Outside check mode: no-op.
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), NewString("junk"), pos)
	if len(r.Check.Diagnostics) != 0 {
		t.Fatal("diagnostic recorded outside check mode")
	}

	done := r.Check.Begin()
	defer done()

	// A statically-invalid construction surfaces the runtime message.
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), NewString("junk"), pos)
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v", r.Check.Diagnostics)
	}
	if d := r.Check.Diagnostics[0]; d.Code != "type_error" || d.Row != 3 {
		t.Errorf("diagnostic = %+v", d)
	}
	// The identical finding dedupes.
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), NewString("junk"), pos)
	if len(r.Check.Diagnostics) != 1 {
		t.Error("duplicate diagnostic recorded")
	}
	// A valid construction records nothing.
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), NewString("a@b.io"), pos)
	if len(r.Check.Diagnostics) != 1 {
		t.Error("valid construction flagged")
	}
	// Carrier sources are value-dependent — skipped.
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), NewCarrier(TString), pos)
	carrierField := NewOrderedMap()
	carrierField.Set("user", NewCarrier(TString))
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), NewMap(carrierField), pos)
	// Non-Micron targets are skipped.
	CheckMicronConstruction(r, NewTypeLiteral(TInteger), NewString("junk"), pos)
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("skip paths recorded diagnostics: %v", r.Check.Diagnostics)
	}
}
