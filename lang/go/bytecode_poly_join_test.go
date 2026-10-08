package lang

import "testing"

// bytecode_poly_join_test.go pins the §8.2(3) RETURN-JOIN graduation
// (checker-compiler-completeness-review.0.md §9.11): a gradual-Any arg to a
// multi-overload user fn whose arms declare DIFFERING return types now
// compiles — tryCompileUserPolyArms records the position-wise join of the
// arms' returns (userPolyPlan.outs, a dynamic carrier at the arms' common
// ancestor), the record's identity survives the first-match-partition
// widening (applyGradualContagion preserves out[0].ID), and the VM's
// OpCallUserPoly re-match selects the concrete arm at run time.
func TestPolyReturnJoinCompiles(t *testing.T) {
	mod := `def id fn [[x:Any] [Any] [x]] def g fn [[a:Integer] [Integer] [1] [a:String] [String] ['s']] `
	fnValueM2Native(t, "Integer arm selected at runtime", mod+`g (id 5)`, "[1]")
	fnValueM2Native(t, "String arm selected at runtime", mod+`g (id 'x')`, "[s]")
	fnValueM2Native(t, "the join consumed by a typed word (runtime re-check)",
		`def id fn [[x:Any] [Any] [x]] def g fn [[a:Integer] [Integer] [1] [a:String] [String] ['s']] (g (id 5)) add 1`,
		"[2]")
}

// TestPolyReturnJoinCountMismatchFailsToCompile pins the edge the join deliberately
// keeps: the call site bakes a FIXED nout, so arms whose return COUNTS
// differ can never share one recorded call — the set declines with faithful
// compile failure (userPolyArmShapeOK's count gate).
func TestPolyReturnJoinCountMismatchFailsToCompile(t *testing.T) {
	fnValueM2CompileFailure(t, "arms with differing return counts",
		`def id fn [[x:Any] [Any] [x]] def g fn [[a:Integer] [Integer] [1] [a:String] [String String] ['a' 'b']] g (id 5)`,
		"ambiguous dispatch, no poly re-match")
}
