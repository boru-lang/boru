package native

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// This file holds the shared implementation of the `describe` language word so
// the REPL word (describeWordHandler / describeSelfHandler in native_misc.go)
// and the REPL `/describe` meta-command render identically — and the same way
// the CLI `boru describe` does. The forms mirror the CLI:
//
//	describe                       a categorised guide to words and modules
//	describe add                   full docs for one word
//	describe math                  the words in one category
//	describe "boru:type-util"       a module and the words it exports
//	describe "boru:type-util:foo"   one exported word of a module
//
// Module references carry a ':' so they must be quoted in source; the index
// hints show the quoted form. Unlike the CLI (which has nothing imported and
// scans the module set for a dotted Namespace.word), this runs in a live engine
// and consults the imported namespace bindings first.

// DescribeIndex writes the categorised guide: every built-in word grouped by
// category, the loadable modules, then how to drill into each. It is the body
// of the no-argument `describe`.
func DescribeIndex(w io.Writer) {
	fmt.Fprint(w, "boru language reference — built-in words by category, and loadable modules.\n\n")
	fmt.Fprint(w, "Words:\n")
	help.WriteWordsByCategory(w)
	fmt.Fprint(w, "\nModules (load with import \"boru:<name>\"):\n")
	help.WriteModuleCatalog(w)
	fmt.Fprint(w, "\nDrill in:\n")
	fmt.Fprint(w, "  describe <word>                  e.g. describe add\n")
	fmt.Fprint(w, "  describe <category>              e.g. describe math\n")
	fmt.Fprint(w, "  describe \"boru:<module>\"          e.g. describe \"boru:type-util\"\n")
	fmt.Fprint(w, "  describe \"boru:<module>:<word>\"   e.g. describe \"boru:type-util:tpartial\"\n")
	fmt.Fprint(w, "Docs: "+help.ReferenceURL+"\n")
}

// DescribeName writes documentation for a word, category, module, or module
// word. It resolves live registry state (imported modules, user-defined
// class/surface types) and, for an unrecognised name, tries to load it as a
// module. r may be nil — static word lookups still work.
func DescribeName(r *Registry, w io.Writer, name string) {
	// A ':' marks a module reference: "boru:type-util", "boru:type-util:foo",
	// or "type-util:foo". No word or def name carries a colon.
	if strings.Contains(name, ":") {
		describeModulePathTo(r, w, name)
		return
	}

	// A category groups related words. Checked before words so generic names
	// (math, type, string, …) open the group.
	if cat, ok := help.LookupCategory(name); ok {
		help.WriteCategory(w, cat)
		return
	}

	if r != nil {
		// A dotted name (TypeUtil.tpartial) names an export of an *imported*
		// module — resolve it from the namespace binding import installed.
		if info := BuildQualifiedFuncInfo(r, name); info != nil {
			fmt.Fprint(w, FormatWordHelp(r, *info))
			return
		}
		// A name bound to a class/object or surface type is not a word; show
		// its schema/contract instead of the empty word view.
		if bound, ok := r.Defs.Top(name); ok {
			// A type name denotes its node (the Stage 2 flip); the
			// schema/contract to show is the node's recorded content.
			if body, bok := TypeContentOf(bound); bok {
				bound = body
			}
			if IsClassType(bound) {
				fmt.Fprint(w, formatTypeSchema(name, bound))
				return
			}
			if IsSurfaceType(bound) {
				fmt.Fprint(w, formatSurfaceSchema(name, bound))
				return
			}
		}
		// A registered word renders from live signature data.
		if info := BuildFuncInfo(r, name); info != nil {
			fmt.Fprint(w, FormatWordHelp(r, *info))
			return
		}
	}

	// A documented-but-unregistered word falls back to its static entry.
	if entry := help.Lookup(name); entry != nil {
		fmt.Fprint(w, help.Format(entry))
		return
	}

	// A bare built-in module name (math-util, struct-util, …).
	if help.ModuleSummary(name) != "" {
		if desc, err := ResolveAnyModule(r, "boru:"+name); err == nil {
			writeModuleDesc(w, "boru:"+name, desc)
			return
		}
	}

	// Unknown: give it one more chance as a loadable module (installed/file).
	if r != nil {
		if desc, err := ResolveAnyModule(r, name); err == nil {
			writeModuleDesc(w, name, desc)
			return
		}
	}

	fmt.Fprintf(w, "describe: no description available for %q\n", name)
	fmt.Fprintln(w, "Run `describe` (no argument) to list words and modules.")
}

