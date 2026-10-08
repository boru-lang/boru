package native

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// checkFetchPolicy consults the registry's host policy (if any)
// before fetch issues an outbound request. The check sequence runs
// global.network → network install → network.connect{host, port};
// any denial returns the policy error so no HTTP request is built.
//
// When r is nil or no policy is installed the function is a no-op
// (the historical "no permissions configured" default).
//
// Errors are returned in *policy.Denied shape when the policy
// declines; URL-parse failures are returned as ordinary errors so
// callers can distinguish "bad URL" from "policy denied".
func checkFetchPolicy(r *Registry, urlStr string) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	// Resolve host/port from the URL before invoking the rule check
	// so where-predicates can match on host: and port: fields.
	host, port := hostPortFromURL(urlStr)
	args := policy.Args{
		"url":  urlStr,
		"host": host,
		"port": port,
	}
	// Per design: the network scope's "connect" op is what fetch
	// performs. global.network is consulted by the wrapper sequence
	// via Check; install:false on the network scope produces
	// capability_not_installed.
	if !pol.Installed("network") {
		return PolicyRefusal(r, "fetch", &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "network",
			Op:      "connect",
			Profile: pol.Name(),
			Blame:   "network.install=false",
			Args:    args,
		})
	}
	return PolicyRefusal(r, "fetch", pol.Check("network", "connect", args))
}

// hostPortFromURL extracts (host, port) from a URL. port is
// inferred from the scheme when absent (80 for http, 443 for https).
// Parse errors yield ("", 0) — the policy then matches on the
// surviving args only.
func hostPortFromURL(rawURL string) (string, int) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return "", 0
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			return host, p
		}
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return host, 80
	case "https":
		return host, 443
	case "ws":
		return host, 80
	case "wss":
		return host, 443
	}
	return host, 0
}

// The Fetch family — the Fetch root and its Request / Response leaves
// — is owned by boru:net as per-import module mints (former global
// FixedIDs 3000-3002, retired): BuildNetModule mints them via
// MintFetchTypes and threads them to the fetch handlers, whose
// Response values escape to the importer. Reachable after import as
// Net.Fetch / Net.Request / Net.Response.

// FetchModuleTypes are boru:net's minted types.
type FetchModuleTypes struct {
	Fetch    *Type
	Request  *Type
	Response *Type
}

// MintFetchTypes mints the Fetch family into r's type table (r is
// boru:net's sub-registry) and returns the nodes.
func MintFetchTypes(r *Registry) FetchModuleTypes {
	fetch := r.Types.MintType("Fetch", core.TIdeal)
	return FetchModuleTypes{
		Fetch:    fetch,
		Request:  r.Types.MintTypeWithBehavior("Request", fetch, fetchConvertBehavior{}),
		Response: r.Types.MintTypeWithBehavior("Response", fetch, fetchConvertBehavior{}),
	}
}

const defaultFetchTimeout = 30 * time.Second

// The "fetch" word is registered via the consolidated Natives slice in
// natives.go. Handlers cover [string], [map], and [string, map] forms.
//
// All entry points consult the registry's policy before issuing the
// outbound request (or decline if the network capability is
// uninstalled). The check sequence is:
//  1. global.network hard cap
//  2. network capability install gate
//  3. network.connect{host, port} per-host rule
//
// If any check denies, the HTTP request is never built and no
// outbound packet is sent.
//
// fetchStringHandler handles fetch with a single URL string argument.
// Performs a GET request to the given URL.
func (ft FetchModuleTypes) fetchStringHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	reqOM := NewOrderedMap()
	reqOM.Set("url", args[0])
	return ft.doFetch(reqOM, r)
}

