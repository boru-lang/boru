package core

import "testing"

// TestMatchDispatchRegistry pins the one reading of a nil MatchResult.Reg: a
// delegation dispatch names the module sub-registry owning the signature, and
// any other dispatch matched in main.
func TestMatchDispatchRegistry(t *testing.T) {
	main, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := (&MatchResult{}).DispatchRegistry(main); got != main {
		t.Fatalf("a nil Reg: got %p, want main %p", got, main)
	}
	if got := (&MatchResult{Reg: sub}).DispatchRegistry(main); got != sub {
		t.Fatalf("a delegation Reg: got %p, want the sub-registry %p", got, sub)
	}
}
