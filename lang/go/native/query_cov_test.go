package native

import (
	"strings"
	"testing"
)

// Coverage tests for query.go: the QueryBuilder SQL assembly, its
// materialization paths (source / join / set-op loading into SQLite),
// the lazy-value wrapping, and the clause builders. Malformed clauses
// are pinned as rejections alongside every accepted form.

// covTable bundles a TableData fixture.
type covTable struct{ td TableData }

// covTableNameAge is a small people table: (name String, age Integer).
func covTableNameAge() covTable {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TInteger))
	rows := []Value{
		covEmitMap("name", NewString("alice"), "age", NewInteger(30)),
		covEmitMap("name", NewString("bob"), "age", NewInteger(20)),
		covEmitMap("name", NewString("carol"), "age", NewInteger(25)),
		covEmitMap("name", NewString("dave"), "age", NewInteger(20)),
	}
	return covTable{td: TableData{Record: RecordTypeInfo{Fields: fields}, Rows: rows}}
}

// covPetsTable is a companion table: (owner String, pet String).
func covPetsTable() TableData {
	fields := NewOrderedMap()
	fields.Set("owner", NewTypeLiteral(TString))
	fields.Set("pet", NewTypeLiteral(TString))
	rows := []Value{
		covEmitMap("owner", NewString("alice"), "pet", NewString("rex")),
		covEmitMap("owner", NewString("alice"), "pet", NewString("tom")),
		covEmitMap("owner", NewString("bob"), "pet", NewString("fifi")),
	}
	return TableData{Record: RecordTypeInfo{Fields: fields}, Rows: rows}
}

// covOneRowTable holds a single scalar cell (x = val).
func covOneRowTable(val Value) TableData {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	return TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{covEmitMap("x", val)},
	}
}

// covOneRowStrTable holds a single string cell (s = val). String-typed
// columns round-trip SQLite as TEXT, so materialized cells stay strings.
func covOneRowStrTable(val string) TableData {
	fields := NewOrderedMap()
	fields.Set("s", NewTypeLiteral(TString))
	return TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{covEmitMap("s", NewString(val))},
	}
}

func covCond(elems ...Value) Value { return NewList(elems) }

// ---- buildSQL ----

func TestBuildSQLAllClauses(t *testing.T) {
	right := QueryBuilder{
		Source:    TableData{SQLite: true, TableName: "rt"},
		HasSource: true,
		Limit:     -1,
		Offset:    -1,
	}
	qb := QueryBuilder{
		Distinct: true,
		Alias:    "p",
		Joins: []JoinClause{
			{Type: "LEFT JOIN", Table: "pets", Alias: "q", On: "`p`.`name` = `q`.`owner`"},
			{Type: "JOIN", Table: "scores", UsingCols: "`name`"},
		},
		Where:   "`age` > 18",
		GroupBy: "`age`",
		Having:  "`cnt` > 1",
		OrderBy: "`age` DESC",
		Limit:   5,
		Offset:  2,
		SetOps:  []SetOp{{Op: "UNION", Right: right}},
	}
	sql := qb.buildSQL("people", "*")
	for _, frag := range []string{
		"SELECT DISTINCT * FROM `people` AS `p`",
		"LEFT JOIN `pets` AS `q` ON `p`.`name` = `q`.`owner`",
		"JOIN `scores` USING(`name`)",
		"WHERE `age` > 18",
		"GROUP BY `age`",
		"HAVING `cnt` > 1",
		"UNION SELECT * FROM `rt`",
		"ORDER BY `age` DESC",
		"LIMIT 5",
		"OFFSET 2",
	} {
		if !strings.Contains(sql, frag) {
			t.Errorf("buildSQL missing %q in:\n%s", frag, sql)
		}
	}
}

func TestBuildSQLOffsetWithoutLimit(t *testing.T) {
	qb := QueryBuilder{Limit: -1, Offset: 3}
	sql := qb.buildSQL("t", "*")
	if !strings.Contains(sql, "LIMIT -1 OFFSET 3") {
		t.Errorf("offset without limit = %q", sql)
	}

	// A set-op right side with a non-SQLite source has no resolvable
	// table name yet (resolved during materialize).
	qb2 := QueryBuilder{
		Limit: -1, Offset: -1,
		SetOps: []SetOp{{Op: "EXCEPT", Right: QueryBuilder{
			Source: TableData{}, HasSource: true, Limit: -1, Offset: -1,
		}}},
	}
	sql = qb2.buildSQL("t", "*")
	if !strings.Contains(sql, "EXCEPT SELECT * FROM ``") {
		t.Errorf("unresolved set-op right = %q", sql)
	}
}

// ---- builder constructors / wrapping ----

func TestBuilderConstructors(t *testing.T) {
	tbl := covTableNameAge()
	qb := NewQueryBuilder(nil, tbl.td)
	if !qb.HasSource || qb.Limit != -1 || qb.Offset != -1 {
		t.Errorf("NewQueryBuilder defaults wrong: %+v", qb)
	}
	sel := newSelectBuilder(nil, []columnSpec{{Name: "name"}})
	if sel.HasSource || len(sel.Cols) != 1 || sel.Limit != -1 || sel.Offset != -1 {
		t.Errorf("newSelectBuilder defaults wrong: %+v", sel)
	}
}

// covFakeMat is a Materializer that is not a QueryBuilder — unwrapQB
// must decline it.
type covFakeMat struct{}

