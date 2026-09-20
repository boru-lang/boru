package modules

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// Wave-4 coverage for modules/model.go — the boru:model module (Model.new /
// run / start / stop / model), the spec decoding (parseSpec, buildActions,
// actionFnAndStep, parseStep), the action bridge (makeAction,
// interpretActionResult), the FileOps-backed model.FS (fileOpsFS,
// synthFileInfo), and the small Map readers — each positive path paired
// with the loud rejection.

const mw4Imp = `import "boru:model"  `

// mw4Reg builds a resolver-wired registry with an in-memory FileOps, so the
// whole model pipeline (inline scratch dir, source reads, model.json write)
// runs without touching disk.
func mw4Reg(t *testing.T) (*native.Registry, *capabilities.MemFileOps) {
	t.Helper()
	r := mcovReg(t)
	mem := capabilities.NewMem()
	native.SetHostFileOps(r, mem)
	return r, mem
}

// mw4OkFlag runs src and reads the trailing Boolean.
func mw4OkFlag(t *testing.T, r *native.Registry, src string) bool {
	t.Helper()
	out := mcovRun(t, r, src)
	b, err := out[len(out)-1].AsConcreteBoolean()
	if err != nil {
		t.Fatalf("expected Boolean result, got %v (%v)", out, err)
	}
	return b
}

// TestModelWave4RunInlineMemFS pins the inline-src happy path end to end on
// the in-memory FS: Model.new decodes the spec, the build reads the scratch
// source and writes <base>/model.json, and Model.model returns the unified
// model afterwards.
func TestModelWave4RunInlineMemFS(t *testing.T) {
	r, mem := mw4Reg(t)
	if !mw4OkFlag(t, r, mw4Imp+`def m (Model.new {src:'a: 1 b: 2'}) (Model.run m) get 'ok'`) {
		t.Fatal("inline run ok flag = false, want true")
	}
	// The build wrote the unified model JSON into the mem FS scratch dir.
	found := false
	for name := range mem.Files {
		if strings.HasSuffix(name, "model.json") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a model.json written to the mem FS, files: %v", mem.Files)
	}

	r2, _ := mw4Reg(t)
	out := mcovRun(t, r2, mw4Imp+`def m (Model.new {src:'a: 1 b: 2'}) Model.run m drop (Model.model m) get 'b'`)
	if n, err := out[len(out)-1].AsConcreteInteger(); err != nil || n != 2 {
		t.Errorf("Model.model b = %v (%v), want 2", out[len(out)-1], err)
	}
}

// TestModelWave4PathBaseDefault pins the path-source arm: with no base, the
// base defaults to the source file's directory (and the model resolves).
func TestModelWave4PathBaseDefault(t *testing.T) {
	r, mem := mw4Reg(t)
	mem.Files["/proj/m.jsonic"] = []byte("svc: name: 'orders'")
	out := mcovRun(t, r, mw4Imp+
		`def m (Model.new {path:'/proj/m.jsonic'}) Model.run m drop ((Model.model m) get 'svc') get 'name'`)
	if s, err := out[len(out)-1].AsConcreteString(); err != nil || s != "orders" {
		t.Errorf("path model name = %v (%v), want orders", out[len(out)-1], err)
	}
}

// TestModelWave4SpecFields pins the optional spec decoders: base, args,
// dryrun, order and watch all decode without error, and dryrun keeps the
// model.json write in the overlay (no mem-FS write).
func TestModelWave4SpecFields(t *testing.T) {
	r, mem := mw4Reg(t)
	mem.Files["/proj/m.jsonic"] = []byte("n: 1")
	ok := mw4OkFlag(t, r, mw4Imp+
		`def m (Model.new {path:'/proj/m.jsonic', base:'/proj', dryrun:true, args:{k:'v'}, order:['gen'], watch:{mod:true add:false rem:true}, actions:{gen:([mod:Any] => [true])}}) (Model.run m) get 'ok'`)
	if !ok {
		t.Fatal("spec-fields run ok = false, want true")
	}
	if _, wrote := mem.Files["/proj/model.json"]; wrote {
		t.Error("dryrun must keep the model.json write in the overlay, not the FileOps")
	}
}

