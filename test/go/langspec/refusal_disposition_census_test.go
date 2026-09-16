// The refusal-disposition census — every refusal site has a plan.
//
// The maintainer's definition of done (design/SESSION-HANDOVER.0.md,
// 2026-09-14) is that all valid code compiles, no exceptions. Under it a
// MarkUncompilable call site has exactly three legal futures:
//
//   - generic: the shape lowers to code that makes the interpreter's own
//     decision at run time (the generic lane, a region, a runtime-compiled
//     unit) — design/FULL-COMPILATION.0.md sections 6.2 to 6.7;
//   - trap: the site guards a statically DEFINITE error, so it compiles to
//     an OpTrap raising the interpreter's own error at the same moment
//     (section 6.9);
//   - delete: the guard is an internal invariant, a test fixture, or the
//     mechanism itself, and retires when refusal does (section 6.10).
//
// "Carve-out" is not one of them, which is what the ruling adds.
//
// refusal_site_census_test.go counts the sites; this file assigns each one
// a disposition and the stage that retires it, and gates the assignment in
// BOTH directions: a site with no entry fails (new debt must arrive with
// its plan), and an entry with no site fails (the table can only shrink as
// the sites do). The key is the site's file, its enclosing top-level
// function, and the site's ordinal within that function — a syntactic
// fact, unlike the reason string, which is usually built at run time. A
// reordering inside one function therefore moves keys, and the gate says
// so, which is the intended cost: every change to the refusal surface
// re-states its plan.
//
// The dispositions were assigned by reading each site on `main` at
// c34a2fb (2026-09-14), with the interpreter's own behaviour probed where
// the reason text did not settle trap-versus-generic (`if [] [1] [2]`
// raises `if: condition produced no value` on both lanes, so that site is
// a trap; a 0-netting ARM is legal and is a region). The stage numbers are
// FULL-COMPILATION.0.md section 10's. They are a plan, not a measurement,
// and the next author who retires a site removes its row.
package langspec

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// refusalDisposition is one site's plan under the definition of done.
type refusalDisposition struct {
	kind  string // generic | trap | delete
	stage int    // the FULL-COMPILATION.0.md section 10 stage that retires it
	note  string
}

const (
	dispGeneric = "generic"
	dispTrap    = "trap"
	dispDelete  = "delete"
)

