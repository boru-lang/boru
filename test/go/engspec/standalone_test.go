package engspec

// The core-only lanes: the registry the kernel battery and the fixture
// probes run on — the specfix vocabulary with a standalone Micron ideal
// (Pathon only; the twelve leaf validators are basic's content) and no
// language layer — and the eng/spec corpus and check corpus run on it
// (TestSpecStandalone, TestCheckSpecStandalone), skipping the rows that need
// basic's Micron content. They are the twins of TestSpec and TestCheckSpec,
// which install that content, and the corpus-level proof that the kernel
// needs nothing above it. They moved here from eng/go with the battery
// (design/COMPILER-LANE-REMOVAL.0.md).

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// installStandaloneMicronIdeal registers the mechanism-only stand-in:
// the same Accepts shape the content layer's descriptor has, with
// Instantiate wired to the kernel's own Pathon constructor. A
// non-Pathon kind reaching it is a harness bug — the skip predicate
// keeps those rows out.
func installStandaloneMicronIdeal(r *core.Registry) {
	r.Ideals.Register(&core.Ideal{
		Name:    "Micron",
		Enabled: true,
		Accepts: func(v core.Value) bool {
			return core.IsMicronType(v) || (core.IsBareTypeNode(&v) && (&v).ConformsTo(core.TMicron))
		},
		Instantiate: func(typ, data core.Value, _ *core.Registry) ([]core.Value, error) {
			if (&typ).Equal(core.TPathon) {
				return core.MakePathon(data, false)
			}
			return nil, fmt.Errorf("standalone corpus: %s needs the content layer", typ.String())
		},
	})
}

func standaloneRegistry() (*core.Registry, error) {
	r, err := core.NewRegistry()
	if err != nil {
		return nil, err
	}
	specfix.RegisterSpecWords(r)
	installStandaloneMicronIdeal(r)
	r.InitRootContext()
	return r, nil
}

// standaloneContentRow reports whether a corpus row needs the content
// layer's validators (the Emailon / Urlon constructions).
func standaloneContentRow(input string) bool {
	return strings.Contains(input, "make Emailon") || strings.Contains(input, "make Urlon")
}

// TestSpecStandalone is the value-mode corpus, kernel-only.
func TestSpecStandalone(t *testing.T) {
	specfix.RunDir(t, filepath.Join("..", "..", "..", "eng", "spec"), func(input string) ([]core.Value, error) {
		if standaloneContentRow(input) {
			return nil, specfix.ErrSkipRow
		}
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		r, err := standaloneRegistry()
		if err != nil {
			return nil, err
		}
		return core.NewTop(r).Run(values)
	})
}

// TestCheckSpecStandalone is the check-mode corpus, kernel-only.
func TestCheckSpecStandalone(t *testing.T) {
	specfix.RunDirRendered(t, filepath.Join("..", "..", "..", "eng", "spec", "check"), func(input string) (string, error) {
		if standaloneContentRow(input) {
			return "", specfix.ErrSkipRow
		}
		values, err := parser.Parse(input)
		if err != nil {
			return "", err
		}
		r, err := standaloneRegistry()
		if err != nil {
			return "", err
		}
		specfix.RegisterCheckExtras(r)
		r.Source = input

		done := r.Check.Begin()
		out, runErr := core.NewTop(r).Run(values)
		r.RescueForwardRefDiagnostics()
		r.Check.EmitUnusedDefDiagnostics()
		diags := r.Check.Diagnostics
		done()

		if runErr != nil {
			return "", runErr
		}
		return specfix.RenderCheck(out, diags), nil
	})
}
