// Package do implements `boru do <words...>` — join the remaining
// args with spaces, run as a boru expression, print the result.
package do

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/permsflags"
	"github.com/boru-lang/boru/cmd/go/internal/run"
	lang "github.com/boru-lang/boru/lang/go"
)

type cmd struct{}

// New returns the do subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "do" }
func (*cmd) Synopsis() string { return "evaluate args as a boru expression" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	// Separate permission flags from positional words. We accept
	// every --perms* / --allow / --deny / --no-install / --install
	// flag at the head of the argv, then the remainder forms the
	// expression. flag.FlagSet stops at the first non-flag token,
	// so this works as long as users put flags first.
	fs := flag.NewFlagSet("do", flag.ContinueOnError)
	fs.SetOutput(stderr)
	colorMode := fs.String("color", "auto", "diagnostic color: auto (terminal-only, honors NO_COLOR), always, never")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	source := strings.Join(fs.Args(), " ")
	if source == "" {
		fmt.Fprintf(stderr, "error: boru do requires an expression\n")
		return 1
	}

	pol, err := pf.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if err := run.EvalOptionsColor(stdout, source, run.OptionsFor("", 0, pol), lang.ResolveColor(nil, stderr, *colorMode)); err != nil {
		// `IO.exit N` sets this process's status directly — `boru do` is as
		// much a program driver as `boru run`, and an expression that asks
		// to exit must not be reported as a failure instead.
		if code, isExit := lang.ExitCode(err); isExit {
			return code
		}
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}
