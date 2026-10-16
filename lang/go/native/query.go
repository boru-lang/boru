package native

import (
	"fmt"
	"strings"
)

// JoinClause represents a single JOIN in a query.
type JoinClause struct {
	Type      string // "JOIN", "LEFT JOIN", "CROSS JOIN"
	Table     string // table name in the store
	Alias     string // optional alias
	On        string // ON condition (raw SQL)
	UsingCols string // USING(col1, col2) clause
}

// SetOp represents a set operation combining two queries.
type SetOp struct {
	Op    string       // "UNION", "UNION ALL", "INTERSECT", "EXCEPT"
	Right QueryBuilder // the right-hand query
}

// QueryBuilder accumulates SQL clauses for deferred query execution.
// Instead of running a separate SQL query for each pipeline word
// (where, order, limit), the QueryBuilder collects all clauses and
// executes a single combined query when materialized.
type QueryBuilder struct {
	Source    TableData    // the source table data (valid only when HasSource)
	HasSource bool         // true once `from` has set the source table
	Cols      []columnSpec // projected columns (nil = SELECT *)
	Registry  *Registry    // needed for SQLite access during materialization
	Where     string       // WHERE condition (without keyword)
	OrderBy   string       // ORDER BY clause (without keyword)
	Limit     int          // -1 = no limit
	Offset    int          // -1 = no offset
	Distinct  bool         // true for SELECT DISTINCT
	GroupBy   string       // GROUP BY clause (without keyword)
	Having    string       // HAVING clause (without keyword)
	Joins     []JoinClause
	Alias     string // table alias for the FROM source
	SetOps    []SetOp
}

// NewQueryBuilder creates a QueryBuilder from a table data source.
func NewQueryBuilder(r *Registry, td TableData) QueryBuilder {
	return QueryBuilder{
		Source:    td,
		HasSource: true,
		Registry:  r,
		Limit:     -1,
		Offset:    -1,
	}
}

// newSelectBuilder seeds a sourceless QueryBuilder from a `select`
// projection. `from` fills in the source later; materialization errors
// if it never does. cols == nil means SELECT *.
func newSelectBuilder(r *Registry, cols []columnSpec) QueryBuilder {
	return QueryBuilder{
		Cols:     cols,
		Registry: r,
		Limit:    -1,
		Offset:   -1,
	}
}

// buildSQL constructs the full SQL query string.
func (qb *QueryBuilder) buildSQL(tableName string, colSQL string) string {
	var buf strings.Builder
	buf.WriteString("SELECT ")
	if qb.Distinct {
		buf.WriteString("DISTINCT ")
	}
	buf.WriteString(colSQL)
	buf.WriteString(" FROM ")
	buf.WriteString(quoteIdent(tableName))
	if qb.Alias != "" {
		buf.WriteString(" AS ")
		buf.WriteString(quoteIdent(qb.Alias))
	}
	for _, j := range qb.Joins {
		buf.WriteString(" ")
		buf.WriteString(j.Type)
		buf.WriteString(" ")
		buf.WriteString(quoteIdent(j.Table))
		if j.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(quoteIdent(j.Alias))
		}
		if j.On != "" {
			buf.WriteString(" ON ")
			buf.WriteString(j.On)
		}
		if j.UsingCols != "" {
			buf.WriteString(" USING(")
			buf.WriteString(j.UsingCols)
			buf.WriteString(")")
		}
	}
	if qb.Where != "" {
		buf.WriteString(" WHERE ")
		buf.WriteString(qb.Where)
	}
	if qb.GroupBy != "" {
		buf.WriteString(" GROUP BY ")
		buf.WriteString(qb.GroupBy)
	}
	if qb.Having != "" {
		buf.WriteString(" HAVING ")
		buf.WriteString(qb.Having)
	}

	// Set operations (UNION, INTERSECT, EXCEPT) are appended after the
	// first SELECT but before ORDER BY / LIMIT which apply to the combined result.
	for _, so := range qb.SetOps {
		buf.WriteString(" ")
		buf.WriteString(so.Op)
		buf.WriteString(" ")
		// Build the right-hand SELECT. It uses its own source table.
		rightTable := so.Right.Source.TableName
		if !so.Right.Source.SQLite {
			rightTable = "" // will be resolved during materialize
		}
		rightSQL := so.Right.buildSQL(rightTable, "*")
		buf.WriteString(rightSQL)
	}

	if qb.OrderBy != "" {
		buf.WriteString(" ORDER BY ")
		buf.WriteString(qb.OrderBy)
	}
	if qb.Limit >= 0 {
		fmt.Fprintf(&buf, " LIMIT %d", qb.Limit)
	} else if qb.Offset >= 0 {
		// SQLite has no bare OFFSET clause — an offset without a limit
		// must ride on the unlimited LIMIT -1.
		buf.WriteString(" LIMIT -1")
	}
	if qb.Offset >= 0 {
		fmt.Fprintf(&buf, " OFFSET %d", qb.Offset)
	}
	return buf.String()
}

// ensureSource loads the source table into SQLite if needed.
// Returns the table name to use and whether a temp table was created.
func (qb *QueryBuilder) ensureSource() (string, bool, error) {
	if qb.Source.SQLite {
		return qb.Source.TableName, false, nil
	}
	r := qb.Registry
	if HostSQLite(r) == nil {
		return "", false, fmt.Errorf("SQLite store not initialized")
	}
	tmpName, err := HostSQLite(r).StoreTempTable(qb.Source)
	if err != nil {
		return "", false, err
	}
	return tmpName, true, nil
}

