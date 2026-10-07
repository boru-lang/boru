package core

// The def census (design/IMMUTABLE-DEF.1.md §5, phase 0): the check pass
// classifies every binding install and every read of a leaked binding by
// what the scope rule will make of it, and reports the list instead of
// changing anything — the migration aid for the corpus, the tree and the
// downstream libraries. The classes are the rule's own cases:
//
//	rebind         a second binding of a name in the SAME scope — a module
//	               `def x … def x`, a fn-body local re-bound, a param re-bound
//	               by a body def, a `do` body's def over the enclosing scope's
//	               (do is transparent). Becomes `redefinition`.
//	overlap        a `def f fn […]` whose signature overlaps an overload the
//	               same scope already bound (today it silently replaces).
//	               Becomes `redefinition`.
//	extend-inner   a `def f fn […]` where an ENCLOSING scope binds the fn:
//	               an inner scope cannot extend a word. Becomes `redefinition`.
//	shadow-rebind  a block- or frame-local def of a name an enclosing scope
//	               binds, which this scope READ before binding it — the
//	               loop counter `def n 0 for 3 [def n (n add 1)]`, legal under
//	               the rule and answering differently (0, not 3). Becomes the
//	               `shadow_rebind` warning.
//	shadow         any other inner-scope binding of an enclosing name. Legal.
//	leak-read      a read of a binding a block made, after the block ended
//	               (`if c [def w 1] [def w 2] w`). Becomes `undefined_word`.
//	undef          a use of `undef`, which the rule removes.
//
// Only the check pass records (the interpreter's run is never a census),
// and only the first record of a (class, name, site) — a fn body is analysed
// once at construction and again per call shape.

import (
	"sort"
	"strconv"
)

// defCensusKey dedupes findings: one record per class, name and site.
func defCensusKey(e DefCensusEntry) string {
	return string(e.Class) + "|" + e.Name + "|" + strconv.Itoa(e.Site.Row) + ":" + strconv.Itoa(e.Site.Col)
}

// DefCensusClass is the rule's verdict on one binding or read.
type DefCensusClass string

const (
	CensusRebind       DefCensusClass = "rebind"
	CensusOverlap      DefCensusClass = "overlap"
	CensusExtendInner  DefCensusClass = "extend-inner"
	CensusShadowRebind DefCensusClass = "shadow-rebind"
	CensusShadow       DefCensusClass = "shadow"
	CensusLeakRead     DefCensusClass = "leak-read"
	CensusUndef        DefCensusClass = "undef"
)

// DefCensusEntry is one finding: the site of the binding or read the class
// names, and the standing binding it rebinds, shadows, extends or leaks
// from (zero when the binder knew no site).
type DefCensusEntry struct {
	Class    DefCensusClass `json:"class"`
	Name     string         `json:"name"`
	Site     SrcPos         `json:"site"`
	Standing SrcPos         `json:"standing,omitempty"`
	// Scope is the kind of scope the new binding (or the read) is in.
	Scope ScopeKind `json:"scope"`
	// Note qualifies the class by the standing binding's kind: "param" (a
	// frame binding — a param or a capture), "module" (a namespace), "fn",
	// "type" or "value".
	Note string `json:"note,omitempty"`
	// Fn is the named fn whose body was under analysis, when one was: a
	// finding's positions are the body's source's, which for an imported
	// module's fn is not the file being checked.
	Fn string `json:"fn,omitempty"`
}

// censusFn is the innermost named fn body under analysis, "" outside one.
func (r *Registry) censusFn() string {
	if n := len(r.Check.FnNameStack); n > 0 {
		return r.Check.FnNameStack[n-1]
	}
	return ""
}

// NoteDefCensus records one finding, once per (class, name, site).
func (c *CheckState) NoteDefCensus(e DefCensusEntry) {
	if c == nil || !c.IsActive() {
		return
	}
	key := defCensusKey(e)
	if c.DefCensusSeen == nil {
		c.DefCensusSeen = map[string]bool{}
	}
	if c.DefCensusSeen[key] {
		return
	}
	c.DefCensusSeen[key] = true
	c.DefCensus = append(c.DefCensus, e)
}

// SortedDefCensus is the pass's findings in source order (site, then class).
func (c *CheckState) SortedDefCensus() []DefCensusEntry {
	if c == nil || len(c.DefCensus) == 0 {
		return nil
	}
	out := make([]DefCensusEntry, len(c.DefCensus))
	copy(out, c.DefCensus)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Site.Row != b.Site.Row {
			return a.Site.Row < b.Site.Row
		}
		if a.Site.Col != b.Site.Col {
			return a.Site.Col < b.Site.Col
		}
		return a.Class < b.Class
	})
	return out
}

