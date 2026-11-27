package core

// BarrierBars reports whether an overload among sigs stops its forward
// collection short of nFwd written operands and then reads the stack: its
// barrier (the unmodified word's forward limit) is below nFwd and below its
// own arity. The interpreter's plan for such an overload takes the leading
// operands up to its barrier and the rest off the stack beneath the word —
// never the window a dispatch whose leading nFwd operands were written
// after the word holds — so a flat match of that window against it is not
// the interpreter's match (NUR362: `get (h) {k:1}`, every get overload's
// barrier 1, raises signature_error however (h) comes out).
func BarrierBars(sigs []Signature, nFwd int) bool {
	for i := range sigs {
		if s := &sigs[i]; !s.Fallback && s.BarrierPos >= 0 && s.BarrierPos < nFwd && s.BarrierPos < s.TotalArgs() {
			return true
		}
	}
	return false
}
