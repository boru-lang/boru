//go:build js

package native

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
)

// boruTypeToSQLType maps a boru field type to a SQLite column type.
func boruTypeToSQLType(t *Type) string {
	switch {
	case t.ConformsTo(TInteger):
		return "INTEGER"
	case t.ConformsTo(TFloat):
		return "REAL"
	case t.ConformsTo(TNumber):
		return "REAL"
	case t.ConformsTo(TBoolean):
		return "INTEGER"
	default:
		return "TEXT"
	}
}

// SQLiteStore bridges to sql.js running in the browser.
type SQLiteStore struct {
	mu     sync.Mutex
	db     js.Value
	tables map[string]RecordTypeInfo
	seq    int
}

// NewSQLiteStore creates a sql.js in-memory database.
// Requires sql.js to be loaded and window.__sqlJS set before Go starts.
func NewSQLiteStore() (*SQLiteStore, error) {
	sqlJS := js.Global().Get("__sqlJS")
	if sqlJS.IsUndefined() || sqlJS.IsNull() {
		// sql.js not loaded — return nil (features requiring SQLite will be unavailable)
		return nil, nil
	}

	db := sqlJS.Get("Database").New()

	// Register a regexp function matching the native build.
	regexpFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 || args[0].IsNull() || args[1].IsNull() {
			return nil
		}
		pattern := args[0].String()
		value := args[1].String()
		re := js.Global().Get("RegExp").New(pattern)
		if re.Call("test", value).Bool() {
			return 1
		}
		return 0
	})
	db.Call("create_function", "regexp", regexpFn)

	return &SQLiteStore{
		db:     db,
		tables: make(map[string]RecordTypeInfo),
	}, nil
}

// Close shuts down the database.
func (s *SQLiteStore) Close() error {
	if !s.db.IsUndefined() && !s.db.IsNull() {
		s.db.Call("close")
	}
	return nil
}

// StoreTable creates a table and inserts all rows.
func (s *SQLiteStore) StoreTable(name string, td TableData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	columns := td.Record.Fields.Keys()
	if len(columns) == 0 {
		return nil
	}

	// Drop existing table.
	s.db.Call("run", fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))

	// Create table with typed columns.
	colDefs := make([]string, len(columns))
	colTypes := make([]*Type, len(columns))
	for i, col := range columns {
		fieldVal, _ := td.Record.Fields.Get(col)
		colTypes[i] = ParentOf(&fieldVal)
		colDefs[i] = quoteIdent(col) + " " + boruTypeToSQLType(ParentOf(&fieldVal))
	}
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(name), strings.Join(colDefs, ", "))
	s.db.Call("run", createSQL)

	// Insert rows.
	if len(td.Rows) > 0 {
		placeholders := make([]string, len(columns))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoteIdent(name),
			joinQuoted(columns),
			strings.Join(placeholders, ", "))

		for _, row := range td.Rows {
			m, err := AsMutableMap(row)
			if err != nil {
				continue
			}
			params := js.Global().Get("Array").New(len(columns))
			for i, col := range columns {
				v, exists := m.Get(col)
				if !exists {
					params.SetIndex(i, js.Null())
				} else {
					params.SetIndex(i, boruValueToJSParam(v, colTypes[i]))
				}
			}
			s.db.Call("run", insertSQL, params)
		}
	}

	s.tables[name] = td.Record
	return nil
}

// StoreTempTable stores a table under an auto-generated name.
func (s *SQLiteStore) StoreTempTable(td TableData) (string, error) {
	s.mu.Lock()
	s.seq++
	name := fmt.Sprintf("_tmp_%d", s.seq)
	s.mu.Unlock()
	return name, s.StoreTable(name, td)
}

