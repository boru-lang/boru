package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- Algebraic laws of the type lattice ---
//
// `tand` (intersection) and `tor` (union) form a bounded distributive
// lattice over boru types:
//
//   - tand identity = Any         (T tand Any = T)
//   - tor identity  = Never       (T tor Never = T)
//   - tand annihilator = Never    (T tand Never = Never)
//   - tor annihilator  = Any      (T tor Any = Any)
//   - idempotence: T tand T = T,  T tor T = T
//   - commutativity: A tand B = B tand A, A tor B = B tor A
//   - associativity over both
//   - distribution: tand distributes over tor (DNF)
//
// Equivalence is checked by membership: two types are equivalent if
// they admit the same set of values via `is`. This is more robust
// than structural comparison, which is sensitive to alternative
// ordering and surface representation.

// equivByMembership runs `v is t1` and `v is t2` for each probe v
// and reports the first probe where the answers differ.
func equivByMembership(t *testing.T, name, t1, t2 string, probes []string) {
	t.Helper()
	for _, v := range probes {
		src := "def t1 (" + t1 + ")\ndef t2 (" + t2 + ")\n" +
			v + " is t1\n" + v + " is t2\n"
		a, err := lang.New()
		if err != nil {
			t.Fatalf("%s: new: %v", name, err)
		}
		got, err := runReference(t, a, src)
		if err != nil {
			t.Fatalf("%s: run %q: %v", name, src, err)
		}
		if len(got) != 2 {
			t.Fatalf("%s: probe %s: got %d results, want 2", name, v, len(got))
		}
		if got[0] != got[1] {
			t.Errorf("%s: probe %s: %s is t1 = %v, %s is t2 = %v (mismatch)",
				name, v, v, got[0], v, got[1])
		}
	}
}

// Probe set covering the lattice across scalar branches and a few
// special values. Adjust per-test if a narrower set is more
// appropriate.
var lawProbes = []string{`42`, `"hi"`, `true`, `1.5`, `None`}

// --- Identities and annihilators ---

func TestLaw_TandIdentityAny(t *testing.T) {
	equivByMembership(t, "tand-identity-Any", `Integer tand Any`, `Integer`, lawProbes)
}

func TestLaw_TorIdentityNever(t *testing.T) {
	equivByMembership(t, "tor-identity-Never", `Integer tor Never`, `Integer`, lawProbes)
}

func TestLaw_TandAnnihilatorNever(t *testing.T) {
	equivByMembership(t, "tand-annihilator-Never", `Integer tand Never`, `Never`, lawProbes)
}

func TestLaw_TorAnnihilatorAny(t *testing.T) {
	equivByMembership(t, "tor-annihilator-Any", `Integer tor Any`, `Any`, lawProbes)
}

// --- Idempotence ---

func TestLaw_TandIdempotent(t *testing.T) {
	for _, body := range []string{`Integer`, `String`, `Integer tor String`} {
		equivByMembership(t, "tand-idempotent",
			body+` tand `+body, body, lawProbes)
	}
}

func TestLaw_TorIdempotent(t *testing.T) {
	for _, body := range []string{`Integer`, `String`, `Integer tor String`} {
		equivByMembership(t, "tor-idempotent",
			body+` tor `+body, body, lawProbes)
	}
}

// --- Commutativity ---

// Operands are parenthesised so left-to-right concatenative
// evaluation doesn't reshape the expression under us when one side
// is itself a tor/tand. `(A) tand (B)` always intersects A with B
// regardless of A's or B's internal shape; bare `A tand B` would
// chain into surrounding ops if A or B contains them.
func TestLaw_TandCommutative(t *testing.T) {
	pairs := [][2]string{
		{`Integer`, `Number`},
		{`Integer tor String`, `Integer`},
		{`Integer tor String`, `String tor Boolean`},
	}
	for _, p := range pairs {
		equivByMembership(t, "tand-commutative",
			`(`+p[0]+`) tand (`+p[1]+`)`,
			`(`+p[1]+`) tand (`+p[0]+`)`,
			lawProbes)
	}
}

func TestLaw_TorCommutative(t *testing.T) {
	pairs := [][2]string{
		{`Integer`, `String`},
		{`Integer tor String`, `Boolean`},
	}
	for _, p := range pairs {
		equivByMembership(t, "tor-commutative",
			`(`+p[0]+`) tor (`+p[1]+`)`,
			`(`+p[1]+`) tor (`+p[0]+`)`,
			lawProbes)
	}
}

// --- Associativity ---

func TestLaw_TandAssociative(t *testing.T) {
	a, b, c := `Integer`, `Number`, `Scalar`
	equivByMembership(t, "tand-associative",
		`(`+a+` tand `+b+`) tand `+c,
		a+` tand (`+b+` tand `+c+`)`,
		lawProbes)
}

func TestLaw_TorAssociative(t *testing.T) {
	a, b, c := `Integer`, `String`, `Boolean`
	equivByMembership(t, "tor-associative",
		`(`+a+` tor `+b+`) tor `+c,
		a+` tor (`+b+` tor `+c+`)`,
		lawProbes)
}

// --- Distribution: tand over tor ---