// TestModelWave4ActionSteps pins the actionFnAndStep map form with each step
// spelling (pre / all / post / unknown-defaults-to-post) plus the direct
// Function form.
func TestModelWave4ActionSteps(t *testing.T) {
	r, _ := mw4Reg(t)
	ok := mw4OkFlag(t, r, mw4Imp+`def m (Model.new {src:'a: 1', actions:{
		p:{run:([mod:Any] => [true]), step:'pre'},
		q:{run:([mod:Any] => [true]), step:'all'},
		s:{run:([mod:Any] => [true]), step:'post'},
		z:{run:([mod:Any] => [true]), step:'later'},
		d:([mod:Any] => [true])}}) (Model.run m) get 'ok'`)
	if !ok {
		t.Error("stepped actions run ok = false, want true")
	}
}

// TestModelWave4ActionResults pins interpretActionResult: no result → OK,
// Boolean → the flag, Map → {ok reload} (with non-Boolean fields ignored),
// any other value → OK.
func TestModelWave4ActionResults(t *testing.T) {
	cases := []struct {
		name, body string
		wantOK     bool
	}{
		{"empty result is ok", `([mod:Any] => [mod drop])`, true},
		{"boolean true", `([mod:Any] => [true])`, true},
		{"boolean false fails", `([mod:Any] => [false])`, false},
		{"map ok true reload", `([mod:Any] => [{ok:true reload:true}])`, true},
		{"map ok false", `([mod:Any] => [{ok:false}])`, false},
		{"map non-boolean ok ignored", `([mod:Any] => [{ok:1 reload:2}])`, true},
		{"non-boolean non-map is ok", `([mod:Any] => [42])`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := mw4Reg(t)
			src := mw4Imp + `def m (Model.new {src:'a: 1', actions:{x:` + c.body + `}}) (Model.run m) get 'ok'`
			if got := mw4OkFlag(t, r, src); got != c.wantOK {
				t.Errorf("run ok = %v, want %v", got, c.wantOK)
			}
		})
	}
}

// TestModelWave4ActionErrors pins the makeAction failure arms: an action
// whose signature cannot bind the model Map, and an action whose body
// raises — both fail the build and surface in errs.
func TestModelWave4ActionErrors(t *testing.T) {
	r, _ := mw4Reg(t)
	out := mcovRun(t, r, mw4Imp+
		`def m (Model.new {src:'a: 1', actions:{two:([a:Any b:Any] => [true])}}) (Model.run m)`)
	res, err := native.AsMap(out[len(out)-1])
	if err != nil {
		t.Fatalf("run result not a Map: %v", err)
	}
	okV, _ := res.Get("ok")
	if b, _ := okV.AsConcreteBoolean(); b {
		t.Error("arity-mismatched action: ok = true, want false")
	}
	errsV, _ := res.Get("errs")
	if lst, lerr := native.AsList(errsV); lerr != nil || lst.Len() == 0 ||
		!strings.Contains(lst.Get(0).String(), "no matching signature") {
		t.Errorf("errs should carry the no-matching-signature message, got %v", errsV)
	}

	r2, _ := mw4Reg(t)
	out = mcovRun(t, r2, mw4Imp+
		`def m (Model.new {src:'a: 1', actions:{boom:([mod:Any] => [no-such-word-wave4])}}) (Model.run m)`)
	res, err = native.AsMap(out[len(out)-1])
	if err != nil {
		t.Fatalf("run result not a Map: %v", err)
	}
	okV, _ = res.Get("ok")
	if b, _ := okV.AsConcreteBoolean(); b {
		t.Error("raising action: ok = true, want false")
	}
	errsV, _ = res.Get("errs")
	if lst, lerr := native.AsList(errsV); lerr != nil || lst.Len() == 0 ||
		!strings.Contains(lst.Get(0).String(), "action:") {
		t.Errorf("errs should carry the action error, got %v", errsV)
	}
}