// ensureJoinSources ensures all joined tables are in SQLite.
// Returns a list of temp table names that should be cleaned up.
func (qb *QueryBuilder) ensureJoinSources() ([]string, error) {
	var tmpNames []string
	for i := range qb.Joins {
		j := &qb.Joins[i]
		if HostSQLite(qb.Registry).HasTable(j.Table) {
			continue
		}
		// Look up the table in the context store and load it.
		val, ok := ContextStoreLookup(qb.Registry, j.Table)
		if !ok {
			return tmpNames, fmt.Errorf("join: unknown table %q", j.Table)
		}
		td, ok := val.Data().(TableData)
		if !ok {
			return tmpNames, fmt.Errorf("join: %q has no table data", j.Table)
		}
		if td.SQLite {
			j.Table = td.TableName
		} else {
			tmpName, err := HostSQLite(qb.Registry).StoreTempTable(td)
			if err != nil {
				return tmpNames, err
			}
			j.Table = tmpName
			tmpNames = append(tmpNames, tmpName)
		}
	}
	return tmpNames, nil
}

// mergedSchema returns a combined schema from the source and all joined tables.
func (qb *QueryBuilder) mergedSchema() RecordTypeInfo {
	fields := NewOrderedMap()
	// Add source fields.
	for _, k := range qb.Source.Record.Fields.Keys() {
		v, _ := qb.Source.Record.Fields.Get(k)
		fields.Set(k, v)
	}
	// Add joined table fields.
	for _, j := range qb.Joins {
		val, ok := ContextStoreLookup(qb.Registry, j.Table)
		if !ok {
			// Try the original name if it was remapped to a temp table.
			continue
		}
		td, ok := val.Data().(TableData)
		if !ok {
			continue
		}
		for _, k := range td.Record.Fields.Keys() {
			if _, exists := fields.Get(k); !exists {
				v, _ := td.Record.Fields.Get(k)
				fields.Set(k, v)
			}
		}
	}
	return RecordTypeInfo{Fields: fields}
}

// materializeAll executes the accumulated query with `SELECT *` and
// returns the result as TableData. It is the raw (pointer-receiver)
// engine path; the public value-receiver Materialize (which satisfies
// core.Materializer and handles the FROM-required check + column
// projection) delegates here for the no-projection case.
func (qb *QueryBuilder) materializeAll() (TableData, error) {
	tableName, ownsTmp, err := qb.ensureSource()
	if err != nil {
		return TableData{}, err
	}
	if ownsTmp {
		defer HostSQLite(qb.Registry).DropTable(tableName)
	}

	joinTmps, err := qb.ensureJoinSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range joinTmps {
		defer HostSQLite(qb.Registry).DropTable(t)
	}

	// Ensure set-op right-hand sources are in SQLite.
	setOpTmps, err := qb.ensureSetOpSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range setOpTmps {
		defer HostSQLite(qb.Registry).DropTable(t)
	}

	schema := qb.mergedSchema()
	query := qb.buildSQL(tableName, "*")
	result, err := HostSQLite(qb.Registry).Query(query, &schema)
	if err != nil {
		return TableData{}, err
	}
	return result, nil
}

// MaterializeWithColumns executes with specific column selection.
func (qb *QueryBuilder) MaterializeWithColumns(cols []columnSpec) (TableData, error) {
	tableName, ownsTmp, err := qb.ensureSource()
	if err != nil {
		return TableData{}, err
	}
	if ownsTmp {
		defer HostSQLite(qb.Registry).DropTable(tableName)
	}

	joinTmps, err := qb.ensureJoinSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range joinTmps {
		defer HostSQLite(qb.Registry).DropTable(t)
	}

	setOpTmps, err := qb.ensureSetOpSources()
	if err != nil {
		return TableData{}, err
	}
	for _, t := range setOpTmps {
		defer HostSQLite(qb.Registry).DropTable(t)
	}

	var colSQL string
	if cols == nil {
		colSQL = "*"
	} else {
		parts := make([]string, len(cols))
		for i, c := range cols {
			if c.Raw != "" {
				// Raw SQL expression (aggregate, cast, etc.)
				if c.Alias != "" {
					parts[i] = c.Raw + " AS " + quoteIdent(c.Alias)
				} else {
					parts[i] = c.Raw
				}
			} else if c.Alias != "" {
				parts[i] = quoteIdent(c.Name) + " AS " + quoteIdent(c.Alias)
			} else {
				parts[i] = quoteIdent(c.Name)
			}
		}
		colSQL = strings.Join(parts, ", ")
	}

	// Build schema hint for the result columns.
	merged := qb.mergedSchema()
	resultSchema := &merged
	if cols != nil {
		resultFields := NewOrderedMap()
		for _, c := range cols {
			outputName := c.Name
			if c.Alias != "" {
				outputName = c.Alias
			}
			if c.Raw != "" && c.Alias != "" {
				outputName = c.Alias
			}
			if c.ResultType != nil {
				resultFields.Set(outputName, NewTypeLiteral(c.ResultType))
			} else if fieldVal, ok := merged.Fields.Get(c.Name); ok {
				resultFields.Set(outputName, fieldVal)
			} else {
				resultFields.Set(outputName, NewTypeLiteral(TString))
			}
		}
		resultSchema = &RecordTypeInfo{Fields: resultFields}
	}

	query := qb.buildSQL(tableName, colSQL)
	result, err := HostSQLite(qb.Registry).Query(query, resultSchema)
	if err != nil {
		return TableData{}, err
	}
	return result, nil
}

