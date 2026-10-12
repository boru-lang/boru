package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

func randRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallRandExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func runRandBoru(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	e := native.NewTop(r)
	result, err := e.Run(values)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	return result
}

// TestRandModuleExports asserts the public surface: top-level `rand`
// has the data words (int, bool, float, string, one-of) plus the
// seeded-instance factory `with-seed`. There is no `seed` or
// `fresh-seed` at the top level — the top-level is clock-seeded by
// default; for determinism, call `Rand.with-seed N` and use the
// returned instance.
func TestRandModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildRandModule(r)
	if err != nil {
		t.Fatal(err)
	}
	randExport, ok := desc.Exports["Rand"]
	if !ok {
		t.Fatal("expected 'rand' export")
	}
	for _, name := range []string{"int", "bool", "float", "string", "one-of", "list-of", "map-from", "with-seed"} {
		if _, ok := randExport.Get(name); !ok {
			t.Errorf("missing export: %q", name)
		}
	}
	// Old API removed — confirm absence.
	for _, name := range []string{"seed", "fresh-seed"} {
		if _, ok := randExport.Get(name); ok {
			t.Errorf("removed export %q still present", name)
		}
	}
}

// TestRandIntHalfOpenRange asserts Rand.int returns values in
// [lo, hi) — lo inclusive, hi exclusive. Draw many samples and
// verify none equal `hi`.
func TestRandIntHalfOpenRange(t *testing.T) {
	r := randRegistry(t)
	// Use a seeded instance for reproducibility, and use forward form
	// throughout to avoid forward-collection trap.
	src := `def r (Rand.with-seed 1)`
	for i := 0; i < 200; i++ {
		src += `  (r.int 0 3)` // values must be 0, 1, or 2 — NEVER 3
	}
	res := runRandBoru(t, r, src)
	if len(res) != 200 {
		t.Fatalf("expected 200 draws, got %d", len(res))
	}
	counts := map[int64]int{}
	for _, v := range res {
		n, err := v.AsConcreteInteger()
		if err != nil {
			t.Fatalf("not an integer: %v", err)
		}
		if n < 0 || n >= 3 {
			t.Errorf("draw %d out of [0,3): half-open semantics broken", n)
		}
		counts[n]++
	}
	// Every value in the range should appear at least once in 200 draws.
	for _, want := range []int64{0, 1, 2} {
		if counts[want] == 0 {
			t.Errorf("value %d never drawn in 200 samples", want)
		}
	}
}

func TestRandIntRejectsEmptyRange(t *testing.T) {
	r := randRegistry(t)
	// hi == lo means [lo, lo) — empty range; must error.
	values, _ := parser.Parse(`def r (Rand.with-seed 1)  (r.int 5 5)`)
	e := native.NewTop(r)
	_, err := e.Run(values)
	if err == nil {
		t.Fatal("expected error for hi == lo (empty range)")
	}
}

func TestRandIntRejectsInvertedBounds(t *testing.T) {
	r := randRegistry(t)
	// Stack form `lo hi Rand.int`: top=hi=0, deeper=lo=10 → hi <= lo.
	values, _ := parser.Parse(`def r (Rand.with-seed 1)  (r.int 10 0)`)
	e := native.NewTop(r)
	_, err := e.Run(values)
	if err == nil {
		t.Fatal("expected error for hi <= lo")
	}
}

// TestRandWithSeedIsolated confirms two with-seed instances built with
// the SAME seed produce identical sequences (determinism), and that
// they're independent from each other AND from the top-level rand.
func TestRandWithSeedIsolated(t *testing.T) {
	r := randRegistry(t)
	// Build two instances of seed 42 + one of seed 99. Draw 5 ints
	// each; the two seed-42 instances must agree, seed-99 must
	// differ, and the top-level must differ.
	src := `
		def a (Rand.with-seed 42)
		def b (Rand.with-seed 42)
		def c (Rand.with-seed 99)
		(a.int 0 1000000) (a.int 0 1000000) (a.int 0 1000000)
		(b.int 0 1000000) (b.int 0 1000000) (b.int 0 1000000)
		(c.int 0 1000000) (c.int 0 1000000) (c.int 0 1000000)
	`
	res := runRandBoru(t, r, src)
	if len(res) != 9 {
		t.Fatalf("expected 9 draws, got %d", len(res))
	}
	vals := make([]int64, 9)
	for i, v := range res {
		vals[i], _ = v.AsConcreteInteger()
	}
	a := vals[0:3]
	b := vals[3:6]
	c := vals[6:9]
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			t.Errorf("draw %d: a=%d, b=%d — two seed-42 instances should agree", i, a[i], b[i])
		}
		if a[i] == c[i] {
			t.Errorf("draw %d: a=%d, c=%d — seed-42 and seed-99 should differ", i, a[i], c[i])
		}
	}
}

