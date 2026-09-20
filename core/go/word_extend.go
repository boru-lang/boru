package core

import "fmt"

// Word extensions — the open-words merge (design/OPEN-WORDS.0.md).
//
// `def <word> fn […]` on a word that carries at least one LOCKED
// signature (a native / host registration, or a module-wrapper
// rebinding that inherited locked inner sigs) does not replace the
// word: it constructs a WORD CLONE — the base word's complete
// signature list plus the merge — and binds it through the ordinary
// DefTable shadow stack. Scoping, `undef`, closure capture and
// sub-engine inheritance all fall out of the existing def machinery;
// Registry.Lookup treats a clone as occluding deeper entries for the
// name (the clone already contains them).
//
// Merge rules (§2.1):
//   - a signature whose argument-type tuple exactly matches an
//     existing UNLOCKED signature REPLACES it;
//   - a tuple matching a LOCKED signature is an error
//     ([boru/locked_signature]) — locked sigs can never be replaced;
//   - any other signature APPENDS. Dispatch order among all sigs is
//     the natural specificity order (CompareSignatures); admission is
//     governed by the ownership-anchor rules (requireOwnedAnchor,
//     design/OPEN-WORDS.1.md).

// sealedWords are the words the engine special-cases BY NAME, where a
// shadow binding would break the identity the kernel relies on — so
// they cannot be def-merged at all (§2.3). The inventory comes from
// the §4.5 audit of name-keyed special cases in the kernel:
//   - "def"  — engine.go::bindsReferent, the pending-forward hints,
//     and macro_expand.go all treat the name as a reliable identity.
//   - "make" — execMatch's autoEvalMap gating keys on match.Name.
//   - "word" — carrier.go's splice-paren expansion keys on the name.
//
// The literals true/false/none/inf/nan are name-cased too but are not
// registered words (reservedLiterals guards them separately).
var sealedWords = map[string]bool{
	"def":  true,
	"make": true,
	"word": true,
}

// IsSealedWord reports whether name is sealed against def-merging —
// the strong tier above locked signatures (§2.3).
func IsSealedWord(name string) bool { return sealedWords[name] }

// hasLockedSig reports whether any signature in the slice is locked.
func hasLockedSig(sigs []Signature) bool {
	for i := range sigs {
		if sigs[i].Locked {
			return true
		}
	}
	return false
}

// HasLockedSigs reports whether the definition carries at least one
// locked signature — the merge trigger for `def <word> fn […]` (§4.1):
// locked-bearing words merge, plain user fns keep whole-replacement
// shadowing (the REPL/iterate idiom).
func HasLockedSigs(fn *FnDefInfo) bool {
	return fn != nil && hasLockedSig(fn.Signatures)
}

// IsWordExtension reports whether v is a word-extension clone and
// returns its FnDefInfo. The named-helper protocol for the provenance
// marker (never probe the Extends field inline) — recognised by the
// import machinery for the export transplant (§2.4).
func IsWordExtension(v Value) (FnDefInfo, bool) {
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok || fnDef.Extends == "" {
		return FnDefInfo{}, false
	}
	return fnDef, true
}

// sigTupleEqual reports whether two signatures declare the SAME
// argument tuple — same arity, same declared type at every position,
// and matching value patterns (both absent, or both present and
// canon-identical). This is the exact-match rule the merge uses to
// decide replace-vs-append; it is deliberately stricter than overlap.
func sigTupleEqual(a, b *Signature) bool {
	if a.TotalArgs() != b.TotalArgs() {
		return false
	}
	for i := 0; i < a.TotalArgs(); i++ {
		at, bt := SigArgType(a, i), SigArgType(b, i)
		if at == nil {
			at = TAny
		}
		if bt == nil {
			bt = TAny
		}
		if !at.Equal(bt) {
			return false
		}
		ap, aok := SigPattern(a, i)
		bp, bok := SigPattern(b, i)
		if aok != bok {
			return false
		}
		if aok && CanonValue(ap) != CanonValue(bp) {
			return false
		}
	}
	return true
}

