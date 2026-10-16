package a

import core "github.com/boru-lang/boru/core/go"

type other struct{ n int }

func reported(dst, src []core.Datum, run core.Run) {
	copy(dst, src)                          // want "bulk copy of \\[\\]core.Datum: copy"
	_ = append(dst, src...)                 // want "bulk copy of \\[\\]core.Datum: variadic append"
	copy(run, src)                          // want "bulk copy of \\[\\]core.Datum: copy"
	_ = append(([]core.Datum)(nil), dst...) // want "bulk copy of \\[\\]core.Datum: variadic append"
}

// A []core.Value is an array of handles: copying one copies no datum.
func notReported(dst []core.Datum, vs []core.Value, ts []core.Type, tape core.Tape, os []other, bs []byte, s string) {
	_ = append(dst, core.Datum{}, core.Datum{})
	copy(vs, vs)
	_ = append(vs, vs...)
	_ = append(ts, ts...)
	copy(tape, vs)
	copy(os, os)
	_ = append(os, os...)
	copy(bs, s)
	_ = append(bs, s...)
	_ = len(dst)
	f := func(xs ...core.Datum) int { return len(xs) }
	_ = f(dst...)
}