// bindingKind names what a standing entry binds, for the finding's note.
func bindingKind(e DefEntry) string {
	switch {
	case e.Frame:
		return "param"
	case e.TypeDef != nil:
		return "type"
	case ModuleNSOf(e.Body) != nil:
		return "module"
	case e.Body.Data != nil:
		if _, isFn := e.Body.Data.(FnDefInfo); isFn {
			return "fn"
		}
	}
	return "value"
}

// noteBindCensus classifies the install of name over the standing top
// entry (if any) and records the finding. isFn is whether the new body is a
// fn (an overload candidate); overlap is the entry of name's stack whose
// signatures the new fn overlaps, nil when none does. Called by installDef
// and the type installers before they push, with shadow (frame) installs
// excluded by the callers.
func (r *Registry) noteBindCensus(name string, standing DefEntry, has, isFn bool, overlap *DefEntry) {
	if r == nil || r.Check == nil || !r.Check.IsActive() || !has {
		return
	}
	// `_` is the discard by convention — bound to drop a value and bound
	// again freely, in one scope or a block of it — so the census and the
	// block gate pass it over (phase 3's redefinition rule should make it a
	// formal discard; design/IMMUTABLE-DEF.1.md #15).
	if name == "_" {
		return
	}
	site := r.censusSite()
	cur := r.Defs.ScopeID()
	kind := r.Defs.ScopeKindNow()
	e := DefCensusEntry{Name: name, Site: site, Standing: standing.Site, Scope: kind, Note: bindingKind(standing), Fn: r.censusFn()}
	switch {
	case standing.Leaked:
		// The standing binding is the model's own re-install of a block's
		// def (today's leak); under the rule it does not exist, so a def
		// over it rebinds and shadows nothing.
		return
	case standing.Scope != cur && !r.Defs.LexicallyEncloses(standing.Scope):
		// A caller's binding: visible to this frame as every binding is,
		// but no enclosing scope of it. The install is this scope's own
		// — a module's fns may each keep a local `loop` and call each
		// other.
		return
	case isFn && bindingKind(standing) == "fn":
		if standing.Scope != cur {
			e.Class = CensusExtendInner
		} else if overlap != nil {
			e.Class = CensusOverlap
			e.Standing = overlap.Site
		} else {
			return // an overload added in the scope that bound the word: legal
		}
	case standing.Scope == cur:
		e.Class = CensusRebind
	case r.Defs.ReadInScope(name):
		e.Class = CensusShadowRebind
	default:
		e.Class = CensusShadow
	}
	r.Check.NoteDefCensus(e)
	if kind != ScopeBlock {
		return
	}
	// A BLOCK-LOCAL def over a name an enclosing scope binds (phase 2 of
	// design/IMMUTABLE-DEF.1.md): the enclosing binding is unchanged after
	// the block. One that READ the name it shadows is the counter shape
	// (`def n 0 for 3 [def n (n add 1)] n` answers 0): legal, with the
	// shadow_rebind warning naming the intended spelling. Either shape keeps
	// the compiled lane from the program for now: the compiler's own block
	// scopes land in phase 2's second step, and until then its unit would
	// reproduce the leak the interpreter no longer has — a decline, never a
	// wrong answer, counted in the compile-defect ledgers.
	if e.Class == CensusShadowRebind {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code: "shadow_rebind",
			Detail: "def " + name + ": this block-local def reads the `" + name + "` it shadows; the enclosing binding is unchanged after the block — " +
				"to update a binding from inside a block, declare it with `var " + name + " …` and assign it as `var " + name + " …`",
			Word: name,
			Row:  site.Row,
			Col:  site.Col,
		})
	}
	r.Check.Recorder().MarkUncompilable("block-local def `" + name + "` shadows an enclosing binding (the compiler's block scopes land with phase 2's second step)")
}

// censusSite is the site of the binding being installed: the def-name token
// InstallAndRecordDef staged (PendingBindPos), else the word under dispatch
// (CurWordPos) — a type binding stages nothing, see basic's DefHandler.
func (r *Registry) censusSite() SrcPos {
	if site := r.Check.PendingBindPos; site.Row != 0 {
		return site
	}
	return r.Check.CurWordPos
}

