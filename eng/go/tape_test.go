package eng

import (
	"math/rand/v2"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// refTape is the trivially-correct reference model: a flat slice with
// the same logical operations. The differential test below drives Tape
// and refTape with the same random op sequence and asserts identical
// logical content after every step.
type refTape struct{ s []core.Value }

func (r *refTape) Len() int                { return len(r.s) }
func (r *refTape) At(i int) core.Value     { return r.s[i] }
func (r *refTape) Set(i int, v core.Value) { r.s[i] = v }
func (r *refTape) Insert(i int, v core.Value) {
	r.s = append(r.s, core.Value{})
	copy(r.s[i+1:], r.s[i:len(r.s)-1])
	r.s[i] = v
}
func (r *refTape) Remove(i int) {
	copy(r.s[i:], r.s[i+1:])
	r.s = r.s[:len(r.s)-1]
}
func (r *refTape) Splice(i, count int, repl ...core.Value) {
	tail := append([]core.Value{}, r.s[i+count:]...)
	r.s = append(r.s[:i], append(repl, tail...)...)
}

func tapeEquals(t *testing.T, tape *core.Tape, ref *refTape, step int) {
	t.Helper()
	if tape.Len() != ref.Len() {
		t.Fatalf("step %d: Len = %d, want %d", step, tape.Len(), ref.Len())
	}
	for i := 0; i < ref.Len(); i++ {
		a, _ := core.AsInteger(tape.At(i))
		b, _ := core.AsInteger(ref.At(i))
		if a != b {
			t.Fatalf("step %d: At(%d) = %d, want %d", step, i, a, b)
		}
	}
}

// TestTapeDifferential drives Tape and the slice reference with the
// same pseudo-random operation sequence (seeded — deterministic) and
// checks full logical equality after every operation.
func TestTapeDifferential(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))
	tape := core.NewTape(nil, 8)
	ref := &refTape{}
	next := int64(0)

	for step := 0; step < 5000; step++ {
		n := ref.Len()
		switch op := rng.IntN(5); {
		case op == 0 || n == 0: // insert
			i := rng.IntN(n + 1)
			v := core.NewInteger(next)
			next++
			tape.Insert(i, v)
			ref.Insert(i, v)
		case op == 1: // remove
			i := rng.IntN(n)
			tape.Remove(i)
			ref.Remove(i)
		case op == 2: // set
			i := rng.IntN(n)
			v := core.NewInteger(next)
			next++
			tape.Set(i, v)
			ref.Set(i, v)
		case op == 3: // splice
			i := rng.IntN(n + 1)
			count := 0
			if i < n {
				count = rng.IntN(n - i + 1)
			}
			repl := make([]core.Value, rng.IntN(4))
			for k := range repl {
				repl[k] = core.NewInteger(next)
				next++
			}
			tape.Splice(i, count, repl...)
			ref.Splice(i, count, repl...)
		default: // gap move (no logical change)
			tape.MoveGap(rng.IntN(n + 1))
		}
		tapeEquals(t, tape, ref, step)
	}
}

func TestTapeBasics(t *testing.T) {
	vals := []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}
	tp := core.NewTape(vals, 4)
	if tp.Len() != 3 {
		t.Fatalf("Len = %d, want 3", tp.Len())
	}
	// NewTape copies its input.
	vals[0] = core.NewInteger(99)
	if n, _ := core.AsInteger(tp.At(0)); n != 1 {
		t.Error("NewTape aliased the caller's slice")
	}

	tp.Insert(1, core.NewInteger(10))   // 1 10 2 3
	tp.Remove(3)                        // 1 10 2
	tp.Splice(0, 2, core.NewInteger(7)) // 7 2
	want := []int64{7, 2}
	for i, w := range want {
		if n, _ := core.AsInteger(tp.At(i)); n != w {
			t.Errorf("At(%d) = %d, want %d", i, n, w)
		}
	}
}

func TestTapeOutOfRangeNeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tape panicked on out-of-range op: %v", r)
		}
	}()
	tp := core.NewTape([]core.Value{core.NewInteger(1)}, 0)
	_ = tp.At(-1)
	_ = tp.At(99)
	tp.Set(-1, core.NewInteger(2))
	tp.Set(99, core.NewInteger(2))
	tp.Remove(-1)
	tp.Remove(99)
	tp.Insert(-5, core.NewInteger(3)) // clamps to 0
	tp.Insert(99, core.NewInteger(4)) // clamps to end
	tp.Splice(-1, 99, core.NewInteger(5))
	tp.MoveGap(-3)
	tp.MoveGap(99)
	_ = tp.Prefix(-1)
	_ = tp.CopyRange(5, 2)
}

