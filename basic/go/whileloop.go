package basic

// RunWhileLoop builds the mark + condition + move tokens for a while
// loop — the condition region runs first, and the move's while-mode
// Loop (core engine.go stepMoveWhile) alternates condition and body
// regions from there. Both operands are quoted code lists (NoEvalArgs).
// Every region is engine-stepped, so the Run loop's step budget meters
// the loop: `while [true] []` is bounded by evaluation_limit, never a
// silent hang. Body values accumulate across iterations and splice onto
// the stack when the condition goes falsy, exactly as `for` leaves its
// per-iteration values; `break` ends the loop with the values collected
// so far, `continue` abandons the current body round.
func RunWhileLoop(r *Registry, cond, body Value) ([]Value, error) {
	if !IsConcrete(&cond) {
		return nil, r.BoruError("while_error",
			"while: condition must be a concrete list, got type literal", "while")
	}
	if !IsConcrete(&body) {
		return nil, r.BoruError("while_error",
			"while: body must be a concrete list, got type literal", "while")
	}
	// The Loop replays these copies of the two regions every iteration.
	condLst, _ := AsList(cond)
	bodyLst, _ := AsList(body)
	condSlice := condLst.Slice()
	bodySlice := bodyLst.Slice()

	cont := &Loop{
		Registry:  r,
		Word:      "while",
		Count:     -1,
		Body:      bodySlice,
		WhileCond: condSlice,
		CondPos:   cond.Pos(),
	}

	// First region: the condition. Its move fires stepMoveWhile with
	// WhileInBody false, which reads the region's last value and either
	// splices the body or finishes the loop. The mark carries no body.
	id := NextMarkID()
	tokens := make([]Value, 0, len(condSlice)+2)
	tokens = append(tokens, NewMark(id))
	tokens = append(tokens, condSlice...)
	tokens = append(tokens, NewMoveCont(id, "while loop", cont))
	return tokens, nil
}

// WhileHandler is the run-mode implementation of the `while` word.
func WhileHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return RunWhileLoop(r, args[0], args[1])
}

// whileReturnsFn is the check-mode model of a while loop: analyse the
// CONDITION and the BODY each through the loop-body fixed point
// (AnalyseLoopBody with no iterator binding — reads join the enclosing
// bindings exactly as a for body's do), then model the residual as a
// variadic spread of the body's residual type, or nothing for a zero-net
// body. A while loop's trip count is NEVER static, so there is no
// zero-prune and no exact-count spread. Modelling here — instead of letting
// the analysis pass step the spliced regions with carrier conditions — is
// what keeps a carrier-conditioned `while` from looping the checker.
func whileReturnsFn(args []Value, r *Registry) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	// A STATICALLY EMPTY condition is a guaranteed runtime error, so the
	// check pass MIRRORS it: `boru check` reports what the program will
	// raise instead of staying silent about a certainty. It is stamped a
	// RuntimeMirror (CheckAddUniqueDiagnostic does that for its callers),
	// which is what lets the compile pipeline keep compiling the program
	// to the terminal trap below rather than declining on an error
	// diagnostic — the finding's model is exact, and the trap raises the
	// identical error.
	//
	// Only where the loop is UNCONDITIONALLY reached: inside a fn body or
	// any nested body the loop runs only if that body does, and a mirror
	// claiming "the program errors" must not be made from a conditional
	// site. That is the same reachability rule the trap's top-level-only
	// guard enforces one layer down.
	if emptyWhileCond(args[0]) && r.Check.FnBodyDepth == 0 && r.Check.NestedBodyDepth == 0 {
		// Shaped here rather than through CheckAddUniqueDiagnostic because
		// the code is the RUNTIME's own — `runtime_error`, so the check
		// report and the raise a user meets read alike — and that code has
		// no entry in the severity table (an unclassified code defaults to
		// info, which would not gate `boru check`). The mirror flag is set
		// explicitly for the same reason CheckAddUniqueDiagnostic sets it.
		CheckAddUnique(r, CheckDiagnostic{
			Code:          "runtime_error",
			Detail:        "while: condition produced no value",
			Word:          "while",
			Row:           args[0].Pos().Row,
			Col:           args[0].Pos().Col,
			Severity:      SeverityError,
			RuntimeMirror: true,
		})
	}
	// The BODY is analysed first, the condition second: a def the body
	// REBINDS (`while [n lt 3] [def n (n add 1)]`) is joined by the body's
	// analysis, and the condition analysed after it reads the joined
	// binding. Both analyses run to their fixed points over the joined
	// bindings, so the order changes no verdict.
	stk := AnalyseLoopBody(r, args[1], nil, nil, false)
	AnalyseLoopBody(r, args[0], nil, nil, false)
	var top Value
	if len(stk) > 0 {
		top = stk[len(stk)-1]
	}
	if len(stk) == 0 {
		return []Value{}
	}
	// A while leaves 0-OR-MORE values of the body's residual
	// type — its trip count is never static — so the honest residual is a
	// VARIADIC SPREAD (SpreadPayload, the device a `[]`-declared recursive
	// fn's leak and await's winner-takes-all already use), not a typed-List
	// carrier: a List where the runtime leaves N scalars is a false claim
	// (measured: 5 violations the moment the while rows entered the main
	// corpus, control.tsv §7).
	elem := NewTypeLiteral(top.Parent())
	if IsDisjunct(&top) {
		elem = top
	}
	// A body that nets MORE THAN ONE value per round leaves one of EACH on
	// every trip, interleaved: `while […] [ set 'n' (…) c end … (c get 'n')
	// ]` leaves FlexMap, Integer, FlexMap, Integer. The spread carries ONE
	// element type and it has to cover all of them, so a body whose residual
	// is not of a single type widens to Any.
	//
	// Taking stk's LAST value alone was a false claim about the other N-1,
	// and it went unmeasured because it happened to hold wherever that last
	// value was a dynamic carrier — which covers anything, and is what
	// control.tsv §7's counter loop leaves. The row that exposed it is the
	// one the fifty-first increment graduated, whose last value is a
	// concrete Integer over a body that also leaves the flex container.
	for _, v := range stk {
		if v.Parent() != top.Parent() {
			elem = NewTypeLiteral(TAny)
			break
		}
	}
	return []Value{NewVariadicCarrier(elem)}
}

// emptyWhileCond reports whether a while's condition operand is the
// literally-empty list — the one condition shape whose value count is
// known from the SOURCE rather than inferred from the analysis. The
// concreteness guard rides in the same expression deliberately: a
// carrier list also carries a payload (ChildTypeInfo), and reading a
// zero length off one would trap a condition whose runtime length is
// not known at all.
func emptyWhileCond(cond Value) bool {
	lst, err := AsList(cond)
	return err == nil && IsConcrete(&cond) && lst.Len() == 0
}
