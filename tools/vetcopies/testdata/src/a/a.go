package a

import core "github.com/boru-lang/boru/core/go"

type other struct{ n int }

func reported(dst, src []core.Value, ts []core.Type, tape core.Tape) {
	copy(dst, src)                          // want "bulk copy of \\[\\]core.Value: copy"
	_ = append(dst, src...)                 // want "bulk copy of \\[\\]core.Value: variadic append"
	_ = append(ts, ts...)                   // want "bulk copy of \\[\\]core.Value: variadic append"
	copy(tape, src)                         // want "bulk copy of \\[\\]core.Value: copy"
	_ = append(([]core.Value)(nil), dst...) // want "bulk copy of \\[\\]core.Value: variadic append"
}

func notReported(dst []core.Value, v core.Value, os []other, bs []byte, s string) {
	_ = append(dst, v, v)
	copy(os, os)
	_ = append(os, os...)
	copy(bs, s)
	_ = append(bs, s...)
	_ = len(dst)
	f := func(xs ...core.Value) int { return len(xs) }
	_ = f(dst...)
}
