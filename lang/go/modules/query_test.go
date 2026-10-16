package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// queryRegistry returns a registry with the boru:query module installed
// and a parse func wired, plus an in-memory "people" table registered in
// the context store.
func queryRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallQueryExports(r); err != nil {
		t.Fatal(err)
	}
	makePeopleTable(r)
	return r
}

// makePeopleTable registers a "people" table: name, age, city.
func makePeopleTable(r *native.Registry) {
	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("age", native.NewTypeLiteral(native.TInteger))
	fields.Set("city", native.NewTypeLiteral(native.TString))
	rec := native.RecordTypeInfo{Fields: fields}

	mkRow := func(name string, age int64, city string) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("age", native.NewInteger(age))
		om.Set("city", native.NewString(city))
		return native.NewMap(om)
	}

	td := native.TableData{
		Record: rec,
		Rows: []native.Value{
			mkRow("Alice", 30, "London"),
			mkRow("Bob", 25, "Paris"),
			mkRow("Carol", 40, "London"),
			mkRow("Dave", 18, "Berlin"),
		},
	}
	r.ContextSet("people", native.NewValueRaw(native.TList, td))
}

// makeVisitsTable registers a "visits" table: who, place.
func makeVisitsTable(r *native.Registry) {
	fields := native.NewOrderedMap()
	fields.Set("who", native.NewTypeLiteral(native.TString))
	fields.Set("place", native.NewTypeLiteral(native.TString))
	rec := native.RecordTypeInfo{Fields: fields}

	mkRow := func(who, place string) native.Value {
		om := native.NewOrderedMap()
		om.Set("who", native.NewString(who))
		om.Set("place", native.NewString(place))
		return native.NewMap(om)
	}

	td := native.TableData{
		Record: rec,
		Rows: []native.Value{
			mkRow("Alice", "Museum"),
			mkRow("Bob", "Park"),
			mkRow("Zoe", "Cafe"), // no matching person
		},
	}
	r.ContextSet("visits", native.NewValueRaw(native.TList, td))
}

func runQuerySrc(t *testing.T, r *native.Registry, src string) ([]native.Value, error) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return native.NewTop(r).Run(values)
}

// materialize forces a query result Value to concrete TableData. The
// query is lazy (a MaterializerPayload), so it runs here; an already
// concrete TableData is returned directly.
func materialize(t *testing.T, v native.Value) native.TableData {
	t.Helper()
	if mp, ok := v.Data().(native.MaterializerPayload); ok {
		td, err := mp.M.Materialize()
		if err != nil {
			t.Fatalf("materialize: %v", err)
		}
		return td
	}
	if td, ok := v.Data().(native.TableData); ok {
		return td
	}
	t.Fatalf("expected a lazy query or TableData, got %T", v.Data())
	return native.TableData{}
}

func rowCount(t *testing.T, src string) int {
	t.Helper()
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if len(result) != 1 {
		t.Fatalf("%q: expected 1 result, got %d", src, len(result))
	}
	return len(materialize(t, result[0]).Rows)
}

// --- Module structure ---

func TestQueryModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildQueryModule(r)
	if err != nil {
		t.Fatal(err)
	}
	exp, ok := desc.Exports["Query"]
	if !ok {
		t.Fatal("missing 'query' export map")
	}
	for _, w := range []string{"from", "where", "select", "order", "group", "having", "limit", "offset", "distinct"} {
		if _, ok := exp.Get(w); !ok {
			t.Errorf("missing export query.%s", w)
		}
	}
}

// --- select * ---

func TestQuerySelectStar(t *testing.T) {
	if got := rowCount(t, `Query.select [] Query.from people`); got != 4 {
		t.Errorf("select [] (all rows): expected 4, got %d", got)
	}
}

// --- where ---

func TestQueryWhereFilter(t *testing.T) {
	if got := rowCount(t, `Query.select [] Query.from people Query.where [age gt 25]`); got != 2 {
		t.Errorf("where age>25: expected 2 (Alice,Carol), got %d", got)
	}
}

