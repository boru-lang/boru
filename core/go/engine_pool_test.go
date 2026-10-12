package core

import "testing"

// The pool contract (engine_pool.go): RunPooled behaves exactly like a
// fresh New(r).Run, engines are reused across sequential invocations,
// nesting acquires distinct engines, errors still release the engine,
// results never alias the pooled tape, forks start with an empty pool,
// and a grown tape is dropped rather than pinned.

func poolTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunPooledMatchesFresh(t *testing.T) {
	r := poolTestRegistry(t)
	prog := func() []Value { return []Value{NewInteger(1), NewInteger(2), NewInteger(3)} }

	fresh, err := New(r).Run(prog())
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ { // rounds 2+ exercise the reuse path
		pooled, perr := RunPooled(r, prog())
		if perr != nil {
			t.Fatal(perr)
		}
		if len(pooled) != len(fresh) {
			t.Fatalf("round %d: pooled len %d, fresh len %d", round, len(pooled), len(fresh))
		}
		for i := range pooled {
			a, _ := AsInteger(pooled[i])
			b, _ := AsInteger(fresh[i])
			if a != b {
				t.Fatalf("round %d: pooled[%d]=%d, fresh[%d]=%d", round, i, a, i, b)
			}
		}
	}
}

func TestRunPooledReusesEngine(t *testing.T) {
	r := poolTestRegistry(t)
	if _, err := RunPooled(r, []Value{NewInteger(1)}); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 1 {
		t.Fatalf("pool size after run = %d, want 1", len(r.enginePool))
	}
	parked := r.enginePool[0]
	if _, err := RunPooled(r, []Value{NewInteger(2)}); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 1 || r.enginePool[0] != parked {
		t.Error("second RunPooled did not reuse the parked engine")
	}
	if !parked.ReuseTape {
		t.Error("pooled engine must have reuseTape set")
	}
}

func TestRunPooledResidualNotClobberedByReuse(t *testing.T) {
	r := poolTestRegistry(t)
	first, err := RunPooled(r, []Value{NewInteger(10), NewInteger(20)})
	if err != nil {
		t.Fatal(err)
	}
	// The next pooled run reloads the same tape; a residual that aliased
	// the tape buffer would now read 77s.
	if _, err := RunPooled(r, []Value{NewInteger(77), NewInteger(77)}); err != nil {
		t.Fatal(err)
	}
	a, _ := AsInteger(first[0])
	b, _ := AsInteger(first[1])
	if a != 10 || b != 20 {
		t.Fatalf("first residual clobbered by reuse: got [%d %d], want [10 20]", a, b)
	}
}

func TestRunPooledNestedAcquiresDistinctEngines(t *testing.T) {
	r := poolTestRegistry(t)
	var outer, inner *Engine
	r.Register("pooltest-nest", Signature{
		Args:    []*Type{TInteger},
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			// Record which engine is live for the enclosing pooled run,
			// then re-enter the pool from inside it.
			outer = reg.debugEngines[len(reg.debugEngines)-1]
			res, err := RunPooled(reg, []Value{NewInteger(41), NewInteger(1)})
			if err != nil {
				return nil, err
			}
			inner = nil
			if len(reg.enginePool) > 0 {
				inner = reg.enginePool[len(reg.enginePool)-1]
			}
			n, _ := AsInteger(args[0])
			m, _ := AsInteger(res[len(res)-1])
			_ = m
			return []Value{NewInteger(n + 1)}, nil
		}),
	})
	res, err := RunPooled(r, []Value{NewInteger(5), NewWord("pooltest-nest")})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := AsInteger(res[len(res)-1]); n != 6 {
		t.Fatalf("nested pooled result = %d, want 6", n)
	}
	if outer == nil || inner == nil {
		t.Fatal("nested run did not record engines")
	}
	if outer == inner {
		t.Error("nested RunPooled reused the LIVE outer engine — pool must pop on acquire")
	}
	if len(r.enginePool) != 2 {
		t.Errorf("pool size after nested runs = %d, want 2", len(r.enginePool))
	}
}

func TestRunPooledErrorStillReleases(t *testing.T) {
	r := poolTestRegistry(t)
	if _, err := RunPooled(r, []Value{NewWord("no-such-word-xyz")}); err == nil {
		t.Fatal("undefined word must error under RunPooled, as under a fresh sub-engine")
	}
	if len(r.enginePool) != 1 {
		t.Fatalf("pool size after error = %d, want 1 (engine must be released)", len(r.enginePool))
	}
	res, err := RunPooled(r, []Value{NewInteger(9)})
	if err != nil {
		t.Fatalf("pooled run after error: %v", err)
	}
	if n, _ := AsInteger(res[0]); n != 9 {
		t.Fatalf("post-error pooled run = %d, want 9", n)
	}
}