// refusalDispositions is keyed by "<module path>:<enclosing func>#<ordinal>".
// A site outside any function (the EmitRecorder interface's own method
// declaration) has the function "<decl>".
var refusalDispositions = map[string]refusalDisposition{
	// basic/go — the definition and control words.
	"basic/go/native_control.go:if3ReturnsFn#1":                         {dispGeneric, 5, "a 0-netting arm is a region shape (the fifty-first increment took the top-level case)"},
	"basic/go/native_definition.go:InstallAndRecordDef#1":               {dispGeneric, 4, "computed fn shadowing a live binding: the binder half makes Defs and the carrier table one store"},
	"basic/go/native_definition.go:InstallAndRecordDef#2":               {dispGeneric, 3, "a dropped apply in a curried chain: the Apply kernel models the apply the analysis returned unchanged"},
	"basic/go/native_definition.go:MarkTypedContainerDefUncompilable#1": {dispGeneric, 6, "typed-def over a flex body: a runtime validate-and-tag op, the typed-def construction family"},
	"basic/go/native_definition.go:markRefineDefUncompilable#1":         {dispGeneric, 6, "a compiled store-with-reparent for the dynamic refinement"},
	"basic/go/native_definition.go:MarkFnPredicateBindUncompilable#1":   {dispGeneric, 6, "a predicate unit run at the bind, section 6.3"},
	"basic/go/native_definition.go:DefTypedHandler#1":                   {dispGeneric, 6, "DepScalar predicate validation as a compiled runtime check"},
	"basic/go/native_definition.go:FnTripleHandler#1":                   {dispGeneric, 7, "a fn built from computed parts is runtime-supplied code: compile it when it is built"},
	"basic/go/native_definition.go:AfnHandler#1":                        {dispGeneric, 7, "as FnTripleHandler, for afn"},

	// check/go — the analysis pass's own refusals.
	"check/go/carrier.go:RunFnBodyOnce#1":                  {dispGeneric, 8, "an analysis failure is not a program error (soundiness): the body lowers generically instead of refusing"},
	"check/go/check_fnbody.go:BuildFnBodyReturnsFn#1":      {dispGeneric, 3, "an Atom param bound to a computed value in a closure body: the capture rides as a value"},
	"check/go/check_recovery.go:RefuseForwardStackDrift#1": {dispGeneric, 5, "forward accounting across a dynamic residual: inside a region the stack is the address"},
	"check/go/check_recovery.go:refuseStrandedMemberFn#1":  {dispGeneric, 3, "a member fn value auto-applying mid-expression: the arrival model over the Apply kernel"},
	"check/go/check_recovery.go:checkModeSurfaceShape#1":   {dispGeneric, 4, "surface-shape typed dispatch: OpDispatchGeneric selects at run time"},
	"check/go/check_recovery.go:checkModeAssumeSig#1":      {dispGeneric, 4, "a recovered window rematches at run time (OpDispatchRematch); the definite-operand case already traps"},
	"check/go/check_recovery.go:checkModeAssumeSig#2":      {dispGeneric, 4, "as #1"},
	"check/go/check_recovery.go:checkModeAssumeSig#3":      {dispGeneric, 4, "as #1"},
	"check/go/method_shape.go:TryShapedMethodDispatch#1":   {dispGeneric, 3, "a 0-arg member landing the model cannot claim: the Apply kernel decides at the landing"},
	"check/go/method_shape.go:TryRecordMethodApply#1":      {dispGeneric, 5, "an operand of unknown provenance under a shaped apply: a region"},
	"check/go/method_shape.go:refuseArrival#1":             {dispGeneric, 3, "the member-arrival model's declines (the NUR038 seal): admit the stack form, the computed first argument, the anonymous member"},

	// compiler/go — the recorder.
	"compiler/go/callable_words.go:tryRecordClosure#1":                {dispGeneric, 4, "a gradual-Any collection's List-versus-Map overload: a runtime re-match"},
	"compiler/go/compiler_dispatch_record.go:recordDispatchOutcome#1": {dispGeneric, 4, "a code body naming a fn-local fn (NUR037), narrowed by the seventy-second increment to a CAPTURING local fn (a closure the frame placement cannot bake) or a def the unit's frames do not hold: a capture-free local fn's def is placed as a registry-visible install for the frame and the body resolves it on every path"},
	"compiler/go/compiler_dispatch_record.go:recordDispatchOutcome#2": {dispGeneric, 6, "a context read inside an inline-lowered body: a per-region context frame (family K)"},
	"compiler/go/emit.go:resolveOperand#1":                            {dispGeneric, 5, "a body literal embedding an enclosing container: construct the spine per call over a live member read"},
	"compiler/go/emit.go:NotifyNameRebound#1":                         {dispGeneric, 4, "a frozen read in an ESCAPING unit (the memo re-records the rest, Stage 4b): the lookup half"},
	"compiler/go/emit.go:NotifyNameRebound#2":                         {dispGeneric, 4, "the stored-handler latch, narrowed by the seventy-first increment to a dep the unit BAKED: a bare read of a module-scope value is seated live, a slot routes, a declared fn dispatched by name routes with a live lead (fnUnitRec.liveNames); what still refuses is a lambda helper as the original binding (no declaration site for the routed op to locate a unit by) and a dep a nested closure body baked"},
	"compiler/go/emit.go:RecordBranch#1":                              {dispGeneric, 7, "an arm the pass did not capture is a runtime value: compile it when it is known"},
	"compiler/go/emit.go:RecordBranch#2":                              {dispTrap, 8, "a condition netting no value raises `if: condition produced no value` on both lanes; a diverging condition raises its own error"},
	"compiler/go/emit.go:RecordBranch#3":                              {dispGeneric, 5, "a condition result of unknown provenance: a region"},
	"compiler/go/emit.go:RecordBranch#4":                              {dispGeneric, 5, "a condition operand of unknown provenance: a region"},
	"compiler/go/emit.go:RecordBranch#5":                              {dispGeneric, 7, "as RecordBranch#1, for the then arm"},
	"compiler/go/emit.go:RecordBranch#6":                              {dispGeneric, 5, "a then value of unknown provenance: a region"},
	"compiler/go/emit.go:RecordBranch#7":                              {dispGeneric, 5, "a computed then value under a non-stack condition: a region"},
	"compiler/go/emit.go:RecordBranch#8":                              {dispGeneric, 5, "an else value of unknown provenance: a region"},
	"compiler/go/emit.go:RecordBranch#9":                              {dispGeneric, 5, "two computed arms without an event condition: a region"},
	"compiler/go/emit.go:RecordBranch#10":                             {dispGeneric, 5, "a computed else value under a non-stack condition: a region"},
	"compiler/go/emit.go:NoteLoopCarried#1":                           {dispGeneric, 4, "a pre-loop value that no longer resolves: a live lookup of the carried binding"},
	"compiler/go/emit.go:RecordDefRebind#1":                           {dispGeneric, 3, "a fn value in a loop-carried slot: universal fn values"},
	"compiler/go/emit.go:RecordDefRebind#2":                           {dispGeneric, 5, "a carried rebind of unknown provenance: a region"},
	"compiler/go/emit.go:refuseUndef#1":                               {dispGeneric, 4, "the binder hooks' one site (RefuseCarriedUndef, RefuseSpeculativeUndef, RecordSpeculativeUndef's declines, the def-after-undef, forward-slot and unseated-read guards; since the seventieth increment RecordSpeculativeFnDef's declines, the unrouted-dispatch and value-read guards of a conditionally-defined fn): a speculative undef of a module-scope value binding is PLACED since the sixty-eighth increment (OpUndefDynScope, live reads seated at their tokens), a forward word slot of the name ROUTES since the sixty-ninth, and a fn def inside a conditional body at module scope is PLACED since the seventieth (OpBindResident at its site, its dispatches routed with a live lead, family L's replace included); what still refuses — an undef of a carried name, of a type, fn-family or frame binding, one the recorder cannot seat (a suspended recording, an arm-resident bracket), a def of the name inside its region, a forward slot the op cannot drive, a `/v` read the hook does not seat, a conditional fn def in a loop body, an each or do body, a fn body's replace, a dispatch of such a fn at the poly or rematch seat (an undrivable window takes the slot-less descriptor), a `/v` read of it — is the resident bridge's pairing, the region host's evaluations and the frame-local binder"},
	"compiler/go/emit.go:StartFnCompile#1":                            {dispGeneric, 3, "a closure capturing a runtime-minted value: the capture rides by value at construction"},
	"compiler/go/emit.go:StartFnCompile#2":                            {dispGeneric, 3, "an apply of a dynamic fn value mid-body: the Apply kernel at that point, section 6.4"},
	"compiler/go/emit.go:StartFnCompile#3":                            {dispGeneric, 5, "a body count the model cannot settle: a region; a definite mismatch is the RET contract's own raise"},
	"compiler/go/emit.go:StartFnCompile#4":                            {dispGeneric, 3, "a residual the frame replay cannot seat (NUR123): the bare fn-valued read is a word dispatch"},
	"compiler/go/emit.go:StartFnCompile#5":                            {dispGeneric, 3, "an unapplied fn value in a closure residual: the Apply kernel"},
	"compiler/go/emit.go:RecordUserCall#1":                            {dispGeneric, 5, "a call operand of unknown provenance: a region"},
	"compiler/go/emit.go:RecordUserCall#2":                            {dispGeneric, 3, "a capture unreachable at a call site: the capture resolves from the live frame"},
	"compiler/go/emit.go:RecordUserPolyCall#1":                        {dispGeneric, 5, "as RecordUserCall#1, for the poly call"},
	"compiler/go/emit.go:RecordDynApply#1":                            {dispGeneric, 3, "the dynamic-apply record's declines: the Apply kernel"},
	"compiler/go/emit.go:RecordLoop#1":                                {dispGeneric, 5, "a range of unknown provenance: FOR_SETUP over live values"},
	"compiler/go/emit.go:RecordLoop#2":                                {dispGeneric, 5, "a computed range start or step: FOR_SETUP over live values"},
	"compiler/go/emit.go:recordLoopEvent#1":                           {dispGeneric, 5, "a loop caller's own refusal (the while condition shapes): regions"},
	"compiler/go/emit.go:recordLoopEvent#2":                           {dispGeneric, 5, "a body netting several values per iteration: a runtime-count region"},
	"compiler/go/emit.go:recordLoopEvent#3":                           {dispDelete, 9, "an internal invariant (the loop setup registers the slot): a structured internal_error, never a refusal"},
	"compiler/go/emit.go:RecordPoly#1":                                {dispGeneric, 4, "polymorphic dispatch: OpDispatchGeneric"},
	"compiler/go/emit.go:RecordCall#1":                                {dispGeneric, 5, "a region whose run may carry a callable (NUR129): the run's callables go through the Apply kernel"},
	"compiler/go/emit.go:recordCallElided#1":                          {dispGeneric, 4, "apply over a dynamic lead with an unprovable overload: a runtime re-match"},
	"compiler/go/emit.go:recordCallRefusal#1":                         {dispGeneric, 4, "dispatch without a signature: the generic lane's lookup"},
	"compiler/go/emit.go:recordCallRefusal#2":                         {dispGeneric, 3, "anonymous function dispatch: the Apply kernel"},
	"compiler/go/emit.go:recordCallRefusal#3":                         {dispGeneric, 7, "a check-mode word's runtime effect: compile the expansion when it is produced"},
	"compiler/go/emit.go:recordCallRefusal#4":                         {dispGeneric, 3, "a user fn call without a unit: universal fn values"},
	"compiler/go/emit.go:recordCallRefusal#5":                         {dispGeneric, 5, "a full-stack word beyond FoldFullStack's exactness: a region"},
	"compiler/go/emit.go:recordCallRefusal#6":                         {dispGeneric, 3, "a fn value read from a container auto-dispatching: the arrival model"},
	"compiler/go/emit.go:recordCallRefusal#7":                         {dispGeneric, 6, "a context-dependent word: a per-region context frame"},
	"compiler/go/emit.go:recordCallRefusal#8":                         {dispGeneric, 6, "a code-body word: units, not tokens (the handler migration)"},
	"compiler/go/emit.go:recordCallRefusal#9":                         {dispGeneric, 6, "a quoted-operand word: the modifier words lower to a dispatch"},
	"compiler/go/emit.go:recordCallRefusal#10":                        {dispGeneric, 4, "core-default dispatch over a carrier: OpDispatchGeneric"},
	"compiler/go/emit.go:recordCallRefusal#11":                        {dispGeneric, 4, "a dynamic input: OpDispatchGeneric"},
	"compiler/go/emit.go:recordCallRefusal#12":                        {dispGeneric, 6, "an unannotated or opaque word: the declaration triple, every signature declares"},
	"compiler/go/emit.go:recordCallRefusal#13":                        {dispGeneric, 5, "a computed range list: FOR_SETUP over a live list"},
	"compiler/go/emit.go:RecordCallOperands#1":                        {dispGeneric, 3, "a function-valued operand: universal fn values"},
	"compiler/go/emit.go:RecordCallOperands#2":                        {dispGeneric, 3, "a function value reaching a word: universal fn values"},
	"compiler/go/emit.go:RecordCallOperands#3":                        {dispGeneric, 7, "a capturing handler stored at a word: the stored body compiles with its captures at store time"},
	"compiler/go/emit.go:RecordCallOperands#4":                        {dispGeneric, 5, "an operand not statically materialisable: a region"},
	"compiler/go/emit.go:RecordPolyCall#1":                            {dispGeneric, 3, "as recordCallRefusal#6, for the poly call"},
	"compiler/go/emit.go:argIsProducedClosure#1":                      {dispGeneric, 3, "a produced closure at an argument slot whose apply did not collapse: the Apply kernel"},
	"compiler/go/emit.go:recordCodeBodyClosureRead#1":                 {dispGeneric, 7, "a code body reading a def-bound compiled closure: the body compiles instead of re-running"},
	"compiler/go/emit.go:RecordMakeListInner#1":                       {dispGeneric, 7, "a computed fn read inside an unevaluated body: the body compiles when it is evaluated"},
	"compiler/go/lower.go:planValueDefLocals#1":                       {dispGeneric, 5, "a side-effect loop's consumed result: a region"},
	"compiler/go/unit_memo.go:residualStands#1":                       {dispGeneric, 5, "a residual of unknown provenance or with a read hazard: a region"},
	"compiler/go/user_poly_plan.go:planUserPolyDispatch#1":            {dispGeneric, 4, "a gradual-Any argument to a multi-overload user fn: a runtime re-match"},
	"compiler/go/user_poly_plan.go:planUserPolyDispatch#2":            {dispGeneric, 4, "a fn-predicate-typed overload: predicate units inside matching, then a runtime re-match"},

	// core/go — the interpreter's own recorder calls.
	"core/go/core_helpers.go:installDef#1":        {dispGeneric, 4, "a capturing closure's conditional redefinition, and a fn-body redefinition by a capturing fn value (family L's closure arms): the binder half, NUR110's third binding state; a capture-free conditional-body redefinition is PLACED since the seventieth increment (NoteSpecFnDef — the recorder refuses what it cannot place)"},
	"core/go/emit_recorder.go:<decl>#1":           {dispDelete, 9, "the EmitRecorder method itself retires with the refusal mechanism"},
	"core/go/engine.go:stepLiteral#1":             {dispGeneric, 5, "a splice over a computed payload: the generalized mark region"},
	"core/go/engine.go:evalInterpString#1":        {dispGeneric, 7, "an interpolated string's runtime-computed part: compile the part when it is computed"},
	"core/go/engine.go:EvalXmlInterp#1":           {dispGeneric, 7, "as evalInterpString, for XML"},
	"core/go/engine.go:recordParenLeadingApply#1": {dispGeneric, 3, "a paren-bounded application with the fn value first: the Apply kernel"},
	"core/go/engine.go:recordParenLeadingApply#2": {dispGeneric, 3, "as #1"},

	// lang/go — the module layer.
	"lang/go/native/native_macro.go:miniHandler#1": {dispGeneric, 7, "a mini hook's expansion: compiled when it is produced"},

	// test/specfix — fixtures of the refusal path, counted by the site census.
	"test/specfix/control.go:fixIf3ReturnsFn#1": {dispDelete, 9, "a fixture of the refusal path; retires with it"},
	"test/specfix/control.go:fixIf3ReturnsFn#2": {dispDelete, 9, "a fixture of the refusal path; retires with it"},
}

