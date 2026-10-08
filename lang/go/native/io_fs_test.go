package native

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// ioFSReg builds a registry with the io words under bare names and an
// in-memory FS, returning the registry and the MemFileOps for setup/assert.
func ioFSReg(t *testing.T) (*Registry, *capabilities.MemFileOps) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := setupMemFS(t, r)
	return r, mem
}

func statField(t *testing.T, r *Registry, path string) ReadMap {
	t.Helper()
	res := runBoru(t, r, []Value{NewWord("stat"), pathV(path)})
	if len(res) != 1 {
		t.Fatalf("stat %q: expected 1 result, got %d", path, len(res))
	}
	m, err := AsMap(res[0])
	if err != nil || m == nil {
		t.Fatalf("stat %q: not a record: %v (%v)", path, res[0], err)
	}
	return m
}

func TestStatFileAndDir(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("d/a.txt", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	m := statField(t, r, "d/a.txt")
	if name, _ := m.Get("name"); name.String() != "'a.txt'" {
		t.Errorf("name = %v", name)
	}
	if size, _ := m.Get("size"); size.String() != "5" {
		t.Errorf("size = %v", size)
	}
	typ, _ := m.Get("type")
	if a, _ := typ.AsConcreteAtom(); a != "file" {
		t.Errorf("type = %v, want file atom", typ)
	}
	if mode, _ := m.Get("mode"); mode.String() != "420" { // 0644
		t.Errorf("mode = %v, want 420", mode)
	}

	dm := statField(t, r, "d")
	dtyp, _ := dm.Get("type")
	if a, _ := dtyp.AsConcreteAtom(); a != "dir" {
		t.Errorf("dir type = %v, want dir atom", dtyp)
	}
}

func TestStatAbsentReturnsNone(t *testing.T) {
	r, _ := ioFSReg(t)
	res := runBoru(t, r, []Value{NewWord("stat"), pathV("ghost")})
	if len(res) != 1 || !res[0].Is(TNone) {
		t.Fatalf("stat of absent path: expected none, got %v", res)
	}
}

func TestStatSymlinkAndFollow(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("t.txt", []byte("yo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mem.Symlink("t.txt", "link"); err != nil {
		t.Fatal(err)
	}

	// Default follow=true dereferences: type is the target's (file).
	fm := statField(t, r, "link")
	ftyp, _ := fm.Get("type")
	if a, _ := ftyp.AsConcreteAtom(); a != "file" {
		t.Errorf("followed symlink type = %v, want file", ftyp)
	}

	// {follow:false} describes the link itself and exposes .target.
	res := runBoru(t, r, []Value{
		NewWord("stat"), pathV("link"),
		wrapMap(func(om *OrderedMap) { om.Set("follow", NewBoolean(false)) }),
	})
	lm, err := AsMap(res[0])
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	ltyp, _ := lm.Get("type")
	if a, _ := ltyp.AsConcreteAtom(); a != "symlink" {
		t.Errorf("lstat type = %v, want symlink", ltyp)
	}
	if tgt, _ := lm.Get("target"); tgt.String() != "'t.txt'" {
		t.Errorf("lstat target = %v", tgt)
	}
}

func TestStatResolveOption(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("a.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runBoru(t, r, []Value{
		NewWord("stat"), pathV("a.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("resolve", NewBoolean(true)) }),
	})
	m, err := AsMap(res[0])
	if err != nil {
		t.Fatal(err)
	}
	// mem ResolvePath cleans a relative path to itself; the field is present.
	if p, ok := m.Get("path"); !ok || p.String() != "'a.txt'" {
		t.Errorf("resolved path = %v", p)
	}
}

func TestStatCycleErrors(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.Symlink("cyc", "cyc"); err != nil { // self-referential
		t.Fatal(err)
	}
	if err := runBoruError(t, r, []Value{NewWord("stat"), pathV("cyc")}); err == nil {
		t.Error("expected stat error on a symlink cycle")
	}
}

func TestStatPathonTarget(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("p.txt", []byte("z"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runBoru(t, r, []Value{NewWord("stat"), NewPathon([]string{"p.txt"}, false)})
	if _, err := AsMap(res[0]); err != nil {
		t.Fatalf("stat of Pathon target: %v", err)
	}
}

func TestListNamesDetailRecursive(t *testing.T) {
	r, mem := ioFSReg(t)
	for _, p := range []string{"root/a.txt", "root/b.txt", "root/sub/c.txt"} {
		if err := mem.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Shallow names.
	res := runBoru(t, r, []Value{NewWord("list"), pathV("root")})
	lst, err := AsList(res[0])
	if err != nil {
		t.Fatal(err)
	}
	if lst.Len() != 3 { // a.txt, b.txt, sub
		t.Errorf("shallow list len = %d, want 3", lst.Len())
	}

	// Detail records.
	res = runBoru(t, r, []Value{
		NewWord("list"), pathV("root"),
		wrapMap(func(om *OrderedMap) { om.Set("detail", NewBoolean(true)) }),
	})
	dlst, _ := AsList(res[0])
	if _, err := AsMap(dlst.Get(0)); err != nil {
		t.Errorf("detail entry not a record: %v", err)
	}

	// Recursive walk.
	res = runBoru(t, r, []Value{
		NewWord("list"), pathV("root"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(true)) }),
	})
	rlst, _ := AsList(res[0])
	if rlst.Len() != 4 { // a.txt, b.txt, sub, sub/c.txt
		t.Errorf("recursive list len = %d, want 4", rlst.Len())
	}
}

func TestListMatchFilter(t *testing.T) {
	r, mem := ioFSReg(t)
	for _, p := range []string{"m/a.txt", "m/b.log", "m/sub/c.txt", "m/[ab].txt"} {
		if err := mem.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	names := func(v Value) []string {
		lst, _ := AsList(v)
		out := make([]string, 0, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			s, _ := AsString(lst.Get(i))
			out = append(out, s)
		}
		return out
	}

	// Shallow `*` filter: only direct .txt children ( `*` stops at `/` ).
	res := runBoru(t, r, []Value{
		NewWord("list"), pathV("m"),
		wrapMap(func(om *OrderedMap) { om.Set("match", NewString("*.txt")) }),
	})
	got := names(res[0])
	if len(got) != 2 || got[0] != "[ab].txt" || got[1] != "a.txt" {
		t.Errorf("shallow match = %v, want [[ab].txt a.txt]", got)
	}

	// Recursive `**` spans components.
	res = runBoru(t, r, []Value{
		NewWord("list"), pathV("m"),
		wrapMap(func(om *OrderedMap) {
			om.Set("recursive", NewBoolean(true))
			om.Set("match", NewString("**.txt"))
		}),
	})
	if got := names(res[0]); len(got) != 3 {
		t.Errorf("recursive ** match = %v, want 3 entries incl sub/c.txt", got)
	}

	// Character classes are NOT special: `[ab].txt` matches only the
	// literal name, never a.txt / b.txt.
	res = runBoru(t, r, []Value{
		NewWord("list"), pathV("m"),
		wrapMap(func(om *OrderedMap) { om.Set("match", NewString("[ab].txt")) }),
	})
	if got := names(res[0]); len(got) != 1 || got[0] != "[ab].txt" {
		t.Errorf("char-class-literal match = %v, want exactly [[ab].txt]", got)
	}

	// A match that filters everything yields an empty list, not an error.
	res = runBoru(t, r, []Value{
		NewWord("list"), pathV("m"),
		wrapMap(func(om *OrderedMap) { om.Set("match", NewString("*.nope")) }),
	})
	if got := names(res[0]); len(got) != 0 {
		t.Errorf("no-hit match = %v, want empty", got)
	}
}

func TestCopyMoveOverwriteOption(t *testing.T) {
	r, mem := ioFSReg(t)
	seed := func(p, body string) {
		t.Helper()
		if err := mem.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	seed("src.txt", "new")
	seed("dst.txt", "old")

	// {overwrite:false} declines an existing destination for copy and move.
	if err := runBoruError(t, r, []Value{
		NewWord("copy"), pathV("src.txt"), pathV("dst.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("overwrite", NewBoolean(false)) }),
	}); err == nil {
		t.Error("copy {overwrite:false} onto existing dst should error")
	}
	if err := runBoruError(t, r, []Value{
		NewWord("move"), pathV("src.txt"), pathV("dst.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("overwrite", NewBoolean(false)) }),
	}); err == nil {
		t.Error("move {overwrite:false} onto existing dst should error")
	}
	if body, _ := mem.ReadFile("dst.txt"); string(body) != "old" {
		t.Errorf("declined overwrite still mutated dst: %q", body)
	}

	// An absent destination is fine under {overwrite:false}.
	runBoru(t, r, []Value{
		NewWord("copy"), pathV("src.txt"), pathV("fresh.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("overwrite", NewBoolean(false)) }),
	})
	if body, _ := mem.ReadFile("fresh.txt"); string(body) != "new" {
		t.Errorf("copy to absent dst = %q", body)
	}
	runBoru(t, r, []Value{
		NewWord("move"), pathV("fresh.txt"), pathV("moved.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("overwrite", NewBoolean(false)) }),
	})
	if _, err := mem.Stat("moved.txt", false); err != nil {
		t.Error("move to absent dst did not happen")
	}

	// Explicit {overwrite:true} (and the default) still replaces.
	runBoru(t, r, []Value{
		NewWord("copy"), pathV("src.txt"), pathV("dst.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("overwrite", NewBoolean(true)) }),
	})
	if body, _ := mem.ReadFile("dst.txt"); string(body) != "new" {
		t.Errorf("explicit overwrite copy = %q, want new", body)
	}
	// Move opts form with default overwrite replaces too.
	seed("again.txt", "v2")
	runBoru(t, r, []Value{
		NewWord("move"), pathV("again.txt"), pathV("dst.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(false)) }),
	})
	if body, _ := mem.ReadFile("dst.txt"); string(body) != "v2" {
		t.Errorf("default-overwrite move = %q, want v2", body)
	}
}

func TestListErrors(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runBoruError(t, r, []Value{NewWord("list"), pathV("ghost")}); err == nil {
		t.Error("expected list error on nonexistent dir")
	}
	if err := runBoruError(t, r, []Value{NewWord("list"), pathV("f.txt")}); err == nil {
		t.Error("expected list error on a file")
	}
}

// ── pure helpers ─────────────────────────────────────────────────────────

func TestFileTypeName(t *testing.T) {
	cases := []struct {
		fi   capabilities.FileInfo
		want string
	}{
		{capabilities.FileInfo{Symlink: true}, "symlink"},
		{capabilities.FileInfo{IsDir: true}, "dir"},
		{capabilities.FileInfo{Mode: 0}, "file"},                 // regular
		{capabilities.FileInfo{Mode: os.ModeNamedPipe}, "other"}, // neither
		{capabilities.FileInfo{Mode: os.ModeDevice}, "other"},    // neither
	}
	for _, c := range cases {
		if got := fileTypeName(c.fi); got != c.want {
			t.Errorf("fileTypeName(%+v) = %q, want %q", c.fi, got, c.want)
		}
	}
}

func TestMtimeUnix(t *testing.T) {
	if got := mtimeUnix(time.Time{}); got != 0 {
		t.Errorf("mtimeUnix(zero) = %d, want 0", got)
	}
	ts := time.Unix(12345, 0)
	if got := mtimeUnix(ts); got != 12345 {
		t.Errorf("mtimeUnix(%v) = %d, want 12345", ts, got)
	}
}

func TestIsFileTypeAtom(t *testing.T) {
	ft := NewFileType()
	if !isFileTypeAtom(newFileTypeAtom("file", ft)) {
		t.Error("file atom should be a FileType member")
	}
	if isFileTypeAtom(NewAtom("nope")) {
		t.Error("unrelated atom is not a FileType member")
	}
	if isFileTypeAtom(NewString("file")) {
		t.Error("the string \"file\" is not a FileType member")
	}
}

func TestMapBoolOptNonMap(t *testing.T) {
	// A type-literal (non-concrete) opts value falls back to the default.
	if !mapBoolOpt(NewTypeLiteral(TMap), "follow", true) {
		t.Error("type-literal opts should yield the true default")
	}
	if mapBoolOpt(NewTypeLiteral(TMap), "detail", false) {
		t.Error("type-literal opts should yield the false default")
	}
}

// wrapMap builds a concrete Map value from a setter, for opts arguments.
func wrapMap(set func(*OrderedMap)) Value {
	om := NewOrderedMap()
	set(om)
	return NewMap(om)
}

// pathV builds a Pathon micron from a POSIX path string, mirroring the
// engine's `make Pathon "s"` parse (segments split on "/", absolute iff it
// begins with "/"). The io words are Pathon-only, so tests pass every file
// target through this rather than a bare string.
func pathV(s string) Value {
	abs := strings.HasPrefix(s, "/")
	var parts []string
	for _, p := range strings.Split(s, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return NewPathon(parts, abs)
}

// TestMapBoolOptNonOrderedMap covers the AsMap-nil path: an Options-typed
// value is a concrete TMap whose AsMap returns nil, so mapBoolOpt falls back
// to the default rather than dereferencing a nil map.
func TestMapBoolOptNonOrderedMap(t *testing.T) {
	opts := NewOptionsType(NewOrderedMap())
	if !mapBoolOpt(opts, "follow", true) {
		t.Error("options-typed opts should yield the true default")
	}
	if mapBoolOpt(opts, "recursive", false) {
		t.Error("options-typed opts should yield the false default")
	}
}

// walkFailOps lists a single subdirectory at the root but fails to list that
// subdirectory, so a recursive walk hits the recursion-error propagation.
type walkFailOps struct{ *capabilities.MemFileOps }

func (w walkFailOps) ReadDir(path string) ([]capabilities.FileInfo, error) {
	if strings.Contains(path, "sub") {
		return nil, fmt.Errorf("readdir boom")
	}
	return []capabilities.FileInfo{{Name: "sub", IsDir: true}}, nil
}

func TestCollectEntriesRecursionError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	SetHostFileOps(r, walkFailOps{capabilities.NewMem()})
	if _, err := collectEntries(r, "root", "", true); err == nil {
		t.Error("expected the recursive-walk error to propagate")
	}
}

// ── remove / move / copy ─────────────────────────────────────────────────

func TestRemoveWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// remove a file → returns the path, file gone.
	res := runBoru(t, r, []Value{NewWord("remove"), pathV("f.txt")})
	if !IsPathon(res[0]) || res[0].String() != "f.txt" {
		t.Errorf("remove returned %v", res[0])
	}
	if _, err := mem.Stat("f.txt", false); err == nil {
		t.Error("file not removed")
	}
	// removing an absent path errors without {force}.
	if err := runBoruError(t, r, []Value{NewWord("remove"), pathV("f.txt")}); err == nil {
		t.Error("expected error removing an absent path")
	}
	// {force:true} makes an absent removal a no-op.
	res = runBoru(t, r, []Value{
		NewWord("remove"), pathV("ghost"),
		wrapMap(func(om *OrderedMap) { om.Set("force", NewBoolean(true)) }),
	})
	if !IsPathon(res[0]) || res[0].String() != "ghost" {
		t.Errorf("forced remove returned %v", res[0])
	}
	// removing a non-empty dir needs {recursive}.
	if err := mem.WriteFile("d/c.txt", []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runBoruError(t, r, []Value{NewWord("remove"), pathV("d")}); err == nil {
		t.Error("expected error removing a non-empty dir non-recursively")
	}
	runBoru(t, r, []Value{
		NewWord("remove"), pathV("d"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(true)) }),
	})
	if _, err := mem.Stat("d/c.txt", false); err == nil {
		t.Error("tree not removed")
	}
}

func TestMoveWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("a.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runBoru(t, r, []Value{NewWord("move"), pathV("a.txt"), pathV("b.txt")})
	if !IsPathon(res[0]) || res[0].String() != "b.txt" {
		t.Errorf("move returned %v", res[0])
	}
	if _, err := mem.Stat("b.txt", false); err != nil {
		t.Error("move destination missing")
	}
	if _, err := mem.Stat("a.txt", false); err == nil {
		t.Error("move source remains")
	}
	if err := runBoruError(t, r, []Value{NewWord("move"), pathV("ghost"), pathV("z")}); err == nil {
		t.Error("expected error moving an absent path")
	}
	// A Pathon destination is returned as a Pathon.
	if err := mem.WriteFile("c.txt", []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	res = runBoru(t, r, []Value{NewWord("move"), pathV("c.txt"), NewPathon([]string{"e.txt"}, false)})
	if !IsPathon(res[0]) {
		t.Errorf("move to Pathon returned %v (want Pathon)", res[0])
	}
}

func TestCopyWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("src.txt", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// copy a file.
	res := runBoru(t, r, []Value{NewWord("copy"), pathV("src.txt"), pathV("dst.txt")})
	if !IsPathon(res[0]) || res[0].String() != "dst.txt" {
		t.Errorf("copy returned %v", res[0])
	}
	if b, err := mem.ReadFile("dst.txt"); err != nil || string(b) != "hello" {
		t.Errorf("copy content = %q (%v)", b, err)
	}
	// copy a symlink recreates it.
	if err := mem.Symlink("src.txt", "link"); err != nil {
		t.Fatal(err)
	}
	runBoru(t, r, []Value{NewWord("copy"), pathV("link"), pathV("link2")})
	if li, err := mem.Stat("link2", false); err != nil || !li.Symlink || li.Target != "src.txt" {
		t.Errorf("copied symlink = %+v (%v)", li, err)
	}
	// copying a symlink onto an EXISTING destination REPLACES it (overwrite
	// parity with the file case — a bare Symlink would EEXIST): PR-review fix.
	if err := mem.WriteFile("occupied.txt", []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	runBoru(t, r, []Value{NewWord("copy"), pathV("link"), pathV("occupied.txt")})
	if li, err := mem.Stat("occupied.txt", false); err != nil || !li.Symlink || li.Target != "src.txt" {
		t.Errorf("symlink copy did not overwrite = %+v (%v)", li, err)
	}
	// Overwriting a NON-EMPTY directory with a symlink still declines (the
	// Remove fails), the same as writing a file over a directory.
	if err := mem.WriteFile("busydir/keep.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runBoruError(t, r, []Value{NewWord("copy"), pathV("link"), pathV("busydir")}); err == nil {
		t.Error("symlink copy over a non-empty dir should decline")
	}
	// copying a directory needs {recursive}.
	if err := mem.WriteFile("tree/a.txt", []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteFile("tree/sub/b.txt", []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runBoruError(t, r, []Value{NewWord("copy"), pathV("tree"), pathV("treecopy")}); err == nil {
		t.Error("expected error copying a dir without {recursive}")
	}
	runBoru(t, r, []Value{
		NewWord("copy"), pathV("tree"), pathV("treecopy"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(true)) }),
	})
	if b, err := mem.ReadFile("treecopy/sub/b.txt"); err != nil || string(b) != "2" {
		t.Errorf("recursive copy child = %q (%v)", b, err)
	}
	// copying an absent source errors.
	if err := runBoruError(t, r, []Value{NewWord("copy"), pathV("ghost"), pathV("z")}); err == nil {
		t.Error("expected error copying an absent path")
	}
}

// failAtOps wraps a MemFileOps and fails a chosen operation when its path
// contains the configured substring, so doCopy's and applyTouch's error
// branches are reachable.
type failAtOps struct {
	*capabilities.MemFileOps
	failReadFile, failWriteFile, failMkdir, failReadDir, failSymlink string
	failStat, failChmod, failChtimes, failTruncate                   string
	failXattrGet, failXattrList, failXattrSet                        string
	failTempFile, failRename                                         string
}

func (f *failAtOps) TempFile(dir, pattern string) (string, error) {
	if f.failTempFile != "" && strings.Contains(dir, f.failTempFile) {
		return "", fmt.Errorf("tempfile boom")
	}
	return f.MemFileOps.TempFile(dir, pattern)
}

func (f *failAtOps) Rename(oldPath, newPath string) error {
	if f.failRename != "" && strings.Contains(newPath, f.failRename) {
		return fmt.Errorf("rename boom")
	}
	return f.MemFileOps.Rename(oldPath, newPath)
}

func (f *failAtOps) XattrList(p string) ([]string, error) {
	if f.failXattrList != "" && strings.Contains(p, f.failXattrList) {
		return nil, fmt.Errorf("xattr-list boom")
	}
	return f.MemFileOps.XattrList(p)
}

func (f *failAtOps) XattrSet(p, name string, v []byte) error {
	if f.failXattrSet != "" && strings.Contains(p, f.failXattrSet) {
		return fmt.Errorf("xattr-set boom")
	}
	return f.MemFileOps.XattrSet(p, name, v)
}

func (f *failAtOps) XattrGet(p, name string) ([]byte, error) {
	if f.failXattrGet != "" && strings.Contains(p, f.failXattrGet) {
		return nil, fmt.Errorf("xattr-get boom")
	}
	return f.MemFileOps.XattrGet(p, name)
}

func (f *failAtOps) Stat(p string, follow bool) (capabilities.FileInfo, error) {
	if f.failStat != "" && strings.Contains(p, f.failStat) {
		return capabilities.FileInfo{}, fmt.Errorf("stat boom")
	}
	return f.MemFileOps.Stat(p, follow)
}
func (f *failAtOps) Chmod(p string, m os.FileMode) error {
	if f.failChmod != "" && strings.Contains(p, f.failChmod) {
		return fmt.Errorf("chmod boom")
	}
	return f.MemFileOps.Chmod(p, m)
}
func (f *failAtOps) Chtimes(p string, a, mt time.Time) error {
	if f.failChtimes != "" && strings.Contains(p, f.failChtimes) {
		return fmt.Errorf("chtimes boom")
	}
	return f.MemFileOps.Chtimes(p, a, mt)
}
func (f *failAtOps) Truncate(p string, size int64) error {
	if f.failTruncate != "" && strings.Contains(p, f.failTruncate) {
		return fmt.Errorf("truncate boom")
	}
	return f.MemFileOps.Truncate(p, size)
}

func (f *failAtOps) ReadFile(p string) ([]byte, error) {
	if f.failReadFile != "" && strings.Contains(p, f.failReadFile) {
		return nil, fmt.Errorf("readfile boom")
	}
	return f.MemFileOps.ReadFile(p)
}
func (f *failAtOps) WriteFile(p string, d []byte, m os.FileMode) error {
	if f.failWriteFile != "" && strings.Contains(p, f.failWriteFile) {
		return fmt.Errorf("writefile boom")
	}
	return f.MemFileOps.WriteFile(p, d, m)
}
func (f *failAtOps) MkdirAll(p string, m os.FileMode) error {
	if f.failMkdir != "" && strings.Contains(p, f.failMkdir) {
		return fmt.Errorf("mkdir boom")
	}
	return f.MemFileOps.MkdirAll(p, m)
}
func (f *failAtOps) ReadDir(p string) ([]capabilities.FileInfo, error) {
	if f.failReadDir != "" && strings.Contains(p, f.failReadDir) {
		return nil, fmt.Errorf("readdir boom")
	}
	return f.MemFileOps.ReadDir(p)
}
func (f *failAtOps) Symlink(target, link string) error {
	if f.failSymlink != "" && strings.Contains(link, f.failSymlink) {
		return fmt.Errorf("symlink boom")
	}
	return f.MemFileOps.Symlink(target, link)
}

func TestCopyErrorBranches(t *testing.T) {
	newReg := func(setup func(*capabilities.MemFileOps), f *failAtOps) *Registry {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		mem := capabilities.NewMem()
		setup(mem)
		f.MemFileOps = mem
		SetHostFileOps(r, f)
		return r
	}

	// Stat error: copying an absent source.
	r := newReg(func(*capabilities.MemFileOps) {}, &failAtOps{})
	if err := doCopy(r, "ghost", "dst", false); err == nil {
		t.Error("expected Stat error copying absent source")
	}
	// ReadFile error on a file copy.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("s.txt", []byte("x"), 0644) },
		&failAtOps{failReadFile: "s.txt"})
	if err := doCopy(r, "s.txt", "d.txt", false); err == nil {
		t.Error("expected ReadFile error")
	}
	// WriteFile error on a file copy.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("s.txt", []byte("x"), 0644) },
		&failAtOps{failWriteFile: "d.txt"})
	if err := doCopy(r, "s.txt", "d.txt", false); err == nil {
		t.Error("expected WriteFile error")
	}
	// MkdirAll error in copyTree.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("dir/a.txt", []byte("x"), 0644) },
		&failAtOps{failMkdir: "out"})
	if err := doCopy(r, "dir", "out", true); err == nil {
		t.Error("expected MkdirAll error")
	}
	// ReadDir error in copyTree.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("dir/a.txt", []byte("x"), 0644) },
		&failAtOps{failReadDir: "dir"})
	if err := doCopy(r, "dir", "out", true); err == nil {
		t.Error("expected ReadDir error")
	}
	// Symlink error copying a symlink.
	r = newReg(func(m *capabilities.MemFileOps) { m.Symlink("t", "sl") },
		&failAtOps{failSymlink: "d"})
	if err := doCopy(r, "sl", "d", false); err == nil {
		t.Error("expected Symlink error")
	}
	// Recursion error: a child copy fails inside copyTree.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("dir/a.txt", []byte("x"), 0644) },
		&failAtOps{failReadFile: "a.txt"})
	if err := doCopy(r, "dir", "out", true); err == nil {
		t.Error("expected child-copy error to propagate")
	}
}

// ── link / touch ─────────────────────────────────────────────────────────

func TestLinkWord(t *testing.T) {
	r, mem := ioFSReg(t)
	// symbolic link (default).
	res := runBoru(t, r, []Value{NewWord("link"), pathV("t.txt"), pathV("sl")})
	if !IsPathon(res[0]) || res[0].String() != "sl" {
		t.Errorf("link returned %v", res[0])
	}
	if li, err := mem.Stat("sl", false); err != nil || !li.Symlink || li.Target != "t.txt" {
		t.Errorf("symlink = %+v (%v)", li, err)
	}
	// linking over an existing path errors.
	if err := runBoruError(t, r, []Value{NewWord("link"), pathV("t.txt"), pathV("sl")}); err == nil {
		t.Error("expected error creating a link over an existing path")
	}
	// hard link.
	if err := mem.WriteFile("f.txt", []byte("body"), 0644); err != nil {
		t.Fatal(err)
	}
	runBoru(t, r, []Value{
		NewWord("link"), pathV("f.txt"), pathV("hl"),
		wrapMap(func(om *OrderedMap) { om.Set("hard", NewBoolean(true)) }),
	})
	if b, err := mem.ReadFile("hl"); err != nil || string(b) != "body" {
		t.Errorf("hard link content = %q (%v)", b, err)
	}
	// hard-linking an absent target errors.
	if err := runBoruError(t, r, []Value{
		NewWord("link"), pathV("ghost"), pathV("hl2"),
		wrapMap(func(om *OrderedMap) { om.Set("hard", NewBoolean(true)) }),
	}); err == nil {
		t.Error("expected error hard-linking an absent target")
	}
	// A Pathon destination is returned as a Pathon.
	res = runBoru(t, r, []Value{NewWord("link"), pathV("t.txt"), NewPathon([]string{"pl"}, false)})
	if !IsPathon(res[0]) {
		t.Errorf("link to Pathon returned %v (want Pathon)", res[0])
	}
}

func TestTouchWord(t *testing.T) {
	r, mem := ioFSReg(t)
	// touch creates an empty file when absent.
	res := runBoru(t, r, []Value{NewWord("touch"), pathV("new.txt")})
	if !IsPathon(res[0]) || res[0].String() != "new.txt" {
		t.Errorf("touch returned %v", res[0])
	}
	if fi, err := mem.Stat("new.txt", false); err != nil || fi.Size != 0 {
		t.Errorf("touched file = %+v (%v)", fi, err)
	}
	// touch on an existing file leaves its content.
	if err := mem.WriteFile("keep.txt", []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	runBoru(t, r, []Value{NewWord("touch"), pathV("keep.txt")})
	if b, _ := mem.ReadFile("keep.txt"); string(b) != "data" {
		t.Errorf("touch clobbered content: %q", b)
	}
	// {mode} chmods, {mtime} sets the time, {size} truncates/grows.
	runBoru(t, r, []Value{
		NewWord("touch"), pathV("new.txt"),
		wrapMap(func(om *OrderedMap) {
			om.Set("mode", NewInteger(0o600))
			om.Set("mtime", NewInteger(1000))
			om.Set("size", NewInteger(3))
		}),
	})
	fi, _ := mem.Stat("new.txt", false)
	if fi.Mode.Perm() != 0o600 || fi.Size != 3 || fi.ModTime.Unix() != 1000 {
		t.Errorf("touch metadata = %+v", fi)
	}
	// a negative size is rejected.
	if err := runBoruError(t, r, []Value{
		NewWord("touch"), pathV("new.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("size", NewInteger(-1)) }),
	}); err == nil {
		t.Error("expected error on a negative touch size")
	}
	// {atime} alone still drives Chtimes.
	runBoru(t, r, []Value{
		NewWord("touch"), pathV("new.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("atime", NewInteger(2000)) }),
	})
}

func TestTouchErrorBranches(t *testing.T) {
	mkOpts := func(set func(*OrderedMap)) Value { return wrapMap(set) }

	// Stat error (non-not-exist) short-circuits.
	f := &failAtOps{MemFileOps: capabilities.NewMem(), failStat: "p"}
	if err := applyTouch(f, "p.txt", NewTypeLiteral(TMap), false); err == nil {
		t.Error("expected Stat error")
	}
	// WriteFile error when creating an absent file.
	f = &failAtOps{MemFileOps: capabilities.NewMem(), failWriteFile: "p"}
	if err := applyTouch(f, "p.txt", NewTypeLiteral(TMap), false); err == nil {
		t.Error("expected WriteFile-create error")
	}
	// Chmod error.
	mem := capabilities.NewMem()
	mem.WriteFile("p.txt", []byte("x"), 0644)
	f = &failAtOps{MemFileOps: mem, failChmod: "p"}
	if err := applyTouch(f, "p.txt", mkOpts(func(om *OrderedMap) { om.Set("mode", NewInteger(0o600)) }), true); err == nil {
		t.Error("expected Chmod error")
	}
	// Chtimes error.
	mem = capabilities.NewMem()
	mem.WriteFile("p.txt", []byte("x"), 0644)
	f = &failAtOps{MemFileOps: mem, failChtimes: "p"}
	if err := applyTouch(f, "p.txt", mkOpts(func(om *OrderedMap) { om.Set("mtime", NewInteger(1)) }), true); err == nil {
		t.Error("expected Chtimes error")
	}
	// Truncate error.
	mem = capabilities.NewMem()
	mem.WriteFile("p.txt", []byte("x"), 0644)
	f = &failAtOps{MemFileOps: mem, failTruncate: "p"}
	if err := applyTouch(f, "p.txt", mkOpts(func(om *OrderedMap) { om.Set("size", NewInteger(1)) }), true); err == nil {
		t.Error("expected Truncate error")
	}
}

func TestMapIntOpt(t *testing.T) {
	if _, ok := mapIntOpt(NewTypeLiteral(TMap), "mode"); ok {
		t.Error("type-literal opts should report absent")
	}
	if _, ok := mapIntOpt(NewOptionsType(NewOrderedMap()), "mode"); ok {
		t.Error("options-typed opts should report absent")
	}
	opts := wrapMap(func(om *OrderedMap) { om.Set("mode", NewInteger(7)) })
	if v, ok := mapIntOpt(opts, "mode"); !ok || v != 7 {
		t.Errorf("mapIntOpt = %d %v, want 7 true", v, ok)
	}
}

// TestHardLinkSharesStorage pins the inode contract THROUGH the boru:io
// words on the mem FS: after `link src dst {hard:true}`, writing via
// either name is visible via the other, and removing the original name
// keeps the content alive under the link — matching the real filesystem
// (the divergence the strict-fidelity mem rework closed).
func TestHardLinkSharesStorage(t *testing.T) {
	r, mem := ioFSReg(t)
	runBoru(t, r, []Value{NewWord("write"), pathV("t.txt"), NewString("v1")})
	runBoru(t, r, []Value{
		NewWord("link"), pathV("t.txt"), pathV("hl"),
		wrapMap(func(om *OrderedMap) { om.Set("hard", NewBoolean(true)) }),
	})
	// Mutate via the ORIGINAL; the link sees it.
	runBoru(t, r, []Value{NewWord("write"), pathV("t.txt"), NewString("v2")})
	res := runBoru(t, r, []Value{NewWord("read"), pathV("hl")})
	if res[0].String() != "'v2'" {
		t.Errorf("hard link stale after original write: %v", res[0])
	}
	// Mutate via the LINK; the original sees it.
	runBoru(t, r, []Value{NewWord("write"), pathV("hl"), NewString("v3")})
	res = runBoru(t, r, []Value{NewWord("read"), pathV("t.txt")})
	if res[0].String() != "'v3'" {
		t.Errorf("original stale after link write: %v", res[0])
	}
	// Remove the original; the content survives under the link.
	runBoru(t, r, []Value{NewWord("remove"), pathV("t.txt")})
	res = runBoru(t, r, []Value{NewWord("read"), pathV("hl")})
	if res[0].String() != "'v3'" {
		t.Errorf("content died with the original name: %v", res[0])
	}
	if _, err := mem.Stat("t.txt", false); err == nil {
		t.Error("original name still present after remove")
	}
}
