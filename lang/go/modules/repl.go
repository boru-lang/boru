package modules

import (
	"fmt"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// BuildReplModule creates the "boru:repl" native module — a socket REPL
// server and client whose implementation is WRITTEN IN boru (the
// replBoruPreamble below), following the boru:test hybrid pattern. It is
// the first verification app of the networking stack
// (design/legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore §1.5): a service over the
// `lines` codec whose handler evaluates each received line and replies
// with the rendered result.
//
//	import "boru:repl"
//	def ln (Repl.serve {port: 4004})            # server: evaluate lines
//	def ep (Repl.connect "127.0.0.1:4004")      # client: an Endpoint
//	Repl.eval ep "def x 21 x mul 2"             # → "42"
//
// Evaluation uses Vm.run (a fresh sandboxed sub-engine per line), so
// the session replays its accumulated history each line to give the
// illusion of persistent definitions — `def x 5` on line 1 is visible
// to `x` on line 2. `Repl.eval ep "/reset"` clears the history. This
// is a deliberately basic REPL: one shared session per server, pure
// sandboxed evaluation (no I/O or network inside evaluated code).
func BuildReplModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("repl: parser not configured")
	}
	replParseOnce.Do(func() {
		replParsed, replParseErr = parent.ParseFunc(replBoruPreamble)
	})
	if replParseErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("repl: parse preamble: %w", replParseErr)
	}

	modReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, fmt.Errorf("repl: init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	// Ledger rides with the writers so this sub-registry's effects count
	// against the parent's compiled-mode fallback fence (eng effects.go);
	// the observability hooks ride along for the same reason.
	modReg.Effects = parent.Effects
	modReg.InheritObserveHooks(parent)
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	modReg.Modules.InheritConfig(parent.Modules)
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	} else {
		native.Register(modReg)
	}

	// Collect `export "Repl" {…}` from the preamble (the boru:test-style
	// local exporter; see modules/test.go for why RunModuleBody cannot
	// be reused directly).
	exports := map[string]*native.OrderedMap{}
	modReg.Defs.Delete("export")
	modReg.RegisterNativeFunc(native.NativeFunc{
		Name: "export",
		Signatures: []native.Signature{
			// The preamble's exporter is always the String form
			// (`export "Repl" {…}`); no Atom overload is needed.
			{
				Args: []*native.Type{native.TString, native.TMap},
				Impl: native.Go(func(eargs []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := eargs[0].AsConcreteString()
					return resolveExport(modReg, exports, name, eargs[1])
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			},
		},
	})

	tokens := append([]native.Value(nil), replParsed...)
	sub := native.New(modReg)
	// C4 attribution: the preamble run IS the module load (see boru:test).
	restoreAtt := modReg.SetInterpAttribution("module-load")
	_, runErr := sub.Run(tokens)
	restoreAtt()
	if runErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("repl: run preamble: %w", runErr)
	}

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: exports,
	}, nil
}

var (
	replParseOnce sync.Once
	replParsed    []native.Value
	replParseErr  error
)

// replBoruPreamble is the module's implementation — plain boru over the
// boru:net codec tier and the core service words.
const replBoruPreamble = `

import "boru:net"
import "boru:vm"

# ============================================================
# boru:repl — a socket REPL, written in Boru.
#
# The server is a service over the lines codec: each received line is
# boru source, evaluated in a sandboxed sub-engine (Vm.run) against the
# session's accumulated history, and the rendered result (canon) is
# the reply line. Errors reply as "error: <message>" instead of
# killing the connection.
# ============================================================

# Render a caught evaluation error as the reply line.
def repl-format-err fn [[m:Any] [String] [ join "" ["error: " m] ]]

# Evaluate one line against the session history carried in the service
# state; on success the line joins the history (the replay model that
# makes defs persist across sandboxed one-shot evaluations).
def repl-eval-line fn [[st:Any line:String] [String] [
  if (line eq "/reset") [
    st set history "" drop
    "ok: session reset"
  ] [
    def src (if (st.history eq "") [ line ] [ join "\n" [st.history line] ])
    do [
      def out (canon (Vm.run src))
      st set history src drop
      out
    ] error [
      dot message
      repl-format-err
    ]
  ]
]]

# Start a REPL server: returns the Listener (close it to stop).
def repl-serve fn [[opts:Map] [Any] [
  def svc (service {history: ""})
  add {} ([req:Map state:Any] => [ def line req.line  repl-eval-line state line ]) svc
  Net.listen {tcp: opts.port codec: Net.lines} svc
]]

# Dial a REPL server: returns an Endpoint.
def repl-connect fn [[addr:String] [Any] [
  Net.connect {tcp: addr codec: Net.lines}
]]

# Evaluate one source line remotely; returns the reply text.
def repl-eval fn [[ep:Any src:String] [String] [
  def reply (call {line: src} ep {timeout: 10000})
  reply.line
]]

# Close a REPL server or endpoint (re-exported so callers need no
# separate boru:net import for teardown).
def repl-close fn [[h:Any] [] [ Net.close h ]]

export "Repl" {
  serve:   repl-serve/v
  connect: repl-connect/v
  eval:    repl-eval/v
  close:   repl-close/v
}
`
