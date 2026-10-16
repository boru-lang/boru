package modules

// addrOf returns a pointer to a copy of v, so a test can call a
// pointer-receiver method (core.Value's accessors take *Value since the
// no-copy plan's phase 1) on a value that is not addressable: a call's
// result, a map read, a tape cell.
func addrOf[T any](v T) *T { return &v }
