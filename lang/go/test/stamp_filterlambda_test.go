package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestStampServiceFilterLambda pins the mini-s3 bucket-list wall: an
// ANONYMOUS service handler whose body runs `filter` with a CAPTURING
// lambda over a convert/slice chain on the callback entry's dynamic
// `.value`. Two fixes carry it: (1) every closure/stored probe inherits
// the detached stamp's gradual-nesting modality (probe and real must
// judge the SAME unit — forkForProbe / tryReturnedClosure /
// compileStoredBody); (2) RecordPolyCall resolves an un-stepped
// TYPE-NAME word in the claimed window (`convert Bytes <dynamic>` — the
// dispatch never committed, so `Bytes` was never stepped to its
// literal) to the canonical type literal via stepWord's own
// typeNames/ResolveTypePath cascade, so the poly window re-matches over
// exactly the operands the interpreter's dispatch sees.
func TestStampServiceFilterLambda(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	modules.InstallResolver(reg)
	reg.EnableRuntimeStamping()
	engine := native.NewTop(reg)
	steps := []string{`
module [
  def mk-store fn [[] [Any] [
    def store (service {objects: {}})
    add {op:"put"} ([req:Map state:Any] => [
      state.objects set (req.key) req.val
      drop
      0
    ]) store
    add {op:"list"} ([req:Map state:Any] => [
      def objects state.objects
      def prefix (join "" [req.bucket "/"])
      def plen (size prefix)
      filter ([e:Any] => [
        def kk e.value
        if ((objects get kk) eq None) [ false ] [
          if ((size kk) lt plen) [ false ] [
            (convert String (slice 0 plen (convert Bytes kk))) eq prefix
          ]
        ]
      ]) (keys objects)
    ]) store
    store
  ]]
  export "M" { mk-store: mk-store/v }
] import`,
		`def s (M.mk-store)
call {op:"put" key:"b/x" val:"1"} s drop
call {op:"put" key:"c/y" val:"2"} s drop
def r (call {op:"list" bucket:"b"} s)
size r`,
	}
	var out []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			t.Fatal(pErr)
		}
		out, err = engine.Run(vals)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Runtime parity: exactly one key matches the "b/" prefix.
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	if n, nErr := out[0].AsConcreteInteger(); nErr != nil || n != 1 {
		t.Fatalf("list filter parity: got %v (%v), want 1", out[0], nErr)
	}
	// Stamp outcomes: every service handler — the capturing-filter list
	// handler included — compiled to its unit; any compile failure here is a
	// regression of the probe-modality or poly-type-name fixes.
	for _, ev := range reg.StampEvents() {
		if !ev.Stamped {
			t.Fatalf("service handler declined: %s (%s)", ev.Name, ev.Reason)
		}
	}
}
