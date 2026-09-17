package formatter

import "strings"

// A Doc is a node in a Wadler/Prettier-style layout document — the output
// side of a declarative formatter. Rules (expressed as boru, see the
// boru:fmt module's `render` word and design/legacy/fmt-module-and-xslt.0.ignore)
// build a Doc and RenderDoc lays it out to width. This is the substrate an
// XSLT-style template set emits into: `text` is <xsl:value-of>, `group`
// and `line` are the width-driven line-breaking, `indent` is nesting.
type Doc interface{ isDoc() }

// DocText is literal text; it is never broken.
type DocText struct{ S string }

// DocLine is a break point: a single space when its enclosing group is
// laid out flat, or a newline + current indent when the group is broken.
// A Hard line always breaks (and forces its group to break).
type DocLine struct{ Hard bool }

// DocConcat is an ordered sequence of docs.
type DocConcat struct{ Parts []Doc }

// DocGroup lays its body out flat if it fits the remaining width,
// otherwise broken.
type DocGroup struct{ Body Doc }

// DocIndent increases the indentation of its body by By columns.
type DocIndent struct {
	By   int
	Body Doc
}

func (DocText) isDoc()   {}
func (DocLine) isDoc()   {}
func (DocConcat) isDoc() {}
func (DocGroup) isDoc()  {}
func (DocIndent) isDoc() {}

// RenderDoc lays out doc within the given width, returning the formatted
// text. Groups that fit flat are kept on one line; groups that do not are
// broken at their line points, with nested indentation applied.
func RenderDoc(width int, doc Doc) string {
	var b strings.Builder
	type item struct {
		indent int
		flat   bool
		doc    Doc
	}
	col := 0
	stack := []item{{0, false, doc}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch d := it.doc.(type) {
		case DocText:
			b.WriteString(d.S)
			col += len(d.S)
		case DocLine:
			if it.flat && !d.Hard {
				b.WriteByte(' ')
				col++
			} else {
				b.WriteByte('\n')
				// A document node may carry a hostile indent (`{fmt:'indent'
				// by:-1}` drives it.indent negative; a huge `by` drives it
				// absurdly large). pad clamps a negative run to zero and an
				// oversized one to maxPad, so a caller-supplied document tree
				// can never panic strings.Repeat or force an unbounded
				// allocation (ADR-005). col tracks what was actually emitted.
				ind := pad(it.indent)
				b.WriteString(ind)
				col = len(ind)
			}
		case DocConcat:
			for i := len(d.Parts) - 1; i >= 0; i-- {
				stack = append(stack, item{it.indent, it.flat, d.Parts[i]})
			}
		case DocIndent:
			stack = append(stack, item{it.indent + d.By, it.flat, d.Body})
		case DocGroup:
			flat := it.flat || fitsFlat(width-col, d.Body)
			stack = append(stack, item{it.indent, flat, d.Body})
		}
	}
	return b.String()
}

// fitsFlat reports whether doc, laid out flat (every line a single
// space), fits within remaining columns. A hard line never fits flat.
func fitsFlat(remaining int, doc Doc) bool {
	stack := []Doc{doc}
	for len(stack) > 0 && remaining >= 0 {
		d := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch x := d.(type) {
		case DocText:
			remaining -= len(x.S)
		case DocLine:
			if x.Hard {
				return false
			}
			remaining--
		case DocConcat:
			for i := len(x.Parts) - 1; i >= 0; i-- {
				stack = append(stack, x.Parts[i])
			}
		case DocIndent:
			stack = append(stack, x.Body)
		case DocGroup:
			stack = append(stack, x.Body)
		}
	}
	return remaining >= 0
}
