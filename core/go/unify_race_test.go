package core

import (
	"sync"
	"testing"
)

// TestUnifyRegistryArmedConcurrentNoRace pins the registry threading of
// the unifier: the process runs many engines on many goroutines (timer
// and interval bodies, the net acceptor's per-connection forks, spawned
// forks), and UnifyExplainR sits on hot paths in every one of them
// (class-instance construction, conditionals, generics instantiation,
// the unify / type words). The registry an armed unify consults inside
// its family handlers must therefore travel with the call, never
// through package state — a package-level stack shared between
// goroutines is a data race (`go test -race` reports the write in one
// goroutine's UnifyExplainR against the read in another's unifyInner)
// and, beyond the detector, can lose a push, hand a handler a stale
// registry, or index out of range.
//
// Several goroutines, each with its OWN armed registry, loop over
// registry-armed unifies whose handlers recurse (typed list vs concrete
// list, typed map vs concrete map, a predicate-typed literal vs a
// candidate, a disjunct with a predicate alternative) plus a plain
// Unify. Every result is asserted; under -race the test also asserts
// that no goroutine's registry leaks into another's.
func TestUnifyRegistryArmedConcurrentNoRace(t *testing.T) {
	const workers = 4
	const rounds = 300

	type outcome struct {
		worker int
		err    string
	}
	results := make(chan outcome, workers*8)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			r, err := NewRegistry()
			if err != nil {
				results <- outcome{w, "NewRegistry: " + err.Error()}
				return
			}
			r.InitRootContext()
			fn := x5PredFn()
			if err := InstallType(r, "RacePos", fn); err != nil {
				results <- outcome{w, "InstallType: " + err.Error()}
				return
			}
			def := r.LookupTypeName("RacePos")
			if def == nil {
				results <- outcome{w, "LookupTypeName(RacePos) = nil"}
				return
			}
			typedList := NewTypedList(NewTypeLiteral(TInteger))
			concreteList := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
			typedMap := NewTypedMap(NewTypeLiteral(TInteger))
			om := NewOrderedMap()
			om.Set("a", NewInteger(1))
			om.Set("b", NewInteger(2))
			concreteMap := NewMap(om)
			predList := NewTypedList(NewTypeLiteral(def))
			disj := NewDisjunct([]Value{fn, NewTypeLiteral(TString)})

			for i := 0; i < rounds; i++ {
				// Typed list vs concrete list: recurses through the list
				// family (unifyZip → unifyInner per element).
				got, uerr := UnifyExplainR(typedList, concreteList, r)
				if uerr != nil {
					results <- outcome{w, "typed list: " + uerr.Error()}
					return
				}
				if lst, lerr := AsList(got); lerr != nil || lst.Len() != 3 {
					results <- outcome{w, "typed list: unexpected result"}
					return
				}
				// Typed map vs concrete map: recurses through the map
				// family (unifyMapValues → unifyInner per value).
				got, uerr = UnifyExplainR(typedMap, concreteMap, r)
				if uerr != nil {
					results <- outcome{w, "typed map: " + uerr.Error()}
					return
				}
				if m, merr := AsMap(got); merr != nil || m.Len() != 2 {
					results <- outcome{w, "typed map: unexpected result"}
					return
				}
				// Predicate-typed literal vs a candidate: the registry
				// pre-pass resolves the node and runs the body.
				got, uerr = UnifyExplainR(NewTypeLiteral(def), NewInteger(5), r)
				if uerr != nil {
					results <- outcome{w, "predicate literal: " + uerr.Error()}
					return
				}
				if n, _ := AsInteger(got); n != 5 {
					results <- outcome{w, "predicate literal: expected 5"}
					return
				}
				// A predicate-typed list child: the element walk consults
				// the registry at every element.
				got, uerr = UnifyExplainR(predList, concreteList, r)
				if uerr != nil {
					results <- outcome{w, "predicate list: " + uerr.Error()}
					return
				}
				if lst, lerr := AsList(got); lerr != nil || lst.Len() != 3 {
					results <- outcome{w, "predicate list: unexpected result"}
					return
				}
				// A disjunct carrying a predicate-fn alternative routes
				// through the registry-aware disjunct walk.
				got, uerr = UnifyExplainR(disj, NewInteger(7), r)
				if uerr != nil {
					results <- outcome{w, "predicate disjunct: " + uerr.Error()}
					return
				}
				if n, _ := AsInteger(got); n != 7 {
					results <- outcome{w, "predicate disjunct: expected 7"}
					return
				}
				// A plain (unarmed) unify interleaved with the armed ones.
				if v, ok := Unify(typedList, concreteList); !ok {
					results <- outcome{w, "plain Unify failed"}
					return
				} else if lst, lerr := AsList(v); lerr != nil || lst.Len() != 3 {
					results <- outcome{w, "plain Unify: unexpected result"}
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(results)
	for o := range results {
		t.Errorf("worker %d: %s", o.worker, o.err)
	}
}
