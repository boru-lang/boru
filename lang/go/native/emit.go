package native

import (
	"fmt"
	"strconv"
	"strings"
)

// The canonical walk-based emitter — the single source of truth for turning
// a boru Value into a string. It is the symmetric inverse of the tabnas
// decoder core (tabnas.go): where TabnasKinds drives string→value for the
// `parse` word and `read`, EmitKinds drives value→string for the `emit` word
// and `write`.
//
// HARD REQUIREMENT: every encoder traverses values through the walk engine
// (childrenOf / walkEntry from walk_core.go) via the shared node classifier
// (classifyNode) — there is NO bespoke parallel traversal. json, jsonic,
// yaml, csv, tsv, toml, ini and xml all flow through this one classifier, so
// `write`, `emit`, and any future encoder share exactly one code path.

// emitNodeKind is the structural classification of a value, decided by TYPE
// (so an EMPTY container still classifies as a container even though
// childrenOf reports isContainer=false for it).
type emitNodeKind int

const (
	emitScalar emitNodeKind = iota // a leaf: render via scalar rules
	emitList                       // list-like: TList or Ideal Array/Table
	emitMap                        // map-like: concrete Map or Ideal Object/Store/Error
	emitXml                        // an Xml element
)

// classifyNode reports a value's structural kind plus its ordered children
// (via the walk engine). The container-ness is decided by TYPE, so empty
// containers still classify as emitList / emitMap and emit `[]` / `{}`.
// Every concrete access is guarded — a type literal or a non-concrete map
// carrier (Record/Options/ChildType, whose AsMap is nil) classifies as a
// scalar leaf and never panics.
func classifyNode(v Value) (emitNodeKind, []walkEntry) {
	entries, _ := childrenOf(v)

	// XML first: an Xml element is neither list nor map.
	if IsXmlValue(v) {
		return emitXml, entries
	}

	// List-like: a concrete List, or an Ideal Table (which childrenOf
	// projects to a List).
	if IsConcrete(&v) {
		if v.Parent().ConformsTo(TList) {
			return emitList, entries
		}
		if v.Parent().ConformsTo(TTable) {
			return emitList, entries
		}
		if v.Parent().ConformsTo(TMap) {
			return emitMap, entries
		}
		if v.Parent().ConformsTo(TIdeal) {
			// Object / Store / Error / Module / … project to a Map.
			return emitMap, entries
		}
	}

	// A non-concrete map/list CARRIER (a typed-map ChildTypeInfo, a list
	// carrier) has no readable children but is structurally a container — it
	// emits as the empty form `{}` / `[]`, matching the legacy valueToJsonic.
	if v.Parent().ConformsTo(TList) {
		return emitList, nil
	}
	if v.Parent().ConformsTo(TMap) {
		return emitMap, nil
	}

	return emitScalar, nil
}

// EmitKind describes one value→string encoder. It mirrors TabnasKind: a name,
// the types it is the natural target for, and the encode fn.
type EmitKind struct {
	// Name is the format / emit-kind atom ("json", "csv", …).
	Name string
	// Natural are the types this kind is the natural emit target for. Used by
	// NaturalEmitKind to resolve a bare `emit <data>` to a kind.
	Natural []*Type
	// Encode renders v to a string with the given opts, or an error for a
	// shape it cannot represent (never a panic).
	Encode func(v Value, opts map[string]any) (string, error)
}

// EmitKinds returns the built-in emit kinds in a fixed order. The order is
// significant — EmitLang.kinds is pinned to it (lang/spec/module-emitlang.tsv),
// so new kinds append at the end rather than insert mid-list.
func EmitKinds() []EmitKind {
	return []EmitKind{
		{Name: "json", Natural: []*Type{TMap, TList, TScalar, TError}, Encode: encodeJSON},
		{Name: "jsonic", Natural: nil, Encode: encodeJsonic},
		{Name: "yaml", Natural: nil, Encode: encodeYAML},
		{Name: "csv", Natural: []*Type{TTable}, Encode: func(v Value, opts map[string]any) (string, error) {
			return encodeTabular(v, optSeparator(opts, ","), "csv")
		}},
		{Name: "tsv", Natural: nil, Encode: func(v Value, opts map[string]any) (string, error) {
			return encodeTabular(v, "\t", "tsv")
		}},
		{Name: "toml", Natural: nil, Encode: encodeTOML},
		{Name: "ini", Natural: nil, Encode: encodeINI},
		{Name: "xml", Natural: []*Type{TXml}, Encode: encodeXML},
	}
}

