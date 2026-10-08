package capabilities

import (
	"io"
	"os"
	"sync"
)

// locks.go — advisory file locks (FileOps.Lock) and memory-mapped files
// (FileOps.Mmap) for the OS and mem backends. The OS backend uses flock /
// mmap behind the platform seams (lock_*.go, mmap_*.go); the mem backend
// models locks with a canonical-keyed reader/writer table + a sync.Cond,
// and mmap as a COPY of the file's bytes flushed back on Flush/Close (the
// portable contract: no live sharing — Flush is what makes writes
// durable). A crashed OS process auto-releases its flock; the mem model
// is single-process and STRICTER (a same-process double exclusive lock
// blocks/refuses where flock would re-grant to the same fd).

// --- OSFileOps advisory locks --------------------------------------------

// Lock opens path read-only and takes an advisory flock on it. The file
// must exist. A non-blocking call on a held lock returns ErrLockBusy.
func (o *OSFileOps) Lock(path string, shared, block bool) (io.Closer, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(resolved, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	if lerr := osLockFile(f, shared, block); lerr != nil {
		_ = f.Close()
		return nil, lerr
	}
	return &osLock{f: f}, nil
}

// osLock releases the flock and closes the fd exactly once.
type osLock struct {
	f    *os.File
	once sync.Once
	err  error
}

func (l *osLock) Close() error {
	l.once.Do(func() {
		l.err = osUnlockFile(l.f)
		if cerr := l.f.Close(); l.err == nil {
			l.err = cerr
		}
	})
	return l.err
}

// --- OSFileOps memory-mapped files ---------------------------------------

// errMmapEmpty is returned when mapping a zero-length file (mmap(2)
// rejects a zero length).
var errMmapEmpty = os.ErrInvalid

// osFileStat is the seam for Mmap's size probe (runner-independent branch
// coverage — a fresh fd's Stat effectively never fails otherwise).
var osFileStat = (*os.File).Stat

// Mmap maps path into memory. The whole file is mapped and the caller's
// {offset,length} windows it (Bytes returns the window), so no page
// alignment is required. writable requires write access to the file.
func (o *OSFileOps) Mmap(path string, offset int64, length int, writable bool) (MmapRegion, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	flag := os.O_RDONLY
	if writable {
		flag = os.O_RDWR
	}
	f, err := os.OpenFile(resolved, flag, 0)
	if err != nil {
		return nil, err
	}
	fi, err := osFileStat(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	size := fi.Size()
	lo, hi, werr := mmapWindow(offset, length, size)
	if werr != nil {
		_ = f.Close()
		return nil, &os.PathError{Op: "mmap", Path: path, Err: werr}
	}
	full, merr := osMmapFile(f, int(size), writable)
	if merr != nil {
		_ = f.Close()
		return nil, &os.PathError{Op: "mmap", Path: path, Err: merr}
	}
	return &osMmapRegion{f: f, full: full, view: full[lo:hi], writable: writable}, nil
}

// mmapWindow validates and resolves an {offset,length} window against a
// file of the given size: length<=0 means "to end". A zero-size file
// cannot be mapped.
func mmapWindow(offset int64, length int, size int64) (lo, hi int64, err error) {
	if size == 0 {
		return 0, 0, errMmapEmpty
	}
	if offset < 0 || offset > size {
		return 0, 0, os.ErrInvalid
	}
	hi = size
	if length > 0 {
		hi = offset + int64(length)
	}
	if hi > size {
		return 0, 0, os.ErrInvalid
	}
	return offset, hi, nil
}

// osMmapRegion is a live mapping; Bytes is the requested window, Flush
// msyncs, Close msyncs (if writable) then munmaps and closes the fd.
type osMmapRegion struct {
	f        *os.File
	full     []byte
	view     []byte
	writable bool
	once     sync.Once
	err      error
}

func (r *osMmapRegion) Bytes() []byte { return r.view }

func (r *osMmapRegion) Flush() error {
	if !r.writable {
		return nil
	}
	return osMsync(r.f, r.full)
}

func (r *osMmapRegion) Close() error {
	r.once.Do(func() {
		if r.writable {
			r.err = osMsync(r.f, r.full)
		}
		if e := osMunmap(r.full); r.err == nil {
			r.err = e
		}
		if e := r.f.Close(); r.err == nil {
			r.err = e
		}
	})
	return r.err
}

// --- MemFileOps advisory locks -------------------------------------------

// memLockState is one file's advisory-lock state: a reader count and an
// exclusive-writer flag, keyed by canonical path.
type memLockState struct {
	readers int
	writer  bool
}

// Lock takes an advisory lock on a mem path. It waits (block=true) via
// lockCond or returns ErrLockBusy (block=false) on contention. The path
// must exist. The mem model is single-process: a blocking lock is only
// released by another goroutine's Close.
func (m *MemFileOps) Lock(path string, shared, block bool) (io.Closer, error) {
	m.mu.Lock()
	final, err := m.resolve(path, true)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if _, ok := m.infoFor(final); !ok {
		m.mu.Unlock()
		return nil, &os.PathError{Op: "lock", Path: path, Err: os.ErrNotExist}
	}
	c := m.canon(final)
	m.mu.Unlock()

	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	st := m.fileLocks[c]
	if st == nil {
		st = &memLockState{}
		m.fileLocks[c] = st
	}
	for {
		if shared {
			if !st.writer {
				st.readers++
				return &memLock{m: m, key: c, shared: true}, nil
			}
		} else if !st.writer && st.readers == 0 {
			st.writer = true
			return &memLock{m: m, key: c, shared: false}, nil
		}
		if !block {
			return nil, ErrLockBusy
		}
		m.lockCond.Wait()
	}
}

// memLock releases its hold once, waking any blocked waiters.
type memLock struct {
	m      *MemFileOps
	key    string
	shared bool
	once   sync.Once
}

func (l *memLock) Close() error {
	l.once.Do(func() {
		l.m.lockMu.Lock()
		defer l.m.lockMu.Unlock()
		if st := l.m.fileLocks[l.key]; st != nil {
			if l.shared {
				st.readers--
			} else {
				st.writer = false
			}
		}
		l.m.lockCond.Broadcast()
	})
	return nil
}

// --- MemFileOps memory-mapped files --------------------------------------

// Mmap returns a COPY of path's [offset,offset+length) bytes; a writable
// region splices the copy back into the file on Flush/Close. The path
// must be an existing file.
func (m *MemFileOps) Mmap(path string, offset int64, length int, writable bool) (MmapRegion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	if m.Dirs[final] {
		return nil, &os.PathError{Op: "mmap", Path: path, Err: errMemIsDir}
	}
	c := m.canon(final)
	data, ok := m.Files[c]
	if !ok {
		return nil, &os.PathError{Op: "mmap", Path: path, Err: os.ErrNotExist}
	}
	lo, hi, werr := mmapWindow(offset, length, int64(len(data)))
	if werr != nil {
		return nil, &os.PathError{Op: "mmap", Path: path, Err: werr}
	}
	view := make([]byte, hi-lo)
	copy(view, data[lo:hi])
	return &memMmap{m: m, key: c, final: final, offset: lo, view: view, writable: writable}, nil
}

// memMmap is a copied view; Flush/Close splice it back into the file.
type memMmap struct {
	m        *MemFileOps
	key      string
	final    string
	offset   int64
	view     []byte
	writable bool
	once     sync.Once
}

func (r *memMmap) Bytes() []byte { return r.view }

// writeBack splices the view into the file at its offset (growing if the
// file shrank under it). Caller holds m.mu.
func (r *memMmap) writeBack() {
	r.m.Files[r.key] = writeInto(r.m.Files[r.key], r.view, r.offset)
	if meta, ok := r.m.Meta[r.key]; ok {
		meta.MTime = r.m.now()
	}
	r.m.emit("write", r.final)
}

func (r *memMmap) Flush() error {
	if !r.writable {
		return nil
	}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.writeBack()
	return nil
}

func (r *memMmap) Close() error {
	r.once.Do(func() {
		if r.writable {
			r.m.mu.Lock()
			r.writeBack()
			r.m.mu.Unlock()
		}
	})
	return nil
}
