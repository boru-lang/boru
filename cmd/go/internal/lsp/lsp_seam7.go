// Test seams (design/TEST-SEAMS.10.md). Each var defaults to the real
// implementation; tests swap it (with a t.Cleanup restore) to drive an
// otherwise-unreachable init/marshal/transport failure arm.

package lsp

import (
	"encoding/json"
	"net"

	"github.com/boru-lang/boru/cmd/go/internal/permsflags"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

var (
	// langNew seams lang.New for computeDiagnostics's init-failure arm.
	langNew = lang.New
	// envPolicy seams permsflags.EnvPolicy for the policy-resolution arms
	// (NUR079): the environment policy an editor session inherits.
	envPolicy = permsflags.EnvPolicy

	// nativeDefaultRegistry seams native.DefaultRegistry for
	// ensureRegistry's init-failure arm.
	nativeDefaultRegistry = native.DefaultRegistry

	// jsonMarshal seams json.Marshal for notify's outer-envelope encode
	// step. Unlike respond/respondError (whose id argument is caller-
	// supplied and can carry invalid raw JSON directly), notify's
	// rawMessage only ever holds an already-validated Params value, so
	// only a seam can observe this error arm.
	jsonMarshal = json.Marshal

	// netListen seams net.Listen for the TCP Start path so tests can
	// substitute a fake listener whose Accept returns an arbitrary
	// (non-ErrClosed, non-ctx-canceled) error.
	netListen = net.Listen
)
