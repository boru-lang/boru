package native

import "fmt"

// maxPositionedWriteSize caps a positioned write's resulting file size. A
// positioned splice materialises the whole file in memory (read-modify-
// write), so a huge offset is a mistake, not a real file. The cap also
// keeps offset+len below the int64-overflow and Go make-slice ceilings, so
// the growth arithmetic in writeAtOffset can never wrap negative or panic
// with "makeslice: len out of range".
const maxPositionedWriteSize = int64(1) << 40 // 1 TiB

// io_binary.go adds binary and positioned I/O to the boru:io read/write words,
// using the Scalar/Bytes type (NewBytesValue/AsBytesValue). Binary/positioned
// forms are file-oriented: read gains {enc:'bytes'} (whole-file bytes) and
// {offset}/{length} (a byte/text slice); write gains a Bytes payload plus
// {offset} (positioned splice) alongside the existing {mode:'append'}.

// mapStrOpt reads one string key from an options map, reporting presence.
func mapStrOpt(opts Value, key string) (string, bool) {
	if !opts.Parent.Equal(TMap) || !IsConcrete(&opts) {
		return "", false
	}
	m, _ := AsMap(opts)
	if m == nil {
		return "", false
	}
	return MapFieldString(m, key)
}

// isStreamPath reports whether path is one of the stdio sentinels.
func isStreamPath(path string) bool {
	return path == pathStdin || path == pathStdout || path == pathStderr
}

// doReadRaw reads a file's raw bytes with no format decoding or newline
// normalization. Binary/positioned reads are file-only; a stream errors.
func doReadRaw(r *Registry, path string) ([]byte, error) {
	if isStreamPath(path) {
		return nil, fmt.Errorf("cannot do a binary or positioned read on a stream")
	}
	return EffectiveFileOps(r).ReadFile(path)
}

// sliceBytes returns data[offset : offset+length], clamped to the data's
// bounds. A missing offset starts at 0; a missing length runs to the end.
func sliceBytes(data []byte, offset, length int64, hasOffset, hasLength bool) []byte {
	start := int64(0)
	if hasOffset {
		start = offset
	}
	if start < 0 {
		start = 0
	}
	if start > int64(len(data)) {
		start = int64(len(data))
	}
	end := int64(len(data))
	if hasLength {
		end = start + length
	}
	if end < start {
		end = start
	}
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	return data[start:end]
}

// tryBinaryRead handles read's binary ({enc:'bytes'}) and positioned
// ({offset}/{length}) forms. It returns handled=false to fall through to the
// normal format-decoding read. A binary read returns Bytes; a positioned
// text read returns the sliced String.
func tryBinaryRead(r *Registry, path, enc string, opts Value) (result []Value, handled bool, err error) {
	wantBytes := enc == "bytes" || enc == "binary"
	offset, hasOffset := mapIntOpt(opts, "offset")
	length, hasLength := mapIntOpt(opts, "length")
	if !wantBytes && !hasOffset && !hasLength {
		return nil, false, nil
	}
	data, rerr := doReadRaw(r, path)
	if rerr != nil {
		return nil, true, rerr
	}
	if hasOffset || hasLength {
		data = sliceBytes(data, offset, length, hasOffset, hasLength)
	}
	if wantBytes {
		return []Value{NewBytesValue(data)}, true, nil
	}
	return []Value{NewString(string(data))}, true, nil
}

// writeAtOffset splices data into the file at path starting at offset,
// growing the file with zero bytes as needed.
func writeAtOffset(r *Registry, path string, data []byte, offset int64) error {
	// Reject an offset whose resulting size would exceed the positioned-write
	// cap, before any arithmetic or allocation. The cap sits far below both
	// the int64-overflow point and Go's make-slice ceiling, so `end` can
	// never wrap negative (which would skip the growth branch and then panic
	// on existing[offset:]) nor blow makeslice with "len out of range".
	if offset > maxPositionedWriteSize-int64(len(data)) {
		return fmt.Errorf("offset %d + %d bytes exceeds the maximum positioned-write size", offset, len(data))
	}
	ops := EffectiveFileOps(r)
	existing, _ := ops.ReadFile(path) // absent → treated as empty
	end := offset + int64(len(data))
	if int64(len(existing)) < end {
		grown := make([]byte, end)
		copy(grown, existing)
		existing = grown
	}
	copy(existing[offset:], data)
	r.NoteEffect()
	return ops.WriteFile(path, existing, 0o644)
}

// doWriteBytesWord implements the binary write path: raw bytes with {mode}
// (append) and {offset} (positioned) options. Returns the target.
func doWriteBytesWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	path := extractPath(args[0])
	data, _ := AsBytesValue(args[1])
	mode := "write"
	atomic := false
	exclusive := false
	var offset int64
	hasOffset := false
	if hasOpts {
		if m, ok := mapStrOpt(args[2], "mode"); ok {
			mode = m
		}
		offset, hasOffset = mapIntOpt(args[2], "offset")
		atomic = mapBoolOpt(args[2], "atomic", false)
		exclusive = mapBoolOpt(args[2], "exclusive", false)
	}
	if hasOffset {
		if atomic {
			return nil, r.BoruError("write_error", "write: {atomic} cannot combine with {offset} (a positioned splice is inherently in-place)", "write")
		}
		if exclusive {
			return nil, r.BoruError("write_error", "write: {exclusive} cannot combine with {offset} (an exclusive create starts empty)", "write")
		}
		if offset < 0 {
			return nil, r.BoruError("write_error", "write: negative offset", "write")
		}
		if isStreamPath(path) {
			return nil, r.BoruError("write_error", "write: cannot use offset with a stream", "write")
		}
		if err := writeAtOffset(r, path, data, offset); err != nil {
			return nil, r.BoruError("write_error", fmt.Sprintf("write: %v", err), "write")
		}
		return []Value{returnPath(args[0])}, nil
	}
	// Plain and append writes reuse doWrite (which also handles the stdio
	// streams). nl="raw" means no newline rewriting; doWrite writes the
	// content bytes verbatim, so a Go string over raw bytes round-trips.
	if _, err := doWrite(r, path, string(data), "utf8", "text", mode, "raw", atomic, exclusive, args[0].Pos()); err != nil {
		return nil, err
	}
	return []Value{returnPath(args[0])}, nil
}

func writeBytesHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doWriteBytesWord(args, r, false)
}

func writeBytesOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doWriteBytesWord(args, r, true)
}