func (covFakeMat) Materialize() (TableData, error) { return TableData{}, nil }
func (covFakeMat) SourceRecord() RecordTypeInfo {
	return RecordTypeInfo{Fields: NewOrderedMap()}
}

func TestWrapUnwrapAndToQueryBuilder(t *testing.T) {
	tbl := covTableNameAge()
	qb := NewQueryBuilder(nil, tbl.td)
	wrapped := wrapQB(qb)
	got, ok := unwrapQB(wrapped)
	if !ok || !got.HasSource {
		t.Fatalf("unwrapQB(wrapQB) = (%+v,%v)", got, ok)
	}

	// A Materializer that is not a QueryBuilder is declined.
	if _, ok := unwrapQB(Value{Parent: TList, Data: MaterializerPayload{M: covFakeMat{}}}); ok {
		t.Error("unwrapQB accepted a non-QueryBuilder materializer")
	}
	// The legacy ExtensionPayload wrapping still unwraps.
	if _, ok := unwrapQB(Value{Parent: TList, Data: ExtensionPayload{Body: qb}}); !ok {
		t.Error("unwrapQB declined the legacy ExtensionPayload form")
	}

	// toQueryBuilder: wrapped query, bare table, and a rejection.
	if got, err := toQueryBuilder(nil, wrapped); err != nil || !got.HasSource {
		t.Errorf("toQueryBuilder(wrapped) = (%+v,%v)", got, err)
	}
	if got, err := toQueryBuilder(nil, Value{Parent: TList, Data: tbl.td}); err != nil || !got.HasSource {
		t.Errorf("toQueryBuilder(table) = (%+v,%v)", got, err)
	}
	if _, err := toQueryBuilder(nil, NewInteger(42)); err == nil {
		t.Error("toQueryBuilder(42): expected error, got nil")
	}
}

func TestQueryBuilderClone(t *testing.T) {
	tbl := covTableNameAge()
	orig := NewQueryBuilder(nil, tbl.td)
	orig.Joins = []JoinClause{{Type: "JOIN", Table: "pets"}}
	orig.SetOps = []SetOp{{Op: "UNION", Right: QueryBuilder{}}}
	cp := orig.clone()
	cp.Joins = append(cp.Joins, JoinClause{Type: "JOIN", Table: "more"})
	cp.SetOps = append(cp.SetOps, SetOp{Op: "EXCEPT", Right: QueryBuilder{}})
	if len(orig.Joins) != 1 || len(orig.SetOps) != 1 {
		t.Errorf("clone aliased the original: joins=%d setops=%d",
			len(orig.Joins), len(orig.SetOps))
	}
}

// ---- materialization ----

func TestMaterializeRequiresSourceAndStore(t *testing.T) {
	// No FROM table.
	if _, err := newSelectBuilder(nil, nil).Materialize(); err == nil {
		t.Error("no source: expected error, got nil")
	}
	// A registry without a SQLite store cannot load the source.
	bare, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	qb := NewQueryBuilder(bare, covTableNameAge().td)
	if _, err := qb.Materialize(); err == nil {
		t.Error("no sqlite store: expected error, got nil")
	}
}

func TestQueryMaterializeEndToEnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tbl := covTableNameAge()

	qb := NewQueryBuilder(r, tbl.td)
	if qb.Where, err = buildWhereClause(covCond(NewAtom("age"), NewAtom("gt"), NewInteger(21))); err != nil {
		t.Fatal(err)
	}
	if qb.OrderBy, err = buildOrderClause(covCond(NewAtom("age"), NewAtom("desc"))); err != nil {
		t.Fatal(err)
	}
	qb.Limit = 2
	out, err := qb.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out.Rows))
	}
	m, _ := AsMap(out.Rows[0])
	name, _ := m.Get("name")
	if s, _ := AsString(name); s != "alice" {
		t.Errorf("first row = %v, want alice", name)
	}

	// Offset without limit, plus DISTINCT.
	qb2 := NewQueryBuilder(r, tbl.td)
	qb2.Distinct = true
	if qb2.OrderBy, err = buildOrderClause(covCond(NewAtom("age"), NewAtom("asc"))); err != nil {
		t.Fatal(err)
	}
	qb2.Offset = 1
	out, err = qb2.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 3 {
		t.Errorf("offset 1 of 4 rows = %d rows, want 3", len(out.Rows))
	}
}

func TestMaterializeWithColumnsEndToEnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tbl := covTableNameAge()

	// Projection with alias + cast.
	cols, err := parseColumnSpec(NewList([]Value{
		NewAtom("name"),
		NewList([]Value{NewAtom("age"), NewAtom("years")}),
		NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("text"), NewAtom("age_s")}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	qb := NewQueryBuilder(r, tbl.td)
	qb.Cols = cols
	out, err := qb.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	keys := out.Record.Fields.Keys()
	if strings.Join(keys, ",") != "name,years,age_s" {
		t.Errorf("projected columns = %v", keys)
	}

	// Aggregate + GROUP BY + HAVING.
	aggCols, err := parseColumnSpec(NewList([]Value{
		NewAtom("age"),
		NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("cnt")}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	qb2 := NewQueryBuilder(r, tbl.td)
	qb2.Cols = aggCols
	if qb2.GroupBy, err = buildGroupByClause(covCond(NewAtom("age"))); err != nil {
		t.Fatal(err)
	}
	if qb2.Having, err = buildWhereClause(covCond(NewAtom("cnt"), NewAtom("gt"), NewInteger(1))); err != nil {
		t.Fatal(err)
	}
	out, err = qb2.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 {
		t.Fatalf("grouped rows = %d, want 1 (only age 20 repeats)", len(out.Rows))
	}
	// Aggregate cells come back through the Number column mapping.
	m, _ := AsMap(out.Rows[0])
	cnt, _ := m.Get("cnt")
	if f, _ := AsFloat(cnt); f != 2 {
		t.Errorf("cnt = %v, want 2", cnt)
	}
}

func TestQueryJoinsEndToEnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store := HostSQLite(r)
	people := covTableNameAge().td
	pets := covPetsTable()
	if err := store.StoreTable("cov_people", people); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreTable("cov_pets", pets); err != nil {
		t.Fatal(err)
	}
	r.ContextSet("cov_pets", Value{Parent: TList, Data: pets})

	src := people
	src.SQLite = true
	src.TableName = "cov_people"

	// ON join against a table already in SQLite (ensureJoinSources skip).
	qb := NewQueryBuilder(r, src)
	on, err := buildJoinCondition(covCond(
		NewAtom("cov_people.name"), NewAtom("eq"), NewAtom("cov_pets.owner")))
	if err != nil {
		t.Fatal(err)
	}
	qb.Joins = []JoinClause{{Type: "JOIN", Table: "cov_pets", On: on}}
	out, err := qb.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 3 {
		t.Errorf("join rows = %d, want 3", len(out.Rows))
	}
	// mergedSchema merges join columns behind the source's.
	if !strings.Contains(strings.Join(out.Record.Fields.Keys(), ","), "pet") {
		t.Errorf("merged schema missing pet: %v", out.Record.Fields.Keys())
	}

	// CROSS JOIN against context tables: one already SQLite-backed
	// (rename branch), one in-memory (temp-table branch).
	sqliteBacked := pets
	sqliteBacked.SQLite = true
	sqliteBacked.TableName = "cov_pets"
	r.ContextSet("cov_alias", Value{Parent: TList, Data: sqliteBacked})
	r.ContextSet("cov_mem", Value{Parent: TList, Data: covPetsTable()})

	qb2 := NewQueryBuilder(r, src)
	qb2.Joins = []JoinClause{
		{Type: "CROSS JOIN", Table: "cov_alias"},
		{Type: "CROSS JOIN", Table: "cov_mem"},
	}
	out, err = qb2.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 4*3*3 {
		t.Errorf("cross join rows = %d, want 36", len(out.Rows))
	}
}

func TestQueryJoinErrors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.ContextSet("cov_badtbl", NewInteger(1))
	src := covTableNameAge().td

	qb := NewQueryBuilder(r, src)
	qb.Joins = []JoinClause{{Type: "JOIN", Table: "cov_nosuch"}}
	if _, err := qb.Materialize(); err == nil || !strings.Contains(err.Error(), "unknown table") {
		t.Errorf("unknown join table: got %v", err)
	}

	qb2 := NewQueryBuilder(r, src)
	qb2.Joins = []JoinClause{{Type: "JOIN", Table: "cov_badtbl"}}
	if _, err := qb2.Materialize(); err == nil || !strings.Contains(err.Error(), "no table data") {
		t.Errorf("non-table join value: got %v", err)
	}
}

func TestQuerySetOpsEndToEnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	left := NewQueryBuilder(r, covTableNameAge().td)
	right := NewQueryBuilder(r, covTableNameAge().td)
	left.SetOps = []SetOp{{Op: "UNION", Right: right}}
	out, err := left.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	// UNION dedupes the identical tables.
	if len(out.Rows) != 4 {
		t.Errorf("union rows = %d, want 4", len(out.Rows))
	}

	// A right side already in SQLite skips the temp-table load.
	store := HostSQLite(r)
	if err := store.StoreTable("cov_setop", covTableNameAge().td); err != nil {
		t.Fatal(err)
	}
	sqliteTD := covTableNameAge().td
	sqliteTD.SQLite = true
	sqliteTD.TableName = "cov_setop"
	left2 := NewQueryBuilder(r, covTableNameAge().td)
	left2.SetOps = []SetOp{{Op: "UNION ALL", Right: NewQueryBuilder(r, sqliteTD)}}
	out, err = left2.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 8 {
		t.Errorf("union all rows = %d, want 8", len(out.Rows))
	}
}

func TestSourceRecordShapes(t *testing.T) {
	// No source: empty record.
	if rec := newSelectBuilder(nil, nil).SourceRecord(); rec.Fields.Len() != 0 {
		t.Errorf("no-source record = %v", rec.Fields.Keys())
	}

	bare, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	qb := NewQueryBuilder(bare, covTableNameAge().td)
	if got := strings.Join(qb.SourceRecord().Fields.Keys(), ","); got != "name,age" {
		t.Errorf("source record = %q, want name,age", got)
	}

	qb.Cols = []columnSpec{
		{Name: "name", Alias: "who"},
		{Raw: "COUNT(*)", Alias: "cnt", ResultType: TInteger},
		{Name: "ghost"},
	}
	rec := qb.SourceRecord()
	if got := strings.Join(rec.Fields.Keys(), ","); got != "who,cnt,ghost" {
		t.Errorf("projected record = %q, want who,cnt,ghost", got)
	}
	// The unknown column falls back to String.
	ghost, _ := rec.Fields.Get("ghost")
	if !(&ghost).Equal(TString) {
		t.Errorf("ghost column type = %s, want String", ghost.String())
	}
}