// --- projection + alias ---

func TestQuerySelectColumns(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [name age] Query.from people`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	cols := td.Record.Fields.Keys()
	if len(cols) != 2 || cols[0] != "name" || cols[1] != "age" {
		t.Errorf("expected [name age], got %v", cols)
	}
}

// --- order + limit ---

func TestQueryOrderLimit(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [name age] Query.from people Query.order [age desc] Query.limit 2`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(td.Rows))
	}
	first, _ := native.AsMap(td.Rows[0])
	nameVal, _ := first.Get("name")
	if got, _ := native.AsString(nameVal); got != "Carol" {
		t.Errorf("expected top row Carol (age 40), got %q", got)
	}
}

// --- group + having ---

func TestQueryGroupHaving(t *testing.T) {
	r := queryRegistry(t)
	// Cities with more than one person: London (Alice, Carol).
	src := `Query.select [city [count city cnt]] Query.from people Query.group [city] Query.having [cnt gt 1]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 1 {
		t.Fatalf("expected 1 group (London), got %d", len(td.Rows))
	}
}

// --- distinct ---

func TestQueryDistinct(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [city] Query.from people Query.distinct`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 distinct cities, got %d", len(td.Rows))
	}
}

// --- join ---

func TestQueryJoinOn(t *testing.T) {
	r := queryRegistry(t)
	makeVisitsTable(r)
	// people JOIN visits ON people.name = visits.who
	src := `Query.select [name place] Query.from people Query.join visits Query.on [name eq who]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 joined rows (Alice, Bob), got %d", len(td.Rows))
	}
}

// --- set operation ---

func TestQueryUnion(t *testing.T) {
	r := queryRegistry(t)
	// Londoners UNION Berliners.
	src := `Query.select [] Query.from people Query.where [city eq 'London'] Query.union (Query.select [] Query.from people Query.where [city eq 'Berlin'])`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows (Alice, Carol, Dave), got %d", len(td.Rows))
	}
}

// --- negative: unknown table ---

func TestQueryUnknownTable(t *testing.T) {
	r := queryRegistry(t)
	_, err := runQuerySrc(t, r, `Query.select [] Query.from nonexistent`)
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

// --- negative: select on a non-table ---

func TestQueryFromNonTable(t *testing.T) {
	r := queryRegistry(t)
	r.ContextSet("notable", native.NewInteger(42))
	_, err := runQuerySrc(t, r, `Query.select [] Query.from notable`)
	if err == nil {
		t.Fatal("expected error when source is not a table")
	}
}

// --- lazy materialization ---

// TestQueryLazyPrintsAsTable confirms a select-first query is lazy: the
// value carries a query, and it renders as a formatted table when its
// String() is taken (the print path), with no explicit run word.
func TestQueryLazyPrintsAsTable(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [name] Query.from people Query.where [age gt 35]`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result[0].Data().(native.MaterializerPayload); !ok {
		t.Fatalf("expected a lazy MaterializerPayload, got %T", result[0].Data())
	}
	rendered := result[0].String()
	if !strings.Contains(rendered, "Carol") {
		t.Errorf("lazy query did not render its row: %q", rendered)
	}
}