// fetchStringMapHandler handles fetch with a URL string and an options map.
// The URL is merged into the options map as the "url" field.
func (ft FetchModuleTypes) fetchStringMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	opts, _ := AsMap(args[1])
	if opts == nil {
		return nil, r.BoruError("fetch_error", "fetch: expected map for options, got nil", "fetch")
	}
	reqOM := NewOrderedMap()
	reqOM.Set("url", args[0])
	// Copy options into request map (url from first arg takes precedence).
	for _, key := range opts.Keys() {
		if key == "url" {
			continue
		}
		val, _ := opts.Get(key)
		reqOM.Set(key, val)
	}
	return ft.doFetch(reqOM, r)
}

// fetchMapHandler handles fetch with a full request map.
// The map must contain a "url" field.
func (ft FetchModuleTypes) fetchMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	m, _ := AsMap(args[0])
	if m == nil {
		return nil, r.BoruError("fetch_error", "fetch: expected map argument, got nil", "fetch")
	}
	return ft.doFetch(m, r)
}

// doFetch performs a synchronous HTTP request from the given request map
// and returns a Map/Fetch/Response value.
//
// Consults the registry's policy (if any) before issuing the request:
// global.network hard cap, network scope install, and
// network.connect{host, port} per-host rule. Denial returns the
// policy error without building or sending any HTTP request.
//
// Request map fields:
//   - url     (string, required) — the URL to fetch
//   - method  (string, optional, default "GET") — HTTP method
//   - headers (map, optional) — request headers
//   - body    (string, optional) — request body
//   - timeout (integer, optional, default 30000) — timeout in milliseconds
func (ft FetchModuleTypes) doFetch(reqOM ReadMap, r *Registry) ([]Value, error) {
	// Extract url (required).
	urlVal, ok := reqOM.Get("url")
	if !ok {
		return nil, r.BoruError("fetch_error", "fetch: missing required \"url\" field", "fetch")
	}
	urlStr, err := AsString(urlVal)
	if err != nil {
		return nil, r.BoruError("fetch_error", "fetch: url: "+err.Error(), "fetch")
	}

	// Policy gate: consult host policy before opening any socket.
	if err := checkFetchPolicy(r, urlStr); err != nil {
		return nil, err
	}

	// Extract method (default GET).
	method := "GET"
	if mv, ok := reqOM.Get("method"); ok {
		mvStr, err := AsString(mv)
		if err != nil {
			return nil, r.BoruError("fetch_error", "fetch: method: "+err.Error(), "fetch")
		}
		method = strings.ToUpper(mvStr)
	}

	// Extract body.
	var bodyReader io.Reader
	if bv, ok := reqOM.Get("body"); ok {
		bvStr, err := AsString(bv)
		if err != nil {
			return nil, r.BoruError("fetch_error", "fetch: body: "+err.Error(), "fetch")
		}
		bodyReader = strings.NewReader(bvStr)
	}

	// Build http.Request.
	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, r.BoruError("fetch_error", "fetch: "+err.Error(), "fetch")
	}

	// Set headers.
	if hv, ok := reqOM.Get("headers"); ok && hv.Parent.ConformsTo(TMap) {
		hm, _ := AsMap(hv)
		for _, key := range hm.Keys() {
			val, _ := hm.Get(key)
			valStr, err := AsString(val)
			if err != nil {
				return nil, r.BoruError("fetch_error",
					fmt.Sprintf("fetch: header %q: %v", key, err), "fetch")
			}
			req.Header.Set(key, valStr)
		}
	}

	// Timeout.
	timeout := defaultFetchTimeout
	if tv, ok := reqOM.Get("timeout"); ok {
		tvInt, err := AsInteger(tv)
		if err != nil {
			return nil, r.BoruError("fetch_error", "fetch: timeout: "+err.Error(), "fetch")
		}
		timeout = time.Duration(tvInt) * time.Millisecond
	}

	// TLS options (§4.3 of the TLS plan). Parsed and policy-gated
	// before the effect is noted — a denied `verify: false` must not send.
	var tlsProfile capabilities.TLSProfile
	if tv, ok := reqOM.Get("tls"); ok {
		tlsProfile, err = ParseTLSOpts(r, tv, "fetch")
		if err != nil {
			return nil, err
		}
		host, port := hostPortFromURL(urlStr)
		if err := CheckTLSPolicy(r, tlsProfile, host, port); err != nil {
			return nil, err
		}
	}

	// The code is `transport` per NETWORK-CLIENTS.0.md §8.2 so a guest
	// can discriminate it in `do […] error [case …]`. The surrounding
	// legacy paths still return bare fmt.Errorf (and so surface as
	// internal_error); new paths do not inherit that.
	transport, err := ResolveTransport(r, tlsProfile, "fetch")
	if err != nil {
		// An already-coded error (an unknown identity, a bad CA) passes
		// through with ITS code — re-wrapping would both bury the
		// specific code under `transport` and print the rendered inner
		// error inside the outer one.
		var coded *BoruError
		if errors.As(err, &coded) {
			return nil, err
		}
		return nil, r.BoruError("transport",
			"fetch: tls: "+err.Error(), "fetch")
	}

	// Execute request. effect ledger (core effects.go): once Do runs, the
	// request may have reached the peer even when it returns an error (sent,
	// response lost), so the effect is noted on the attempt — everything
	// before this point (policy denial, a malformed request) provably sent
	// nothing and stays uncounted.
	r.NoteEffect()
	client := &http.Client{Timeout: timeout, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		// The wire failed: classify into closed/timeout/transport so a
		// guest can retry a timeout and fail fast on the rest.
		return nil, MapTransportErr(r, "fetch", err)
	}
	defer resp.Body.Close()

	// Read response body.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, MapTransportErrAs(r, "fetch", "fetch: reading body", err)
	}

	// Build response headers map with lowercase keys in sorted order.
	headersOM := NewOrderedMap()
	headerKeys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)
	for _, k := range headerKeys {
		headersOM.Set(strings.ToLower(k), NewString(strings.Join(resp.Header[k], ", ")))
	}

	// Build response map.
	respOM := NewOrderedMap()
	respOM.Set("ok", NewBoolean(resp.StatusCode >= 200 && resp.StatusCode <= 299))
	respOM.Set("status", NewInteger(int64(resp.StatusCode)))
	respOM.Set("headers", NewMap(headersOM))
	respOM.Set("body", NewString(string(bodyBytes)))
	respOM.Set("url", NewString(resp.Request.URL.String()))

	return []Value{{Parent: ft.Response, Data: MapPayload{M: respOM}}}, nil
}

