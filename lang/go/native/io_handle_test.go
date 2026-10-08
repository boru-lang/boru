package native

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_handle_test.go — P4 word layer: IO.open / seek / flush / close, the
// File overloads on read/write, the {exclusive} create path, and the
// permissioned / notinstalled / mount postures for Open.

// openFileVal opens a handle through the IO.open word and returns the File.
func openFileVal(t *testing.T, r *Registry, path string, opts func(*OrderedMap)) Value {
	t.Helper()
	res := runBoru(t, r, []Value{NewWord("open"), pathV(path), wrapMap(opts)})
	if _, ok := asFileHandle(res[0]); !ok {
		t.Fatalf("open did not return a File handle: %v", res[0])
	}
	return res[0]
}

func TestOpenWordForms(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Default open ({}) is read-only.
	f := openFileVal(t, r, "f.txt", func(*OrderedMap) {})
	fh, _ := asFileHandle(f)
	if fh.Path != "f.txt" {
		t.Errorf("handle path = %q", fh.Path)
	}
	runBoru(t, r, []Value{NewWord("close"), f})

	// {mode:'write'} creates + truncates.
	runBoru(t, r, []Value{NewWord("open"), pathV("new.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("mode", NewString("write")) })})
	if _, err := mem.Stat("new.txt", false); err != nil {
		t.Errorf("write-open did not create the file: %v", err)
	}

	// An absent read target errors; an unknown mode errors.
	if err := runBoruError(t, r, []Value{NewWord("open"), pathV("ghost")}); err == nil {
		t.Error("opening an absent file read-only should error")
	}
	if err := runBoruError(t, r, []Value{NewWord("open"), pathV("f.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("mode", NewString("bogus")) })}); err == nil {
		t.Error("an unknown mode should error")
	}
	// {perm} + {exclusive} shape the open (exclusive on an absent path
	// succeeds and creates it).
	runBoru(t, r, []Value{NewWord("open"), pathV("x.txt"),
		wrapMap(func(om *OrderedMap) {
			om.Set("mode", NewString("write"))
			om.Set("exclusive", NewBoolean(true))
			om.Set("perm", NewInteger(0o600))
		})})
	if fi, _ := mem.Stat("x.txt", false); fi.Mode.Perm() != 0o600 {
		t.Errorf("perm open = %v", fi.Mode.Perm())
	}
}

func TestHandleReadWriteThread(t *testing.T) {
	r, mem := ioFSReg(t)
	// Open write, write via handle, close; the write is visible to a
	// stateless read before close (mem page-cache), then persists.
	f := openFileVal(t, r, "h.txt", func(om *OrderedMap) { om.Set("mode", NewString("write")) })
	runBoru(t, r, []Value{NewWord("write"), f, NewString("hello world")})
	if b, _ := mem.ReadFile("h.txt"); string(b) != "hello world" {
		t.Errorf("pre-close visible = %q", b)
	}
	runBoru(t, r, []Value{NewWord("close"), f})

	// Open read; seek + sequential read; positioned read; bytes read.
	rf := openFileVal(t, r, "h.txt", func(*OrderedMap) {})
	runBoru(t, r, []Value{NewWord("seek"), rf, NewInteger(6)})
	seq := runBoru(t, r, []Value{NewWord("read"), rf,
		wrapMap(func(om *OrderedMap) { om.Set("length", NewInteger(5)) })})
	if s, _ := AsString(seq[0]); s != "world" {
		t.Errorf("seek+read = %q", s)
	}
	pos := runBoru(t, r, []Value{NewWord("read"), rf,
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(0)); om.Set("length", NewInteger(5)) })})
	if s, _ := AsString(pos[0]); s != "hello" {
		t.Errorf("positioned read = %q", s)
	}
	bin := runBoru(t, r, []Value{NewWord("read"), rf,
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bytes")); om.Set("offset", NewInteger(0)) })})
	if b, ok := AsBytesValue(bin[0]); !ok || len(b) != 11 {
		t.Errorf("bytes read = %v", bin[0])
	}
	runBoru(t, r, []Value{NewWord("close"), rf})
}

func TestHandleWriteForms(t *testing.T) {
	r, mem := ioFSReg(t)
	f := openFileVal(t, r, "w.bin", func(om *OrderedMap) { om.Set("mode", NewString("write")) })
	// A Bytes payload writes verbatim; a positioned {offset} splices.
	runBoru(t, r, []Value{NewWord("write"), f, NewBytesValue([]byte{1, 2, 3, 4})})
	runBoru(t, r, []Value{NewWord("write"), f, NewBytesValue([]byte{9}),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(1)) })})
	runBoru(t, r, []Value{NewWord("flush"), f})
	runBoru(t, r, []Value{NewWord("close"), f})
	if b, _ := mem.ReadFile("w.bin"); len(b) != 4 || b[1] != 9 {
		t.Errorf("handle write forms = %v", b)
	}
	// A write of a non-string/bytes payload errors.
	g := openFileVal(t, r, "w2.txt", func(om *OrderedMap) { om.Set("mode", NewString("write")) })
	if err := runBoruError(t, r, []Value{NewWord("write"), g, wrapMap(func(om *OrderedMap) { om.Set("a", NewInteger(1)) })}); err == nil {
		t.Error("writing a map to a handle should error")
	}
}

func TestSeekForms(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("s.txt", []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := openFileVal(t, r, "s.txt", func(*OrderedMap) {})
	end := runBoru(t, r, []Value{NewWord("seek"), f, NewInteger(0),
		wrapMap(func(om *OrderedMap) { om.Set("from", NewString("end")) })})
	if n, _ := AsInteger(end[0]); n != 10 {
		t.Errorf("seek end = %v", n)
	}
	cur := runBoru(t, r, []Value{NewWord("seek"), f, NewInteger(-3),
		wrapMap(func(om *OrderedMap) { om.Set("from", NewString("current")) })})
	if n, _ := AsInteger(cur[0]); n != 7 {
		t.Errorf("seek current = %v", n)
	}
	st := runBoru(t, r, []Value{NewWord("seek"), f, NewInteger(2),
		wrapMap(func(om *OrderedMap) { om.Set("from", NewString("start")) })})
	if n, _ := AsInteger(st[0]); n != 2 {
		t.Errorf("seek start = %v", n)
	}
	// A bad {from}, and a seek to a negative offset, error.
	if err := runBoruError(t, r, []Value{NewWord("seek"), f, NewInteger(0),
		wrapMap(func(om *OrderedMap) { om.Set("from", NewString("nope")) })}); err == nil {
		t.Error("bad {from} should error")
	}
	if err := runBoruError(t, r, []Value{NewWord("seek"), f, NewInteger(-100)}); err == nil {
		t.Error("seek to a negative offset should error")
	}
	runBoru(t, r, []Value{NewWord("close"), f})
}

func TestCloseAndFlushErrors(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// close/seek/flush/read/write of a NON-handle (a bare Integer) error —
	// these guard arms are reached by calling the handlers directly (the
	// sigs would otherwise reject a non-handle before the handler).
	nonHandle := NewInteger(7)
	for _, fn := range []func([]Value, *Registry) ([]Value, error){doCloseWord, doSeekWord, doFlushWord, readHandleWord, writeHandleWord} {
		if _, err := fn([]Value{nonHandle, NewInteger(0), NewString("x")}, r); err == nil {
			t.Error("a handler on a non-handle should error")
		}
	}
	// A closed handle re-close is a no-op via sync.Once (idempotent).
	f := openFileVal(t, r, "f", func(*OrderedMap) {})
	fh, _ := asFileHandle(f)
	if err := fh.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fh.Close(); err != nil {
		t.Errorf("second Close should be a clean no-op: %v", err)
	}
}

func TestCloseWatcher(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("w.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// IO.close is polymorphic: it also stops a Watcher.
	body := NewList([]Value{NewWord("drop")})
	body.Quoted = true
	w := runBoru(t, r, []Value{NewWord("watch"), pathV("w.txt"), body})
	if _, ok := asWatcherInfo(w[0]); !ok {
		t.Fatalf("watch did not return a Watcher: %v", w[0])
	}
	runBoru(t, r, []Value{NewWord("close"), w[0]})
}

func TestExclusiveWriteWord(t *testing.T) {
	r, mem := ioFSReg(t)
	// Exclusive create on an absent path succeeds.
	runBoru(t, r, []Value{NewWord("write"), pathV("e.txt"), NewString("fresh"),
		wrapMap(func(om *OrderedMap) { om.Set("exclusive", NewBoolean(true)) })})
	if b, _ := mem.ReadFile("e.txt"); string(b) != "fresh" {
		t.Errorf("exclusive create = %q", b)
	}
	// Exclusive on an existing path declines.
	if err := runBoruError(t, r, []Value{NewWord("write"), pathV("e.txt"), NewString("x"),
		wrapMap(func(om *OrderedMap) { om.Set("exclusive", NewBoolean(true)) })}); err == nil {
		t.Error("exclusive over an existing file should decline")
	}
	// Exclusive cannot combine with append.
	if err := runBoruError(t, r, []Value{NewWord("write"), pathV("e2.txt"), NewString("x"),
		wrapMap(func(om *OrderedMap) {
			om.Set("exclusive", NewBoolean(true))
			om.Set("mode", NewString("append"))
		})}); err == nil {
		t.Error("exclusive+append should decline")
	}
	// Exclusive cannot combine with a positioned {offset} (binary path).
	if err := runBoruError(t, r, []Value{NewWord("write"), pathV("e3.bin"), NewBytesValue([]byte{1}),
		wrapMap(func(om *OrderedMap) {
			om.Set("exclusive", NewBoolean(true))
			om.Set("offset", NewInteger(0))
		})}); err == nil {
		t.Error("exclusive+offset should decline")
	}
}

func TestNotInstalledOpen(t *testing.T) {
	var ops capabilities.FileOps = notInstalledFileOps{}
	if _, err := ops.Open("x", capabilities.OpenOpts{Read: true}); err == nil {
		t.Error("not-installed Open should error")
	}
}

func TestPermissionedOpen(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	var d *policy.Denied
	if _, err := ops.Open("/tmp/x", capabilities.OpenOpts{Read: true}); !errors.As(err, &d) {
		t.Errorf("read-open gate: %v", err)
	}
	if _, err := ops.Open("/tmp/x", capabilities.OpenOpts{Write: true, Create: true}); !errors.As(err, &d) {
		t.Errorf("write-open gate: %v", err)
	}
	// Trusted passes through to the inner backend.
	mem := capabilities.NewMem()
	if err := mem.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapped := NewPermissionedFileOps(mem, loadPolicy(t, "trusted"))
	if _, err := wrapped.Open("f", capabilities.OpenOpts{Read: true}); err != nil {
		t.Errorf("trusted open: %v", err)
	}
}

func TestMountOpenEmulation(t *testing.T) {
	ops := HostFileOps(mountFixture(t, `def files (flex {})  mount {
	  read: (p:Pathon => [files get `+"`${p}`"+`])
	  write: ([p:Pathon d:Any] => [files set `+"`${p}`"+` d drop])
	}`))

	// Create + write through a handle; nothing is visible to a stateless
	// read until Sync/Close (the buffered-emulation durability caveat).
	h, err := ops.Open("a.txt", capabilities.OpenOpts{Read: true, Write: true, Create: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Write([]byte("buffered")); err != nil {
		t.Fatal(err)
	}
	if b, _ := ops.ReadFile("a.txt"); string(b) == "buffered" {
		t.Error("mount handle write should NOT be visible before flush")
	}
	if err := h.Sync(); err != nil {
		t.Fatal(err)
	}
	if b, _ := ops.ReadFile("a.txt"); string(b) != "buffered" {
		t.Errorf("post-sync content = %q", b)
	}
	// Positioned + seek + read on the buffered handle.
	if _, err := h.WriteAt([]byte("X"), 0); err != nil {
		t.Fatal(err)
	}
	if off, err := h.Seek(0, io.SeekStart); err != nil || off != 0 {
		t.Fatalf("seek: %d %v", off, err)
	}
	got := make([]byte, 3)
	n, _ := h.Read(got)
	if string(got[:n]) != "Xuf" {
		t.Errorf("buffered read = %q", got[:n])
	}
	rn := make([]byte, 2)
	if _, err := h.ReadAt(rn, 1); err != nil {
		t.Fatal(err)
	}
	if err := h.Truncate(2); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	// Every op on the closed handle errors.
	if err := h.Sync(); err == nil {
		t.Error("sync on closed mount handle")
	}
	if err := h.Close(); err == nil {
		t.Error("double close on mount handle")
	}
	if _, err := h.Read(got); err == nil {
		t.Error("read on closed mount handle")
	}
	if _, err := h.ReadAt(got, 0); err == nil {
		t.Error("readat on closed mount handle")
	}
	if _, err := h.Write(got); err == nil {
		t.Error("write on closed mount handle")
	}
	if _, err := h.WriteAt(got, 0); err == nil {
		t.Error("writeat on closed mount handle")
	}
	if _, err := h.Seek(0, io.SeekStart); err == nil {
		t.Error("seek on closed mount handle")
	}
	if err := h.Truncate(0); err == nil {
		t.Error("truncate on closed mount handle")
	}
}

func TestMountOpenArms(t *testing.T) {
	ops := HostFileOps(mountFixture(t, `def files (flex {})  mount {
	  read: (p:Pathon => [files get `+"`${p}`"+`])
	  write: ([p:Pathon d:Any] => [files set `+"`${p}`"+` d drop])
	}`))

	if err := ops.WriteFile("here.txt", []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Read-only open of an existing entry; a truncate open starts empty.
	if _, err := ops.Open("here.txt", capabilities.OpenOpts{Read: true}); err != nil {
		t.Fatalf("read open: %v", err)
	}
	if _, err := ops.Open("here.txt", capabilities.OpenOpts{Write: true, Truncate: true}); err != nil {
		t.Fatalf("truncate open: %v", err)
	}
	// Absent + no-create errors; exclusive over an existing entry errors.
	if _, err := ops.Open("absent", capabilities.OpenOpts{Read: true}); err == nil {
		t.Error("mount open absent no-create should error")
	}
	if _, err := ops.Open("here.txt", capabilities.OpenOpts{Write: true, Create: true, Exclusive: true}); err == nil {
		t.Error("mount exclusive over existing should error")
	}
	// A read-only handle declines writes; a write-only handle declines reads.
	rh, _ := ops.Open("here.txt", capabilities.OpenOpts{Read: true})
	if _, err := rh.Write([]byte("x")); err == nil {
		t.Error("write on a read-only mount handle")
	}
	if _, err := rh.WriteAt([]byte("x"), 0); err == nil {
		t.Error("writeat on a read-only mount handle")
	}
	if err := rh.Truncate(0); err == nil {
		t.Error("truncate on a read-only mount handle")
	}
	_ = rh.Close()
	wh, _ := ops.Open("here.txt", capabilities.OpenOpts{Write: true})
	if _, err := wh.Read(make([]byte, 1)); err == nil {
		t.Error("read on a write-only mount handle")
	}
	if _, err := wh.ReadAt(make([]byte, 1), 0); err == nil {
		t.Error("readat on a write-only mount handle")
	}
	// Negative offsets and a bad whence are invalid.
	ah, _ := ops.Open("here.txt", capabilities.OpenOpts{Read: true, Write: true})
	if _, err := ah.ReadAt(make([]byte, 1), -1); err == nil {
		t.Error("negative readat")
	}
	if _, err := ah.WriteAt([]byte("x"), -1); err == nil {
		t.Error("negative writeat")
	}
	if _, err := ah.Seek(0, 99); err == nil {
		t.Error("bad whence")
	}
	if _, err := ah.Seek(-1, io.SeekStart); err == nil {
		t.Error("negative seek")
	}
	if err := ah.Truncate(-1); err == nil {
		t.Error("negative truncate")
	}
	// A write-append handle appends at end; ReadAt past EOF is EOF.
	if _, err := ah.ReadAt(make([]byte, 2), 100); err != io.EOF {
		t.Errorf("mount readat past EOF")
	}
	_ = ah.Name()
	_ = ah.Close()
}

// failCapHandle wraps a real handle but can fail chosen methods, to reach
// the word layer's error-forward arms.
type failCapHandle struct {
	capabilities.FileHandle
	failRead, failWrite, failClose, failSync bool
}

func (h failCapHandle) Read(p []byte) (int, error) {
	if h.failRead {
		return 0, errors.New("read boom")
	}
	return h.FileHandle.Read(p)
}
func (h failCapHandle) ReadAt(p []byte, off int64) (int, error) {
	if h.failRead {
		return 0, errors.New("readat boom")
	}
	return h.FileHandle.ReadAt(p, off)
}
func (h failCapHandle) Write(p []byte) (int, error) {
	if h.failWrite {
		return 0, errors.New("write boom")
	}
	return h.FileHandle.Write(p)
}
func (h failCapHandle) Close() error {
	if h.failClose {
		return errors.New("close boom")
	}
	return h.FileHandle.Close()
}
func (h failCapHandle) Sync() error {
	if h.failSync {
		return errors.New("sync boom")
	}
	return h.FileHandle.Sync()
}

// failOpenOps returns a preset (possibly failing) handle from Open.
type failOpenOps struct {
	*capabilities.MemFileOps
	handle capabilities.FileHandle
}

func (o *failOpenOps) Open(string, capabilities.OpenOpts) (capabilities.FileHandle, error) {
	return o.handle, nil
}

func TestOpenOptsModeArms(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f", []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	// append mode: writes land at end.
	af := openFileVal(t, r, "f", func(om *OrderedMap) { om.Set("mode", NewString("append")) })
	runBoru(t, r, []Value{NewWord("write"), af, NewString("B")})
	runBoru(t, r, []Value{NewWord("close"), af})
	if b, _ := mem.ReadFile("f"); string(b) != "AB" {
		t.Errorf("append mode = %q", b)
	}
	// explicit read mode.
	rd := openFileVal(t, r, "f", func(om *OrderedMap) { om.Set("mode", NewString("read")) })
	runBoru(t, r, []Value{NewWord("close"), rd})
	// rw mode: read and write.
	rw := openFileVal(t, r, "f", func(om *OrderedMap) { om.Set("mode", NewString("rw")) })
	runBoru(t, r, []Value{NewWord("close"), rw})
	// create + truncate refinements on a read base.
	openFileVal(t, r, "nc.txt", func(om *OrderedMap) { om.Set("create", NewBoolean(true)); om.Set("mode", NewString("rw")) })
	if _, err := mem.Stat("nc.txt", false); err != nil {
		t.Errorf("{create} did not make the file: %v", err)
	}
	if err := mem.WriteFile("tr.txt", []byte("xyz"), 0o644); err != nil {
		t.Fatal(err)
	}
	tf := openFileVal(t, r, "tr.txt", func(om *OrderedMap) { om.Set("mode", NewString("rw")); om.Set("truncate", NewBoolean(true)) })
	runBoru(t, r, []Value{NewWord("close"), tf})
	if b, _ := mem.ReadFile("tr.txt"); len(b) != 0 {
		t.Errorf("{truncate} left %q", b)
	}
}

func TestReadHandleVariants(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("r.txt", []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := openFileVal(t, r, "r.txt", func(*OrderedMap) {})
	// Sequential, no length → io.ReadAll to EOF.
	all := runBoru(t, r, []Value{NewWord("read"), f})
	if s, _ := AsString(all[0]); s != "hello world" {
		t.Errorf("read-all = %q", s)
	}
	// Positioned, no length → readAtToEnd.
	pos := runBoru(t, r, []Value{NewWord("read"), f,
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(6)) })})
	if s, _ := AsString(pos[0]); s != "world" {
		t.Errorf("positioned to EOF = %q", s)
	}
	// An unknown enc fails to decode.
	if err := runBoruError(t, r, []Value{NewWord("read"), f,
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bogus")); om.Set("offset", NewInteger(0)) })}); err == nil {
		t.Error("read with a bogus enc should error")
	}
	// seek with a non-integer offset (reached by calling the handler
	// directly — the sig would reject it).
	if _, err := doSeekWord([]Value{f, NewString("x")}, r); err == nil {
		t.Error("seek with a non-integer should error")
	}
	// flush of a CLOSED handle errors (Sync declines).
	runBoru(t, r, []Value{NewWord("close"), f})
	if _, err := doFlushWord([]Value{f}, r); err == nil {
		t.Error("flush of a closed handle should error")
	}
}

func TestHandleErrorForwards(t *testing.T) {
	newReg := func(h capabilities.FileHandle) *Registry {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		registerIOWords(r)
		mem := capabilities.NewMem()
		_ = mem.WriteFile("f", []byte("data"), 0o644)
		SetHostFileOps(r, &failOpenOps{MemFileOps: mem, handle: h})
		return r
	}
	base := func(r *Registry) capabilities.FileHandle {
		h, _ := capabilities.NewMem().Open("x", capabilities.OpenOpts{Read: true, Write: true, Create: true})
		return h
	}
	// A read failure forwards from every read shape: sequential-to-EOF
	// (io.ReadAll), sequential+length (readSeqFull), positioned+length
	// (ReadAt), and positioned-to-EOF (readAtToEnd).
	r := newReg(failCapHandle{FileHandle: base(nil), failRead: true})
	f := openFileVal(t, r, "f", func(*OrderedMap) {})
	for _, opt := range []func(*OrderedMap){
		func(*OrderedMap) {},
		func(om *OrderedMap) { om.Set("length", NewInteger(2)) },
		func(om *OrderedMap) { om.Set("offset", NewInteger(0)); om.Set("length", NewInteger(2)) },
		func(om *OrderedMap) { om.Set("offset", NewInteger(0)) },
	} {
		if err := runBoruError(t, r, []Value{NewWord("read"), f, wrapMap(opt)}); err == nil {
			t.Error("a handle read failure should forward")
		}
	}
	// A write failure forwards from IO.write f and from a {exclusive} write.
	r = newReg(failCapHandle{FileHandle: base(nil), failWrite: true})
	f = openFileVal(t, r, "f", func(om *OrderedMap) { om.Set("mode", NewString("write")) })
	if err := runBoruError(t, r, []Value{NewWord("write"), f, NewString("x")}); err == nil {
		t.Error("a handle write failure should forward")
	}
	if err := runBoruError(t, r, []Value{NewWord("write"), pathV("new"), NewString("x"),
		wrapMap(func(om *OrderedMap) { om.Set("exclusive", NewBoolean(true)) })}); err == nil {
		t.Error("an exclusive write's write failure should forward")
	}
	// A sync failure forwards from IO.flush f.
	r = newReg(failCapHandle{FileHandle: base(nil), failSync: true})
	f = openFileVal(t, r, "f", func(*OrderedMap) {})
	if err := runBoruError(t, r, []Value{NewWord("flush"), f}); err == nil {
		t.Error("a handle sync failure should forward")
	}
	// close of a File whose underlying Close fails, and of a Watcher whose
	// stop fails, both surface close_error (built directly).
	fh := &FileHandleInfo{ID: "F_x", Path: "p", h: failCapHandle{FileHandle: base(nil), failClose: true}}
	if _, err := doCloseWord([]Value{NewValueRaw(TAny, ExtensionPayload{Body: fh})}, r); err == nil {
		t.Error("a File close failure should surface")
	}
	wi := &WatcherInfo{ID: "W_x", Path: "p", stop: func() error { return errors.New("stop boom") }}
	if _, err := doCloseWord([]Value{NewValueRaw(TAny, ExtensionPayload{Body: wi})}, r); err == nil {
		t.Error("a Watcher stop failure should surface")
	}
}

func TestMountHandleInternals(t *testing.T) {
	rw := HostFileOps(mountFixture(t, `def files (flex {})  mount {
	  read: (p:Pathon => [files get `+"`${p}`"+`])
	  write: ([p:Pathon d:Any] => [files set `+"`${p}`"+` d drop])
	}`))
	if err := rw.WriteFile("f", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, _ := rw.Open("f", capabilities.OpenOpts{Read: true, Write: true})
	// Read to EOF, then a further Read is EOF.
	all := make([]byte, 5)
	_, _ = h.Read(all)
	if _, err := h.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("mount read at EOF = %v", err)
	}
	// ReadAt with a buffer larger than the remainder → short read + EOF.
	big := make([]byte, 10)
	if n, err := h.ReadAt(big, 2); n != 3 || err != io.EOF {
		t.Errorf("mount readat partial = %d %v", n, err)
	}
	// Seek current and end.
	if off, _ := h.Seek(-2, io.SeekCurrent); off != 3 {
		t.Errorf("mount seek current = %d", off)
	}
	if off, _ := h.Seek(0, io.SeekEnd); off != 5 {
		t.Errorf("mount seek end = %d", off)
	}
	_ = h.Close()
	// An append-mode handle writes at end regardless of the cursor.
	ah, _ := rw.Open("f", capabilities.OpenOpts{Write: true, Append: true})
	_, _ = ah.Seek(0, io.SeekStart)
	_, _ = ah.Write([]byte("!"))
	_ = ah.Close()
	if b, _ := rw.ReadFile("f"); string(b) != "hello!" {
		t.Errorf("mount append = %q", b)
	}

	// A write-less mount whose read reports the file ABSENT: creating a new
	// file must materialise it, which fails cleanly (no write handler).
	absent := HostFileOps(mountFixture(t, `mount { read: (p:Pathon => [none]) }`))
	if _, err := absent.Open("x", capabilities.OpenOpts{Write: true, Create: true}); err == nil {
		t.Error("create of a NEW file over a write-less mount should error")
	}
	// A write-less mount whose read reports the file PRESENT: opening it for
	// create-write (no truncate) must NOT rewrite it — the open succeeds,
	// proving no phantom write handler fires at open time. A flush (the
	// caller's actual write) then fails cleanly.
	ro := HostFileOps(mountFixture(t, `mount { read: (p:Pathon => ["seed"]) }`))
	if _, err := ro.Open("x", capabilities.OpenOpts{Create: true, Write: true}); err != nil {
		t.Errorf("create-write open of an existing file must not rewrite it: %v", err)
	}
	wh, err := ro.Open("x", capabilities.OpenOpts{Read: true, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = wh.Write([]byte("z"))
	if err := wh.Sync(); err == nil {
		t.Error("flush over a write-less mount should error")
	}
	// A positioned WriteAt on an append handle is declined, like os.File.
	apnd, _ := rw.Open("f", capabilities.OpenOpts{Write: true, Append: true})
	if _, err := apnd.WriteAt([]byte("x"), 0); err == nil {
		t.Error("WriteAt on an append mount handle should decline")
	}
	_ = apnd.Close()
}

func TestFileHandleTypeFormat(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ft := MintFileHandleType(r)
	if NewFileHandleType() == nil {
		t.Error("NewFileHandleType returned nil")
	}
	// A live handle renders File(id,path); the non-handle arm renders
	// File(nil).
	info := &FileHandleInfo{ID: "F_x", Path: "p.txt"}
	live := NewValueRaw(ft, ExtensionPayload{Body: info})
	if got := live.String(); !strings.Contains(got, "File(F_x,p.txt)") {
		t.Errorf("live format = %q", got)
	}
	if got := (fileHandleFormatBehavior{}).Format(NewInteger(1)); got != "File(nil)" {
		t.Errorf("non-handle format = %q", got)
	}
	// Match / Equal delegate to the default behavior.
	if !(fileHandleFormatBehavior{}).Match(NewTypeLiteral(ft), ft) {
		t.Error("Match should accept a File literal against File")
	}
	if !(fileHandleFormatBehavior{}).Equal(live, live) {
		t.Error("Equal should hold reflexively")
	}
}
