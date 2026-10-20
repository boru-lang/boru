package modules

import (
	"os"
	"path/filepath"
	"testing"
)

// A `serve-raw` handler exported by a MODULE must resolve its free words in
// that module, not in the program that started the server
// (design/FUNCTION-VALUE-SCOPE.0.md).
//
// serve-raw is the one seam where the fix is not just a routing choice: each
// connection runs on its own goroutine, so the handler cannot simply be handed
// the module's registry — it gets a per-connection ForkConcurrent OF that
// registry, which clones Defs/Types and takes a private context layer. This
// test pins both halves at once: the greeting proves the handler read the
// MODULE's `greet`, and two sequential connections prove the per-connection
// forks are independent rather than sharing one module registry.
//
// Before the fix the handler ran on a fork of the CALLER's registry, so this
// returned the caller's "caller-hello" — silently, on both engines.
func TestServeRawHandlerResolvesDefiningModule(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "greeter.aql")
	if err := os.WriteFile(mod, []byte(
		"import \"boru:net\"\n"+
			"def greet fn [[n:Integer] [String] [ \"module-hello\\n\" ]]\n"+
			"def onconn fn [[conn:Any] [Any] [\n"+
			"  Net.send-bytes (convert Bytes (greet 0)) conn\n"+
			"]]\n"+
			"export \"G\" { onconn: onconn/v }\n"), 0o600); err != nil {
		t.Fatalf("write module: %v", err)
	}

	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`import "` + filepath.ToSlash(mod) + `" end`, // a `\` in a boru string is an escape
		// The caller binds the SAME name to a different definition. Nothing
		// type-checks the difference — both are [[Integer] [String]] — so a
		// wrong answer here is silent.
		`def greet fn [[n:Integer] [String] [ "caller-hello\n" ]]`,
		`def nl (convert Bytes "\n")`,
		`def ln (Net.serve-raw {tcp: 0} G.onconn)`,
		`def port ((Net.addr ln).port)`,
		`def dial {tcp: (join "" ["127.0.0.1:" (convert String port)])}`,
		`def s1 (Net.connect-raw dial)`,
		`def r1 (convert String (Net.recv-until s1 nl {within: 5000}))`,
		`Net.close s1;`,
		// A SECOND connection: each gets its own fork of the module registry.
		`def s2 (Net.connect-raw dial)`,
		`def r2 (convert String (Net.recv-until s2 nl {within: 5000}))`,
		`Net.close s2;`,
		`Net.close ln;`,
		`join "|" [r1 r2]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := lastString(t, out), "module-hello|module-hello"; got != want {
		t.Errorf("serve-raw handler greeting = %q, want %q "+
			"(%q means it ran on the CALLER's registry)",
			got, want, "caller-hello|caller-hello")
	}
}
