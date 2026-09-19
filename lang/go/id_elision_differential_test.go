package lang

import "testing"

// The runtime→compile differential for the mode-gated ID elision: values
// minted during plain runtime execution carry no compile identity, so a
// LATER compile pass that meets them must degrade conservatively
// (refusal / dynamic-scope read → interpreter fallback) and never
// miscompile. The sharpest shape is a closure whose capture snapshot is a
// runtime mint — capture slots are positional, so an identity-less
// capture must refuse the unit rather than collapse slots.
func TestRuntimeMintedCaptureCompileDifferential(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// Runtime phase: the factory runs OUTSIDE any pass, so the captured
	// x/y snapshots are identity-less runtime mints.
	if _, err := a.RunInterp(`def mk fn [[x:Integer y:Integer] [Function] [fn [[n:Integer] [Integer] [n add (x add y)]]]]
def addboth (mk 3 4)`); err != nil {
		t.Fatalf("runtime factory: %v", err)
	}

	// Compile phase over a source that calls the runtime-built closure.
	prog, reason, _, err := a.CompileCheck(`addboth 10`)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog != nil {
		// If it compiled anyway (a future emitter may place it soundly),
		// the compiled result must agree with the interpreter — never
		// silently-collapsed capture slots.
		out, compiled, rerr := a.RunCompiled(`addboth 10`)
		if noteCompileDefect(t, `addboth 10`, out, rerr) {
			return
		}
		if rerr != nil {
			t.Fatalf("RunCompiled: %v", rerr)
		}
		if len(out) != 1 || out[0] != int64(17) {
			t.Fatalf("compiled=%v result %v, want [17]", compiled, out)
		}
	} else if reason == "" {
		t.Fatal("refused with no reason")
	}

	// The interpreter fallback path must produce the right answer.
	out, err := a.RunInterp(`addboth 10`)
	if err != nil {
		t.Fatalf("interpreted call: %v", err)
	}
	if len(out) != 1 || out[0] != int64(17) {
		t.Fatalf("interpreted result %v, want [17]", out)
	}
}
