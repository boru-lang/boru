package compiler

import (
	"maps"
	"testing"
)

// TestMergeBindConsumes pins mergeBindConsumes' nil-safe union. Both
// callers hand it a fresh map from collectRootBindConsumes or
// collectResidentBindConsumes, so its nil first set is reached only here:
// a nil set with members to add yields a new set of those members; an empty
// second set returns the first unchanged, nil included.
func TestMergeBindConsumes(t *testing.T) {
	if got := mergeBindConsumes(nil, map[int]bool{3: true}); !maps.Equal(got, map[int]bool{3: true}) {
		t.Errorf("nil ∪ {3} = %v, want {3}", got)
	}
	if got := mergeBindConsumes(nil, nil); got != nil {
		t.Errorf("nil ∪ nil = %v, want nil", got)
	}
	a := map[int]bool{1: true}
	if got := mergeBindConsumes(a, map[int]bool{}); !maps.Equal(got, map[int]bool{1: true}) {
		t.Errorf("{1} ∪ {} = %v, want {1}", got)
	}
	if got := mergeBindConsumes(a, map[int]bool{2: true}); !maps.Equal(got, map[int]bool{1: true, 2: true}) {
		t.Errorf("{1} ∪ {2} = %v, want {1 2}", got)
	}
}
