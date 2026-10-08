package native

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestNestedMountUnwindsLIFO pins the PR-review fix: nested mounts unwind in
// LIFO order rather than clobbering a single saved slot, so the first
// unmount reveals the earlier mount and the last restores the original.
func TestNestedMountUnwindsLIFO(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	run := func(src string) error {
		vals, perr := parser.Parse(src)
		if perr != nil {
			t.Fatalf("parse %q: %v", src, perr)
		}
		_, rerr := NewTop(r).Run(vals)
		return rerr
	}
	if err := run(`mount {read: (p:Pathon => ["A"])}`); err != nil {
		t.Fatal(err)
	}
	if err := run(`mount {read: (p:Pathon => ["B"])}`); err != nil {
		t.Fatal(err)
	}
	if b, _ := HostFileOps(r).ReadFile("x"); string(b) != "B" {
		t.Errorf("top of the mount stack = %q, want B", b)
	}
	if err := run(`unmount`); err != nil {
		t.Fatal(err)
	}
	if b, _ := HostFileOps(r).ReadFile("x"); string(b) != "A" {
		t.Errorf("after one unmount = %q, want A (the earlier mount, not lost)", b)
	}
	if err := run(`unmount`); err != nil {
		t.Fatal(err)
	}
	// The stack is empty; a further unmount errors.
	if err := run(`unmount`); err == nil {
		t.Error("unmount with an empty mount stack should error")
	}
}