// ensureSetOpSources ensures all set-op right-hand sources are in SQLite.
func (qb *QueryBuilder) ensureSetOpSources() ([]string, error) {
	var tmpNames []string
	for i := range qb.SetOps {
		so := &qb.SetOps[i]
		if so.Right.Source.SQLite {
			continue
		}
		tmpName, err := HostSQLite(qb.Registry).StoreTempTable(so.Right.Source)
		if err != nil {
			return tmpNames, err
		}
		so.Right.Source.SQLite = true
		so.Right.Source.TableName = tmpName
		tmpNames = append(tmpNames, tmpName)
	}
	return tmpNames, nil
}

// The query DSL words live in QueryNatives (native_query.go) and are
// surfaced through the boru:query module (modules/query.go); the
// supporting parser/exec helpers stay in this file. Aggregate
// functions (count, sum, avg, min, max) and CAST are handled
// directly in parseColumnSpec when they appear as the first element
// of a sub-list in the column spec, e.g.:
//   select [[count name cnt]] from people
//   select [[cast age integer]] from people

// clone returns a deep-enough copy of the QueryBuilder so that
// mutating one pipeline stage does not alias the slice backing arrays
// of an upstream builder. Scalar clause fields copy by value; the
// Joins and SetOps slices are duplicated.
func (qb QueryBuilder) clone() QueryBuilder {
	cp := qb
	if qb.Joins != nil {
		cp.Joins = append([]JoinClause(nil), qb.Joins...)
	}
	if qb.SetOps != nil {
		cp.SetOps = append([]SetOp(nil), qb.SetOps...)
	}
	return cp
}

// toQueryBuilder coerces a pipeline value into a QueryBuilder. It
// accepts a lazy query (the MaterializerPayload wrapper produced by
// wrapQB), cloned so upstream stages are not mutated, or a bare
// TableData source.
func toQueryBuilder(r *Registry, v Value) (QueryBuilder, error) {
	if qb, ok := unwrapQB(v); ok {
		return qb.clone(), nil
	}
	if td, ok := v.Data().(TableData); ok {
		return NewQueryBuilder(r, td), nil
	}
	return QueryBuilder{}, fmt.Errorf("argument is not a table or query")
}

// wrapQB wraps a QueryBuilder as a LAZY table Value. The builder rides
// in a MaterializerPayload under the TList tag: it satisfies the kernel
// Materializer interface (Materialize / SourceRecord), so the engine
// auto-runs the query only when rows or schema are actually needed
// (printing, iteration, AsList). No terminal word is required.
func wrapQB(qb QueryBuilder) Value {
	return NewValueRaw(TList, MaterializerPayload{M: qb})
}

// Materialize runs the accumulated query and returns concrete rows.
// Implements core.Materializer (value receiver so it satisfies the
// interface when stored by value in MaterializerPayload). A query with
// no FROM table — a `select` that was never given a `from` — errors
// here rather than silently producing nothing.
func (qb QueryBuilder) Materialize() (TableData, error) {
	if !qb.HasSource {
		return TableData{}, fmt.Errorf("select: query has no FROM table")
	}
	local := qb
	if local.Cols == nil {
		return local.materializeAll()
	}
	return local.MaterializeWithColumns(local.Cols)
}

// SourceRecord returns the query's result schema without running it.
// Implements core.Materializer. Cheap: derived from the source/join
// schema (and projected columns when present), no SQLite hit.
func (qb QueryBuilder) SourceRecord() RecordTypeInfo {
	if !qb.HasSource {
		return RecordTypeInfo{Fields: NewOrderedMap()}
	}
	local := qb
	merged := local.mergedSchema()
	if local.Cols == nil {
		return merged
	}
	fields := NewOrderedMap()
	for _, c := range local.Cols {
		name := c.Name
		if c.Alias != "" {
			name = c.Alias
		}
		if c.ResultType != nil {
			fields.Set(name, NewTypeLiteral(c.ResultType))
		} else if v, ok := merged.Fields.Get(c.Name); ok {
			fields.Set(name, v)
		} else {
			fields.Set(name, NewTypeLiteral(TString))
		}
	}
	return RecordTypeInfo{Fields: fields}
}

// resolveParenSubExprs walks a clause list and evaluates any
// parenthesized sub-expressions (scalar subqueries, nested queries) in
// a sub-engine, splicing their results back into the list. Nested
// lists are recursed into, except those that are already TableData or
// QueryBuilder values (subquery results carried as data). Shared by
// the select column list and the where/having condition list.
func resolveParenSubExprs(r *Registry, list Value) (Value, error) {
	if !list.Parent().Equal(TList) {
		return list, nil
	}
	_lst, err := AsList(list)
	if err != nil {
		return list, nil
	}
	elems := _lst.Elems()
	if len(elems) == 0 {
		return list, nil
	}

	// Recurse into nested lists that are not themselves query results.
	work := make([]Value, len(elems))
	for i, e := range elems {
		if e.Parent().Equal(TList) && !isTableOrQuery(e) {
			resolved, err := resolveParenSubExprs(r, e)
			if err != nil {
				return Value{}, err
			}
			work[i] = resolved
			continue
		}
		work[i] = e
	}

	// Fast path: no paren markers at this level.
	hasParen := false
	for _, e := range work {
		if IsOpenParen(&e) {
			hasParen = true
			break
		}
	}
	if !hasParen {
		return NewList(work), nil
	}

	// Evaluate each parenthesized group in a sub-engine.
	var out []Value
	i := 0
	for i < len(work) {
		if IsOpenParen(&work[i]) {
			depth := 1
			j := i + 1
			for j < len(work) && depth > 0 {
				if IsOpenParen(&work[j]) {
					depth++
				} else if IsCloseParen(&work[j]) {
					depth--
				}
				j++
			}
			if depth != 0 {
				return Value{}, fmt.Errorf("unmatched parenthesis in query clause")
			}
			subExpr := work[i+1 : j-1]
			sub := New(r)
			evalResult, err := sub.Run(subExpr)
			if err != nil {
				return Value{}, fmt.Errorf("query subexpression: %w", err)
			}
			out = append(out, evalResult...)
			i = j
		} else {
			out = append(out, work[i])
			i++
		}
	}

	return NewList(out), nil
}