// ---- resolveParenSubExprs ----

func TestResolveParenSubExprs(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Non-list passes through.
	got, err := resolveParenSubExprs(r, NewInteger(7))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := AsInteger(got); n != 7 {
		t.Errorf("non-list passthrough = %v", got)
	}

	// No parens: unchanged element count.
	got, err = resolveParenSubExprs(r, covCond(NewAtom("a"), NewInteger(1)))
	if err != nil {
		t.Fatal(err)
	}
	lst, _ := AsList(got)
	if lst.Len() != 2 {
		t.Errorf("no-paren list len = %d, want 2", lst.Len())
	}

	// A paren group evaluates in a sub-engine (also inside a nested list).
	inner := covCond(NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen())
	got, err = resolveParenSubExprs(r, covCond(NewAtom("a"), inner))
	if err != nil {
		t.Fatal(err)
	}
	lst, _ = AsList(got)
	innerLst, _ := AsList(lst.Slice()[1])
	if innerLst.Len() != 1 {
		t.Fatalf("nested paren result = %v", lst.Slice()[1])
	}
	if n, _ := AsInteger(innerLst.Slice()[0]); n != 3 {
		t.Errorf("paren eval = %v, want 3", innerLst.Slice()[0])
	}

	// Unmatched paren is rejected.
	if _, err = resolveParenSubExprs(r, covCond(NewOpenParen(), NewInteger(1))); err == nil {
		t.Error("unmatched paren: expected error, got nil")
	}

	// A failing sub-expression is rejected.
	bad := covCond(NewOpenParen(), NewWord("cov_definitely_undefined"), NewCloseParen())
	if _, err = resolveParenSubExprs(r, bad); err == nil {
		t.Error("failing subexpression: expected error, got nil")
	}
}

// ---- parseColumnSpec extras ----

