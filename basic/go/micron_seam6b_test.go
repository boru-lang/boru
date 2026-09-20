package basic

// Seam-6b cluster B5: unit tests exercising previously-unreached blocks
// in micron.go (the Micron family Behavior, constructors, and the make
// dispatch) and micron_grammar.go (the merged literal grammar). Tests
// that swap the micronMergedGrammar package var restore it via
// t.Cleanup and do not run in parallel (design/TEST-SEAMS.10.md).

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	tabnas "github.com/tabnas/parser/go"
)

// --- micron.go: Behavior edges ---------------------------------------------

func TestS6b5MicronEqualCrossPayload(t *testing.T) {
	b := micronBehavior{}
	pathon := NewPathon([]string{"a"}, false)
	email := s6b5MustEmailon(t, "alice@example.com")

	if b.Equal(pathon, NewInteger(1)) {
		t.Error("Pathon vs non-Pathon must not be equal")
	}
	if b.Equal(email, NewInteger(1)) {
		t.Error("Micron instance vs non-Micron must not be equal")
	}
	// Neither side a Micron payload: delegate to the default equality.
	if !b.Equal(NewInteger(3), NewInteger(3)) {
		t.Error("default-equality delegation failed for equal integers")
	}
}

func TestS6b5MicronCompareKeyAndRenderFallbacks(t *testing.T) {
	// A non-MicronPayload value keys by its canonical render.
	if got := micronCompareKey(NewInteger(7)); got != CanonValue(NewInteger(7)) {
		t.Errorf("micronCompareKey(7) = %q, want CanonValue", got)
	}
	// micronRender on a non-Micron value falls back to the kernel default.
	if got := micronRender(NewInteger(7)); got != "7" {
		t.Errorf("micronRender(7) = %q, want 7", got)
	}
}

// s6b5MustEmailon builds an Emailon instance through the real constructor.
func s6b5MustEmailon(t *testing.T, s string) Value {
	t.Helper()
	out, err := makeEmailon(NewString(s))
	if err != nil {
		t.Fatalf("makeEmailon(%q): %v", s, err)
	}
	return out[0]
}

// --- micron.go: constructor guard arms --------------------------------------

func TestS6b5MakeEmailonUrlonIponBadMapPayload(t *testing.T) {
	// Parent says Map and the value is "concrete", but the payload is not
	// readable as a map: RequireConcreteMap's error must surface.
	bad := Value{Parent: TMap, Data: ListPayload{}}
	if _, err := makeEmailon(bad); err == nil {
		t.Error("makeEmailon must fail on an unreadable map payload")
	}
	if _, err := makeUrlon(bad); err == nil {
		t.Error("makeUrlon must fail on an unreadable map payload")
	}
	if _, err := makeIpon(bad); err == nil {
		t.Error("makeIpon must fail on an unreadable map payload")
	}
}

func TestS6b5UrlonFromStringPortOverflow(t *testing.T) {
	// url.Parse accepts an all-digit port of any length; the int64
	// conversion must reject the overflow loudly.
	if _, err := urlonFromString("http://h:99999999999999999999/x"); err == nil {
		t.Error("urlonFromString must reject an over-int64 port")
	}
}

func TestS6b5MakeMicronUserEdges(t *testing.T) {
	r := newTestRegistry(t)

	// Unnamed kind: error messages use the family name "Micron".
	_, err := makeMicronUser(MicronTypeInfo{Fields: NewOrderedMap()}, NewInteger(1), r)
	if err == nil || !strings.Contains(err.Error(), "Micron takes a map of fields") {
		t.Errorf("unnamed kind error = %v, want the Micron fallback name", err)
	}

	// Unreadable map payload: RequireConcreteMap's arm.
	bad := Value{Parent: TMap, Data: ListPayload{}}
	if _, err := makeMicronUser(MicronTypeInfo{Name: "S6b5on", Fields: NewOrderedMap()}, bad, r); err == nil {
		t.Error("makeMicronUser must fail on an unreadable map payload")
	}

	// Nil info.Type: the instance parents at the Micron root.
	out, err := makeMicronUser(MicronTypeInfo{Name: "S6b5on", Fields: NewOrderedMap()}, NewMap(NewOrderedMap()), r)
	if err != nil {
		t.Fatalf("makeMicronUser(empty schema): %v", err)
	}
	if len(out) != 1 || out[0].Parent == nil || !out[0].Parent.Equal(TMicron) {
		t.Errorf("nil info.Type must parent at TMicron, got %v", out)
	}
}

func TestS6b5CheckMicronConstructionUnreadableMap(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	// A map-typed source whose payload cannot be read as a map is
	// value-dependent: the check-mode mirror must stay silent.
	src := Value{Parent: TMap, Data: ListPayload{}}
	CheckMicronConstruction(r, NewTypeLiteral(TEmailon), src, SrcPos{})
	if n := len(r.Check.Diagnostics); n != 0 {
		t.Errorf("expected no diagnostics for an unreadable map source, got %d", n)
	}
}

func TestS6b5MicronConstructDoesNotLowerNamedTypeBody(t *testing.T) {
	r := newTestRegistry(t)
	base := NewValueRaw(TMicron, MicronTypeInfo{Name: "S6b5Foon", Fields: NewOrderedMap()})
	_, err := micronConstruct(base, NewMap(NewOrderedMap()), r)
	if err == nil || !strings.Contains(err.Error(), "S6b5Foon") {
		t.Errorf("refine of a named Micron kind must name the kind in the error, got %v", err)
	}
}

