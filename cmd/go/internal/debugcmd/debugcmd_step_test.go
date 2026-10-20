package debugcmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/debugger"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/debugserve"
	"github.com/boru-lang/boru/parser/go"
)

// stepTestServer mounts a real program under a Remote-fronted session on
// an httptest server — the attach verbs' in-process twin of
// `boru debug serve --step`. The routes go through the debugserve
// server's real Auth gate (empty token: open).
func stepTestServer(t *testing.T, src string) (*debugserve.Client, *debugger.Remote, func()) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	var progOut bytes.Buffer
	reg.Output = &progOut
	sess := debugger.New(reg, debugger.Config{
		In: strings.NewReader(""), Out: &bytes.Buffer{},
		File: "remote.boru", Source: src,
	})
	rem := debugger.NewRemote(sess)
	toks, perr := parser.Parse(src)
	if perr != nil {
		t.Fatal(perr)
	}
	go func() {
		_, rerr := sess.RunProgram(toks, src)
		rem.Finish(rerr)
	}()
	srv := debugserve.NewServer(reg, "")
	mux := http.NewServeMux()
	rem.Register(mux, srv.Auth)
	mux.Handle("/", srv.Handler())
	ts := httptest.NewServer(mux)
	c := debugserve.NewClient(ts.URL, "")
	// Wait for the entry stop so verb tests start deterministic.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if st, perr := c.StepPause(); perr == nil && st.State == "paused" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the program never reached its entry stop")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return c, rem, func() { rem.Shutdown(); ts.Close() }
}

func TestAttachStepVerbs(t *testing.T) {
	c, _, done := stepTestServer(t, "def a 1\ndef b 2\na add b\n")
	defer done()

	run := func(verb ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := attachVerb(c, verb, &out, &errOut)
		return code, out.String(), errOut.String()
	}

	code, out, _ := run("pause")
	if code != 0 || !strings.Contains(out, "paused at remote.boru:1  def a 1") {
		t.Fatalf("pause: %d %q", code, out)
	}
	code, out, _ = run("break", "3")
	if code != 0 || !strings.Contains(out, "breakpoint set: line 3") {
		t.Fatalf("break: %d %q", code, out)
	}
	code, out, _ = run("step")
	if code != 0 || !strings.Contains(out, "paused at remote.boru:2  def b 2") {
		t.Fatalf("step: %d %q", code, out)
	}
	code, out, _ = run("continue")
	if code != 0 || !strings.Contains(out, "breakpoint: line 3 — at remote.boru:3  a add b") {
		t.Fatalf("continue: %d %q", code, out)
	}
	code, out, _ = run("delete", "3")
	if code != 0 || !strings.Contains(out, "deleted: line 3") {
		t.Fatalf("delete: %d %q", code, out)
	}
	code, out, _ = run("continue")
	if code != 0 || !strings.Contains(out, "exited") {
		t.Fatalf("final continue: %d %q", code, out)
	}
}

func TestAttachQuitVerb(t *testing.T) {
	c, rem, done := stepTestServer(t, "1 add 2\n")
	defer done()
	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"quit"}, &out, &errOut); code != 0 {
		t.Fatalf("quit: %d %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "(detached — the program continues to completion)") {
		t.Errorf("out = %q", out.String())
	}
	// The detached program drains on its own.
	deadline := time.Now().Add(5 * time.Second)
	for rem.ExitError() == "" {
		if st, err := c.StepPause(); err == nil && st.State == "exited" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the detached program never exited")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAttachStepVerbErrors(t *testing.T) {
	// Transport failures fail each stepping verb; a break without a
	// spec is a usage error before any request.
	c := debugserve.NewClient("http://127.0.0.1:1", "")
	for _, verb := range [][]string{{"pause"}, {"step"}, {"quit"}, {"break", "3"}} {
		var out, errOut bytes.Buffer
		if code := attachVerb(c, verb, &out, &errOut); code != 1 {
			t.Errorf("%v: exit = %d, want 1", verb, code)
		}
		if !strings.Contains(errOut.String(), "debug attach:") {
			t.Errorf("%v: stderr = %q", verb, errOut.String())
		}
	}
	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"break"}, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "usage: boru debug attach break") {
		t.Errorf("bare break: %d %q", code, errOut.String())
	}
}

