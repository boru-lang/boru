package native

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_temp_test.go — P3 word-layer coverage: IO.temp / IO.space handlers,
// the {atomic:true} write path (happy + every failure arm), and the
// permissioned / notinstalled / mount postures for the new operations.

func TestTempWordForms(t *testing.T) {
	r, mem := ioFSReg(t)

	// {} default: a file in the (implicit) temp root, mode 0600.
	res := runBoru(t, r, []Value{NewWord("temp"), wrapMap(func(*OrderedMap) {})})
	if !IsPathon(res[0]) {
		t.Fatalf("temp returned %v", res[0])
	}
	fi, err := mem.Stat(extractPath(res[0]), false)
	if err != nil || fi.IsDir || fi.Mode.Perm() != 0o600 {
		t.Errorf("temp file stat = %+v (%v)", fi, err)
	}

	// {dir:true} yields a 0700 directory.
	res = runBoru(t, r, []Value{NewWord("temp"), wrapMap(func(om *OrderedMap) { om.Set("dir", NewBoolean(true)) })})
	if fi, _ := mem.Stat(extractPath(res[0]), false); !fi.IsDir || fi.Mode.Perm() != 0o700 {
		t.Errorf("temp dir stat = %+v", fi)
	}

	// {in prefix suffix} shape the name and the location.
	if err := mem.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	res = runBoru(t, r, []Value{NewWord("temp"), wrapMap(func(om *OrderedMap) {
		om.Set("in", pathV("work"))
		om.Set("prefix", NewString("log-"))
		om.Set("suffix", NewString(".txt"))
	})})
	p := extractPath(res[0])
	if !strings.HasPrefix(p, "work/log-") || !strings.HasSuffix(p, ".txt") {
		t.Errorf("shaped temp = %q", p)
	}

	// An absent {in} directory declines (os.CreateTemp fidelity).
	if err := runBoruError(t, r, []Value{NewWord("temp"), wrapMap(func(om *OrderedMap) {
		om.Set("in", pathV("ghost"))
	})}); err == nil {
		t.Error("temp in an absent dir should error")
	}
}

func TestSpaceWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f", []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := runBoru(t, r, []Value{NewWord("space"), pathV("f")})
	m, err := AsMap(res[0])
	if err != nil {
		t.Fatal(err)
	}
	if typ, _ := m.Get("type"); typ.String() != "'mem'" {
		t.Errorf("space type = %v", typ)
	}
	total, _ := m.Get("total")
	free, _ := m.Get("free")
	if total.String() != fmt.Sprintf("%d", 1<<30) || free.String() != fmt.Sprintf("%d", (1<<30)-5) {
		t.Errorf("space accounting = total %v free %v", total, free)
	}
	if _, ok := m.Get("bsize"); !ok {
		t.Error("space record missing bsize")
	}
	// Absent path declines.
	if err := runBoruError(t, r, []Value{NewWord("space"), pathV("ghost")}); err == nil {
		t.Error("space on absent path should error")
	}
}

