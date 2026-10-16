package native

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// io_handle.go — stateful file handles. IO.open acquires a File resource
// (a live capabilities.FileHandle behind an ExtensionPayload, the Watcher
// resource pattern); IO.seek / IO.flush / IO.close drive it; and the
// polymorphic read/write words gain File overloads (read f {length offset
// enc}; write f data {offset} → f, threading the handle). A handle indexes
// the SAME backing store as the stateless words — the mem backend shares
// its map (a handle write is visible to a stateless read before Close),
// while a mount buffers and only flushes on Sync/Close (durability caveat).

// FileHandleInfo is the payload behind a File handle: a live
// capabilities.FileHandle plus identity, closed exactly once (idempotent,
// like the Watcher's Stop).
type FileHandleInfo struct {
	ID       string
	Path     string
	h        capabilities.FileHandle
	once     sync.Once
	closeErr error

	// lines is the handle's line-read buffer, created on first IO.read-line.
	// It lives HERE rather than registry-side because a handle already has a
	// stable per-resource home: the payload is a pointer, so every boru copy of
	// the File value shares this struct, and two handles on the same path each
	// get their own cursor — which is what a caller reading two files line by
	// line expects.
	//
	// Buffering means a line read consumes ahead of the handle's cursor, so
	// mixing IO.read-line with a cursor-relative IO.read on the SAME handle
	// reads from where the buffer left off, not where the last line ended.
	// Positioned reads ({offset}) are unaffected — they use ReadAt and leave
	// the cursor alone.
	linesMu sync.Mutex
	lines   *bufio.Reader
}

// dropLineBuffer discards the handle's line-read buffer so the next line read
// starts from the handle's current position. Called by IO.seek.
func (fh *FileHandleInfo) dropLineBuffer() {
	fh.linesMu.Lock()
	defer fh.linesMu.Unlock()
	fh.lines = nil
}

// lineReader returns the handle's line-read buffer, creating it on first use.
// The caller MUST hold linesMu; use ReadLine instead unless you are it.
func (fh *FileHandleInfo) lineReaderLocked() *bufio.Reader {
	if fh.lines == nil {
		fh.lines = bufio.NewReader(fh.h)
	}
	return fh.lines
}

// ReadLine consumes one line from the handle's buffer WHILE HOLDING the
// mutex. Returning the *bufio.Reader and reading outside the lock — which is
// what this did — synchronised only the buffer's CREATION, so two forks
// sharing a handle raced inside bufio.Reader itself and could duplicate,
// interleave or lose input. A bufio.Reader is not safe for concurrent use;
// the lock has to cover the read, not the pointer fetch.
func (fh *FileHandleInfo) ReadLine() (string, bool, error) {
	fh.linesMu.Lock()
	defer fh.linesMu.Unlock()
	return readLineFrom(fh.lineReaderLocked())
}

// Close releases the handle exactly once and reports the close error.
func (fh *FileHandleInfo) Close() error {
	fh.once.Do(func() { fh.closeErr = fh.h.Close() })
	return fh.closeErr
}

// fileHandleFormatBehavior renders a File as "File(id,path)".
type fileHandleFormatBehavior struct{}

