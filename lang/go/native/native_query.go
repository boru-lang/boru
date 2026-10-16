package native

import (
	"fmt"
	"strings"
)

// QueryNatives is the set of SQL-style query DSL words surfaced through
// the boru:query module (see lang/go/modules/query.go). The words form a
// left-to-right pipeline in natural SQL order: the entry word `select`
// seeds a lazy QueryBuilder with the projection columns, `from` sets the
// source table, and each subsequent clause word takes the builder off
// the stack plus its own clause as a forward argument and returns the
// updated builder. The query is lazy — it materializes (hits SQLite)
// only when its result is printed, iterated, or otherwise needs rows.
//
//	Query.select [name age]
//	  Query.from people
//	  Query.where [age gt 18]
//	  Query.order [age desc]
//
// All supporting parser/exec helpers (toQueryBuilder, buildWhereClause,
// parseColumnSpec, …) live in query.go.
//
// Argument convention (post §1.4 unified, top-first sig order): each
// pipeline word reads its clause from the forward token (sig position
// 0) and the upstream builder from the stack (sig position 1). The
// entry word `select` reads only its one forward arg (it creates the
// builder rather than consuming one).
var QueryNatives = []NativeFunc{
	{
		// `select` is the SQL-order entry word: it seeds a new lazy
		// query with the projection columns. Single forward arg — it
		// does not consume a builder from the stack, it creates one.
		Name:          "select",
		CompileEffect: CompileFallbackBody,
		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(selectColsHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		// `from` sets the source table of the select-seeded query.
		// name form: `from people` — name forward (quoted), builder
		// from the stack. value form: `from <table-value>`.
		Name: "from",
		Signatures: []Signature{
			{
				Args:       []*Type{TAtom, TList},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(fromNameHandler),
				Returns:    []*Type{TList},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TList, TList},
				Impl:       Go(fromValueHandler),
				Returns:    []*Type{TList},
				BarrierPos: -1,
			},
		},
	},
	{
		Name:          "where",
		CompileEffect: CompileFallbackBody,
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(queryWhereHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name:          "order",
		CompileEffect: CompileFallbackBody,
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(queryOrderHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name:          "group",
		CompileEffect: CompileFallbackBody,
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(queryGroupHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name:          "having",
		CompileEffect: CompileFallbackBody,
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(queryHavingHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name: "limit",
		Signatures: []Signature{{
			Args:       []*Type{TInteger, TList},
			Impl:       Go(queryLimitHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name: "offset",
		Signatures: []Signature{{
			Args:       []*Type{TInteger, TList},
			Impl:       Go(queryOffsetHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name: "distinct",
		Signatures: []Signature{{
			Args:       []*Type{TList},
			Impl:       Go(queryDistinctHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},

	// Join words — share queryJoinNative.
	queryJoinNative("join", "JOIN"),
	queryJoinNative("innerjoin", "JOIN"),
	queryJoinNative("leftjoin", "LEFT JOIN"),
	queryJoinNative("crossjoin", "CROSS JOIN"),
	{
		Name: "on",
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(queryOnHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},
	{
		Name: "using",
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(queryUsingHandler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	},

	// Set operations — share querySetOpNative.
	querySetOpNative("union", "UNION"),
	querySetOpNative("unionall", "UNION ALL"),
	querySetOpNative("intersect", "INTERSECT"),
	querySetOpNative("except", "EXCEPT"),
}

// fromNameHandler sets the source table of the query the preceding
// `select` seeded. args[0] is the quoted table-name atom (forward);
// args[1] is the upstream builder (stack). The table is resolved from
// the context store.
func fromNameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	val, ok := ContextStoreLookup(r, name)
	if !ok {
		return nil, fmt.Errorf("from: unknown table %q", name)
	}
	td, ok := val.Data().(TableData)
	if !ok {
		return nil, fmt.Errorf("from: %q is not a table", name)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	qb.Source = td
	qb.HasSource = true
	return []Value{wrapQB(qb)}, nil
}

// fromValueHandler sets the source from a table value already on the
// stack. args[0] is the source table value (forward); args[1] is the
// upstream builder (stack).
func fromValueHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	src, err := toQueryBuilder(r, args[0])
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	if !src.HasSource {
		return nil, fmt.Errorf("from: value is not a table")
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	qb.Source = src.Source
	qb.HasSource = true
	return []Value{wrapQB(qb)}, nil
}

// queryWhereHandler sets the WHERE clause. args[0] is the condition list
// (forward), args[1] is the upstream builder (stack).
func queryWhereHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	condList, err := resolveParenSubExprs(r, args[0])
	if err != nil {
		return nil, fmt.Errorf("where: %w", err)
	}
	clause, err := buildWhereClause(condList)
	if err != nil {
		return nil, fmt.Errorf("where: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("where: %w", err)
	}
	qb.Where = clause
	return []Value{wrapQB(qb)}, nil
}

// selectColsHandler seeds a new lazy query with the projection columns.
// It is the SQL-order entry word: `select [name age] from people …`.
// args[0] is the column list (forward). An empty list means SELECT *.
// The returned query has no source yet — `from` supplies it; the query
// runs only when materialized (printed / iterated).
func selectColsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	colList, err := resolveParenSubExprs(r, args[0])
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}
	cols, err := parseColumnSpec(colList)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		// Empty column list means SELECT * .
		cols = nil
	}
	return []Value{wrapQB(newSelectBuilder(r, cols))}, nil
}

// queryOrderHandler sets the ORDER BY clause.
func queryOrderHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	clause, err := buildOrderClause(args[0])
	if err != nil {
		return nil, fmt.Errorf("order: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("order: %w", err)
	}
	qb.OrderBy = clause
	return []Value{wrapQB(qb)}, nil
}

// queryGroupHandler sets the GROUP BY clause.
func queryGroupHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	clause, err := buildGroupByClause(args[0])
	if err != nil {
		return nil, fmt.Errorf("group: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("group: %w", err)
	}
	qb.GroupBy = clause
	return []Value{wrapQB(qb)}, nil
}

// queryHavingHandler sets the HAVING clause. It shares the WHERE-clause
// builder since HAVING uses the same condition grammar.
func queryHavingHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	condList, err := resolveParenSubExprs(r, args[0])
	if err != nil {
		return nil, fmt.Errorf("having: %w", err)
	}
	clause, err := buildWhereClause(condList)
	if err != nil {
		return nil, fmt.Errorf("having: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("having: %w", err)
	}
	qb.Having = clause
	return []Value{wrapQB(qb)}, nil
}

// queryLimitHandler sets the row limit. args[0] is the integer (forward),
// args[1] is the upstream builder (stack).
func queryLimitHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	n, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, fmt.Errorf("limit: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("limit: %w", err)
	}
	qb.Limit = int(n)
	return []Value{wrapQB(qb)}, nil
}

// queryOffsetHandler sets the row offset.
func queryOffsetHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	n, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, fmt.Errorf("offset: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("offset: %w", err)
	}
	qb.Offset = int(n)
	return []Value{wrapQB(qb)}, nil
}

// queryDistinctHandler flags the query as SELECT DISTINCT. It takes only
// the upstream builder (forward-eligible or from the stack).
func queryDistinctHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	qb, err := toQueryBuilder(r, args[0])
	if err != nil {
		return nil, fmt.Errorf("distinct: %w", err)
	}
	qb.Distinct = true
	return []Value{wrapQB(qb)}, nil
}

// queryJoinNative builds a join word (join/innerjoin/leftjoin/crossjoin).
// The handler captures the boru word name (for errors) and the SQL join
// type. args[0] is the joined table name (forward, quoted), args[1] is
// the upstream builder (stack).
func queryJoinNative(name, joinType string) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		tableName, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		qb, err := toQueryBuilder(r, args[1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		qb.Joins = append(qb.Joins, JoinClause{Type: joinType, Table: tableName})
		return []Value{wrapQB(qb)}, nil
	}
	return NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args:       []*Type{TAtom, TList},
			QuoteArgs:  map[int]bool{0: true},
			Impl:       Go(handler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	}
}

// queryOnHandler sets the ON condition of the most recent join.
func queryOnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	clause, err := buildJoinCondition(args[0])
	if err != nil {
		return nil, fmt.Errorf("on: %w", err)
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("on: %w", err)
	}
	if len(qb.Joins) == 0 {
		return nil, fmt.Errorf("on: no preceding join")
	}
	qb.Joins[len(qb.Joins)-1].On = clause
	return []Value{wrapQB(qb)}, nil
}

// queryUsingHandler sets the USING columns of the most recent join.
func queryUsingHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	lst, err := AsList(args[0])
	if err != nil {
		return nil, fmt.Errorf("using: %w", err)
	}
	elems := lst.Elems()
	cols := make([]string, 0, len(elems))
	for _, e := range elems {
		colName := valueToColName(e)
		if colName == "" {
			return nil, fmt.Errorf("using: expected column name, got %s", e.Parent())
		}
		cols = append(cols, quoteIdent(colName))
	}
	qb, err := toQueryBuilder(r, args[1])
	if err != nil {
		return nil, fmt.Errorf("using: %w", err)
	}
	if len(qb.Joins) == 0 {
		return nil, fmt.Errorf("using: no preceding join")
	}
	qb.Joins[len(qb.Joins)-1].UsingCols = strings.Join(cols, ", ")
	return []Value{wrapQB(qb)}, nil
}

// querySetOpNative builds a set-operation word (union/unionall/intersect/
// except). args[0] is the right-hand query/table (forward), args[1] is
// the left-hand upstream builder (stack).
func querySetOpNative(name, op string) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		rightQB, err := toQueryBuilder(r, args[0])
		if err != nil {
			return nil, fmt.Errorf("%s: right operand: %w", name, err)
		}
		leftQB, err := toQueryBuilder(r, args[1])
		if err != nil {
			return nil, fmt.Errorf("%s: left operand: %w", name, err)
		}
		leftQB.SetOps = append(leftQB.SetOps, SetOp{Op: op, Right: rightQB})
		return []Value{wrapQB(leftQB)}, nil
	}
	return NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			Impl:       Go(handler),
			Returns:    []*Type{TList},
			BarrierPos: -1,
		}},
	}
}