func TestTapePrefixContiguous(t *testing.T) {
	tp := core.NewTape([]core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}, 4)
	tp.MoveGap(2) // gap at cursor position 2
	p := tp.Prefix(2)
	if len(p) != 2 {
		t.Fatalf("Prefix(2) len = %d, want 2", len(p))
	}
	if n, _ := core.AsInteger(p[1]); n != 2 {
		t.Errorf("Prefix[1] = %d, want 2", n)
	}
	// Crossing the gap falls back to a copy with the right content.
	p = tp.Prefix(3)
	if n, _ := core.AsInteger(p[2]); n != 3 {
		t.Errorf("Prefix(3)[2] = %d, want 3", n)
	}
}

// ---- benchmarks: the recursion access pattern ----
//
// One simulated call level: splice a 12-token body at the cursor, do 14
// insert+remove pairs there, then leave 8 tokens pending and move on —
// the op mix measured from the real engine (design/legacy/RECURSION-PERFORMANCE.10.ignore).
// The slice variant drags the pending tail on every edit; the Tape keeps
// the gap at the cursor so the tail never moves.

func benchSliceLevel(s *[]core.Value, ptr int, body []core.Value) int {
	stackSplice(s, ptr, 0, body...)
	ptr += len(body)
	var v core.Value
	for i := 0; i < 14; i++ {
		stackInsert(s, ptr, v)
		stackRemove(s, ptr)
	}
	return ptr - 8
}

func BenchmarkTapeSliceRecursion(b *testing.B) {
	body := make([]core.Value, 12)
	for depth := 1000; depth <= 4000; depth *= 2 {
		b.Run(itoa(depth), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				s := make([]core.Value, 0, 64)
				ptr := 0
				for d := 0; d < depth; d++ {
					ptr = benchSliceLevel(&s, ptr, body)
				}
			}
		})
	}
}

func BenchmarkTapeGapRecursion(b *testing.B) {
	body := make([]core.Value, 12)
	for depth := 1000; depth <= 4000; depth *= 2 {
		b.Run(itoa(depth), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				tp := core.NewTape(nil, 64)
				ptr := 0
				var v core.Value
				for d := 0; d < depth; d++ {
					tp.Splice(ptr, 0, body...)
					ptr += len(body)
					for i := 0; i < 14; i++ {
						tp.Insert(ptr, v)
						tp.Remove(ptr)
					}
					ptr -= 8
				}
			}
		})
	}
}

// ---- former engine primitives (engine_stack.go), retained as the
// benchmark baseline after the engine moved to the gap-buffer Tape ----

func stackInsert(s *[]core.Value, i int, val core.Value) {
	*s = append(*s, core.Value{})
	copy((*s)[i+1:], (*s)[i:len(*s)-1])
	(*s)[i] = val
}

func stackRemove(s *[]core.Value, i int) {
	copy((*s)[i:], (*s)[i+1:])
	(*s)[len(*s)-1] = core.Value{}
	*s = (*s)[:len(*s)-1]
}

func stackSplice(s *[]core.Value, i, count int, replacements ...core.Value) {
	delta := len(replacements) - count
	oldLen := len(*s)
	newLen := oldLen + delta

	if delta > 0 {
		for cap(*s) < newLen {
			*s = append(*s, core.Value{})
		}
		*s = (*s)[:newLen]
		copy((*s)[i+len(replacements):], (*s)[i+count:oldLen])
	} else if delta < 0 {
		copy((*s)[i+len(replacements):], (*s)[i+count:])
		for j := newLen; j < oldLen; j++ {
			(*s)[j] = core.Value{}
		}
		*s = (*s)[:newLen]
	}
	copy((*s)[i:], replacements)
}

// ---- bounded growth ----

