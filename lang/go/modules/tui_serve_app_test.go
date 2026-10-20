package modules

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// The graduated acceptance app (design/examples/apps/todo-tui.boru)
// served over a real wire: the SAME app map that TestAppTodoTUI runs
// locally (lang/go/test) is loaded as a FILE MODULE here and driven
// through TodoTui.serve by a raw json-lines viewer — the §5 run/serve
// reuse idiom end to end.
func TestTuiServeGraduatedTodoApp(t *testing.T) {
	src, err := os.ReadFile("../../../design/examples/apps/todo-tui.boru")
	if err != nil {
		t.Fatalf("read the graduated app: %v", err)
	}
	mem := capabilities.NewMem()
	mem.Files[filepath.FromSlash("/apps/todo-tui.boru")] = src

	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }

	reg := tcReg(t)
	native.SetHostFileOps(reg, mem)
	finals := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		out, sErr := runTuiStepsOn(t, reg, []string{
			`import "/apps/todo-tui.boru"`,
			`def final (TodoTui.serve {tcp: 0  token: "s3cret"})`,
			`(final.todos get 0).text`,
		})
		if sErr != nil {
			errs <- sErr
			return
		}
		finals <- func() string {
			s, _ := out[len(out)-1].AsConcreteString()
			return s
		}()
	}()
	var addr net.Addr
	select {
	case addr = <-bound:
	case sErr := <-errs:
		t.Fatalf("serve failed before binding: %v", sErr)
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
	}

	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 44, "rows": 12, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	// type "hi", submit, hop to the list, quit — the layout stays
	// client-side: every frame is a whole widget tree
	sawTree := false
	for _, ev := range []map[string]any{
		{"tag": "key", "key": "h", "char": "h", "mods": []string{}},
		{"tag": "key", "key": "i", "char": "i", "mods": []string{}},
		{"tag": "key", "key": "enter", "char": "", "mods": []string{}},
		{"tag": "key", "key": "tab", "char": "", "mods": []string{}},
		{"tag": "key", "key": "q", "char": "q", "mods": []string{}},
	} {
		c.send(t, ev)
	}
	for {
		msg := c.recv(t)
		if msg["tag"] == "frame" {
			if tree, ok := msg["tree"].(map[string]any); ok && tree["w"] == "rows" {
				sawTree = true
			}
			continue
		}
		if msg["tag"] == "quit" {
			break
		}
	}
	if !sawTree {
		t.Fatal("no rows-shaped widget tree crossed the wire")
	}
	select {
	case final := <-finals:
		if final != "hi" {
			t.Fatalf("final todo = %q, want hi", final)
		}
	case sErr := <-errs:
		t.Fatalf("serve error: %v", sErr)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return")
	}
}
