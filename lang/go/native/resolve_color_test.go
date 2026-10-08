package native

import (
	"bytes"
	"os"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// resolve_color_test.go — the colour cascade, both halves of the NO_COLOR
// read. This test moved here with ResolveColor itself (it was
// eng/go/diag_render_test.go::TestResolveColor) and gained the arm the move
// exists for: a host-installed EnvOps governs the decision, so the renderer
// and `IO.env` can no longer disagree about what the environment is.

// The explicit modes and the auto cascade, all through the process fallback
// (nil registry — the CLI styling its own output before any instance exists).
func TestResolveColorProcessFallback(t *testing.T) {
	var buf bytes.Buffer
	if !ResolveColor(nil, &buf, "always") {
		t.Fatal("always must color any writer")
	}
	if ResolveColor(nil, &buf, "never") {
		t.Fatal("never must not color")
	}
	// auto: a non-file writer never colors.
	if ResolveColor(nil, &buf, "auto") {
		t.Fatal("auto must not color a non-file writer")
	}
	// auto: NO_COLOR wins over everything below it.
	t.Setenv("NO_COLOR", "1")
	if ResolveColor(nil, os.Stdout, "auto") {
		t.Fatal("NO_COLOR must disable auto coloring")
	}
	t.Setenv("NO_COLOR", "")
	// Restore: empty NO_COLOR means unset semantics in our check.
	os.Unsetenv("NO_COLOR")
	// auto: a regular file is not a terminal.
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if ResolveColor(nil, f, "auto") {
		t.Fatal("a regular file is not a terminal")
	}
	// auto: /dev/null is a character device and NOT a terminal, so it must
	// not colour. This block used to assert the opposite and t.Skip on
	// failure, which meant the fix to the probe turned the whole rest of this
	// test into a silent skip rather than a failure.
	if dev, derr := os.Open(os.DevNull); derr == nil {
		defer dev.Close()
		if ResolveColor(nil, dev, "auto") {
			t.Fatal("os.DevNull is not a terminal — auto must not colour it")
		}
	}
	// auto: a real terminal DOES colour, which is the arm that matters.
	terminal := openTestTerminal(t)
	if !ResolveColor(nil, terminal, "auto") {
		t.Fatal("auto must colour a real terminal")
	}

	// A closed file makes Stat fail — the defensive arm declines color.
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if ResolveColor(nil, closed, "auto") {
		t.Fatal("a stat-failing writer must not color")
	}
}

// A registry with no EnvOps installed still falls back to the process: the
// registry argument is where the environment is READ from, not a requirement
// that one be installed.
func TestResolveColorRegistryWithoutEnvOps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NO_COLOR", "1")
	if ResolveColor(r, os.Stdout, "auto") {
		t.Fatal("with no EnvOps installed the process NO_COLOR must still win")
	}
}

// The arm the move exists for: when the host installed an environment view,
// THAT view governs — both directions. A host that hides NO_COLOR hides it
// from the renderer, and a host that sets it in a hermetic environment
// disables colour even though the real process never had it.
func TestResolveColorUsesInstalledEnvOps(t *testing.T) {
	dev := openTestTerminal(t)

	// Sanity: the destination really is a terminal, so colour hinges on
	// NO_COLOR alone.
	if !ResolveColor(nil, dev, "always") || !isCharDevice(dev) {
		t.Fatal("an initialized terminal must read as a terminal")
	}

	// The host's view SETS NO_COLOR while the process does not: no colour.
	set, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("NO_COLOR")
	SetHostEnvOps(set, capabilities.MapEnvOps{Vars: map[string]string{"NO_COLOR": "1"}})
	if ResolveColor(set, dev, "auto") {
		t.Fatal("an installed EnvOps setting NO_COLOR must disable color")
	}

	// The host's view HIDES NO_COLOR while the process sets it: colour, and
	// the empty-string reading is "unset" here exactly as it is for the
	// process fallback.
	hidden, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NO_COLOR", "1")
	SetHostEnvOps(hidden, capabilities.MapEnvOps{Vars: map[string]string{"NO_COLOR": ""}})
	if !ResolveColor(hidden, dev, "auto") {
		t.Fatal("an installed EnvOps hiding NO_COLOR must not inherit the process value")
	}
}
