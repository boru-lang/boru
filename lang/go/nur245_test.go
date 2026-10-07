package lang

import "testing"

// TestNUR245BothArmsDefineOneFn pins NUR245's close for arms that agree on
// the fn's shape: a fn family both arms of a branch define is joined to a
// MODEL the pass types the call against — the running arm's fn when the
// condition is decided, the then arm's otherwise — where it used to be a
// payload-less carrier no call past the merge could dispatch
// ("unconsumed fn-value carrier in residual"). The family's dispatches
// route live, so the running arm's binding picks the body: a decided
// condition's running arm is an ordinary install, noted as a taken arm's
// (its bind twin keeps the live registry current), and an undecided one's
// other arm has its unit compiled where it is placed, under the family's
// name.
func TestNUR245BothArmsDefineOneFn(t *testing.T) {
	// Since phase 2 (design/IMMUTABLE-DEF.1.md §2.1) each arm is a BLOCK: a
	// fn both arms define is each arm's own and neither survives the
	// branch, so every call past the arms is undefined_word on the
	// interpreter — decided or not, whatever the arms' shapes agree or
	// disagree on — and the compiled lane agrees or declines at the check.
	// The joined model NUR245 closed is mooted; phase 4 retires it. The
	// intended spelling binds the branch VALUE (the last rows).
	const (
		f7   = `def f fn [[a:Integer] [Any] [7]]`
		f8   = `def f fn [[a:Integer] [Any] [8]]`
		fS8  = `def f fn [[a:String] [Any] [8]]`
		mOff = `def m {e: false} `
		mOn  = `def m {e: true} `
		und  = `if (m 'e' get) [`
	)
	const unbound = "ERROR:undefined word: f"
	for _, r := range []struct{ src, want string }{
		{`if false [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{`if true [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{`def c false if c [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{`if (1 gt 2) [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{`if [false] [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{mOff + `if (m 'e' get) [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{mOn + `if (m 'e' get) [` + f7 + `] [` + f8 + `] end 3 f`, unbound},
		{mOff + `if (m 'e' get) [` + f7 + `] [` + f8 + `] end [(f 3) (f 4)]`, unbound},
		{`def g fn [[b:Boolean] [Any] [if b [` + f7 + `] [` + f8 + `] 3 f]] end [(g true) (g false)]`, unbound},
		{mOff + `if (m 'e' get) [def f fn [[a:Integer] [Integer] [a add 7]]] [def f fn [[a:Integer] [Integer] [a add 8]]] end (f 3) mul 2`, unbound},
		{mOff + und + f7 + `] [` + fS8 + `] end 3 f`, unbound},
		{mOn + und + f7 + `] [` + fS8 + `] end "x" f`, unbound},
		{`def g fn [[b:Boolean] [Any] [if b [` + f7 + `] [` + fS8 + `] "x" f]] end [(g false)]`, unbound},
		{mOff + und + `def f fn [[a:Integer] [Integer] [a add 1]]] [def f fn [[a:Integer] [String] ['s']]] end (3 f) typeof`, unbound},
		{mOff + und + f7 + `] [def f fn [[a:String b:String] [Any] [8]]] end 3 f`, unbound},
		// The intended spelling: the branch value is the fn called.
		{`def f (if false [fn [[a:Integer] [Any] [7]]] [fn [[a:Integer] [Any] [8]]]) end 3 f`, "[8]"},
		{`def f (if true [fn [[a:Integer] [Any] [7]]] [fn [[a:Integer] [Any] [8]]]) end 3 f`, "[7]"},
		{mOff + `def f (if (m 'e' get) [fn [[a:Integer] [Integer] [a add 7]]] [fn [[a:Integer] [Integer] [a add 8]]]) end (f 3) mul 2`, "[22]"},
		{`def g fn [[b:Boolean] [Any] [def f (if b [fn [[a:Integer] [Any] [7]]] [fn [[a:Integer] [Any] [8]]]) 3 f]] end [(g true) (g false)]`, "[[7 8]]"},
	} {
		ruleOrDecline(t, r.src, r.want)
	}
}