// TestModelWave4SpecRejections pins the loud spec failures of Model.new.
func TestModelWave4SpecRejections(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"no source", `Model.new {}`, "model_bad_spec"},
		{"actions not a map value", `Model.new {src:'a:1', actions:42}`, "model_bad_action"},
		{"actions type literal", `Model.new {src:'a:1', actions:Integer}`, "model_bad_action"},
		{"action entry scalar", `Model.new {src:'a:1', actions:{x:42}}`, "model_bad_action"},
		{"action map missing run", `Model.new {src:'a:1', actions:{x:{step:'pre'}}}`, "model_bad_action"},
		{"action run not a function", `Model.new {src:'a:1', actions:{x:{run:42}}}`, "model_bad_action"},
		{"model before build", `Model.model (Model.new {src:'a:1'})`, "model_not_built"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := mw4Reg(t)
			err := mcovErr(t, r, mw4Imp+c.src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v does not contain %q", err, c.want)
			}
		})
	}
}

// TestModelWave4ConflictErrs pins buildResultValue's error marshalling: a
// unification conflict fails the build and lands in errs.
func TestModelWave4ConflictErrs(t *testing.T) {
	r, _ := mw4Reg(t)
	out := mcovRun(t, r, mw4Imp+`def m (Model.new {src:'a: 1 a: 2'}) (Model.run m)`)
	res, err := native.AsMap(out[len(out)-1])
	if err != nil {
		t.Fatalf("run result not a Map: %v", err)
	}
	okV, _ := res.Get("ok")
	if b, _ := okV.AsConcreteBoolean(); b {
		t.Error("conflicting source: ok = true, want false")
	}
	errsV, _ := res.Get("errs")
	if lst, lerr := native.AsList(errsV); lerr != nil || lst.Len() == 0 {
		t.Errorf("conflicting source should report errs, got %v", errsV)
	}
}

// TestModelWave4StartStop pins the watch path: Model.start builds once on a
// forked registry (ok result), Model.stop ends the watch, and a second stop
// is a safe no-op.
func TestModelWave4StartStop(t *testing.T) {
	r, mem := mw4Reg(t)
	mem.Files["/proj/m.jsonic"] = []byte("n: 1")
	ok := mw4OkFlag(t, r, mw4Imp+
		`def m (Model.new {path:'/proj/m.jsonic', base:'/proj', actions:{tick:([mod:Any] => [true])}}) (Model.start m) get 'ok'`)
	if !ok {
		t.Fatal("start ok = false, want true")
	}
	mcovRun(t, r, `Model.stop m`)
	mcovRun(t, r, `Model.stop m`) // idempotent
}

// TestModelWave4BadHandles pins asModelHandle's rejections through every
// word handler: a non-Extension value and an Extension wrapping a foreign
// body are both loud model_bad_handle errors, never panics.
func TestModelWave4BadHandles(t *testing.T) {
	r := mcovReg(t)
	notModel := native.NewInteger(7)
	foreign := core.NewExtension(native.TMap, "not-a-model-handle")
	handlers := map[string]native.Handler{
		"run":   modelRunHandler,
		"start": modelStartHandler,
		"stop":  modelStopHandler,
		"model": modelModelHandler,
	}
	for name, h := range handlers {
		for _, bad := range []native.Value{notModel, foreign} {
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("%s(%v) panicked: %v", name, bad, rec)
					}
				}()
				_, err := h([]native.Value{bad}, nil, nil, r)
				if err == nil || !strings.Contains(err.Error(), "model_bad_handle") {
					t.Errorf("%s(%v): error %v, want model_bad_handle", name, bad, err)
				}
			}()
		}
	}
}

