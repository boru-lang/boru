// Type-directed member completion: when the cursor sits just after `RECV.`,
// offer the members that may follow the dot instead of the flat word list.
//
// The receiver's members are resolved in layers:
//
//   1. module namespace  — `Rand.` → the module's exports (scanned from the
//      buffer's `import "boru:…"` statements; no type-check needed).
//   2. static type       — truncate the buffer right before the dot, run the
//      checker, and read the receiver's type off the residual stack; then ask
//      native.MembersOfType for that type's Micron properties / class /
//      record / user-Micron-kind fields.
//   3. map keys (v2)     — when the type erases to a bare Map, recover the keys
//      from the nearest `def <recv> { … }` literal in the source.
//
// Anything else falls back to the global word list.

package lsp

import (
	"regexp"
	"strings"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
)

// completionItemsAt returns member completions when the cursor follows a
// `RECV.`, else the global word list. In a member context it returns members
// or nothing — never the global vocabulary, which is never valid after a dot.
func (s *server) completionItemsAt(src string, pos Position) []CompletionItem {
	if recv, ok := receiverBefore(src, pos); ok {
		return s.memberItems(src, recv)
	}
	return s.buildCompletionItems()
}

// receiverInfo describes the `RECV.` context left of the cursor.
type receiverInfo struct {
	word      string // the identifier immediately left of the dot (e.g. "Rand", "host")
	simple    bool   // recv is a standalone identifier (not a `a.b` chain / group)
	dotOffset int    // byte offset of the dot in src
}

// receiverBefore reports whether the cursor sits after a `RECV.` and, if so,
// describes the receiver. Character is treated as a byte column (boru code is
// ASCII); a multibyte line degrades to the global list rather than misfiring.
func receiverBefore(src string, pos Position) (receiverInfo, bool) {
	// Locate the line containing pos.
	lineStart, curLine := 0, 0
	for i := 0; i < len(src) && curLine < pos.Line; i++ {
		if src[i] == '\n' {
			curLine++
			lineStart = i + 1
		}
	}
	if curLine != pos.Line {
		return receiverInfo{}, false
	}
	lineEnd := lineStart
	for lineEnd < len(src) && src[lineEnd] != '\n' {
		lineEnd++
	}
	line := src[lineStart:lineEnd]
	if pos.Character < 0 || pos.Character > len(line) {
		return receiverInfo{}, false
	}
	// Skip the partial member already typed after the dot (identifier chars,
	// but NOT '.').
	col := pos.Character
	for col > 0 && isMemberChar(line[col-1]) {
		col--
	}
	// The char immediately left must be the '.'.
	if col == 0 || line[col-1] != '.' {
		return receiverInfo{}, false
	}
	dotCol := col - 1
	// The receiver word is the identifier run immediately left of the dot.
	wEnd := dotCol
	wStart := wEnd
	for wStart > 0 && isMemberChar(line[wStart-1]) {
		wStart--
	}
	if wStart == wEnd {
		// No identifier immediately left of the dot. A parenthesised group
		// like `(make X …).` is still a member context — resolve it purely
		// via the truncate-and-check path (word empty, not simple).
		if dotCol > 0 && line[dotCol-1] == ')' {
			return receiverInfo{word: "", simple: false, dotOffset: lineStart + dotCol}, true
		}
		return receiverInfo{}, false
	}
	word := line[wStart:wEnd]
	// "simple" iff the receiver is a bare identifier — the char before it is
	// not part of a longer token (no `a.b` chain, no `)` group).
	simple := wStart == 0 || (line[wStart-1] != '.' && line[wStart-1] != ')')
	return receiverInfo{word: word, simple: simple, dotOffset: lineStart + dotCol}, true
}

// isMemberChar reports whether c is a boru member/identifier char (excludes
// '.', which delimits members).
func isMemberChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// memberItems resolves the members for a `RECV.` context. It always returns a
// slice (empty when the receiver has no statically-known members) — never the
// global word list.
func (s *server) memberItems(src string, recv receiverInfo) []CompletionItem {
	// 1. Module namespace fast-path — no type-check needed.
	if recv.simple && isCapitalised(recv.word) {
		if items := s.moduleMemberItems(src, recv.word); len(items) > 0 {
			return items
		}
	}
	// 2. Static type of the receiver, via truncate-and-check.
	if a, err := newCompletionBoru(); err == nil {
		res, _ := a.Check(src[:recv.dotOffset])
		typeName := lastStackType(res.Stack)
		if members := native.MembersOfType(typeName, a.NativeRegistry()); len(members) > 0 {
			return membersToItems(members)
		}
		// 3. v2 — the type erased to a bare Map; recover keys from a
		//    `def recv {…}` literal (covers `def m {a:1 b:2}` and
		//    `def x (make R {a:1 …})`).
		if recv.simple && isMapLike(typeName) {
			if keys := mapLiteralKeys(src, recv.word); len(keys) > 0 {
				return keyItems(keys)
			}
		}
	}
	return []CompletionItem{}
}

// lastStackType returns the top (last) residual-stack type name, or "".
func lastStackType(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1]
}

func isCapitalised(s string) bool { return s != "" && s[0] >= 'A' && s[0] <= 'Z' }

