package core

import (
	"sort"
	"strconv"
	"strings"
)

// This file gives DeepEqual-based scans a sub-quadratic shape. The
// collection words (unique / member / indices / group) need "find a
// deq-equal value among N seen so far" — a nested DeepEqual scan makes
// that O(N·M). DeqKey assigns each value a bucket such that deq-equal
// values always land in the same bucket, so a scan only has to compare
// within one bucket.
//
// The key is NOT an equality: two values with the same key may be
// deq-unequal (NaN shares the "NaN" bucket with itself; a float64
// projection can collide). Callers must still confirm candidates with
// DeepEqual. The contract is one-directional, and it mirrors
// DeepEqual's dispatch branch by branch:
//
//	DeepEqual(a, b) == true  ⇒  DeqKey(a) and DeqKey(b) return either
//	  - DeqKeyed with identical keys, or
//	  - DeqUnkeyed on at least one side.
//
// DeqUnkeyed marks the container shapes DeepEqual compares by RENDER
// when structural access fails (typed lists / tables / records — the
// `a.String() == b.String()` fallback arms): a plain container can
// deq-match one of those across the plain/typed boundary, so no
// per-value key is sound and the value must be scanned pairwise.
// Containers propagate unkeyedness up from their children.
//
// DeqNeverEqual marks values that reach DeepEqual's unsupported
// fall-through: deq-equal to nothing, including themselves, so scans
// can skip them entirely. NUR031 emptied this class of everything a
// user can name — functions and words are keyed by canon, declared
// type values by their type identity, sealed host payloads scanned by
// box identity — leaving the internal shapes and the instances with no
// readable fields.

// DeqKeyClass classifies how a value participates in bucketed deq
// scans — see the contract above.
type DeqKeyClass int

const (
	// DeqKeyed — the key is a sound bucket for this value.
	DeqKeyed DeqKeyClass = iota
	// DeqUnkeyed — no sound key; scan pairwise within the family.
	DeqUnkeyed
	// DeqNeverEqual — deq-equal to nothing (including itself).
	DeqNeverEqual
)

// deqKeyMaxDepth caps DeqKey's structural recursion. A self-referential
// flex container would otherwise recurse forever computing its own key
// — where the pairwise scans it replaces only recurse when a comparison
// actually happens. Past the cap the subtree is DeqUnkeyed, which is
// always sound (it just falls back to the pairwise scan).
const deqKeyMaxDepth = 64

// DeqKey returns the deq bucket key for v. See the contract at the top
// of this file; the branch structure deliberately mirrors DeepEqual's.
func DeqKey(v Value) (string, DeqKeyClass) {
	return deqKeyAtDepth(v, 0)
}

