package capabilities

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Map the whole file; locks.go exposes the requested window and keeps
// the file open until Flush and Close have finished with the view.
func osMmapFile(f *os.File, length int, writable bool) ([]byte, error) {
	protect, access := uint32(windows.PAGE_READONLY), uint32(windows.FILE_MAP_READ)
	if writable {
		protect, access = windows.PAGE_READWRITE, windows.FILE_MAP_WRITE
	}
	mapping, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, protect, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(mapping) }()
	addr, err := windows.MapViewOfFile(mapping, access, 0, 0, uintptr(length))
	if err != nil {
		return nil, err
	}
	return mappedBytes(addr, length), nil
}

// MapViewOfFile returns an address outside the Go heap; its lifetime ends
// at UnmapViewOfFile. Read its pointer representation without uintptr
// arithmetic or constructing a standalone reflect.SliceHeader.
func mappedBytes(addr uintptr, length int) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&addr)), length)
}

func osMunmap(b []byte) error {
	return windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&b[0])))
}

func osMsync(b []byte) error {
	return windows.FlushViewOfFile(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
}
