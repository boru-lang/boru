package core

// StoreShapeInfo's state methods, moved beside their type at the core
// cut (Stage 4g): the shape RECORD and its accessors are core-owned
// state; the check piece's store-shape ANALYSIS stays above.

// RecordKey joins a written value carrier into the shape under key —
// the per-store twin of CheckState.RecordContextSet, same join
// semantics. Join-only: repeat writes widen, never replace (replace-
// on-write is the stage-4 flow-sensitivity step; join keeps every
// sandbox/branch interaction monotone).
func (s *StoreShapeInfo) RecordKey(key string, v Value) {
	if s == nil || key == "" || s.KeysPoisoned {
		return
	}
	if s.KeyTypes == nil {
		s.KeyTypes = map[string]Value{}
	}
	if existing, ok := s.KeyTypes[key]; ok {
		s.KeyTypes[key] = JoinCarriers(existing, v)
		return
	}
	s.KeyTypes[key] = v
}

// LookupKey returns the joined carrier recorded for key through THIS
// store, or ok=false when this store never saw the key written (the
// caller then falls back to the flat map — today's behaviour).
func (s *StoreShapeInfo) LookupKey(key string) (Value, bool) {
	if s == nil || s.KeysPoisoned {
		return Value{}, false
	}
	v, ok := s.KeyTypes[key]
	return v, ok
}

// RecordVal joins an unkeyed value write (a patrun `add`) into Vals. A
// value the shape cannot describe honestly — a dispatch-bearing stored
// Function/FnDef/Reach/Splice, whose reader re-dispatches it — poisons
// the join instead, and LookupVals declines from then on.
func (s *StoreShapeInfo) RecordVal(v Value) {
	if s == nil || s.ValsPoisoned {
		return
	}
	if v.Parent == nil || v.Parent.ConformsTo(TFunction) ||
		IsReach(v) || IsSplice(v) {
		s.ValsPoisoned = true
		s.Vals = Value{}
		return
	}
	if s.Vals.Parent == nil {
		s.Vals = v
		return
	}
	s.Vals = JoinCarriers(s.Vals, v)
}

// Poison marks every claim of the shape unusable, keyed and unkeyed alike:
// the container reached a writer the shape cannot see — a user fn it was
// passed to, which may write through its parameter's alias (NUR315: `poke
// fl drop def j (fl get 0) j`, where poke sets a fn at 0, read the stale
// Integer claim and answered `fn j` for the interpreter's 42). Readers keep
// the dynamic(Any) hatch from then on; a declared element type (a typed
// patrun's) is a declaration, not a claim, and stands.
func (s *StoreShapeInfo) Poison() {
	if s == nil {
		return
	}
	s.KeysPoisoned, s.KeyTypes = true, nil
	s.ValsPoisoned, s.Vals = true, Value{}
}

// LookupVals returns the unkeyed value join, or ok=false when nothing
// was recorded or the join is poisoned.
func (s *StoreShapeInfo) LookupVals() (Value, bool) {
	if s == nil || s.ValsPoisoned || s.Vals.Parent == nil {
		return Value{}, false
	}
	return s.Vals, true
}

// CloneShape returns a shape with a COPIED KeyTypes map (entries — and
// any nested shape pointers inside them — still shared). Used where the
// RUNTIME disconnects aliasing with a deep copy (`flex` of a flex): the
// copy's later writes must not narrow reads through the original's
// claims and vice versa. Entry-level sharing is safe because all
// mutation is join-only.
func (s *StoreShapeInfo) CloneShape() *StoreShapeInfo {
	if s == nil {
		return nil
	}
	cp := &StoreShapeInfo{Scope: s.Scope, Vals: s.Vals, ValsPoisoned: s.ValsPoisoned, KeysPoisoned: s.KeysPoisoned, DeclaredVal: s.DeclaredVal}
	if s.KeyTypes != nil {
		cp.KeyTypes = make(map[string]Value, len(s.KeyTypes))
		for k, v := range s.KeyTypes {
			cp.KeyTypes[k] = v
		}
	}
	return cp
}
