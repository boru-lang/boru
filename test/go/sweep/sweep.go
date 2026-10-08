// Package sweep is the generated sweep of design/FULL-COMPILATION-REPLAN.0.md
// step S0 (FULL-COMPILATION-REVIEW.0.md §3.5): for every declaration-relevant
// word of the default registry — a word whose handler takes a code body,
// quotes an operand, declares a callable convention or can receive a fn
// value — one program per OPERAND KIND (the body written literally, an
// inline lambda, a fn value from a def, from a factory, read from a
// container, exported by a module, a body bound at run time), each
// classified through the dual interpreter/compiler pipeline of test/go/vary
// (the interpreter is the oracle) and re-embedded in every compile context
// of vary's transform table, which is the CALL-FORM axis. The result is a
// word × kind matrix. A cell with no program is a hole in the instrument; a
// program the interpreter rejects is a seed to fix or an n/a to justify;
// and every valid program that fails to compile, islands or diverges is a
// compiler defect on the ledger the later steps retire from.
//
// The seeds are hand-written in seeds.tsv, one line per cell, the same
// shape as the spec corpus, because a valid program for `def`, `import` or
// `walk` cannot be synthesised from a signature. What the sweep automates
// is the kinds, the call forms and the classification.
package sweep

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/boru-lang/boru/test/go/vary"
)

// Kind is the operand kind of a word's declaration-relevant slot.
type Kind string

// The kinds, in matrix column order.
const (
	Literal      Kind = "literal"       // the body or operand written at the call site
	Lambda       Kind = "lambda"        // an inline lambda value
	NamedFn      Kind = "named-fn"      // a fn value from a def (f/v)
	Factory      Kind = "factory"       // a fn value returned by a fn
	Container    Kind = "container"     // a fn value read from a map, list or class field
	ModuleExport Kind = "module-export" // a fn value exported by a module
	Computed     Kind = "computed"      // a body bound at run time (quote) and passed by name
)

// Kinds is every kind, in column order.
var Kinds = []Kind{Literal, Lambda, NamedFn, Factory, Container, ModuleExport, Computed}

// KnownKind reports whether k is one of Kinds.
func KnownKind(k Kind) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Class says which kinds a word's relevant slot admits.
type Class string

const (
	// Body: the slot takes a code body or a fn value — every kind.
	Body Class = "body"
	// Quoted: the slot is a written token the handler reads — the literal
	// kind only; the call forms carry the variation.
	Quoted Class = "quoted"
)

// KindsOf is the columns a class fills.
func KindsOf(c Class) []Kind {
	if c == Quoted {
		return []Kind{Literal}
	}
	return Kinds
}

// Seed is one hand-written program for a (word, kind) cell. NA marks a
// PROBE: a program the interpreter must reject, which is how a cell the
// language cannot express is claimed — checkably, because a probe the
// interpreter starts accepting is a stale claim and the cell becomes real.
type Seed struct {
	Word string
	Kind Kind
	Src  string
	NA   bool
}

// Key is the seed's cell, "word/kind".
func (s Seed) Key() string { return s.Word + "/" + string(s.Kind) }

//go:embed seeds.tsv
var seedsTSV string

// Seeds parses the embedded table.
func Seeds() ([]Seed, error) { return ParseSeeds("seeds.tsv", seedsTSV) }

// ParseSeeds parses `word TAB kind TAB program [TAB n/a]` lines; `#` lines
// and blank lines are skipped; a (word, kind) pair appears at most once.
func ParseSeeds(path, data string) ([]Seed, error) {
	var seeds []Seed
	seen := map[string]bool{}
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(strings.TrimSuffix(line, "\r"), " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cells := strings.Split(line, "\t")
		if len(cells) < 3 || len(cells) > 4 || (len(cells) == 4 && cells[3] != "n/a") {
			return nil, fmt.Errorf("%s:%d: want <word>\\t<kind>\\t<program>[\\tn/a], got %q", path, i+1, line)
		}
		s := Seed{Word: cells[0], Kind: Kind(cells[1]), Src: strings.TrimSpace(cells[2]), NA: len(cells) == 4}
		if !KnownKind(s.Kind) {
			return nil, fmt.Errorf("%s:%d: unknown kind %q, want one of %v", path, i+1, cells[1], Kinds)
		}
		if s.Word == "" || s.Src == "" {
			return nil, fmt.Errorf("%s:%d: the word and the program must both be present", path, i+1)
		}
		if seen[s.Key()] {
			return nil, fmt.Errorf("%s:%d: %s %s is listed twice", path, i+1, s.Word, s.Kind)
		}
		seen[s.Key()] = true
		seeds = append(seeds, s)
	}
	return seeds, nil
}