// TestQueryNoFromErrors confirms `from` is required: a select with no
// from errors when it is materialized.
func TestQueryNoFromErrors(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [name]`)
	if err != nil {
		t.Fatalf("seeding select should not error eagerly: %v", err)
	}
	mp, ok := result[0].Data().(native.MaterializerPayload)
	if !ok {
		t.Fatalf("expected a lazy query, got %T", result[0].Data())
	}
	if _, mErr := mp.M.Materialize(); mErr == nil {
		t.Fatal("expected a 'no FROM' error when materializing a sourceless select")
	}
}

// --- destructuring DX: unpack lifts query words to bare names ---

// TestQueryUnpackBareWords is the headline DX use case: after importing
// boru:query, destructure the words so the whole pipeline reads like SQL with
// no `query.` prefix. The bound module wrappers preserve their inner natives'
// QuoteArgs (from's bare table name) and NoEvalArgs (the where/select clause
// lists) because InstallDef rebinds a trivial-delegation wrapper to the inner
// native's real signatures — exactly what dot-access dispatches against.
func TestQueryUnpackBareWords(t *testing.T) {
	r := queryRegistry(t)
	src := `unpack [select from where] Query
	        select [name age] from people where [age gt 25]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("bare-word pipeline failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("where age>25: expected 2 rows (Alice, Carol), got %d", len(td.Rows))
	}
	cols := td.Record.Fields.Keys()
	if len(cols) != 2 || cols[0] != "name" || cols[1] != "age" {
		t.Errorf("expected projection [name age], got %v", cols)
	}
}

// TestQueryUnpackClauseCoverage exercises a full bare-word pipeline across the
// clause families (order, group/having, distinct, aggregates) to confirm the
// rebinding preserves each inner native's arg-handling, not just where/select.
func TestQueryUnpackClauseCoverage(t *testing.T) {
	cases := []struct {
		name string
		src  string
		rows int
	}{
		{"order", `unpack [select from where order] Query  select [] from people where [city eq 'London'] order [age desc]`, 2},
		{"group-having", `unpack [select from group having] Query  select [city [count city cnt]] from people group [city] having [cnt gt 1]`, 1},
		{"distinct", `unpack [select from distinct] Query  select [city] from people distinct`, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := queryRegistry(t)
			result, err := runQuerySrc(t, r, c.src)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := len(materialize(t, result[0]).Rows); got != c.rows {
				t.Errorf("%s: expected %d rows, got %d", c.name, c.rows, got)
			}
		})
	}
}

// --- where: full comparison-operator coverage ---
// (Ported from the pre-module global-word tests in lang/go/native — the
// unqualified `from people where […]` surface was removed when query
// moved into this module's sub-registry.)

func TestQueryWhereOperators(t *testing.T) {
	// people: Alice 30 London, Bob 25 Paris, Carol 40 London, Dave 18 Berlin.
	cases := []struct {
		name string
		src  string
		rows int
	}{
		{"lt", `Query.select [] Query.from people Query.where [age lt 30]`, 2},
		{"gte", `Query.select [] Query.from people Query.where [age gte 30]`, 2},
		{"lte", `Query.select [] Query.from people Query.where [age lte 25]`, 2},
		{"neq", `Query.select [] Query.from people Query.where [name neq 'Alice']`, 3},
		{"eq-none-match", `Query.select [] Query.from people Query.where [age gt 99]`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rowCount(t, c.src); got != c.rows {
				t.Errorf("%s: expected %d rows, got %d", c.src, c.rows, got)
			}
		})
	}
}

// TestQueryWhereUnknownColumn pins the negative: a filter naming a
// column the table doesn't have errors at materialization, not silently.
func TestQueryWhereUnknownColumn(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [] Query.from people Query.where [bogus gt 1]`)
	if err != nil {
		t.Fatalf("building the query should not error eagerly: %v", err)
	}
	mp, ok := result[0].Data().(native.MaterializerPayload)
	if !ok {
		t.Fatalf("expected a lazy query, got %T", result[0].Data())
	}
	if _, mErr := mp.M.Materialize(); mErr == nil {
		t.Fatal("expected an error materializing a filter on an unknown column")
	}
}

// --- order: ascending (the bare-column form, no desc flag) ---

func TestQueryOrderAscending(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [name age] Query.from people Query.order [age]`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 4 {
		t.Fatalf("expected all 4 rows, got %d", len(td.Rows))
	}
	first, _ := native.AsMap(td.Rows[0])
	nameVal, _ := first.Get("name")
	if got, _ := native.AsString(nameVal); got != "Dave" {
		t.Errorf("ascending order: expected youngest (Dave, 18) first, got %q", got)
	}
}

// --- offset, alone and with limit ---

