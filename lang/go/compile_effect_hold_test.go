package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// A call through the specialised param over an argument inc may refuse: the
// specialisation declines and CompileCheck retries without it.
const specRetrySrc = specInc + `def h fn [[g:Function x:Any][Any][g x]] end  h inc/v "s"`

// zzCheckEffect registers `zz-effect`, a RunInCheckMode word that makes an
// effect the hold cannot keep (a counted one, as a file write is) and prints.
func zzCheckEffect(a *Boru) {
	a.Register("zz-effect", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, reg *native.Registry) ([]native.Value, error) {
			reg.NoteEffect()
			fmt.Fprint(reg.Output, "F")
			return nil, nil
		}, native.RunInCheck()),
	})
}

// The retry does not repeat the first pass's output: it is held, and dropped
// for the retry's own. A kept pass writes its held output once too.
func TestCompileRetryEmitsCheckPassOutputOnce(t *testing.T) {
	for name, src := range map[string]string{
		"retried": specRetrySrc,
		"kept":    specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  h inc/v`,
	} {
		a := mustNew(t)
		zzCheckEmit(a)
		var out bytes.Buffer
		a.SetOutput(&out)
		prog, reason, _, err := a.CompileCheck(`zz-emit ; ` + src)
		if prog == nil || err != nil {
			t.Fatalf("%s: compile: reason=%q err=%v", name, reason, err)
		}
		if out.String() != "E" {
			t.Errorf("%s: check-pass output = %q, want exactly one %q", name, out.String(), "E")
		}
		if a.NativeRegistry().Output != &out {
			t.Errorf("%s: the writer must be restored after the compile", name)
		}
	}
}

// A first pass that made an effect the hold cannot keep is not retried: the
// program it declined does not compile, and the effect happened once.
func TestCompileRetryRefusedAfterUnrepeatableEffect(t *testing.T) {
	a := mustNew(t)
	zzCheckEffect(a)
	var out bytes.Buffer
	a.SetOutput(&out)
	before := a.NativeRegistry().Effects.Count()
	prog, reason, _, err := a.CompileCheck(`zz-effect ; ` + specRetrySrc)
	if prog != nil || err != nil || !strings.Contains(reason, "unrepeatable") {
		t.Fatalf("want the unrepeatable-effect compile failure, got prog=%v reason=%q err=%v", prog != nil, reason, err)
	}
	if n := a.NativeRegistry().Effects.Count() - before; n != 1 || out.String() != "F" {
		t.Errorf("the effect must happen once: %d counted, output %q", n, out.String())
	}
	// The same effect beside a specialisation that does not decline is
	// never retried, so it compiles.
	b := mustNew(t)
	zzCheckEffect(b)
	b.SetOutput(&bytes.Buffer{})
	if prog, reason, _, err := b.CompileCheck(`zz-effect ; ` + specInc + `def h fn [[g:Function][Integer][(g 2) add 10]] end  h inc/v`); prog == nil || err != nil {
		t.Errorf("a kept pass with an effect compiles: reason=%q err=%v", reason, err)
	}
}

// A stdin read is unrepeatable; so is a counted effect; a pass with neither
// is repeatable.
func TestCheckPassHoldUnrepeatable(t *testing.T) {
	a := mustNew(t)
	r := a.NativeRegistry()
	r.Input = strings.NewReader("alpha\n")
	h := holdCheckPass(r)
	if h.unrepeatable() {
		t.Error("nothing done yet: repeatable")
	}
	if _, err := a.RunInterp(`import "boru:io"  IO.read-line (IO.stdin)`); err != nil {
		t.Fatal(err)
	}
	if !h.unrepeatable() {
		t.Error("a stdin read must make the pass unrepeatable")
	}
	h.release(false)
	h = holdCheckPass(r)
	r.NoteEffect()
	if !h.unrepeatable() {
		t.Error("a counted effect must make the pass unrepeatable")
	}
	h.release(false)
	if got := native.StdinReads(&native.Registry{}); got != 0 {
		t.Errorf("a registry with no shared reader counts no reads, got %d", got)
	}
}

// Held output keeps its order across both writers; a kept hold writes it,
// a discarded one drops it; after release a held writer writes straight
// through (a module sub-registry keeps the writer it copied), and it
// unwraps to its destination. An absent writer stays absent.
func TestCheckPassHoldWriters(t *testing.T) {
	a := mustNew(t)
	r := a.NativeRegistry()
	var out, errOut bytes.Buffer
	r.Output, r.ErrOutput = &out, &errOut
	h := holdCheckPass(r)
	held := r.Output
	fmt.Fprint(r.Output, "a")
	fmt.Fprint(r.ErrOutput, "b")
	fmt.Fprint(r.Output, "c")
	if out.Len()+errOut.Len() != 0 {
		t.Fatalf("held output escaped: %q %q", out.String(), errOut.String())
	}
	if hw, ok := held.(heldWriter); !ok || hw.Unwrap() != &out {
		t.Errorf("a held writer unwraps to its destination")
	}
	h.release(true)
	if out.String() != "ac" || errOut.String() != "b" || r.Output != &out || r.ErrOutput != &errOut {
		t.Errorf("kept: out %q err %q", out.String(), errOut.String())
	}
	fmt.Fprint(held, "d")
	if out.String() != "acd" {
		t.Errorf("after release a held writer writes through, got %q", out.String())
	}

	out.Reset()
	h = holdCheckPass(r)
	fmt.Fprint(r.Output, "x")
	h.release(false)
	if out.Len() != 0 {
		t.Errorf("a discarded hold writes nothing, got %q", out.String())
	}

	r.Output, r.ErrOutput = nil, nil
	h = holdCheckPass(r)
	if r.Output != nil || r.ErrOutput != nil {
		t.Error("an absent writer stays absent under a hold")
	}
	h.release(true)
}