// Query executes a SELECT and returns results as TableData.
func (s *SQLiteStore) Query(querySQL string, schema *RecordTypeInfo) (TableData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := s.db.Call("exec", querySQL)
	if results.Length() == 0 {
		// Empty result set.
		fields := NewOrderedMap()
		if schema != nil {
			for _, k := range schema.Fields.Keys() {
				v, _ := schema.Fields.Get(k)
				fields.Set(k, v)
			}
		}
		return TableData{Record: RecordTypeInfo{Fields: fields}}, nil
	}

	result := results.Index(0)
	jsCols := result.Get("columns")
	jsVals := result.Get("values")
	numCols := jsCols.Length()

	cols := make([]string, numCols)
	colTypes := make([]*Type, numCols)
	for i := 0; i < numCols; i++ {
		cols[i] = jsCols.Index(i).String()
		colTypes[i] = TString // default
		if schema != nil {
			if fieldVal, ok := schema.Fields.Get(cols[i]); ok {
				colTypes[i] = ParentOf(&fieldVal)
			}
		}
	}

	// Build record schema.
	fields := NewOrderedMap()
	for i, col := range cols {
		fields.Set(col, NewTypeLiteral(colTypes[i]))
	}
	record := RecordTypeInfo{Fields: fields}

	// Convert rows.
	numRows := jsVals.Length()
	resultRows := make([]Value, 0, numRows)
	for r := 0; r < numRows; r++ {
		jsRow := jsVals.Index(r)
		om := NewOrderedMap()
		for i, col := range cols {
			jsVal := jsRow.Index(i)
			om.Set(col, jsValueToBoru(jsVal, colTypes[i]))
		}
		resultRows = append(resultRows, NewMap(om))
	}

	return TableData{Record: record, Rows: resultRows}, nil
}

// HasTable reports whether a table exists.
func (s *SQLiteStore) HasTable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tables[name]
	return ok
}

// DropTable removes a table.
func (s *SQLiteStore) DropTable(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Call("run", fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))
	delete(s.tables, name)
}

// boruValueToJSParam converts a boru Value to a JS value for sql.js binding.
func boruValueToJSParam(v Value, colType *Type) any {
	if ParentOf(&v).Equal(TNone) {
		return js.Null()
	}
	switch {
	case colType.ConformsTo(TInteger):
		if ParentOf(&v).ConformsTo(TInteger) {
			_as0, _ := AsInteger(v)
			return _as0
		}
		if ParentOf(&v).ConformsTo(TString) {
			_as1, _ := AsString(v)
			if n, err := strconv.ParseInt(_as1, 10, 64); err == nil {
				return n
			}
		}
		if ParentOf(&v).ConformsTo(TBoolean) {
			_as2, _ := AsBoolean(v)
			if _as2 {
				return 1
			}
			return 0
		}
		return ValToString(v)
	case colType.ConformsTo(TNumber):
		if ParentOf(&v).ConformsTo(TFloat) {
			_as3, _ := AsFloat(v)
			return _as3
		}
		if ParentOf(&v).ConformsTo(TInteger) {
			_as4, _ := AsInteger(v)
			return float64(_as4)
		}
		if ParentOf(&v).ConformsTo(TString) {
			_as5, _ := AsString(v)
			if f, err := strconv.ParseFloat(_as5, 64); err == nil {
				return f
			}
		}
		return ValToString(v)
	case colType.ConformsTo(TBoolean):
		if ParentOf(&v).ConformsTo(TBoolean) {
			_as6, _ := AsBoolean(v)
			if _as6 {
				return 1
			}
			return 0
		}
		if ParentOf(&v).ConformsTo(TString) {
			_as7, _ := AsString(v)
			if _as7 == "true" {
				return 1
			}
			return 0
		}
		return ValToString(v)
	default:
		if ParentOf(&v).ConformsTo(TString) {
			_as8, _ := AsString(v)
			return _as8
		}
		return ValToString(v)
	}
}

// jsValueToBoru converts a sql.js result value to a boru Value.
func jsValueToBoru(v js.Value, colType *Type) Value {
	if v.IsNull() || v.IsUndefined() {
		return NewValueRaw(TNone, nil)
	}
	switch {
	case colType.ConformsTo(TInteger):
		return NewInteger(int64(v.Float()))
	case colType.ConformsTo(TNumber):
		return NewFloat(v.Float())
	case colType.ConformsTo(TBoolean):
		return NewBoolean(v.Float() != 0)
	default:
		return NewString(v.String())
	}
}
