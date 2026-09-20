package native

// NetModuleNatives holds the HTTP / API access words that were moved out of
// the core registry into the loadable `boru:net` module (namespace `Net`).
// They are registered ONLY into that module's sub-registry by
// modules.BuildNetModule — deliberately absent from the global registry. The
// handlers themselves still live in their feature files (fetch.go's
// fetch*Handler, the API prepare/direct handlers in this package).
//
// At dispatch the module FnDef wrapper short-circuits to the inner native's
// handler, which runs against the LIVE engine registry (execMatch passes
// e.registry, not the sub-registry) — so Net.fetch reaches the host network
// policy and Net.prepare / Net.direct reach the host API SDKs, exactly as the
// former core words did.
//
//	fetch    perform an HTTP request (URL string and/or options map)
//	prepare  build a request from an API descriptor without sending
//	direct   build-and-send a request against an API descriptor
//
// prepare/direct carry the shared API-descriptor pattern on arg 0 (the inner
// native's signature drives dot-access dispatch, so the pattern is preserved
// even though the wrapper FnSig does not copy it).
// Parameterised by the per-import Fetch mints (ft) — the Response
// values fetch returns carry that import's types.
// apiDescriptorMirror is the check-mode guaranteed-error mirror for the
// API words (prepare / direct): it runs ValidateAPIDescriptor — getSDK's
// own pure prefix, sharing its lines so the message cannot drift — so a
// descriptor that provably fails validation (a missing "spec", a
// non-String spec or entity) is reported at check time with the
// byte-identical detail. Everything getSDK does AFTER that prefix reads
// host state (the SDK cache, the manager) and never runs during
// analysis.
//
// Gated on DeepConcreteOptions: a descriptor whose spec is computed
// (`{kind:"api" spec:(join …)}`) is a concrete Map with a carrier field,
// and reading that field as a missing/malformed spec would flag a
// correct program. The residual is the declared Any either way.
func apiDescriptorMirror(word string) ReturnsFunc {
	return MirrorReturns(word, DeepConcreteOptions,
		func(args []Value, _ *Registry) error {
			m, err := RequireConcreteMap(args[0], word)
			if err != nil {
				return nil // not a readable map: dispatch owns the compile failure
			}
			_, _, vErr := ValidateAPIDescriptor(m, word)
			return vErr
		},
		ReturnsStatic(TAny))
}

func NetModuleNatives(ft FetchModuleTypes) []NativeFunc {
	return []NativeFunc{
		{
			Name: "fetch",
			Signatures: []Signature{
				// Every form returns this import's Fetch/Response value (doFetch).
				{Args: []*Type{TString, TMap}, Impl: Go(ft.fetchStringMapHandler), Returns: []*Type{ft.Response}, BarrierPos: -1},
				{Args: []*Type{TMap}, Impl: Go(ft.fetchMapHandler), Returns: []*Type{ft.Response}, BarrierPos: -1},
				{Args: []*Type{TString}, Impl: Go(ft.fetchStringHandler), Returns: []*Type{ft.Response}, BarrierPos: -1},
			},
		},
		{
			Name: "prepare",
			Signatures: []Signature{
				// The SDK's prepared fetch definition (anyToValue): Any.
				{Args: []*Type{TMap}, Impl: Go(prepareAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TAny}, ReturnsFn: apiDescriptorMirror("prepare"), BarrierPos: -1},
			},
		},
		{
			Name: "direct",
			Signatures: []Signature{
				// The SDK's raw response (anyToValue): Any.
				{Args: []*Type{TMap}, Impl: Go(directAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TAny}, ReturnsFn: apiDescriptorMirror("direct"), BarrierPos: -1},
			},
		},
	}
}
