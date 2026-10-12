package core

import (
	"fmt"
	"strings"
)

// The `print` / `printstr` word registrations live in
// lang/go/engine/native_print.go. The handlers and FormatForPrint
// rendering algorithm stay here as exported eng primitives.

func PrintHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	out := FormatForPrint(v)
	fmt.Fprintln(r.Output, out)
	return nil, nil
}

func PrintstrHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	out := FormatForPrint(v)
	fmt.Fprint(r.Output, out)
	return nil, nil
}

// FormatForPrint returns the print representation of a value.
func FormatForPrint(v Value) string {
	// The value `none` (the unique inhabitant of None, Data != nil
	// sentinel) prints as "none".
	if IsNone(&v) {
		return "none"
	}
	// Type literals (Data==nil) — print as the leaf of the type path
	// (e.g. Integer → "Integer", List → "List"). Type names are
	// globally unique. The None type literal prints as "None".
	if v.Data == nil {
		return TypeNodeOf(v).Leaf()
	}

	// Table: formatted with headers and aligned columns.
	if IsTableType(v) {
		if td, ok := v.Data.(TableData); ok {
			return formatTable(td)
		}
		if mp, ok := v.Data.(MaterializerPayload); ok {
			td, err := mp.M.Materialize()
			if err != nil {
				return "query(error:" + err.Error() + ")"
			}
			return formatTable(td)
		}
		if mz, ok := v.Data.(Materializer); ok {
			td, err := mz.Materialize()
			if err != nil {
				return "query(error:" + err.Error() + ")"
			}
			return formatTable(td)
		}
	}

	// Error: print the error message.
	if IsError(v) {
		info, _ := AsError(v)
		return info.Message
	}

	// Dependent scalar: render the constraint, not the underlying base.
	// Must run before TString / TInteger / etc. matches because a
	// DepScalar's Parent also matches its base via the lattice override.
	if v.IsDepScalar() {
		return ValToString(v)
	}

	// String: printed as-is (no quotes).
	if v.Parent.ConformsTo(TString) {
		_as0, _ := AsString(v)
		return _as0
	}

	// Options type: use String() representation.
	if IsOptionsType(v) {
		return v.String()
	}

	// Map: JSON-like output.
	if v.Parent.Equal(TMap) {
		return formatMapJSON(v)
	}

	// List: JSON-like output.
	if v.Parent.Equal(TList) && v.Data != nil {
		return formatListJSON(v)
	}

	// Everything else: use ValToString (integers, booleans, atoms).
	return ValToString(v)
}

// formatMapJSON formats a map value as a JSON-like string.
func formatMapJSON(v Value) string {
	om, err := AsMutableMap(v)
	if err != nil {
		return "{}"
	}
	parts := make([]string, 0, om.Len())
	for _, k := range om.Keys() {
		val, _ := om.Get(k)
		parts = append(parts, fmt.Sprintf("%q: %s", k, FormatValueJSON(val)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// formatListJSON formats a list value as a JSON-like string.
func formatListJSON(v Value) string {
	elems, _ := AsList(v)
	parts := make([]string, elems.Len())
	for i, e := range elems.Elems() {
		parts[i] = FormatValueJSON(e)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// FormatValueJSON formats any value for JSON-like output.
func FormatValueJSON(v Value) string {
	// The value `none` and the None type literal both render as JSON
	// null — that's how JSON encodes the unit type / absent value.
	if IsNoneShape(v) {
		return "null"
	}
	if v.Data == nil {
		// Type literal — render the leaf, quoted as a JSON string.
		return fmt.Sprintf("%q", TypeNodeOf(v).Leaf())
	}
	// DepScalar pre-empts the ConformsTo(TString)/... dispatch so its
	// constraint payload renders via the DepScalar formatter rather
	// than crashing through AsString. Quote the form so it's a valid
	// JSON string.
	if s := renderDepScalar(v); s != "" {
		return fmt.Sprintf("%q", s)
	}
	switch {
	case v.Parent.ConformsTo(TString):
		_as1, _ := AsString(v)
		return fmt.Sprintf("%q", _as1)
	case v.Parent.ConformsTo(TInteger):
		_as2, _ := AsInteger(v)
		return fmt.Sprintf("%d", _as2)
	case v.Parent.ConformsTo(TBoolean):
		_as3, _ := AsBoolean(v)
		if _as3 {
			return "true"
		}
		return "false"
	case v.Parent.Equal(TNone):
		return "null"
	case v.Parent.Equal(TMap):
		return formatMapJSON(v)
	case v.Parent.Equal(TList):
		return formatListJSON(v)
	default:
		return v.String()
	}
}

// formatTable formats a TableData as an aligned text table with headers.
func formatTable(td TableData) string {
	columns := td.Record.Fields.Keys()
	if len(columns) == 0 {
		return "(empty table)"
	}

	// Collect all cell values as strings.
	cells := make([][]string, len(td.Rows))
	for i, row := range td.Rows {
		cells[i] = make([]string, len(columns))
		om, _ := AsMap(row)
		for j, col := range columns {
			if val, ok := om.Get(col); ok {
				cells[i][j] = ValToString(val)
			}
		}
	}

	// Calculate column widths.
	widths := make([]int, len(columns))
	for j, col := range columns {
		widths[j] = len(col)
	}
	for _, row := range cells {
		for j, cell := range row {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}

	var b strings.Builder

	// Header row.
	for j, col := range columns {
		if j > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(PadRight(col, widths[j]))
	}
	b.WriteByte('\n')

	// Separator row.
	for j := range columns {
		if j > 0 {
			b.WriteString("-+-")
		}
		b.WriteString(strings.Repeat("-", widths[j]))
	}
	b.WriteByte('\n')

	// Data rows.
	for _, row := range cells {
		for j, cell := range row {
			if j > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(PadRight(cell, widths[j]))
		}
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

// PadRight pads s with spaces on the right to reach width.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
