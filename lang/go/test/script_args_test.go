package test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// runOne runs source and returns the single resulting value.
func runOneScriptArgs(t *testing.T, a *lang.Boru, src string) any {
	t.Helper()
	res, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("%s: returned %d values, want 1", src, len(res))
	}
	return res[0]
}

// IO.args surfaces the host-installed script arguments (Options.ScriptArgs
// → SetHostScriptArgs) as a List of Strings, in order.
func TestIOArgsSurfacesScriptArgs(t *testing.T) {
	a, err := lang.New(lang.Options{ScriptArgs: []string{"notes.json", "--fast"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := runOneScriptArgs(t, a, `import "boru:io"  size (IO.args)`); n != int64(2) {
		t.Fatalf("size (IO.args) = %#v, want 2", n)
	}
	if got := runOneScriptArgs(t, a, `import "boru:io"  (IO.args) get 0`); got != "notes.json" {
		t.Errorf("IO.args[0] = %#v, want notes.json", got)
	}
	if got := runOneScriptArgs(t, a, `import "boru:io"  (IO.args) get 1`); got != "--fast" {
		t.Errorf("IO.args[1] = %#v, want --fast", got)
	}
}

// With no host installation (the default), IO.args is an EMPTY list —
// never an error, never none — so scripts can iterate unconditionally.
func TestIOArgsDefaultsEmpty(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	got := runOneScriptArgs(t, a, `import "boru:io"  size (IO.args)`)
	if n, ok := got.(int64); !ok || n != 0 {
		t.Errorf("default size (IO.args) = %#v, want 0", got)
	}
}

// An empty-but-set vector is also an empty list (set ≠ unset must not
// change the observable surface).
func TestIOArgsEmptyVector(t *testing.T) {
	a, err := lang.New(lang.Options{ScriptArgs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	got := runOneScriptArgs(t, a, `import "boru:io"  size (IO.args)`)
	if n, ok := got.(int64); !ok || n != 0 {
		t.Errorf("empty-vector size (IO.args) = %#v, want 0", got)
	}
}

// SetHostScriptArgs(nil) clears a previously-installed vector.
func TestSetHostScriptArgsNilClears(t *testing.T) {
	a, err := lang.New(lang.Options{ScriptArgs: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	if got := native.HostScriptArgs(reg); len(got) != 1 {
		t.Fatalf("installed args = %v, want [x]", got)
	}
	native.SetHostScriptArgs(reg, nil)
	if got := native.HostScriptArgs(reg); got != nil {
		t.Fatalf("after nil clear, HostScriptArgs = %v, want nil", got)
	}
	got := runOneScriptArgs(t, a, `import "boru:io"  size (IO.args)`)
	if n, ok := got.(int64); !ok || n != 0 {
		t.Errorf("after clear, size (IO.args) = %#v, want 0", got)
	}
}

// The core fn-arguments word `args` is untouched by the io module's
// script-args export: inside an fn it still names the CALL's arguments.
func TestCoreArgsWordUnaffected(t *testing.T) {
	a, err := lang.New(lang.Options{ScriptArgs: []string{"cli-arg"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Run(`import "boru:io"
def f fn [[x:Integer] [Integer] [ (args) get 0 ]]
f 7`)
	if err != nil {
		t.Fatalf("core args inside fn: %v", err)
	}
	if got := res[len(res)-1]; got != int64(7) {
		t.Errorf("fn (args) get 0 = %#v, want 7", got)
	}
}
