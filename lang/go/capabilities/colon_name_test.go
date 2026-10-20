package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A first path element ending in ':' is a NAME, not a drive: `mem://a.txt`
// lives in a directory `mem:` that a temp file — an atomic write's staging —
// can be created in, and that lists the file. On Windows filepath.Join read
// the ':' as a drive and the model stored the file as `mem:a.txt` beside an
// unrecorded `mem:` directory, so the staging temp found no parent
// (joinPath).
func TestMemColonNamedElementIsADirectory(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("mem://a.txt", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir("mem://a.txt")
	tmp, err := m.TempFile(dir, ".stage-*")
	if err != nil {
		t.Fatalf("TempFile(%q): %v", dir, err)
	}
	want, _ := m.ResolvePath(dir)
	if got := filepath.Dir(tmp); got != want {
		t.Errorf("temp %q is in %q, want the file's own directory %q", tmp, got, want)
	}
	if err := m.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename(tmp, "mem://a.txt"); err != nil {
		t.Fatal(err)
	}
	if got, err := m.ReadFile("mem://a.txt"); err != nil || string(got) != "new" {
		t.Errorf("after the rename the file holds %q (%v), want new", got, err)
	}
	infos, err := m.ReadDir(dir)
	if err != nil || len(infos) != 1 || infos[0].Name != "a.txt" {
		t.Errorf("ReadDir(%q) = %+v (%v), want just a.txt", dir, infos, err)
	}

	// A colon-named directory nothing created does not exist.
	if _, err := m.TempFile("nodir:", ".stage-*"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("TempFile in an absent colon-named dir: err = %v, want not-exist", err)
	}
}

func TestJoinPathKeepsTheSeparatorAfterAColonName(t *testing.T) {
	sep := string(filepath.Separator)
	if got, want := joinPath("mem:", "a.txt"), "mem:"+sep+"a.txt"; got != want {
		t.Errorf("joinPath(mem:, a.txt) = %q, want %q", got, want)
	}
	if got, want := joinPath("d", "a.txt"), filepath.Join("d", "a.txt"); got != want {
		t.Errorf("joinPath(d, a.txt) = %q, want filepath.Join's %q", got, want)
	}
	if runtime.GOOS == "windows" {
		// A real drive keeps Join's drive-relative reading.
		if got := joinPath("C:", "f"); got != "C:f" {
			t.Errorf("joinPath(C:, f) = %q, want the drive-relative C:f", got)
		}
	}
}