// --- micron.go: newtype instantiation walk ----------------------------------

func TestS6b5MicronInstantiateNewtypeOfBuiltinLeaves(t *testing.T) {
	r := newTestRegistry(t)

	// Newtype of Pathon: the parent-chain walk must construct via the
	// Pathon base then reparent to the newtype.
	pn := r.Types.MintType("S6b5Pathnewon", TPathon)
	out, err := micronInstantiate(NewTypeLiteral(pn), NewString("/a/b"), r)
	if err != nil || len(out) != 1 {
		t.Fatalf("newtype-of-Pathon make: %v %v", out, err)
	}
	if out[0].Parent == nil || !out[0].Parent.Equal(pn) {
		t.Errorf("newtype-of-Pathon result parent = %v, want the newtype", out[0].Parent)
	}

	// Newtype of Urlon.
	un := r.Types.MintType("S6b5Urlnewon", TUrlon)
	out, err = micronInstantiate(NewTypeLiteral(un), NewString("http://h/p"), r)
	if err != nil || len(out) != 1 {
		t.Fatalf("newtype-of-Urlon make: %v %v", out, err)
	}
	if out[0].Parent == nil || !out[0].Parent.Equal(un) {
		t.Errorf("newtype-of-Urlon result parent = %v, want the newtype", out[0].Parent)
	}
}

func TestS6b5MicronInstantiateNewtypeOfUserKind(t *testing.T) {
	r := newTestRegistry(t)

	// A user Micron kind, wired the way InstallType's Micron branch does.
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TString))
	kind := r.Types.MintType("S6b5Kindon", TMicron)
	info := MicronTypeInfo{Name: "S6b5Kindon", Type: kind, Fields: fields}
	kind.SetBehavior(micronBehavior{kind: kind, info: &info})

	// A bare nominal newtype of that kind: the walk must find the user
	// kind's schema on the parent chain and construct through it.
	newt := r.Types.MintType("S6b5Kidon", kind)
	data := NewOrderedMap()
	data.Set("a", NewString("x"))
	out, err := micronInstantiate(NewTypeLiteral(newt), NewMap(data), r)
	if err != nil || len(out) != 1 {
		t.Fatalf("newtype-of-user-kind make: %v %v", out, err)
	}
	if out[0].Parent == nil || !out[0].Parent.Equal(newt) {
		t.Errorf("newtype-of-user-kind result parent = %v, want the newtype", out[0].Parent)
	}
}

// --- micron_grammar.go -------------------------------------------------------

func TestS6b5MicronGrammarConstructorCompileFailureFallsToPathon(t *testing.T) {
	// The Emailon gate matches but net/mail declines (consecutive dots):
	// the action's fallback constructs a Pathon instead of failing.
	v, err := MicronFromString("a..b@x.com")
	if err != nil {
		t.Fatalf("MicronFromString(a..b@x.com): %v", err)
	}
	if !v.Parent.ConformsTo(TPathon) {
		t.Errorf("declined email literal should fall back to Pathon, got %s", v.Parent.String())
	}

	// The Urlon gate matches but url.Parse declines (unclosed bracket).
	v, err = MicronFromString("http://[::1")
	if err != nil {
		t.Fatalf("MicronFromString(http://[::1): %v", err)
	}
	if !v.Parent.ConformsTo(TPathon) {
		t.Errorf("declined url literal should fall back to Pathon, got %s", v.Parent.String())
	}
}

// (TestS6b5MicronGrammarWithExtraErrors died with the extras hook — the
// +m literal grammar is fixed to the builtin leaves.)

func TestS6b5MicronFromStringGrammarUnavailable(t *testing.T) {
	saved := micronMergedGrammar
	t.Cleanup(func() { micronMergedGrammar = saved })
	micronMergedGrammar = func() (*tabnas.Tabnas, error) {
		return nil, errors.New("s6b5 grammar down")
	}
	_, err := MicronFromString("x")
	if err == nil || !strings.Contains(err.Error(), "grammar unavailable") {
		t.Errorf("expected the grammar-unavailable error, got %v", err)
	}
}

func TestS6b5MicronFromStringNonValueNode(t *testing.T) {
	// A grammar whose action leaves a non-Value parse node must be
	// rejected with the did-not-produce-a-value error.
	g := tabnas.Make(tabnas.Options{
		Tag: "S6b5NV",
		Match: &tabnas.MatchOptions{
			Token:      map[string]*regexp.Regexp{"#S6B5NV": regexp.MustCompile(`\A\S+\z`)},
			TokenOrder: []string{"#S6B5NV"},
		},
	})
	tin := g.Token("#S6B5NV")
	g.Rule("val", func(rs *tabnas.RuleSpec, _ *tabnas.Parser) {
		rs.AddOpen(&tabnas.AltSpec{
			S: [][]tabnas.Tin{{tin}},
			A: func(r *tabnas.Rule, _ *tabnas.Context) { r.Node = "s6b5-not-a-value" },
		})
	})
	saved := micronMergedGrammar
	t.Cleanup(func() { micronMergedGrammar = saved })
	micronMergedGrammar = func() (*tabnas.Tabnas, error) { return g, nil }

	_, err := MicronFromString("zz")
	if err == nil || !strings.Contains(err.Error(), "did not produce a value") {
		t.Errorf("expected the non-value-node error, got %v", err)
	}
}
