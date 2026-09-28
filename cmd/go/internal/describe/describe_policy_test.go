package describe

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NUR079: describing a FILE module runs its body to learn its exports, so
// the documented environment policy governs it as it governs a run. An
// unresolvable BORU_POLICY is an error rather than a silently ungated run,
// and a policy that refuses the module's import refuses the description.
func TestDescribeHonoursEnvironmentPolicy(t *testing.T) {
	t.Setenv("BORU_POLICY", "no-such-profile")
	var out bytes.Buffer
	if code := Run([]string{"add"}, &out); code != 1 {
		t.Errorf("an unresolvable environment policy must fail describe, exit %d: %s", code, out.String())
	}

	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.boru")
	if err := os.WriteFile(lib, []byte("def twice fn [[n:Integer] [Integer] [n mul 2]]\nexport \"Lib\" {twice: twice/v}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse := `{version:1 name:"nofile" extends:"trusted" scopes:{modules:{words:{rules:[{deny:["import"] where:{kind:["file"]}}]}}}}`
	t.Setenv("BORU_POLICY", refuse)
	out.Reset()
	if code := Run([]string{lib + ":twice"}, &out); code == 0 {
		t.Errorf("a policy refusing file modules must refuse describing one, got:\n%s", out.String())
	}

	// Negative: with no policy the same module describes.
	t.Setenv("BORU_POLICY", "")
	out.Reset()
	if code := Run([]string{lib + ":twice"}, &out); code != 0 || !strings.Contains(out.String(), "twice") {
		t.Errorf("with no policy the module describes, exit %d: %s", code, out.String())
	}
}