// aggregateFuncs is the set of recognized aggregate function names.
var aggregateFuncs = map[string]bool{
	"count": true,
	"sum":   true,
	"avg":   true,
	"min":   true,
	"max":   true,
}

// columnSpec describes a column selection, with optional alias and raw SQL.
type columnSpec struct {
	Name       string // column name (empty if Raw is set)
	Alias      string // empty means no alias
	Raw        string // raw SQL expression (for aggregates, cast, etc.)
	ResultType *Type  // expected result type (zero value means inherit from source)
}

// parseColumnSpec parses the column list from a select word.
// Supports:
//   - [a, b]                     — plain columns
//   - [[a x] b]                  — column aliasing
//   - [[count name cnt]]         — aggregate with alias: COUNT("name") AS "cnt"
//   - [[count * total]]          — COUNT(*) AS "total"
//   - [[cast age integer]]       — CAST("age" AS INTEGER)
//   - [[cast age integer a]]     — CAST("age" AS INTEGER) AS "a"
func parseColumnSpec(colList Value) ([]columnSpec, error) {
	_lst, _ := AsList(colList)
	elems := _lst.Elems()
	cols := make([]columnSpec, 0, len(elems))
	for _, e := range elems {
		switch {
		case e.Parent().Equal(TAtom):
			_as6, _ := AsAtom(e)
			cols = append(cols, columnSpec{Name: _as6})
		case e.Parent().ConformsTo(TString):
			_as7, _ := AsString(e)
			cols = append(cols, columnSpec{Name: _as7})
		case IsWord(&e):
			// A word that appears in the column list without evaluation
			// is treated as a column name OR as an aggregate function name.
			_as8, _ := AsWord(&e)
			wname := _as8.Name
			cols = append(cols, columnSpec{Name: wname})
		case e.Parent().Equal(TList):
			_lst, _ := AsList(e)
			pair := _lst.Elems()
			if len(pair) < 2 {
				return nil, fmt.Errorf("select: column spec list must have at least 2 elements")
			}

			firstName := nameFromValue(pair[0])

			// Check for cast: [cast col type] or [cast col type alias]
			if firstName == "cast" {
				spec, err := parseCastSpec(pair)
				if err != nil {
					return nil, err
				}
				cols = append(cols, spec)
				continue
			}

			// Check for aggregate: [count col alias] or [sum col alias] etc.
			if aggregateFuncs[firstName] {
				spec, err := parseAggregateSpec(firstName, pair[1:])
				if err != nil {
					return nil, err
				}
				cols = append(cols, spec)
				continue
			}

			// Check for scalar subquery: [<TableData/QueryBuilder> alias]
			if isTableOrQuery(pair[0]) {
				scalar, err := resolveScalarValue(pair[0])
				if err != nil {
					return nil, fmt.Errorf("select scalar subquery: %w", err)
				}
				sqlVal, err := valueToSQL(scalar)
				if err != nil {
					return nil, fmt.Errorf("select scalar subquery: %w", err)
				}
				alias := ""
				if len(pair) >= 2 {
					alias = nameFromValue(pair[1])
				}
				cols = append(cols, columnSpec{Raw: sqlVal, Alias: alias})
				continue
			}

			// Standard [name alias] pair
			if len(pair) != 2 {
				return nil, fmt.Errorf("select: column alias must be [name alias], got %d elements", len(pair))
			}
			name := valueToColName(pair[0])
			alias := valueToColName(pair[1])
			if name == "" || alias == "" {
				return nil, fmt.Errorf("select: column alias elements must be atoms, strings, or words")
			}
			cols = append(cols, columnSpec{Name: name, Alias: alias})
		default:
			return nil, fmt.Errorf("select: unsupported column spec type: %s", e.Parent())
		}
	}
	return cols, nil
}

// nameFromValue extracts a name from an atom, string, or word value.
// Unlike valueToColName, this also recognizes unevaluated word values.
func nameFromValue(v Value) string {
	if v.Parent().Equal(TAtom) {
		_as9, _ := AsAtom(v)
		return _as9
	}
	if v.Parent().ConformsTo(TString) {
		_as10, _ := AsString(v)
		return _as10
	}
	if IsWord(&v) {
		_as11, _ := AsWord(&v)
		return _as11.Name
	}
	return ""
}

