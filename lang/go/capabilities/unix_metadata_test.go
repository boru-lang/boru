package capabilities

import (
	"runtime"
	"testing"
)

// skipWithoutUnixMetadata skips a test that pins POSIX file metadata on the
// REAL filesystem — permission bits, ownership, statfs, mode-enforced access,
// Unix rename errors — against the in-memory model, which emulates POSIX on
// every host. Windows has none of these: a file is read-only or not, chown and
// statfs are unsupported, and access control is ACLs rather than mode bits, so
// there the comparison measures the platform, not the code. The model's own
// POSIX behaviour stays pinned by the mem-only tests, which run everywhere.
func skipWithoutUnixMetadata(t *testing.T, what string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("Windows has no POSIX %s; this test pins the real filesystem's against the model's", what)
	}
}