// stampDefSite records the census site of the binding just pushed under
// name as the entry's Site, the position a later finding reports as the
// standing binding's. Only the check pass sites its bindings.
func (r *Registry) stampDefSite(name string) {
	if r == nil || r.Check == nil || !r.Check.IsActive() {
		return
	}
	r.Defs.SetTopSite(name, r.censusSite())
}

// noteReadCensus records a read of name under the check pass: the innermost
// scope's read set (the shadow-rebind question), and a leak-read finding
// when the binding read is one a block left behind and nothing live stands
// under it — under the rule the name is then undefined. A live binding
// under the leaked one is what the read resolves to under the rule; that
// shape is the block def's own shadow finding, legal.
func (r *Registry) noteReadCensus(name string, pos SrcPos) {
	if r == nil || r.Check == nil || !r.Check.IsActive() {
		return
	}
	r.Defs.NoteRead(name)
	e, ok := r.Defs.TopEntry(name)
	if !ok || !e.Leaked {
		return
	}
	for _, under := range r.Defs.Entries(name) {
		if !under.Leaked {
			return
		}
	}
	r.Check.NoteDefCensus(DefCensusEntry{Class: CensusLeakRead, Name: name, Site: pos, Standing: e.Site, Scope: r.Defs.ScopeKindNow(), Note: bindingKind(e), Fn: r.censusFn()})
}

// blockTypeGate keeps the compiled lane from a program that binds a TYPE
// inside a block — fresh or a shadow, `each [def ZB (Integer gt 0) 7] …`,
// `for 2 [do [def Big Integer 15 is Big]]` — until the compiler's block
// scopes land (phase 2's second step): a loop body and an arm are not
// scopes there yet, so the second iteration's install finds the first's
// name reservation standing where the interpreter retired it with the
// block. A decline, never a wrong answer; the type installers call it.
func (r *Registry) blockTypeGate(name string) {
	if r == nil || r.Check == nil || !r.Check.IsActive() || r.Defs.ScopeKindNow() != ScopeBlock {
		return
	}
	r.Check.Recorder().MarkUncompilable("block-local type def `" + name + "` (the compiler's block scopes land with phase 2's second step)")
}

// NoteBlockImport records the namespace an `import` binds INSIDE A BLOCK
// under an active check pass (CheckState.BlockImportNames) — `if c [import
// module […] end M.x] [0]`, `for 2 [import "boru:math-util" end …]`. An
// import is a compile-time word: the check pass runs it and the compiled
// program reads the binding it installed, never re-importing. Inside a
// block that binding ends with the body's check run, so a RUN-TIME read of
// the name (a dynamic-scope lookup) would miss it — the lowerer declines
// such a read (never a wrong answer; a loud bail before), while a read the
// pass folds compiles as it did. Outside a block, or outside a check pass,
// nothing is noted. The language layer's import installers call it.
func NoteBlockImport(r *Registry, name string) {
	if r == nil || r.Check == nil || !r.Check.IsActive() || r.Defs.ScopeKindNow() != ScopeBlock {
		return
	}
	if r.Check.BlockImportNames == nil {
		r.Check.BlockImportNames = map[string]bool{}
	}
	r.Check.BlockImportNames[name] = true
}

// noteUnboundReadCensus records the read of an UNBOUND name under the check
// pass as a leak-read finding when a block that closed in this lexical
// region bound it (DefTable.BlockBoundSite): under the rule the binding
// ended with the block, which is why the read finds nothing — `if b [def z
// 9] [] end z`, `for 2 [def y 9] y`. The engine's undefined-word error is
// the read's site.
func (r *Registry) noteUnboundReadCensus(name string, pos SrcPos) {
	if r == nil || r.Check == nil || !r.Check.IsActive() {
		return
	}
	note, ok := r.Defs.BlockBoundSite(name)
	if !ok {
		return
	}
	r.Check.NoteDefCensus(DefCensusEntry{Class: CensusLeakRead, Name: name, Site: pos, Standing: note.Site, Scope: r.Defs.ScopeKindNow(), Note: note.Note, Fn: r.censusFn()})
}

// NoteDefCensusUse records a use of a construct the rule removes — `undef`
// or the `var` word — at pos, for the language layer's handlers.
func NoteDefCensusUse(r *Registry, class DefCensusClass, name string, pos SrcPos) {
	if r == nil || r.Check == nil {
		return
	}
	r.Check.NoteDefCensus(DefCensusEntry{Class: class, Name: name, Site: pos, Scope: r.Defs.ScopeKindNow(), Fn: r.censusFn()})
}
