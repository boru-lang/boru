package native

import (
	"bytes"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

func TestBinaryWriteRead(t *testing.T) {
	r, mem := ioFSReg(t)
	// write raw bytes, then read them back as Bytes.
	res := runBoru(t, r, []Value{NewWord("write"), pathV("b.bin"), NewBytesValue([]byte{0, 1, 2, 255})})
	if !IsPathon(res[0]) || res[0].String() != "b.bin" {
		t.Errorf("binary write returned %v", res[0])
	}
	if got, _ := mem.ReadFile("b.bin"); !bytes.Equal(got, []byte{0, 1, 2, 255}) {
		t.Errorf("stored bytes = %v", got)
	}
	res = runBoru(t, r, []Value{
		NewWord("read"), pathV("b.bin"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bytes")) }),
	})
	if b, ok := AsBytesValue(res[0]); !ok || !bytes.Equal(b, []byte{0, 1, 2, 255}) {
		t.Errorf("read bytes = %v (ok=%v)", b, ok)
	}
	// {enc:'binary'} is an alias for bytes.
	res = runBoru(t, r, []Value{
		NewWord("read"), pathV("b.bin"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("binary")) }),
	})
	if _, ok := AsBytesValue(res[0]); !ok {
		t.Errorf("read {enc:binary} did not return Bytes: %v", res[0])
	}
}

func TestPositionedRead(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("h.txt", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// positioned text slice.
	res := runBoru(t, r, []Value{
		NewWord("read"), pathV("h.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(1)); om.Set("length", NewInteger(3)) }),
	})
	if res[0].String() != "'ell'" {
		t.Errorf("positioned read = %v, want 'ell'", res[0])
	}
	// positioned byte slice.
	res = runBoru(t, r, []Value{
		NewWord("read"), pathV("h.txt"),
		wrapMap(func(om *OrderedMap) {
			om.Set("enc", NewString("bytes"))
			om.Set("offset", NewInteger(1))
			om.Set("length", NewInteger(3))
		}),
	})
	if b, ok := AsBytesValue(res[0]); !ok || string(b) != "ell" {
		t.Errorf("positioned byte read = %v (ok=%v)", b, ok)
	}
	// binary read of an absent file errors.
	if err := runBoruError(t, r, []Value{
		NewWord("read"), pathV("ghost"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bytes")) }),
	}); err == nil {
		t.Error("expected error on a binary read of an absent file")
	}
}

func TestBinaryWriteAppendAndOffset(t *testing.T) {
	r, mem := ioFSReg(t)
	// append.
	runBoru(t, r, []Value{NewWord("write"), pathV("a.bin"), NewBytesValue([]byte("ab"))})
	runBoru(t, r, []Value{
		NewWord("write"), pathV("a.bin"), NewBytesValue([]byte("cd")),
		wrapMap(func(om *OrderedMap) { om.Set("mode", NewString("append")) }),
	})
	if got, _ := mem.ReadFile("a.bin"); string(got) != "abcd" {
		t.Errorf("append = %q, want abcd", got)
	}
	// positioned splice into an existing file.
	if err := mem.WriteFile("p.bin", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runBoru(t, r, []Value{
		NewWord("write"), pathV("p.bin"), NewBytesValue([]byte("XY")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(1)) }),
	})
	if got, _ := mem.ReadFile("p.bin"); string(got) != "hXYlo" {
		t.Errorf("offset write = %q, want hXYlo", got)
	}
	// positioned write past the end grows the file with zero bytes.
	runBoru(t, r, []Value{
		NewWord("write"), pathV("grow.bin"), NewBytesValue([]byte("Z")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(3)) }),
	})
	if got, _ := mem.ReadFile("grow.bin"); !bytes.Equal(got, []byte{0, 0, 0, 'Z'}) {
		t.Errorf("grow write = %v", got)
	}
	// negative offset and stream offset are rejected.
	if err := runBoruError(t, r, []Value{
		NewWord("write"), pathV("p.bin"), NewBytesValue([]byte("x")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(-1)) }),
	}); err == nil {
		t.Error("expected error on a negative offset write")
	}
	// A huge non-negative offset is rejected before the growth arithmetic —
	// otherwise it would overflow `end` and panic on existing[offset:]
	// (PR-review fix). 1<<62 is far past the positioned-write cap.
	if err := runBoruError(t, r, []Value{
		NewWord("write"), pathV("p.bin"), NewBytesValue([]byte("x")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(int64(1)<<62)) }),
	}); err == nil {
		t.Error("expected error on an oversize offset write (no overflow panic)")
	}
}

