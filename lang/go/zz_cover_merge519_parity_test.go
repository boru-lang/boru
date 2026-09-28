package lang

import "testing"

// TestMerge519ParityPrograms pins, on both lanes (RunInterp is the
// reference), the program-level shapes behind four analysis arms:
//
//   - a `do` body over an EXTENDED stack word — `drop` with a second,
//     user-owned overload — is no longer proven raise-free by
//     shuffleOnlyBody (basic), yet the run nets nothing, as the stock
//     word's does;
//   - a COMPUTED typed-list `if` arm runs through __arm, whose handler
//     (ArmSpliceHandler) hands a typed list back as the arm's one value,
//     where a computed plain list runs as a code body;
//   - `clone` of a type VALUE types as a Type (cloneReturnsFn, NUR323), so
//     arithmetic over it raises on both lanes;
//   - a concrete list mixing a value and a type literal joins its element
//     as Integer tor Type (check's elementJoinCarrier).
func TestMerge519ParityPrograms(t *testing.T) {
	const ext = `def P (refine Integer) def drop fn [[x:P] [] []] end `
	const mkTyped = `def mk fn [[][List][[:Integer 1 2]]] end `
	for _, c := range []struct{ src, want string }{
		{ext + `1 do [1 drop] drop`, "[]"},
		{`1 do [1 drop] drop`, "[]"},
		{mkTyped + `if true (mk) ["f"]`, "[[:Integer 1 2]]"},
		{mkTyped + `def c true end if c (mk) ["f"]`, "[[:Integer 1 2]]"},
		{`def f fn [[c:Boolean l:List][Any][if c l ["f"]]] end f true [:Integer 1 2]`, "[[:Integer 1 2]]"},
		{`def mk fn [[][List][quote [1 2]]] end if true (mk) ["f"]`, "[1 2]"},
		{`import "boru:struct-util" StructUtil.clone Integer`, "[Integer]"},
		{`import "boru:struct-util" (StructUtil.clone Integer) add 1`, "ERROR:cannot call `add`"},
		{`import "boru:struct-util" (StructUtil.clone 5) add 1`, "[6]"},
		{`[1 Integer] each [dup]`, "[[1 Integer]]"},
		{`[1 2.5] each [dup]`, "[[1 2.5]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