func (fileHandleFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (fileHandleFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (fileHandleFormatBehavior) Format(v Value) string {
	if fh, ok := asFileHandle(v); ok {
		return fmt.Sprintf("File(%s,%s)", fh.ID, fh.Path)
	}
	return "File(nil)"
}

// MintFileHandleType mints the module-scoped File resource type —
// per-import, like Watcher, never a global builtin. Distinct from
// FileType (the file/dir/symlink/other atom enum): this is a live
// open-file handle carrying an fd / mem cursor.
func MintFileHandleType(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("File", core.TIdeal, fileHandleFormatBehavior{})
}

// NewFileHandleType mints a standalone File type (own dynamic table) for
// test helpers that register the io words under bare names.
func NewFileHandleType() *Type {
	return core.NewDynamicTypeTable().MintTypeWithBehavior("File", core.TIdeal, fileHandleFormatBehavior{})
}

// asFileHandle unwraps a File handle's payload (two-value assertion, so a
// bare File type literal or a non-File value yields (nil, false)).
func asFileHandle(v Value) (*FileHandleInfo, bool) {
	ext, ok := v.Data().(ExtensionPayload)
	if !ok {
		return nil, false
	}
	fh, ok := ext.Body.(*FileHandleInfo)
	return fh, ok
}

// openOptsFromMap parses IO.open's options into capabilities.OpenOpts.
// {mode:read|write|append|rw} is the base (default read-only): write is
// create+truncate (fopen "w"), append is create+append ("a"), rw is
// read+write ("r+"). {create}/{exclusive}/{truncate} refine it; {perm}
// sets the creation mode.
func openOptsFromMap(opts Value) (capabilities.OpenOpts, error) {
	o := capabilities.OpenOpts{Read: true}
	if mode, ok := mapStrOpt(opts, "mode"); ok {
		switch mode {
		case "read":
			o = capabilities.OpenOpts{Read: true}
		case "write":
			o = capabilities.OpenOpts{Write: true, Create: true, Truncate: true}
		case "append":
			o = capabilities.OpenOpts{Write: true, Append: true, Create: true}
		case "rw":
			o = capabilities.OpenOpts{Read: true, Write: true}
		default:
			return o, fmt.Errorf("unknown mode %q (want read/write/append/rw)", mode)
		}
	}
	if mapBoolOpt(opts, "create", false) {
		o.Create = true
	}
	if mapBoolOpt(opts, "exclusive", false) {
		o.Create = true
		o.Exclusive = true
	}
	if mapBoolOpt(opts, "truncate", false) {
		o.Truncate = true
	}
	if perm, ok := mapIntOpt(opts, "perm"); ok {
		o.Perm = os.FileMode(perm)
	}
	return o, nil
}

// doOpenWord implements IO.open: acquire a File handle on a Pathon and
// return it as a File value.
func doOpenWord(args []Value, r *Registry, fileType *Type) ([]Value, error) {
	path := extractPath(args[0])
	opts := capabilities.OpenOpts{Read: true}
	if len(args) > 1 {
		parsed, err := openOptsFromMap(args[1])
		if err != nil {
			return nil, r.BoruError("open_error", fmt.Sprintf("open: %v", err), "open")
		}
		opts = parsed
	}
	r.NoteEffect()
	h, err := EffectiveFileOps(r).Open(path, opts)
	if err != nil {
		return nil, r.BoruError("open_error", fmt.Sprintf("open: %v", err), "open")
	}
	info := &FileHandleInfo{ID: GenerateID("F_"), Path: path, h: h}
	return []Value{core.NewValueRaw(fileType, ExtensionPayload{Body: info})}, nil
}

// doSeekWord implements IO.seek f n {from:start|current|end} → new offset.
func doSeekWord(args []Value, r *Registry) ([]Value, error) {
	fh, ok := asFileHandle(args[0])
	if !ok {
		return nil, r.BoruError("seek_error", fmt.Sprintf("seek: not a File handle (got %s)", args[0].Parent()), "seek")
	}
	n, err := args[1].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("seek_error", fmt.Sprintf("seek: %v", err), "seek")
	}
	whence := io.SeekStart
	if len(args) > 2 {
		if from, ok := mapStrOpt(args[2], "from"); ok {
			switch from {
			case "start":
				whence = io.SeekStart
			case "current":
				whence = io.SeekCurrent
			case "end":
				whence = io.SeekEnd
			default:
				return nil, r.BoruError("seek_error", fmt.Sprintf("seek: unknown {from} %q (want start/current/end)", from), "seek")
			}
		}
	}
	off, serr := fh.h.Seek(n, whence)
	if serr != nil {
		return nil, r.BoruError("seek_error", fmt.Sprintf("seek: %v", serr), "seek")
	}
	// A seek repositions the handle, so the line buffer's read-ahead is now
	// bytes from somewhere else. Dropping it is the only defensible answer: a
	// read-line after `IO.seek f 0` must re-read from the start, not continue
	// serving lines buffered from before the seek.
	fh.dropLineBuffer()
	return []Value{NewInteger(off)}, nil
}

// doFlushWord implements IO.flush → the argument: sync a File handle to
// its backend, or flush a writable Mmap region's changes (polymorphic,
// like IO.close).
func doFlushWord(args []Value, r *Registry) ([]Value, error) {
	if fh, ok := asFileHandle(args[0]); ok {
		if err := fh.h.Sync(); err != nil {
			return nil, r.BoruError("flush_error", fmt.Sprintf("flush: %v", err), "flush")
		}
		return []Value{args[0]}, nil
	}
	if mi, ok := asMmapInfo(args[0]); ok {
		if err := mi.flush(); err != nil {
			return nil, r.BoruError("flush_error", fmt.Sprintf("flush: %v", err), "flush")
		}
		return []Value{args[0]}, nil
	}
	return nil, r.BoruError("flush_error", fmt.Sprintf("flush: not a File or Mmap handle (got %s)", args[0].Parent()), "flush")
}

// readHandleWord reads from a File handle: {offset} positions the read
// (ReadAt, cursor unchanged), {length} bounds it (else to EOF), {enc}
// decodes ('bytes'/'binary' returns Bytes, otherwise text).
func readHandleWord(args []Value, r *Registry) ([]Value, error) {
	fh, ok := asFileHandle(args[0])
	if !ok {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: not a File handle (got %s)", args[0].Parent()), "read")
	}
	enc := "utf8"
	offset, length := int64(-1), int64(-1)
	if len(args) > 1 {
		if e, ok := mapStrOpt(args[1], "enc"); ok {
			enc = e
		}
		if o, ok := mapIntOpt(args[1], "offset"); ok {
			offset = o
		}
		if l, ok := mapIntOpt(args[1], "length"); ok {
			length = l
		}
	}
	data, err := readFromHandle(fh.h, offset, length)
	if err != nil {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: %v", err), "read")
	}
	if enc == "bytes" || enc == "binary" {
		return []Value{NewBytesValue(data)}, nil
	}
	text, derr := decodeEnc(data, enc)
	if derr != nil {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: %v", derr), "read")
	}
	return []Value{NewString(text)}, nil
}

// writeHandleWord writes to a File handle and returns it (threading):
// {offset} positions the write (WriteAt, cursor unchanged), else at the
// cursor. A Bytes payload writes verbatim; a String writes its UTF-8.
func writeHandleWord(args []Value, r *Registry) ([]Value, error) {
	fh, ok := asFileHandle(args[0])
	if !ok {
		return nil, r.BoruError("write_error", fmt.Sprintf("write: not a File handle (got %s)", args[0].Parent()), "write")
	}
	data, err := handleWriteBytes(args[1])
	if err != nil {
		return nil, r.BoruError("write_error", fmt.Sprintf("write: %v", err), "write")
	}
	offset := int64(-1)
	if len(args) > 2 {
		if o, ok := mapIntOpt(args[2], "offset"); ok {
			offset = o
		}
	}
	r.NoteEffect()
	if offset >= 0 {
		_, err = fh.h.WriteAt(data, offset)
	} else {
		_, err = fh.h.Write(data)
	}
	if err != nil {
		return nil, r.BoruError("write_error", fmt.Sprintf("write: %v", err), "write")
	}
	return []Value{args[0]}, nil
}

// handleWriteBytes extracts the bytes to write from a String or Bytes
// value (a Bytes payload writes verbatim, a String as UTF-8).
func handleWriteBytes(v Value) ([]byte, error) {
	if b, ok := AsBytesValue(v); ok {
		return b, nil
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return nil, fmt.Errorf("a File write needs a String or Bytes payload (got %s)", v.Parent())
	}
	return []byte(s), nil
}

// readFromHandle reads bytes from a handle per the offset/length policy:
// positioned (offset>=0, ReadAt) vs sequential (Read), bounded (length>=0)
// vs to-EOF.
func readFromHandle(h capabilities.FileHandle, offset, length int64) ([]byte, error) {
	positioned := offset >= 0
	if length >= 0 {
		buf := make([]byte, length)
		var n int
		var err error
		if positioned {
			n, err = h.ReadAt(buf, offset)
		} else {
			n, err = readSeqFull(h, buf)
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		return buf[:n], nil
	}
	if positioned {
		return readAtToEnd(h, offset)
	}
	return io.ReadAll(h)
}

// readSeqFull reads up to len(buf) bytes sequentially, stopping at EOF.
func readSeqFull(h capabilities.FileHandle, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := h.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// readAtToEnd reads positioned from offset to EOF in chunks.
func readAtToEnd(h capabilities.FileHandle, offset int64) ([]byte, error) {
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := h.ReadAt(buf, offset)
		out = append(out, buf[:n]...)
		offset += int64(n)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}
