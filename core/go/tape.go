package core

import "fmt"

// Tape — the engine's gap-buffer tape (see design/legacy/TAPE-DATA-STRUCTURE.10.ignore).
//
// The engine executes on a tape of Values with a cursor (Engine.pointer)
// that mostly moves forward, and every structural edit — body splice,
// forward parking, result splice — lands at or within a few tokens of
// the cursor. The previous flat []Value representation made each such
// edit memmove the entire tail beyond the edit point; during recursion
// the tail held every enclosing call's pending continuation, so the cost
// was O(depth) per edit and O(depth²) overall (measured: 95.9% of a deep
// recursion's runtime was runtime.memmove — design/legacy/RECURSION-PERFORMANCE.10.ignore).
//
// A gap buffer is the text-editor answer to exactly this access pattern
// (Emacs uses one per buffer): the storage keeps one hole (the gap) at
// the cursor. Logical layout:
//
//	indices  0 … gapStart-1   [ gap ]   gapStart … Len()-1
//	physical buf[:gapStart]  (unused)  buf[gapEnd:]
//
// Edits AT the gap are O(edit size): insert writes into the hole, delete
// widens it. Moving the cursor k positions moves k Values across the
// hole. The tail beyond the gap NEVER moves for an edit at the gap — the
// recursion tail therefore stays put and deep recursion becomes O(depth)
// total instead of O(depth²).
//
// Random access stays O(1) (one branch translates a logical index across
// the gap), and both regions are contiguous, so the two scan directions
// the engine uses (backward over collected values, forward over future
// tokens) remain cache-friendly contiguous walks.
//
// Bounded growth (never allow infinite consumption). The buffer starts
// at an initial capacity and may grow at most MaxGrows times, each by
// GrowthFactor, giving a hard ceiling of InitialSize·GrowthFactorᴺ
// entries. A program that would exceed it — a runaway that splices onto
// the tape without bound — latches `exhausted`; the engine detects that
// and fails loudly with a tape_exhausted error rather than allocating
// without limit. Crossing 90/95/99% of the ceiling emits a one-time
// warning. The defaults (N=6, M=2.7) and the initial size are
// configurable via TapeConfig / lang.Options.
type Tape struct {
	buf      []Value
	gapStart int // physical index of the first gap slot == logical gap position
	gapEnd   int // physical index one past the last gap slot

	// forwards counts the Forward cells currently in the logical tape.
	// It exists so pendingForwardIdx can answer its dominant case — "is
	// anything collecting this value?" — WITHOUT walking the stack: a tape
	// holding zero Forwards cannot yield an index, so the answer is -1 by
	// construction. Without it that walk is O(stack depth) per literal push
	// and per word dispatch, which is O(n^2) over a program that accumulates
	// a residual with no paren or forward barrier in it — `for 8000 [add 1
	// 2]` spent 32M scan steps (avg walk 668 cells) before this counter.
	//
	// It is maintained HERE, from the writes themselves, rather than by the
	// engine: every content mutation goes through Set/Insert/Remove/Splice
	// (plus construction and Reload), so the count cannot drift from what is
	// actually on the tape. MoveGap and grow relocate cells without changing
	// the multiset, so they leave it alone. TestTapeForwardCountInvariant
	// pins it against a recount.
	forwards int

	maxCap    int          // hard ceiling on len(buf) (entries)
	maxGrows  int          // remaining reallocations allowed
	grows0    int          // original maxGrows, restored by Reload
	factor    float64      // per-grow multiplier
	exhausted bool         // set once the ceiling is hit; engine fails loudly
	warned    [3]bool      // 90% / 95% / 99% warnings emitted
	warn      func(string) // sink for capacity warnings (nil = silent)
}

// Bounded-growth defaults.
const (
	DefaultTapeMaxGrows     = 6   // N: maximum reallocations
	DefaultTapeGrowthFactor = 2.7 // M: per-grow size multiplier
	// DefaultTapeInitialFloor is the minimum initial capacity (entries)
	// when the caller does not pin one: small programs still get enough
	// headroom that the growth ceiling (floor·Mᴺ ≈ floor·387, ~397k
	// entries ≈ 64MB of 160-byte Values) comfortably covers deep-but-
	// real NON-tail recursion (~13 parked tokens per level ⇒ ~30k
	// depth) while bounding a runaway's memory. Tail recursion needs
	// no headroom at all under tail-call elimination
	// (design/legacy/TCO-STAGED.10.ignore Stage 6); that guarantee is what brought
	// the ceiling down from N=7 (~1.07M entries, ~171MB), whose extra
	// range existed for deep tail chains that now run in O(1).
	DefaultTapeInitialFloor = 1024
	// maxIntCap clamps the computed ceiling so an absurd factor/N cannot
	// overflow int. ~268M entries is far past any real need and a hard stop.
	maxIntCap = 1 << 28
)

