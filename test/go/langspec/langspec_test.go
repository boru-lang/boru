// Spec-runner test for the production-language spec suite at
// boru/lang/spec/. Each TSV row is parsed with the boru parser
// (eng/go/parser) and run against a fresh production registry
// (native.DefaultRegistry + native.Register) — the full language
// layer, so these specs can exercise any registered word (record /
// object / make / get / size / …) and the builtin Resource / Entity
// types installed by installResourceTypes.
//
// The kernel-only spec suite (q-suffixed fixtures, eng.RegisterCoreWords,
// specs at eng/spec/) lives next door at test/go/engspec — it tests the
// engine kernel in isolation. The shared TSV scaffolding lives in
// test/go/specfix.
//
// Both spec tests live in the test module so neither eng nor lang has a
// dep on test — the dep arrows point one way: test → eng, test → lang.
package langspec

import (
	"path/filepath"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// specClock freezes time at a fixed instant so temporal words (`now`,
// boru:time) and the default boru:rand seed are deterministic in specs.
var specClock = capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}

// TestSpecProd runs the .tsv spec files under boru/lang/spec/ against a
// production-boru registry (native.DefaultRegistry + native.Register).
// These specs cover the production language layer — words and types
// that aren't part of the eng kernel (record, object, make, get/set on
// Stores, Resource / Entity, …). They sit at lang/spec/ to mirror the
// engine kernel's eng/spec/ layout.
func TestSpecProd(t *testing.T) {
	t.Parallel()
	runSpecProd(t, false)
}

// TestSpecProdTCODisabled re-runs the entire production spec suite
// with tail-call elision switched off (Registry.TCO.Disable). Every
// row pins an exact output, so green in BOTH modes is the dual-mode
// differential gate of design/legacy/TCO-STAGED.10.ignore: elision must be
// observationally invisible row-for-row.
func TestSpecProdTCODisabled(t *testing.T) {
	t.Parallel()
	runSpecProd(t, true)
}

func runSpecProd(t *testing.T, tcoDisabled bool) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	specfix.RunDir(t, specDir, func(input string) ([]core.Value, error) {
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		reg, err := native.DefaultRegistry()
		if err != nil {
			return nil, err
		}
		reg.TCO.Disable = tcoDisabled
		// Install the shared q-suffixed spec fixtures so tsv files
		// originally written for engspec (object, record, inspect, …)
		// can run under the production setup too.
		specfix.RegisterQFixtures(reg)
		// Wire the parser so the boru-implemented modules (report, test)
		// can parse their source on import — exactly what lang.New() does
		// in production. Without this `import "boru:report"` fails with
		// "parser not configured".
		reg.SetParseFunc(parser.Parse)
		// Install the loadable-module resolver so specs can `import
		// "boru:math-util"` etc. — matching what lang.New() wires up in
		// production. Without this the module words are unreachable and
		// the formal spec could not cover them.
		modules.InstallResolver(reg)
		// Freeze the clock so temporal / rand specs are reproducible.
		native.SetHostClock(reg, specClock)
		return native.NewTop(reg).Run(values)
	})
}
