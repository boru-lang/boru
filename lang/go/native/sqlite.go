//go:build !js

package native

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterFunction("regexp", &sqlite.FunctionImpl{
		NArgs:         2,
		Deterministic: true,
		Scalar: func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil || args[1] == nil {
				return nil, nil
			}
			pattern, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("regexp: pattern must be a string")
			}
			value, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("regexp: value must be a string")
			}
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				return nil, fmt.Errorf("regexp: %w", err)
			}
			if matched {
				return int64(1), nil
			}
			return int64(0), nil
		},
	})
}

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

// SQLiteStore manages an in-memory SQLite database for table storage.
type SQLiteStore struct {
	mu     sync.Mutex
	db     *sql.DB
	tables map[string]RecordTypeInfo // schema per table name
	seq    int                       // auto-incrementing counter for temp tables
}

// NewSQLiteStore opens a shared in-memory SQLite database.
func NewSQLiteStore() (*SQLiteStore, error) {
	db, err := w8SQLOpen("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	return &SQLiteStore{
		db:     db,
		tables: make(map[string]RecordTypeInfo),
	}, nil
}

// Close shuts down the SQLite database.
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// StoreTable creates a SQLite table and inserts all rows from a TableData.
// Returns the table name used in SQLite.
func (s *SQLiteStore) StoreTable(name string, td TableData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	columns := td.Record.Fields.Keys()
	if len(columns) == 0 {
		return nil
	}

	// Drop existing table if it exists.
	_, _ = s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))

	// Create table with typed columns based on the record schema.
	colDefs := make([]string, len(columns))
	colTypes := make([]*Type, len(columns))
	for i, col := range columns {
		fieldVal, _ := td.Record.Fields.Get(col)
		colTypes[i] = fieldVal.Parent()
		colDefs[i] = quoteIdent(col) + " " + boruTypeToSQLType(fieldVal.Parent())
	}
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(name), strings.Join(colDefs, ", "))
	if _, err := s.db.Exec(createSQL); err != nil {
		return fmt.Errorf("sqlite create: %w", err)
	}

	// Insert rows with native typed values.
	if len(td.Rows) > 0 {
		placeholders := make([]string, len(columns))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoteIdent(name),
			joinQuoted(columns),
			strings.Join(placeholders, ", "))

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("sqlite begin: %w", err)
		}
		stmt, err := tx.Prepare(insertSQL)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite prepare: %w", err)
		}
		defer stmt.Close()

		for _, row := range td.Rows {
			m, err := AsMutableMap(row)
			if err != nil {
				continue
			}
			vals := make([]interface{}, len(columns))
			for i, col := range columns {
				v, exists := m.Get(col)
				if !exists {
					vals[i] = nil
				} else {
					vals[i] = boruValueToSQLParam(v, colTypes[i])
				}
			}
			if _, err := stmt.Exec(vals...); err != nil {
				tx.Rollback()
				return fmt.Errorf("sqlite insert: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlite commit: %w", err)
		}
	}

	s.tables[name] = td.Record
	return nil
}

// StoreTempTable stores a table under an auto-generated name and returns
// the name. Used for non-SQLite tables that need querying.
func (s *SQLiteStore) StoreTempTable(td TableData) (string, error) {
	s.mu.Lock()
	s.seq++
	name := fmt.Sprintf("_tmp_%d", s.seq)
	s.mu.Unlock()
	return name, s.StoreTable(name, td)
}

// Query executes a SELECT query and returns results as TableData.
// The optional schema provides type hints for reading values back with
// proper boru types. If nil, the result schema is inferred from SQLite
// column types reported by the driver.
func (s *SQLiteStore) Query(querySQL string, schema *RecordTypeInfo) (TableData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(querySQL)
	if err != nil {
		return TableData{}, fmt.Errorf("sqlite query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return TableData{}, fmt.Errorf("sqlite columns: %w", err)
	}

	// Resolve the type for each result column.
	colTypes := make([]*Type, len(cols))
	for i, col := range cols {
		colTypes[i] = TString // default
		if schema != nil {
			if fieldVal, ok := schema.Fields.Get(col); ok {
				colTypes[i] = fieldVal.Parent()
			}
		}
	}

	// Build record schema for the result.
	fields := NewOrderedMap()
	for i, col := range cols {
		fields.Set(col, NewTypeLiteral(colTypes[i]))
	}
	record := RecordTypeInfo{Fields: fields}

	// Scan using interface{} to get native SQLite types.
	var resultRows []Value
	scanDest := make([]interface{}, len(cols))
	for i := range scanDest {
		scanDest[i] = new(interface{})
	}

	for rows.Next() {
		if err := rows.Scan(scanDest...); err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return TableData{}, fmt.Errorf("sqlite scan: %w", err)
		}
		om := NewOrderedMap()
		for i, col := range cols {
			raw := *(scanDest[i].(*interface{}))
			om.Set(col, sqlResultToBoruValue(raw, colTypes[i]))
		}
		resultRows = append(resultRows, NewMap(om))
	}
	if err := rows.Err(); err != nil {
		return TableData{}, fmt.Errorf("sqlite rows: %w", err)
	}

	return TableData{Record: record, Rows: resultRows}, nil
}

