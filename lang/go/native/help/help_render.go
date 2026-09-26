package help

import (
	"io"
	"sort"
)

// ModuleInfo is one row of the built-in module catalog: the bare module id
// (the part after "boru:", e.g. "type-util") and its one-line summary.
type ModuleInfo struct {
	Name    string
	Summary string
}

// moduleCatalog is the catalog of compiled-in native modules and their
// one-line summaries. It lives here — rather than in the cmd or modules
// package — so every help surface that can't import the modules package (the
// `describe` language word in particular) can still render the module index.
// TestModuleCatalogMatchesModules (modules package) pins this list to
// modules.Names() so it cannot drift.
var moduleCatalog = []ModuleInfo{
	{"math-util", "Floating-point math: trig, logs, roots, constants."},
	{"array-util", "APL-style array vocabulary: shape, select, group, windows."},
	{"time-util", "Dates, durations, timezones, clocks, timers, and intervals."},
	{"matrix-util", "Tensors, matrices, and vectors with linear algebra."},
	{"bin-util", "Bitwise and byte-buffer helpers: masks, rotates, hashes."},
	{"tui", "Terminal UIs: raw-terminal drawing and input events behind a host-injected backend."},
	{"type-util", "Type introspection and construction utilities."},
	{"fn-util", "Point-free function vocabulary: compose, pipe, curry, partial, const, identity, flip, on, memoize."},
	{"vm", "Low-level virtual-machine primitives."},
	{"report", "Tabular reporting and formatting."},
	{"test", "Assertions and helpers for in-language tests."},
	{"rand", "Pseudo-random number generation."},
	{"query", "SQL-style query pipelines: from, where, join, group, order."},
	{"struct-util", "Structured-data utilities: merge, walk, transform, jsonify, …."},
	{"io", "I/O: read/write (text/binary/positioned), stat, list, remove, move, copy, link, touch, folder, watch/unwatch, mount/unmount, stdin/stdout/stderr, printstr, trace."},
	{"net", "Networking: HTTP requests, raw TCP sockets, codecs, and service endpoints."},
	{"logic-util", "Derived boolean connectives: nand, nor, xnor, iff, implies."},
	{"string-util", "String manipulation: concat, split, trim, upper, lower, …."},
	{"minilang", "Embedded mini-languages behind the `mini` word: re (Go regexp), bf (brainfuck), gex (globs), register."},
	{"parselang", "Named parsers behind the `parse` word: register a parser, parse a source into an AST."},
	{"cli", "Command-line argument parsing, written in boru: a spec map drives parse / dispatch / usage / main, with argv always an explicit parameter."},
	{"sift", "Semi-structured text parsing (the awk tier): kv, blocks, columns, dsv, fixed, and pattern families via the Sift namespace (Sift.parse / define / …), plus spec-map extensions."},
	{"emitlang", "Emit data structures to strings behind the `emit` word: json, jsonic, csv, tsv, yaml, xml, toml, ini."},
	{"parse", "Define custom parsers: ABNF grammars, declarative rules, lex matchers, and custom-type actions, registered as `parse` kinds."},
	{"model", "Build a system model from .jsonic source (via aontu unification) and run generator actions over it: new, run, start, stop, model."},
	{"log", "Structured logging: severity levels, fields, console/memory sinks, a global threshold."},
	{"debug", "Debugging: print taps, value sizing, and performance measurement (its seven self-knowledge words are deprecated copies of boru:scry's)."},
	{"scry", "Self-knowledge as data: the words, defs, modules, signatures, bodies, references and value shapes of the running system."},
	{"repl", "A socket REPL server and client, written in boru over boru:net."},
	{"fmt", "Source formatting: pretty-print boru into canonical layout (shared with the `boru fmt` CLI)."},
	{"vault", "Bridge to the host's secret vault: status, secrets, capabilities, passwords, maintenance, multi-vault — behind a host-injected backend."},
	{"vault-tui", "The interactive vault TUI, written in boru over boru:tui + boru:vault (the `boru vault -i --boru` app)."},
}

// ModuleCatalog returns the built-in module catalog sorted by name. The
// returned slice is a fresh copy; callers may keep or mutate it freely.
func ModuleCatalog() []ModuleInfo {
	out := make([]ModuleInfo, len(moduleCatalog))
	copy(out, moduleCatalog)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ModuleSummary returns the one-line summary for a built-in module (keyed by
// its bare id, e.g. "type-util"), or "" when the module is unknown.
func ModuleSummary(name string) string {
	for _, m := range moduleCatalog {
		if m.Name == name {
			return m.Summary
		}
	}
	return ""
}

// WriteWordsByCategory writes the "words grouped by category" block: each
// category's name and summary, then its words wrapped under an indent. Shared
// by the CLI `boru describe` index and the REPL `describe` word so the two read
// identically.
func WriteWordsByCategory(w io.Writer) {
	for _, cat := range categories {
		writeLine(w, "  "+pad(cat.Name, 9)+" "+cat.Summary)
		writeWordGrid(w, cat.Words)
		// Module-provided words are listed under the import that binds
		// them. Without this they read as builtins and every one of them
		// fails as undefined_word in a bare program.
		for _, id := range cat.ModuleIDs() {
			writeLine(w, "    (import \"boru:"+id+"\")")
			writeWordGrid(w, cat.Module[id])
		}
	}
}

// WriteModuleCatalog writes the module catalog block: each module id and its
// one-line summary, sorted by name.
func WriteModuleCatalog(w io.Writer) {
	for _, m := range ModuleCatalog() {
		writeLine(w, "  "+pad(m.Name, 12)+" "+m.Summary)
	}
}

// WriteCategory writes one category's header and its words, each with its
// one-line summary — the body of `describe <category>`.
func WriteCategory(w io.Writer, cat Category) {
	writeLine(w, cat.Name+" — "+cat.Summary)
	writeLine(w, "")
	wrote := false
	if len(cat.Words) > 0 {
		writeLine(w, "Words:")
		for _, word := range cat.Words {
			writeLine(w, "  "+pad(word, 12)+" "+wordSummary(word))
		}
		wrote = true
	}
	for _, id := range cat.ModuleIDs() {
		if wrote {
			writeLine(w, "")
		}
		wrote = true
		writeLine(w, "Words from import \"boru:"+id+"\" (call as <Export>.<word>):")
		for _, word := range cat.Module[id] {
			writeLine(w, "  "+pad(word, 12)+" "+wordSummary(word))
		}
	}
}

// wordSummary is a word's one-line summary, or "" when it has no Entry.
func wordSummary(word string) string {
	if e := Lookup(word); e != nil {
		return e.Summary
	}
	return ""
}

// writeWordGrid prints words wrapped under a 4-space indent at ~72 columns.
func writeWordGrid(w io.Writer, words []string) {
	const indent = "    "
	const width = 72
	line := indent
	for _, word := range words {
		if len(line) > len(indent) && len(line)+1+len(word) > width {
			writeLine(w, line)
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += word
	}
	if len(line) > len(indent) {
		writeLine(w, line)
	}
}

// pad right-pads s to at least n runes with spaces (like %-Ns).
func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// writeLine writes s followed by a newline, ignoring write errors (these
// render to a REPL/CLI writer where a failed write is already fatal upstream).
func writeLine(w io.Writer, s string) {
	_, _ = io.WriteString(w, s+"\n")
}
