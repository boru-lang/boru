package specfix

import "testing"

// BORU_SPEC_FILES selects spec files by basename or glob; unset selects all.
func TestSpecFileSelected(t *testing.T) {
	t.Setenv("BORU_SPEC_FILES", "")
	if !SpecFileSelected("anything.tsv") {
		t.Error("unset: every file is selected")
	}
	t.Setenv("BORU_SPEC_FILES", " callbacks.tsv, fold-*.tsv ,,")
	cases := map[string]bool{
		"callbacks.tsv":       true,
		"fold-map-filter.tsv": true,
		"each-variants.tsv":   false,
		"callbacks.tsv.bak":   false,
	}
	for name, want := range cases {
		if got := SpecFileSelected(name); got != want {
			t.Errorf("SpecFileSelected(%q) = %v, want %v", name, got, want)
		}
	}
}