func TestLaw_TandDistributesOverTor(t *testing.T) {
	// (A tor B) tand C  ≡  (A tand C) tor (B tand C)
	a, b, c := `Integer`, `String`, `Integer tor Boolean`
	equivByMembership(t, "tand-distributes-over-tor",
		`(`+a+` tor `+b+`) tand (`+c+`)`,
		`((`+a+`) tand (`+c+`)) tor ((`+b+`) tand (`+c+`))`,
		lawProbes)
}

// --- Empty-fold identities ---

func TestLaw_EmptyTallIsAny(t *testing.T) {
	// Any matches every concrete value EXCEPT None — None has its
	// own unification rule (only unifies with itself), independent
	// of the lattice. So None is excluded from this probe set.
	got := runOne(t, `def t ([] tall)
42 is t
"hi" is t
true is t
1.5 is t`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	for i, v := range got {
		if v != "true" {
			t.Errorf("probe %d: %v, want true (Any matches everything)", i, v)
		}
	}
}

func TestLaw_EmptyTanyIsNever(t *testing.T) {
	got := runOne(t, `def t ([] tany)
42 is t
"hi" is t
None is t`)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 results", got)
	}
	for i, v := range got {
		if v != "false" {
			t.Errorf("probe %d: %v, want false (Never matches nothing)", i, v)
		}
	}
}

// --- DepScalar tand DepScalar: interval refinement ---

func TestLaw_DepScalarInterval(t *testing.T) {
	// (Integer gte 10) tand (Integer lte 20) = closed interval [10, 20].
	got := runOne(t, `def t ((Integer gte 10) tand (Integer lte 20))
9 is t
10 is t
15 is t
20 is t
21 is t`)
	if len(got) != 5 {
		t.Fatalf("got %v, want 5 results", got)
	}
	want := []string{"false", "true", "true", "true", "false"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Same-side bounds tighten: gte 10 tand gte 5 = gte 10.
func TestLaw_DepScalarSameSideTightens(t *testing.T) {
	got := runOne(t, `def t ((Integer gte 10) tand (Integer gte 5))
4 is t
9 is t
10 is t
100 is t`)
	if len(got) != 4 {
		t.Fatalf("got %v, want 4 results", got)
	}
	want := []string{"false", "false", "true", "true"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Empty interval: gt 10 tand lt 5 = Never.
func TestLaw_DepScalarEmptyInterval(t *testing.T) {
	got := runOne(t, `(Integer gt 10) tand (Integer lt 5)`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("(Int gt 10) tand (Int lt 5) = %v, want [\"Never\"]", got)
	}
}

// Touching strict bounds: gt 10 tand lt 10 = Never.
func TestLaw_DepScalarStrictTouching(t *testing.T) {
	got := runOne(t, `(Integer gt 10) tand (Integer lt 10)`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("(Int gt 10) tand (Int lt 10) = %v, want [\"Never\"]", got)
	}
}

// Equal inclusive bounds: gte 10 tand lte 10 = single value 10.
func TestLaw_DepScalarSingleton(t *testing.T) {
	got := runOne(t, `def t ((Integer gte 10) tand (Integer lte 10))
9 is t
10 is t
11 is t`)
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 results", got)
	}
	want := []string{"false", "true", "false"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Cross-base: DepInteger tand DepString = Never.
func TestLaw_DepScalarCrossBase(t *testing.T) {
	got := runOne(t, `(Integer gte 10) tand (String lt "z")`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("DepInt tand DepStr = %v, want [\"Never\"]", got)
	}
}

// --- between: closed-interval sugar ---
//
// `Integer between 10 20` desugars to (Integer gte 10) tand (Integer
// lte 20) — a closed interval. Same membership semantics, single
// word.

func TestBetween_ClosedInterval(t *testing.T) {
	got := runOne(t, `def t (Integer between 10 20)
9 is t
10 is t
15 is t
20 is t
21 is t`)
	want := []string{"false", "true", "true", "true", "false"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d results", got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// String bounds work too — between is generic over orderable scalars.
func TestBetween_StringInterval(t *testing.T) {
	got := runOne(t, `def t (String between "b" "d")
"a" is t
"b" is t
"c" is t
"d" is t
"e" is t`)
	want := []string{"false", "true", "true", "true", "false"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d results", got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Inverted bounds collapse to Never (between is total).
func TestBetween_InvertedIsNever(t *testing.T) {
	got := runOne(t, `Integer between 20 10`)
	if len(got) != 1 || got[0] != "Never" {
		t.Errorf("Integer between 20 10 = %v, want [\"Never\"]", got)
	}
}

// Equal bounds form a singleton interval.
func TestBetween_SingletonInterval(t *testing.T) {
	got := runOne(t, `def t (Integer between 10 10)
9 is t
10 is t
11 is t`)
	want := []string{"false", "true", "false"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d results", got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("probe %d: %v, want %v", i, got[i], w)
		}
	}
}

// Membership-equivalent to the long form.
func TestBetween_EquivalentToLongForm(t *testing.T) {
	equivByMembership(t, "between-equivalent",
		`Integer between 10 20`,
		`(Integer gte 10) tand (Integer lte 20)`,
		[]string{`5`, `10`, `15`, `20`, `25`, `"hi"`})
}
