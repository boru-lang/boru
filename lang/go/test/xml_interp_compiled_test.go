package test

import (
	"fmt"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// An interpolated XML literal (`<p>${x}</p>`) is a RUNTIME builder (the
// Word/__XI skeleton), so the bytecode compiler must never const-fold a
// check-mode evaluation of it: under static analysis a `${expr}` hole resolves
// to a carrier, and a Node-valued hole's carrier renders as its type tag
// ("Xml") — baking that froze a wrong child (`<a>Xml</a>` instead of
// `<a><b/></a>`). evalXmlInterp now mirrors evalInterpString: an all-concrete
// interpolation compiles native; a hole that is non-concrete under analysis
// refuses, so the program falls back to the interpreter and builds the real
// tree. This test pins compiled == interpreted for every interpolation shape.
func TestXmlInterpCompiledParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_failed; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	cases := []struct {
		src  string
		want string // the (shared) result both engines must produce
	}{
		// Scalar / text / attribute holes — compile native.
		{`def n "x" <p>${n}</p>`, `[<p>x</p>]`},
		{`def n 42 <p>n=${n}</p>`, `[<p>n=42</p>]`},
		{`def u "/x" <a href=${u}/>`, `[<a href="/x"/>]`},
		{`def k "big" <a class="btn ${k}"/>`, `[<a class="btn big"/>]`},
		// Node-valued holes — the miscompile case: must build the element, not
		// stringify the carrier to "Xml". (Falls back faithfully.)
		{`def b <b/> <a>${b}</a>`, `[<a><b/></a>]`},
		{`def ks [<li>x</li> <li>y</li>] <ul>${ks}</ul>`, `[<ul><li>x</li><li>y</li></ul>]`},
		// A non-interpolated literal still const-bakes (native).
		{`<a x="1"><b/>hi</a>`, `[<a x="1"><b/>hi</a>]`},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			ai, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			interp, ierr := ai.RunInterp(c.src)
			if ierr != nil {
				t.Fatalf("interp: %v", ierr)
			}
			ac, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			comp, _, cerr := ac.RunCompiled(c.src)
			if cerr != nil {
				t.Fatalf("compiled: %v", cerr)
			}
			is, cs := fmt.Sprint(interp), fmt.Sprint(comp)
			if is != c.want {
				t.Errorf("interpreted = %s, want %s", is, c.want)
			}
			if cs != is {
				t.Errorf("compiled = %s diverges from interpreted = %s (miscompile)", cs, is)
			}
		})
	}
}
