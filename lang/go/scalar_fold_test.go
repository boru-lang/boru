package lang

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// checkResidual runs src through a fresh check pass and returns the residual
// stack values (raw, pre-render).
func checkResidual(t *testing.T, src string) []core.Value {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	r := a.NativeRegistry()
	r.Source = src
	fin := r.Check.Begin()
	defer fin()
	e := native.NewTop(r)
	out, err := e.Run(values)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return out
}

// CompileScalarFold — a comparison over compile-time-known scalars folds to
// its CONCRETE result in check mode (tryFoldScalarConst), instead of the
// declared-type carrier: the prerequisite for static branch commitment
// (`(n eq 0)` with const n must read as a known false, not "some Boolean").
func TestScalarConstFold(t *testing.T) {
	folded := []struct {
		src  string
		want bool
	}{
		{`5 eq 0`, false},
		{`0 eq 0`, true},
		{`def n 5  (n eq 0)`, false},
		{`5 gt 1`, true},
		{`"a" eq "a"`, true},
	}
	for _, c := range folded {
		out := checkResidual(t, c.src)
		if len(out) != 1 {
			t.Fatalf("%q: %d residuals", c.src, len(out))
		}
		b, ok := core.DataOf(&out[0]).(core.BoolPayload)
		if !ok {
			t.Errorf("%q: residual not a concrete boolean (Data=%T carrier=%v)", c.src, core.DataOf(&out[0]), core.IsCarrier(&out[0]))
			continue
		}
		if b.B != c.want {
			t.Errorf("%q folded to %v, want %v", c.src, b.B, c.want)
		}
	}

	// NEGATIVE: a dynamic / unknown operand must NOT fold — the result stays
	// the declared-type carrier (gradual optimism unchanged).
	out := checkResidual(t, `def f fn [[x:Integer] [Boolean] [x eq 0]]  f 5`)
	if len(out) != 1 {
		t.Fatalf("fn-body eq: %d residuals", len(out))
	}
	if _, concrete := core.DataOf(&out[0]).(core.BoolPayload); concrete {
		// x inside the body is a param carrier — folding it would claim
		// knowledge the checker does not have.
		if !core.IsCarrier(&out[0]) {
			t.Errorf("fn-body eq over a param folded concretely; must stay a carrier")
		}
	}

	// NEGATIVE: a family-restricted ordering over cross-family operands keeps
	// its diagnostic path (the fold declines on handler error).
	a, _ := New()
	res, err := a.Check(`1 lt "a"`)
	if err == nil {
		gated := false
		for _, d := range res.Diagnostics {
			if d.Severity == "error" {
				gated = true
			}
		}
		if !gated {
			t.Errorf("1 lt \"a\": expected an error diagnostic, got %+v", res.Diagnostics)
		}
	}
}
