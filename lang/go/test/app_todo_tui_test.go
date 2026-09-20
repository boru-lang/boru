package test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
	parser "github.com/boru-lang/boru/parser/go"
)

// End-to-end test for the TUI verification app (design/examples/apps/
// todo-tui.boru — the TUI.0.md acceptance app, the terminal twin of
// TestAppTodoAPI): the app file loads as a FILE MODULE and runs a real
// Tui.run session against a scripted virtual terminal.

// runTuiAppSteps mirrors runAppSteps with a virtual terminal backend
// registered, so TUI example apps can drive a real Tui.run headlessly.
func runTuiAppSteps(t *testing.T, vb *tuikit.VirtualBackend, appFiles []string, steps []string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for _, name := range appFiles {
		src, err := os.ReadFile("../../../design/examples/apps/" + name)
		if err != nil {
			t.Fatalf("read app %s: %v", name, err)
		}
		mem.Files["/apps/"+name] = src
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	native.SetHostFileOps(reg, mem)
	modules.InstallResolver(reg)
	if err := modules.RegisterHostTui(reg, modules.TuiSpec{
		Name: "virtual",
		Open: func() (tuikit.Backend, error) { return vb, nil },
	}); err != nil {
		t.Fatal(err)
	}
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// The todo TUI (todo-tui.boru): type a todo, submit it, hop to the list,
// toggle it done, quit — the final state carries the session's work and
// the init frame carries the §5 layout chrome.
func TestAppTodoTUI(t *testing.T) {
	vb := tuikit.NewVirtualBackend(44, 12)
	for _, ev := range []tuikit.Event{
		{Tag: "key", Key: "m", Char: "m"},
		{Tag: "key", Key: "i", Char: "i"},
		{Tag: "key", Key: "l", Char: "l"},
		{Tag: "key", Key: "k", Char: "k"},
		{Tag: "key", Key: "enter", Char: ""},
		{Tag: "key", Key: "tab", Char: ""},
		{Tag: "key", Key: "space", Char: ""},
		{Tag: "key", Key: "q", Char: "q"},
	} {
		vb.Inject(ev)
	}
	out, err := runTuiAppSteps(t, vb, []string{"todo-tui.boru"}, []string{
		`import "/apps/todo-tui.boru"`,
		`def final (TodoTui.run {})`,
		`def first (final.todos get 0)`,
		`join "|" [(convert String (size final.todos)) (first.text)
		           (convert String first.done) (convert String final.next) (final.focus)]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := appLastString(t, out); got != "1|milk|true|2|list" {
		t.Fatalf("final state = %q, want 1|milk|true|2|list", got)
	}
	// the init frame rendered the full §5 chrome before any input folded
	if vb.FrameCount() == 0 {
		t.Fatal("no frames presented")
	}
	first := strings.Join(vb.ScreenAt(0), "\n")
	for _, want := range []string{"new todo", "what needs doing?", "todos 0/0", "enter add"} {
		if !strings.Contains(first, want) {
			t.Errorf("init frame is missing %q:\n%s", want, first)
		}
	}

	// negative: the serve entry demands its transport options (a missing
	// token reaches Tui.serve as None via dot-leniency and is declined)
	vb2 := tuikit.NewVirtualBackend(44, 12)
	_, sErr := runTuiAppSteps(t, vb2, []string{"todo-tui.boru"}, []string{
		`import "/apps/todo-tui.boru"`,
		`TodoTui.serve {tcp: 0}`,
	})
	if sErr == nil || !strings.Contains(sErr.Error(), "token: must be a String") {
		t.Fatalf("tokenless serve = %v", sErr)
	}
}

// pollScreen waits until the virtual screen shows want (the app is live
// on another goroutine; worker round trips land asynchronously).
func pollScreen(t *testing.T, vb *tuikit.VirtualBackend, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(vb.Screen(), "\n"), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q:\n%s", want, strings.Join(vb.Screen(), "\n"))
}

// The REST-backed client (todo-tui-client.boru) against the real
// todo-api server over loopback HTTP: the UI's list is a cache of the
// server's, every mutation is a spawned worker round trip, and the
// status line tracks sync state — TUI.0.md §2.3 end to end.
func TestAppTodoTUIClient(t *testing.T) {
	vb := tuikit.NewVirtualBackend(60, 12)
	type result struct {
		out []native.Value
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runTuiAppSteps(t, vb, []string{"todo-api.boru", "todo-tui-client.boru"}, []string{
			`import "/apps/todo-api.boru"`,
			`import "/apps/todo-tui-client.boru"`,
			`import "boru:net"`,
			`def ln (TodoApi.serve {port: 0})`,
			`def addr (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)])`,
			`def final (TodoTuiClient.run {tcp: addr})`,
			`def first (final.todos get 0)`,
			`join "|" [(convert String (size final.todos)) (first.text)
			           (convert String first.done) (convert String (final.err eq ""))]`,
		})
		done <- result{out, err}
	}()

	// the initial GET lands: the empty list is in sync
	pollScreen(t, vb, "in sync")
	// type a todo and submit — the POST worker round-trips
	for _, ch := range []string{"m", "i", "l", "k"} {
		vb.Inject(tuikit.Event{Tag: "key", Key: ch, Char: ch})
	}
	vb.Inject(tuikit.Event{Tag: "key", Key: "enter", Char: ""})
	pollScreen(t, vb, "[ ] milk")
	// hop to the list and toggle it done — the PUT worker round-trips
	vb.Inject(tuikit.Event{Tag: "key", Key: "tab", Char: ""})
	vb.Inject(tuikit.Event{Tag: "key", Key: "space", Char: ""})
	pollScreen(t, vb, "[x] milk")
	// quit carries the synced state out
	vb.Inject(tuikit.Event{Tag: "key", Key: "q", Char: "q"})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if got := appLastString(t, res.out); got != "1|milk|true|true" {
			t.Fatalf("final state = %q, want 1|milk|true|true", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the client app never returned")
	}
}