// EmitOptsSchema returns the Options-schema type literal for a BUILT-IN emit
// kind's opts slot — the exact key set the kind's Encode handler reads (G6):
// json/jsonic take {pretty indent}, yaml reads the same pair via optIndent,
// csv takes {separation}, xml takes {pretty}, and tsv/toml/ini read nothing
// (an EMPTY schema, so any key is rejected). Every field is optional
// (Disjunct with None), mirroring the `convert` opts precedent
// (convertOptsPattern): dispatch unifies a CONCRETE opts map against the
// schema, so a typo'd key (`{prety:true}`) is a hard signature rejection at
// check AND run time instead of a silently ignored option, while a dynamic
// opts map still matches (Options vs a non-concrete Map preserves the
// schema). ok is false for a kind that is not built in (host- and
// boru-registered emitters own their key sets, so their opts stay a plain
// Map). The schema for emit_auto is EmitAutoOptsSchema.
func EmitOptsSchema(kind string) (Value, bool) {
	switch kind {
	case "json", "jsonic", "yaml":
		return emitOptsSchema("pretty", "indent"), true
	case "csv":
		return emitOptsSchema("separation"), true
	case "tsv", "toml", "ini":
		return emitOptsSchema(), true
	case "xml":
		return emitOptsSchema("pretty"), true
	}
	return Value{}, false
}

// EmitAutoOptsSchema is the opts schema for emit_auto: the UNION of the keys
// of the natural-target kinds auto can delegate to (json / csv / xml — see
// NaturalEmitKind), since the concrete kind is unknown until the value's type
// is inspected.
func EmitAutoOptsSchema() Value {
	return emitOptsSchema("pretty", "indent", "separation")
}

// emitOptsSchema assembles an Options type literal whose fields are the given
// keys, each typed per emitOptFieldTypes and optional (Disjunct with None —
// the missing-key default rule in unifyOptionsFamily).
func emitOptsSchema(keys ...string) Value {
	fields := NewOrderedMap()
	for _, k := range keys {
		fields.Set(k, NewDisjunct([]Value{
			NewTypeLiteral(emitOptFieldTypes[k]), NewTypeLiteral(TNone),
		}))
	}
	return NewOptionsType(fields)
}

// emitOptFieldTypes maps each built-in emit opt key to its value type: the
// type the opts helpers (optBool / optIndent / optSeparator) actually read.
// indent is Number, not Integer — optIndent accepts int64 AND float64, so the
// schema mirrors the handler's real tolerance.
var emitOptFieldTypes = map[string]*Type{
	"pretty":     TBoolean,
	"indent":     TNumber,
	"separation": TString,
}

// NaturalEmitKind maps a value's type to the name of its natural emit kind.
// Map/Record/Options/Object/Store/Error/Scalars → json; List/Array → json;
// Table → csv; Xml → xml. (json is the broad default.) Returns false for a
// type with no natural target (e.g. a Function).
func NaturalEmitKind(v Value) (string, bool) {
	if IsXmlValue(v) {
		return "xml", true
	}
	if IsConcrete(&v) {
		// A concrete table from read / CSVFormat.Decode is a TList whose payload
		// is TableData (its Parent does NOT conform to TTable), so detect the
		// payload directly — otherwise the TList branch below would pick json and
		// `read "x.csv" emit` would emit JSON instead of the natural CSV.
		if _, ok := v.Data().(TableData); ok {
			return "csv", true
		}
		if v.Parent().ConformsTo(TTable) {
			return "csv", true
		}
		if v.Parent().ConformsTo(TList) {
			return "json", true
		}
		if v.Parent().ConformsTo(TMap) {
			return "json", true // includes Record/Options leaves — json renders them
		}
		if v.Parent().ConformsTo(TIdeal) {
			return "json", true // Object / Store / Error / …
		}
	}
	// Scalars (String / Number / Boolean / Atom / None) and Error → json.
	switch {
	case v.Parent().ConformsTo(TScalar),
		v.Parent().ConformsTo(TString),
		v.Parent().ConformsTo(TNumber),
		v.Parent().ConformsTo(TInteger),
		v.Parent().ConformsTo(TFloat),
		v.Parent().ConformsTo(TBoolean),
		v.Parent().ConformsTo(TAtom),
		IsNoneShape(v),
		v.Parent().ConformsTo(TError):
		return "json", true
	}
	return "", false
}