// TestBinaryStreamGuards exercises the stream-compile failure branches directly: a
// bare stream WORD (stdin/stdout) can't be forward-collected as a word
// argument, so these are driven through the handlers with stream atoms.
func TestBinaryStreamGuards(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// A binary/positioned read of a stream is declined.
	if _, _, rerr := tryBinaryRead(r, pathStdin, "bytes", NewTypeLiteral(TMap)); rerr == nil {
		t.Error("expected a binary read of a stream to error")
	}
	// A positioned (offset) write to a stream is declined.
	args := []Value{
		NewAtom("stdin"), NewBytesValue([]byte("x")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(0)) }),
	}
	if _, werr := doWriteBytesWord(args, r, true); werr == nil {
		t.Error("expected an offset write to a stream to error")
	}
}

func TestBinaryWriteToStdout(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf
	// A stdout atom routes the raw bytes to r.Output via doWrite.
	if _, err := doWriteBytesWord([]Value{NewAtom("stdout"), NewBytesValue([]byte("hi!"))}, r, false); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hi!" {
		t.Errorf("stdout bytes = %q, want hi!", buf.String())
	}
}

func TestBinaryWriteErrorBranches(t *testing.T) {
	newReg := func(f *failAtOps) *Registry {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		f.MemFileOps = capabilities.NewMem()
		SetHostFileOps(r, f)
		return r
	}
	// doWrite (non-offset) WriteFile error.
	r := newReg(&failAtOps{failWriteFile: "f.bin"})
	if _, err := doWriteBytesWord([]Value{pathV("f.bin"), NewBytesValue([]byte("x"))}, r, false); err == nil {
		t.Error("expected WriteFile error on a binary write")
	}
	// writeAtOffset WriteFile error.
	r = newReg(&failAtOps{failWriteFile: "f.bin"})
	args := []Value{
		pathV("f.bin"), NewBytesValue([]byte("x")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(0)) }),
	}
	if _, err := doWriteBytesWord(args, r, true); err == nil {
		t.Error("expected WriteFile error on a positioned write")
	}
}

func TestSliceBytes(t *testing.T) {
	data := []byte("hello")
	cases := []struct {
		off, ln       int64
		hasOff, hasLn bool
		want          string
	}{
		{0, 0, false, false, "hello"}, // whole
		{1, 3, true, true, "ell"},     // middle
		{-1, 2, true, true, "he"},     // negative offset clamps to 0
		{10, 0, true, false, ""},      // offset past end clamps
		{1, 100, true, true, "ello"},  // length past end clamps
		{2, -5, true, true, ""},       // negative length → empty
	}
	for _, c := range cases {
		if got := sliceBytes(data, c.off, c.ln, c.hasOff, c.hasLn); string(got) != c.want {
			t.Errorf("sliceBytes(%d,%d,%v,%v) = %q, want %q", c.off, c.ln, c.hasOff, c.hasLn, got, c.want)
		}
	}
}

func TestMapStrOpt(t *testing.T) {
	if _, ok := mapStrOpt(NewTypeLiteral(TMap), "mode"); ok {
		t.Error("type-literal opts should report absent")
	}
	if _, ok := mapStrOpt(NewOptionsType(NewOrderedMap()), "mode"); ok {
		t.Error("options-typed opts should report absent")
	}
	opts := wrapMap(func(om *OrderedMap) { om.Set("mode", NewString("append")) })
	if v, ok := mapStrOpt(opts, "mode"); !ok || v != "append" {
		t.Errorf("mapStrOpt = %q %v, want append true", v, ok)
	}
}
