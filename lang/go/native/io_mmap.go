package native

import (
	"errors"
	"fmt"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// io_mmap.go — memory-mapped files. IO.mmap maps a file's bytes into a
// Mmap resource; `read region {offset length enc}` COPIES bytes out (a
// read must never hand boru a slice that a later close unmaps — a
// use-after-free), `write region bytes {offset}` splices INTO the mapping
// in place, IO.flush syncs, and the polymorphic IO.close unmaps.

// errMmapClosed is returned by any region operation attempted after the
// handle was closed — the mapping is unmapped, so touching its bytes would
// be a use-after-free (a SIGSEGV on the OS backend). All region access is
// serialised through mi.mu so a concurrent close (e.g. from an IO.watch
// callback fork) can never unmap the bytes mid-copy.
var errMmapClosed = errors.New("this Mmap region is closed")

// errMmapNoFit is returned when a positioned write would exceed the mapping
// (an mmap cannot grow).
var errMmapNoFit = errors.New("does not fit the mapped region (an mmap cannot grow)")

// MmapInfo is the payload behind a Mmap handle: a live mapped region whose
// access is guarded by mu and closed exactly once.
type MmapInfo struct {
	ID       string
	Path     string
	Writable bool
	mu       sync.Mutex
	region   capabilities.MmapRegion
	closed   bool
	closeErr error
}

// Close unmaps the region exactly once (idempotent) and reports the release
// error, if any.
func (mi *MmapInfo) Close() error {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	if mi.closed {
		return mi.closeErr
	}
	mi.closed = true
	mi.closeErr = mi.region.Close()
	return mi.closeErr
}

// read copies [offset, offset+length) out of the mapping under the lock,
// refusing a closed region. The COPY happens while mu is held so a
// concurrent Close cannot unmap the bytes mid-copy.
func (mi *MmapInfo) read(offset, length int) ([]byte, error) {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	if mi.closed {
		return nil, errMmapClosed
	}
	buf := mi.region.Bytes()
	lo, hi, err := regionSlice(offset, length, len(buf))
	if err != nil {
		return nil, err
	}
	out := make([]byte, hi-lo) // COPY out of the mapping (munmap-safe)
	copy(out, buf[lo:hi])
	return out, nil
}

// write splices data into the mapping at offset under the lock, refusing a
// closed region and a window that does not fit.
func (mi *MmapInfo) write(data []byte, offset int) error {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	if mi.closed {
		return errMmapClosed
	}
	buf := mi.region.Bytes()
	if offset < 0 || offset+len(data) > len(buf) {
		return errMmapNoFit
	}
	copy(buf[offset:], data)
	return nil
}

// flush syncs the region under the lock, refusing a closed region.
func (mi *MmapInfo) flush() error {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	if mi.closed {
		return errMmapClosed
	}
	return mi.region.Flush()
}

// mmapFormatBehavior renders a Mmap as "Mmap(id,path)".
type mmapFormatBehavior struct{}

func (mmapFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (mmapFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (mmapFormatBehavior) Format(v Value) string {
	if mi, ok := asMmapInfo(v); ok {
		return fmt.Sprintf("Mmap(%s,%s)", mi.ID, mi.Path)
	}
	return "Mmap(nil)"
}

// MintMmapType / NewMmapType mint the module-scoped Mmap resource type.
func MintMmapType(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("Mmap", core.TIdeal, mmapFormatBehavior{})
}
func NewMmapType() *Type {
	return core.NewDynamicTypeTable().MintTypeWithBehavior("Mmap", core.TIdeal, mmapFormatBehavior{})
}

// asMmapInfo unwraps a Mmap handle's payload.
func asMmapInfo(v Value) (*MmapInfo, bool) {
	ext, ok := DataOf(&v).(ExtensionPayload)
	if !ok {
		return nil, false
	}
	mi, ok := ext.Body.(*MmapInfo)
	return mi, ok
}

// doMmapWord implements IO.mmap p {offset length writable}: map a file
// region. writable permits in-place writes flushed on flush/close.
func doMmapWord(args []Value, r *Registry, mmapType *Type) ([]Value, error) {
	path := extractPath(args[0])
	var offset int64
	length := 0
	writable := false
	if len(args) > 1 {
		if o, ok := mapIntOpt(args[1], "offset"); ok {
			offset = o
		}
		if l, ok := mapIntOpt(args[1], "length"); ok {
			length = int(l)
		}
		writable = mapBoolOpt(args[1], "writable", false)
	}
	r.NoteEffect()
	region, err := EffectiveFileOps(r).Mmap(path, offset, length, writable)
	if err != nil {
		return nil, r.BoruError("mmap_error", fmt.Sprintf("mmap: %v", err), "mmap")
	}
	info := &MmapInfo{ID: GenerateID("M_"), Path: path, Writable: writable, region: region}
	return []Value{core.NewValueRaw(mmapType, ExtensionPayload{Body: info})}, nil
}

// readMmapWord reads from a mapped region: {offset}/{length} window it,
// {enc} decodes ('bytes' returns Bytes, else text). The bytes are COPIED
// out of the mapping so a later close (munmap) can never invalidate the
// boru value.
func readMmapWord(args []Value, r *Registry) ([]Value, error) {
	mi, ok := asMmapInfo(args[0])
	if !ok {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: not a Mmap region (got %s)", ParentOf(&args[0])), "read")
	}
	enc := "utf8"
	offset, length := 0, -1
	if len(args) > 1 {
		if e, ok := mapStrOpt(args[1], "enc"); ok {
			enc = e
		}
		if o, ok := mapIntOpt(args[1], "offset"); ok {
			offset = int(o)
		}
		if l, ok := mapIntOpt(args[1], "length"); ok {
			length = int(l)
		}
	}
	out, werr := mi.read(offset, length)
	if werr != nil {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: %v", werr), "read")
	}
	if enc == "bytes" || enc == "binary" {
		return []Value{NewBytesValue(out)}, nil
	}
	text, derr := decodeEnc(out, enc)
	if derr != nil {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: %v", derr), "read")
	}
	return []Value{NewString(text)}, nil
}

// writeMmapWord splices bytes INTO the mapping at {offset} (default 0),
// in place — an mmap cannot grow, so the write must fit the region. It
// returns the region for threading. A read-only region refuses.
func writeMmapWord(args []Value, r *Registry) ([]Value, error) {
	mi, ok := asMmapInfo(args[0])
	if !ok {
		return nil, r.BoruError("write_error", fmt.Sprintf("write: not a Mmap region (got %s)", ParentOf(&args[0])), "write")
	}
	if !mi.Writable {
		return nil, r.BoruError("write_error", "write: this Mmap region is read-only", "write")
	}
	data, err := handleWriteBytes(args[1])
	if err != nil {
		return nil, r.BoruError("write_error", fmt.Sprintf("write: %v", err), "write")
	}
	offset := 0
	if len(args) > 2 {
		if o, ok := mapIntOpt(args[2], "offset"); ok {
			offset = int(o)
		}
	}
	r.NoteEffect()
	if err := mi.write(data, offset); err != nil {
		return nil, r.BoruError("write_error", fmt.Sprintf("write: %v", err), "write")
	}
	return []Value{args[0]}, nil
}

// regionSlice validates an {offset,length} window against a region of
// size n (length<0 means "to end").
func regionSlice(offset, length, n int) (lo, hi int, err error) {
	if offset < 0 || offset > n {
		return 0, 0, fmt.Errorf("offset %d out of range [0,%d]", offset, n)
	}
	hi = n
	if length >= 0 {
		hi = offset + length
	}
	if hi > n {
		return 0, 0, fmt.Errorf("window [%d,%d) exceeds region size %d", offset, hi, n)
	}
	return offset, hi, nil
}