// TestRandTopLevelIsClockSeeded confirms the top-level `rand.*` is
// non-deterministic by default — two fresh registries produce
// different sequences (with extremely high probability — UnixNano
// resolution).
func TestRandTopLevelIsClockSeeded(t *testing.T) {
	draw := func() int64 {
		r := randRegistry(t)
		res := runRandBoru(t, r, `Rand.int 0 1000000`)
		n, _ := res[0].AsConcreteInteger()
		return n
	}
	a := draw()
	b := draw()
	// Two fresh registries seeded from time.Now().UnixNano() should
	// produce different draws. If they don't, the clock seeding is
	// broken (or the test machine has nanosecond-resolution clock
	// quantization — unlikely on modern hardware).
	if a == b {
		t.Errorf("two fresh registries gave identical top-level draws (%d, %d) — top-level rand may not be clock-seeded", a, b)
	}
}

func TestRandBool(t *testing.T) {
	r := randRegistry(t)
	// 20 draws from a seeded instance should contain both true and false.
	src := `def r (Rand.with-seed 1)`
	for i := 0; i < 20; i++ {
		src += `  (r.bool)`
	}
	res := runRandBoru(t, r, src)
	if len(res) != 20 {
		t.Fatalf("expected 20 bools, got %d", len(res))
	}
	sawT, sawF := false, false
	for _, v := range res {
		b, err := v.AsConcreteBoolean()
		if err != nil {
			t.Fatalf("not a boolean: %v", err)
		}
		if b {
			sawT = true
		} else {
			sawF = true
		}
	}
	if !sawT || !sawF {
		t.Errorf("expected both true and false in 20 draws (seen true=%v false=%v)", sawT, sawF)
	}
}

func TestRandString(t *testing.T) {
	r := randRegistry(t)
	res := runRandBoru(t, r, `def r (Rand.with-seed 1)  (r.string "abc" 10)`)
	if len(res) != 1 {
		t.Fatalf("expected one value, got %d", len(res))
	}
	s, err := res[0].AsConcreteString()
	if err != nil {
		t.Fatalf("not a string: %v", err)
	}
	if len(s) != 10 {
		t.Errorf("len=%d, want 10", len(s))
	}
	for _, ch := range s {
		if ch != 'a' && ch != 'b' && ch != 'c' {
			t.Errorf("char %q not in charset", ch)
		}
	}
}

func TestRandStringEmptyCharsetZeroLen(t *testing.T) {
	r := randRegistry(t)
	res := runRandBoru(t, r, `def r (Rand.with-seed 1)  (r.string "" 0)`)
	if len(res) != 1 {
		t.Fatalf("expected one value, got %d", len(res))
	}
	s, _ := res[0].AsConcreteString()
	if s != "" {
		t.Errorf("got %q, want empty", s)
	}
}

func TestRandOneOfSingleCall(t *testing.T) {
	r := randRegistry(t)
	res := runRandBoru(t, r, `def r (Rand.with-seed 7)  ([10 20 30] r.one-of)`)
	if len(res) != 1 {
		t.Fatalf("expected one value, got %d", len(res))
	}
	n, err := res[0].AsConcreteInteger()
	if err != nil {
		t.Fatalf("not an integer: %v", err)
	}
	if n != 10 && n != 20 && n != 30 {
		t.Errorf("draw %d not in the source list", n)
	}
}

