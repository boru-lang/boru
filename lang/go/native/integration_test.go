package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// helper to run boru expressions through the engine and return results
func runBoru(t *testing.T, r *Registry, tokens []Value) []Value {
	t.Helper()
	registerIOWords(r) // read/write/etc moved to boru:io; seed them for the behaviour suite
	e := NewTop(r)
	result, err := e.Run(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func runBoruError(t *testing.T, r *Registry, tokens []Value) error {
	t.Helper()
	registerIOWords(r)
	e := NewTop(r)
	_, err := e.Run(tokens)
	return err
}

// --- Comparison word integration tests ---

func TestEngineLt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("lt"), NewInteger(2)})
	_as0, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as0 {
		t.Errorf("1 lt 2 = %v, want true", result)
	}
}

func TestEngineGt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(3), NewWord("gt"), NewInteger(1)})
	_as1, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as1 {
		t.Errorf("3 gt 1 = %v, want true", result)
	}
}

func TestEngineLte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("lte"), NewInteger(1)})
	_as2, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as2 {
		t.Errorf("1 lte 1 = %v, want true", result)
	}
}

func TestEngineGte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(2), NewWord("gte"), NewInteger(1)})
	_as3, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as3 {
		t.Errorf("2 gte 1 = %v, want true", result)
	}
}

func TestEngineEq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(5), NewWord("eq"), NewInteger(5)})
	_as4, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as4 {
		t.Errorf("5 eq 5 = %v, want true", result)
	}
}

func TestEngineNeq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(5), NewWord("neq"), NewInteger(3)})
	_as5, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as5 {
		t.Errorf("5 neq 3 = %v, want true", result)
	}
	result = runBoru(t, r, []Value{NewInteger(5), NewWord("neq"), NewInteger(5)})
	_as6, _ := AsBoolean(result[0])
	if len(result) != 1 || _as6 {
		t.Errorf("5 neq 5 = %v, want false", result)
	}
}

func TestEngineDeq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewString("a"), NewWord("deq"), NewString("a")})
	_as7, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as7 {
		t.Errorf("'a' deq 'a' = %v, want true", result)
	}
}

func TestEngineLtTotalOrder(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// lt is family-restricted: comparing across type branches (Integer
	// vs List) now raises [boru/incomparable].
	if err := runBoruError(t, r, []Value{NewInteger(1), NewWord("lt"), NewList([]Value{NewInteger(2)})}); err == nil {
		t.Errorf("1 lt [2]: want an incomparable error, got none")
	} else if !strings.Contains(err.Error(), "incomparable") {
		t.Errorf("1 lt [2]: want incomparable error, got %v", err)
	}
	// tcmp keeps the cross-branch total order: Integer ranks below List.
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("tcmp"), NewList([]Value{NewInteger(2)})})
	if len(result) != 1 || !result[0].Parent().Equal(TInteger) {
		t.Errorf("1 tcmp [2] = %v, want an Integer", result)
	}
}

// --- If word integration tests ---

func TestEngineIf3True(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(1), NewInteger(2),
	})
	_as8, _ := AsInteger(result[0])
	if len(result) != 1 || _as8 != 1 {
		t.Errorf("if true 1 2 = %v, want 1", result)
	}
}

func TestEngineIf3False(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(1), NewInteger(2),
	})
	_as9, _ := AsInteger(result[0])
	if len(result) != 1 || _as9 != 2 {
		t.Errorf("if false 1 2 = %v, want 2", result)
	}
}

func TestEngineIf2True(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(true), NewInteger(42),
	})
	_as10, _ := AsInteger(result[0])
	if len(result) != 1 || _as10 != 42 {
		t.Errorf("if true 42 = %v, want 42", result)
	}
}

func TestEngineIf2False(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(false), NewInteger(42),
	})
	if len(result) != 0 {
		t.Errorf("if false 42 = %v, want empty", result)
	}
}

func TestEngineIfListCondition(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// if [1 lt 2] 10 20 → should evaluate condition [1 lt 2] → true → return 10
	condList := NewList([]Value{NewInteger(1), NewWord("lt"), NewInteger(2)})
	result := runBoru(t, r, []Value{
		NewWord("if"), condList, NewInteger(10), NewInteger(20),
	})
	_as11, _ := AsInteger(result[0])
	if len(result) != 1 || _as11 != 10 {
		t.Errorf("if [1 lt 2] 10 20 = %v, want 10", result)
	}
}

func TestEngineIfListBranch(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// if true [1 add 2] [3 add 4] → should evaluate [1 add 2] → 3
	thenList := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	elseList := NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)})
	result := runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(true), thenList, elseList,
	})
	_as12, _ := AsInteger(result[0])
	if len(result) != 1 || _as12 != 3 {
		t.Errorf("if true [1 add 2] [3 add 4] = %v, want 3", result)
	}
}

func TestEngineIfOnlyChosenBranchExecutes(t *testing.T) {
	// Register a side-effect word that increments a counter
	callCount := 0
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	r.Register("side-effect",
		Signature{
			Args: []*Type{TAny},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				callCount++
				return args, nil
			}), BarrierPos: -

			// if true [side-effect 1] [side-effect 2] → only then-branch runs
			1,
		},
	)

	thenList := NewList([]Value{NewWord("side-effect"), NewInteger(1)})
	elseList := NewList([]Value{NewWord("side-effect"), NewInteger(2)})
	result := runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(true), thenList, elseList,
	})
	if callCount != 1 {
		t.Errorf("expected side-effect called once, got %d", callCount)
	}
	_as13, _ := AsInteger(result[0])
	if len(result) != 1 || _as13 != 1 {
		t.Errorf("expected [1], got %v", result)
	}

	// Reset and test false branch
	callCount = 0
	result = runBoru(t, r, []Value{
		NewWord("if"), NewBoolean(false), thenList, elseList,
	})
	if callCount != 1 {
		t.Errorf("expected side-effect called once, got %d", callCount)
	}
	_as14, _ := AsInteger(result[0])
	if len(result) != 1 || _as14 != 2 {
		t.Errorf("expected [2], got %v", result)
	}
}

