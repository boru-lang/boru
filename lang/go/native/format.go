package native

import (
	"errors"
	"fmt"
	"math"
	"strings"

	csvpkg "github.com/tabnas/csv/go"
	jsonic "github.com/tabnas/jsonic/go"
	multisource "github.com/tabnas/multisource/go"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// Format encodes and decodes file content for a named representation
// (text, json, csv, …). The host package owns this interface — borueng
// has no knowledge of file formats. Word handlers look formats up via
// HostFormats(r) (see capabilities.go).
type Format interface {
	Decode(content string) ([]Value, error)
	Encode(v Value) (string, error)
}

// DecodeOpter is an optional extension of Format: a decoder that accepts
// parser options. doRead prefers DecodeOpts when the format implements it
// and the caller supplied options, otherwise it falls back to Decode. The
// built-in encodable formats (text/json/csv/…) implement only Format and
// are unaffected.
type DecodeOpter interface {
	DecodeOpts(content string, opts map[string]any) ([]Value, error)
}

// ReadOnlyFormat marks a Format whose Encode is unsupported — parser-backed
// formats are decode-only. write consults this marker (NOT by executing
// Encode, which could run arbitrary host-side work) to reject an explicit
// read-only {fmt:…}. TabnasFormat implements it; a host's own decode-only
// format can too.
type ReadOnlyFormat interface {
	ReadOnly() bool
}

// errReadOnlyFormat is the sentinel wrapped by parser-backed formats whose
// Encode is not supported. IsReadOnlyFormatErr detects it so write can
// surface a clean message.
var errReadOnlyFormat = errors.New("format is read-only")

// csvConfigure seams csvpkg.Csv for decodeDelimited's plugin-configuration
// error arm (design/TEST-SEAMS.10.md). The constant options never fail in
// practice, but the third-party plugin owns that guarantee, not this
// package.
var csvConfigure = csvpkg.Csv

// jsonicDecodeValue seams jsonicToValue for the conversion-failure arms of
// JSONFormat.Decode / JsonicFormat.Decode (design/TEST-SEAMS.10.md). A
// default jsonic parse only yields nil/bool/float64/string/list/map — all
// convertible — but the parser owns that output-shape guarantee, not this
// package.
var jsonicDecodeValue = jsonicToValue

// IsReadOnlyFormatErr reports whether err is (or wraps) the read-only
// sentinel returned by a parser-backed format's Encode.
func IsReadOnlyFormatErr(err error) bool {
	return errors.Is(err, errReadOnlyFormat)
}

// TabnasFormat wraps one TabnasKind (see tabnas.go) as a read-only Format.
// Decode / DecodeOpts run the tabnas decoder + converter; Encode always
// errors with the read-only sentinel — the tabnas family is parse-only, so
// write cannot serialise back through these formats.
type TabnasFormat struct {
	Kind TabnasKind
}

func (f *TabnasFormat) Decode(content string) ([]Value, error) {
	return f.DecodeOpts(content, nil)
}

func (f *TabnasFormat) DecodeOpts(content string, opts map[string]any) ([]Value, error) {
	v, err := f.Kind.Parse(content, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", f.Kind.Name, FirstCleanLine(err.Error()))
	}
	return []Value{f.Kind.Convert(v)}, nil
}

func (f *TabnasFormat) Encode(Value) (string, error) {
	return "", fmt.Errorf("format %q is %w", f.Kind.Name, errReadOnlyFormat)
}

// ReadOnly reports that the tabnas family is decode-only, letting write
// reject an explicit {fmt:…} without invoking Encode (see ReadOnlyFormat).
func (f *TabnasFormat) ReadOnly() bool { return true }

// TextFormat handles plain text (no parsing).
type TextFormat struct{}

func (f *TextFormat) Decode(content string) ([]Value, error) {
	return []Value{NewString(content)}, nil
}

func (f *TextFormat) Encode(v Value) (string, error) {
	if ParentOf(&v).ConformsTo(TString) {
		_as0, _ := AsString(v)
		return _as0, nil
	}
	return v.String(), nil
}

// JSONFormat handles strict JSON via jsonic.Parse (standard JSON is valid jsonic).
type JSONFormat struct{}

func (f *JSONFormat) Decode(content string) ([]Value, error) {
	result, err := parser.SafeParse(content)
	if err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	v, err := jsonicDecodeValue(result)
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

func (f *JSONFormat) Encode(v Value) (string, error) {
	return encodeJSON(v, nil) // canonical walk-based json encoder (emit.go)
}

// JsonicFormat handles relaxed JSON (unquoted keys, trailing commas, etc.).
// When a Resolver is set, the multisource plugin is enabled so that
// @"path" references in .jsonic files are resolved and merged.
type JsonicFormat struct {
	Resolver multisource.Resolver
}

func (f *JsonicFormat) Decode(content string) ([]Value, error) {
	var j *jsonic.Jsonic
	if f.Resolver != nil {
		// Constructed directly: tabnas/parser v0.8.0's instance counter is
		// atomic and Make is documented concurrent-safe, so this no longer
		// needs to be funnelled through a locking wrapper (ADR-014).
		j = multisource.MakeJsonic(multisource.MultiSourceOptions{
			Resolver: f.Resolver,
		})
	} else {
		j = parser.SafeMake()
	}
	result, err := j.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("invalid jsonic: %w", err)
	}
	if result == nil {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	v, err := jsonicDecodeValue(result)
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

// MakeFileOpsResolver creates a multisource.Resolver backed by a FileOps
// implementation. This bridges the boru file abstraction to multisource's
// path resolution so that @"path" references in .jsonic files work.
func MakeFileOpsResolver(ops capabilities.FileOps) multisource.Resolver {
	return func(spec multisource.PathSpec, opts *multisource.MultiSourceOptions, _ *jsonic.Context) multisource.Resolution {
		res := multisource.Resolution{
			PathSpec: spec,
		}

		// Build candidate paths with implicit extensions.
		candidates := []string{spec.Full}
		if spec.Kind == "" && opts != nil {
			for _, ext := range opts.ImplicitExt {
				candidates = append(candidates, spec.Full+"."+ext)
			}
		}

		for _, path := range candidates {
			data, err := ops.ReadFile(path)
			if err == nil {
				res.Src = string(data)
				res.Found = true
				res.Full = path

				// Determine kind from extension if not set.
				if res.Kind == "" {
					ext := ""
					for i := len(path) - 1; i >= 0; i-- {
						if path[i] == '.' {
							ext = path[i+1:]
							break
						}
					}
					res.Kind = ext
				}
				return res
			}
			res.Search = append(res.Search, path)
		}
		return res
	}
}

func (f *JsonicFormat) Encode(v Value) (string, error) {
	return encodeJSON(v, nil) // compact json (jsonic file writes are valid json)
}

// LinesFormat splits/joins on newlines, producing/consuming a list of strings.
type LinesFormat struct{}

func (f *LinesFormat) Decode(content string) ([]Value, error) {
	lines := strings.Split(content, "\n")
	elems := make([]Value, len(lines))
	for i, line := range lines {
		elems[i] = NewString(line)
	}
	return []Value{NewList(elems)}, nil
}

func (f *LinesFormat) Encode(v Value) (string, error) {
	if ParentOf(&v).ConformsTo(TList) {
		if elems, err := AsMutableList(v); err == nil {
			parts := make([]string, len(elems))
			for i, e := range elems {
				if ParentOf(&e).ConformsTo(TString) {
					_as1, _ := AsString(e)
					parts[i] = _as1
				} else {
					parts[i] = e.String()
				}
			}
			return strings.Join(parts, "\n"), nil
		}
	}
	return v.String(), nil
}

// CSVFormat parses CSV content into a table value.
// The first row is treated as column headers, producing a table type
// where each row is a record with those column names as fields.
type CSVFormat struct{}

func (f *CSVFormat) Decode(content string) ([]Value, error) {
	return decodeDelimited(content, ",")
}

func (f *CSVFormat) Encode(v Value) (string, error) {
	return encodeDelimited(v, ",")
}

// TSVFormat parses TSV (tab-separated) content into a table value.
type TSVFormat struct{}

func (f *TSVFormat) Decode(content string) ([]Value, error) {
	return decodeDelimited(content, "\t")
}

func (f *TSVFormat) Encode(v Value) (string, error) {
	return encodeDelimited(v, "\t")
}

// decodeDelimited parses CSV/TSV content into a table value.
// The first row is used as column headers. Each subsequent row becomes
// a record (map) keyed by those headers. The result is wrapped in a
// table type whose record schema has all-string fields.
func decodeDelimited(content string, sep string) ([]Value, error) {
	// tabnas/csv is a jsonic plugin (not a standalone Parse like jsonicjs/csv):
	// configure a jsonic instance with the CSV grammar, then parse. object:false
	// + header:false yields raw rows ([]any of []any), matching the previous
	// CsvOptions{Object:false, Header:false, Field:{Separation:sep}}.
	j := parser.SafeMake()
	if err := csvConfigure(j, map[string]any{
		"object": false,
		"header": false,
		// number/value coercion OFF: keep field text literal, matching the
		// legacy jsonicjs/csv behaviour (decodeDelimited builds an all-String
		// record schema; numeric typing is the caller's concern) — without
		// number/value, "30" stays the String "30" rather than the Integer 30.
		//
		// NB: the tabnas/csv `strict` flag (which would also keep a
		// jsonic-shaped field like "{x:1}" literal instead of parsing it to a
		// map) is deliberately NOT set — with strict:true the plugin
		// infinite-loops on ordinary CSV (see decodeDelimited tests). A
		// jsonic-shaped cell is the rare case and degrades to its string form
		// in the conversion loop below, which is acceptable.
		"number": false,
		"value":  false,
		"field":  map[string]any{"separation": sep},
	}); err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}
	parsed, err := j.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}
	records, _ := parsed.([]any)
	return []Value{convertDelimitedRecords(records)}, nil
}

// convertDelimitedRecords converts the parsed row-of-rows shape into the
// table value decodeDelimited returns: the first row is the header, each
// later row a record keyed by those headers. A missing/empty header or an
// empty record set yields an empty list; a non-slice data row is skipped.
func convertDelimitedRecords(records []any) Value {
	if len(records) == 0 {
		return NewList([]Value{})
	}

	// First row is the header.
	headerRow, ok := records[0].([]any)
	if !ok || len(headerRow) == 0 {
		return NewList([]Value{})
	}

	columns := make([]string, len(headerRow))
	for i, h := range headerRow {
		columns[i] = strings.TrimSpace(fmt.Sprintf("%v", h))
	}

	// Build the record type schema (all fields are string type).
	fields := NewOrderedMap()
	for _, col := range columns {
		fields.Set(col, NewTypeLiteral(TString))
	}
	recType := RecordTypeInfo{Fields: fields}

	// Convert each data row into a boru map value.
	rows := make([]Value, 0, len(records)-1)
	for _, rec := range records[1:] {
		arr, ok := rec.([]any)
		if !ok {
			continue
		}
		om := NewOrderedMap()
		for i, col := range columns {
			if i < len(arr) {
				om.Set(col, delimitedCellValue(arr[i]))
			} else {
				om.Set(col, NewString(""))
			}
		}
		rows = append(rows, NewMap(om))
	}

	// Return a table value: list with TableData holding both schema and rows.
	return NewValueRaw(TList, TableData{
		Record: recType,
		Rows:   rows,
	})
}

// delimitedCellValue converts one parsed CSV/TSV cell to its Value. The
// decoder parses with number/value coercion off, so cells are normally
// strings (or, rarely, a jsonic-shaped structural that degrades to its
// display string); the numeric/boolean arms keep the conversion total
// should a decoder configuration ever re-enable coercion.
func delimitedCellValue(cell any) Value {
	switch v := cell.(type) {
	case string:
		return NewString(v)
	case float64:
		if v == float64(int64(v)) && !math.IsInf(v, 0) && !math.IsNaN(v) {
			return NewInteger(int64(v))
		}
		return NewFloat(v)
	case bool:
		return NewBoolean(v)
	default:
		return NewString(fmt.Sprintf("%v", v))
	}
}

// TableData is re-exported by aliases.go (defined in borueng).

// encodeDelimited converts a table value to CSV/TSV text. It is a thin
// adapter over the canonical walk-based tabular encoder (emit.go) — the single
// emit code path — keeping its name and signature for the existing callers and
// tests. Unlike the `emit csv` surface (which raises emit_unsupported for a
// non-tabular shape), the legacy write/read path degrades a non-table value to
// its display string, preserving prior behaviour.
func encodeDelimited(v Value, sep string) (string, error) {
	s, err := encodeTabular(v, sep, "csv")
	if err != nil && EmitErrorCode(err) == "emit_unsupported" {
		return v.String(), nil
	}
	return s, err
}

// DefaultFormats returns the built-in format registry: read's own
// encodable decoders (text/json/jsonic/lines/csv/tsv) plus the read-only
// tabnas family folded in from TabnasKinds. The tabnas kinds whose names
// read already owns (json/jsonic/csv) are skipped, so read keeps its
// table-producing csv and its multisource-aware jsonic — only the formats
// read otherwise lacks (ini/json5/jsonc/toml/yaml/xml/zon/markdown/feed)
// are added.
func DefaultFormats() map[string]Format {
	m := map[string]Format{
		"text":   &TextFormat{},
		"json":   &JSONFormat{},
		"jsonic": &JsonicFormat{},
		"lines":  &LinesFormat{},
		"csv":    &CSVFormat{},
		"tsv":    &TSVFormat{},
	}
	for _, k := range TabnasKinds() {
		if _, taken := m[k.Name]; taken {
			continue
		}
		m[k.Name] = &TabnasFormat{Kind: k}
	}
	return m
}

// RegisterFormat installs f under name on r's format registry and maps each
// ext (leading dot optional, case-insensitive) to that format name. Once
// registered the format is reachable from `read` by name (via the fmt
// option) and by file extension. It pairs with the value constructor
// NewFormatParserFn, which wraps the same format's Decode as a parser
// Function value for `parse` (the parse kinds themselves are fixed).
// Returns an error when the formats capability is not installed.
func RegisterFormat(r *Registry, name string, f Format, exts ...string) error {
	if name == "" {
		return fmt.Errorf("register format: name must not be empty")
	}
	formats := HostFormats(r)
	if formats == nil {
		return fmt.Errorf("register format %q: formats capability not installed", name)
	}
	formats[name] = f // the map is host-owned and mutated in place (see capabilities.go)
	em := HostExtensions(r)
	if em == nil {
		em = DefaultExtensions()
		SetHostExtensions(r, em)
	}
	for _, e := range exts {
		em[strings.ToLower(strings.TrimPrefix(e, "."))] = name
	}
	return nil
}