// TapeConfig parameterises a Tape's bounded growth. The zero value means
// "use defaults": InitialSize 0 derives from the program size, MaxGrows
// 0 → DefaultTapeMaxGrows, GrowthFactor 0 → DefaultTapeGrowthFactor.
type TapeConfig struct {
	InitialSize  int     // starting capacity in entries; 0 = derive from program
	MaxGrows     int     // N: max reallocations; 0 = default
	GrowthFactor float64 // M: per-grow multiplier; 0 = default
}

// resolve fills zero fields with defaults given the program length.
func (c TapeConfig) Resolve(progLen int) (initial, maxGrows int, factor float64) {
	maxGrows = c.MaxGrows
	if maxGrows <= 0 {
		maxGrows = DefaultTapeMaxGrows
	}
	factor = c.GrowthFactor
	if factor <= 1 {
		factor = DefaultTapeGrowthFactor
	}
	initial = c.InitialSize
	if initial <= 0 {
		initial = progLen + StackHeadroom
		if initial < DefaultTapeInitialFloor {
			initial = DefaultTapeInitialFloor
		}
	}
	if initial < progLen {
		initial = progLen // must at least hold the program
	}
	return initial, maxGrows, factor
}

// NewTape builds a tape holding vals with the gap at the end and DEFAULT
// growth bounds. The input is copied, mirroring Engine.Run's copy of its
// program. headroom only nudges the initial size; the bounded-growth
// ceiling comes from the defaults.
func NewTape(vals []Value, headroom int) *Tape {
	if headroom < 0 {
		headroom = 0
	}
	return NewTapeWith(vals, TapeConfig{InitialSize: len(vals) + headroom}, nil)
}

// growthCeiling computes the bounded-growth ceiling initial·factorᴺ in float
// (to avoid overflow), clamped to [initial, maxIntCap]. It is the single source
// of the growth-ceiling arithmetic, shared by NewTapeWith (the tape's own cap)
// and vmStackCeiling (the VM value-stack / frame cap) so both engines fail with
// the same resource taxonomy at the same bound.
func GrowthCeiling(initial, maxGrows int, factor float64) int {
	ceil := float64(initial)
	for i := 0; i < maxGrows; i++ {
		ceil *= factor
	}
	capped := maxIntCap
	if ceil < float64(maxIntCap) {
		capped = int(ceil)
	}
	if capped < initial { // never below the starting capacity
		capped = initial
	}
	return capped
}

// NewTapeWith builds a tape holding vals with bounded growth configured
// by cfg. warn (may be nil) receives one-time 90/95/99% capacity
// warnings.
func NewTapeWith(vals []Value, cfg TapeConfig, warn func(string)) *Tape {
	// resolve guarantees initial >= len(vals) (its final clamp raises
	// initial to progLen), so no re-check is needed here.
	initial, maxGrows, factor := cfg.Resolve(len(vals))
	buf := make([]Value, initial)
	copy(buf, vals)
	return &Tape{
		buf:      buf,
		forwards: countForwards(vals),
		gapStart: len(vals),
		gapEnd:   initial,
		maxCap:   GrowthCeiling(initial, maxGrows, factor),
		maxGrows: maxGrows,
		grows0:   maxGrows,
		factor:   factor,
		warn:     warn,
	}
}

// Reload re-seeds the tape with a new program IN PLACE, reusing the
// existing backing array when it is large enough. It returns false when
// the array is too small (the caller then builds a fresh tape). Used by
// a reusable sub-engine (the bytecode VM's island runner) so a hot
// fallback island in a loop does not allocate a tape per execution. The
// grow budget and exhaustion/warn flags reset to their original state;
// vals is copied, so the caller's slice is never mutated.
func (t *Tape) Reload(vals []Value) bool {
	if cap(t.buf) < len(vals) {
		return false
	}
	n := cap(t.buf)
	t.buf = t.buf[:n]
	copy(t.buf, vals)
	for i := len(vals); i < n; i++ {
		t.buf[i] = Value{} // clear stale entries in the gap region
	}
	t.gapStart = len(vals)
	t.gapEnd = n
	t.forwards = countForwards(vals)
	t.maxGrows = t.grows0
	t.exhausted = false
	t.warned = [3]bool{}
	return true
}