func TestEngineIfFalsy(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// if 0 1 2 → 0 is falsy → return 2
	result := runBoru(t, r, []Value{
		NewWord("if"), NewInteger(0), NewInteger(1), NewInteger(2),
	})
	_as15, _ := AsInteger(result[0])
	if len(result) != 1 || _as15 != 2 {
		t.Errorf("if 0 1 2 = %v, want 2", result)
	}
}

// --- File I/O integration tests ---

func TestEngineReadBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["test.txt"] = []byte("hello world")
	SetHostFileOps(r, mem)

	result := runBoru(t, r, []Value{NewWord("read"), pathV("test.txt")})
	_as16, _ := AsString(result[0])
	if len(result) != 1 || _as16 != "hello world" {
		t.Errorf("read 'test.txt' = %v, want 'hello world'", result)
	}
}

func TestEngineReadWithOpts(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.txt"] = []byte("a\nb\nc")
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("lines"))
	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.txt"), NewMap(opts)})
	if len(result) != 1 || !result[0].Parent().Equal(TList) {
		t.Errorf("read with lines fmt = %v, want list", result)
	}
	_lst, _ := AsList(result[0])
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Errorf("expected 3 lines, got %d", len(elems))
	}
}

func TestEngineReadJSON(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.json"] = []byte(`{"x":1}`)
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.json"), NewMap(opts)})
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Errorf("read json = %v, want map", result)
	}
}

func TestEngineReadNotFound(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	err = runBoruError(t, r, []Value{NewWord("read"), pathV("nope.txt")})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEngineReadUnknownFormat(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["test.txt"] = []byte("data")
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("nonesuch"))
	err = runBoruError(t, r, []Value{NewWord("read"), pathV("test.txt"), NewMap(opts)})
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestEngineWriteBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	result := runBoru(t, r, []Value{NewWord("write"), pathV("out.txt"), NewString("hello")})
	if len(result) != 1 || !IsPathon(result[0]) || result[0].String() != "out.txt" {
		t.Errorf("write result = %v, want 'out.txt'", result)
	}
	if string(mem.Files["out.txt"]) != "hello" {
		t.Errorf("file content = %q, want %q", mem.Files["out.txt"], "hello")
	}
}

func TestEngineWriteWithOpts(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("nl", NewString("crlf"))
	result := runBoru(t, r, []Value{NewWord("write"), pathV("out.txt"), NewString("a\nb"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if string(mem.Files["out.txt"]) != "a\r\nb" {
		t.Errorf("file content = %q, want %q", mem.Files["out.txt"], "a\r\nb")
	}
}

func TestEngineWriteAppend(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["log.txt"] = []byte("first\n")
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runBoru(t, r, []Value{NewWord("write"), pathV("log.txt"), NewString("second\n"), NewMap(opts)})
	if string(mem.Files["log.txt"]) != "first\nsecond\n" {
		t.Errorf("file content = %q, want %q", mem.Files["log.txt"], "first\nsecond\n")
	}
}

func TestEngineWriteAppendNewFile(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runBoru(t, r, []Value{NewWord("write"), pathV("new.txt"), NewString("data"), NewMap(opts)})
	if string(mem.Files["new.txt"]) != "data" {
		t.Errorf("file content = %q, want %q", mem.Files["new.txt"], "data")
	}
}

func TestEngineWriteAnyOpts(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// write non-string (map) value with options → auto-serializes with jsonic
	// The write [string, any, map] signature expects path, data, opts
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	// All prefix: nearest→sig[0]=path, next→sig[1]=data, deepest→sig[2]=opts
	runBoru(t, r, []Value{NewMap(opts), NewMap(m), pathV("out.json"), NewWord("write")})
	content := string(mem.Files["out.json"])
	if content == "" {
		t.Errorf("file was not written")
	}
}

func TestEngineReadLineEndings(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["crlf.txt"] = []byte("a\r\nb\r\nc")
	SetHostFileOps(r, mem)

	// Default nl:"lf" normalizes \r\n to \n
	result := runBoru(t, r, []Value{NewWord("read"), pathV("crlf.txt")})
	_as18, _ := AsString(result[0])
	if _as18 != "a\nb\nc" {
		_as19, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as19, "a\nb\nc")
	}

	// nl:"raw" preserves original
	opts := NewOrderedMap()
	opts.Set("nl", NewString("raw"))
	result = runBoru(t, r, []Value{NewWord("read"), pathV("crlf.txt"), NewMap(opts)})
	_as20, _ := AsString(result[0])
	if _as20 != "a\r\nb\r\nc" {
		_as21, _ := AsString(result[0])
		t.Errorf("raw got %q, want %q", _as21, "a\r\nb\r\nc")
	}
}

// --- Registry integration tests ---

func TestRegistrySetFileOps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)
	if HostFileOps(r) != mem {
		t.Error("SetHostFileOps did not install the FileOps capability")
	}
}

func TestRegistryMatchNoFunction(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := r.Match("nonexistent", []Value{}, WordInfo{})
	if result != nil {
		t.Error("expected nil for nonexistent function")
	}
}