func TestAtomicWrite(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("d/t.txt", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Happy path: content replaced, and no .boru-atomic temp remains.
	runBoru(t, r, []Value{
		NewWord("write"), pathV("d/t.txt"), NewString("new"),
		wrapMap(func(om *OrderedMap) { om.Set("atomic", NewBoolean(true)) }),
	})
	if b, _ := mem.ReadFile("d/t.txt"); string(b) != "new" {
		t.Errorf("atomic write content = %q", b)
	}
	infos, _ := mem.ReadDir("d")
	for _, fi := range infos {
		if strings.Contains(fi.Name, ".boru-atomic") {
			t.Errorf("stranded atomic temp: %s", fi.Name)
		}
	}
	// Atomic works for a Bytes payload (non-offset binary write).
	runBoru(t, r, []Value{
		NewWord("write"), pathV("d/b.bin"), NewBytesValue([]byte{1, 2}),
		wrapMap(func(om *OrderedMap) { om.Set("atomic", NewBoolean(true)) }),
	})
	if b, _ := mem.ReadFile("d/b.bin"); len(b) != 2 {
		t.Errorf("atomic bytes = %v", b)
	}
	// {atomic} + {offset} is a contradiction and declines.
	if err := runBoruError(t, r, []Value{
		NewWord("write"), pathV("d/b.bin"), NewBytesValue([]byte{9}),
		wrapMap(func(om *OrderedMap) {
			om.Set("atomic", NewBoolean(true))
			om.Set("offset", NewInteger(0))
		}),
	}); err == nil {
		t.Error("atomic+offset should decline")
	}
	// Append + atomic merges then replaces in one rename.
	runBoru(t, r, []Value{
		NewWord("write"), pathV("d/t.txt"), NewString("+more"),
		wrapMap(func(om *OrderedMap) {
			om.Set("atomic", NewBoolean(true))
			om.Set("mode", NewString("append"))
		}),
	})
	if b, _ := mem.ReadFile("d/t.txt"); string(b) != "new+more" {
		t.Errorf("atomic append = %q", b)
	}
}

func TestAtomicWriteFailureArms(t *testing.T) {
	newReg := func(f *failAtOps) (*Registry, *capabilities.MemFileOps) {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		registerIOWords(r)
		mem := capabilities.NewMem()
		if err := mem.MkdirAll("d", 0o755); err != nil {
			t.Fatal(err)
		}
		f.MemFileOps = mem
		SetHostFileOps(r, f)
		return r, mem
	}
	atomicWrite := func(r *Registry) error {
		return runBoruError(t, r, []Value{
			NewWord("write"), pathV("d/t.txt"), NewString("x"),
			wrapMap(func(om *OrderedMap) { om.Set("atomic", NewBoolean(true)) }),
		})
	}

	// TempFile compile failure (a minimal mount-like backend) is a clean error.
	r, _ := newReg(&failAtOps{failTempFile: "d"})
	if err := atomicWrite(r); err == nil || !strings.Contains(err.Error(), "atomic") {
		t.Errorf("temp-fail arm: %v", err)
	}
	// A write failure into the temp removes it (no stranding).
	r, mem := newReg(&failAtOps{failWriteFile: ".boru-atomic"})
	if err := atomicWrite(r); err == nil {
		t.Error("temp-write-fail arm should error")
	}
	for p := range mem.Files {
		if strings.Contains(p, ".boru-atomic") {
			t.Errorf("stranded temp after write failure: %s", p)
		}
	}
	// A rename failure (mount without rename) removes the temp too.
	r, mem = newReg(&failAtOps{failRename: "d/t.txt"})
	if err := atomicWrite(r); err == nil {
		t.Error("rename-fail arm should error")
	}
	for p := range mem.Files {
		if strings.Contains(p, ".boru-atomic") {
			t.Errorf("stranded temp after rename failure: %s", p)
		}
	}
}

func TestNotInstalledTempStatfs(t *testing.T) {
	var ops capabilities.FileOps = notInstalledFileOps{}
	if _, err := ops.TempFile("", "x-*"); err == nil {
		t.Error("temp-file stub should error")
	}
	if _, err := ops.TempDir("", "x-*"); err == nil {
		t.Error("temp-dir stub should error")
	}
	if _, err := ops.Statfs("x"); err == nil {
		t.Error("statfs stub should error")
	}
}

func TestPermissionedTempStatfs(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	var d *policy.Denied
	if _, err := ops.TempFile("/tmp", "x-*"); !errors.As(err, &d) {
		t.Errorf("temp-file gate: %v", err)
	}
	if _, err := ops.TempDir("/tmp", "x-*"); !errors.As(err, &d) {
		t.Errorf("temp-dir gate: %v", err)
	}
	if _, err := ops.Statfs("/tmp"); !errors.As(err, &d) {
		t.Errorf("statfs gate: %v", err)
	}
	// Trusted passes through to the inner backend.
	mem := capabilities.NewMem()
	wrapped := NewPermissionedFileOps(mem, loadPolicy(t, "trusted"))
	if _, err := wrapped.TempFile("", "x-*"); err != nil {
		t.Errorf("trusted temp: %v", err)
	}
	if _, err := wrapped.TempDir("", "x-*"); err != nil {
		t.Errorf("trusted tempdir: %v", err)
	}
	if _, err := wrapped.Statfs("."); err != nil {
		t.Errorf("trusted statfs: %v", err)
	}
}

func TestMountTempStatfsBridge(t *testing.T) {
	// statfs handler: map, none, and mis-shape; temp handler present.
	rich := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  statfs: (p:Pathon => [{total:1000 free:600 available:500 bsize:512 type:"borufs"}])
	  temp: (fn [[d:Pathon pat:String] [Pathon] [(make Pathon "mounted-temp")]
	             [d:Pathon pat:String isdir:Boolean] [Pathon] [(make Pathon "mounted-temp")]])
	}`))
	fs, err := rich.Statfs("x")
	if err != nil || fs.Type != "borufs" || fs.TotalBytes != 1000 || fs.AvailBytes != 500 || fs.BlockSize != 512 {
		t.Errorf("bridged statfs = %+v (%v)", fs, err)
	}
	if p, err := rich.TempFile("", "x-*"); err != nil || p != "mounted-temp" {
		t.Errorf("bridged temp = %q (%v)", p, err)
	}
	if p, err := rich.TempDir("", "x-*"); err != nil || p != "mounted-temp" {
		t.Errorf("bridged tempdir = %q (%v)", p, err)
	}

	noneFS := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  statfs: (p:Pathon => [none])
	}`))
	if _, err := noneFS.Statfs("x"); err == nil {
		t.Error("none statfs should decline")
	}
	badFS := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  statfs: (p:Pathon => [7])
	}`))
	if _, err := badFS.Statfs("x"); err == nil || !strings.Contains(err.Error(), "want a map or none") {
		t.Errorf("mis-shaped statfs: %v", err)
	}

	// WITHOUT a temp handler the emulation routes through write/mkdir —
	// and declines cleanly when the map lacks even those.
	emu := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  write: ([p:Pathon d:Any] => [p])
	  mkdir: (p:Pathon => [p])
	}`))
	if p, err := emu.TempFile("/z", "e-*"); err != nil || !strings.HasPrefix(p, "/z/e-") {
		t.Errorf("emulated temp = %q (%v)", p, err)
	}
	if p, err := emu.TempDir("/z", "ed-*"); err != nil || !strings.HasPrefix(p, "/z/ed-") {
		t.Errorf("emulated tempdir = %q (%v)", p, err)
	}
	bare := HostFileOps(mountFixture(t, `mount { read: (p:Pathon => [none]) }`))
	if _, err := bare.TempFile("", "x-*"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("write-less emulation: %v", err)
	}
	if _, err := bare.TempDir("", "x-*"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("mkdir-less emulation: %v", err)
	}
	// Statfs with no handler declines.
	if _, err := bare.Statfs("x"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("handlerless statfs: %v", err)
	}

	// A temp handler that RAISES surfaces the error from both TempFile and
	// TempDir (the a.call error arm, not the emulation fallback).
	raising := HostFileOps(mountFixture(t, `mount {
	  read: (p:Pathon => [none])
	  temp: (fn [[d:Pathon pat:String] [Pathon] [raise "temp boom"]
	             [d:Pathon pat:String isdir:Boolean] [Pathon] [raise "temp boom"]])
	}`))
	if _, err := raising.TempFile("", "x-*"); err == nil {
		t.Error("a raising temp handler should error from TempFile")
	}
	if _, err := raising.TempDir("", "x-*"); err == nil {
		t.Error("a raising temp handler should error from TempDir")
	}
}
