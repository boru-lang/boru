// Package describe implements `boru describe` — documentation for the
// boru *language*: its built-in words, their categories, and loadable
// modules. The forms are:
//
//	boru describe                     a categorised guide to words and modules
//	boru describe <word>              full per-word docs (e.g. boru describe add)
//	boru describe <category>          the words in one category (e.g. math)
//	boru describe boru:<module>        a module and the words it exports
//	boru describe boru:<module>:<word> one exported word of a module
//
// A name it doesn't recognise is given one more chance: describe tries to
// load it as a module (native, installed, or file) before giving up.
// CLI/command help lives separately under `boru help`.
package describe

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	helppkg "github.com/boru-lang/boru/lang/go/native/help"
	parse "github.com/boru-lang/boru/parser/go"
)

// The built-in module catalog (names + one-line summaries) lives in the help
// package — see help.ModuleCatalog / help.ModuleSummary — so the CLI here and
// the `describe` language word render the same module index.

// defaultRegistry is a test seam (design/TEST-SEAMS.10.md); tests swap it
// to drive newRegistry's construction-error arm, which no input can
// provoke.
var defaultRegistry = native.DefaultRegistry

// modulesResolve is a test seam (design/TEST-SEAMS.10.md); tests swap it
// to drive the module-resolution failure arms — the built-in modules all
// resolve successfully.
var modulesResolve = modules.Resolve

type cmd struct{}

// New returns the describe subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "describe" }
func (*cmd) Synopsis() string { return "document a boru word, category, or module (or list them)" }
func (*cmd) Run(args []string, _ io.Reader, stdout, _ io.Writer) int {
	return Run(args, stdout)
}

// Run dispatches `boru describe [name]`. Precedence for a bare name (no colon):
// category first (so generic names like `math`/`type`/`string` show their
// group), then word, then dotted export, then a built-in module, and finally
// a best-effort module load. A name containing ':' is always a module path.
func Run(args []string, w io.Writer) int {
	if len(args) == 0 {
		writeIndex(w)
		return 0
	}

	name := args[0]

	// A colon marks a module reference: "boru:type-util" or, with a word,
	// "boru:type-util:foo" / "type-util:foo". No built-in word carries a colon,
	// so this is unambiguous and takes precedence over every bare-name form.
	if strings.Contains(name, ":") {
		return describeModulePath(w, name)
	}

	// A category groups related words. Checked before words so that browsing
	// names (math, type, string, stack, …) open the group; the only words that
	// share a category name are `help` (use `boru help`) and the niche `stack`
	// dump word, both better served by the group view here.
	if cat, ok := helppkg.LookupCategory(name); ok {
		writeCategory(w, cat)
		return 0
	}

	return describeName(w, name)
}

// describeName resolves a bare name to a word, a dotted module export, a
// built-in module, or — failing all of those — a module it tries to load.
func describeName(w io.Writer, name string) int {
	reg, err := newRegistry()
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load registry: %s\n", err)
		return 1
	}

	// A registered word (or simple def) renders from live signature data.
	if info := native.BuildFuncInfo(reg, name); info != nil {
		fmt.Fprint(w, native.FormatWordHelp(reg, *info))
		return 0
	}

	// A dotted name (ArrayUtil.indices) names a single module export.
	if strings.Contains(name, ".") {
		if info := qualifiedExportInfo(reg, name); info != nil {
			fmt.Fprint(w, native.FormatWordHelp(reg, *info))
			return 0
		}
	}

	// A documented-but-unregistered word falls back to its static entry.
	if entry := helppkg.Lookup(name); entry != nil {
		fmt.Fprint(w, helppkg.Format(entry))
		return 0
	}

	// A bare built-in module name (math-util, struct-util, …).
	if isModule(name) {
		desc, derr := modulesResolve(name, reg)
		if derr != nil {
			fmt.Fprintf(w, "describe: cannot load module %q: %s\n", name, derr)
			return 1
		}
		return renderModuleDesc(w, "boru:"+name, desc)
	}

	// Unknown: give it one more chance as a loadable module (installed or file).
	if desc, lerr := native.ResolveAnyModule(reg, name); lerr == nil {
		return renderModuleDesc(w, name, desc)
	}

	fmt.Fprintf(w, "describe: no description available for %q\n", name)
	fmt.Fprintln(w, "Run 'boru describe' to list available words and modules.")
	return 1
}

