package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The portable link contract deliberately avoids Unix-only chmod bits.
// Run against both backends on each OS, including Windows absolute paths.
func TestPortableHardLink(t *testing.T) {
	for _, backend := range []string{"os", "mem"} {
		t.Run(backend, func(t *testing.T) {
			var ops FileOps = &OSFileOps{}
			if backend == "mem" {
				ops = NewMem()
			}
			root := t.TempDir()
			p := func(name string) string { return filepath.Join(root, name) }
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(ops.WriteFile(p("original"), []byte("one"), 0o644))
			must(ops.Link(p("original"), p("alias")))
			must(ops.WriteFile(p("alias"), []byte("two"), 0o644))
			data, err := ops.ReadFile(p("original"))
			must(err)
			if string(data) != "two" {
				t.Fatalf("hard link did not share content: %q", data)
			}
			must(ops.Remove(p("original"), false))
			data, err = ops.ReadFile(p("alias"))
			must(err)
			if string(data) != "two" {
				t.Fatalf("removing original lost alias content: %q", data)
			}

			for _, target := range []string{"alias", "missing"} {
				sym, linked := p("sym-"+target), p("linked-"+target)
				must(ops.Symlink(target, sym))
				must(ops.Link(sym, linked))
				fi, err := ops.Stat(linked, false)
				must(err)
				if !fi.Symlink || fi.Target != target {
					t.Fatalf("hard link followed symlink: %+v", fi)
				}
				if backend == "os" {
					a, err := os.Lstat(sym)
					must(err)
					b, err := os.Lstat(linked)
					must(err)
					if !os.SameFile(a, b) {
						t.Fatal("symlink was copied instead of hard-linked")
					}
				}
			}

			for _, tc := range []struct {
				source, dest string
				want         error
			}{
				{"alias", "alias", os.ErrExist},
				{"missing", "new", os.ErrNotExist},
			} {
				err := ops.Link(p(tc.source), p(tc.dest))
				if !errors.Is(err, tc.want) {
					t.Errorf("Link(%q, %q) = %v, want %v", tc.source, tc.dest, err, tc.want)
				}
				if backend == "os" {
					var linkErr *os.LinkError
					if !errors.As(err, &linkErr) || linkErr.Old != p(tc.source) || linkErr.New != p(tc.dest) {
						t.Errorf("Link must retain its paths in *os.LinkError: %v", err)
					}
				}
			}
			must(ops.MkdirAll(p("dir"), 0o755))
			if err := ops.Link(p("dir"), p("dir-link")); err == nil {
				t.Fatal("hard-linking a directory must fail")
			}
		})
	}
}