// Exhausted reports whether the tape hit its growth ceiling. The engine
// checks this each step and fails loudly when set.
func (t *Tape) Exhausted() bool { return t.exhausted }

// Cap returns the current buffer capacity in entries.
func (t *Tape) Cap() int { return len(t.buf) }

// MaxCap returns the hard growth ceiling in entries.
func (t *Tape) MaxCap() int { return t.maxCap }

// Len reports the number of logical elements.
func (t *Tape) Len() int { return len(t.buf) - (t.gapEnd - t.gapStart) }

// capEntries reports the tape's current physical capacity in entries —
// the size of the backing buffer, independent of the gap. Used by the
// registry's engine pool to drop engines whose tape grew too large to
// keep around.
func (t *Tape) capEntries() int { return len(t.buf) }

// countForwards tallies the Forward cells in vals — the seed for the
// tape's running count at construction and Reload.
func countForwards(vals []Value) int {
	n := 0
	for i := range vals {
		if IsForward(vals[i]) {
			n++
		}
	}
	return n
}

// hasForward reports whether any Forward cell is on the tape. A false
// answer lets pendingForwardIdx skip its backward walk: with no Forward
// anywhere, no index can be returned. Nil-safe so a tape-less engine
// (pointer still at 0) answers without a panic.
func (t *Tape) hasForward() bool { return t != nil && t.forwards > 0 }

// phys translates a logical index to a physical one.
func (t *Tape) phys(i int) int {
	if i < t.gapStart {
		return i
	}
	return i + (t.gapEnd - t.gapStart)
}

// At returns the element at logical index i. Out-of-range reads return
// the zero Value (never panic — ADR-005); the engine bounds-checks its
// pointer before reading, so this is a belt-and-braces guard.
func (t *Tape) At(i int) Value {
	if i < 0 || i >= t.Len() {
		return Value{}
	}
	return t.buf[t.phys(i)]
}

// Set replaces the element at logical index i. Out-of-range writes are
// ignored (never panic — ADR-005).
func (t *Tape) Set(i int, v Value) {
	if i < 0 || i >= t.Len() {
		return
	}
	p := t.phys(i)
	if IsForward(t.buf[p]) {
		t.forwards--
	}
	if IsForward(v) {
		t.forwards++
	}
	t.buf[p] = v
}

// MoveGap relocates the gap so it sits at logical index i — O(|i - gap|)
// Value copies. The engine calls this implicitly via the edit methods;
// the cursor's step-by-step advance makes the move cheap (usually 0-4).
func (t *Tape) MoveGap(i int) {
	if i < 0 {
		i = 0
	}
	if max := t.Len(); i > max {
		i = max
	}
	switch {
	case i < t.gapStart:
		n := t.gapStart - i
		copy(t.buf[t.gapEnd-n:t.gapEnd], t.buf[i:t.gapStart])
		zero(t.buf[i : i+min(n, t.gapEnd-t.gapStart)])
		t.gapStart, t.gapEnd = i, t.gapEnd-n
	case i > t.gapStart:
		n := i - t.gapStart
		copy(t.buf[t.gapStart:t.gapStart+n], t.buf[t.gapEnd:t.gapEnd+n])
		zero(t.buf[max(t.gapEnd, t.gapStart+n) : t.gapEnd+n])
		t.gapStart, t.gapEnd = i, t.gapEnd+n
	}
}

// grow widens the gap to at least need slots, reallocating by GrowthFactor
// up to MaxGrows times. Returns false when the growth ceiling is reached:
// the tape latches `exhausted` and the edit that called grow becomes a
// no-op (the engine aborts loudly on the next step, so dropping one edit
// is harmless and avoids any out-of-bounds write). When grow succeeds it
// reports capacity-threshold crossings via the warn sink.
func (t *Tape) grow(need int) bool {
	if t.gapEnd-t.gapStart >= need {
		return true
	}
	if t.maxGrows <= 0 || len(t.buf) >= t.maxCap {
		t.exhausted = true
		return false
	}
	newCap := int(float64(len(t.buf))*t.factor) + need
	if newCap > t.maxCap {
		newCap = t.maxCap
	}
	if newCap-(len(t.buf)-(t.gapEnd-t.gapStart)) < need {
		// Even the ceiling cannot fit this edit's payload.
		t.exhausted = true
		return false
	}
	nb := make([]Value, newCap)
	copy(nb, t.buf[:t.gapStart])
	tail := len(t.buf) - t.gapEnd
	copy(nb[newCap-tail:], t.buf[t.gapEnd:])
	t.buf = nb
	t.gapEnd = newCap - tail
	t.maxGrows--
	t.maybeWarn()
	return true
}