// Orphans lists the seeds that are not cells of the matrix: their word is
// not in the inventory, or their kind is not one the word's class admits.
func Orphans(inventory map[string]Class, seeds []Seed) []Seed {
	var out []Seed
	for _, s := range seeds {
		class, ok := inventory[s.Word]
		if !ok || (class == Quoted && s.Kind != Literal) {
			out = append(out, s)
		}
	}
	return out
}

// Status is a cell's verdict.
type Status string

const (
	Empty         Status = "empty"        // no seed
	NotApplicable Status = "n/a"          // a probe the interpreter rejects, as claimed
	StaleNA       Status = "n/a-STALE"    // a probe the interpreter accepts: the claim is stale
	Invalid       Status = "invalid"      // a seed the interpreter rejects
	CheckReject   Status = "check-reject" // the interpreter runs it and CompileCheck hard-errors
	Failed        Status = "failed"       // fails to compile
	Islanded      Status = "islanded"     // compiles with an interpreter island
	Diverged      Status = "DIVERGED"     // compiles and answers differently: a miscompile
	Panicked      Status = "PANIC"        // an engine crashed on it — recovered by the classifier
	Hung          Status = "HUNG"         // no engine answered within vary.Deadline
	Pass          Status = "pass"         // compiles natively with parity
)

// Cell is one (word, kind) of the matrix after the sweep.
type Cell struct {
	Word     string
	Kind     Kind
	Seed     *Seed
	Base     vary.Result    // the seed's own classification
	Variants []vary.Variant // the transform variants, when the base passes
}

// Key is the cell, "word/kind".
func (c Cell) Key() string { return c.Word + "/" + string(c.Kind) }

// Status is the cell's verdict.
func (c Cell) Status() Status {
	switch {
	case c.Seed == nil:
		return Empty
	case c.Seed.NA:
		if c.Base.Outcome == vary.InterpReject {
			return NotApplicable
		}
		return StaleNA
	}
	switch c.Base.Outcome {
	case vary.InterpReject:
		return Invalid
	case vary.CheckReject:
		return CheckReject
	case vary.Declined:
		return Failed
	case vary.Islanded:
		return Islanded
	case vary.Diverged:
		return Diverged
	case vary.Panicked:
		return Panicked
	case vary.Hung:
		return Hung
	}
	return Pass
}

// VariantsPassing counts the cell's transform variants that pass.
func (c Cell) VariantsPassing() int {
	n := 0
	for _, v := range c.Variants {
		if v.Res.Outcome == vary.Pass {
			n++
		}
	}
	return n
}

// Run classifies every seed — and, for a passing base, its transform
// variants — and lays the cells out over the inventory (word → class), in
// word order then column order. Seeds that are not cells are ignored;
// Orphans names them.
func Run(inventory map[string]Class, seeds []Seed, report func(done, total int)) []Cell {
	vseeds := make([]vary.Seed, len(seeds))
	for i, s := range seeds {
		vseeds[i] = vary.Seed{File: s.Word, Line: i, Input: s.Src}
	}
	byIndex := map[int][]vary.Variant{}
	for _, v := range vary.SweepSeeds(vseeds, report) {
		byIndex[v.Seed.Line] = append(byIndex[v.Seed.Line], v)
	}
	index := map[string]int{}
	for i, s := range seeds {
		index[s.Key()] = i
	}
	var cells []Cell
	for _, word := range sortedWords(inventory) {
		for _, kind := range KindsOf(inventory[word]) {
			cell := Cell{Word: word, Kind: kind}
			if i, ok := index[cell.Key()]; ok {
				cell.Seed = &seeds[i]
				vs := byIndex[i]
				cell.Base = vs[0].Res
				cell.Variants = vs[1:]
			}
			cells = append(cells, cell)
		}
	}
	return cells
}

func sortedWords(inventory map[string]Class) []string {
	words := make([]string, 0, len(inventory))
	for w := range inventory {
		words = append(words, w)
	}
	sort.Strings(words)
	return words
}

// Counts is the sweep's tally: cells by status, transform variants by
// outcome.
type Counts struct {
	Cells    map[Status]int
	Variants map[vary.Outcome]int
}