// scalarLeaf renders the format-independent scalar arms shared by the leaf
// renderers (emitScalarText / yamlLeaf / tomlScalar / iniScalar): Float,
// Integer and Boolean render identically in every format, while String and
// Atom route through the per-format quote fn. ok=false for any other shape
// (None, containers, Xml, …) so each format keeps its own None / default /
// error arm. (None and Atom are disjoint lattice families — None sits at the
// top-level "None" path, Atom under Scalar — so checking Atom here before
// each caller's None arm cannot change which arm a value hits.)
func scalarLeaf(v Value, quote func(string) string) (string, bool) {
	switch {
	case v.Parent().ConformsTo(TString):
		s, _ := AsString(v)
		return quote(s), true
	case v.Parent().ConformsTo(TFloat):
		f, _ := AsFloat(v)
		return strconv.FormatFloat(f, 'f', -1, 64), true
	case v.Parent().ConformsTo(TInteger):
		i, _ := AsInteger(v)
		return fmt.Sprintf("%d", i), true
	case v.Parent().ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		if b {
			return "true", true
		}
		return "false", true
	case v.Parent().ConformsTo(TAtom):
		a, _ := AsAtom(v)
		return quote(a), true
	}
	return "", false
}

// emitScalarText renders a scalar leaf for json/jsonic: String→JSON-quoted,
// Float→FormatFloat(f,'f',-1,64), Integer→%d, Boolean→true/false, None→null,
// Atom→JSON-quoted, anything else→JSON-quoted String(). Strings go through
// jsonQuote (not %q) so control bytes escape as \u00XX and the output is always
// valid, parseable JSON. Shared by json and jsonic.
func emitScalarText(v Value) string {
	if s, ok := scalarLeaf(v, jsonQuote); ok {
		return s
	}
	if IsNoneShape(v) {
		return "null"
	}
	return jsonQuote(v.String())
}

// jsonQuote renders s as a valid JSON string literal. It escapes the JSON
// mandatory characters — quote, backslash and the C0 control range — using the
// short forms where defined and \u00XX otherwise. Unlike fmt.Sprintf("%q", …)
// it never emits Go-only escapes such as \x01, so the result always parses as
// JSON. It deliberately does NOT escape <,>,& (output stays readable and, for
// printable ASCII, byte-identical to the previous %q form, so existing golden
// expectations are unaffected).
func jsonQuote(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + 2)
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// ---- opts helpers --------------------------------------------------------

// optBool reads a boolean opt (default false).
func optBool(opts map[string]any, key string) bool {
	if opts == nil {
		return false
	}
	if b, ok := opts[key].(bool); ok {
		return b
	}
	return false
}

// optIndent reads an indentation width from opts: {indent:N} sets N,
// {pretty:true} implies the default. Returns (width, pretty?). A negative width
// is clamped to 0 so a hostile {indent:-1} produces newline-only pretty output
// rather than a strings.Repeat panic.
func optIndent(opts map[string]any, def int) (int, bool) {
	if opts != nil {
		n, ok := 0, false
		switch x := opts["indent"].(type) {
		case int:
			n, ok = x, true
		case int64:
			n, ok = int(x), true
		case float64:
			n, ok = int(x), true
		}
		if ok {
			if n < 0 {
				n = 0
			}
			return n, true
		}
	}
	if optBool(opts, "pretty") {
		return def, true
	}
	return 0, false
}