// parseAggregateSpec parses the arguments after the aggregate function name.
// remaining is the elements after the function name, e.g., for [count name cnt]
// remaining = [name, cnt].
func parseAggregateSpec(fnName string, remaining []Value) (columnSpec, error) {
	if len(remaining) == 0 || len(remaining) > 2 {
		return columnSpec{}, fmt.Errorf("%s: expected [%s col] or [%s col alias]", fnName, fnName, fnName)
	}

	fn := strings.ToUpper(fnName)
	col := nameFromValue(remaining[0])
	if col == "" {
		return columnSpec{}, fmt.Errorf("%s: expected column name", fnName)
	}

	var raw string
	if col == "*" {
		raw = fn + "(*)"
	} else {
		raw = fn + "(" + quoteIdent(col) + ")"
	}

	alias := strings.ToLower(fn) + "_" + col
	if len(remaining) == 2 {
		alias = nameFromValue(remaining[1])
	}

	return columnSpec{
		Raw:        raw,
		Alias:      alias,
		ResultType: TInteger,
	}, nil
}

// parseCastSpec parses [cast col type] or [cast col type alias] into a columnSpec.
func parseCastSpec(elems []Value) (columnSpec, error) {
	if len(elems) < 3 || len(elems) > 4 {
		return columnSpec{}, fmt.Errorf("cast: expected [cast column type] or [cast column type alias]")
	}
	col := valueToColName(elems[1])
	typeName := valueToColName(elems[2])
	if col == "" || typeName == "" {
		return columnSpec{}, fmt.Errorf("cast: column and type must be atoms or strings")
	}

	sqlType := boruTypenameToSQLType(typeName)
	raw := "CAST(" + quoteIdent(col) + " AS " + sqlType + ")"

	alias := col
	if len(elems) == 4 {
		alias = valueToColName(elems[3])
	}

	return columnSpec{
		Raw:        raw,
		Alias:      alias,
		ResultType: sqlTypeToBoruType(sqlType),
	}, nil
}

// boruTypenameToSQLType maps a boru type name string to a SQL type.
func boruTypenameToSQLType(name string) string {
	switch strings.ToLower(name) {
	case "integer", "int":
		return "INTEGER"
	case "real", "float", "number", "decimal":
		return "REAL"
	case "text", "string":
		return "TEXT"
	case "boolean", "bool":
		return "INTEGER" // SQLite stores booleans as integers
	default:
		return strings.ToUpper(name)
	}
}

// sqlTypeToBoruType maps a SQL type string back to a boru *Type.
func sqlTypeToBoruType(sqlType string) *Type {
	switch sqlType {
	case "INTEGER":
		return TInteger
	case "REAL":
		return TFloat
	case "TEXT":
		return TString
	default:
		return TString
	}
}

// valueToColName extracts the string content from an atom, string, or word value.
func valueToColName(v Value) string {
	if v.Parent().Equal(TAtom) {
		_as12, _ := AsAtom(v)
		return _as12
	}
	if v.Parent().ConformsTo(TString) {
		_as13, _ := AsString(v)
		return _as13
	}
	if IsWord(&v) {
		_as14, _ := AsWord(&v)
		return _as14.Name
	}
	return ""
}

// unwrapQB extracts a QueryBuilder from a Value, accepting both the
// wrapped form (ExtensionPayload{Body: qb} — the post-Step-5 storage
// shape so QueryBuilder satisfies Payload) and the legacy bare form.
// Returns (QueryBuilder{}, false) when v is not a query builder.
func unwrapQB(v Value) (QueryBuilder, bool) {
	// Current wrapper: the lazy MaterializerPayload produced by wrapQB.
	if mp, ok := v.Data().(MaterializerPayload); ok {
		if qb, ok := mp.M.(QueryBuilder); ok {
			return qb, true
		}
	}
	// Back-compat: the older ExtensionPayload-wrapped form.
	if ep, ok := v.Data().(ExtensionPayload); ok {
		if qb, ok := ep.Body.(QueryBuilder); ok {
			return qb, true
		}
	}
	return QueryBuilder{}, false
}

// comparisonOps maps boru comparison word names to SQL operators.
var comparisonOps = map[string]string{
	"eq":     "=",
	"neq":    "!=",
	"lt":     "<",
	"gt":     ">",
	"lte":    "<=",
	"gte":    ">=",
	"like":   "LIKE",
	"glob":   "GLOB",
	"regexp": "REGEXP",
}

// logicalOps maps boru logical word names to SQL connectors.
var logicalOps = map[string]string{
	"and": "AND",
	"or":  "OR",
}

