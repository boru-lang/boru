package fmt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestFmtByExtension covers formatByExt: Markdown and HTML files have
// only their embedded boru reformatted, while other extensions are treated
// as whole boru source files.
func TestFmtByExtension(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ name, content, want string }{
		{"doc.md", "```boru\ndef  x  1\n```\n", "```boru\ndef x 1\n```\n"},
		{"page.html", "<!-- borufmt -->\ndef  y  2\n<!-- /borufmt -->\n", "<!-- borufmt -->\ndef y 2\n<!-- /borufmt -->\n"},
		{"code.boru", "def   z   3\n", "def z 3\n"},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.name)
		if err := os.WriteFile(p, []byte(c.content), 0644); err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		if code := Run([]string{p}, &out, &errOut); code != 0 {
			t.Fatalf("%s: code=%d stderr=%q", c.name, code, errOut.String())
		}
		got, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if string(got) != c.want {
			t.Errorf("%s: got %q, want %q", c.name, string(got), c.want)
		}
	}
}

// TestFmtPositionalTildeExpanded guards that a leading ~ in a positional
// file path that the shell left verbatim (e.g. a quoted "~/x.boru")
// resolves under the home folder rather than being read as a literal
// "~/x.boru" relative to the working directory.
func TestFmtPositionalTildeExpanded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)        // os.UserHomeDir honors $HOME on Unix.
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	src := filepath.Join(home, "x.boru")
	if err := os.WriteFile(src, []byte("foo:1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"~/x.boru"}, &out, &errOut); code != 0 {
		t.Fatalf("fmt code=%d, stderr=%q", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr (a literal ~ would fail to open): %q", errOut.String())
	}
}