// optSeparator reads a {separation:sep} opt, falling back to def.
func optSeparator(opts map[string]any, def string) string {
	if opts != nil {
		if s, ok := opts["separation"].(string); ok && s != "" {
			return s
		}
	}
	return def
}

// ---- json / jsonic -------------------------------------------------------

var bareIdent = func() func(string) bool {
	// ^[A-Za-z_][A-Za-z0-9_]*$ without a regexp dependency.
	return func(s string) bool {
		if s == "" {
			return false
		}
		for i := 0; i < len(s); i++ {
			c := s[i]
			ok := c == '_' ||
				(c >= 'A' && c <= 'Z') ||
				(c >= 'a' && c <= 'z') ||
				(i > 0 && c >= '0' && c <= '9')
			if !ok {
				return false
			}
		}
		return true
	}
}()

func encodeJSON(v Value, opts map[string]any) (string, error) {
	indent, pretty := optIndent(opts, 2)
	var sb strings.Builder
	jsonNode(&sb, v, indent, pretty, 0, false)
	return sb.String(), nil
}

func encodeJsonic(v Value, opts map[string]any) (string, error) {
	indent, pretty := optIndent(opts, 2)
	var sb strings.Builder
	jsonNode(&sb, v, indent, pretty, 0, true)
	return sb.String(), nil
}

// jsonNode renders a node into sb. bare=true renders identifier map keys
// without quotes (the jsonic profile). The traversal is via classifyNode (=
// the walk engine).
func jsonNode(sb *strings.Builder, v Value, indent int, pretty bool, depth int, bare bool) {
	kind, entries := classifyNode(v)
	switch kind {
	case emitMap:
		jsonContainer(sb, entries, indent, pretty, depth, bare, '{', '}', true)
	case emitList:
		jsonContainer(sb, entries, indent, pretty, depth, bare, '[', ']', false)
	case emitXml:
		// XML inside json renders as its source string (the {tag,attr,children}
		// projection is optional; the textual form round-trips for parse xml).
		sb.WriteString(jsonQuote(v.String()))
	default:
		sb.WriteString(emitScalarText(v))
	}
}

func jsonContainer(sb *strings.Builder, entries []walkEntry, indent int, pretty bool, depth int, bare bool, open, close byte, isMap bool) {
	sb.WriteByte(open)
	if len(entries) == 0 {
		sb.WriteByte(close)
		return
	}
	childDepth := depth + 1
	for i, e := range entries {
		if i > 0 {
			sb.WriteByte(',')
		}
		if pretty {
			sb.WriteByte('\n')
			sb.WriteString(strings.Repeat(" ", indent*childDepth))
		}
		if isMap {
			jsonKey(sb, e.key, bare)
			if pretty {
				sb.WriteString(": ")
			} else {
				sb.WriteByte(':')
			}
		}
		jsonNode(sb, e.val, indent, pretty, childDepth, bare)
	}
	if pretty {
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat(" ", indent*depth))
	}
	sb.WriteByte(close)
}

func jsonKey(sb *strings.Builder, key string, bare bool) {
	if bare && bareIdent(key) {
		sb.WriteString(key)
		return
	}
	sb.WriteString(jsonQuote(key))
}

// ---- yaml ----------------------------------------------------------------

func encodeYAML(v Value, opts map[string]any) (string, error) {
	indent, _ := optIndent(opts, 2)
	if indent <= 0 {
		indent = 2
	}
	var sb strings.Builder
	yamlNode(&sb, v, indent, 0, true)
	out := sb.String()
	return strings.TrimRight(out, "\n"), nil
}