// buildWhereClause translates a condition list into a SQL WHERE clause.
// Supported forms:
//
//	[column op value]                          — standard comparison
//	[column is null]                           — IS NULL
//	[column is not null]                       — IS NOT NULL
//	[column between value1 value2]             — BETWEEN ... AND ...
//	[column not between value1 value2]         — NOT BETWEEN ... AND ...
//	[column in [v1 v2 v3]]                     — IN (v1, v2, v3)
//	[column in (select [col] from table)]      — IN (subquery result)
//	[column not in [v1 v2 v3]]                 — NOT IN (v1, v2, v3)
//	[... and/or ...]                           — logical connectives
func buildWhereClause(condList Value) (string, error) {
	_lst, _ := AsList(condList)
	elems := _lst.Elems()
	if len(elems) == 0 {
		return "1=1", nil
	}

	var parts []string
	i := 0
	for i < len(elems) {
		// --- NOT prefix ---
		// [not col op val ...]  → NOT (col op val)
		// [not [sub-condition]] → NOT (sub-condition)
		firstCol := valueToColName(elems[i])
		if firstCol == "not" {
			i++
			if i >= len(elems) {
				return "", fmt.Errorf("incomplete condition: expected condition after not")
			}
			// If followed by a sub-list, negate the whole group.
			if elems[i].Parent().Equal(TList) {
				inner, err := buildWhereClause(elems[i])
				if err != nil {
					return "", err
				}
				parts = append(parts, "NOT ("+inner+")")
				i++
				// Check for logical connector after the NOT group.
				if i < len(elems) {
					connName := valueToColName(elems[i])
					if sqlConn, ok := logicalOps[connName]; ok {
						parts = append(parts, sqlConn)
						i++
					}
				}
				continue
			}
			// Otherwise, NOT applies to the next single condition.
			// Collect tokens for the single condition: col op val
			// (or col not in [...], col is null, etc.)
			singleCond, consumed, err := parseSingleCondition(elems, i)
			if err != nil {
				return "", err
			}
			parts = append(parts, "NOT ("+singleCond+")")
			i = consumed
			// Check for logical connector.
			if i < len(elems) {
				connName := valueToColName(elems[i])
				if sqlConn, ok := logicalOps[connName]; ok {
					parts = append(parts, sqlConn)
					i++
				}
			}
			continue
		}

		// --- Sub-list (parenthesized group) ---
		// [[col op val or col op val] and ...]  → (...) AND ...
		if elems[i].Parent().Equal(TList) {
			inner, err := buildWhereClause(elems[i])
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+inner+")")
			i++
			// Check for logical connector after the group.
			if i < len(elems) {
				connName := valueToColName(elems[i])
				if sqlConn, ok := logicalOps[connName]; ok {
					parts = append(parts, sqlConn)
					i++
				}
			}
			continue
		}

		// --- Standard condition ---
		singleCond, consumed, err := parseSingleCondition(elems, i)
		if err != nil {
			return "", err
		}
		parts = append(parts, singleCond)
		i = consumed

		// Check for logical connector.
		if i < len(elems) {
			connName := valueToColName(elems[i])
			sqlConn, ok := logicalOps[connName]
			if ok {
				parts = append(parts, sqlConn)
				i++
			}
		}
	}

	return strings.Join(parts, " "), nil
}

// parseSingleCondition parses one condition starting at elems[start].
// Returns the SQL string and the new index after the condition.
func parseSingleCondition(elems []Value, start int) (string, int, error) {
	i := start
	col := valueToColName(elems[i])
	if col == "" {
		return "", i, fmt.Errorf("expected column name, got %s", elems[i].Parent())
	}
	i++

	if i >= len(elems) {
		return "", i, fmt.Errorf("incomplete condition after column %q", col)
	}

	opName := valueToColName(elems[i])

	switch opName {
	case "is":
		// is null / is not null
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("incomplete condition: expected null or not after is")
		}
		next := valueToColName(elems[i])
		if next == "null" {
			i++
			return fmt.Sprintf("%s IS NULL", quoteIdent(col)), i, nil
		} else if next == "not" {
			i++
			if i >= len(elems) {
				return "", i, fmt.Errorf("incomplete condition: expected null after is not")
			}
			nn := valueToColName(elems[i])
			if nn != "null" {
				return "", i, fmt.Errorf("expected null after is not, got %q", nn)
			}
			i++
			return fmt.Sprintf("%s IS NOT NULL", quoteIdent(col)), i, nil
		} else {
			return "", i, fmt.Errorf("expected null or not after is, got %q", next)
		}

	case "between":
		// between value1 value2
		i++
		if i+1 >= len(elems) {
			return "", i, fmt.Errorf("between requires two values")
		}
		lo, err := valueToSQL(elems[i])
		if err != nil {
			return "", i, err
		}
		i++
		hi, err := valueToSQL(elems[i])
		if err != nil {
			return "", i, err
		}
		i++
		return fmt.Sprintf("%s BETWEEN %s AND %s", quoteIdent(col), lo, hi), i, nil

	case "in":
		// in [v1 v2 v3]
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("in requires a value list")
		}
		inSQL, err := buildInList(elems[i])
		if err != nil {
			return "", i, err
		}
		i++
		return fmt.Sprintf("%s IN (%s)", quoteIdent(col), inSQL), i, nil

	case "not":
		// not between / not in
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("incomplete condition: expected between or in after not")
		}
		next := valueToColName(elems[i])
		switch next {
		case "between":
			i++
			if i+1 >= len(elems) {
				return "", i, fmt.Errorf("not between requires two values")
			}
			lo, err := valueToSQL(elems[i])
			if err != nil {
				return "", i, err
			}
			i++
			hi, err := valueToSQL(elems[i])
			if err != nil {
				return "", i, err
			}
			i++
			return fmt.Sprintf("%s NOT BETWEEN %s AND %s", quoteIdent(col), lo, hi), i, nil
		case "in":
			i++
			if i >= len(elems) {
				return "", i, fmt.Errorf("not in requires a value list")
			}
			inSQL, err := buildInList(elems[i])
			if err != nil {
				return "", i, err
			}
			i++
			return fmt.Sprintf("%s NOT IN (%s)", quoteIdent(col), inSQL), i, nil
		default:
			return "", i, fmt.Errorf("expected between or in after not, got %q", next)
		}

	default:
		// Standard comparison: op value [collate nocase|binary|rtrim]
		sqlOp, ok := comparisonOps[opName]
		if !ok {
			return "", i, fmt.Errorf("unknown comparison operator %q", opName)
		}
		i++
		if i >= len(elems) {
			return "", i, fmt.Errorf("incomplete condition: expected value after %q", opName)
		}
		val := elems[i]
		// Handle scalar subquery result (TableData or QueryBuilder).
		val, err := resolveScalarValue(val)
		if err != nil {
			return "", i, fmt.Errorf("where scalar subquery: %w", err)
		}
		sqlVal, err := valueToSQL(val)
		if err != nil {
			return "", i, err
		}
		i++

		// Optional COLLATE modifier.
		collateSuffix := ""
		if i < len(elems) {
			next := valueToColName(elems[i])
			if strings.ToLower(next) == "collate" {
				i++
				if i >= len(elems) {
					return "", i, fmt.Errorf("collate must be followed by nocase, binary, or rtrim")
				}
				cname := strings.ToLower(valueToColName(elems[i]))
				switch cname {
				case "nocase", "binary", "rtrim":
					collateSuffix = " COLLATE " + strings.ToUpper(cname)
					i++
				default:
					return "", i, fmt.Errorf("collate must be followed by nocase, binary, or rtrim, got %q", cname)
				}
			}
		}

		return fmt.Sprintf("%s %s %s%s", quoteIdent(col), sqlOp, sqlVal, collateSuffix), i, nil
	}
}

