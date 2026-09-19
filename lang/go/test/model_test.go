package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// The boru:model module is a thin boru surface over the upstream Go
// implementation of @voxgig/model (github.com/voxgig/model): it unifies
// .jsonic source into one system model (via aontu) and runs boru-function
// actions over it. These tests drive Model.new / run / model / actions
// through the live module.

const modelImp = `import "boru:model"  `

// TestModelRunInline pins the build-once path over inline source: the result
// reports ok, and Model.model returns the unified model.
func TestModelRunInline(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	prog := modelImp + `def m (Model.new {src:'a: 1 b: 2'}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "true" {
		t.Fatalf("run ok: got %v, want true", got)
	}
	if got := fmt.Sprintf("%v", runLast(t, a, modelImp+`def m (Model.new {src:'a: 1 b: 2'}) Model.run m drop (Model.model m) get 'b'`)); got != "2" {
		t.Errorf("model b: got %v, want 2", got)
	}
}

// TestModelType pins that a Model value reports type Model.
func TestModelType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := fmt.Sprintf("%v", runLast(t, a, modelImp+`typeof (Model.new {src:'a: 1'})`)); got != "Model" {
		t.Errorf("typeof Model.new: got %v, want Model", got)
	}
}

// TestModelActionSeesModel pins the action bridge: a boru-function action
// receives the unified model Map and its result controls the build ok flag.
func TestModelActionSeesModel(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	okProg := modelImp + `def m (Model.new {src:'a: 1', actions:{check:([mod:Any] => [({ok: ((mod get 'a') eq 1)})])}}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, okProg)); got != "true" {
		t.Errorf("action ok: got %v, want true", got)
	}
	// An action returning false fails the build.
	badProg := modelImp + `def m (Model.new {src:'a: 1', actions:{bad:([mod:Any] => [false])}}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, badProg)); got != "false" {
		t.Errorf("failing action: got %v, want false", got)
	}
}

// TestModelFilePath pins file-path source: a .jsonic file on disk resolves
// the same as inline source.
func TestModelFilePath(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "model.jsonic")
	if werr := os.WriteFile(file, []byte("svc: name: 'orders'"), 0o600); werr != nil {
		t.Fatalf("write model: %v", werr)
	}
	prog := fmt.Sprintf("%sdef m (Model.new {path:'%s', base:'%s', dryrun:true}) Model.run m drop ((Model.model m) get 'svc') get 'name'",
		modelImp, file, dir)
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "orders" {
		t.Errorf("file path model: got %v, want orders", got)
	}
}

// TestModelMemFS pins the core capability requirement: with an in-memory
// FileOps installed, the model reads its source and writes <base>/<name>.json
// entirely in memory — no disk access. The source file is seeded into the mem
// FS, the model is built from it, and the produced JSON is read back out of
// the same mem FS.
func TestModelMemFS(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	mem := capabilities.NewMem()
	mem.Files["/proj/model.jsonic"] = []byte("svc: name: 'orders'\nsvc: port: 8080")
	a.SetFileOps(mem)

	prog := modelImp + `def m (Model.new {path:'/proj/model.jsonic', base:'/proj'}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "true" {
		t.Fatalf("mem-fs run ok: got %v, want true", got)
	}
	// The model writer wrote the unified model to /proj/model.json in the mem FS.
	out, ok := mem.Files["/proj/model.json"]
	if !ok {
		t.Fatalf("expected /proj/model.json written to the in-memory FS; files: %v", keysOf(mem.Files))
	}
	if !strings.Contains(string(out), "orders") || !strings.Contains(string(out), "8080") {
		t.Errorf("written model JSON missing fields: %s", out)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestModelWatchForkNoRace exercises the watch path: Model.start forks the
// registry so the watch goroutine's rebuilds run their boru actions on an
// isolated registry, never racing the foreground interpreter. The test rewrites
// the source (driving watch-goroutine rebuilds that run the action) while the
// MAIN goroutine keeps executing boru on the live registry; under `go test
// -race` this proves the two never touch the same registry. (Without the fork,
// the action's CallBoru would run on the foreground registry and the race
// detector would fire.)
func TestModelWatchForkNoRace(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "m.jsonic")
	if err := os.WriteFile(src, []byte("n: 1"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	// The action does a little work touching the model, so each watch-goroutine
	// rebuild holds its registry through a real CallBoru — a wide window for the
	// race detector to observe overlap if it ran on the foreground registry.
	start := fmt.Sprintf("%sdef mdl (Model.new {path:'%s', base:'%s', actions:{tick:([mod:Any] => [((mod get 'n') add 1)])}}) (Model.start mdl) get 'ok'",
		modelImp, src, dir)
	if got := aStr(t, a, start); got != "true" {
		t.Fatalf("start ok: %v", got)
	}

	// Background: rewrite the source a few times with gaps LONGER than the
	// watch's idle debounce (111ms), so each change stabilises and fires a
	// rebuild on the watch goroutine while the foreground loop below is busy.
	go func() {
		for i := 2; i <= 4; i++ {
			time.Sleep(180 * time.Millisecond)
			_ = os.WriteFile(src, []byte(fmt.Sprintf("n: %d", i)), 0o600)
		}
	}()

	// Foreground: keep running boru on the live registry for the whole window,
	// concurrent with the watch goroutine's rebuilds.
	deadline := time.Now().Add(900 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := aStr(t, a, "6 mul 7"); got != "42" {
			t.Fatalf("foreground boru corrupted during watch: got %v, want 42", got)
		}
	}
	if _, err := runReference(t, a, "Model.stop mdl"); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// TestModelConflict pins that a unification conflict in the source surfaces
// as a failed build (ok:false) rather than a crash.
func TestModelConflict(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	prog := modelImp + `def m (Model.new {src:'a: 1 a: 2'}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "false" {
		t.Errorf("conflict ok: got %v, want false", got)
	}
}

// TestModelErrors pins the loud failures of the spec and word contract.
func TestModelErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"no source", modelImp + `Model.new {}`, "model_bad_spec"},
		{"bad action", modelImp + `Model.new {src:'a:1', actions:{x:42}}`, "model_bad_action"},
		{"model before build", modelImp + `Model.model (Model.new {src:'a:1'})`, "model_not_built"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}