func TestParseColumnSpecMoreForms(t *testing.T) {
	// A word element is a column name.
	specs, err := parseColumnSpec(NewList([]Value{NewWord("age")}))
	if err != nil || specs[0].Name != "age" {
		t.Errorf("word column = (%+v,%v)", specs, err)
	}

	// Scalar subquery [<table> alias] becomes a raw SQL cell.
	oneRow := Value{Parent: TList, Data: covOneRowTable(NewInteger(42))}
	specs, err = parseColumnSpec(NewList([]Value{
		NewList([]Value{oneRow, NewAtom("answer")}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if specs[0].Raw != "42" || specs[0].Alias != "answer" {
		t.Errorf("subquery column = %+v", specs[0])
	}

	// cast to an unmapped type name upper-cases it.
	specs, err = parseColumnSpec(NewList([]Value{
		NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("blob")}),
	}))
	if err != nil || !strings.Contains(specs[0].Raw, "AS BLOB") {
		t.Errorf("blob cast = (%+v,%v)", specs, err)
	}
}

func TestParseColumnSpecRejections(t *testing.T) {
	twoRow := TableData{
		Record: covOneRowTable(NewInteger(1)).Record,
		Rows: []Value{
			covEmitMap("x", NewInteger(1)),
			covEmitMap("x", NewInteger(2)),
		},
	}
	cases := []struct {
		name string
		cols Value
	}{
		{"short sub-list", NewList([]Value{NewList([]Value{NewAtom("a")})})},
		{"three-element alias", NewList([]Value{NewList([]Value{NewAtom("a"), NewAtom("b"), NewAtom("c")})})},
		{"non-name alias pair", NewList([]Value{NewList([]Value{NewInteger(1), NewInteger(2)})})},
		{"unsupported element", NewList([]Value{NewInteger(5)})},
		{"aggregate too many args", NewList([]Value{NewList([]Value{NewAtom("count"), NewAtom("a"), NewAtom("b"), NewAtom("c")})})},
		{"aggregate bad column", NewList([]Value{NewList([]Value{NewAtom("count"), NewInteger(5)})})},
		{"cast too short", NewList([]Value{NewList([]Value{NewAtom("cast"), NewAtom("age")})})},
		{"cast too long", NewList([]Value{NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("text"), NewAtom("a"), NewAtom("b")})})},
		{"cast bad column", NewList([]Value{NewList([]Value{NewAtom("cast"), NewInteger(5), NewAtom("text")})})},
		{"multi-row scalar subquery", NewList([]Value{NewList([]Value{
			{Parent: TList, Data: twoRow}, NewAtom("a"),
		})})},
	}
	for _, c := range cases {
		if _, err := parseColumnSpec(c.cols); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

// ---- buildWhereClause ----

func TestBuildWhereClauseForms(t *testing.T) {
	oneRow := Value{Parent: TList, Data: covOneRowTable(NewInteger(25))}
	cases := []struct {
		name string
		cond Value
		want string
	}{
		{"empty", covCond(), "1=1"},
		{"simple", covCond(NewAtom("age"), NewAtom("gt"), NewInteger(18)), "`age` > 18"},
		{"and", covCond(NewAtom("age"), NewAtom("gt"), NewInteger(18),
			NewAtom("and"), NewAtom("name"), NewAtom("eq"), NewString("x")),
			"`age` > 18 AND `name` = 'x'"},
		{"not single", covCond(NewAtom("not"), NewAtom("age"), NewAtom("gt"), NewInteger(18)),
			"NOT (`age` > 18)"},
		{"not single then or", covCond(NewAtom("not"), NewAtom("age"), NewAtom("gt"), NewInteger(18),
			NewAtom("or"), NewAtom("age"), NewAtom("lt"), NewInteger(5)),
			"NOT (`age` > 18) OR `age` < 5"},
		{"not group", covCond(NewAtom("not"),
			covCond(NewAtom("age"), NewAtom("gt"), NewInteger(18)),
			NewAtom("and"), NewAtom("age"), NewAtom("lt"), NewInteger(99)),
			"NOT (`age` > 18) AND `age` < 99"},
		{"group", covCond(
			covCond(NewAtom("a"), NewAtom("eq"), NewInteger(1),
				NewAtom("or"), NewAtom("a"), NewAtom("eq"), NewInteger(2)),
			NewAtom("and"), NewAtom("b"), NewAtom("eq"), NewInteger(3)),
			"(`a` = 1 OR `a` = 2) AND `b` = 3"},
		{"is null", covCond(NewAtom("a"), NewAtom("is"), NewAtom("null")), "`a` IS NULL"},
		{"is not null", covCond(NewAtom("a"), NewAtom("is"), NewAtom("not"), NewAtom("null")),
			"`a` IS NOT NULL"},
		{"between", covCond(NewAtom("a"), NewAtom("between"), NewInteger(1), NewInteger(10)),
			"`a` BETWEEN 1 AND 10"},
		{"not between", covCond(NewAtom("a"), NewAtom("not"), NewAtom("between"), NewInteger(1), NewInteger(10)),
			"`a` NOT BETWEEN 1 AND 10"},
		{"in", covCond(NewAtom("a"), NewAtom("in"), NewList([]Value{NewInteger(1), NewInteger(2)})),
			"`a` IN (1, 2)"},
		{"not in", covCond(NewAtom("a"), NewAtom("not"), NewAtom("in"), NewList([]Value{NewInteger(1)})),
			"`a` NOT IN (1)"},
		{"like", covCond(NewAtom("name"), NewAtom("like"), NewString("%a%")), "`name` LIKE '%a%'"},
		{"collate", covCond(NewAtom("name"), NewAtom("eq"), NewString("x"), NewAtom("collate"), NewAtom("nocase")),
			"`name` = 'x' COLLATE NOCASE"},
		{"scalar subquery value", covCond(NewAtom("age"), NewAtom("eq"), oneRow), "`age` = 25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildWhereClause(c.cond)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("where %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestBuildWhereClauseRejections(t *testing.T) {
	twoRow := Value{Parent: TList, Data: TableData{
		Record: covOneRowTable(NewInteger(1)).Record,
		Rows:   []Value{covEmitMap("x", NewInteger(1)), covEmitMap("x", NewInteger(2))},
	}}
	cases := []struct {
		name string
		cond Value
	}{
		{"dangling not", covCond(NewAtom("not"))},
		{"column only", covCond(NewAtom("a"))},
		{"is dangling", covCond(NewAtom("a"), NewAtom("is"))},
		{"is not dangling", covCond(NewAtom("a"), NewAtom("is"), NewAtom("not"))},
		{"is not bogus", covCond(NewAtom("a"), NewAtom("is"), NewAtom("not"), NewAtom("bogus"))},
		{"is bogus", covCond(NewAtom("a"), NewAtom("is"), NewAtom("bogus"))},
		{"between one value", covCond(NewAtom("a"), NewAtom("between"), NewInteger(1))},
		{"in dangling", covCond(NewAtom("a"), NewAtom("in"))},
		{"not dangling after col", covCond(NewAtom("a"), NewAtom("not"))},
		{"not bogus", covCond(NewAtom("a"), NewAtom("not"), NewAtom("bogus"), NewInteger(1))},
		{"not between one value", covCond(NewAtom("a"), NewAtom("not"), NewAtom("between"), NewInteger(1))},
		{"not in dangling", covCond(NewAtom("a"), NewAtom("not"), NewAtom("in"))},
		{"unknown op", covCond(NewAtom("a"), NewAtom("bogus"), NewInteger(1))},
		{"missing value", covCond(NewAtom("a"), NewAtom("eq"))},
		{"float value", covCond(NewAtom("a"), NewAtom("eq"), NewFloat(1.5))},
		{"non-name column", covCond(NewInteger(5), NewAtom("eq"), NewInteger(1))},
		{"collate dangling", covCond(NewAtom("a"), NewAtom("eq"), NewString("x"), NewAtom("collate"))},
		{"collate bogus", covCond(NewAtom("a"), NewAtom("eq"), NewString("x"), NewAtom("collate"), NewAtom("bogus"))},
		{"multi-row scalar subquery", covCond(NewAtom("a"), NewAtom("eq"), twoRow)},
	}
	for _, c := range cases {
		if _, err := buildWhereClause(c.cond); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

// ---- buildInList / scalar helpers ----

func TestBuildInListMore(t *testing.T) {
	if _, err := buildInList(NewList([]Value{})); err == nil {
		t.Error("empty IN list: expected error, got nil")
	}
	if _, err := buildInList(NewList([]Value{NewFloat(1.5)})); err == nil {
		t.Error("float IN value: expected error, got nil")
	}

	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A wrapped query materializes to its first column values.
	got, err := buildInList(wrapQB(NewQueryBuilder(r, covOneRowStrTable("v9"))))
	if err != nil || got != "'v9'" {
		t.Errorf("query IN list = (%q,%v), want 'v9'", got, err)
	}
	// A sourceless query is rejected.
	if _, err := buildInList(wrapQB(newSelectBuilder(r, nil))); err == nil {
		t.Error("sourceless query IN list: expected error, got nil")
	}
}

func TestBuildInListFromTableMore(t *testing.T) {
	if _, err := buildInListFromTable(TableData{Record: RecordTypeInfo{Fields: NewOrderedMap()}}); err == nil {
		t.Error("no columns: expected error, got nil")
	}
	// A row value that has no SQL form is rejected.
	bad := covOneRowTable(NewFloat(1.5))
	if _, err := buildInListFromTable(bad); err == nil {
		t.Error("float cell: expected error, got nil")
	}
	// Rows missing the first column collapse to NULL.
	missing := TableData{
		Record: covOneRowTable(NewInteger(1)).Record,
		Rows:   []Value{covEmitMap("other", NewInteger(1))},
	}
	got, err := buildInListFromTable(missing)
	if err != nil || got != "NULL" {
		t.Errorf("missing column = (%q,%v), want NULL", got, err)
	}
}

func TestResolveScalarValueQueryBuilder(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveScalarValue(wrapQB(NewQueryBuilder(r, covOneRowStrTable("s7"))))
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := AsString(got); s != "s7" {
		t.Errorf("scalar from query = %v, want s7", got)
	}
	if _, err := resolveScalarValue(wrapQB(newSelectBuilder(r, nil))); err == nil {
		t.Error("sourceless scalar query: expected error, got nil")
	}
}

func TestValueToSQLFloatRejected(t *testing.T) {
	if _, err := valueToSQL(NewFloat(1.5)); err == nil {
		t.Error("float: expected error, got nil")
	}
}

func TestScalarFromTableNoColumns(t *testing.T) {
	if _, err := scalarFromTable(TableData{Record: RecordTypeInfo{Fields: NewOrderedMap()}}); err == nil {
		t.Error("no columns: expected error, got nil")
	}
}

// ---- buildJoinCondition ----

func TestBuildJoinConditionForms(t *testing.T) {
	got, err := buildJoinCondition(covCond(
		NewAtom("a.id"), NewAtom("eq"), NewAtom("b.id"),
		NewAtom("and"), NewAtom("x"), NewAtom("lt"), NewAtom("b.y")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "`a`.`id` = `b`.`id` AND `x` < `b`.`y`" {
		t.Errorf("join condition = %q", got)
	}
	got, err = buildJoinCondition(covCond())
	if err != nil || got != "1=1" {
		t.Errorf("empty join condition = (%q,%v)", got, err)
	}

	cases := []struct {
		name string
		cond Value
	}{
		{"non-name lhs", covCond(NewInteger(5), NewAtom("eq"), NewAtom("b"))},
		{"incomplete", covCond(NewAtom("a"))},
		{"unknown op", covCond(NewAtom("a"), NewAtom("bogus"), NewAtom("b"))},
		{"missing rhs", covCond(NewAtom("a"), NewAtom("eq"))},
		{"non-name rhs", covCond(NewAtom("a"), NewAtom("eq"), NewInteger(5))},
	}
	for _, c := range cases {
		if _, err := buildJoinCondition(c.cond); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

// ---- buildOrderClause ----

func TestBuildOrderClauseForms(t *testing.T) {
	cases := []struct {
		name string
		cols Value
		want string
	}{
		{"positional", covCond(NewInteger(1), NewInteger(2), NewAtom("desc")), "1, 2 DESC"},
		{"asc nulls first", covCond(NewAtom("name"), NewAtom("asc"), NewAtom("nulls"), NewAtom("first")),
			"`name` ASC NULLS FIRST"},
		{"desc nulls last", covCond(NewAtom("name"), NewAtom("desc"), NewAtom("nulls"), NewAtom("last")),
			"`name` DESC NULLS LAST"},
		{"collate", covCond(NewAtom("name"), NewAtom("collate"), NewAtom("nocase")),
			"`name` COLLATE NOCASE"},
		{"multi", covCond(NewAtom("a"), NewAtom("b"), NewAtom("desc")), "`a`, `b` DESC"},
	}
	for _, c := range cases {
		got, err := buildOrderClause(c.cols)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("order %s = %q, want %q", c.name, got, c.want)
		}
	}

	rejects := []struct {
		name string
		cols Value
	}{
		{"empty", covCond()},
		{"modifier first", covCond(NewAtom("asc"))},
		{"nulls dangling", covCond(NewAtom("name"), NewAtom("nulls"))},
		{"nulls bogus", covCond(NewAtom("name"), NewAtom("nulls"), NewAtom("bogus"))},
		{"collate dangling", covCond(NewAtom("name"), NewAtom("collate"))},
		{"collate bogus", covCond(NewAtom("name"), NewAtom("collate"), NewAtom("bogus"))},
		{"bad element", covCond(NewFloat(1.5))},
	}
	for _, c := range rejects {
		if _, err := buildOrderClause(c.cols); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

// ---- buildGroupByClause ----

func TestBuildGroupByBadElement(t *testing.T) {
	if _, err := buildGroupByClause(covCond(NewFloat(1.5))); err == nil {
		t.Error("non-name group column: expected error, got nil")
	}
}

// ---- residual edge branches ----

func TestQueryMaterializeSQLErrors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	people := covTableNameAge().td

	// An unknown column in WHERE surfaces the SQLite error.
	qb := NewQueryBuilder(r, people)
	qb.Where = "`nosuchcol` > 1"
	if _, err := qb.Materialize(); err == nil {
		t.Error("bad where column: expected error, got nil")
	}

	// An unknown projected column errors through the column path.
	qb2 := NewQueryBuilder(r, people)
	qb2.Cols = []columnSpec{{Name: "ghost"}}
	if _, err := qb2.Materialize(); err == nil {
		t.Error("bad projected column: expected error, got nil")
	}
}

func TestQueryClosedStoreErrors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store := HostSQLite(r)
	people := covTableNameAge().td
	if err := store.StoreTable("cov_closed", people); err != nil {
		t.Fatal(err)
	}
	src := people
	src.SQLite = true
	src.TableName = "cov_closed"
	r.ContextSet("cov_closed_mem", Value{Parent: TList, Data: covPetsTable()})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Loading a non-SQLite source fails on the closed store.
	if _, err := NewQueryBuilder(r, people).Materialize(); err == nil {
		t.Error("closed store source load: expected error, got nil")
	}

	// Loading a joined in-memory table fails.
	qbJ := NewQueryBuilder(r, src)
	qbJ.Joins = []JoinClause{{Type: "CROSS JOIN", Table: "cov_closed_mem"}}
	if _, err := qbJ.Materialize(); err == nil {
		t.Error("closed store join load: expected error, got nil")
	}

	// Loading a set-op right side fails.
	qbS := NewQueryBuilder(r, src)
	qbS.SetOps = []SetOp{{Op: "UNION", Right: NewQueryBuilder(r, people)}}
	if _, err := qbS.Materialize(); err == nil {
		t.Error("closed store set-op load: expected error, got nil")
	}

	// A plain query against the closed store fails at execution.
	if _, err := NewQueryBuilder(r, src).Materialize(); err == nil {
		t.Error("closed store query: expected error, got nil")
	}

	// The same three failure points exist on the column path.
	qbC := NewQueryBuilder(r, people)
	qbC.Cols = []columnSpec{{Name: "name"}}
	if _, err := qbC.Materialize(); err == nil {
		t.Error("closed store cols source: expected error, got nil")
	}
	qbCJ := NewQueryBuilder(r, src)
	qbCJ.Cols = []columnSpec{{Name: "name"}}
	qbCJ.Joins = []JoinClause{{Type: "JOIN", Table: "cov_totally_unknown"}}
	if _, err := qbCJ.Materialize(); err == nil {
		t.Error("cols join error: expected error, got nil")
	}
	qbCS := NewQueryBuilder(r, src)
	qbCS.Cols = []columnSpec{{Name: "name"}}
	qbCS.SetOps = []SetOp{{Op: "UNION", Right: NewQueryBuilder(r, people)}}
	if _, err := qbCS.Materialize(); err == nil {
		t.Error("cols set-op load: expected error, got nil")
	}
}

func TestMaterializeWithColumnsRawAndTemps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	people := covTableNameAge().td

	// A raw expression without an alias projects as-is and falls back
	// to a String-typed result column.
	qb := NewQueryBuilder(r, people)
	qb.Cols = []columnSpec{{Raw: "41+1"}}
	out, err := qb.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 4 {
		t.Fatalf("raw column rows = %d, want 4", len(out.Rows))
	}
	m, _ := AsMap(out.Rows[0])
	cell, _ := m.Get("41+1")
	if s, _ := AsString(cell); s != "42" {
		t.Errorf("raw cell = %v, want 42", cell)
	}

	// Joins and set-ops load temp tables through the column path too.
	r.ContextSet("cov_mem2", Value{Parent: TList, Data: covOneRowStrTable("z")})
	qb2 := NewQueryBuilder(r, people)
	qb2.Cols = []columnSpec{{Name: "name"}}
	qb2.Joins = []JoinClause{{Type: "CROSS JOIN", Table: "cov_mem2"}}
	qb2.SetOps = []SetOp{{Op: "UNION ALL", Right: NewQueryBuilder(r, covOneRowStrTable("q"))}}
	out, err = qb2.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 5 {
		t.Errorf("join+setop rows = %d, want 5 (4 cross-joined + 1 union all)", len(out.Rows))
	}
}

func TestMergedSchemaSkipsNonTableEntries(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store := HostSQLite(r)
	people := covTableNameAge().td
	if err := store.StoreTable("cov_people3", people); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreTable("cov_weird", covPetsTable()); err != nil {
		t.Fatal(err)
	}
	// The context entry for the joined name is NOT table data, so the
	// merged schema keeps only the source columns.
	r.ContextSet("cov_weird", NewInteger(1))
	src := people
	src.SQLite = true
	src.TableName = "cov_people3"
	qb := NewQueryBuilder(r, src)
	qb.Joins = []JoinClause{{Type: "CROSS JOIN", Table: "cov_weird"}}
	out, err := qb.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 12 {
		t.Errorf("cross join rows = %d, want 12", len(out.Rows))
	}
}

func TestResolveParenSubExprsEdges(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A list-typed value with no readable elements passes through.
	odd := Value{Parent: TList, Data: StrPayload{S: "x"}}
	if _, err := resolveParenSubExprs(r, odd); err != nil {
		t.Errorf("odd list payload: unexpected error %v", err)
	}
	// An empty list passes through.
	if _, err := resolveParenSubExprs(r, NewList([]Value{})); err != nil {
		t.Errorf("empty list: unexpected error %v", err)
	}

	// A paren group mixed with plain elements at the same level.
	got, err := resolveParenSubExprs(r, covCond(
		NewAtom("a"),
		NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen()))
	if err != nil {
		t.Fatal(err)
	}
	lst, _ := AsList(got)
	if lst.Len() != 2 {
		t.Fatalf("mixed list = %v", got)
	}
	if n, _ := AsInteger(lst.Slice()[1]); n != 3 {
		t.Errorf("paren value = %v, want 3", lst.Slice()[1])
	}

	// Nested parens are depth-counted as one outer group.
	got, err = resolveParenSubExprs(r, covCond(
		NewOpenParen(),
		NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
		NewWord("mul"), NewInteger(2),
		NewCloseParen()))
	if err != nil {
		t.Fatal(err)
	}
	lst, _ = AsList(got)
	if lst.Len() != 1 {
		t.Fatalf("nested paren list = %v", got)
	}
	if n, _ := AsInteger(lst.Slice()[0]); n != 6 {
		t.Errorf("nested paren value = %v, want 6", lst.Slice()[0])
	}

	// A nested list carrying a broken group propagates the error.
	if _, err = resolveParenSubExprs(r, covCond(
		covCond(NewOpenParen(), NewInteger(1)))); err == nil {
		t.Error("nested unmatched paren: expected error, got nil")
	}
}

func TestBuildWhereClauseNestedErrors(t *testing.T) {
	fl := NewFloat(1.5)
	cases := []struct {
		name string
		cond Value
	}{
		{"not group inner error", covCond(NewAtom("not"), covCond(NewAtom("a")))},
		{"not single inner error", covCond(NewAtom("not"), NewAtom("a"))},
		{"group inner error", covCond(covCond(NewAtom("a")), NewAtom("and"), NewAtom("b"), NewAtom("eq"), NewInteger(1))},
		{"between bad lo", covCond(NewAtom("a"), NewAtom("between"), fl, NewInteger(2))},
		{"between bad hi", covCond(NewAtom("a"), NewAtom("between"), NewInteger(1), fl)},
		{"in bad element", covCond(NewAtom("a"), NewAtom("in"), NewList([]Value{fl}))},
		{"not between bad lo", covCond(NewAtom("a"), NewAtom("not"), NewAtom("between"), fl, NewInteger(2))},
		{"not between bad hi", covCond(NewAtom("a"), NewAtom("not"), NewAtom("between"), NewInteger(1), fl)},
		{"not in bad element", covCond(NewAtom("a"), NewAtom("not"), NewAtom("in"), NewList([]Value{fl}))},
	}
	for _, c := range cases {
		if _, err := buildWhereClause(c.cond); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestQueryScalarAndOrderResiduals(t *testing.T) {
	// A scalar subquery row lacking the column resolves to None.
	missing := TableData{
		Record: covOneRowTable(NewInteger(1)).Record,
		Rows:   []Value{covEmitMap("other", NewInteger(1))},
	}
	v, err := scalarFromTable(missing)
	if err != nil || !IsNoneShape(v) {
		t.Errorf("missing scalar column = (%v,%v), want None", v, err)
	}

	// A false boolean has a SQL form.
	got, err := valueToSQL(NewBoolean(false))
	if err != nil || got != "'false'" {
		t.Errorf("false = (%q,%v), want 'false'", got, err)
	}

	// A bare non-list IN value with no SQL form is rejected.
	if _, err := buildInList(NewFloat(1.5)); err == nil {
		t.Error("float IN scalar: expected error, got nil")
	}

	// A subquery column with a cell valueToSQL rejects is declined.
	if _, err := parseColumnSpec(NewList([]Value{NewList([]Value{
		{Parent: TList, Data: covOneRowTable(NewFloat(1.5))}, NewAtom("a"),
	})})); err == nil {
		t.Error("float subquery cell: expected error, got nil")
	}

	// Word elements resolve as column names.
	clause, err := buildOrderClause(covCond(NewWord("age")))
	if err != nil || clause != "`age`" {
		t.Errorf("word order column = (%q,%v)", clause, err)
	}

	// Bare first/last modifiers attach to the preceding column.
	clause, err = buildOrderClause(covCond(NewAtom("name"), NewAtom("first")))
	if err != nil || clause != "`name` FIRST" {
		t.Errorf("bare first = (%q,%v)", clause, err)
	}
	clause, err = buildOrderClause(covCond(NewAtom("name"), NewAtom("last")))
	if err != nil || clause != "`name` LAST" {
		t.Errorf("bare last = (%q,%v)", clause, err)
	}
}

func TestMaterializeWithColumnsNilCols(t *testing.T) {
	// Direct callers may pass nil columns: the projection falls back
	// to SELECT * (the Materialize entry point routes nil elsewhere).
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	qb := NewQueryBuilder(r, covTableNameAge().td)
	out, err := qb.MaterializeWithColumns(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 4 || out.Record.Fields.Len() != 2 {
		t.Errorf("nil-cols materialize = %d rows / %d cols, want 4/2",
			len(out.Rows), out.Record.Fields.Len())
	}
}
