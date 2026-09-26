package lang

import "testing"

// TestNUR252ForeignValueKeepsItsCount pins NUR252 and NUR242's shaped
// method apply count. A module export fn VALUE whose body leaves the wrong
// count is the interpreter's count error on every path, as its named dispatch
// enforces it (NUR191). Two compiled paths did not:
//
//   - The dynamic apply hosted the module's stamped VALUE unit (dynApplyForeign)
//     under the trim discipline, and that unit declares no contract of its
//     own, so `m.f 5` over a factory-returned map answered `[5 1]` — silent.
//     The results now answer to the applied value's declared contract, as the
//     Apply kernel's frame does (applyRetContract).
//   - The shaped method apply (OpCallDynMethod) read a count that missed its
//     claim as a host-registration violation and bailed; `do [m.f 5]` catches
//     the interpreter's count error now (namedFnCountError).
func TestNUR252ForeignValueKeepsItsCount(t *testing.T) {
	const (
		inc = `import module [def inc fn [[n:Integer] [Integer] [n 1]] export "M" {inc: inc/v}] end `
		ok  = `import module [def inc fn [[n:Integer] [Integer] [n add 1]] export "M" {inc: inc/v}] end `
		two = `import module [def two fn [[n:Integer] [Integer Integer] [n n]] export "M" {two: two/v}] end `
		mk  = `def mk fn [[] [Map] [{f: M.inc/v}]] end def m (mk) end `
	)
	for _, r := range []struct{ src, want string }{
		{inc + mk + `m.f 5`, "ERROR:inc: expected 1 return value(s), got 2"},
		{inc + `def m {f: M.inc/v} end m.f 5`, "ERROR:inc: expected 1 return value(s), got 2"},
		{inc + mk + `do [m.f 5]`, "[error(inc: expected 1 return value(s), got 2 — [5 1])]"},
		// The declared count holds: the value answers as before.
		{ok + mk + `m.f 5`, "[6]"},
		{ok + mk + `do [m.f 5]`, "[6]"},
		{two + `def mk fn [[] [Map] [{f: M.two/v}]] end def m (mk) end m.f 5`, "[5 5]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
