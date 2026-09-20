package native

// TimeAsyncModuleNatives holds the clock / timer / async words that were moved
// out of the core registry into the boru:time-util module (TimeUtil namespace):
//
//	now       the current instant (host clock)
//	sleep     block for N milliseconds
//	timeout   run a body once after a delay (returns a Timeout handle)
//	interval  run a body repeatedly every N ms (returns an Interval handle)
//	await     wait for a timer/async body to complete
//	cancel    cancel a Timeout or Interval
//
// The handlers stay in their feature files (natives.go / native_misc.go) in
// this package; only modules.BuildTimeModule registers these, so they are no
// longer available unqualified. At dispatch the module wrapper short-circuits
// to the inner handler against the live registry (host clock, timers).
// Parameterised by the per-import temporal mints (tt) — the Timeout /
// Interval handles the timer words return carry that import's types.
func TimeAsyncModuleNatives(tt TemporalModuleTypes) []NativeFunc {
	return []NativeFunc{
		{
			Name: "now",
			Signatures: []Signature{{
				Args:    []*Type{},
				Impl:    Go(nowHandler),
				Returns: []*Type{TInstant}, BarrierPos: 0,
			}},
		},
		{
			Name: "sleep",
			Signatures: []Signature{{
				Args:    []*Type{TInteger},
				Impl:    Go(sleepHandler),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "timeout",
			// The /q'd body (a code list, or an atom naming a fn) is inert data the
			// handler stores in the Timeout instance; it bakes + CALL_NATIVE (the body
			// still runs interpreted when the timer fires). The list body is also
			// NoEvalArgs: the execMatch auto-eval gate keys ONLY off NoEvalArgs (not
			// QuoteArgs), so without it `TimeUtil.timeout 1000 [body]` — dispatched
			// via the module wrapper's trivial delegation, which runs execMatch on
			// THIS sig — would sub-Run the body eagerly instead of storing it.
			CompileEffect: CompileQuoteInert,
			Signatures: []Signature{
				{Args: []*Type{TInteger, TList}, QuoteArgs: map[int]bool{1: true}, NoEvalArgs: map[int]bool{1: true}, Impl: Go(timeoutListHandler(tt)), Returns: []*Type{tt.Timeout}, BarrierPos: -1},
				{Args: []*Type{TInteger, TAtom}, QuoteArgs: map[int]bool{1: true}, Impl: Go(timeoutWordHandler(tt)), Returns: []*Type{tt.Timeout}, BarrierPos: -1},
			},
		},
		{
			Name: "interval",
			// Like timeout: the /q'd body is inert data stored in the Interval, and
			// the list body is NoEvalArgs so the wrapper does not eagerly sub-Run it.
			CompileEffect: CompileQuoteInert,
			Signatures: []Signature{
				{Args: []*Type{TInteger, TList}, QuoteArgs: map[int]bool{1: true}, NoEvalArgs: map[int]bool{1: true}, Impl: Go(intervalListHandler(tt)), Returns: []*Type{tt.Interval}, BarrierPos: -1},
				{Args: []*Type{TInteger, TAtom}, QuoteArgs: map[int]bool{1: true}, Impl: Go(intervalAtomHandler(tt)), Returns: []*Type{tt.Interval}, BarrierPos: -1},
			},
		},
		{
			Name: "await",
			// The parallels list's ELEMENTS are branch code-bodies stored to run
			// later on per-branch forks: the recorder compiles each to its own
			// 0-param unit (the spawn pattern, per element) and runParallelBranch
			// runs the carriers via RunUnit; declined elements interpret unchanged.
			CompileEffect: CompileStoresBodyList,
			Signatures: []Signature{
				{Args: []*Type{TOptions, TList}, NoEvalArgs: map[int]bool{1: true}, Impl: Go(awaitWithOptsHandler), ReturnsFn: awaitModeMirror(), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TList}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(awaitDefaultHandler), ReturnsFn: awaitDefaultReturns, Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},
		{
			Name: "cancel",
			Signatures: []Signature{
				{Args: []*Type{tt.Timeout}, Impl: Go(cancelTimeoutHandler), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{tt.Interval}, Impl: Go(cancelIntervalofHandler), Returns: []*Type{}, BarrierPos: -1},
			},
		},
	}
}
