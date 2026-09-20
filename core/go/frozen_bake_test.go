package core

import "testing"

// FrozenBake.String supplies the noun in the freeze discipline's compile failure, so
// each member is pinned to the word a reader of that compile failure will see. The
// INVALID ZERO is pinned too, and deliberately to a neutral noun: NoteFrozenRead
// drops an unclassified note rather than recording it, so this arm is reachable
// only if some future caller renders a bake it never set — and a compile failure reading
// "baked its binding" is a weaker claim than one naming an artifact that never
// froze.
func TestFrozenBakeString(t *testing.T) {
	for _, c := range []struct {
		bake FrozenBake
		want string
	}{
		{FrozenBakeValue, "value"},
		{FrozenBakeType, "type"},
		{FrozenBakeCall, "call target"},
		{FrozenBakeNone, "binding"},
		{FrozenBake(200), "binding"},
	} {
		if got := c.bake.String(); got != c.want {
			t.Errorf("FrozenBake(%d).String() = %q, want %q", uint8(c.bake), got, c.want)
		}
	}
}
