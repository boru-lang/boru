package lang

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// A function value defined in one module and invoked from another must resolve
// its free words in the module that DEFINED it, not the one that RUNS it
// (design/FUNCTION-VALUE-SCOPE.0.md).
//
// The native-callback seams used to run the body on the invoking registry, so a
// caller that happened to bind the same name silently supplied ITS definition:
// the call returned a plausible wrong answer, with `boru check` reporting
// nothing. Where the caller bound no such name at all, the body failed with
// `undefined_word` instead — the same defect, louder.
//
// Every case below is a REGRESSION: each one returned the caller's answer (or
// failed outright) before the seam fix, and each is checked on both engines,
// because a fix that repaired only one would trade a wrong answer for a
// compile != interpret divergence.

// fnScopeCase is one cross-module callback: a module that keeps a private
// helper and exports a callback using it, and a caller that binds the SAME
// helper name to a different definition and invokes the callback through a
// native word.
type fnScopeCase struct {
	name string
	// word names the native seam under test, for the failure message.
	word string
	mod  string
	// caller is the calling module's source; %s is the module's import path.
	caller string
	// want is the whole result stack rendered with fmt.Sprint.
	want string
	// wrong is what the pre-fix engine produced, quoted in the failure message
	// so a regression reads as itself rather than as a bare mismatch.
	wrong string
}

func fnScopeCases() []fnScopeCase {
	return []fnScopeCase{
		{
			// filter's Function form — native/filter.go's runFilterCallback.
			// The compiled path ALSO had to change: the compiler lowers a
			// higher-order word's fn operand to a closure unit compiled against
			// the CALLING module, so the seam fix alone would have left the two
			// engines disagreeing (foreignFnHome now declines that lowering).
			name: "filter Function form",
			word: "filter",
			mod: "def limit fn [[n:Integer] [Integer] [ 2 ]]\n" +
				"def big fn [[e:Map] [Boolean] [ (e dot value) gt (limit 0) ]]\n" +
				"export \"P\" { big: big/v }\n",
			caller: "import \"%s\" end\n" +
				"def limit fn [[n:Integer] [Integer] [ 100 ]]\n" +
				"filter P.big/v [1 2 3 4]\n",
			want:  "[[3 4]]",
			wrong: "[[]] (the caller's limit=100 kept nothing)",
		},
		{
			// each over a MAP with a Function body — native_map_iter.go's
			// mapBody.callLambda.
			name: "each over a map",
			word: "each",
			mod: "def tag fn [[n:Integer] [String] [ \"P\" ]]\n" +
				"def show fn [[kv:Map] [String] [ (tag 0) add (convert String (kv dot v)) ]]\n" +
				"export \"Q\" { show: show/v }\n",
			caller: "import \"%s\" end\n" +
				"def tag fn [[n:Integer] [String] [ \"CALLER\" ]]\n" +
				"each Q.show/v {a:1 b:2}\n",
			want:  "[{a:'P1' b:'P2'}]",
			wrong: "CALLER1/CALLER2",
		},
		{
			// StructUtil.walk's before callback — native/walk.go.
			name: "StructUtil.walk callback",
			word: "StructUtil.walk",
			mod: "def mark fn [[n:Integer] [String] [ \"P\" ]]\n" +
				"def hook fn [[m:Map] [Any] [ mark 0 ]]\n" +
				"export \"W\" { hook: hook/v }\n",
			caller: "import \"boru:struct-util\"\n" +
				"import \"%s\" end\n" +
				"def mark fn [[n:Integer] [String] [ \"CALLER\" ]]\n" +
				"StructUtil.walk W.hook/v {a:1}\n",
			want:  "[P]",
			wrong: "[CALLER]",
		},
		{
			// core walk's descend hook — native/walk_core.go's callWalkHook.
			// The hook is visit-only, so it records into a module-level flex
			// the module also exports a reader for. Here the caller binds NO
			// `acc` at all, which is the loud half of the defect: pre-fix this
			// failed with `undefined word: acc` rather than returning a wrong
			// value.
			name: "core walk descend hook",
			word: "walk",
			mod: "def acc (flex [])\n" +
				"def mark fn [[n:Integer] [String] [ \"P\" ]]\n" +
				"def hook fn [[m:Map] [Any] [ acc append (mark 0) ]]\n" +
				"def seen fn [[n:Integer] [List] [ acc ]]\n" +
				"export \"C\" { hook: hook/v, seen: seen/v }\n",
			caller: "import \"%s\" end\n" +
				"def mark fn [[n:Integer] [String] [ \"CALLER\" ]]\n" +
				"def _ (walk {mode:\"depth\"} {a:1} C.hook/v)\n" +
				"C.seen 0\n",
			want:  "[['P' 'P']]",
			wrong: "an `undefined word: acc` error",
		},
		{
			// A service handler — native/native_service.go, the seam that
			// InvokeCallbackFn replaced. Module H's handler calls H-private
			// `bonus` (n+1); the caller binds its own `bonus` (n*100).
			name: "service handler",
			word: "call",
			mod: "def bonus fn [[n:Integer] [Integer] [ n add 1 ]]\n" +
				"def handler fn [[req:Map state:Any] [Integer] [ bonus 5 ]]\n" +
				"export \"H\" { handler: handler/v }\n",
			caller: "import \"%s\" end\n" +
				"def bonus fn [[n:Integer] [Integer] [ n mul 100 ]]\n" +
				"def svc (service {})\n" +
				"add {op:\"go\"} H.handler/v svc\n" +
				"call {op:\"go\"} svc\n",
			want:  "[6]",
			wrong: "[500]",
		},
	}
}

// A cross-module callback must not be REPORTED as a problem either: the
// defect was invisible to `boru check`, and the fix must not make it visible
// as a false positive. Every fixture above checks clean.
func TestFunctionValueScopeChecksClean(t *testing.T) {
	for _, tc := range fnScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mod := filepath.Join(dir, "mod.aql")
			if err := os.WriteFile(mod, []byte(tc.mod), 0o600); err != nil {
				t.Fatalf("write module: %v", err)
			}
			a, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			res, err := a.Check(fmt.Sprintf(tc.caller, filepath.ToSlash(mod))) // a `\` in a boru string is an escape
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			for _, d := range res.Diagnostics {
				if d.Severity == native.SeverityError {
					t.Errorf("check reported an error on a valid cross-module callback: [%s] %s",
						d.Code, d.Detail)
				}
			}
		})
	}
}
