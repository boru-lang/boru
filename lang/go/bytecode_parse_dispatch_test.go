package lang

import (
	"strings"
	"testing"
)

// TestCompileParseOverEnclosingParserDef pins Stage 1: compiling a
// `parse <parser> src` dispatch where the parser is a TOP-LEVEL computed
// value-def (`def p (Parse.parser g)`) read from inside a fn body — the shape
// every voxgig-boru/template lexer uses (`def lex fn [… (parse mustache src) …]`).
//
// Two coupled fixes make it compile+execute:
//   - installDef declines to install the non-concrete Function carrier in Defs,
//     so RecordDynBind now records its ID (rootComputedBindIDs) as an enclosing
//     binding; the fn body's read then rescues to a runtime dyn-scope read
//     instead of an unreachable enclosing-producer operand (which declined
//     "branch reads enclosing computation").
//   - the read uses OpLookupDynScopeData (Signature.FnDataArgs on
//     parselang-fn-dispatch arg0), the DATA-position twin that PUSHES the parser
//     FnDefInfo binding instead of deferring like the plain OpLookupDynScope
//     (whose FnDefInfo defer is for a name-position read the interpreter would
//     dispatch). Byte-identical to the interpreter, which passes the
//     /q-captured name as data to parseFnDispatchHandler.
func TestCompileParseOverEnclosingParserDef(t *testing.T) {
	// A custom grammar with a boru-closure matcher that pushes tokens onto an
	// enclosing mutable flex accumulator, finalized into a parser VALUE bound at
	// top level, then read + dispatched from inside a fn body (`parse myp src`)
	// with the accumulator sliced around the parse — the lexer shape verbatim.
	src := `import "boru:parse"
import "boru:parselang"
def acc (flex [])
def g Parse.grammar
Parse.matcher g lex 5 ([s:String] => [ def _ (acc push {v:s})  {src: s tin:'#TX' val:'t'} ]) end
Parse.rule g val { open:[{s:'#TX' p:'val'}{}] close:[{s:'#ZZ'}{}] } end
def myp (Parse.parser g) end
def lex-it fn [ [src:String] [List] [
  def st (acc size)
  def _ (parse myp src)
  slice st (acc size) acc
] ]
(lex-it "hello")`

	// Compiles, runs on the VM, and agrees with the interpreter.
	requireEngineParity(t, src, true)

	// The lexer's compiled body reads the parser via the read-as-data opcode.
	dis := compileDisasm(t, src)
	if !strings.Contains(dis, "LOOKUP_DYN_SCOPE_DATA") {
		t.Errorf("want LOOKUP_DYN_SCOPE_DATA (parser read as data) in lex-it's body, got:\n%s", dis)
	}

	// The enclosing parser def read from a SECOND, deeper fn frame still rescues.
	nested := strings.Replace(src, `(lex-it "hello")`,
		`def deeper fn [[s:String] [List] [ (lex-it s) ]]
(deeper "hi")`, 1)
	requireEngineParity(t, nested, true)
}