// mergeExtensionSigs merges incoming signatures into a copy of base's
// dispatch list (fallbacks dropped — Lookup re-synthesises one), per
// the §2.1 rules. origin tags each installed signature's provenance
// ("" for a direct user def; the module ref for a transplant). It
// reports whether anything changed — an entirely idempotent transplant
// (diamond re-import) installs nothing.
func mergeExtensionSigs(r *Registry, name string, base *FnDefInfo, incoming []Signature, origin string) ([]Signature, bool, error) {
	merged := make([]Signature, 0, len(base.Signatures)+len(incoming))
	for i := range base.Signatures {
		if base.Signatures[i].Fallback {
			continue
		}
		merged = append(merged, base.Signatures[i])
	}
	changed := false
	for i := range incoming {
		ns := incoming[i]
		ns.Locked = false
		ns.Origin = origin
		at := -1
		for j := range merged {
			if sigTupleEqual(&merged[j], &ns) {
				at = j
				break
			}
		}
		if at < 0 {
			merged = append(merged, ns)
			changed = true
			continue
		}
		if merged[at].Locked {
			return nil, false, r.BoruErrorHint("locked_signature",
				fmt.Sprintf("def %s: signature %s matches a locked built-in signature and cannot replace it",
					name, sigTupleString(&ns)),
				"def",
				"locked signatures are frozen; add a signature with a different argument-type tuple, or define a new word")
		}
		if origin != "" {
			// Transplant collision policy (§4.4): the same unlocked tuple
			// arriving from a DIFFERENT module than the one that installed
			// it is a loud error — two files that never mention each other
			// must not silently shadow. Identical provenance (diamond
			// re-import) is idempotent and quiet. A direct user def
			// (existing Origin == "") still shadows freely.
			if merged[at].Origin == origin {
				continue
			}
			if merged[at].Origin != "" {
				return nil, false, r.BoruErrorHint("extend_conflict",
					fmt.Sprintf("import: modules %q and %q both extend %s with signature %s",
						merged[at].Origin, origin, name, sigTupleString(&ns)),
					"import",
					"import only one of the extensions, or firewall one module behind an inline module that re-exports just what you need")
			}
		}
		merged[at] = ns
		changed = true
	}
	return merged, changed, nil
}

// IsNominalAnchor reports whether t can ANCHOR a word-extension
// signature (design/OPEN-WORDS.1.md §3.1): an OWNED type whose
// membership is TAG-carried — a value is a member only if constructed
// as one — so no pre-existing value can ever match through it.
// Content-based types (predicate, member-fn, union, negation, schema,
// bounded-Type — everything marked ContentMembership) are excluded:
// their members can predate the type, which would let an anchored
// signature capture previously-valid calls. Unowned ad-hoc types
// (empty OwnerID) never anchor.
func IsNominalAnchor(t *Type) bool {
	if t == nil || t.OwnerID() == "" {
		return false
	}
	// behaviorIsContent walks through a Match-delegating `behave` wrapper
	// so a behave'd predicate/surface stays excluded (design/OPEN-WORDS.1.md
	// §3.1) — a bare marker check would miss the content behavior beneath it.
	return !behaviorIsContent(t.Behavior())
}

// sigHasOwnedAnchor reports whether at least one of the signature's
// argument types is a nominal anchor OWNED by the extending author
// (rule R1, design/OPEN-WORDS.1.md §2): the provenance proof that no
// call predating the merge can match the added signature.
func sigHasOwnedAnchor(s *Signature, owner string) bool {
	if owner == "" {
		return false
	}
	for i := 0; i < s.TotalArgs(); i++ {
		if t := SigArgType(s, i); t != nil && t.OwnerID() == owner && IsNominalAnchor(t) {
			return true
		}
	}
	return false
}

