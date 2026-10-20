package native

import (
	"runtime"
	"testing"
)

// isFilePath decides whether an import string names a file or a bare module
// searched for under .boru/. An absolute path must always be a file: on
// Windows that includes a volume path (`C:/x/m.boru`), which used to be
// searched for as a bare module named after the whole path
// ("searched .boru/C:/x/m.boru/ …").
func TestIsFilePathClassifiesImports(t *testing.T) {
	files := []string{"/x/m.boru", "./m.boru", "../m.boru"}
	bare := []string{"barem", "boru:io", "a/b", "m.boru", ".boru"}
	if runtime.GOOS == "windows" {
		files = append(files, `C:/x/m.boru`, `C:\x\m.boru`, `\x\m.boru`, `\\srv\share\m.boru`)
	} else {
		// Off Windows a drive letter is just a name.
		bare = append(bare, `C:/x/m.boru`)
	}
	for _, p := range files {
		if !isFilePath(p) {
			t.Errorf("isFilePath(%q) = false, want true (a file import)", p)
		}
	}
	for _, p := range bare {
		if isFilePath(p) {
			t.Errorf("isFilePath(%q) = true, want false (a bare module)", p)
		}
	}
}
