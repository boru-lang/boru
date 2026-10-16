package native

import (
	"bytes"
	"strings"
	"testing"
)

// ioRegistry returns a registry with the boru:io words registered against
// a freshly minted StreamKind — the same shape BuildIOModule produces per
// import — and returns that StreamKind so tests can assert against it.
func ioRegistry(t *testing.T) (*Registry, *Type) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	sk := MintStreamKind(r)
	for _, n := range IOModuleNativeFuncs(IOModuleTypes{StreamKind: sk, FileType: MintFileType(r), Watcher: MintWatcherType(r), File: MintFileHandleType(r), Lock: MintLockType(r), Mmap: MintMmapType(r)}) {
		r.RegisterNativeFunc(n)
	}
	return r, sk
}

// The stream handles are Atoms of the StreamKind type — not string sentinels.
func TestStreamHandlesAreStreamKindAtoms(t *testing.T) {
	r, sk := ioRegistry(t)

	for _, name := range []string{"stdin", "stdout", "stderr"} {
		result := runBoru(t, r, []Value{NewWord(name)})
		if len(result) != 1 {
			t.Fatalf("%s: expected 1 result, got %d", name, len(result))
		}
		got := result[0]
		if !IsAtom(got) {
			t.Errorf("%s: result is not an Atom (parent=%s)", name, got.Parent())
		}
		if n, _ := AsAtom(got); n != name {
			t.Errorf("%s: atom name = %q, want %q", name, n, name)
		}
		if !got.Is(sk) {
			t.Errorf("%s: handle does not inhabit StreamKind", name)
		}
		if !got.Parent().Equal(sk) {
			t.Errorf("%s: typeof handle = %s, want StreamKind", name, got.Parent())
		}
	}
}

// StreamKind admits exactly the three handles — and nothing else. Membership
// is name-based, so a handle minted by one StreamKind is admitted by another.
func TestStreamKindTypeMembership(t *testing.T) {
	r, _ := DefaultRegistry()
	sk := MintStreamKind(r)
	other := MintStreamKind(r) // a distinct StreamKind node

	if !addrOf(newStreamAtom("stdout", sk)).Is(sk) {
		t.Error("stdout handle should be a StreamKind")
	}
	if !addrOf(newStreamAtom("stdout", sk)).Is(other) {
		t.Error("a handle minted by one StreamKind should inhabit another (name-based)")
	}
	// A bare atom of the right name is admitted too (the type IS those atoms).
	if !addrOf(NewAtom("stderr")).Is(sk) {
		t.Error("a bare `stderr` atom should be a StreamKind")
	}
	// Unrelated atoms and non-atoms are rejected.
	if addrOf(NewAtom("foo")).Is(sk) {
		t.Error("`foo` atom must not be a StreamKind")
	}
	if addrOf(NewString("stdout")).Is(sk) {
		t.Error("the String \"stdout\" must not be a StreamKind")
	}
	if addrOf(NewInteger(1)).Is(sk) {
		t.Error("an Integer must not be a StreamKind")
	}
}

// Writing non-string data requires no options map — the value is serialized
// automatically and round-trips through read.
func TestWriteNonStringNoOptsMap(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		write  []Value
		expect string
	}{
		{
			name:   "map",
			path:   "mem://m.json",
			write:  mapTokens("a", NewInteger(1), "b", NewInteger(2)),
			expect: "{a:1 b:2}",
		},
		{
			name:   "list",
			path:   "mem://l.jsonic",
			write:  []Value{NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})},
			expect: "[1 2 3]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := ioRegistry(t)
			setupMemFS(t, r)

			// write path value  (no opts map)
			toks := append([]Value{NewWord("write"), pathV(tc.path)}, tc.write...)
			wres := runBoru(t, r, toks)
			if len(wres) != 1 {
				t.Fatalf("write returned %d values, want 1 (the path)", len(wres))
			}
			if !IsPathon(wres[0]) {
				t.Fatalf("write returned %v, want the Pathon target", wres[0])
			}

			// read it back
			rres := runBoru(t, r, []Value{NewWord("read"), pathV(tc.path)})
			if len(rres) != 1 {
				t.Fatalf("read returned %d values, want 1", len(rres))
			}
			if got := rres[0].String(); got != tc.expect {
				t.Errorf("round-trip = %s, want %s", got, tc.expect)
			}
		})
	}
}

// Writing to a stream handle routes to the host writer (no newline), and
// returns the handle itself.
func TestWriteToStdoutStream(t *testing.T) {
	r, sk := ioRegistry(t)
	var buf bytes.Buffer
	r.Output = &buf

	result := runBoru(t, r, []Value{NewWord("write"), newStreamAtom("stdout", sk), NewString("hello")})
	if buf.String() != "hello" {
		t.Errorf("stdout = %q, want %q", buf.String(), "hello")
	}
	if len(result) != 1 || !result[0].Is(sk) {
		t.Fatalf("write should return the StreamKind handle, got %v", result)
	}

	// A non-string value to a stream serializes with no opts map.
	buf.Reset()
	runBoru(t, r, append([]Value{NewWord("write"), newStreamAtom("stdout", sk)},
		mapTokens("x", NewInteger(7))...))
	if got := buf.String(); got != `{"x":7}` {
		t.Errorf("stdout = %q, want %q", got, `{"x":7}`)
	}
}

// Reading from an output stream is a clean error, not a panic.
func TestReadOutputStreamErrors(t *testing.T) {
	r, sk := ioRegistry(t)

	err := runBoruError(t, r, []Value{NewWord("read"), newStreamAtom("stdout", sk)})
	if err == nil {
		t.Fatal("expected an error reading from stdout")
	}
	if !strings.Contains(err.Error(), "read_error") {
		t.Errorf("error = %v, want a read_error", err)
	}
}

// Passing a bare StreamKind type literal must not panic any IO handler.
func TestStreamKindTypeLiteralNoPanic(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic on StreamKind type literal: %v", rec)
		}
	}()
	r, _ := DefaultRegistry()
	sk := MintStreamKind(r)
	lit := NewTypeLiteral(sk)
	_, _ = streamSentinel(lit) // type literal → not a concrete stream atom
	_ = lit.Is(sk)
	_ = extractPath(lit)
}

// mapTokens builds the token form `{k0:v0 k1:v1 …}` as a single map Value.
func mapTokens(kv ...any) []Value {
	m := NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i].(string), kv[i+1].(Value))
	}
	return []Value{NewMap(m)}
}