// boruValueToSQLParam converts a boru Value to a Go value suitable for
// a SQL parameter placeholder, respecting the target column type.
func boruValueToSQLParam(v Value, colType *Type) interface{} {
	if v.Parent().Equal(TNone) {
		return nil
	}

	switch {
	case colType.ConformsTo(TInteger):
		// Column wants INTEGER. Coerce the value.
		if v.Parent().ConformsTo(TInteger) {
			_as0, _ := AsInteger(v)
			return _as0
		}
		// String that looks numeric → parse it.
		if v.Parent().ConformsTo(TString) {
			_as1, _ := AsString(v)
			if n, err := strconv.ParseInt(_as1, 10, 64); err == nil {
				return n
			}
		}
		if v.Parent().ConformsTo(TBoolean) {
			_as2, _ := AsBoolean(v)
			if _as2 {
				return int64(1)
			}
			return int64(0)
		}
		// Fallback: store as text.
		return ValToString(v)

	case colType.ConformsTo(TNumber):
		// Column wants REAL.
		if v.Parent().ConformsTo(TFloat) {
			_as3, _ := AsFloat(v)
			return _as3
		}
		if v.Parent().ConformsTo(TInteger) {
			_as4, _ := AsInteger(v)
			return float64(_as4)
		}
		if v.Parent().ConformsTo(TString) {
			_as5, _ := AsString(v)
			if f, err := strconv.ParseFloat(_as5, 64); err == nil {
				return f
			}
		}
		return ValToString(v)

	case colType.ConformsTo(TBoolean):
		// Column stored as INTEGER (0/1).
		if v.Parent().ConformsTo(TBoolean) {
			_as6, _ := AsBoolean(v)
			if _as6 {
				return int64(1)
			}
			return int64(0)
		}
		if v.Parent().ConformsTo(TString) {
			_as7, _ := AsString(v)
			if _as7 == "true" {
				return int64(1)
			}
			return int64(0)
		}
		return ValToString(v)

	default:
		// TEXT column.
		if v.Parent().ConformsTo(TString) {
			_as8, _ := AsString(v)
			return _as8
		}
		return ValToString(v)
	}
}

// sqlResultToBoruValue converts a raw SQLite result value to the appropriate
// boru Value based on the expected column type.
func sqlResultToBoruValue(raw interface{}, colType *Type) Value {
	if raw == nil {
		return NewValueRaw(TNone, nil)
	}

	switch {
	case colType.ConformsTo(TInteger):
		return NewInteger(toInt64(raw))
	case colType.ConformsTo(TNumber):
		return NewFloat(toFloat64(raw))
	case colType.ConformsTo(TBoolean):
		return NewBoolean(toInt64(raw) != 0)
	default:
		return NewString(toString(raw))
	}
}

// toInt64 coerces a database/sql scanned value to int64.
func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(x), 10, 64)
		return n
	default:
		return 0
	}
}

// toFloat64 coerces a database/sql scanned value to float64.
func toFloat64(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case []byte:
		f, _ := strconv.ParseFloat(string(x), 64)
		return f
	default:
		return 0
	}
}

// toString coerces a database/sql scanned value to string.
func toString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// HasTable reports whether a table with the given name exists in the store.
func (s *SQLiteStore) HasTable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tables[name]
	return ok
}

// DropTable removes a table from the SQLite store.
func (s *SQLiteStore) DropTable(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(name)))
	delete(s.tables, name)
}