func TestRunPooledTopResetsOnRelease(t *testing.T) {
	r := poolTestRegistry(t)
	if _, err := RunPooledTop(r, []Value{NewInteger(1)}); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 1 {
		t.Fatalf("pool size = %d, want 1", len(r.enginePool))
	}
	if r.enginePool[0].IsTop {
		t.Error("released engine kept isTop set — top-ness must not leak into later pooled runs")
	}
}

func TestForkConcurrentStartsWithEmptyPool(t *testing.T) {
	r := poolTestRegistry(t)
	if _, err := RunPooled(r, []Value{NewInteger(1)}); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) == 0 {
		t.Fatal("precondition: parent pool must be non-empty")
	}
	fork := r.ForkConcurrent()
	if fork.enginePool != nil {
		t.Error("fork inherited the parent's engine pool — pooled engines alias the parent registry")
	}
}

func TestRunPooledDropsGrownTape(t *testing.T) {
	t.Parallel()
	r := poolTestRegistry(t)
	// A program longer than pooledTapeMaxEntries forces a tape whose
	// capacity exceeds the parking threshold; the engine must be dropped,
	// not pinned in the pool.
	big := make([]Value, pooledTapeMaxEntries+100)
	for i := range big {
		big[i] = NewInteger(int64(i))
	}
	if _, err := RunPooled(r, big); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 0 {
		t.Fatalf("pool size after oversized run = %d, want 0 (grown tape must be dropped)", len(r.enginePool))
	}
	// And a normal-sized run afterwards parks a floor-sized engine again.
	if _, err := RunPooled(r, []Value{NewInteger(1)}); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 1 {
		t.Fatalf("pool size after normal run = %d, want 1", len(r.enginePool))
	}
}

// TestPutSubEngineClearsScratch pins that returning a sub-engine to the pool
// releases the forward-collection scratch buffer (rrValues): its slots are
// zeroed to capacity so a pooled idle engine cannot pin a prior call's
// (possibly large) collected-arg payloads, while the backing array is
// retained for reuse.
func TestPutSubEngineClearsScratch(t *testing.T) {
	r := poolTestRegistry(t)
	e := r.TakeSubEngine()
	e.rrValues = append(e.rrValues, NewInteger(1), NewInteger(2))
	r.PutSubEngine(e)
	if len(r.enginePool) != 1 {
		t.Fatalf("engine not pooled: pool size %d", len(r.enginePool))
	}
	pooled := r.TakeSubEngine() // LIFO: the same engine back
	if len(pooled.rrValues) != 0 {
		t.Fatalf("scratch not reset to empty: rrValues len %d", len(pooled.rrValues))
	}
	for i, v := range pooled.rrValues[:cap(pooled.rrValues)] {
		if v != (Value{}) {
			t.Fatalf("rrValues[%d] not zeroed after pooling: %+v", i, v)
		}
	}
}

// …and every other per-run Value scratch buffer the same way (exceptions
// list C2): a pooled engine pins no Value of its last run.
func TestPutSubEngineClearsEveryScratch(t *testing.T) {
	r := poolTestRegistry(t)
	e := r.TakeSubEngine()
	e.loopTokens = append(e.loopTokens, NewInteger(1))
	e.peScratch = append(e.peScratch, NewInteger(2))
	e.resolvedScratch = append(e.resolvedScratch, NewInteger(3))
	e.excludeScratch = map[int]bool{4: true}
	r.PutSubEngine(e)
	pooled := r.TakeSubEngine() // LIFO: the same engine back
	for name, buf := range map[string][]Value{"loopTokens": pooled.loopTokens, "peScratch": pooled.peScratch, "resolvedScratch": pooled.resolvedScratch} {
		if len(buf) != 0 {
			t.Fatalf("%s not reset to empty: len %d", name, len(buf))
		}
		full := buf[:cap(buf)]
		for i := range full {
			if full[i] != (Value{}) {
				t.Fatalf("%s[%d] not zeroed after pooling", name, i)
			}
		}
	}
	if len(pooled.excludeScratch) != 0 {
		t.Fatalf("excludeScratch not cleared: %v", pooled.excludeScratch)
	}
}