func deqKeyAtDepth(v Value, depth int) (string, DeqKeyClass) {
	if depth > deqKeyMaxDepth {
		return "", DeqUnkeyed
	}

	// none — DeepEqual's both-None branch.
	if ValueType(v).Equal(TNone) {
		return "N", DeqKeyed
	}

	// A bare type literal is not a concrete value: it is deq-equal only
	// to another literal of the SAME type (NUR032 for the numeric
	// families; the scalar families already agree). Bucket by type
	// identity BEFORE the numeric branch so `Integer` never shares 0's
	// bucket. Two container literals (`List`, `Map`) whose renders match
	// still land here and are resolved pairwise by DeepEqual (whose
	// NUR034 type-literal arm answers `List deq List` true and
	// `List deq Map` false), so the bucket stays a sound filter.
	if IsBareTypeNode(v) {
		return "tlit:" + ValueType(v).ID, DeqKeyed
	}

	// DepScalar — equal only to a DepScalar of the identical Parent.
	if v.IsDepScalar() {
		return "D:" + v.Parent.ID, DeqKeyed
	}

	// Numbers — scalarFamilyEqual compares int64-exact (int/int),
	// float64-projected (a Float on either side), or rat-exact (a Big
	// on either side). All three agree that equal values share the
	// same real number, so the correctly-rounded float64 projection of
	// that number is a sound shared bucket: equal int64s project
	// identically, and a Big rat-equal to any operand projects to the
	// same float64 as that operand. Collisions (2^53 vs 2^53+1) are
	// disambiguated pairwise. The error-ignoring AsNumber mirrors
	// scalarFamilyEqual's own accessor use, so whatever projection the
	// equality sees, the key sees too.
	if v.Parent.ConformsTo(TNumber) {
		var f float64
		if numIsBig(v) {
			var err error
			f, err = AsFloatApprox(v)
			if err != nil {
				// toRatExact fails on the same accessor error, so the
				// value is deq-equal to nothing.
				return "", DeqNeverEqual
			}
		} else {
			f, _ = AsNumber(v)
		}
		if f == 0 {
			f = 0 // fold -0.0 into +0.0 — they compare equal
		}
		return "n:" + strconv.FormatFloat(f, 'g', -1, 64), DeqKeyed
	}

	// The by-value scalar families — same error-ignoring accessors as
	// scalarFamilyEqual.
	if v.Parent.ConformsTo(TString) {
		s, _ := AsString(v)
		return "s:" + s, DeqKeyed
	}
	if v.Parent.ConformsTo(TBoolean) {
		b, _ := AsBoolean(v)
		return "b:" + strconv.FormatBool(b), DeqKeyed
	}
	if v.Parent.ConformsTo(TAtom) {
		a, _ := AsAtom(v)
		return "a:" + a, DeqKeyed
	}

	// Remaining scalars — scalarSemanticEqual equates values only when
	// their Parents coincide or one Parent is the other's ancestor, so
	// any equal pair shares its topmost family node under Scalar. One
	// bucket per family (Time, Micron, …) is sound; members are
	// disambiguated pairwise.
	if v.Parent.ConformsTo(TScalar) {
		return "c:" + scalarFamilyRoot(v.Parent).ID, DeqKeyed
	}

	// Lists — structural recursion over element VALUES (populated typed
	// lists included, via deqListElems; NUR033). DeqUnkeyed remains only
	// for a list carrier — a type-level operand DeepEqual still compares
	// by render, so it must be scanned pairwise within its family.
	if containerFamily(v.Parent).Equal(TList) {
		elems, ok := deqListElems(v)
		if !ok {
			return "", DeqUnkeyed
		}
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range elems {
			ek, ec := deqKeyAtDepth(e, depth+1)
			if ec != DeqKeyed {
				// An unkeyed child (or a never-equal one, whose pair
				// could still render-match a typed list) makes the
				// whole container render-reachable — no sound key.
				return "", DeqUnkeyed
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(ek)
		}
		b.WriteByte(']')
		return b.String(), DeqKeyed
	}

	// Maps — key-order-insensitive structural recursion over entry
	// VALUES (populated typed maps included, via deqMapEntries; NUR033).
	// DeqUnkeyed remains only for a map carrier or a Record/Options type
	// constructor, which DeepEqual still compares by render.
	if containerFamily(v.Parent).Equal(TMap) {
		m, ok := deqMapEntries(v)
		if !ok {
			return "", DeqUnkeyed
		}
		keys := append([]string(nil), m.Keys()...)
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			mv, _ := m.Get(k)
			vk, vc := deqKeyAtDepth(mv, depth+1)
			if vc != DeqKeyed {
				return "", DeqUnkeyed
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(k))
			b.WriteByte('=')
			b.WriteString(vk)
		}
		b.WriteByte('}')
		return b.String(), DeqKeyed
	}

	// XML — xmlElementsEqual requires equal tags, so the tag is a
	// sound bucket; attribute/child equality is confirmed pairwise.
	if tag, _, _, ok := xmlParts(v); ok {
		return "x:" + tag, DeqKeyed
	}

	// Flat instances — deq requires the SAME exact Parent and
	// field-wise deep equality over identical present-field sets, and
	// there is no render fallback for instances, so a never-equal
	// field makes the instance itself never-equal.
	if fields, _, ok := FlatInstanceParts(v); ok {
		if fields == nil {
			return "", DeqNeverEqual
		}
		keys := append([]string(nil), fields.Keys()...)
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("i:")
		b.WriteString(v.Parent.ID)
		b.WriteByte('{')
		for i, k := range keys {
			fv, _ := fields.Get(k)
			fk, fc := deqKeyAtDepth(fv, depth+1)
			switch fc {
			case DeqNeverEqual:
				return "", DeqNeverEqual
			case DeqUnkeyed:
				return "", DeqUnkeyed
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(k))
			b.WriteByte('=')
			b.WriteString(fk)
		}
		b.WriteByte('}')
		return b.String(), DeqKeyed
	}

	// NUR031: an opaque Ideal handle that is now deq-comparable pairwise
	// (Store by entries, Error by fields, a timer or Module descriptor
	// by identity). No sound per-value key, so scan it within its handle
	// family.
	if isDeqComparableHandle(v) {
		return "", DeqUnkeyed
	}

	// A type that installed the DeepEqualer capability answers pairwise
	// for values DeepEqual would otherwise reject at its terminal arm —
	// so those values are scannable, not never-equal. Without this arm
	// the index diverges from DeepEqual: Add files the value nowhere,
	// FirstMatch answers -1, and `unique` keeps what `deq` calls a
	// duplicate. deqFam gives the same values a per-owner scan family.
	if DeepEqualerOwner(ValueType(v)) != nil {
		return "", DeqUnkeyed
	}

	// A sealed HOST payload with no capability (NUR031): compared by
	// POINTER identity, which is not a per-value key the kernel may
	// compute without reading the payload — so scan pairwise within the
	// host family. Below the capability arm for the same reason deqFam's
	// twin branch is; restricted to pointer bodies for the same reason
	// hostPayloadIdentity is, so a contents-only payload stays never-equal
	// rather than being scanned for a match it can never have.
	if p, ok := v.Data.(ExtensionPayload); ok && isPointerBody(p.Body) {
		return "", DeqUnkeyed
	}

	// Functions (NUR031): deq is CONTENT equality, compared as canon —
	// which NUR059 and NUR031 made name-independent — so canon is itself
	// a sound bucket. The defining registry discriminates too
	// (fnStructurallyEqual rejects equal canon from different modules),
	// which only makes this bucket COARSER than equality, and coarser is
	// what the contract allows: DeepEqual confirms each candidate.
	//
	// Classifying functions DeqNeverEqual — as this arm did while deq
	// rejected them — is what the deq change would otherwise have left
	// stale, and the divergence is silent: Add files the value nowhere
	// and FirstMatch answers -1, so `unique` keeps two functions `deq`
	// calls duplicates.
	if fd, ok := v.Data.(FnDefInfo); ok {
		return "f:" + canonFnDef(fd), DeqKeyed
	}

	// Words (NUR031): a word is value-like — eq and deq both compare the
	// whole WordInfo — and canon renders every field of it, name and
	// modifiers alike, so canon is an exact key here rather than merely a
	// sound one.
	if _, ok := v.Data.(WordInfo); ok && v.Parent != nil {
		return "w:" + v.Parent.ID + ":" + CanonValue(v), DeqKeyed
	}

	// Declared type VALUES (NUR031): deq shares eq's type-body arm, which
	// requires the SAME nominal type, so the type's own identity is a
	// sound bucket — coarser than equality, and DeepEqual confirms each
	// candidate. The BARE literals never reach here: the type-identity
	// arm near the top already keys them, with the same discriminator.
	//
	// No deqFam entry to match. A declared type is deq only to another
	// value of its own type, and the unkeyed residents (render-matched
	// containers, handles, capability-owned values) are none of those, so
	// there is nothing for a family to keep reachable — while an
	// inaccurate one could hide a render-matched type-level operand from
	// the scan.
	if IsTypeBody(v) && v.Parent != nil {
		return "t:" + v.Parent.ID, DeqKeyed
	}

	// DeepEqual's unsupported fall-through. NUR031 emptied this class of
	// everything a user can name — the code values, the type values and
	// the host payloads all found an answer above — leaving the internal
	// shapes (a carrier, a forward/move marker) and the instances with no
	// readable fields: deq-equal to nothing, including themselves.
	return "", DeqNeverEqual
}