// describeModulePathTo renders a module, or one word within it, for a
// colon-form name. Built-in modules resolve through the native resolver;
// anything else is loaded on demand.
func describeModulePathTo(r *Registry, w io.Writer, name string) {
	modRef, word := splitModulePath(name)
	resolveRef := modRef
	displayRef := modRef
	if short := strings.TrimPrefix(modRef, "boru:"); help.ModuleSummary(short) != "" {
		// A built-in: route to the native resolver and show the canonical id.
		resolveRef = "boru:" + short
		displayRef = "boru:" + short
	}

	desc, err := ResolveAnyModule(r, resolveRef)
	if err != nil {
		fmt.Fprintf(w, "describe: cannot load module %q: %s\n", modRef, err)
		fmt.Fprintln(w, "Run `describe` (no argument) to list words and modules.")
		return
	}

	if word == "" {
		writeModuleDesc(w, displayRef, desc)
		return
	}

	if info := exportInfoFromDesc(desc, word); info != nil {
		fmt.Fprint(w, FormatWordHelp(r, *info))
		return
	}

	fmt.Fprintf(w, "describe: module %s has no exported word %q\n", displayRef, word)
	if names := exportWordNames(desc); len(names) > 0 {
		fmt.Fprintf(w, "Exported words: %s\n", strings.Join(names, " "))
	}
}

// splitModulePath separates a colon-form name into a module reference and an
// optional word. "boru:type-util:foo" → ("boru:type-util", "foo");
// "boru:type-util" → ("boru:type-util", ""); "type-util:foo" → ("type-util",
// "foo"). The "boru:" prefix is preserved on the module reference.
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

// writeModuleDesc prints a module's summary (when built-in) and the words it
// exports, plus how to import and drill into them.
func writeModuleDesc(w io.Writer, ref string, desc ModuleDesc) {
	if summary := help.ModuleSummary(strings.TrimPrefix(ref, "boru:")); summary != "" {
		fmt.Fprintf(w, "%s — %s\n", ref, summary)
	} else {
		fmt.Fprintf(w, "%s\n", ref)
	}
	fmt.Fprint(w, "\n")

	exportNames := make([]string, 0, len(desc.Exports))
	for k := range desc.Exports {
		exportNames = append(exportNames, k)
	}
	sort.Strings(exportNames)

	fmt.Fprint(w, "Words (call as <export>.<word>):\n")
	for _, export := range exportNames {
		words := desc.Exports[export].Keys()
		sort.Strings(words)
		for _, word := range words {
			fmt.Fprintf(w, "  %s.%s\n", export, word)
		}
	}

	fmt.Fprint(w, "\n")
	fmt.Fprintf(w, "Load with import \"%s\", then call e.g. <export>.<word>.\n", ref)
	fmt.Fprintf(w, "Describe one word with describe \"%s:<word>\".\n", ref)
	fmt.Fprint(w, "Docs: "+help.ReferenceURL+"\n")
}

// exportInfoFromDesc resolves a single word across a descriptor's export
// namespaces (walked sorted, for determinism) and builds its help.FuncInfo.
func exportInfoFromDesc(desc ModuleDesc, word string) *help.FuncInfo {
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
		fn, ok := FnDefFromValue(v)
		if !ok {
			continue
		}
		return FnDefFuncInfo(ns+"."+word, fn)
	}
	return nil
}

// exportWordNames returns every exported word (dotted with its namespace),
// sorted — used to hint after a missing-word error.
func exportWordNames(desc ModuleDesc) []string {
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
