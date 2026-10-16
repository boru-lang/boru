package basic

import "testing"

// TestMakeSemveron exercises the string and map construction paths and
// their negative space, mirroring TestMakeHoston / TestMakeIpon.
func TestMakeSemveron(t *testing.T) {
	mk := func(s string) Value {
		out, err := makeSemveron(NewString(s))
		if err != nil {
			t.Fatalf("makeSemveron(%q): %v", s, err)
		}
		return out[0]
	}

	// Canonical render round-trips accepted strings verbatim.
	for _, s := range []string{
		"1.2.3", "1.0.0-rc.1+build.5", "1.2.3+001", "0.0.0",
		"1.10.0", "1.2.3-alpha.beta", "1.2.3-alpha-1.a-b", "10.20.30-rc.1",
	} {
		v := mk(s)
		if !v.Parent().Equal(TSemveron) {
			t.Fatalf("semveron parent = %v", v.Parent())
		}
		if got := v.String(); got != s {
			t.Errorf("render(%q) = %q", s, got)
		}
	}

	// Stored + derived properties.
	v := mk("1.2.3-rc.1+b.2")
	checkInt := func(key string, want int64) {
		p, ok := MicronProperty(v, key)
		if !ok {
			t.Fatalf("no %s property", key)
		}
		if n, _ := AsInteger(p); n != want {
			t.Errorf("%s = %d, want %d", key, n, want)
		}
	}
	checkStr := func(key, want string) {
		p, ok := MicronProperty(v, key)
		if !ok {
			t.Fatalf("no %s property", key)
		}
		if s, _ := AsString(p); s != want {
			t.Errorf("%s = %q, want %q", key, s, want)
		}
	}
	checkInt("major", 1)
	checkInt("minor", 2)
	checkInt("patch", 3)
	checkStr("prerelease", "rc.1")
	checkStr("build", "b.2")
	checkStr("release", "1.2.3")
	checkStr("version", "1.2.3-rc.1+b.2")
	if pp, ok := MicronProperty(v, "prereleaseParts"); !ok {
		t.Error("no prereleaseParts")
	} else if l, _ := AsList(pp); l.Len() != 2 {
		t.Errorf("prereleaseParts len = %d", l.Len())
	}
	if bp, ok := MicronProperty(v, "buildParts"); !ok {
		t.Error("no buildParts")
	} else if l, _ := AsList(bp); l.Len() != 2 {
		t.Errorf("buildParts len = %d", l.Len())
	}
	if st, ok := MicronProperty(v, "stable"); !ok {
		t.Error("no stable")
	} else if b, _ := AsBoolean(st); b {
		t.Error("prerelease reported stable")
	}
	if _, ok := MicronProperty(v, "nope"); ok {
		t.Error("unknown Semveron property returned ok")
	}

	// A release (no prerelease) is stable; empty parts are empty lists.
	rel := mk("1.2.3+only")
	if st, _ := MicronProperty(rel, "stable"); func() bool { b, _ := AsBoolean(st); return !b }() {
		t.Error("build-only version not stable")
	}
	noBuild := mk("1.2.3")
	if bp, _ := MicronProperty(noBuild, "buildParts"); func() bool { l, _ := AsList(bp); return l.Len() != 0 }() {
		t.Error("absent build → non-empty buildParts")
	}
	if _, ok := MicronProperty(noBuild, "prerelease"); ok {
		t.Error("absent prerelease read ok")
	}

	// Map form: integers + string prerelease/build, and List identifiers.
	out, err := makeSemveron(mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3)))
	if err != nil || out[0].String() != "1.2.3" {
		t.Errorf("map basic = %v (%v)", out, err)
	}
	out, err = makeSemveron(mapOf(
		"major", NewInteger(1), "minor", NewInteger(0), "patch", NewInteger(0),
		"prerelease", NewString("rc.1"), "build", NewString("b.5")))
	if err != nil || out[0].String() != "1.0.0-rc.1+b.5" {
		t.Errorf("map string-fields = %v (%v)", out, err)
	}
	out, err = makeSemveron(mapOf(
		"major", NewInteger(1), "minor", NewInteger(0), "patch", NewInteger(0),
		"prerelease", NewList([]Value{NewString("rc"), NewString("1")}),
		"build", NewList([]Value{NewString("b"), NewString("5")})))
	if err != nil || out[0].String() != "1.0.0-rc.1+b.5" {
		t.Errorf("map list-fields = %v (%v)", out, err)
	}

	// Negative space — strings.
	for _, bad := range []string{
		"", "v1.2.3", "V1.2.3", "1.2", "1.2.3.4", "01.2.3", "1.02.3", "1.2.03",
		"1..3", "1.a.3", "99999999999999999999.0.0",
		"1.2.3-", "1.2.3-01", "1.2.3-rc..1", "1.2.3-rc_1", "1.2.3-+b",
		"1.2.3+", "1.2.3+bad_char", "1.2.3+b..2", "1.2.3-rc.1+",
	} {
		if _, err := makeSemveron(NewString(bad)); err == nil {
			t.Errorf("bad semver %q accepted", bad)
		}
	}

	// Negative space — map + source.
	badMaps := []Value{
		mapOf("major", NewInteger(1), "minor", NewInteger(2)),                                                                        // missing patch
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "x", NewInteger(0)),                            // unknown field
		mapOf("major", NewInteger(-1), "minor", NewInteger(0), "patch", NewInteger(0)),                                               // negative
		mapOf("major", NewString("1"), "minor", NewInteger(2), "patch", NewInteger(3)),                                               // non-integer
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "prerelease", NewInteger(9)),                   // bad prerelease type
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "build", NewBoolean(true)),                     // bad build type
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "prerelease", NewList([]Value{NewInteger(5)})), // bad list element
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "prerelease", NewString("")),                   // explicit empty prerelease → '1.2.3-' rejected
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "build", NewString("")),                        // explicit empty build → '1.2.3+' rejected
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "prerelease", NewList([]Value{})),              // empty prerelease list
		mapOf("major", NewInteger(1), "minor", NewInteger(2), "patch", NewInteger(3), "prerelease", NewList([]Value{NewString("")})), // single empty prerelease identifier
	}
	for i, m := range badMaps {
		if _, err := makeSemveron(m); err == nil {
			t.Errorf("bad map #%d accepted", i)
		}
	}
	if _, err := makeSemveron(NewInteger(42)); err == nil {
		t.Error("integer source accepted")
	}
	if _, err := makeSemveron(NewBoolean(true)); err == nil {
		t.Error("boolean source accepted")
	}
	// Unreadable map / list payloads exercise the RequireConcrete* error paths.
	if _, err := makeSemveron(ValueOf(&Datum{Parent: TMap, Data: ListPayload{}})); err == nil {
		t.Error("unreadable map payload accepted")
	}
	if _, err := semveronIdentField(ValueOf(&Datum{Parent: TList, Data: MapPayload{M: NewOrderedMap()}}), "prerelease"); err == nil {
		t.Error("unreadable list payload accepted")
	}
}