// scalarFamilyRoot walks up to the child-of-Scalar ancestor — the node
// scalarSemanticEqual's Parent/LCA rules guarantee equal values share.
func scalarFamilyRoot(t *Type) *Type {
	for t.Parent != nil && !t.Parent.Equal(TScalar) && !t.Equal(TScalar) {
		t = t.Parent
	}
	return t
}

// deqFam names the cross-scan family a DeqUnkeyed value can deq-match
// within: the render fallbacks only fire list-vs-list and map-vs-map,
// and instance equality only fires within one exact class. Values in
// families that can never contain an unkeyed member return "".
func deqFam(v Value) string {
	if nodeFamily(v.Parent).Equal(TList) {
		return "L"
	}
	if nodeFamily(v.Parent).Equal(TMap) {
		return "M"
	}
	if _, _, ok := FlatInstanceParts(v); ok {
		return "I:" + v.Parent.ID
	}
	// NUR031 handles scan within one family per PAYLOAD KIND, NOT per
	// type ID: a timer's deq is pointer identity, which ignores Parent,
	// so a timer reparented to a `refine` subtype (same handle pointer)
	// is deq to its base and MUST share its scan family. Keying by kind
	// (rather than ValueType) also stays sound for Store/Error, whose
	// deq requires the same exact type — DeepEqual settles those pairs
	// within the one family. A Store never deq-matches an Error, so the
	// per-kind split keeps the scan tight.
	if k := handleKind(v); k != "" {
		return "H:" + k
	}
	// Functions scan among themselves (NUR031): a fn is deq only to
	// another fn, so its own family keeps the scan tight and stops the
	// depth-capped unkeyed values (the only other residents of the
	// empty family) from being rescanned on every fn lookup.
	if _, ok := v.Data.(FnDefInfo); ok {
		return "F"
	}
	// Capability-carrying values scan within the family of the OWNING
	// node — the nearest ancestor implementing DeepEqualer. Sound and
	// tight: DeepEqual consults the capability via the LCA walk, and two
	// values whose nearest owners differ have an LCA strictly above
	// both, where no DeepEqualer exists — so they can never match and
	// need never share a scan family. Without a family here the
	// DeqUnkeyed classification above is inert (Add files unkeyed
	// entries by family; FirstMatch scans allByFam only).
	if owner := DeepEqualerOwner(ValueType(v)); owner != nil {
		return "DE:" + owner.ID
	}
	// A sealed HOST payload (NUR031): compared by box identity, so every
	// one of them scans in one family. BELOW the capability arm on
	// purpose — a host type that installed a DeepEqualer keeps its own,
	// tighter family, and only the payloads with no capability land here.
	if p, ok := v.Data.(ExtensionPayload); ok && isPointerBody(p.Body) {
		return "H:Host"
	}
	return ""
}

