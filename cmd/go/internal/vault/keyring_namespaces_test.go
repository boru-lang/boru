package vault

import (
	"errors"
	"fmt"
	"reflect"
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