// describeModulePath handles a name containing ':' — a module, or a single
// word within one. The module is resolved as a built-in when known, otherwise
// loaded on demand.
func describeModulePath(w io.Writer, name string) int {
	reg, err := newRegistry()
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load registry: %s\n", err)
		return 1
	}

	modRef, word := splitModulePath(name)
	desc, err := resolveModule(reg, modRef)
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load module %q: %s\n", modRef, err)
		fmt.Fprintln(w, "Run 'boru describe' to list available words and modules.")
		return 1
	}

	if word == "" {
		return renderModuleDesc(w, modRef, desc)
	}

	if info := exportInfoFromDesc(desc, word); info != nil {
		fmt.Fprint(w, native.FormatWordHelp(reg, *info))
		return 0
	}

	fmt.Fprintf(w, "describe: module %s has no exported word %q\n", modRef, word)
	if names := exportWordNames(desc); len(names) > 0 {
		fmt.Fprintf(w, "Exported words: %s\n", strings.Join(names, " "))
	}
	return 1
}

// splitModulePath separates a colon-form argument into a module reference and
// an optional word. "boru:type-util:foo" → ("boru:type-util", "foo");
// "boru:type-util" → ("boru:type-util", ""); "type-util:foo" → ("type-util",
// "foo"). The "boru:" prefix is preserved on the module reference so the
// resolver routes it as a native import.
func splitModulePath(name string) (modRef, word string) {
	hadBoru := strings.HasPrefix(name, "boru:")
	rest := strings.TrimPrefix(name, "boru:")
	short := rest
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		short, word = rest[:i], rest[i+1:]
	}
	if hadBoru {
		return "boru:" + short, word
	}
	return short, word
}

// resolveModule resolves a module reference to its descriptor, using the
// built-in native path when the module is compiled in and a best-effort load
// otherwise ("attempt to load a module if unknown").
func resolveModule(reg *native.Registry, modRef string) (native.ModuleDesc, error) {
	if short := strings.TrimPrefix(modRef, "boru:"); isModule(short) {
		return modulesResolve(short, reg)
	}
	return native.ResolveAnyModule(reg, modRef)
}

// writeIndex prints the categorised guide: every word grouped by category,
// then the loadable modules, then how to drill into each.
func writeIndex(w io.Writer) {
	fmt.Fprintln(w, "boru language reference — built-in words by category, and loadable modules.")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Words:")
	helppkg.WriteWordsByCategory(w)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Modules (load with import \"boru:<name>\"):")
	helppkg.WriteModuleCatalog(w)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Drill in:")
	fmt.Fprintln(w, "  boru describe <word>                e.g. boru describe add")
	fmt.Fprintln(w, "  boru describe <category>            e.g. boru describe math")
	fmt.Fprintln(w, "  boru describe boru:<module>          e.g. boru describe boru:type-util")
	fmt.Fprintln(w, "  boru describe boru:<module>:<word>   e.g. boru describe boru:type-util:tpartial")
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
}

// writeCategory prints one category and the words it contains (shared body),
// then a CLI-flavoured footer.
func writeCategory(w io.Writer, cat helppkg.Category) {
	helppkg.WriteCategory(w, cat)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use 'boru describe <word>' for full docs (signatures, examples) on any of these.")
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
}

