package capabilities

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableWindowsRoots(t *testing.T) {
	for _, root := range []string{`C:\`, `\\server\share\`} {
		t.Run(root, func(t *testing.T) {
			if !isMemRoot(root) || isMemRoot(filepath.Join(root, "sub")) {
				t.Fatal("root classification must distinguish a volume from its children")
			}
			m := NewMem()
			path := filepath.Join(root, "sub", "file")
			if err := m.WriteFile(path, []byte("body"), 0o644); err != nil {
				t.Fatal(err)
			}
			if m.Dirs[root] {
				t.Fatal("volume roots must stay implicit")
			}
			link := filepath.Join(root, "link")
			if err := m.Symlink("sub", link); err != nil {
				t.Fatal(err)
			}
			data, err := m.ReadFile(filepath.Join(link, "file"))
			if err != nil || string(data) != "body" {
				t.Fatalf("read through directory link = %q, %v", data, err)
			}
			if _, err := m.ReadFile(filepath.Join(link, "missing")); !os.IsNotExist(err) {
				t.Fatalf("absent file must fail: %v", err)
			}
		})
	}
}
