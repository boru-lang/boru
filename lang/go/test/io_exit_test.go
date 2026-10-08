package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// io_exit_test.go — the LANGUAGE half of IO.exit: what the runtime hands a
// driver, and what happens at the boundaries that must not honour it. The
// per-driver decisions (process status for `boru run` / `boru do` / a built
// binary, and `boru test`'s per-file handling) are in cmd/go.

func runExit(t *testing.T, src string) error {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := a.Run(`import "boru:io"  ` + src)
	return runErr
}

// The runtime does not exit anything: it hands the driver an error carrying
// the requested status, and lang.ExitCode is how a host reads it.
func TestIOExitSurfacesCode(t *testing.T) {
	for _, want := range []int{0, 1, 2, 42, 125} {
		err := runExit(t, "IO.exit "+itoa(want))
		if err == nil {
			t.Fatalf("IO.exit %d returned no error", want)
		}
		got, isExit := lang.ExitCode(err)
		if !isExit {
			t.Fatalf("IO.exit %d: not recognised as an exit: %v", want, err)
		}
		if got != want {
			t.Errorf("IO.exit %d: ExitCode = %d", want, got)
		}
	}
}

// An ordinary failure is NOT an exit request. This is the whole reason
// ExitCode returns a bool rather than a code and a nil check.
func TestExitCodeRejectsOrdinaryErrors(t *testing.T) {
	err := runExit(t, `IO.read "not-a-pathon"`)
	if err == nil {
		t.Fatal("expected a signature error")
	}
	if _, isExit := lang.ExitCode(err); isExit {
		t.Errorf("an ordinary error was reported as an exit request: %v", err)
	}
	if _, isExit := lang.ExitCode(nil); isExit {
		t.Error("nil was reported as an exit request")
	}
}

// The range is the shell's: 126/127 and 128+n mean specific things a
// program must not claim.
func TestIOExitRangeRefused(t *testing.T) {
	for _, code := range []string{"-1", "126", "127", "128", "200"} {
		err := runExit(t, "IO.exit "+code)
		if err == nil {
			t.Fatalf("IO.exit %s was accepted", code)
		}
		if _, isExit := lang.ExitCode(err); isExit {
			t.Errorf("IO.exit %s produced an exit request rather than refusing", code)
		}
		if !strings.Contains(err.Error(), "0..125") {
			t.Errorf("IO.exit %s: error does not state the range: %v", code, err)
		}
	}
}

// A handler-less `do` converts a body error into an Error VALUE — its
// escape-hatch contract — but an exit request crosses it unchanged.
// Demoting it to data would silently turn `IO.exit 4` into exit 0.
func TestIOExitCrossesDo(t *testing.T) {
	err := runExit(t, `do [IO.exit 4]`)
	code, isExit := lang.ExitCode(err)
	if !isExit || code != 4 {
		t.Fatalf("do [IO.exit 4]: ExitCode = (%d, %v), want (4, true)", code, isExit)
	}
	// The same `do` still catches an ordinary raise as data.
	a, _ := lang.New()
	res, rerr := runReference(t, a, `import "boru:io"  do [raise {code:'boom' message:'bang'}]`)
	if rerr != nil {
		t.Fatalf("ordinary raise should still be caught: %v", rerr)
	}
	if len(res) != 1 {
		t.Fatalf("expected the caught error as one value, got %v", res)
	}
}

// A sub-engine must NOT be able to exit its host: `Vm.run-sandbox` exists
// to contain what the code it runs can do, and honouring an exit request
// from inside it would terminate the host process. The boundary converts
// it to an ordinary error, keeping the code visible but powerless.
func TestIOExitCannotEscapeSandbox(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := a.Run(`import "boru:vm"  Vm.run-sandbox "import \"boru:io\" IO.exit 5"`)
	if runErr == nil {
		t.Fatal("a sub-program's exit request vanished entirely")
	}
	if _, isExit := lang.ExitCode(runErr); isExit {
		t.Fatal("SANDBOX ESCAPE: a sub-program's exit request reached the host driver")
	}
	if !strings.Contains(runErr.Error(), "code 5") {
		t.Errorf("the converted error should still name the requested code: %v", runErr)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
