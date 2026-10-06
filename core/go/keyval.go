package core

// TKeyVal is the Node/Map/KeyVal type — the entry value a map-iteration body
// receives (the `e` in `m each ([e:KeyVal] => …)`). It is a child of Map, so
// every map word works on it and it renders as a map; it carries exactly four
// fields:
//
//	k : String    the entry's key
//	v : Any       the entry's value
//	i : Integer   the current index in the key iteration (0-based)
//	n : Integer   the total number of keys being iterated
//
// Being a Map subtype means `e.k` / `e.v` / `e.i` / `e.n` read with ordinary
// dotted access, `e is Map` is true, and `typeof e` is `KeyVal`. Matching is
// nominal (DefaultBehavior): a plain `{k:… v:… i:… n:…}` map is NOT a KeyVal —
// only a value the constructor tags is — so the type reliably marks "this came
// from map iteration".
//
// FixedID 5002 is the next free id in the 5000–9999 kernel/language band,
// after Module (5000) and the retired ModuleExport wrapper (5001,
// never recycled — NUR038). See CLAUDE.md "FixedID Allocation".
//
// KERNEL-RESIDENT (ADR-012 rule 1): the compiled-callback pair shape
// is dispatch mechanics — keyValCarrier (callable_words.go) types the
// compiled each/filter/fold/scan map callbacks with it, and NewKeyVal
// is the one constructor the map-iteration words use. Moved in from
// lang/go/native/native_keyval.go, FixedID preserved.
var TKeyVal = mustType("Node/Map/KeyVal")

// KeyVal field names. A KeyVal map carries exactly these keys, in this order.
const (
	KeyValK = "k" // key
	KeyValV = "v" // value
	KeyValI = "i" // 0-based iteration index
	KeyValN = "n" // total number of keys
)

// NewKeyVal builds a KeyVal entry for key k with value v, at iteration index i
// of n total keys. The result is a Map tagged Node/Map/KeyVal — it is at once a
// KeyVal and a Map. This is the single constructor the map-iteration words use
// to present each entry to a body.
func NewKeyVal(k string, v Value, i, n int64) Value {
	om := NewOrderedMap()
	om.Set(KeyValK, NewString(k))
	om.Set(KeyValV, v)
	om.Set(KeyValI, NewInteger(i))
	om.Set(KeyValN, NewInteger(n))
	return NewValueRaw(TKeyVal, MapPayload{M: om})
}

// CallbackWantsKeyVal reports whether fn — a Function value handed to a
// map-iteration word (each / for-each / fold / scan / filter over a Map) —
// declares its ENTRY param as a KeyVal. The entry is the LAST param in every
// map callback convention (`[kv]` for each and filter, `[acc kv]` for fold
// and scan), and a Function form hands the entry's VALUE unless the callback
// asks for the whole entry this way: `each ([v:Integer] => [v mul 2]) m` sees
// 1, 2, …; `each ([kv:KeyVal] => [kv.k]) m` sees {k v i n}. The decision is
// the SIGNATURE's, so a lambda, a named fn value and a compiled closure's
// bridged signature (ClosureAsFnDef) all answer alike; a fn with several
// signatures wants the entry when any of them declares it. A value that is no
// FnDefInfo (a token quotation, a closure met outside a VM run) wants the
// value.
func CallbackWantsKeyVal(fn Value) bool {
	fd, ok := fn.Data.(FnDefInfo)
	if !ok {
		return false
	}
	for _, sig := range fd.OwnSigs() {
		n := len(sig.Params)
		if n == 0 {
			continue
		}
		if t := sig.Params[n-1].Type; t != nil && t.ConformsTo(TKeyVal) {
			return true
		}
	}
	return false
}