var funcHeader = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z0-9_]+)`)

// refusalSiteKeys walks the same files as refusalSites and keys every
// MarkUncompilable call site by file, enclosing top-level function and
// ordinal within that function. It skips the same directories and the
// method's own declarations, so its count equals refusalSites' total.
func refusalSiteKeys(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	var keys []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "vendor", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		keys = append(keys, siteKeysIn(filepath.ToSlash(rel), string(src))...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(keys)
	return keys
}

// siteKeysIn keys the MarkUncompilable call sites of one file. A top-level
// `func` line opens a function; a `}` at column 0 closes it, so a site in
// an interface block or at file scope keys under "<decl>".
func siteKeysIn(rel, src string) []string {
	var keys []string
	fn := "<decl>"
	ordinal := map[string]int{}
	for _, line := range strings.Split(src, "\n") {
		if m := funcHeader.FindStringSubmatch(line); m != nil {
			fn = m[1]
		} else if line == "}" {
			fn = "<decl>"
		}
		if !strings.Contains(line, "MarkUncompilable(") || strings.Contains(line, "func ") {
			continue
		}
		ordinal[fn]++
		keys = append(keys, rel+":"+fn+"#"+itoa(ordinal[fn]))
	}
	return keys
}

// refusalDispositionCeiling pins the table's size in BOTH directions, which
// is what makes "the table only shrinks" a gate rather than a sentence: a
// site-and-row pair added together passes the two membership checks, so
// growth has to be caught by the count; and a site that retires must lower
// this number in the same change, so the history below records every
// retirement. Equal to refusalSiteCeiling by construction (the two scans
// count the same sites).
const refusalDispositionCeiling = 92 // 92 (2026-09-14, the census's first cut) -> 0 (Stage 9)

// dispositionFindings is the gate, factored so its negative arms can be
// driven over synthetic input: every finding is one string, and an empty
// result is a pass.
func dispositionFindings(keys []string, table map[string]refusalDisposition, ceiling int) (findings []string, byKind map[string]int, byStage map[int]int) {
	found := map[string]bool{}
	byKind = map[string]int{}
	byStage = map[int]int{}
	for _, k := range keys {
		found[k] = true
		d, ok := table[k]
		if !ok {
			findings = append(findings, "refusal site "+k+" has no disposition — under the definition of done every site needs one of generic, trap or delete")
			continue
		}
		switch d.kind {
		case dispGeneric, dispTrap, dispDelete:
		default:
			findings = append(findings, "refusal site "+k+": disposition "+d.kind+" is not one of the three the ruling permits")
		}
		if d.stage < 3 || d.stage > 9 {
			findings = append(findings, "refusal site "+k+": stage "+itoa(d.stage)+" is not a FULL-COMPILATION.0.md section 10 stage")
		}
		if strings.TrimSpace(d.note) == "" || strings.Contains(d.note, "\n") {
			findings = append(findings, "refusal site "+k+": the note must be one non-empty line naming the mechanism that retires the site")
		}
		byKind[d.kind]++
		byStage[d.stage]++
	}
	for k := range table {
		if !found[k] {
			findings = append(findings, "disposition table entry "+k+" names no refusal site — the site retired, so drop the row (or the key moved with a reordering: re-key it)")
		}
	}
	switch {
	case len(keys) > ceiling:
		findings = append(findings, "refusal-disposition census "+itoa(len(keys))+" exceeds ceiling "+itoa(ceiling)+" — a new refusal site was added with its row; the count only falls, so the site needs a generic lowering, a trap or a deletion instead")
	case len(keys) < ceiling:
		findings = append(findings, "refusal-disposition census "+itoa(len(keys))+" is below ceiling "+itoa(ceiling)+" — a site retired: lower the ceiling and record which, so the history stays exact")
	}
	sort.Strings(findings)
	return findings, byKind, byStage
}

func TestRefusalDispositionCensus(t *testing.T) {
	keys := refusalSiteKeys(t)
	_, total := refusalSites(t)
	if len(keys) != total {
		t.Errorf("disposition scan found %d sites, the site census %d — the two scans must agree", len(keys), total)
	}

	findings, byKind, byStage := dispositionFindings(keys, refusalDispositions, refusalDispositionCeiling)
	for _, f := range findings {
		t.Error(f)
	}

	stages := make([]int, 0, len(byStage))
	for s := range byStage {
		stages = append(stages, s)
	}
	sort.Ints(stages)
	parts := make([]string, len(stages))
	for i, s := range stages {
		parts[i] = "stage " + itoa(s) + "×" + itoa(byStage[s])
	}
	t.Logf("refusal-disposition census: %d sites — generic %d, trap %d, delete %d; by retiring stage: %s",
		len(keys), byKind[dispGeneric], byKind[dispTrap], byKind[dispDelete], strings.Join(parts, ", "))
}

// The gate's negative arms, over synthetic input: each shape that must fail
// produces exactly the finding that names it, and the clean shape produces
// none. The growth case is the one the membership checks alone cannot see —
// a site and its row added together.
func TestRefusalDispositionGateRefuses(t *testing.T) {
	ok := refusalDisposition{dispGeneric, 5, "a region"}
	base := map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": ok}
	keys := []string{"m/a.go:f#1", "m/a.go:f#2"}
	if f, _, _ := dispositionFindings(keys, base, 2); len(f) != 0 {
		t.Fatalf("a matched table at the ceiling must pass, got %v", f)
	}

	cases := []struct {
		name  string
		keys  []string
		table map[string]refusalDisposition
		ceil  int
		want  string
	}{
		{"a site and its row added together", append(keys, "m/a.go:g#1"),
			map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": ok, "m/a.go:g#1": ok}, 2, "exceeds ceiling"},
		{"a site retired without lowering the ceiling", keys[:1],
			map[string]refusalDisposition{"m/a.go:f#1": ok}, 2, "below ceiling"},
		{"a site with no row", keys, map[string]refusalDisposition{"m/a.go:f#1": ok}, 2, "has no disposition"},
		{"a row with no site", keys,
			map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": ok, "m/a.go:z#1": ok}, 2, "names no refusal site"},
		{"a disposition outside the three", keys,
			map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": {"carve-out", 5, "x"}}, 2, "not one of the three"},
		{"a stage outside section 10", keys,
			map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": {dispGeneric, 2, "x"}}, 2, "not a FULL-COMPILATION"},
		{"a blank note", keys,
			map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": {dispGeneric, 5, "  "}}, 2, "one non-empty line"},
		{"a multi-line note", keys,
			map[string]refusalDisposition{"m/a.go:f#1": ok, "m/a.go:f#2": {dispGeneric, 5, "a\nb"}}, 2, "one non-empty line"},
	}
	for _, c := range cases {
		f, _, _ := dispositionFindings(c.keys, c.table, c.ceil)
		hit := false
		for _, s := range f {
			if strings.Contains(s, c.want) {
				hit = true
			}
		}
		if !hit {
			t.Errorf("%s: want a finding containing %q, got %v", c.name, c.want, f)
		}
	}
}

func TestRefusalSiteKeysAreSyntactic(t *testing.T) {
	src := "package x\n\nfunc (es *E) A() {\n\tes.MarkUncompilable(\"one\")\n\tes.MarkUncompilable(\n\t\t\"two\")\n}\n\ntype R interface {\n\tMarkUncompilable(reason string)\n}\n\nfunc b() {\n\tx.MarkUncompilable(\"three\")\n}\n"
	got := siteKeysIn("m/f.go", src)
	want := []string{"m/f.go:A#1", "m/f.go:A#2", "m/f.go:<decl>#1", "m/f.go:b#1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("site keys: got %v, want %v", got, want)
	}
	// A method declaration is not a site, and neither scan counts a call on
	// a `func` line — the two scans must agree, so this one follows the
	// site census's rule rather than improving on it.
	if got := siteKeysIn("m/g.go", "func (inactive) MarkUncompilable(string) {}\nfunc c() { x.MarkUncompilable(\"same line\") }\n"); len(got) != 0 {
		t.Errorf("a func line is never a site, got %v", got)
	}
}
