package native

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Wave-6 coverage for fetch.go: hostPortFromURL edge arms, the nil-map
// and wrong-typed-field error arms of the fetch handlers, the policy
// denial gate inside doFetch, the body-read failure arm, and the
// fetchConvertBehavior delegation one-liners.

func TestHostPortFromURLEdgeArms(t *testing.T) {
	// Unparsable URL → ("", 0).
	if h, p := hostPortFromURL("http://[::1"); h != "" || p != 0 {
		t.Fatalf("parse error should yield (\"\", 0), got (%q, %d)", h, p)
	}
	// Unknown scheme with no explicit port → (host, 0).
	if h, p := hostPortFromURL("ftp://example.com/x"); h != "example.com" || p != 0 {
		t.Fatalf("unknown scheme should yield (host, 0), got (%q, %d)", h, p)
	}
}

func TestFetchStringMapHandlerArms(t *testing.T) {
	ft := testFetchTypes(t)
	r := seam5Reg(t)

	// Options that are a Map type literal (nil data) → fetch_error.
	_, err := ft.fetchStringMapHandler(
		[]Value{NewString("http://example.invalid"), NewTypeLiteral(TMap)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected map for options") {
		t.Fatalf("expected nil-options error, got %v", err)
	}

	// A "url" key inside the options map is skipped (first arg wins).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()
	opts := NewOrderedMap()
	opts.Set("url", NewString("http://other.invalid"))
	opts.Set("method", NewString("GET"))
	out, err := ft.fetchStringMapHandler(
		[]Value{NewString(ts.URL), NewMap(opts)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("fetchStringMapHandler: %v", err)
	}
	m, _ := AsMap(out[0])
	got, _ := m.Get("url")
	if s, _ := AsString(got); !strings.HasPrefix(s, ts.URL) {
		t.Fatalf("first-arg url must win, got %q", s)
	}
}

func TestFetchMapHandlerNilMap(t *testing.T) {
	ft := testFetchTypes(t)
	r := seam5Reg(t)
	_, err := ft.fetchMapHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "expected map argument") {
		t.Fatalf("expected nil-map error, got %v", err)
	}
}

// fetchReq builds a request map from key/value pairs.
func fetchReq(kv ...any) ReadMap {
	om := NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		om.Set(kv[i].(string), kv[i+1].(Value))
	}
	m, _ := AsMap(NewMap(om))
	return m
}

func TestDoFetchFieldTypeErrorArms(t *testing.T) {
	ft := testFetchTypes(t)

	cases := []struct {
		name string
		req  ReadMap
		want string
	}{
		{"url not string", fetchReq("url", NewInteger(1)), "fetch: url:"},
		{"method not string", fetchReq(
			"url", NewString("http://example.invalid"),
			"method", NewInteger(1)), "fetch: method:"},
		{"body not string", fetchReq(
			"url", NewString("http://example.invalid"),
			"body", NewInteger(1)), "fetch: body:"},
		{"invalid method token", fetchReq(
			"url", NewString("http://example.invalid"),
			"method", NewString("ba d")), "invalid method"},
		{"header not string", func() ReadMap {
			hm := NewOrderedMap()
			hm.Set("x-a", NewInteger(1))
			return fetchReq(
				"url", NewString("http://example.invalid"),
				"headers", NewMap(hm))
		}(), "fetch: header"},
		{"timeout not integer", fetchReq(
			"url", NewString("http://example.invalid"),
			"timeout", NewString("soon")), "fetch: timeout:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ft.doFetch(tc.req, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestDoFetchPolicyDenied(t *testing.T) {
	// sandbox uninstalls the network scope: doFetch must decline before
	// building any request.
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ft := testFetchTypes(t)
	_, err = ft.doFetch(fetchReq("url", NewString("http://example.invalid")), r)
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("expected policy denial, got %v", err)
	}
}

func TestDoFetchBodyReadError(t *testing.T) {
	// The handler declares a longer Content-Length than it writes, so
	// the client's io.ReadAll fails with an unexpected EOF.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer ts.Close()
	ft := testFetchTypes(t)
	_, err := ft.doFetch(fetchReq("url", NewString(ts.URL)), nil)
	if err == nil || !strings.Contains(err.Error(), "reading body") {
		t.Fatalf("expected body-read error, got %v", err)
	}
}

func TestFetchConvertBehaviorDelegates(t *testing.T) {
	b := fetchConvertBehavior{}
	v := NewString("x")
	if !b.Match(v, TString) {
		t.Error("Match must delegate to the default conforms-to check")
	}
	if !b.Equal(v, NewString("x")) || b.Equal(v, NewString("y")) {
		t.Error("Equal must delegate to default value equality")
	}
	if got := b.Format(v); got != v.String() {
		t.Errorf("Format must delegate to String(), got %q", got)
	}
}