// TestTapeBoundedGrowthCeiling verifies the tape grows at most MaxGrows
// times by GrowthFactor, latches Exhausted at the ceiling, and never
// allocates past it.
func TestTapeBoundedGrowthCeiling(t *testing.T) {
	cfg := core.TapeConfig{InitialSize: 10, MaxGrows: 3, GrowthFactor: 2.0}
	tp := core.NewTapeWith(nil, cfg, nil)
	// ceiling = 10 * 2^3 = 80
	if tp.MaxCap() != 80 {
		t.Fatalf("MaxCap = %d, want 80", tp.MaxCap())
	}
	// Fill well past the ceiling; inserts past it must be dropped and the
	// tape must latch Exhausted rather than grow without bound.
	for i := 0; i < 1000; i++ {
		tp.Insert(tp.Len(), core.NewInteger(int64(i)))
	}
	if !tp.Exhausted() {
		t.Error("tape did not latch Exhausted past its ceiling")
	}
	if tp.Cap() > tp.MaxCap() {
		t.Errorf("buffer cap %d exceeded ceiling %d", tp.Cap(), tp.MaxCap())
	}
	if tp.Len() > tp.MaxCap() {
		t.Errorf("logical len %d exceeded ceiling %d", tp.Len(), tp.MaxCap())
	}
}

// TestTapeWarnings verifies one-time 90/95/99% warnings fire in order.
func TestTapeWarnings(t *testing.T) {
	var msgs []string
	cfg := core.TapeConfig{InitialSize: 10, MaxGrows: 20, GrowthFactor: 1.2}
	tp := core.NewTapeWith(nil, cfg, func(s string) { msgs = append(msgs, s) })
	for i := 0; i < 5000 && !tp.Exhausted(); i++ {
		tp.Insert(tp.Len(), core.NewInteger(int64(i)))
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d warnings, want exactly 3 (90/95/99%%): %v", len(msgs), msgs)
	}
	for i, want := range []string{"90%", "95%", "99%"} {
		if !strings.Contains(msgs[i], want) {
			t.Errorf("warning %d = %q, want it to mention %s", i, msgs[i], want)
		}
	}
}

// TestTapeConfigDefaults verifies the zero config resolves to the
// documented defaults and the initial-size floor.
func TestTapeConfigDefaults(t *testing.T) {
	tp := core.NewTapeWith(nil, core.TapeConfig{}, nil)
	// initial floor 1024, ceiling 1024 * 2.7^6.
	if got := tp.Cap(); got != core.DefaultTapeInitialFloor {
		t.Errorf("default initial cap = %d, want floor %d", got, core.DefaultTapeInitialFloor)
	}
	want := float64(core.DefaultTapeInitialFloor)
	for i := 0; i < core.DefaultTapeMaxGrows; i++ {
		want *= core.DefaultTapeGrowthFactor
	}
	if got := tp.MaxCap(); got != int(want) {
		t.Errorf("default ceiling = %d, want %d", got, int(want))
	}
}

// TestTapeInitialSizeAtLeastProgram: the initial capacity must hold the
// whole program even if the configured initial is smaller.
func TestTapeInitialSizeAtLeastProgram(t *testing.T) {
	prog := make([]core.Value, 50)
	for i := range prog {
		prog[i] = core.NewInteger(int64(i))
	}
	tp := core.NewTapeWith(prog, core.TapeConfig{InitialSize: 10}, nil)
	if tp.Len() != 50 {
		t.Fatalf("Len = %d, want 50", tp.Len())
	}
	if tp.Cap() < 50 {
		t.Errorf("initial cap %d does not hold the %d-entry program", tp.Cap(), 50)
	}
}

// Reload re-seeds the tape in place when the backing array fits (the
// island-engine reuse path), restoring the gap, grow budget, and
// exhaustion/warn flags; it reports false when the array is too small so
// the caller allocates fresh. Stale gap entries must not survive.
func TestTapeReload(t *testing.T) {
	tp := core.NewTapeWith([]core.Value{core.NewInteger(1), core.NewInteger(2)}, core.TapeConfig{}, nil)
	cap0 := tp.Cap()
	if cap0 < 2 {
		t.Fatalf("initial cap %d too small", cap0)
	}

	// Reuse: a program that fits is reloaded in place, same backing array.
	if !tp.Reload([]core.Value{core.NewInteger(9)}) {
		t.Fatal("Reload of a fitting program returned false")
	}
	if tp.Cap() != cap0 {
		t.Errorf("Reload reallocated: cap %d -> %d", cap0, tp.Cap())
	}
	if tp.Len() != 1 {
		t.Fatalf("after Reload Len = %d, want 1", tp.Len())
	}
	if n, _ := tp.At(0).AsConcreteInteger(); n != 9 {
		t.Errorf("Reload content = %d, want 9", n)
	}

	// Negative: a program larger than the backing array declines reuse.
	big := make([]core.Value, cap0+1)
	for i := range big {
		big[i] = core.NewInteger(int64(i))
	}
	if tp.Reload(big) {
		t.Error("Reload of an over-capacity program returned true; must decline so the caller reallocates")
	}
}