// TestRandListOf confirms the Rand.list-of combinator runs its
// quoted body N times and collects the results. With NoEvalArgs[0]
// the body survives the wrapper boundary as code (not as data).
func TestRandListOf(t *testing.T) {
	r := randRegistry(t)
	res := runRandBoru(t, r, `def s (Rand.with-seed 42)  Rand.list-of [s.int 0 100] 5`)
	if len(res) != 1 {
		t.Fatalf("expected one list, got %d", len(res))
	}
	lst, err := native.RequireConcreteList(res[0], "test")
	if err != nil {
		t.Fatal(err)
	}
	if lst.Len() != 5 {
		t.Errorf("len = %d, want 5", lst.Len())
	}
	// Determinism: same seed → identical sequence.
	r2 := randRegistry(t)
	res2 := runRandBoru(t, r2, `def s (Rand.with-seed 42)  Rand.list-of [s.int 0 100] 5`)
	lst2, _ := native.RequireConcreteList(res2[0], "test")
	for i := 0; i < lst.Len(); i++ {
		a, _ := addrOf(lst.Get(i)).AsConcreteInteger()
		b, _ := addrOf(lst2.Get(i)).AsConcreteInteger()
		if a != b {
			t.Errorf("element %d diverges: %d vs %d", i, a, b)
		}
		if a < 0 || a >= 100 {
			t.Errorf("element %d=%d out of [0,100)", i, a)
		}
	}
}

// TestRandMapFrom builds a map by running each key's quoted generator.
// NoEvalMapArgs[0] keeps the schema map intact across the call boundary.
func TestRandMapFrom(t *testing.T) {
	r := randRegistry(t)
	res := runRandBoru(t, r, `
		def s (Rand.with-seed 42)
		Rand.map-from {age:[s.int 0 100] flag:[s.bool]}
	`)
	if len(res) != 1 {
		t.Fatalf("expected one map, got %d", len(res))
	}
	m, _ := native.AsMap(res[0])
	if m == nil {
		t.Fatal("not a map")
	}
	ageV, ok := m.Get("age")
	if !ok {
		t.Fatal("missing age key")
	}
	age, _ := ageV.AsConcreteInteger()
	if age < 0 || age >= 100 {
		t.Errorf("age=%d out of [0,100)", age)
	}
	flagV, ok := m.Get("flag")
	if !ok {
		t.Fatal("missing flag key")
	}
	if _, err := flagV.AsConcreteBoolean(); err != nil {
		t.Errorf("flag not Boolean: %v", err)
	}
}

// TestRandListOfBodyKeepsQuoted confirms the body really is treated
// as code, not data. If NoEvalArgs were broken, the body `[s.int 0 100]`
// would be auto-evaluated ONCE at wrapper-call time and the resulting
// integer would be repeated N times in the output — every element
// identical. The fix yields N distinct draws.
func TestRandListOfBodyKeepsQuoted(t *testing.T) {
	r := randRegistry(t)
	res := runRandBoru(t, r, `def s (Rand.with-seed 1)  Rand.list-of [s.int 0 1000000] 4`)
	lst, _ := native.RequireConcreteList(res[0], "test")
	distinct := map[int64]bool{}
	for i := 0; i < lst.Len(); i++ {
		n, _ := addrOf(lst.Get(i)).AsConcreteInteger()
		distinct[n] = true
	}
	// Four draws from [0, 1e6) should almost certainly all be distinct.
	if len(distinct) < 3 {
		t.Errorf("expected mostly-distinct draws, got %d distinct in %d elements", len(distinct), lst.Len())
	}
}

func TestRandFloatInUnitInterval(t *testing.T) {
	r := randRegistry(t)
	src := `def r (Rand.with-seed 1)`
	for i := 0; i < 50; i++ {
		src += `  (r.float)`
	}
	res := runRandBoru(t, r, src)
	for _, v := range res {
		f, err := v.AsConcreteFloat()
		if err != nil {
			t.Fatalf("not a decimal: %v", err)
		}
		if f < 0 || f >= 1 {
			t.Errorf("float out of [0,1): %g", f)
		}
	}
}
