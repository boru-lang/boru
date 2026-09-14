package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/wire"
)

func TestFirstFoundNamespaces(t *testing.T) {
	services := wire.KeychainServices()
	broken := errors.New("keychain locked")
	for _, tc := range []struct {
		name    string
		results []error
		want    string
		err     error
	}{
		{"current wins", []error{nil}, "current", nil},
		{"legacy fallback", []error{fmt.Errorf("missing: %w", ErrNotFound), nil}, "legacy", nil},
		{"all missing", []error{ErrNotFound, ErrNotFound, ErrNotFound}, "", ErrNotFound},
		{"failure stops search", []error{broken}, "", broken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var visited []string
			got, err := firstFound(services, func(service string) (string, error) {
				index := len(visited)
				visited = append(visited, service)
				if index >= len(tc.results) {
					t.Fatalf("unexpected lookup in %s", service)
				}
				return tc.want, tc.results[index]
			})
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Errorf("lookup = %q, %v; want %q, %v", got, err, tc.want, tc.err)
			}
			if !reflect.DeepEqual(visited, services[:len(tc.results)]) {
				t.Errorf("lookup order = %v", visited)
			}
		})
	}
}

func TestSecretServiceDeleteLegacyNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       string
		diagnostic string
		wantError  bool
	}{
		{"missing legacy", "1", "", false},
		{"backend failure", "1", "keychain locked", true},
		{"other exit", "2", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			t.Setenv("BORU_STUB_DELETE_LOG", log)
			t.Setenv("BORU_STUB_DELETE_CODE", tc.code)
			t.Setenv("BORU_STUB_DELETE_ERROR", tc.diagnostic)
			stubBin(t, dir, "secret-tool", `
[ "$1" = clear ] || exit 3
printf '%s\n' "$3" >> "$BORU_STUB_DELETE_LOG"
[ "$3" = boru ] && exit 0
[ -z "$BORU_STUB_DELETE_ERROR" ] || printf '%s\n' "$BORU_STUB_DELETE_ERROR" >&2
exit "$BORU_STUB_DELETE_CODE"
`)
			prependPath(t, dir)
			err := (&secretService{}).Delete("alias")
			if (err != nil) != tc.wantError {
				t.Errorf("Delete = %v; want error %v", err, tc.wantError)
			}
			if tc.diagnostic != "" && (err == nil || !strings.Contains(err.Error(), tc.diagnostic)) {
				t.Errorf("Delete lost diagnostic: %v", err)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(calls), strings.Join(wire.KeychainServices(), "\n")+"\n"; got != want {
				t.Errorf("delete namespaces = %q, want %q", got, want)
			}
		})
	}
}

func TestSecretServiceSharesShippedNamespace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BORU_STUB_CREDENTIALS", dir)
	stubBin(t, dir, "secret-tool", `
op="$1"
shift
while [ "$#" -gt 1 ]; do
  if [ "$1" = service ]; then service="$2"; break; fi
  shift
done
case "$op" in
  store) cat > "$BORU_STUB_CREDENTIALS/$service";;
  lookup) [ -f "$BORU_STUB_CREDENTIALS/$service" ] || exit 1
          cat "$BORU_STUB_CREDENTIALS/$service";;
  *) exit 3;;
esac
`)
	prependPath(t, dir)
	oldPath := filepath.Join(dir, "boru")
	developmentPath := filepath.Join(dir, "vlt.secrets")
	if err := os.WriteFile(oldPath, []byte("old-binary-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(developmentPath, []byte("stale-development-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	k := &secretService{}
	if got, err := k.Get("alias"); err != nil || got != "old-binary-value" {
		t.Fatalf("current reader shadowed shipped credentials: %q, %v", got, err)
	}
	if err := k.Set("alias", "new-binary-value"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(oldPath); err != nil || string(got) != "new-binary-value" {
		t.Fatalf("older reader cannot see new write: %q, %v", got, err)
	}
	if got, err := os.ReadFile(developmentPath); err != nil || string(got) != "stale-development-value" {
		t.Fatalf("write migrated to development namespace: %q, %v", got, err)
	}
	if err := os.WriteFile(oldPath, []byte("later-old-binary-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Get("alias"); err != nil || got != "later-old-binary-value" {
		t.Fatalf("current reader missed older binary's update: %q, %v", got, err)
	}
}

func TestDeleteEachSweepsNamespacesAfterFailure(t *testing.T) {
	services := wire.KeychainServices()
	first := errors.New("first backend failure")
	second := errors.New("second backend failure")
	for _, fail := range []bool{false, true} {
		var visited []string
		err := deleteEach(services, func(service string) error {
			visited = append(visited, service)
			if fail {
				if len(visited) == 1 {
					return first
				}
				return second
			}
			return nil
		})
		if !reflect.DeepEqual(visited, services) {
			t.Errorf("delete order = %v, want %v", visited, services)
		}
		if fail && !errors.Is(err, first) || !fail && err != nil {
			t.Errorf("delete with failure=%v returned %v", fail, err)
		}
	}
}