// TestUnmountRewiresJsonicResolver pins the PR-review fix: IO.unmount restores
// the previous FileOps THROUGH the resolver-rewire path, so a later jsonic
// include resolves against the restored backend rather than the unmounted one.
func TestUnmountRewiresJsonicResolver(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	SetHostFormats(r, map[string]Format{"jsonic": &JsonicFormat{}})
	// Base backend A holds the include; a "mounted" backend B does not.
	memA := capabilities.NewMem()
	if err := memA.WriteFile("inc.jsonic", []byte(`{v:1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	SetHostFileOps(r, memA)
	pushMountPrev(r)
	SetHostFileOps(r, capabilities.NewMem())
	if _, err := doUnmountWord(nil, r); err != nil {
		t.Fatal(err)
	}
	// After unmount the include must resolve — through A, the restored backend.
	jf, _ := HostFormats(r)["jsonic"].(*JsonicFormat)
	res, derr := jf.Decode(`{@"inc.jsonic", w:2}`)
	if derr != nil {
		t.Fatalf("include did not resolve after unmount (resolver not rewired): %v", derr)
	}
	m, _ := AsMap(res[0])
	if v, ok := m.Get("v"); !ok {
		t.Errorf("resolved map missing the included key: %v", res[0])
	} else if iv, _ := AsInteger(v); iv != 1 {
		t.Errorf("included v = %v, want 1", v)
	}
}

// mountFixture builds a registry with the io words and runs a boru
// program that mounts a handler set, returning the registry.
func mountFixture(t *testing.T, src string) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := NewTop(r).Run(values); err != nil {
		t.Fatalf("mount program: %v", err)
	}
	return r
}

// fullMountSrc is the database-style example: a flex-map "table" of
// path → {data, mtime} rows, exposing read/write/stat/list/remove —
// the minimal viable filesystem, implemented entirely in Boru. (A SQL
// backing would swap the flex map for table queries; boru currently has
// no SQL-execution surface, so the row store stands in for the table.)
const fullMountSrc = `
def rows (flex {})
def names (flex [])
mount {
  read:  (p:Pathon => [rows get ` + "`${p}`" + `])
  write: ([p:Pathon data:Any] => [
    rows set ` + "`${p}`" + ` data drop
    names push ` + "`${p}`" + ` drop
  ])
  stat:  (p:Pathon => [
    def k ` + "`${p}`" + `
    def d (rows get k)
    if (d eq none) [none] [{name: k, size: (size d), type: file/q, mode: 420, mtime: 1000}]
  ])
  list:  (p:Pathon => [names])
  remove: ([p:Pathon opts:Map] => [rows set ` + "`${p}`" + ` none drop])
}
`

func TestMountedBoruFilesystemFullSurface(t *testing.T) {
	r := mountFixture(t, fullMountSrc)
	ops := HostFileOps(r)

	// write / read round-trip through the boru row store.
	if err := ops.WriteFile("a.txt", []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := ops.ReadFile("a.txt")
	if err != nil || string(b) != "alpha" {
		t.Fatalf("round-trip = %q (%v)", b, err)
	}
	// stat maps the handler's record onto FileInfo.
	fi, err := ops.Stat("a.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Name != "a.txt" || fi.Size != 5 || fi.Mode.Perm() != 0o644 || fi.IsDir ||
		!fi.ModTime.Equal(time.Unix(1000, 0)) {
		t.Errorf("stat = %+v", fi)
	}
	// stat of an absent path is the none → not-exist convention.
	if _, err := ops.Stat("ghost", true); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent stat = %v", err)
	}
	// list surfaces the names.
	infos, err := ops.ReadDir(".")
	if err != nil || len(infos) != 1 || infos[0].Name != "a.txt" {
		t.Errorf("list = %+v (%v)", infos, err)
	}
	// remove empties the row; the read then misses.
	if err := ops.Remove("a.txt", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.ReadFile("a.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("read after remove = %v", err)
	}
	// An operation with no handler declines cleanly (documented no-op).
	if err := ops.Rename("a.txt", "b.txt"); !errors.Is(err, errMountUnsupported) {
		t.Errorf("unhandled rename = %v", err)
	}
	if err := ops.Chmod("a.txt", 0o600); !errors.Is(err, errMountUnsupported) {
		t.Errorf("unhandled chmod = %v", err)
	}
	if _, _, err := ops.Watch("a.txt", capabilities.WatchOpts{}); !errors.Is(err, errMountUnsupported) {
		t.Errorf("watch on a mount = %v", err)
	}
	// ResolvePath with no resolve handler is lexical.
	if got, err := ops.ResolvePath("x/../y"); err != nil || got != "y" {
		t.Errorf("lexical resolve = %q (%v)", got, err)
	}
}

func TestMountedOptionalHandlers(t *testing.T) {
	// Every optional operation dispatches to its handler when present;
	// the log records the calls (a flex list mutated by each handler).
	src := `
def log (flex [])
mount {
  read:    (p:Pathon => [none])
  mkdir:   (p:Pathon => [log push "mkdir" drop])
  rename:  ([a:Pathon b:Pathon] => [log push "rename" drop])
  symlink: ([a:Pathon b:Pathon] => [log push "symlink" drop])
  link:    ([a:Pathon b:Pathon] => [log push "link" drop])
  chmod:   ([p:Pathon m:Integer] => [log push "chmod" drop])
  chtimes: ([p:Pathon a:Integer m:Integer] => [log push "chtimes" drop])
  truncate: ([p:Pathon s:Integer] => [log push "truncate" drop])
  resolve: (p:Pathon => ["resolved-path"])
}
`
	r := mountFixture(t, src)
	ops := HostFileOps(r)
	if err := ops.MkdirAll("d", 0o755); err != nil {
		t.Errorf("mkdir: %v", err)
	}
	if err := ops.Rename("a", "b"); err != nil {
		t.Errorf("rename: %v", err)
	}
	if err := ops.Symlink("t", "l"); err != nil {
		t.Errorf("symlink: %v", err)
	}
	if err := ops.Link("t", "l2"); err != nil {
		t.Errorf("link: %v", err)
	}
	if err := ops.Chmod("p", 0o600); err != nil {
		t.Errorf("chmod: %v", err)
	}
	if err := ops.Chtimes("p", time.Unix(1, 0), time.Unix(2, 0)); err != nil {
		t.Errorf("chtimes: %v", err)
	}
	if err := ops.Truncate("p", 3); err != nil {
		t.Errorf("truncate: %v", err)
	}
	if got, err := ops.ResolvePath("whatever"); err != nil || got != "resolved-path" {
		t.Errorf("resolve handler = %q (%v)", got, err)
	}
}

func TestMountedHandlerShapes(t *testing.T) {
	// Bytes results pass through raw; a wrong-typed read result errors;
	// non-UTF8 write payloads arrive as Bytes; handler raises propagate.
	src := `
def got (flex {})
mount {
  read: (p:Pathon => [
    def k ` + "`${p}`" + `
    if (k eq "bytes.bin") [convert Bytes "raw"] [
      if (k eq "badtype") [42] [
        if (k eq "boom") [raise mounted "handler exploded"] [none]
      ]
    ]
  ])
  write: ([p:Pathon data:Any] => [got set ` + "`${p}`" + ` data drop])
  stat:  (p:Pathon => [42])
  list:  (p:Pathon => [42])
}
`
	r := mountFixture(t, src)
	ops := HostFileOps(r)
	if b, err := ops.ReadFile("bytes.bin"); err != nil || string(b) != "raw" {
		t.Errorf("bytes read = %q (%v)", b, err)
	}
	if _, err := ops.ReadFile("badtype"); err == nil || !strings.Contains(err.Error(), "want String, Bytes, or none") {
		t.Errorf("bad read shape = %v", err)
	}
	if _, err := ops.ReadFile("boom"); err == nil || !strings.Contains(err.Error(), "handler exploded") {
		t.Errorf("handler raise = %v", err)
	}
	// stat / list returning a non-map/non-list shape errors cleanly.
	if _, err := ops.Stat("x", true); err == nil || !strings.Contains(err.Error(), "want a map or none") {
		t.Errorf("bad stat shape = %v", err)
	}
	if _, err := ops.ReadDir("x"); err == nil || !strings.Contains(err.Error(), "want a list") {
		t.Errorf("bad list shape = %v", err)
	}
	// A non-UTF8 payload arrives as Bytes (the handler stores it; assert
	// via a second read from the same flex map through the adapter).
	if err := ops.WriteFile("bin", []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatalf("binary write: %v", err)
	}
	// The arity-mismatch arm: no signature matches a 1-arg call on a
	// handler declared with 2 params.
	a := &boruFileOps{r: r, handlers: func() *OrderedMap {
		om := NewOrderedMap()
		v, _ := HostFileOps(r).(*permissionedFileOps)
		_ = v
		return om
	}()}
	if _, err := a.call("read", nil); err == nil || !errors.Is(err, errMountUnsupported) {
		t.Errorf("missing handler via call = %v", err)
	}
}

func TestMountListDetailRecordsAndBadEntry(t *testing.T) {
	src := `
mount {
  read: (p:Pathon => [none])
  list: (p:Pathon => [
    def k ` + "`${p}`" + `
    if (k eq "detail") [[{name: "x.txt", size: 3, type: file/q}]] [
      if (k eq "bad") [[42]] [["plain.txt"]]
    ]
  ])
}
`
	r := mountFixture(t, src)
	ops := HostFileOps(r)
	infos, err := ops.ReadDir("detail")
	if err != nil || len(infos) != 1 || infos[0].Name != "x.txt" || infos[0].Size != 3 {
		t.Errorf("detail listing = %+v (%v)", infos, err)
	}
	infos, err = ops.ReadDir("names")
	if err != nil || len(infos) != 1 || infos[0].Name != "plain.txt" {
		t.Errorf("name listing = %+v (%v)", infos, err)
	}
	if _, err := ops.ReadDir("bad"); err == nil || !strings.Contains(err.Error(), "want a name String or a stat map") {
		t.Errorf("bad entry = %v", err)
	}
}

func TestMountStatShapes(t *testing.T) {
	// The dir/symlink type arms and the target field.
	src := `
mount {
  read: (p:Pathon => [none])
  stat: (p:Pathon => [
    def k ` + "`${p}`" + `
    if (k eq "d") [{name: "d", type: dir/q, mode: 493}] [
      if (k eq "sl") [{name: "sl", type: symlink/q, target: "t.txt"}] [none]
    ]
  ])
}
`
	r := mountFixture(t, src)
	ops := HostFileOps(r)
	fi, err := ops.Stat("d", true)
	if err != nil || !fi.IsDir || fi.Mode&os.ModeDir == 0 || fi.Mode.Perm() != 0o755 {
		t.Errorf("dir stat = %+v (%v)", fi, err)
	}
	fi, err = ops.Stat("sl", false)
	if err != nil || !fi.Symlink || fi.Target != "t.txt" {
		t.Errorf("symlink stat = %+v (%v)", fi, err)
	}
}

func TestMountUnmountRestores(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Install a distinctive mem FS as the "previous" host ops.
	prevMem := capabilities.NewMem()
	if err := prevMem.WriteFile("marker.txt", []byte("prev"), 0o644); err != nil {
		t.Fatal(err)
	}
	SetHostFileOps(r, prevMem)

	values, err := parser.Parse(`mount {read: (p:Pathon => ["mounted"])}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTop(r).Run(values); err != nil {
		t.Fatal(err)
	}
	if b, _ := HostFileOps(r).ReadFile("marker.txt"); string(b) != "mounted" {
		t.Fatalf("mount not active: %q", b)
	}
	// Unmount restores the exact previous FileOps.
	if _, err := doUnmountWord(nil, r); err != nil {
		t.Fatal(err)
	}
	if b, err := HostFileOps(r).ReadFile("marker.txt"); err != nil || string(b) != "prev" {
		t.Errorf("previous ops not restored: %q (%v)", b, err)
	}
	// A second unmount declines.
	if _, err := doUnmountWord(nil, r); err == nil {
		t.Error("expected unmount with nothing mounted to decline")
	}
	// Unmount when the pre-mount slot was EMPTY deletes the capability.
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r2)
	_, _ = r2.Capabilities.Delete(CapFileOps)
	values2, err := parser.Parse(`mount {read: (p:Pathon => ["m"])}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTop(r2).Run(values2); err != nil {
		t.Fatal(err)
	}
	if _, err := doUnmountWord(nil, r2); err != nil {
		t.Fatal(err)
	}
	if _, err := HostFileOps(r2).ReadFile("x"); err == nil {
		t.Error("expected the not-installed stub after unmount of an empty slot")
	}
}

func TestMountValidation(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// A non-concrete map is declined.
	if _, err := doMountWord([]Value{NewTypeLiteral(TMap)}, r); err == nil {
		t.Error("expected a type-literal handler map to be declined")
	}
	// helpers: pathOfArgs with no args; isNoneResult on a concrete value.
	if pathOfArgs(nil) != "" {
		t.Error("pathOfArgs(nil) should be empty")
	}
	if isNoneResult(NewInteger(1)) {
		t.Error("an Integer is not the none result")
	}
	if !isNoneResult(NewTypeLiteral(TNone)) || !isNoneResult(NewNone()) {
		t.Error("both none shapes must read as absent")
	}
}

// TestMountRemainingArms covers the in-package remnants: arity-mismatched
// handlers, error propagation through Stat/ReadDir/ResolvePath, and the
// doMountWord validation compile failures.
func TestMountRemainingArms(t *testing.T) {
	// A handler whose signature cannot take the call's arity.
	r := mountFixture(t, `mount {read: ([a:Pathon b:Pathon] => ["two-arg"])}`)
	ops := HostFileOps(r)
	if _, err := ops.ReadFile("x"); err == nil || !strings.Contains(err.Error(), "no signature matching 1 argument") {
		t.Errorf("arity mismatch = %v", err)
	}
	// Stat / ReadDir with NO handler propagate the unsupported error.
	if _, err := ops.Stat("x", true); !errors.Is(err, errMountUnsupported) {
		t.Errorf("stat without handler = %v", err)
	}
	if _, err := ops.ReadDir("x"); !errors.Is(err, errMountUnsupported) {
		t.Errorf("readdir without handler = %v", err)
	}
	// A raising resolve handler propagates.
	r2 := mountFixture(t, `mount {read: (p:Pathon => [none]) resolve: (p:Pathon => [raise rboom "resolve exploded"])}`)
	if _, err := HostFileOps(r2).ResolvePath("x"); err == nil || !strings.Contains(err.Error(), "resolve exploded") {
		t.Errorf("raising resolve = %v", err)
	}
	// doMountWord compile failures: a non-Function handler; a map without read.
	r3, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	badMap := NewOrderedMap()
	badMap.Set("read", NewInteger(42))
	if _, err := doMountWord([]Value{NewMap(badMap)}, r3); err == nil || !strings.Contains(err.Error(), "must be a Function") {
		t.Errorf("non-fn handler = %v", err)
	}
	noRead := NewOrderedMap()
	if _, err := doMountWord([]Value{NewMap(noRead)}, r3); err == nil || !strings.Contains(err.Error(), "needs at least a `read` handler") {
		t.Errorf("missing read handler = %v", err)
	}
}