func TestQueryOffset(t *testing.T) {
	if got := rowCount(t, `Query.select [] Query.from people Query.order [age desc] Query.offset 1`); got != 3 {
		t.Errorf("offset 1: expected 3 rows, got %d", got)
	}
}

func TestQueryLimitOffsetPagination(t *testing.T) {
	r := queryRegistry(t)
	// Page 2 of size 2 by descending age: full order is Carol(40),
	// Alice(30), Bob(25), Dave(18) — skip 1, take 2 → Alice, Bob.
	src := `Query.select [name] Query.from people Query.order [age desc] Query.limit 2 Query.offset 1`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Fatalf("limit 2 offset 1: expected 2 rows, got %d", len(td.Rows))
	}
	first, _ := native.AsMap(td.Rows[0])
	nameVal, _ := first.Get("name")
	if got, _ := native.AsString(nameVal); got != "Alice" {
		t.Errorf("expected page to start at Alice (2nd by age desc), got %q", got)
	}
}

// --- join … using (shared column name) ---

func TestQueryJoinUsing(t *testing.T) {
	r := queryRegistry(t)
	// scores shares the `name` column with people.
	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("score", native.NewTypeLiteral(native.TInteger))
	rec := native.RecordTypeInfo{Fields: fields}
	mkRow := func(name string, score int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("score", native.NewInteger(score))
		return native.NewMap(om)
	}
	td := native.TableData{Record: rec, Rows: []native.Value{
		mkRow("Alice", 95),
		mkRow("Bob", 88),
	}}
	r.ContextSet("scores", native.NewValueRaw(native.TList, td))

	src := `Query.select [name score] Query.from people Query.join scores Query.using [name]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(materialize(t, result[0]).Rows); got != 2 {
		t.Errorf("join using [name]: expected 2 rows (Alice, Bob), got %d", got)
	}
}

// TestQueryAsListMaterializes pins the list-projection path: AsList on
// a lazy query value materializes it and exposes the rows, so list
// words can consume a query result directly.
func TestQueryAsListMaterializes(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `Query.select [] Query.from people`)
	if err != nil {
		t.Fatal(err)
	}
	lst, lerr := native.AsList(result[0])
	if lerr != nil {
		t.Fatalf("AsList did not accept a lazy query value: %v", lerr)
	}
	if got := len(lst.Slice()); got != 4 {
		t.Errorf("expected 4 rows via AsList, got %d", got)
	}
}

// --- remaining set operations: unionall, intersect, except ---

func TestQuerySetOperations(t *testing.T) {
	londoners := `Query.select [] Query.from people Query.where [city eq 'London']`
	cases := []struct {
		name string
		src  string
		rows int
	}{
		// union dedupes (2 Londoners twice → 2); unionall keeps both copies.
		{"union-dedupe", londoners + ` Query.union (` + londoners + `)`, 2},
		{"unionall", londoners + ` Query.unionall (` + londoners + `)`, 4},
		// Londoners ∩ age≥30 = {Alice, Carol}.
		{"intersect", londoners + ` Query.intersect (Query.select [] Query.from people Query.where [age gte 30])`, 2},
		// Londoners \ age≥35 = {Alice}.
		{"except", londoners + ` Query.except (Query.select [] Query.from people Query.where [age gte 35])`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rowCount(t, c.src); got != c.rows {
				t.Errorf("%s: expected %d rows, got %d", c.name, c.rows, got)
			}
		})
	}
}

// TestQueryUnpackRename confirms a wrapper rebound under a DIFFERENT name
// (def w Query.where) still dispatches via the inner native, including its
// NoEvalArgs — the body word names the original inner native to look up.
func TestQueryUnpackRename(t *testing.T) {
	r := queryRegistry(t)
	// fr is from under a new name; the clause words stay bare via unpack.
	src := `def fr Query.from  unpack [select where] Query  select [] fr people where [age gt 25]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("rename-alias pipeline failed: %v", err)
	}
	if got := len(materialize(t, result[0]).Rows); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
}
