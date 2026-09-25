// Package fmt implements `boru fmt [file.boru ...]` — format boru
// source files in place via lang/go/formatter.Format.
//
// With no arguments, formats every .boru file in the current
// directory tree (skipping anything inside .boru/).
package fmt

import (
	stdfmt "fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/lang/go/formatter"
)

// osWriteFile is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive Run's write-failure arm, which cannot be forced via file
// permissions when the suite runs as root.
var osWriteFile = os.WriteFile

type cmd struct{}

// New returns the fmt subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "fmt" }
func (*cmd) Synopsis() string { return "format .boru source files in place" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// formatByExt formats a file's contents according to its extension:
// Markdown (.md/.markdown) and HTML (.html/.htm) files have only their
// embedded boru (```boru fences / <!-- borufmt --> regions) reformatted;
// everything else is treated as a whole boru source file.
func formatByExt(path, src string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return formatter.FormatMarkdown(src)
	case ".html", ".htm":
		return formatter.FormatHTML(src)
	default:
		return formatter.Format(src)
	}
}

// printUsage is the contract `boru help fmt` points the reader at.
func printUsage(w io.Writer) {
	stdfmt.Fprintln(w, "usage: boru fmt [file ...]")
	stdfmt.Fprintln(w, "  Formats .boru source files in place. With no files it walks the current")
	stdfmt.Fprintln(w, "  directory tree (skipping .boru/); .md and .html files have only their")
	stdfmt.Fprintln(w, "  embedded boru reformatted.")
	stdfmt.Fprintln(w, "  -h, --help  print this usage and exit 0")
}

// Run handles `boru fmt [file.boru ...]`.
func Run(args []string, stdout, stderr io.Writer) int {
	// -h is a help request, not a file named "-h" and not an error: it
	// prints the usage and exits 0, as `check` does (NUR084).
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			printUsage(stdout)
			return 0
		}
	}
	var files []string
	if len(args) == 0 {
		err := pathutil.WalkSources(".", filepath.WalkDir, ".boru", func(path string) { files = append(files, path) })
		if err != nil {
			stdfmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	} else {
		// Expand a leading ~ the shell left verbatim (e.g. a quoted
		// "~/proj/x.boru") on each positional file path.
		files = make([]string, len(args))
		for i, a := range args {
			files[i] = pathutil.Expand(a)
		}
	}

	if len(files) == 0 {
		stdfmt.Fprintln(stdout, "no .boru files found")
		return 0
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			stdfmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		formatted := formatByExt(path, string(data))
		if string(data) != formatted {
			if err := osWriteFile(path, []byte(formatted), 0644); err != nil {
				stdfmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			stdfmt.Fprintln(stdout, path)
		}
	}
	return 0
}