// isMapLike reports whether a checker type name is a bare/erased map, whose
// keys are not carried in the static type.
func isMapLike(typeName string) bool {
	return typeName == "Map" || typeName == "FlexMap" ||
		strings.Contains(typeName, "Map") && strings.HasPrefix(typeName, "dynamic")
}

var importRe = regexp.MustCompile(`(?m)^\s*import\s+"boru:([a-z0-9][a-z0-9-]*)"`)

// importedModuleIDs returns the bare module ids imported in src.
func importedModuleIDs(src string) []string {
	var ids []string
	for _, m := range importRe.FindAllStringSubmatch(src, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// moduleMemberItems resolves each module imported in src and, when the receiver
// word matches one of its export NAMESPACES (the real bound name — e.g. IO,
// StringUtil, Rand — read straight off the module rather than guessed), returns
// that namespace's exports.
func (s *server) moduleMemberItems(src, word string) []CompletionItem {
	reg := s.ensureRegistry()
	if reg == nil {
		return nil
	}
	for _, id := range importedModuleIDs(src) {
		desc, err := modules.Resolve(id, reg)
		if err != nil {
			continue
		}
		exp, ok := desc.Exports[word]
		if !ok || exp == nil {
			continue
		}
		items := make([]CompletionItem, 0, exp.Len())
		for _, name := range exp.Keys() {
			items = append(items, CompletionItem{
				Label:      name,
				Kind:       completionKindMethod,
				InsertText: name,
				Detail:     word + "." + name,
			})
		}
		return items
	}
	return nil
}

// membersToItems converts native.Member values to completion items.
func membersToItems(members []native.Member) []CompletionItem {
	items := make([]CompletionItem, 0, len(members))
	for _, m := range members {
		kind := completionKindProperty
		if m.Kind == "field" {
			kind = completionKindField
		}
		items = append(items, CompletionItem{
			Label:      m.Name,
			Kind:       kind,
			InsertText: m.Name,
			Detail:     m.Detail,
		})
	}
	return items
}

func keyItems(keys []string) []CompletionItem {
	items := make([]CompletionItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, CompletionItem{Label: k, Kind: completionKindField, InsertText: k, Detail: "key"})
	}
	return items
}

var defRe = regexp.MustCompile(`\bdef\s+([A-Za-z_][A-Za-z0-9_-]*)\b`)

// mapLiteralKeys recovers the top-level keys of the first `{ … }` map literal
// on the binding `def <recv> …`, so anonymous maps and record-instance makes
// complete their keys. Braces inside strings and nested maps are skipped.
func mapLiteralKeys(src, recv string) []string {
	// Find `def <recv>`; bound the search to that binding (up to the next def
	// or EOF) so a later statement's braces aren't mistaken for this one.
	loc := findDef(src, recv)
	if loc < 0 {
		return nil
	}
	window := src[loc:]
	if next := nextDef(window); next > 0 {
		window = window[:next]
	}
	open := strings.IndexByte(window, '{')
	if open < 0 {
		return nil
	}
	return topLevelKeys(window[open:])
}

// findDef returns the byte offset just after `def <recv>`, or -1.
func findDef(src, recv string) int {
	for _, m := range defRe.FindAllStringSubmatchIndex(src, -1) {
		if src[m[2]:m[3]] == recv {
			return m[3]
		}
	}
	return -1
}

// nextDef returns the offset of the next `def ` after position 0 in window (to
// bound a binding), or -1.
func nextDef(window string) int {
	m := defRe.FindStringIndex(window[1:])
	if m == nil {
		return -1
	}
	return m[0] + 1
}

// topLevelKeys scans a `{ … }` block starting at s[0]=='{' and returns the
// `key:` identifiers at brace-depth 1, honouring nested braces and strings.
func topLevelKeys(s string) []string {
	var keys []string
	depth := 0
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			i = skipString(s, i)
			continue
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return keys
			}
		case depth == 1 && (isMemberChar(c) || c == '.'):
			// A candidate key: read the identifier, then require a ':'.
			j := i
			for j < len(s) && (isMemberChar(s[j]) || s[j] == '.') {
				j++
			}
			k := s[i:j]
			// skip spaces before ':'
			t := j
			for t < len(s) && (s[t] == ' ' || s[t] == '\t') {
				t++
			}
			if t < len(s) && s[t] == ':' && k != "" && !(k[0] >= '0' && k[0] <= '9') {
				keys = append(keys, k)
			}
			i = j
			continue
		}
		i++
	}
	return keys
}

// skipString advances past a quoted string starting at s[i], returning the
// index just after the closing quote (or len(s)).
func skipString(s string, i int) int {
	q := s[i]
	i++
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == q {
			return i + 1
		}
		i++
	}
	return i
}

// newCompletionBoru is the instance a completion's truncate-and-check runs
// on: under the documented environment policy, as the diagnostics' check is
// (NUR079) — a policy that cannot be resolved offers no completion from it.
func newCompletionBoru() (*lang.Boru, error) {
	pol, err := envPolicy()
	if err != nil {
		return nil, err
	}
	return langNew(lang.Options{Policy: pol})
}