func TestAttachStepPollTimeout(t *testing.T) {
	// An action that outlives the poll deadline reports "running" — for
	// a long computation the user checks again with `attach pause`. Two
	// arms: the server keeps answering "running", and the pause poll
	// itself failing at the deadline.
	prevD, prevI := stepPollDeadline, stepPollInterval
	stepPollDeadline, stepPollInterval = 30*time.Millisecond, 5*time.Millisecond
	defer func() { stepPollDeadline, stepPollInterval = prevD, prevI }()

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/action", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"running","seq":1}`))
	})
	pauseFails := false
	mux.HandleFunc("/debug/pause", func(w http.ResponseWriter, _ *http.Request) {
		if pauseFails {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"running","seq":1}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := debugserve.NewClient(ts.URL, "")

	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"next"}, &out, &errOut); code != 0 ||
		!strings.Contains(out.String(), "running") {
		t.Errorf("running timeout: %d %q", code, out.String())
	}
	pauseFails = true
	out.Reset()
	if code := attachVerb(c, []string{"out"}, &out, &errOut); code != 0 ||
		!strings.Contains(out.String(), "running") {
		t.Errorf("failing-poll timeout: %d %q", code, out.String())
	}
}

func TestRenderStepStateArms(t *testing.T) {
	if got := renderStepState(debugserve.StepState{State: "paused", Row: 0, InBody: true,
		File: "f.boru", Line: "", Stack: "[]"}); !strings.Contains(got, "at f.boru:? (in body)") {
		t.Errorf("in-body unknown-row = %q", got)
	}
	if got := renderStepState(debugserve.StepState{State: "exited", Exit: "boom"}); got != "exited: boom" {
		t.Errorf("exited-error = %q", got)
	}
	if got := renderStepState(debugserve.StepState{State: "running"}); got != "running" {
		t.Errorf("running = %q", got)
	}
}

// --- serve --step wiring ---

func TestServeStepShutdownDrains(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}
	// `serve --step` parks the program at its entry stop; SIGINT stops
	// the server, and the shutdown path detaches + DRAINS the program —
	// its side effects land before runServe returns 0.
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", dir)
	prog := filepath.Join(dir, "p.boru")
	if err := os.WriteFile(prog, []byte("print \"zz-serve-step\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runServe([]string{"--bind", "127.0.0.1:0", "--step", prog}, &out, &errOut)
	}()
	dp := filepath.Join(dir, "boru-debug.json")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(dp); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("discovery never appeared; stderr=%s", errOut.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	self, _ := os.FindProcess(os.Getpid()) // not syscall.Kill: the file must build on Windows
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit = %d; stderr=%s", code, errOut.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runServe --step did not exit after SIGINT")
	}
	if !strings.Contains(out.String(), "zz-serve-step") {
		t.Errorf("the drained program's output must land; out = %q", out.String())
	}
}

func TestServeStepProgramErrorExitsOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}
	// The drained program erroring is reported and turns the exit code.
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", dir)
	prog := filepath.Join(dir, "p.boru")
	if err := os.WriteFile(prog, []byte("1 div 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runServe([]string{"--bind", "127.0.0.1:0", "--step", prog}, &out, &errOut)
	}()
	dp := filepath.Join(dir, "boru-debug.json")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(dp); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("discovery never appeared; stderr=%s", errOut.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	self, _ := os.FindProcess(os.Getpid()) // not syscall.Kill: the file must build on Windows
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 1 || !strings.Contains(errOut.String(), "debug serve: program:") {
			t.Errorf("exit = %d, stderr = %q", code, errOut.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runServe --step did not exit after SIGINT")
	}
}

func TestServeStepArgErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runServe([]string{"--step"}, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "usage: boru debug serve --step") {
		t.Errorf("no file: %d %q", code, errOut.String())
	}
	errOut.Reset()
	if code := runServe([]string{"--step", filepath.Join(t.TempDir(), "zz.boru")}, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "read") {
		t.Errorf("missing file: %d %q", code, errOut.String())
	}
	errOut.Reset()
	bad := filepath.Join(t.TempDir(), "bad.boru")
	if err := os.WriteFile(bad, []byte("(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runServe([]string{"--step", bad}, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "parse") {
		t.Errorf("parse error: %d %q", code, errOut.String())
	}
}