// buildInList converts a list value to a comma-separated SQL value list.
// If the value is a table result (from a subquery), extracts the first column values.
func buildInList(v Value) (string, error) {
	if !v.Parent().Equal(TList) {
		// Single value
		sql, err := valueToSQL(v)
		if err != nil {
			return "", err
		}
		return sql, nil
	}

	// Check for subquery result: TableData or QueryBuilder.
	if td, ok := v.Data().(TableData); ok {
		return buildInListFromTable(td)
	}
	if qb, ok := unwrapQB(v); ok {
		td, err := qb.Materialize()
		if err != nil {
			return "", fmt.Errorf("in subquery: %w", err)
		}
		return buildInListFromTable(td)
	}

	_lst, _ := AsList(v)
	elems := _lst.Elems()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty IN list")
	}
	parts := make([]string, len(elems))
	for i, e := range elems {
		sql, err := valueToSQL(e)
		if err != nil {
			return "", err
		}
		parts[i] = sql
	}
	return strings.Join(parts, ", "), nil
}

// buildInListFromTable extracts the first column values from a TableData
// and returns them as a comma-separated SQL value list for use in IN clauses.
func buildInListFromTable(td TableData) (string, error) {
	cols := td.Record.Fields.Keys()
	if len(cols) == 0 {
		return "", fmt.Errorf("in subquery: result has no columns")
	}
	firstCol := cols[0]

	if len(td.Rows) == 0 {
		// Empty subquery result — use a value that matches nothing.
		return "NULL", nil
	}

	parts := make([]string, 0, len(td.Rows))
	for _, row := range td.Rows {
		m, _ := AsMap(row)
		val, ok := m.Get(firstCol)
		if !ok {
			continue
		}
		sql, err := valueToSQL(val)
		if err != nil {
			return "", fmt.Errorf("in subquery value: %w", err)
		}
		parts = append(parts, sql)
	}
	if len(parts) == 0 {
		return "NULL", nil
	}
	return strings.Join(parts, ", "), nil
}

// isTableOrQuery returns true if the Value wraps a TableData or QueryBuilder.
func isTableOrQuery(v Value) bool {
	if _, ok := v.Data().(TableData); ok {
		return true
	}
	if _, ok := unwrapQB(v); ok {
		return true
	}
	return false
}

// scalarFromTable extracts a single scalar value from a TableData that has
// exactly one row. Used for scalar subquery results.
func scalarFromTable(td TableData) (Value, error) {
	cols := td.Record.Fields.Keys()
	if len(cols) == 0 {
		return Value{}, fmt.Errorf("scalar subquery returned no columns")
	}
	if len(td.Rows) == 0 {
		return NewTypeLiteral(TNone), nil
	}
	if len(td.Rows) > 1 {
		return Value{}, fmt.Errorf("scalar subquery returned %d rows, expected 1", len(td.Rows))
	}
	_m, _ := AsMap(td.Rows[0])
	val, ok := _m.Get(cols[0])
	if !ok {
		return NewTypeLiteral(TNone), nil
	}
	return val, nil
}

// resolveScalarValue extracts a scalar value from a Value that may be a
// TableData or QueryBuilder (result of a scalar subquery).
func resolveScalarValue(v Value) (Value, error) {
	if td, ok := v.Data().(TableData); ok {
		return scalarFromTable(td)
	}
	if qb, ok := unwrapQB(v); ok {
		td, err := qb.Materialize()
		if err != nil {
			return Value{}, fmt.Errorf("scalar subquery: %w", err)
		}
		return scalarFromTable(td)
	}
	return v, nil
}

// valueToSQL converts a Value to a SQL literal string.
func valueToSQL(v Value) (string, error) {
	switch {
	case v.Parent().ConformsTo(TString):
		_as23, _ := AsString(v)
		return "'" + strings.ReplaceAll(_as23, "'", "''") + "'", nil
	case v.Parent().ConformsTo(TInteger):
		_as24, _ := AsInteger(v)
		return fmt.Sprintf("%d", _as24), nil
	case v.Parent().ConformsTo(TBoolean):
		_as25, _ := AsBoolean(v)
		if _as25 {
			return "'true'", nil
		}
		return "'false'", nil
	case v.Parent().Equal(TAtom):
		_as26, _ := AsAtom(v)
		return "'" + strings.ReplaceAll(_as26, "'", "''") + "'", nil
	case v.Parent().Equal(TNone):
		return "NULL", nil
	case v.Parent().Equal(TWord):
		_as27, _ := AsWord(&v)
		return "'" + strings.ReplaceAll(_as27.Name, "'", "''") + "'", nil
	default:
		return "", fmt.Errorf("unsupported value type in condition: %s", v.Parent())
	}
}