// maybeWarn emits a one-time warning when the buffer capacity first
// crosses 90%, 95%, then 99% of the ceiling. Warning on capacity (not
// live length) gives advance notice while reallocations are still
// possible, before the final fill exhausts the tape.
func (t *Tape) maybeWarn() {
	if t.warn == nil || t.maxCap <= 0 {
		return
	}
	frac := float64(len(t.buf)) / float64(t.maxCap)
	thresholds := [3]float64{0.90, 0.95, 0.99}
	for i, th := range thresholds {
		if !t.warned[i] && frac >= th {
			t.warned[i] = true
			t.warn(fmt.Sprintf("tape at %.0f%% of its %d-entry ceiling (%d entries); raise the initial size or grow count if this is a legitimately large program",
				th*100, t.maxCap, len(t.buf)))
		}
	}
}

// Insert places v at logical index i, shifting later elements right
// (logically). O(gap distance + 1).
func (t *Tape) Insert(i int, v Value) {
	if i < 0 {
		i = 0
	}
	if max := t.Len(); i > max {
		i = max
	}
	t.MoveGap(i)
	if !t.grow(1) {
		return // ceiling hit; engine aborts loudly on the next step
	}
	t.buf[t.gapStart] = v
	t.gapStart++
	if IsForward(v) {
		t.forwards++
	}
}

// Remove deletes the element at logical index i. O(gap distance).
func (t *Tape) Remove(i int) {
	if i < 0 || i >= t.Len() {
		return
	}
	t.MoveGap(i)
	// The element at logical i sits just after the gap; widen over it.
	if IsForward(t.buf[t.gapEnd]) {
		t.forwards--
	}
	t.buf[t.gapEnd] = Value{} // release references
	t.gapEnd++
}

// Splice replaces count elements starting at logical index i with the
// replacements. O(gap distance + count + len(repl)).
func (t *Tape) Splice(i, count int, repl ...Value) {
	if i < 0 {
		i = 0
	}
	if max := t.Len(); i > max {
		i = max
	}
	if count < 0 {
		count = 0
	}
	if avail := t.Len() - i; count > avail {
		count = avail
	}
	t.MoveGap(i)
	// Consume count elements after the gap.
	for k := 0; k < count; k++ {
		if IsForward(t.buf[t.gapEnd+k]) {
			t.forwards--
		}
		t.buf[t.gapEnd+k] = Value{}
	}
	t.gapEnd += count
	if !t.grow(len(repl)) {
		return // ceiling hit; engine aborts loudly on the next step
	}
	copy(t.buf[t.gapStart:], repl)
	t.gapStart += len(repl)
	t.forwards += countForwards(repl)
}

// Prefix returns the contiguous region below logical index end, when end
// is at or below the gap — the engine's dominant "scan everything before
// the pointer" view (hint scans, reorder candidates, handler stacks).
// With the gap kept at the cursor this is the zero-copy fast path; a
// request crossing the gap falls back to a copy via CopyRange.
func (t *Tape) Prefix(end int) []Value {
	if end < 0 {
		end = 0
	}
	if end > t.Len() {
		end = t.Len()
	}
	if end <= t.gapStart {
		return t.buf[:end]
	}
	return t.CopyRange(0, end)
}

// CopyRange returns a fresh copy of logical [i, j).
func (t *Tape) CopyRange(i, j int) []Value {
	if i < 0 {
		i = 0
	}
	if j > t.Len() {
		j = t.Len()
	}
	if i >= j {
		return nil
	}
	out := make([]Value, j-i)
	for k := range out {
		out[k] = t.buf[t.phys(i+k)]
	}
	return out
}

// Snapshot returns a fresh copy of the whole logical content (trace
// callbacks, end-of-run drains).
func (t *Tape) Snapshot() []Value { return t.CopyRange(0, t.Len()) }

func zero(s []Value) {
	for i := range s {
		s[i] = Value{}
	}
}

// TakeAll moves the gap to the end and returns the full logical content
// as one contiguous slice — the Engine's end-of-Run handoff. Ownership
// transfers to the caller: the tape must not be edited afterwards (the
// Engine builds a fresh tape on every Run, so the lifetimes never
// overlap).
func (t *Tape) TakeAll() []Value {
	t.MoveGap(t.Len())
	return t.buf[:t.gapStart]
}