// sigHasModuleAnchor reports whether the signature carries a nominal
// anchor minted by ANY module (owner neither kernel nor program). The
// transplant admission accepts this for SOURCE clones so re-export
// keeps working (design/OPEN-WORDS.1.md §2.4-carryover): a re-exported
// chain's sigs stay anchored on the ORIGINAL module's types, and
// reachability holds for any module-minted nominal anchor — the type
// cannot predate the import chain that delivered it.
func sigHasModuleAnchor(s *Signature) bool {
	for i := 0; i < s.TotalArgs(); i++ {
		t := SigArgType(s, i)
		if t == nil || !IsNominalAnchor(t) {
			continue
		}
		if o := t.OwnerID(); o != OwnerKernel && o != OwnerProgram {
			return true
		}
	}
	return false
}

// requireOwnedAnchor enforces R1 over a set of merge candidates:
// every signature must carry at least one nominal argument type the
// extending author owns. owner is the author's provenance id — the
// scope's TypeTable.MintOwner for a source def-merge, the extension's
// ExtOwner for a host-authored clone. word names the error source
// (`def` for a merge, `import` for a transplant).
// allowModuleAnchors widens the accepted set to any module-minted
// anchor — the transplant path's re-export carveout; def-time
// admission passes false (the author must own the anchor where the
// signature is WRITTEN).
func requireOwnedAnchor(r *Registry, name, word string, sigs []Signature, owner string, allowModuleAnchors bool) error {
	for i := range sigs {
		// A ZERO-ARITY sig is provably additive-only — it can claim
		// nothing but the bare application, and an exact 0-arg locked
		// tuple is still replacement-declined — so, like a fallback, it
		// needs no anchor (`def outer fn [[] …]` colliding with the
		// higher-order `outer` word is everyday code, not an override).
		if sigs[i].Fallback || sigs[i].TotalArgs() == 0 || sigHasOwnedAnchor(&sigs[i], owner) {
			continue
		}
		if allowModuleAnchors && sigHasModuleAnchor(&sigs[i]) {
			continue
		}
		return r.BoruErrorHint("extend_owner",
			fmt.Sprintf("%s %s: a core word can be extended only with at least one NOMINAL argument type the extending scope owns per signature — %s has none",
				word, name, sigTupleString(&sigs[i])),
			word,
			"anchor the signature with a type this scope mints (refine / class), or pick a name that is not already a core word; content-based types (predicates, unions, member types) cannot anchor — their members can exist before the type does")
	}
	return nil
}

// sigTupleString renders a signature's argument tuple as `[Integer
// String]` for the locked/conflict error messages.
func sigTupleString(s *Signature) string {
	out := "["
	for i := 0; i < s.TotalArgs(); i++ {
		if i > 0 {
			out += " "
		}
		t := SigArgType(s, i)
		if t == nil {
			t = TAny
		}
		out += t.Leaf()
	}
	return out + "]"
}

// NewWordExtension builds a HOST-authored word-extension clone — the
// shape a Go module builder exports so `import` transplants extra
// overloads onto a core word (boru:time-util's temporal add/sub). Each
// signature is normalised (Args→Params) and barrier-resolved at
// construction, so the clone dispatches identically whether it is
// reached as a VALUE (namespaced `TimeUtil.add …` dot-access, which
// compiles the authored sigs directly) or merged into the importer's
// base word by TransplantExtension.
func NewWordExtension(owner, name string, sigs []Signature) FnDefInfo {
	compiled := make([]Signature, len(sigs))
	for i := range sigs {
		s := sigs[i]
		NormalizeSig(&s)
		if s.BarrierPos == BarrierAllForward {
			s.BarrierPos = s.TotalArgs()
		}
		compiled[i] = s
	}
	SortSignatures(compiled)
	return FnDefInfo{
		Name:           name,
		Signatures:     compiled,
		MaxForwardArgs: calcMaxForwardArgs(compiled),
		Extends:        name,
		// ExtOwner is a HOST assertion (Go-only — source clones never
		// set it): the author owner the transplant admission verifies
		// anchors against. A kernel-shipped module may author sigs on
		// kernel-owned types (boru:io's Pathon list/remove) by passing
		// OwnerKernel; third-party hosts pass their own owner id and
		// anchor on types they registered.
		ExtOwner: owner,
	}
}

