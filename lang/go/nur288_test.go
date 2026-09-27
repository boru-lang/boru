package lang

import "testing"

// TestNUR288FnLiteralIdentityPerEvaluation pins NUR288's close. A fn literal
// written in a fn body is a new function every time the body runs: `def mk
// fn [[][Any][([x:Any] => [x])]] end mk mk eq` is false interpreted. The
// compiled body pushed the literal's pooled const, one identity across
// calls, and answered true. The const is now re-identified per push, as a
// compound body literal is re-constructed; a named fn — one binding read
// twice — stays one function, and a capturing closure was fresh already.
func TestNUR288FnLiteralIdentityPerEvaluation(t *testing.T) {
	const mk = `def mk fn [[][Any][([x:Any] => [x])]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `mk mk eq`, "[false]"},
		{mk + `def a (mk) end def b (mk) end a/v b/v eq`, "[false]"},
		{mk + `def a (mk) end a/v a/v eq`, "[true]"},
		{`def mk fn [[][Any][fn [[x:Any][Any][x]]]] end mk mk eq`, "[false]"},
		{`([x:Any] => [x]) ([x:Any] => [x]) eq`, "[false]"},
		{`def mk fn [[n:Any][Any][([x:Any] => [n])]] end (mk 1) (mk 1) eq`, "[false]"},
		{`def g ([x:Any] => [x]) end def mk fn [[][Any][g/v]] end mk mk eq`, "[true]"},
		{`def mk fn [[][Any][{a: 1}]] end mk mk eq`, "[false]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