// renderModuleDesc prints a module's summary (when built-in) and the words it
// exports, plus how to import and drill into them.
func renderModuleDesc(w io.Writer, ref string, desc native.ModuleDesc) int {
	if summary := helppkg.ModuleSummary(strings.TrimPrefix(ref, "boru:")); summary != "" {
		fmt.Fprintf(w, "%s — %s\n", ref, summary)
	} else {
		fmt.Fprintf(w, "%s\n", ref)
	}
	fmt.Fprintln(w)

	exportNames := make([]string, 0, len(desc.Exports))
	for k := range desc.Exports {
		exportNames = append(exportNames, k)
	}
	sort.Strings(exportNames)

	fmt.Fprintln(w, "Words (call as <export>.<word>):")
	for _, export := range exportNames {
		words := desc.Exports[export].Keys()
		sort.Strings(words)
		for _, word := range words {
			fmt.Fprintf(w, "  %s.%s\n", export, word)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Load with import \"%s\", then call e.g. <export>.<word>.\n", ref)
	fmt.Fprintf(w, "Describe one word with 'boru describe %s:<word>'.\n", ref)
	fmt.Fprintln(w, "Docs: "+helppkg.ReferenceURL)
	return 0
}

// exportInfoFromDesc resolves a single word across a module descriptor's export
// namespaces and builds its help.FuncInfo (doc + provenance + signatures).
// Returns nil when no namespace exports a function by that name. Namespaces are
// walked in sorted order for a deterministic result.
func exportInfoFromDesc(desc native.ModuleDesc, word string) *helppkg.FuncInfo {
	nss := make([]string, 0, len(desc.Exports))
	for ns := range desc.Exports {
		nss = append(nss, ns)
	}
	sort.Strings(nss)
	for _, ns := range nss {
		om := desc.Exports[ns]
		if om == nil {
			continue
		}
		v, ok := om.Get(word)
		if !ok {
			continue
		}
		fn, ok := native.FnDefFromValue(v)
		if !ok {
			continue
		}
		return native.FnDefFuncInfo(ns+"."+word, fn)
	}
	return nil
}

// exportWordNames returns every exported word in a descriptor (dotted with its
// namespace), sorted — used to hint after a missing-word error.
func exportWordNames(desc native.ModuleDesc) []string {
	var out []string
	for ns, om := range desc.Exports {
		if om == nil {
			continue
		}
		for _, word := range om.Keys() {
			out = append(out, ns+"."+word)
		}
	}
	sort.Strings(out)
	return out
}

// qualifiedExportInfo resolves a dotted "Namespace.word" export across the
// native modules and builds its help.FuncInfo. Returns nil when the namespace
// or word is unknown. Unlike the runtime `describe`, the CLI has nothing
// imported, so it consults the module registry directly.
func qualifiedExportInfo(reg *native.Registry, name string) *helppkg.FuncInfo {
	dot := strings.IndexByte(name, '.')
	if dot <= 0 || dot >= len(name)-1 {
		return nil
	}
	ns, word := name[:dot], name[dot+1:]

	for _, m := range modules.Names() {
		desc, derr := modulesResolve(m, reg)
		if derr != nil {
			continue
		}
		exports, ok := desc.Exports[ns]
		if !ok {
			continue
		}
		ev, ok := exports.Get(word)
		if !ok {
			return nil // namespace matched but no such word
		}
		fn, ok := native.FnDefFromValue(ev)
		if !ok {
			return nil
		}
		return native.FnDefFuncInfo(name, fn)
	}
	return nil
}

// newRegistry builds a registry wired for module resolution: the parser is
// installed (file modules need it) and the native-module resolver is enabled
// so "boru:<name>" references load.
func newRegistry() (*native.Registry, error) {
	reg, err := defaultRegistry()
	if err != nil {
		return nil, err
	}
	reg.SetParseFunc(parse.Parse)
	modules.InstallResolver(reg)
	return reg, nil
}

func isModule(name string) bool {
	for _, m := range modules.Names() {
		if m == name {
			return true
		}
	}
	return false
}