// Count tallies the cells.
func Count(cells []Cell) Counts {
	c := Counts{Cells: map[Status]int{}, Variants: map[vary.Outcome]int{}}
	for _, cell := range cells {
		c.Cells[cell.Status()]++
		for _, v := range cell.Variants {
			c.Variants[v.Res.Outcome]++
		}
	}
	return c
}

// Render writes the matrix and its defect lists as Markdown — the committed
// status surface, and the ledger the later steps retire from.
func Render(cells []Cell) string {
	var b strings.Builder
	b.WriteString("# Generated sweep — word × operand kind\n\n")
	b.WriteString("_Generated by `TestGeneratedSweep` (test/go/langspec) from `test/go/sweep/seeds.tsv` — do not edit by hand; refresh with `make sweep-status`._\n")
	b.WriteString("_Rows: every declaration-relevant word of the default registry. Columns: the operand kinds. A cell is `·` empty (no seed), `n/a` (a probe the interpreter rejects, as claimed), `✗` invalid (the interpreter rejects the seed), `C` check-reject, `F` fails to compile, `I` islanded, `D!` DIVERGED (a miscompile), `P!` PANIC (an engine crashed on it), `H!` HUNG (no answer within the deadline), or `✓ p/n` — compiles natively with parity, and p of its n call-form variants do too. The counts are gated in `TestGeneratedSweep`; this file is the list._\n\n")
	b.WriteString("| word |")
	for _, k := range Kinds {
		fmt.Fprintf(&b, " %s |", k)
	}
	b.WriteString("\n|---|")
	for range Kinds {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	byWord := map[string]map[Kind]Cell{}
	var words []string
	for _, c := range cells {
		if byWord[c.Word] == nil {
			byWord[c.Word] = map[Kind]Cell{}
			words = append(words, c.Word)
		}
		byWord[c.Word][c.Kind] = c
	}
	for _, w := range words {
		fmt.Fprintf(&b, "| `%s` |", w)
		for _, k := range Kinds {
			c, ok := byWord[w][k]
			if !ok {
				b.WriteString(" — |")
				continue
			}
			fmt.Fprintf(&b, " %s |", glyph(c))
		}
		b.WriteString("\n")
	}
	counts := Count(cells)
	b.WriteString("\n## Cells\n\n")
	for _, s := range []Status{Pass, Failed, Islanded, Diverged, Panicked, Hung, CheckReject, Invalid, NotApplicable, StaleNA, Empty} {
		fmt.Fprintf(&b, "- %s: %d\n", s, counts.Cells[s])
	}
	b.WriteString("\n## Cells that are not green\n\n")
	n := 0
	for _, c := range cells {
		if s := c.Status(); s != Pass && s != NotApplicable {
			n++
			fmt.Fprintf(&b, "- `%s` %s — **%s**", c.Word, c.Kind, s)
			if c.Seed != nil {
				fmt.Fprintf(&b, ": `%s`", firstN(c.Seed.Src, 120))
			}
			if c.Base.Detail != "" {
				fmt.Fprintf(&b, " — %s", firstN(c.Base.Detail, 120))
			}
			b.WriteString("\n")
		}
	}
	if n == 0 {
		b.WriteString("_None._\n")
	}
	b.WriteString("\n## Call-form variants that are not green\n\n")
	n = 0
	for _, c := range cells {
		for _, v := range c.Variants {
			if v.Res.Outcome != vary.Pass {
				n++
				fmt.Fprintf(&b, "- `%s` %s · %s — **%s** — %s\n", c.Word, c.Kind, v.Transform, v.Res.Outcome, firstN(v.Res.Detail, 120))
			}
		}
	}
	if n == 0 {
		b.WriteString("_None._\n")
	}
	return b.String()
}

func glyph(c Cell) string {
	switch s := c.Status(); s {
	case Empty:
		return "·"
	case NotApplicable:
		return "n/a"
	case StaleNA:
		return "n/a?!"
	case Invalid:
		return "✗"
	case CheckReject:
		return "C"
	case Failed:
		return "F"
	case Islanded:
		return "I"
	case Diverged:
		return "D!"
	case Panicked:
		return "P!"
	case Hung:
		return "H!"
	}
	return fmt.Sprintf("✓ %d/%d", c.VariantsPassing(), len(c.Variants))
}

func firstN(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