// yamlNode renders v in block style. atTop suppresses the leading newline for
// the root container. The traversal is via classifyNode.
func yamlNode(sb *strings.Builder, v Value, indent, depth int, atTop bool) {
	kind, entries := classifyNode(v)
	pad := strings.Repeat(" ", indent*depth)
	switch kind {
	case emitMap:
		if len(entries) == 0 {
			sb.WriteString("{}\n")
			return
		}
		for _, e := range entries {
			ck, _ := classifyNode(e.val)
			sb.WriteString(pad)
			sb.WriteString(yamlScalar(NewString(e.key)))
			sb.WriteByte(':')
			if ck == emitScalar || ck == emitXml {
				sb.WriteByte(' ')
				sb.WriteString(yamlLeaf(e.val))
				sb.WriteByte('\n')
			} else {
				// nested block on following lines
				_, nentries := classifyNode(e.val)
				if len(nentries) == 0 {
					sb.WriteByte(' ')
					if ck == emitList {
						sb.WriteString("[]")
					} else {
						sb.WriteString("{}")
					}
					sb.WriteByte('\n')
				} else {
					sb.WriteByte('\n')
					yamlNode(sb, e.val, indent, depth+1, false)
				}
			}
		}
	case emitList:
		if len(entries) == 0 {
			sb.WriteString("[]\n")
			return
		}
		for _, e := range entries {
			ck, _ := classifyNode(e.val)
			sb.WriteString(pad)
			sb.WriteString("- ")
			if ck == emitScalar || ck == emitXml {
				sb.WriteString(yamlLeaf(e.val))
				sb.WriteByte('\n')
			} else {
				_, nentries := classifyNode(e.val)
				if len(nentries) == 0 {
					if ck == emitList {
						sb.WriteString("[]")
					} else {
						sb.WriteString("{}")
					}
					sb.WriteByte('\n')
				} else {
					// inline the nested block one indent deeper, first line on
					// the same `- ` line.
					sb.WriteByte('\n')
					yamlNode(sb, e.val, indent, depth+1, false)
				}
			}
		}
	default:
		sb.WriteString(pad)
		sb.WriteString(yamlLeaf(v))
		sb.WriteByte('\n')
	}
}

// yamlLeaf renders a scalar (or an Xml element as its string) for yaml.
func yamlLeaf(v Value) string {
	if IsXmlValue(v) {
		return yamlQuote(v.String())
	}
	if s, ok := scalarLeaf(v, yamlQuote); ok {
		return s
	}
	if IsNoneShape(v) {
		return "null"
	}
	return yamlQuote(v.String())
}

// yamlQuote routes a raw string through yaml's minimal-quoting scalar rule.
func yamlQuote(s string) string { return yamlScalar(NewString(s)) }

// yamlScalar renders a string scalar with minimal quoting: quote when empty,
// when it could be mis-parsed as another type, or when it contains special
// characters.
func yamlScalar(v Value) string {
	s, err := AsString(v)
	if err != nil {
		return v.String()
	}
	if yamlNeedsQuote(s) {
		return fmt.Sprintf("%q", s)
	}
	return s
}

func yamlNeedsQuote(s string) bool {
	if s == "" {
		return true
	}
	switch s {
	case "true", "false", "null", "yes", "no", "~":
		return true
	}
	// Looks like a number?
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	// Leading/trailing space, or any YAML-significant punctuation.
	if s[0] == ' ' || s[len(s)-1] == ' ' {
		return true
	}
	if strings.ContainsAny(s, ":#{}[],&*!|>'\"%@`\n\t") {
		return true
	}
	if s[0] == '-' || s[0] == '?' {
		return true
	}
	return false
}

// ---- csv / tsv -----------------------------------------------------------