// fetchConvertBehavior projects a Fetch Request/Response (map-backed
// values) to its map via IdealConverter. Format/Match/Equal stay default.
type fetchConvertBehavior struct{}

func (fetchConvertBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (fetchConvertBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (fetchConvertBehavior) Format(v Value) string       { return core.DefaultBehavior.Format(v) }
func (fetchConvertBehavior) ToMap(v Value) (Value, error) {
	out := NewOrderedMap()
	if m, err := AsMap(v); err == nil {
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			out.Set(k, val)
		}
	}
	return NewMap(out), nil
}
func (fetchConvertBehavior) ToList(v Value) (Value, error) {
	var vals []Value
	if m, err := AsMap(v); err == nil {
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			vals = append(vals, val)
		}
	}
	return NewList(vals), nil
}

// fetchFieldAt reads a field from a map-backed Fetch value (Request or
// Response). Returns the value and whether the key was present.
func fetchFieldAt(recv Value, key string) (Value, bool) {
	m, err := AsMap(recv)
	if err != nil || m == nil {
		return NewTypeLiteral(TNone), false
	}
	v, ok := m.Get(key)
	if !ok {
		return NewTypeLiteral(TNone), false
	}
	return v, true
}

// fetchFieldKey extracts the key from an accessor's first argument,
// which is an Atom for the `.field` sugar and a String for `get "k"`.
//
// It cannot fail: the signatures admit only TAtom and TString in this
// position, so there is no third shape to reject. An unusable value
// (a DepScalar constraint in the String slot) yields "", which reads
// as a miss — the same answer a genuinely absent field gives.
func fetchFieldKey(v Value) string {
	if a, err := v.AsConcreteAtom(); err == nil && a != "" {
		return a
	}
	s, _ := v.AsConcreteString()
	return s
}

