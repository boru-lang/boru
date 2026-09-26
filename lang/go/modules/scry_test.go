package modules

import (
	"sort"
	"strings"
	"testing"
)

// scrySeven is the self-knowledge boru:debug shipped first and boru:scry owns.
var scrySeven = []string{"body", "defs", "deps", "modules", "shape", "sig", "words"}

// TestScryExportsTheSelfKnowledgeWords pins NUR063's surface: boru:scry
// exports exactly the seven, and boru:debug still exports its copies (frozen,
// not removed — the deprecation has a timeline, not a break).
func TestScryExportsTheSelfKnowledgeWords(t *testing.T) {
	var got []string
	for _, n := range (selfKnowledge{prefix: "scry", ns: "Scry"}).natives() {
		got = append(got, (selfKnowledge{prefix: "scry"}).exportName(n.Name))
	}
	sort.Strings(got)
	if strings.Join(got, " ") != strings.Join(scrySeven, " ") {
		t.Errorf("Scry exports %v, want %v", got, scrySeven)
	}
	debug := map[string]bool{}
	natives := append(debugNatives(), stepNatives()...)
	for _, n := range natives {
		debug[debugExportName(n.Name)] = true
	}
	for _, w := range scrySeven {
		if !debug[w] {
			t.Errorf("Debug.%s must stay exported through its deprecation", w)
		}
	}
}

// TestDebugSelfKnowledgeIsDeprecatedToScry pins the describe half of the
// verdict: each Debug copy names its canonical Scry twin and the removal
// timeline, and no other Debug word is marked deprecated.
func TestDebugSelfKnowledgeIsDeprecatedToScry(t *testing.T) {
	seven := map[string]bool{}
	for _, w := range scrySeven {
		seven[w] = true
	}
	for w, doc := range moduleDocs["boru:debug"] {
		deprecated := strings.Contains(doc, "Deprecated")
		if seven[w] {
			if !deprecated || !strings.Contains(doc, "`Scry."+w+"`") || !strings.Contains(doc, "removed in") {
				t.Errorf("Debug.%s doc must mark it deprecated to Scry.%s with a timeline: %q", w, w, doc)
			}
		} else if deprecated {
			t.Errorf("Debug.%s is not one of the seven and must not be marked deprecated: %q", w, doc)
		}
	}
	for _, w := range scrySeven {
		if doc := moduleDocs["boru:scry"][w]; doc == "" || strings.Contains(doc, "Deprecated") {
			t.Errorf("Scry.%s must be documented as the canonical word, got %q", w, doc)
		}
	}
}