// buildGroupByClause translates a column list into a SQL GROUP BY clause.
func buildGroupByClause(colList Value) (string, error) {
	_lst, _ := AsList(colList)
	elems := _lst.Elems()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty group by column list")
	}
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		name := valueToColName(e)
		if name == "" {
			return "", fmt.Errorf("groupby: expected column name, got %s", e.Parent())
		}
		parts = append(parts, quoteIdent(name))
	}
	return strings.Join(parts, ", "), nil
}

// buildJoinCondition translates a condition list into a SQL ON clause.
// Supports dot-separated qualified names: [a.id eq b.id]
func buildJoinCondition(condList Value) (string, error) {
	_lst, _ := AsList(condList)
	elems := _lst.Elems()
	if len(elems) == 0 {
		return "1=1", nil
	}

	var parts []string
	i := 0
	for i < len(elems) {
		lhs := valueToColName(elems[i])
		if lhs == "" {
			return "", fmt.Errorf("expected column name, got %s", elems[i].Parent())
		}
		i++

		if i >= len(elems) {
			return "", fmt.Errorf("incomplete join condition after %q", lhs)
		}

		opName := valueToColName(elems[i])
		sqlOp, ok := comparisonOps[opName]
		if !ok {
			return "", fmt.Errorf("unknown comparison operator %q in join condition", opName)
		}
		i++

		if i >= len(elems) {
			return "", fmt.Errorf("incomplete join condition: expected value after %q", opName)
		}

		rhs := valueToColName(elems[i])
		if rhs == "" {
			return "", fmt.Errorf("expected column name on right side of join condition, got %s", elems[i].Parent())
		}
		i++

		parts = append(parts, fmt.Sprintf("%s %s %s", quoteJoinCol(lhs), sqlOp, quoteJoinCol(rhs)))

		// Check for logical connector.
		if i < len(elems) {
			connName := valueToColName(elems[i])
			sqlConn, ok := logicalOps[connName]
			if ok {
				parts = append(parts, sqlConn)
				i++
			}
		}
	}

	return strings.Join(parts, " "), nil
}

// quoteJoinCol quotes a column reference that may be dot-qualified (table.col).
func quoteJoinCol(name string) string {
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		return quoteIdent(name[:idx]) + "." + quoteIdent(name[idx+1:])
	}
	return quoteIdent(name)
}

// buildOrderClause translates a column list into a SQL ORDER BY clause.
// Supports:
//   - [col1, col2]                   — multiple columns
//   - [col1 asc, col2 desc]          — with direction
//   - [col1 asc nulls first]         — with nulls placement
//   - [1, 2 desc]                    — positional (1-based)
func buildOrderClause(colList Value) (string, error) {
	_lst, _ := AsList(colList)
	elems := _lst.Elems()
	if len(elems) == 0 {
		return "", fmt.Errorf("empty order column list")
	}

	isModifier := func(name string) (string, bool) {
		switch name {
		case "asc":
			return "ASC", true
		case "desc":
			return "DESC", true
		case "nulls":
			return "NULLS", true
		case "first":
			return "FIRST", true
		case "last":
			return "LAST", true
		case "collate":
			return "COLLATE", true
		default:
			return "", false
		}
	}

	collations := map[string]string{
		"nocase": "NOCASE",
		"binary": "BINARY",
		"rtrim":  "RTRIM",
	}

	var parts []string
	i := 0
	for i < len(elems) {
		e := elems[i]

		if e.Parent().ConformsTo(TInteger) {
			_as28, _ := AsInteger(e)
			parts = append(parts, fmt.Sprintf("%d", _as28))
			i++
			continue
		}

		name := valueToColName(e)
		if name == "" {
			return "", fmt.Errorf("expected column name, position, or modifier, got %s", e.Parent())
		}
		lower := strings.ToLower(name)

		if sql, ok := isModifier(lower); ok {
			if len(parts) == 0 {
				return "", fmt.Errorf("%s without preceding column name", lower)
			}
			if lower == "nulls" {
				i++
				if i >= len(elems) {
					return "", fmt.Errorf("nulls must be followed by first or last")
				}
				next := strings.ToLower(valueToColName(elems[i]))
				if next != "first" && next != "last" {
					return "", fmt.Errorf("nulls must be followed by first or last, got %q", next)
				}
				parts[len(parts)-1] += " " + sql + " " + strings.ToUpper(next)
			} else if lower == "collate" {
				i++
				if i >= len(elems) {
					return "", fmt.Errorf("collate must be followed by nocase, binary, or rtrim")
				}
				next := strings.ToLower(valueToColName(elems[i]))
				colSQL, ok := collations[next]
				if !ok {
					return "", fmt.Errorf("collate must be followed by nocase, binary, or rtrim, got %q", next)
				}
				parts[len(parts)-1] += " " + sql + " " + colSQL
			} else {
				parts[len(parts)-1] += " " + sql
			}
		} else {
			parts = append(parts, quoteIdent(name))
		}
		i++
	}

	return strings.Join(parts, ", "), nil
}
