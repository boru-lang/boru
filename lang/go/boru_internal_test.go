package lang

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

func TestRegisterFormat(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Create a simple format that wraps text in brackets.
	a.RegisterFormat("bracket", &bracketFormat{})

	// Verify the format was registered by checking the registry directly.
	if native.HostFormats(a.registry)["bracket"] == nil {
		t.Fatal("expected bracket format to be registered")
	}
}

// TestNewFormatParserFnLangSurface drives the lang-level NewFormatParserFn
// wrapper: the built value binds with DefineValue and parses import-free.
func TestNewFormatParserFnLangSurface(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewFormatParserFn("bracket", &bracketFormat{})
	if err != nil {
		t.Fatalf("NewFormatParserFn: %v", err)
	}
	if err := a.DefineValue("bracket", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	const parseSrc = "parse bracket 'hi'"
	gotC, cErr := a.Run(parseSrc)
	noteCompileDefect(t, parseSrc, gotC, cErr)
	got, err := a.RunInterp(parseSrc)
	if err != nil {
		t.Fatalf("parse bracket: %v", err)
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("parse bracket = %v, want [hi]", got)
	}
}

// TestNewEmitLangFnLangSurface drives the lang-level NewEmitLangFn wrapper:
// the built value binds with DefineValue and emits import-free.
func TestNewEmitLangFnLangSurface(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewEmitLangFn(EmitLangSpec{
		Name: "up",
		Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{native.NewString("UP")}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewEmitLangFn: %v", err)
	}
	if err := a.DefineValue("up", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	got, err := a.Run(`emit up {a:1}`)
	if err != nil {
		t.Fatalf("emit up: %v", err)
	}
	if len(got) != 1 || got[0] != "UP" {
		t.Fatalf("emit up = %v, want [UP]", got)
	}
}

// bracketFormat is a test format.
type bracketFormat struct{}

func (f *bracketFormat) Decode(content string) ([]native.Value, error) {
	return []native.Value{native.NewString(content)}, nil
}

func (f *bracketFormat) Encode(v native.Value) (string, error) {
	return "[" + v.String() + "]", nil
}