// InstallWordExtension performs the def-merge for `def name fn […]`
// where name resolves to a locked-bearing word: it constructs the word
// clone — the base word's full dispatch list merged with ext's
// overloads, compiled through the same pipeline InstallFnDef uses (body
// runner, ReturnsFn, barrier resolution), so an added signature
// dispatches exactly like an installed fn's — and binds it through the
// ordinary DefTable shadow stack. Scope (fn body / module body / top
// level), `undef`, and closure capture all fall out of the def
// machinery. Sealed words decline.
func InstallWordExtension(r *Registry, name string, ext FnDefInfo) error {
	if IsSealedWord(name) {
		return r.BoruError("reserved_word",
			fmt.Sprintf("def %s: '%s' is a sealed word — the engine relies on its identity and it cannot be extended", name, name), "def")
	}
	base := r.Lookup(name)
	if base == nil {
		return r.BoruError("def_error",
			fmt.Sprintf("def %s: no existing word to extend", name), "def")
	}
	compiled := compileFnSigs(r, name, ext, false)
	// Ownership-anchored admission (R1, design/OPEN-WORDS.1.md §2):
	// extending a CORE word — from ANY scope, top level included —
	// requires every added signature to be anchored by a nominal type
	// this scope owns. Enforced at def time so the author sees the
	// compile failure immediately. Module-provided words (wrapper rebindings —
	// locked but not builtin here) are exempt: they are versioned with
	// the module dependency that owns them.
	if r.IsBuiltinWord(name) {
		if err := requireOwnedAnchor(r, name, "def", compiled, r.Types.MintOwner, false); err != nil {
			return err
		}
	}
	merged, _, err := mergeExtensionSigs(r, name, base, compiled, "")
	if err != nil {
		return err
	}
	clone := FnDefInfo{
		Name:       name,
		Signatures: merged,
		Captured:   ext.Captured,
		Extends:    name,
	}
	// Registry stays nil — the clone LIVES in r (the registry the def
	// ran in), and nil means "this registry" everywhere a name lookup
	// re-resolves the value (execFnDefLiteral's foreign-registry
	// branch). Inheriting base.Registry would be wrong for a clone of
	// a module-wrapper rebinding: dot-dispatch of the exported clone
	// would then re-look the name up in the WRAPPER's sub-registry,
	// where only the un-merged inner native exists, silently dropping
	// the merge. Per-signature execution scope is already carried by
	// each sig's compiled handler closure. resolveModuleExport tags
	// the module registry at export exactly because this is nil.
	SortSignatures(clone.Signatures)
	clone.MaxForwardArgs = calcMaxForwardArgs(clone.Signatures)
	r.noteAnalysisFnBinder(name)
	r.Defs.Push(name, NewFunction(clone))
	// A word extension installs a dispatch binding without passing through
	// installDef, so it needs its own ledger note (§6.5) — rollback-and-replay
	// has to restore it like any other.
	r.NoteBindTransition(BindDef, name, SrcPos{})
	// Construction-time body check on the ADDED overloads only — the
	// base's signatures were checked at their own construction, and
	// re-analysing them here would duplicate their diagnostics.
	added := ext
	added.Name = name
	added.Signatures = compiled
	r.analysisFnConstructionPass(name, added)
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
	return nil
}

