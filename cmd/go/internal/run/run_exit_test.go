package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run_exit_test.go — `boru run`'s decision about IO.exit: the requested code
// becomes THIS process's status, nothing is printed, and the residual stack
// is not flushed. The language-level contract is in
// lang/go/test/io_exit_test.go.

func writeExitScript(t *testing.T, body string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "exit.boru")
	if err := os.WriteFile(file, []byte("import \"boru:io\"\n"+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestExecuteExitCodeIsProcessStatus(t *testing.T) {
	for _, want := range []int{0, 1, 3, 125} {
		file := writeExitScript(t, "IO.exit "+itoa(want))
		var out, errb bytes.Buffer
		got := Execute([]string{file}, strings.NewReader(""), &out, &errb)
		if got != want {
			t.Errorf("IO.exit %d: exit status %d, stderr: %s", want, got, errb.String())
		}
		// Exiting is not failing: nothing is reported, for any code.
		if errb.Len() != 0 {
			t.Errorf("IO.exit %d printed to stderr: %q", want, errb.String())
		}
	}
}

// The residual stack is NOT flushed on exit — an exiting program's output
// is whatever it deliberately printed, not the accident of what was left
// on the tape.
func TestExecuteExitSuppressesResidual(t *testing.T) {
	file := writeExitScript(t, "42\nIO.exit 0")
	var out, errb bytes.Buffer
	if code := Execute([]string{file}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if strings.Contains(out.String(), "42") {
		t.Errorf("residual stack was printed on exit: %q", out.String())
	}
}

// Work the program did BEFORE exiting still happened, and work after the
// exit did not — the exit unwinds, it does not discard. Asserted through a
// filesystem side effect rather than stdout: `print` writes to the
// instance's output stream, not the writer handed to Execute.
func TestExecuteExitUnwindsWithoutDiscarding(t *testing.T) {
	dir := t.TempDir()
	before := filepath.Join(dir, "before.txt")
	after := filepath.Join(dir, "after.txt")
	file := filepath.Join(dir, "exit.boru")
	src := "import \"boru:io\"\n" +
		"IO.write (make Pathon \"" + filepath.ToSlash(before) + "\") \"1\" drop\n" + // `\` escapes in boru
		"IO.exit 2\n" +
		"IO.write (make Pathon \"" + filepath.ToSlash(after) + "\") \"1\" drop\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Execute([]string{file}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if _, err := os.Stat(before); err != nil {
		t.Errorf("work done before the exit was rolled back: %v", err)
	}
	if _, err := os.Stat(after); err == nil {
		t.Error("execution continued past the exit")
	}
}

// A code outside 0..125 is a refusal, not an exit: the program is reported
// as failing (status 1) with the range explained.
func TestExecuteExitRangeRefusedIsAFailure(t *testing.T) {
	file := writeExitScript(t, "IO.exit 200")
	var out, errb bytes.Buffer
	if code := Execute([]string{file}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("exit status %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "0..125") {
		t.Errorf("stderr does not explain the range: %q", errb.String())
	}
}

// An ordinary failure is still status 1 with its diagnostic — the exit
// recognition must not swallow error reporting.
func TestExecuteOrdinaryErrorUnaffected(t *testing.T) {
	file := writeExitScript(t, `IO.read "not-a-pathon"`)
	var out, errb bytes.Buffer
	if code := Execute([]string{file}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("exit status %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Error("an ordinary error printed nothing")
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