// TestModelWave4NewHandlerRejections pins modelNewHandlerFor's direct arg
// guards: a Map type literal (non-concrete) and a non-Map concrete value.
func TestModelWave4NewHandlerRejections(t *testing.T) {
	r := mcovReg(t)
	h := modelNewHandlerFor(native.TMap)
	if _, err := h([]native.Value{native.NewTypeLiteral(native.TMap)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "model_bad_spec") {
		t.Errorf("type-literal spec: %v, want model_bad_spec", err)
	}
	if _, err := h([]native.Value{native.NewInteger(3)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "model_bad_spec") {
		t.Errorf("non-map spec: %v, want model_bad_spec", err)
	}
}

// failOpsWave4 is a FileOps whose every operation fails — the seam for the
// model_io arms of the inline-source scratch setup.
type failOpsWave4 struct{ failMkdir bool }

func (f *failOpsWave4) ReadFile(string) ([]byte, error)    { return nil, errors.New("read declined") }
func (f *failOpsWave4) Chown(string, int, int, bool) error { return errors.New("chown declined") }
func (f *failOpsWave4) XattrGet(string, string) ([]byte, error) {
	return nil, errors.New("xattr declined")
}
func (f *failOpsWave4) XattrSet(string, string, []byte) error { return errors.New("xattr declined") }
func (f *failOpsWave4) XattrList(string) ([]string, error) {
	return nil, errors.New("xattr declined")
}
func (f *failOpsWave4) XattrRemove(string, string) error { return errors.New("xattr declined") }
func (f *failOpsWave4) TempFile(string, string) (string, error) {
	return "", errors.New("temp declined")
}
func (f *failOpsWave4) TempDir(string, string) (string, error) {
	return "", errors.New("temp declined")
}
func (f *failOpsWave4) Statfs(string) (capabilities.FsInfo, error) {
	return capabilities.FsInfo{}, errors.New("statfs declined")
}
func (f *failOpsWave4) Watch(string, capabilities.WatchOpts) (<-chan capabilities.WatchEvent, func() error, error) {
	return nil, nil, errors.New("watch declined")
}
func (f *failOpsWave4) Open(string, capabilities.OpenOpts) (capabilities.FileHandle, error) {
	return nil, errors.New("open declined")
}
func (f *failOpsWave4) Lock(string, bool, bool) (io.Closer, error) {
	return nil, errors.New("lock declined")
}
func (f *failOpsWave4) Mmap(string, int64, int, bool) (capabilities.MmapRegion, error) {
	return nil, errors.New("mmap declined")
}
func (f *failOpsWave4) WriteFile(string, []byte, os.FileMode) error {
	return errors.New("write declined")
}
func (f *failOpsWave4) MkdirAll(string, os.FileMode) error {
	if f.failMkdir {
		return errors.New("mkdir declined")
	}
	return nil
}
func (f *failOpsWave4) ResolvePath(string) (string, error) { return "", errors.New("no resolve") }

func (f *failOpsWave4) Stat(string, bool) (capabilities.FileInfo, error) {
	return capabilities.FileInfo{}, errors.New("stat declined")
}
func (f *failOpsWave4) ReadDir(string) ([]capabilities.FileInfo, error) {
	return nil, errors.New("readdir declined")
}
func (f *failOpsWave4) Remove(string, bool) error       { return errors.New("remove declined") }
func (f *failOpsWave4) Rename(string, string) error     { return errors.New("rename declined") }
func (f *failOpsWave4) Symlink(string, string) error    { return errors.New("symlink declined") }
func (f *failOpsWave4) Link(string, string) error       { return errors.New("link declined") }
func (f *failOpsWave4) Chmod(string, os.FileMode) error { return errors.New("chmod declined") }
func (f *failOpsWave4) Chtimes(string, time.Time, time.Time) error {
	return errors.New("chtimes declined")
}
func (f *failOpsWave4) Truncate(string, int64) error { return errors.New("truncate declined") }

// TestModelWave4InlineIOFailures pins the model_io arms: a failing MkdirAll
// and a failing WriteFile during inline scratch setup are loud errors.
func TestModelWave4InlineIOFailures(t *testing.T) {
	r := mcovReg(t)
	native.SetHostFileOps(r, &failOpsWave4{failMkdir: true})
	if err := mcovErr(t, r, mw4Imp+`Model.new {src:'a: 1'}`); err == nil ||
		!strings.Contains(err.Error(), "model_io") || !strings.Contains(err.Error(), "mkdir declined") {
		t.Errorf("mkdir failure: %v, want model_io", err)
	}

	r2 := mcovReg(t)
	native.SetHostFileOps(r2, &failOpsWave4{})
	if err := mcovErr(t, r2, mw4Imp+`Model.new {src:'a: 1'}`); err == nil ||
		!strings.Contains(err.Error(), "model_io") || !strings.Contains(err.Error(), "write declined") {
		t.Errorf("write failure: %v, want model_io", err)
	}
}

// TestModelWave4FileOpsFSDry pins the dryrun overlay of fileOpsFS: writes
// land in (and reads come back from) the overlay; misses fall through to the
// underlying FileOps; MkdirAll is a no-op.
func TestModelWave4FileOpsFSDry(t *testing.T) {
	mem := capabilities.NewMem()
	mem.Files["/proj/under.txt"] = []byte("under")
	f := &fileOpsFS{ops: mem, dry: true, mem: map[string][]byte{}}

	if err := f.WriteFile("/proj/out.txt", []byte("over"), 0o600); err != nil {
		t.Fatalf("dry write: %v", err)
	}
	if _, ok := mem.Files["/proj/out.txt"]; ok {
		t.Error("dry write must not reach the underlying FileOps")
	}
	got, err := f.ReadFile("/proj/out.txt")
	if err != nil || string(got) != "over" {
		t.Errorf("dry read-back = %q (%v), want over", got, err)
	}
	// The returned slice is a copy: mutating it must not change the overlay.
	got[0] = 'X'
	if again, _ := f.ReadFile("/proj/out.txt"); string(again) != "over" {
		t.Error("dry ReadFile must return an independent copy")
	}
	// Overlay miss falls through to the underlying FileOps.
	under, err := f.ReadFile("/proj/under.txt")
	if err != nil || string(under) != "under" {
		t.Errorf("dry fall-through read = %q (%v), want under", under, err)
	}
	if err := f.MkdirAll("/proj/sub", 0o755); err != nil {
		t.Errorf("dry MkdirAll: %v", err)
	}
	if len(mem.Dirs) != 0 {
		t.Error("dry MkdirAll must not reach the underlying FileOps")
	}
}

// TestModelWave4FileOpsFSDelegate pins the non-dry delegation and the
// key() resolve-failure fallback.
func TestModelWave4FileOpsFSDelegate(t *testing.T) {
	mem := capabilities.NewMem()
	f := &fileOpsFS{ops: mem, mem: map[string][]byte{}}
	if err := f.WriteFile("/d/a.txt", []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := f.ReadFile("/d/a.txt"); err != nil || string(got) != "x" {
		t.Errorf("read = %q (%v), want x", got, err)
	}
	if err := f.MkdirAll("/d/sub", 0o755); err != nil || !mem.Dirs["/d/sub"] {
		t.Errorf("MkdirAll should delegate (err %v, dirs %v)", err, mem.Dirs)
	}
	if k := f.key("/d/a.txt"); k != "/d/a.txt" {
		t.Errorf("key resolved = %q", k)
	}
	// A FileOps whose ResolvePath fails: key falls back to the raw name.
	ff := &fileOpsFS{ops: &failOpsWave4{}, mem: map[string][]byte{}}
	if k := ff.key("raw-name"); k != "raw-name" {
		t.Errorf("key fallback = %q, want raw-name", k)
	}
}

// TestModelWave4FileOpsFSNotesEffects pins the C1 effect seam: a non-dry
// write/mkdir reaching the underlying FileOps notes the ledger callback
// exactly once each; dryrun overlay writes never leave the handle and stay
// uncounted.
func TestModelWave4FileOpsFSNotesEffects(t *testing.T) {
	mem := capabilities.NewMem()
	notes := 0
	f := &fileOpsFS{ops: mem, mem: map[string][]byte{}, note: func() { notes++ }}
	if err := f.WriteFile("/d/a.txt", []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.MkdirAll("/d/sub", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if notes != 2 {
		t.Errorf("notes = %d, want 2 (one write + one mkdir)", notes)
	}

	fd := &fileOpsFS{ops: mem, dry: true, mem: map[string][]byte{}, note: func() { notes++ }}
	if err := fd.WriteFile("/d/b.txt", []byte("y"), 0o600); err != nil {
		t.Fatalf("dry write: %v", err)
	}
	if err := fd.MkdirAll("/d/sub2", 0o755); err != nil {
		t.Fatalf("dry mkdir: %v", err)
	}
	if notes != 2 {
		t.Errorf("dry ops noted the ledger: notes = %d, want still 2", notes)
	}
}

// TestModelWave4Stat pins the three Stat arms: a real OS file (real
// FileInfo), an in-memory file (synthetic FileInfo), and a missing file
// (error) — plus every synthFileInfo accessor.
func TestModelWave4Stat(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	fOS := &fileOpsFS{ops: capabilities.NewDefault(), mem: map[string][]byte{}}
	fi, err := fOS.Stat(real)
	if err != nil || fi.Size() != 3 {
		t.Errorf("os Stat = %v (%v), want size 3", fi, err)
	}

	mem := capabilities.NewMem()
	mem.Files["/m/x.txt"] = []byte("hello")
	fMem := &fileOpsFS{ops: mem, mem: map[string][]byte{}}
	fi, err = fMem.Stat("/m/x.txt")
	if err != nil {
		t.Fatalf("mem Stat: %v", err)
	}
	if fi.Name() != "x.txt" || fi.Size() != 0 || fi.Mode() != 0o644 ||
		!fi.ModTime().IsZero() || fi.IsDir() || fi.Sys() != nil {
		t.Errorf("synthetic FileInfo fields off: %v %v %v %v %v %v",
			fi.Name(), fi.Size(), fi.Mode(), fi.ModTime(), fi.IsDir(), fi.Sys())
	}
	if _, err := fMem.Stat("/m/missing.txt"); err == nil {
		t.Error("Stat of a missing in-memory file must error")
	}
}

// TestModelWave4MapReaders pins mapStr / mapBool / mapStrList across the
// present / absent / wrong-type / mixed-element arms.
func TestModelWave4MapReaders(t *testing.T) {
	om := native.NewOrderedMap()
	om.Set("s", native.NewString("hi"))
	om.Set("i", native.NewInteger(4))
	om.Set("b", native.NewBoolean(true))
	om.Set("lit", native.NewTypeLiteral(native.TString))
	om.Set("mixed", native.NewList([]native.Value{
		native.NewString("a"), native.NewAtom("b"), native.NewInteger(9),
	}))

	if s, ok := mapStr(om, "s"); !ok || s != "hi" {
		t.Errorf("mapStr s = %q %v", s, ok)
	}
	if _, ok := mapStr(om, "absent"); ok {
		t.Error("mapStr absent should be false")
	}
	if _, ok := mapStr(om, "i"); ok {
		t.Error("mapStr on an Integer should be false")
	}
	if _, ok := mapStr(om, "lit"); ok {
		t.Error("mapStr on a type literal should be false")
	}

	if b, ok := mapBool(om, "b"); !ok || !b {
		t.Errorf("mapBool b = %v %v", b, ok)
	}
	if _, ok := mapBool(om, "absent"); ok {
		t.Error("mapBool absent should be false")
	}
	if _, ok := mapBool(om, "i"); ok {
		t.Error("mapBool on an Integer should be false")
	}

	if lst, ok := mapStrList(om, "mixed"); !ok || len(lst) != 2 || lst[0] != "a" || lst[1] != "b" {
		t.Errorf("mapStrList mixed = %v %v, want [a b] (Integer skipped)", lst, ok)
	}
	if _, ok := mapStrList(om, "absent"); ok {
		t.Error("mapStrList absent should be false")
	}
	if _, ok := mapStrList(om, "i"); ok {
		t.Error("mapStrList on an Integer should be false")
	}
}

// TestModelWave4ExtractTimeZero pins extractTime's zero fall-through for a
// non-time payload (shared helper used by the temporal words).
func TestModelWave4ExtractTimeZero(t *testing.T) {
	if got := extractTime(native.NewInteger(1)); !got.IsZero() {
		t.Errorf("extractTime(Integer) = %v, want zero time", got)
	}
}