// TestSemveronOrdering pins the custom SemVer 2.0.0 precedence order
// through the family Comparer and directly against compareSemverons and
// its helpers, covering every branch.
func TestSemveronOrdering(t *testing.T) {
	mk := func(s string) Value {
		out, err := makeSemveron(NewString(s))
		if err != nil {
			t.Fatalf("makeSemveron(%q): %v", s, err)
		}
		return out[0]
	}
	lt := func(a, b string) {
		if c := compareSemverons(mk(a), mk(b)); c != -1 {
			t.Errorf("%s cmp %s = %d, want -1", a, b, c)
		}
		if c := compareSemverons(mk(b), mk(a)); c != 1 {
			t.Errorf("%s cmp %s = %d, want 1 (reverse)", b, a, c)
		}
	}
	// The canonical precedence chain (semver.org §11).
	lt("1.0.0", "2.0.0")      // major
	lt("2.0.0", "2.1.0")      // minor
	lt("2.1.0", "2.1.1")      // patch
	lt("1.9.0", "1.10.0")     // numeric, not lexical
	lt("1.0.0-rc.1", "1.0.0") // prerelease precedes release
	lt("1.0.0-alpha", "1.0.0-alpha.1")
	lt("1.0.0-alpha.1", "1.0.0-alpha.beta")
	lt("1.0.0-alpha.beta", "1.0.0-beta")
	lt("1.0.0-beta.2", "1.0.0-beta.11")
	lt("1.0.0-beta.11", "1.0.0-rc.1")
	lt("1.0.0-1", "1.0.0-1.0")  // shorter identifier set precedes longer
	lt("1.0.0-99", "1.0.0-100") // numeric identifiers: longer digit string is larger
	lt("1.2.3+a", "1.2.3+b")    // build tiebreak (present vs present)
	lt("1.2.3", "1.2.3+b")      // build tiebreak (absent < present)

	// Equal renders compare equal.
	if c := compareSemverons(mk("1.2.3-rc.1+b"), mk("1.2.3-rc.1+b")); c != 0 {
		t.Errorf("equal semvers cmp = %d, want 0", c)
	}

	// The family Comparer routes cmp/lt through compareSemverons.
	c, err := CompareValues(mk("1.9.0"), mk("1.10.0"))
	if err != nil || c != -1 {
		t.Errorf("CompareValues via behavior = %d (%v)", c, err)
	}

	// Direct helper coverage.
	if cmpInt64(1, 2) != -1 || cmpInt64(2, 1) != 1 || cmpInt64(2, 2) != 0 {
		t.Error("cmpInt64 wrong")
	}
	if comparePrerelease("a", "a") != 0 {
		t.Error("equal prerelease != 0")
	}
	if comparePrerelease("", "a") != 1 || comparePrerelease("a", "") != -1 {
		t.Error("release vs prerelease ordering wrong")
	}
	if comparePrereleaseIdent("1", "alpha") != -1 || comparePrereleaseIdent("alpha", "1") != 1 {
		t.Error("numeric vs alphanumeric ordering wrong")
	}
	if comparePrereleaseIdent("11", "2") != 1 { // longer digit string is larger
		t.Error("numeric length ordering wrong")
	}
	if comparePrereleaseIdent("10", "11") != -1 { // same length → lex
		t.Error("numeric same-length lex wrong")
	}
	if comparePrereleaseIdent("beta", "beta") != 0 {
		t.Error("equal non-numeric ident != 0")
	}
	if compareSemverBuild("", "") != 0 || compareSemverBuild("", "a") != -1 ||
		compareSemverBuild("a", "") != 1 || compareSemverBuild("a", "b") != -1 {
		t.Error("compareSemverBuild wrong")
	}

	// compareSemverons falls back to the canonical-key lex when a side is
	// not a Semveron payload (defensive — never happens via the Comparer).
	if compareSemverons(NewInteger(1), NewInteger(2)) == 0 {
		t.Error("non-payload fallback returned equal")
	}

	// micronFieldInt defaults an absent field to 0.
	if micronFieldInt(NewOrderedMap(), "major") != 0 {
		t.Error("micronFieldInt absent != 0")
	}
	// semveronParts on an empty string is the empty list.
	if l, _ := AsList(semveronParts("")); l.Len() != 0 {
		t.Error("semveronParts(\"\") not empty")
	}
}
