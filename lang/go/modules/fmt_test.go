package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// fmtFormatHandler returns the raw Go handler behind the boru:fmt "format"
// native so a test can drive both arms directly — the same seam the rand
// module uses (rand_seam7_test.go).
func fmtFormatHandler(t *testing.T) native.Handler {
	t.Helper()
	for _, nf := range FmtNatives {
		if nf.Name == "format" {
			gi, ok := nf.Signatures[0].Impl.(*core.GoImpl)
			if !ok {
				t.Fatalf("format: Impl is %T, want *core.GoImpl", nf.Signatures[0].Impl)
			}
			return gi.Handler
		}
	}
	t.Fatal("boru:fmt native \"format\" not found")
	return nil
}

// TestFmtFormatHandler drives both arms of the format handler: a concrete
// string is normalised, and a non-concrete (type-literal) argument — which
// matches the [String] sig slot yet has no concrete payload — is declined by
// AsConcreteString rather than panicking.
func TestFmtFormatHandler(t *testing.T) {
	h := fmtFormatHandler(t)

	// Positive: messy source collapses to canonical layout.
	out, err := h([]native.Value{native.NewString("{ a:1   b:2 }")}, nil, nil, nil)
	if err != nil {
		t.Fatalf("format: unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("format: got %d results, want 1", len(out))
	}
	got, cerr := out[0].AsConcreteString()
	if cerr != nil {
		t.Fatalf("format: result not a concrete string: %v", cerr)
	}
	if want := "{a:1 b:2}\n"; got != want {
		t.Errorf("format: got %q, want %q", got, want)
	}

	// Negative: a non-concrete source (bare String type literal) errors.
	if _, err := h([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, nil); err == nil {
		t.Error("format: non-concrete source should error, got nil")
	}
}

// TestFmtModuleDispatch proves the runtime API end to end: `import "boru:fmt"`
// followed by `Fmt.format <src>` returns the formatted string through the
// engine, exercising BuildFmtModule and the delegating-wrapper dispatch.
func TestFmtModuleDispatch(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)

	values, perr := parser.Parse(`import "boru:fmt"` + "\n" + `Fmt.format "[ a  b  c ]"`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	stack, rerr := native.NewTop(reg).Run(values)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if len(stack) != 1 {
		t.Fatalf("run: stack depth %d, want 1", len(stack))
	}
	got, cerr := stack[0].AsConcreteString()
	if cerr != nil {
		t.Fatalf("result not a concrete string: %v", cerr)
	}
	if want := "[a b c]\n"; got != want {
		t.Errorf("Fmt.format: got %q, want %q", got, want)
	}
}
