package lang

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// A positioned error raised inside an IMPORTED module names the module's
// file on both lanes (review of #462). The interpreter's stampErrPos
// attaches the registry's BaseFile to every positioned error; the VM's
// stampAt now does the same from the registry the UNIT runs on — the
// module's, not the program's — whether the position was stamped there or
// carried in by the diagnostic's builder (a routed dispatch's diagnostic,
// core/go/region_diag.go, is built at its position). Before: `--> 2:57`
// compiled against the interpreter's `--> /lib.boru:2:57`, for every VM
// error inside a module. The raise is conditioned on a value the check pass
// cannot fold, so the error is the run's, not a check-time mirror.
func TestCompiledModuleErrorNamesItsFile(t *testing.T) {
	lib := "import \"boru:time-util\"\ndef f fn [[][Any][def t (TimeUtil.now) end if (t eq t) [raise \"boom\"] [0]]]\nexport \"M\" { f: f/v }"
	src := "import \"/lib.boru\"\nM.f"
	run := func(compiled bool) *core.BoruError {
		t.Helper()
		mem := capabilities.NewMem()
		mem.Files["/lib.boru"] = []byte(lib)
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		b.SetFileOps(mem)
		var rerr error
		if compiled {
			var ran bool
			_, ran, rerr = b.RunCompiled(src)
			if !ran {
				t.Fatalf("the program compiles: %v", rerr)
			}
		} else {
			_, rerr = b.RunInterp(src)
		}
		var ae *core.BoruError
		if rerr == nil || !errors.As(rerr, &ae) || ae.Code != "user_error" {
			t.Fatalf("compiled=%v: want the module fn's user_error, got %v", compiled, rerr)
		}
		return ae
	}
	c, i := run(true), run(false)
	if c.File == "" || c.File != i.File || c.Row != i.Row || c.Col != i.Col {
		t.Errorf("the compiled error names the module's file at the interpreter's position:\n  compiled    %s:%d:%d\n  interpreted %s:%d:%d", c.File, c.Row, c.Col, i.File, i.Row, i.Col)
	}
}
