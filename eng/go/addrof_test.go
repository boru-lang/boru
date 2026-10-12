package eng

import core "github.com/boru-lang/boru/core/go"

// addrOf returns a pointer to a copy of v, so a test can call a
// pointer-receiver method (core.Value's accessors take *Value since the
// no-copy plan's phase 1) on a value that is not addressable: a call's
// result, a map read, a tape cell.
func addrOf[T any](v T) *T { return &v }

// withID returns v with the identity id: a test builds the value it wants
// and names it in one expression (the ID lives in core's occurrence record
// since the no-copy plan's phase 3, so no literal key reaches it).
func withID(v core.Value, id string) core.Value {
	core.SetID(&v, id)
	return v
}