// encodeTabular renders a table-shaped value to CSV/TSV. It accepts a Table
// value, a Materializer / QueryBuilder (materialized first), a list of records
// (list of maps), or a list of lists. The header comes from the Table record
// schema or the first row's keys. The traversal walks the rows and cells via
// classifyNode. Output reproduces the legacy encodeDelimited.
func encodeTabular(v Value, sep, name string) (string, error) {
	var rows []Value
	var columns []string
	tabular := false // a Table / Materializer carries an explicit schema

	switch data := v.Data().(type) {
	case TableData:
		columns = data.Record.Fields.Keys()
		rows = data.Rows
		tabular = true
	case MaterializerPayload, ExtensionPayload:
		qb, ok := unwrapQB(v)
		if !ok {
			return "", emitUnsupported(name, "value is not a materializable table")
		}
		td, err := qb.Materialize()
		if err != nil {
			return "", fmt.Errorf("encode: %w", err)
		}
		columns = td.Record.Fields.Keys()
		rows = td.Rows
		tabular = true
	default:
		// A list-like value (List, Array, FlexList): walk its rows. A non-list
		// shape (a bare Map or scalar) cannot be represented as a table.
		kind, entries := classifyNode(v)
		if kind != emitList {
			return "", emitUnsupported(name, "value must be a table or a list of rows")
		}
		rows = make([]Value, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, e.val)
		}
		if len(rows) > 0 {
			if m, err := AsMutableMap(rows[0]); err == nil {
				columns = m.Keys()
			}
		}
	}

	if len(columns) == 0 {
		if tabular {
			// A Table with an empty schema renders empty (legacy behaviour).
			return "", nil
		}
		// list-of-lists: no named columns. Emit each row's cells directly.
		return encodeTabularListOfLists(rows, sep)
	}

	var sb strings.Builder
	sb.WriteString(joinFields(columns, sep))
	sb.WriteByte('\n')
	for _, row := range rows {
		m, err := AsMutableMap(row)
		if err != nil {
			continue
		}
		parts := make([]string, len(columns))
		for i, col := range columns {
			val, exists := m.Get(col)
			if !exists {
				parts[i] = ""
				continue
			}
			parts[i] = csvCell(val, sep)
		}
		sb.WriteString(joinFields(parts, sep))
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// encodeTabularListOfLists handles a list of lists (no named columns): each
// inner list is a row, cells walked one level.
func encodeTabularListOfLists(rows []Value, sep string) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, row := range rows {
		kind, entries := classifyNode(row)
		if kind != emitList {
			// a non-list row degrades to a single cell.
			sb.WriteString(csvCell(row, sep))
			sb.WriteByte('\n')
			continue
		}
		parts := make([]string, 0, len(entries))
		for _, e := range entries {
			parts = append(parts, csvCell(e.val, sep))
		}
		sb.WriteString(joinFields(parts, sep))
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// joinFields joins pre-rendered fields with sep (already quoted as needed).
func joinFields(fields []string, sep string) string {
	return strings.Join(fields, sep)
}

// csvCell renders one field, applying RFC-4180 quoting (a String containing
// the separator, a quote, newline or carriage return is wrapped in quotes
// with internal quotes doubled). Non-string cells render via String().
func csvCell(v Value, sep string) string {
	var s string
	if v.Parent().ConformsTo(TString) && IsConcrete(&v) {
		s, _ = AsString(v)
	} else {
		s = v.String()
	}
	if strings.Contains(s, sep) || strings.ContainsAny(s, "\"\n\r") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// ---- toml ----------------------------------------------------------------

// encodeTOML renders a Map of scalars/arrays to top-level `key = value` lines,
// and nested Maps to `[table]` headers (arrays of tables to `[[array]]`).
// An unsupported top shape (bare scalar, top-level list) → emit_unsupported.
func encodeTOML(v Value, _ map[string]any) (string, error) {
	kind, entries := classifyNode(v)
	if kind != emitMap {
		return "", emitUnsupported("toml", "top-level value must be a Map")
	}
	var sb strings.Builder
	if err := tomlTable(&sb, entries, nil); err != nil {
		return "", err
	}
	return strings.Trim(sb.String(), "\n"), nil
}

// tomlTable emits the scalar/array keys of a table, then recurses into nested
// tables under a [path] header. prefix is the dotted table path (raw keys).
func tomlTable(sb *strings.Builder, entries []walkEntry, prefix []string) error {
	// First pass: scalar and array-of-scalar keys.
	var nested []walkEntry
	for _, e := range entries {
		ck, _ := classifyNode(e.val)
		switch ck {
		case emitMap:
			nested = append(nested, e)
		case emitList:
			arr, err := tomlArray(e.val)
			if err != nil {
				return err
			}
			sb.WriteString(tomlKey(e.key))
			sb.WriteString(" = ")
			sb.WriteString(arr)
			sb.WriteByte('\n')
		default:
			sc, err := tomlScalar(e.val)
			if err != nil {
				return err
			}
			sb.WriteString(tomlKey(e.key))
			sb.WriteString(" = ")
			sb.WriteString(sc)
			sb.WriteByte('\n')
		}
	}
	// Second pass: nested tables.
	for _, e := range nested {
		_, nentries := classifyNode(e.val)
		path := append(append([]string{}, prefix...), e.key)
		header := make([]string, len(path))
		for i, p := range path {
			header[i] = tomlKey(p)
		}
		sb.WriteByte('\n')
		sb.WriteByte('[')
		sb.WriteString(strings.Join(header, "."))
		sb.WriteString("]\n")
		if err := tomlTable(sb, nentries, path); err != nil {
			return err
		}
	}
	return nil
}

// tomlKey renders a TOML key: a bare key for TOML's bare-key charset
// [A-Za-z0-9_-], otherwise a quoted basic-string key (JSON escaping), so keys
// with spaces/dots/punctuation round-trip through parse toml.
func tomlKey(s string) string {
	if s == "" {
		return `""`
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c == '-' ||
			(c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9')) {
			return jsonQuote(s)
		}
	}
	return s
}

// tomlArray renders a list. A list of maps is unsupported here (would need
// [[array of tables]] which the flat encoder does not emit inline).
func tomlArray(v Value) (string, error) {
	_, entries := classifyNode(v)
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		ck, _ := classifyNode(e.val)
		if ck != emitScalar {
			return "", emitUnsupported("toml", "arrays of tables are not supported")
		}
		sc, err := tomlScalar(e.val)
		if err != nil {
			return "", err
		}
		parts = append(parts, sc)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

// tomlScalar renders a scalar TOML value, or returns emit_unsupported for a
// value TOML cannot represent (e.g. None — TOML has no null) so the type is
// never silently corrupted to a string on round-trip.
func tomlScalar(v Value) (string, error) {
	if s, ok := scalarLeaf(v, jsonQuote); ok {
		return s, nil
	}
	return "", emitUnsupported("toml", "cannot represent value: "+v.String())
}

// ---- ini -----------------------------------------------------------------

// encodeINI renders `{section:{k:v}}` to `[section]` blocks with key=value
// lines; a flat `{k:scalar}` to bare key=value lines; anything deeper →
// emit_unsupported.
func encodeINI(v Value, _ map[string]any) (string, error) {
	kind, entries := classifyNode(v)
	if kind != emitMap {
		return "", emitUnsupported("ini", "top-level value must be a Map")
	}
	var flat []walkEntry
	var sections []walkEntry
	for _, e := range entries {
		ck, _ := classifyNode(e.val)
		switch ck {
		case emitMap:
			sections = append(sections, e)
		case emitScalar:
			flat = append(flat, e)
		default:
			return "", emitUnsupported("ini", "values must be scalars or one level of sections")
		}
	}
	var sb strings.Builder
	for _, e := range flat {
		sb.WriteString(e.key)
		sb.WriteByte('=')
		sb.WriteString(iniScalar(e.val))
		sb.WriteByte('\n')
	}
	for _, e := range sections {
		_, sentries := classifyNode(e.val)
		if len(flat) > 0 || sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteByte('[')
		sb.WriteString(e.key)
		sb.WriteString("]\n")
		for _, kv := range sentries {
			ck, _ := classifyNode(kv.val)
			if ck != emitScalar {
				return "", emitUnsupported("ini", "section values must be scalars")
			}
			sb.WriteString(kv.key)
			sb.WriteByte('=')
			sb.WriteString(iniScalar(kv.val))
			sb.WriteByte('\n')
		}
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// iniScalar renders a scalar for ini: strings and atoms stay raw (no
// quoting), everything else (incl. None) falls back to String().
func iniScalar(v Value) string {
	if s, ok := scalarLeaf(v, func(s string) string { return s }); ok {
		return s
	}
	return v.String()
}

// ---- xml -----------------------------------------------------------------

// encodeXML walks an Xml value into `<tag attr="...">children</tag>`. With
// {pretty:true} it indents. A non-Xml top → emit_unsupported.
func encodeXML(v Value, opts map[string]any) (string, error) {
	if !IsXmlValue(v) {
		return "", emitUnsupported("xml", "top-level value must be an Xml element")
	}
	if !optBool(opts, "pretty") {
		return v.String(), nil
	}
	var sb strings.Builder
	xmlPretty(&sb, v, 2, 0)
	return strings.TrimRight(sb.String(), "\n"), nil
}

// xmlPretty renders an XML element with indentation. Text-only children stay
// inline; element children break onto their own lines.
func xmlPretty(sb *strings.Builder, v Value, indent, depth int) {
	tag, attr, cren, ok := XmlParts(v)
	if !ok {
		sb.WriteString(strings.Repeat(" ", indent*depth))
		sb.WriteString(xmlText(v.String()))
		sb.WriteByte('\n')
		return
	}
	pad := strings.Repeat(" ", indent*depth)
	sb.WriteString(pad)
	sb.WriteByte('<')
	sb.WriteString(tag)
	if attr != nil {
		for _, k := range attr.Keys() {
			av, _ := attr.Get(k)
			as, _ := AsString(av)
			sb.WriteByte(' ')
			sb.WriteString(k)
			sb.WriteString(`="`)
			sb.WriteString(xmlAttr(as))
			sb.WriteByte('"')
		}
	}
	if len(cren) == 0 {
		sb.WriteString("/>\n")
		return
	}
	// All-text content stays inline.
	allText := true
	for _, c := range cren {
		if IsXmlValue(c) {
			allText = false
			break
		}
	}
	if allText {
		sb.WriteByte('>')
		for _, c := range cren {
			if s, e := AsString(c); e == nil {
				sb.WriteString(xmlText(s))
			} else {
				sb.WriteString(xmlText(c.String()))
			}
		}
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteString(">\n")
		return
	}
	sb.WriteString(">\n")
	for _, c := range cren {
		if IsXmlValue(c) {
			xmlPretty(sb, c, indent, depth+1)
		} else {
			s := c.String()
			if st, e := AsString(c); e == nil {
				s = st
			}
			if strings.TrimSpace(s) == "" {
				continue
			}
			sb.WriteString(strings.Repeat(" ", indent*(depth+1)))
			sb.WriteString(xmlText(s))
			sb.WriteByte('\n')
		}
	}
	sb.WriteString(pad)
	sb.WriteString("</")
	sb.WriteString(tag)
	sb.WriteString(">\n")
}

func xmlText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func xmlAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

// ---- errors --------------------------------------------------------------

// emitUnsupported builds the canonical "emit_unsupported" error for a shape a
// format cannot represent. It is a plain error (no Registry handy in the core)
// — the module/word layer wraps it with the registry source.
func emitUnsupported(kind, detail string) error {
	return &emitError{Code: "emit_unsupported", Msg: kind + ": " + detail}
}

// emitError is a typed error carrying the boru error code so the module layer
// can surface it via r.BoruError without string-matching.
type emitError struct {
	Code string
	Msg  string
}

func (e *emitError) Error() string { return e.Msg }

// EmitErrorCode returns the boru error code for an emit error, or "" if err is
// not an emitError.
func EmitErrorCode(err error) string {
	if e, ok := err.(*emitError); ok {
		return e.Code
	}
	return ""
}

// EmitKindNames lists the built-in kind names in EmitKinds() order, for error
// hints (e.g. emit_auto's emit_no_natural).
func EmitKindNames() string {
	kinds := EmitKinds()
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, k.Name)
	}
	return strings.Join(names, ", ")
}