// handleKind returns the payload-kind tag of a deq-comparable opaque
// handle (Store, Error, Timeout, Interval, the Module descriptor), or
// "" for anything else. Parent-independent by design — see deqFam.
func handleKind(v Value) string {
	switch d := v.Data.(type) {
	case *StoreInstanceInfo:
		return "Store"
	case ErrorInfo:
		return "Error"
	case *TimeoutInfo:
		return "Timeout"
	case *IntervalInfo:
		return "Interval"
	case ExtensionPayload:
		// Only the Ideal/Module DESCRIPTOR (a boxed *ModuleDesc) is a
		// handle the kernel compares HERE. A sealed host payload is
		// deq-comparable too since NUR031 (box identity), but it is
		// classified further down — below the DeepEqualer arm, so a host
		// type that installed the capability keeps answering for its own
		// values instead of being pre-empted by the fallback.
		if _, ok := d.Body.(*ModuleDesc); ok {
			return "Module"
		}
	}
	return ""
}

// DeqIndex answers "which of the values added so far is deq-equal to
// this one?" in near-linear total time. Values are held in insertion
// order; FirstMatch returns the position of the earliest deq-equal
// entry, exactly as a linear DeepEqual scan would, but touching only
// the candidate's bucket plus the (rare) unkeyed values of its family.
type DeqIndex struct {
	vals         []Value
	buckets      map[string][]int // DeqKeyed entries by key
	unkeyedByFam map[string][]int // DeqUnkeyed entries by family
	allByFam     map[string][]int // every keyed+unkeyed entry of a fam
}

// Add appends v and returns its position.
func (x *DeqIndex) Add(v Value) int {
	idx := len(x.vals)
	x.vals = append(x.vals, v)
	key, class := DeqKey(v)
	if class == DeqNeverEqual {
		return idx // matches nothing — no lookup structure needed
	}
	fam := deqFam(v)
	if x.buckets == nil {
		x.buckets = map[string][]int{}
		x.unkeyedByFam = map[string][]int{}
		x.allByFam = map[string][]int{}
	}
	if class == DeqKeyed {
		x.buckets[key] = append(x.buckets[key], idx)
	} else {
		x.unkeyedByFam[fam] = append(x.unkeyedByFam[fam], idx)
	}
	if fam != "" {
		x.allByFam[fam] = append(x.allByFam[fam], idx)
	}
	return idx
}

// FirstMatch returns the position of the earliest added value that is
// DeepEqual to v, or -1. It does not add v.
func (x *DeqIndex) FirstMatch(v Value) int {
	key, class := DeqKey(v)
	switch class {
	case DeqNeverEqual:
		return -1
	case DeqUnkeyed:
		// v is render-reachable: any keyed or unkeyed entry of its
		// family could match.
		for _, i := range x.allByFam[deqFam(v)] {
			if DeepEqual(x.vals[i], v) {
				return i
			}
		}
		return -1
	}
	// Keyed: the bucket, merged in insertion order with the unkeyed
	// entries of v's family (a plain container can deq-match a typed
	// one via the render fallback).
	bucket := x.buckets[key]
	unkeyed := x.unkeyedByFam[deqFam(v)]
	bi, ui := 0, 0
	for bi < len(bucket) || ui < len(unkeyed) {
		var i int
		if ui >= len(unkeyed) || (bi < len(bucket) && bucket[bi] < unkeyed[ui]) {
			i = bucket[bi]
			bi++
		} else {
			i = unkeyed[ui]
			ui++
		}
		if DeepEqual(x.vals[i], v) {
			return i
		}
	}
	return -1
}