// fetchGetHandler is the lenient read (`dot` / `get`): a missing field
// reads none, matching how the accessor family treats Maps.
func fetchGetHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v, _ := fetchFieldAt(args[1], fetchFieldKey(args[0]))
	return []Value{v}, nil
}

// fetchGetrHandler is the strict twin (`dotr` / `getr`): a missing field
// is an error rather than none.
func fetchGetrHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := fetchFieldKey(args[0])
	v, found := fetchFieldAt(args[1], key)
	if !found {
		return nil, r.BoruError("key_error",
			"getr: no field \""+key+"\" on this Fetch value", "getr")
	}
	return []Value{v}, nil
}

// fetchHasHandler answers whether the field is present.
func fetchHasHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_, found := fetchFieldAt(args[1], fetchFieldKey(args[0]))
	return []Value{NewBoolean(found)}, nil
}

// FetchAccessorExtensions builds the accessor WORD EXTENSIONS that make
// a Response answer `.status` directly, instead of requiring the
// undiscoverable `convert Map` step first.
//
// This is the boru:time-util / boru:matrix-util pattern
// (design/OPEN-WORDS.0.md): a module extends a CORE word with overloads
// anchored on its own minted type, and `import` transplants them onto
// the importer's bare word. It is the only mechanism available here —
// the Fetch family is minted per import, so no static signature in
// native_storage.go could name it, and a catch-all sig on TIdeal would
// hand dot access to Error, Table, Record and every other Ideal.
//
// The sigs are anchored on ft.Fetch, the family root, so Request and
// Response both match via ConformsTo.
func FetchAccessorExtensions(ft FetchModuleTypes) []FnDefInfo {
	lenient := func(quote bool) []Signature {
		atom := Signature{Args: []*Type{TAtom, ft.Fetch}, BarrierPos: 1,
			Impl: Go(fetchGetHandler), Returns: []*Type{TAny}}
		if quote {
			atom.QuoteArgs = map[int]bool{0: true}
		}
		return []Signature{atom,
			{Args: []*Type{TString, ft.Fetch}, BarrierPos: 1,
				Impl: Go(fetchGetHandler), Returns: []*Type{TAny}}}
	}
	strict := func(quote bool) []Signature {
		atom := Signature{Args: []*Type{TAtom, ft.Fetch}, BarrierPos: 1,
			Impl: Go(fetchGetrHandler), Returns: []*Type{TAny}}
		if quote {
			atom.QuoteArgs = map[int]bool{0: true}
		}
		return []Signature{atom,
			{Args: []*Type{TString, ft.Fetch}, BarrierPos: 1,
				Impl: Go(fetchGetrHandler), Returns: []*Type{TAny}}}
	}
	hasSigs := []Signature{
		// `has` evaluates its key (2026-08-21 get-parity ruling) — the
		// TAtom overload matches an EVALUATED atom key (`r has status/q`),
		// aligned with the core accessor's sigs.
		{Args: []*Type{TAtom, ft.Fetch}, BarrierPos: 1,
			Impl: Go(fetchHasHandler), Returns: []*Type{TBoolean}},
		{Args: []*Type{TString, ft.Fetch}, BarrierPos: 1,
			Impl: Go(fetchHasHandler), Returns: []*Type{TBoolean}},
	}
	return []FnDefInfo{
		// dot quotes a bare-word key (the `.field` sugar); get evaluates
		// it — the split documented in lang/go/CLAUDE.md.
		NewWordExtension("boru:net", "dot", lenient(true)),
		NewWordExtension("boru:net", "get", lenient(false)),
		NewWordExtension("boru:net", "dotr", strict(true)),
		NewWordExtension("boru:net", "getr", strict(false)),
		NewWordExtension("boru:net", "has", hasSigs),
	}
}
