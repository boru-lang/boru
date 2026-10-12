package core

import "strings"

// listFormatBehavior renders a List value, dispatching on the
// internal payload sub-shape (TableTypeInfo, TableData,
// MaterializerPayload, ChildTypeInfo, plain ListPayload). Moved
// from Value.String at Step 10 — the kernel's render switch no
// longer carries TList sub-payload logic; the Behavior owns it.
type listFormatBehavior struct{}

func (listFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (listFormatBehavior) matchesAsDefault()           {}
func (listFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (listFormatBehavior) Format(v Value) string {
	if tt, ok := v.Data.(TableTypeInfo); ok {
		return formatFieldBag("table", tt.Record.Fields)
	}
	if td, ok := v.Data.(TableData); ok {
		rowParts := make([]string, len(td.Rows))
		for i, row := range td.Rows {
			rowParts[i] = row.String()
		}
		return formatFieldBag("table", td.Record.Fields) + "[" + strings.Join(rowParts, " ") + "]"
	}
	if mp, ok := v.Data.(MaterializerPayload); ok {
		td, err := mp.M.Materialize()
		if err != nil {
			return "query(error:" + err.Error() + ")"
		}
		return formatTableDataAsList(td)
	}
	if mz, ok := v.Data.(Materializer); ok {
		td, err := mz.Materialize()
		if err != nil {
			return "query(error:" + err.Error() + ")"
		}
		return formatTableDataAsList(td)
	}
	if ct, ok := v.Data.(ChildTypeInfo); ok {
		// Include any inline elements (`[:Integer 1 2 3]`) — dropping
		// them made String lossy where canon keeps them (found by the
		// TS parser stream-parity diff).
		parts := make([]string, 0, len(ct.Elements)+1)
		parts = append(parts, ":"+ct.Child.String())
		for _, e := range ct.Elements {
			parts = append(parts, e.String())
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	_lst, _ := AsList(v)
	elems := _lst.Slice()
	parts := make([]string, 0, len(elems))
	for i := 0; i < len(elems); i++ {
		// Display-only: collapse a parser-grouped dot chain
		// `( recv get k … )` back to `recv.k.…` for readable quoted code.
		if s, end, ok := resugarDotChain(elems, i); ok {
			parts = append(parts, s)
			i = end
			continue
		}
		parts = append(parts, elems[i].String())
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// resugarDotChain recognises a parser-grouped dot-access run inside a
// quoted list — `( recv get k1 [get|getr k2]* )` where the receiver and
// every key is a bare Word — and renders it as `recv.k1.k2` (`!.` for
// getr). It returns the rendered string, the index of the run's closing
// paren, and whether a chain matched.
//
// Display-only: this is reached via Value.String (REPL / inspect output),
// NOT CanonValue — which has its own list rendering and doubles as the
// value-comparison key, so it is deliberately left untouched. Only the
// all-bare-Word shape is re-sugared; nested-group receivers (`(expr).k`)
// and computed `(expr)` keys (`m.(expr)`) render verbatim.
func resugarDotChain(elems []Value, i int) (string, int, bool) {
	if !IsOpenParen(&elems[i]) || i+1 >= len(elems) || !IsWord(&elems[i+1]) {
		return "", 0, false
	}
	recv, _ := AsWord(&elems[i+1])
	var b strings.Builder
	b.WriteString(recv.Name)
	j := i + 2
	for j+1 < len(elems) && IsWord(&elems[j]) {
		verb, _ := AsWord(&elems[j])
		if !IsGetWord(verb.Name) && !IsGetrWord(verb.Name) {
			break
		}
		if !IsWord(&elems[j+1]) {
			return "", 0, false // computed / non-word key — leave verbatim
		}
		key, _ := AsWord(&elems[j+1])
		if IsGetrWord(verb.Name) {
			b.WriteString("!.")
		} else {
			b.WriteString(".")
		}
		b.WriteString(key.Name)
		j += 2
	}
	if j == i+2 || j >= len(elems) || !IsCloseParen(&elems[j]) {
		return "", 0, false // no segment consumed, or no clean close
	}
	return b.String(), j, true
}

// formatTableDataAsList renders TableData by re-wrapping it in a
// transient List value and calling String — preserving the
// historical render path for materialized query results.
func formatTableDataAsList(td TableData) string {
	v := NewValueRaw(TList, td)
	return v.String()
}

// joinEntries renders m's entries as "k:render(v)" joined by spaces —
// the ONE k:v body every field-bag and map render shares (Value.String
// for display, CanonValue for canon, class/resource instance field
// bags). Keys iterate in insertion order; commas are optional in boru
// source and the default render omits them.
func joinEntries(m ReadMap, render func(Value) string) string {
	parts := make([]string, 0, m.Len())
	for _, k := range m.Keys() {
		val, _ := m.Get(k)
		parts = append(parts, k+":"+render(val))
	}
	return strings.Join(parts, " ")
}

// formatFieldBag renders an OrderedMap of field name → value as
// `name{k:v …}` — the shared render shape of record / options /
// table-schema type values.
func formatFieldBag(name string, fields *OrderedMap) string {
	return name + "{" + joinEntries(fields, Value.String) + "}"
}

// mapFormatBehavior renders a Map value, dispatching on the
// internal payload sub-shape (ChildTypeInfo, RecordTypeInfo,
// OptionsTypeInfo, plain MapPayload). Moved from Value.String at
// Step 10.
type mapFormatBehavior struct{}

func (mapFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (mapFormatBehavior) matchesAsDefault()           {}
func (mapFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (mapFormatBehavior) Format(v Value) string {
	if ct, ok := v.Data.(ChildTypeInfo); ok {
		// Include any inline entries (`{:Integer a:1}`) — the map twin
		// of the typed-list element render above.
		parts := make([]string, 0, len(ct.Entries)+1)
		parts = append(parts, ":"+ct.Child.String())
		for _, e := range ct.Entries {
			parts = append(parts, e.Key+":"+e.Value.String())
		}
		return "{" + strings.Join(parts, " ") + "}"
	}
	if rt, ok := v.Data.(RecordTypeInfo); ok {
		return formatFieldBag("record", rt.Fields)
	}
	if ot, ok := v.Data.(OptionsTypeInfo); ok {
		return formatFieldBag("options", ot.Fields)
	}
	m, _ := AsMap(v)
	if m == nil {
		return "{}"
	}
	// A module NAMESPACE (facet-carrying Map) renders compactly as its
	// export-key summary: the values are typically whole fn definitions
	// (and, transitively, other namespaces), so the plain k:v body would
	// spill entire module APIs into stack traces and REPL echoes.
	if ns := ModuleNSOf(v); ns != nil {
		return "Module(" + ns.Name + "){" + strings.Join(m.Keys(), " ") + "}"
	}
	return "{" + joinEntries(m, Value.String) + "}"
}

func init() {
	TList.ensureTMeta().Behavior = listFormatBehavior{}
	TMap.ensureTMeta().Behavior = mapFormatBehavior{}
}