// TransplantExtension installs an exported word-extension clone's
// unlocked signatures into the importing registry r as an implicit
// top-level def of the base word (§2.4). The incoming handlers stay
// closed over the module's sub-registry, so module-private helpers
// keep resolving (the module-closure rule). One level only: the
// transplant merges into r's CURRENT view of the base word and stops —
// re-export is what propagates further (the exporting module's clone
// then carries the sigs, and origin becomes that module: it takes
// ownership). Idempotent per origin; returns without installing when
// nothing new arrives.
func TransplantExtension(r *Registry, ext FnDefInfo, origin, owner string) error {
	name := ext.Extends
	// The same sealed-word rejection InstallWordExtension applies for
	// source-level `def`: a host/native module can construct a clone
	// directly (NewWordExtension) and import reaches the transplant
	// without passing through InstallWordExtension, so the guard must
	// hold here too — the engine relies on these words' identity.
	if IsSealedWord(name) {
		return r.BoruError("reserved_word",
			fmt.Sprintf("import: '%s' is a sealed word — the engine relies on its identity and it cannot be extended", name), "import")
	}
	base := r.Lookup(name)
	if base == nil {
		// §4.9: the base name doesn't resolve in the importer —
		// degrade to the plain namespaced binding (no transplant).
		return nil
	}
	incoming := make([]Signature, 0, len(ext.Signatures))
	for i := range ext.Signatures {
		s := ext.Signatures[i]
		if s.Locked || s.Fallback || s.CoreDefault {
			// CoreDefault overloads (the Micron field-wise default) are the
			// kernel's, not the module's: they must not ride a word
			// extension into an importer (their builtin-only tuple would
			// trip the module-scope user-type rule, and the importer already
			// carries its own copy from core registration).
			continue
		}
		// Normalise host-authored clones: a Go module builder writes the
		// Args/BarrierPos constructor-convenience form (e.g. boru:time-util's
		// arithmetic extensions), while a def-merge clone arrives already
		// compiled. normalizeSig is idempotent, and the sentinel resolution
		// mirrors upsertFnDef's so the sig dispatches like any registered one.
		NormalizeSig(&s)
		if s.BarrierPos == BarrierAllForward {
			s.BarrierPos = s.TotalArgs()
		}
		incoming = append(incoming, s)
	}
	if len(incoming) == 0 {
		return nil
	}
	// Defence in depth for R1: the merge that built the exported clone
	// already verified its anchors, but a transplant onto a CORE word
	// re-checks so a hand-built clone can't smuggle an unanchored tuple
	// past the importer. The author is the extension's declared host
	// owner (ExtOwner — how a kernel-shipped module authors sigs on
	// kernel-owned types like Pathon) or, for source clones, the
	// exporting module's own mint owner.
	if r.IsBuiltinWord(name) {
		author := ext.ExtOwner
		if author == "" {
			author = owner
		}
		// Source clones (no ExtOwner) may carry re-exported sigs whose
		// anchors belong to a module deeper in the import chain.
		if err := requireOwnedAnchor(r, name, "import", incoming, author, ext.ExtOwner == ""); err != nil {
			return err
		}
	}
	merged, changed, err := mergeExtensionSigs(r, name, base, incoming, origin)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	clone := FnDefInfo{
		Name:       name,
		Signatures: merged,
		// Registry nil for the same reason as InstallWordExtension's
		// clone: the binding lives in r, and each incoming sig's
		// handler already closes over its module sub-registry.
		Extends: name,
	}
	SortSignatures(clone.Signatures)
	clone.MaxForwardArgs = calcMaxForwardArgs(clone.Signatures)
	r.Defs.Push(name, NewFunction(clone))
	// A word extension installs a dispatch binding without passing through
	// installDef, so it needs its own ledger note (§6.5) — rollback-and-replay
	// has to restore it like any other.
	r.NoteBindTransition(BindDef, name, SrcPos{})
	if r.ready && r.OnRegisterHook != nil {
		r.OnRegisterHook(name)
	}
	return nil
}
