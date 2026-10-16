package test

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// TestModuleInstanceFileModule covers the file-module variation of the
// module namespace / Module descriptor model: a module loaded from a file
// has kind="file" with file/folder populated, while exports stay
// transparently usable and the namespace is a plain facet-carrying Map.
func TestModuleInstanceFileModule(t *testing.T) {
	files := map[string]string{
		"proj/lib.boru": `export "Lib" {answer: 42}`,
	}
	run := func(step string) core.Value {
		t.Helper()
		res, err := runModuleSteps(t, files, []string{`import "./proj/lib.boru"`, step})
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		return res[len(res)-1]
	}

	if ns := run(`Lib`); core.ModuleNSOf(ns) == nil || !core.ParentOf(&ns).Equal(native.TMap) {
		t.Errorf("Lib = %s (type %s), want a facet-carrying namespace Map", ns.String(), core.ParentOf(&ns))
	}
	if v := run(`Lib.answer`); func() bool { n, _ := native.AsInteger(v); return n != 42 }() {
		t.Errorf("Lib.answer = %s, want 42", v.String())
	}
	mod := run(`Lib.$module`)
	if v := core.ParentOf(&mod); !v.Equal(native.TModuleInst) {
		t.Errorf("typeof Lib.$module = %s, want Module", v)
	}
	if s, _ := native.AsString(run(`Lib.$module.kind`)); s != "file" {
		t.Errorf("Lib.$module.kind = %q, want \"file\"", s)
	}
	// file ends in proj/lib.boru; folder ends in proj.
	if s, _ := native.AsString(run(`Lib.$module.file`)); !strings.HasSuffix(s, "proj/lib.boru") {
		t.Errorf("Lib.$module.file = %q, want suffix proj/lib.boru", s)
	}
	if s, _ := native.AsString(run(`Lib.$module.folder`)); !strings.HasSuffix(s, "proj") {
		t.Errorf("Lib.$module.folder = %q, want suffix proj", s)
	}
	if s, _ := native.AsString(run(`Lib.$module.name`)); s != "./proj/lib.boru" {
		t.Errorf("Lib.$module.name = %q, want \"./proj/lib.boru\"", s)
	}
}
