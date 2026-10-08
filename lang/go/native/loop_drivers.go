package native

import "fmt"

// The LoopDriver implementations of the collection words — each, for-each,
// fold, scan, outer, inner, eachrank, foldaxis and filter's quotation form,
// and the map forms of each / for-each / fold / scan over a quotation body.
// A handler builds the driver that holds the construct's Go state (the
// data, the accumulator, the results assembled so far) and exits through
// StartLoop, which splices the first iteration as a sealed region on the
// running tape and lets the engine drive the rest (core loop.go) — or,
// under the VM, drives the same driver from Go through the seam the handler
// used before. One driver therefore defines the construct's iteration order,
// result assembly and error texts for both lanes; nothing below runs a
// sub-engine.
//
// Every driver reuses one small inputs buffer across iterations: the tape
// lane copies the inputs into the region at once, and DriveLoop copies them
// before invoking a body.

// ---- each / for-each over a list ----

type eachListDriver struct {
	reg     *Registry
	word    string // "each" or "for-each"
	body    Value
	data    ReadList
	keep    bool // each keeps the body's top per element; for-each discards
	results []Value
	in      [1]Value
	cur     int
}

func newEachListDriver(reg *Registry, word string, body Value, data ReadList, keep bool) *eachListDriver {
	d := &eachListDriver{reg: reg, word: word, body: body, data: data, keep: keep}
	if keep {
		d.results = make([]Value, data.Len())
	}
	return d
}

func (d *eachListDriver) Next(it int) ([]Value, Value, bool, error) {
	if it >= d.data.Len() {
		return nil, Value{}, false, nil
	}
	d.cur = it
	d.in[0] = d.data.Get(it)
	return d.in[:], d.body, true, nil
}

func (d *eachListDriver) Collect(it int, res []Value) error {
	if !d.keep {
		return nil
	}
	if len(res) == 0 {
		return d.reg.BoruError("each_error", fmt.Sprintf("each: element %d: body produced no result", it), "each")
	}
	d.results[it] = res[len(res)-1] // take top of stack
	return nil
}

func (d *eachListDriver) Finish() ([]Value, error) {
	if !d.keep {
		return nil, nil
	}
	return []Value{NewList(d.results)}, nil
}

func (d *eachListDriver) Describe() string {
	return fmt.Sprintf("%s %d/%d", d.word, d.cur+1, d.data.Len())
}

func (d *eachListDriver) WrapError(it int, err error) error {
	return fmt.Errorf("%s: element %d: %w", d.word, it, err)
}

// ---- fold over a list ----

// foldDriver threads the accumulator over a list: the body sees
// (accumulator, element) and its top is the next accumulator. foldaxis
// composes one per lane (foldaxisDriver), with its own attribution.
type foldDriver struct {
	reg  *Registry
	body Value
	data ReadList
	acc  Value
	in   [2]Value
	cur  int
}

func (d *foldDriver) Next(it int) ([]Value, Value, bool, error) {
	if it >= d.data.Len() {
		return nil, Value{}, false, nil
	}
	d.cur = it
	d.in[0], d.in[1] = d.acc, d.data.Get(it)
	return d.in[:], d.body, true, nil
}

func (d *foldDriver) Collect(it int, res []Value) error {
	if len(res) == 0 {
		return d.reg.BoruError("fold_error", fmt.Sprintf("fold: step %d: body produced no result", it), "fold")
	}
	d.acc = res[len(res)-1]
	return nil
}

func (d *foldDriver) Finish() ([]Value, error) { return []Value{d.acc}, nil }

func (d *foldDriver) Describe() string {
	return fmt.Sprintf("fold %d/%d", d.cur+1, d.data.Len())
}

func (d *foldDriver) WrapError(it int, err error) error {
	return fmt.Errorf("fold: step %d: %w", it, err)
}

// ---- scan over a list ----

// scanDriver is fold keeping every accumulator state: the first element
// seeds (and is output 0); iteration it runs the body over element it+1.
type scanDriver struct {
	reg     *Registry
	body    Value
	data    ReadList
	acc     Value
	results []Value
	in      [2]Value
	cur     int
}

func (d *scanDriver) Next(it int) ([]Value, Value, bool, error) {
	i := it + 1
	if i >= d.data.Len() {
		return nil, Value{}, false, nil
	}
	d.cur = i
	d.in[0], d.in[1] = d.acc, d.data.Get(i)
	return d.in[:], d.body, true, nil
}

func (d *scanDriver) Collect(it int, res []Value) error {
	i := it + 1
	if len(res) == 0 {
		return d.reg.BoruError("scan_error", fmt.Sprintf("scan: step %d: body produced no result", i), "scan")
	}
	d.acc = res[len(res)-1]
	d.results[i] = d.acc
	return nil
}

func (d *scanDriver) Finish() ([]Value, error) { return []Value{NewList(d.results)}, nil }

func (d *scanDriver) Describe() string {
	return fmt.Sprintf("scan %d/%d", d.cur+1, d.data.Len())
}

func (d *scanDriver) WrapError(it int, err error) error {
	return fmt.Errorf("scan: step %d: %w", it+1, err)
}

// ---- outer ----

// outerDriver runs the body over every (left[i], right[j]) pair, row-major,
// producing the 2D list of the body's tops.
type outerDriver struct {
	reg         *Registry
	body        Value
	left, right ReadList
	rows        [][]Value
	in          [2]Value
	cur         int
}

func newOuterDriver(reg *Registry, body Value, left, right ReadList) *outerDriver {
	d := &outerDriver{reg: reg, body: body, left: left, right: right, rows: make([][]Value, left.Len())}
	for i := range d.rows {
		d.rows[i] = make([]Value, right.Len())
	}
	return d
}

func (d *outerDriver) cell(it int) (int, int) { return it / d.right.Len(), it % d.right.Len() }

func (d *outerDriver) Next(it int) ([]Value, Value, bool, error) {
	if d.right.Len() == 0 || it >= d.left.Len()*d.right.Len() {
		return nil, Value{}, false, nil
	}
	d.cur = it
	i, j := d.cell(it)
	d.in[0], d.in[1] = d.left.Get(i), d.right.Get(j)
	return d.in[:], d.body, true, nil
}

func (d *outerDriver) Collect(it int, res []Value) error {
	i, j := d.cell(it)
	if len(res) == 0 {
		return d.reg.BoruError("outer_error", fmt.Sprintf("outer: (%d,%d): body produced no result", i, j), "outer")
	}
	d.rows[i][j] = res[len(res)-1]
	return nil
}

func (d *outerDriver) Finish() ([]Value, error) {
	rows := make([]Value, len(d.rows))
	for i, row := range d.rows {
		rows[i] = NewList(row)
	}
	return []Value{NewList(rows)}, nil
}

func (d *outerDriver) Describe() string {
	i, j := 0, 0
	if d.right.Len() > 0 {
		i, j = d.cell(d.cur)
	}
	return fmt.Sprintf("outer (%d,%d) %d/%d", i, j, d.cur+1, d.left.Len()*d.right.Len())
}

func (d *outerDriver) WrapError(it int, err error) error {
	i, j := d.cell(it)
	return fmt.Errorf("outer: (%d,%d): %w", i, j, err)
}

// ---- inner ----

// innerDriver is the zip-then-fold of `inner`: a PAIR phase runs the pair
// body over (left[k], right[k]), then a FOLD phase threads the agg body
// over the paired values. The 1D form runs the two phases once; the 2D
// form runs them per cell (i, j) of the matrix product, the right operand
// transposed to columns, checking each cell's dimensions as it reaches it.
type innerDriver struct {
	reg         *Registry
	pairOp      Value
	aggOp       Value
	left, right ReadList
	twoD        bool
	rightCols   [][]Value
	rows        [][]Value
	i, j        int // the live cell (2D)
	k           int // the live pair / fold index within the cell
	folding     bool
	paired      []Value
	acc         Value
	started     bool
	in          [2]Value
	cellLeft    ReadList
	cellRight   []Value
}

// 1D: the vectors are the one cell.
func newInnerDriver1D(reg *Registry, pairOp, aggOp Value, left, right ReadList) *innerDriver {
	d := &innerDriver{reg: reg, pairOp: pairOp, aggOp: aggOp, left: left, right: right}
	d.cellLeft, d.cellRight = left, right.Slice()
	d.paired = make([]Value, left.Len())
	return d
}

// 2D: rows of left against the columns of right.
func newInnerDriver2D(reg *Registry, pairOp, aggOp Value, left, right ReadList) *innerDriver {
	d := &innerDriver{reg: reg, pairOp: pairOp, aggOp: aggOp, left: left, right: right, twoD: true}
	d.rightCols = transposeListOfLists(right)
	d.rows = make([][]Value, left.Len())
	for i := range d.rows {
		d.rows[i] = make([]Value, len(d.rightCols))
	}
	return d
}

// enterCell prepares cell (i, j) of the 2D product: its pair vectors, the
// dimension check the handler made on reaching the cell.
func (d *innerDriver) enterCell() error {
	leftRow, _ := AsList(d.left.Get(d.i))
	rightCol := d.rightCols[d.j]
	if leftRow.Len() != len(rightCol) {
		return d.reg.BoruError("inner_error", "inner: dimension mismatch", "inner")
	}
	d.cellLeft, d.cellRight = leftRow, rightCol
	d.paired = make([]Value, leftRow.Len())
	d.k, d.folding = 0, false
	return nil
}

// advance moves to the next step: the next pair, the fold's first step
// after the last pair, the next fold step, or the next cell.
func (d *innerDriver) advance() (bool, error) {
	if !d.started {
		d.started = true
		if d.twoD {
			if d.left.Len() == 0 || len(d.rightCols) == 0 {
				return false, nil
			}
			return true, d.enterCell()
		}
		return d.cellLeft.Len() > 0, nil
	}
	if !d.folding {
		d.k++
		if d.k < d.cellLeft.Len() {
			return true, nil
		}
		// Every pair is in: fold from the second one over the first.
		d.folding, d.k, d.acc = true, 1, d.paired[0]
	} else {
		d.k++
	}
	if d.k < len(d.paired) {
		return true, nil
	}
	// The cell's fold is done.
	if !d.twoD {
		return false, nil
	}
	d.rows[d.i][d.j] = d.acc
	d.j++
	if d.j >= len(d.rightCols) {
		d.j, d.i = 0, d.i+1
		if d.i >= d.left.Len() {
			return false, nil
		}
	}
	return true, d.enterCell()
}

func (d *innerDriver) Next(it int) ([]Value, Value, bool, error) {
	ok, err := d.advance()
	if err != nil || !ok {
		return nil, Value{}, false, err
	}
	if d.folding {
		d.in[0], d.in[1] = d.acc, d.paired[d.k]
		return d.in[:], d.aggOp, true, nil
	}
	d.in[0], d.in[1] = d.cellLeft.Get(d.k), d.cellRight[d.k]
	return d.in[:], d.pairOp, true, nil
}

func (d *innerDriver) Collect(_ int, res []Value) error {
	if len(res) == 0 {
		switch {
		case d.twoD && d.folding:
			return d.reg.BoruError("inner_error", fmt.Sprintf("inner: fold (%d,%d,%d): no result", d.i, d.j, d.k), "inner")
		case d.twoD:
			return d.reg.BoruError("inner_error", fmt.Sprintf("inner: pair (%d,%d,%d): no result", d.i, d.j, d.k), "inner")
		case d.folding:
			return d.reg.BoruError("inner_error", fmt.Sprintf("inner: fold %d: no result", d.k), "inner")
		}
		return d.reg.BoruError("inner_error", fmt.Sprintf("inner: pair %d: no result", d.k), "inner")
	}
	top := res[len(res)-1]
	if d.folding {
		d.acc = top
	} else {
		d.paired[d.k] = top
	}
	return nil
}

func (d *innerDriver) Finish() ([]Value, error) {
	if !d.twoD {
		return []Value{d.acc}, nil
	}
	rows := make([]Value, len(d.rows))
	for i, row := range d.rows {
		rows[i] = NewList(row)
	}
	return []Value{NewList(rows)}, nil
}

func (d *innerDriver) Describe() string {
	phase := "pair"
	if d.folding {
		phase = "fold"
	}
	if d.twoD {
		return fmt.Sprintf("inner %s (%d,%d,%d)", phase, d.i, d.j, d.k)
	}
	return fmt.Sprintf("inner %s %d/%d", phase, d.k+1, d.cellLeft.Len())
}

func (d *innerDriver) WrapError(_ int, err error) error {
	if d.twoD {
		return err // the 2D form reports a body error as it stands
	}
	if d.folding {
		return fmt.Errorf("inner: fold %d: %w", d.k, err)
	}
	return fmt.Errorf("inner: pair %d: %w", d.k, err)
}

// ---- eachrank ----

// eachrankDriver walks the data to the cells at the target depth, running
// the body once per cell in depth-first order and rebuilding the nesting
// around the bodies' tops. The walk is lazy, as the recursion was: a cell
// that is not a list where a list is expected is reported when the walk
// reaches it, after the cells before it ran.
type eachrankDriver struct {
	reg     *Registry
	body    Value
	root    Value
	depth   int // levels to descend from the root to a cell
	stack   []eachrankFrame
	started bool
	result  Value
	in      [1]Value
	cells   int
}

type eachrankFrame struct {
	list  ReadList
	idx   int
	out   []Value
	depth int // this frame's depth above the cells
}

func (d *eachrankDriver) Next(it int) ([]Value, Value, bool, error) {
	cell, ok, err := d.nextCell()
	if err != nil || !ok {
		return nil, Value{}, false, err
	}
	d.cells++
	d.in[0] = cell
	return d.in[:], d.body, true, nil
}

// nextCell advances the walk to the next cell at the target depth,
// completing the frames it climbs out of on the way.
func (d *eachrankDriver) nextCell() (Value, bool, error) {
	var cell Value
	var depth int
	if !d.started {
		d.started = true
		cell, depth = d.root, d.depth
	} else {
		c, dp, ok := d.climb()
		if !ok {
			return Value{}, false, nil
		}
		cell, depth = c, dp
	}
	for {
		for depth > 0 {
			if !cell.Parent.ConformsTo(TList) || !IsConcrete(cell) {
				return Value{}, false, d.reg.BoruError("eachrank_error", fmt.Sprintf("eachrank: rank exceeds nesting depth at %v", cell), "eachrank")
			}
			list, _ := AsList(cell)
			d.stack = append(d.stack, eachrankFrame{list: list, out: make([]Value, list.Len()), depth: depth})
			if list.Len() == 0 {
				break
			}
			cell, depth = list.Get(0), depth-1
		}
		if depth == 0 {
			return cell, true, nil
		}
		// An empty list where cells were expected: it completes at once.
		c, dp, ok := d.climb()
		if !ok {
			return Value{}, false, nil
		}
		cell, depth = c, dp
	}
}

// climb completes every finished frame, delivering its list upward, and
// returns the next sibling to descend into; ok is false when the root is
// complete.
func (d *eachrankDriver) climb() (Value, int, bool) {
	for len(d.stack) > 0 {
		top := &d.stack[len(d.stack)-1]
		if top.idx+1 < top.list.Len() {
			top.idx++
			return top.list.Get(top.idx), top.depth - 1, true
		}
		val := NewList(top.out)
		d.stack = d.stack[:len(d.stack)-1]
		d.deliver(val)
	}
	return Value{}, 0, false
}

// deliver places a finished cell or frame value into its parent frame, or
// as the result when it is the root.
func (d *eachrankDriver) deliver(v Value) {
	if len(d.stack) == 0 {
		d.result = v
		return
	}
	top := &d.stack[len(d.stack)-1]
	top.out[top.idx] = v
}

func (d *eachrankDriver) Collect(_ int, res []Value) error {
	if len(res) == 0 {
		return d.reg.BoruError("eachrank_error", "eachrank: body produced no result", "eachrank")
	}
	d.deliver(res[len(res)-1])
	return nil
}

// Finish hands back the rebuilt nesting: the walk closed every frame on
// its last climb (an empty data list closes its one frame on the first).
func (d *eachrankDriver) Finish() ([]Value, error) { return []Value{d.result}, nil }

func (d *eachrankDriver) Describe() string { return fmt.Sprintf("eachrank cell %d", d.cells) }

func (d *eachrankDriver) WrapError(_ int, err error) error { return fmt.Errorf("eachrank: %w", err) }

// ---- foldaxis ----

// foldaxisDriver reduces each lane (a row, or a transposed column) through
// fold's own step: lane i's first element seeds its accumulator and the
// body threads it over the rest. Errors carry fold's attribution inside
// the lane's, as the handler wrapped doFold's.
type foldaxisDriver struct {
	reg    *Registry
	body   Value
	lanes  [][]Value
	lane   int
	k      int // the live element index within the lane (1-based: 0 seeds)
	acc    Value
	result []Value
	in     [2]Value
	done   bool
}

// advance moves to the next (lane, element) step, seeding each lane as it
// is entered and raising the empty-lane error where the handler did.
func (d *foldaxisDriver) advance() (bool, error) {
	if d.done {
		return false, nil
	}
	for {
		if d.k == 0 || d.k >= len(d.lanes[d.lane]) {
			if d.k > 0 {
				d.result[d.lane] = d.acc
				d.lane++
			}
			if d.lane >= len(d.lanes) {
				d.done = true
				return false, nil
			}
			lane := d.lanes[d.lane]
			if len(lane) == 0 {
				d.done = true
				return false, d.reg.BoruError("foldaxis_error", fmt.Sprintf("foldaxis: lane %d is empty (no initial value)", d.lane), "foldaxis")
			}
			d.acc, d.k = lane[0], 1
			if d.k >= len(lane) {
				continue
			}
			return true, nil
		}
		return true, nil
	}
}

func (d *foldaxisDriver) Next(it int) ([]Value, Value, bool, error) {
	if it > 0 {
		d.k++
	}
	ok, err := d.advance()
	if err != nil || !ok {
		return nil, Value{}, false, err
	}
	d.in[0], d.in[1] = d.acc, d.lanes[d.lane][d.k]
	return d.in[:], d.body, true, nil
}

func (d *foldaxisDriver) Collect(_ int, res []Value) error {
	if len(res) == 0 {
		return fmt.Errorf("foldaxis: lane %d: %w", d.lane,
			d.reg.BoruError("fold_error", fmt.Sprintf("fold: step %d: body produced no result", d.k-1), "fold"))
	}
	d.acc = res[len(res)-1]
	return nil
}

// Finish hands back one value per lane: advance closed the last lane when
// it found no step left.
func (d *foldaxisDriver) Finish() ([]Value, error) { return []Value{NewList(d.result)}, nil }

func (d *foldaxisDriver) Describe() string {
	return fmt.Sprintf("foldaxis lane %d/%d step %d", d.lane+1, len(d.lanes), d.k)
}

func (d *foldaxisDriver) WrapError(_ int, err error) error {
	return fmt.Errorf("foldaxis: lane %d: %w", d.lane, fmt.Errorf("fold: step %d: %w", d.k-1, err))
}

// ---- filter (quotation form) ----

// filterDriver keeps the elements of a list (the entries of a map) whose
// body result is Boolean true; any other result is a loud error.
type filterDriver struct {
	reg    *Registry
	body   Value
	src    Value
	list   ReadList
	data   ReadMap
	keys   []string
	out    []Value
	outMap *OrderedMap
	in     [1]Value
	cur    int
}

func (d *filterDriver) isMap() bool { return d.outMap != nil }

func (d *filterDriver) size() int {
	if d.isMap() {
		return len(d.keys)
	}
	return d.list.Len()
}

func (d *filterDriver) label(it int) string {
	if d.isMap() {
		return fmt.Sprintf("key %q", d.keys[it])
	}
	return fmt.Sprintf("element %d", it)
}

func (d *filterDriver) elem(it int) Value {
	if d.isMap() {
		v, _ := d.data.Get(d.keys[it])
		return v
	}
	return d.list.Get(it)
}

func (d *filterDriver) Next(it int) ([]Value, Value, bool, error) {
	if it >= d.size() {
		return nil, Value{}, false, nil
	}
	d.cur = it
	d.in[0] = d.elem(it)
	return d.in[:], d.body, true, nil
}

func (d *filterDriver) Collect(it int, res []Value) error {
	label := d.label(it)
	if len(res) == 0 {
		return d.reg.BoruError("filter_error", fmt.Sprintf("filter: %s: body produced no result", label), "filter")
	}
	top := res[len(res)-1]
	if !top.Parent.ConformsTo(TBoolean) || !IsConcrete(top) {
		return d.reg.BoruError("filter_error", fmt.Sprintf("filter: %s: body must produce a Boolean, got %s", label, top.Parent.Name()), "filter")
	}
	if b, _ := AsBoolean(top); b {
		if d.isMap() {
			d.outMap.Set(d.keys[it], d.elem(it))
		} else {
			d.out = append(d.out, d.elem(it))
		}
	}
	return nil
}

func (d *filterDriver) Finish() ([]Value, error) {
	// #4 (round 3): filter keeps a subset of unchanged elements — retain
	// the source's [:T] / {:T} tag.
	if d.isMap() {
		return []Value{d2RetainElem(NewMap(d.outMap), d.src)}, nil
	}
	return []Value{d2RetainElem(NewList(d.out), d.src)}, nil
}

func (d *filterDriver) Describe() string {
	return fmt.Sprintf("filter %d/%d", d.cur+1, d.size())
}

func (d *filterDriver) WrapError(it int, err error) error {
	return fmt.Errorf("filter: %s: %w", d.label(it), err)
}

// ---- the map forms over a quotation body ----

// mapQuotKind is which construct a mapQuotDriver drives.
type mapQuotKind int

const (
	mapQuotEach mapQuotKind = iota
	mapQuotForEach
	mapQuotFold
	mapQuotScan
)

// mapQuotDriver iterates a map's entries in insertion order with the
// entry's VALUE pushed for a quotation body (the lambda forms, handed a
// KeyVal through the callback seam, keep their Go loop). each keeps the
// map shape over the body's tops; for-each discards; fold threads an
// accumulator from `start` (0 seeded, 1 seeded by the first value); scan
// is fold keeping every state under its key.
type mapQuotDriver struct {
	reg   *Registry
	kind  mapQuotKind
	word  string
	body  Value
	data  ReadMap
	keys  []string
	start int
	acc   Value
	out   *OrderedMap
	in    [2]Value
	cur   int
}

func (d *mapQuotDriver) Next(it int) ([]Value, Value, bool, error) {
	idx := d.start + it
	if idx >= len(d.keys) {
		return nil, Value{}, false, nil
	}
	d.cur = idx
	v, _ := d.data.Get(d.keys[idx])
	if d.kind == mapQuotFold || d.kind == mapQuotScan {
		d.in[0], d.in[1] = d.acc, v
		return d.in[:2], d.body, true, nil
	}
	d.in[0] = v
	return d.in[:1], d.body, true, nil
}

func (d *mapQuotDriver) Collect(it int, res []Value) error {
	if d.kind == mapQuotForEach {
		return nil
	}
	k := d.keys[d.start+it]
	if len(res) == 0 {
		return d.reg.BoruError(d.word+"_error", fmt.Sprintf("%s: key %q: body produced no result", d.word, k), d.word)
	}
	top := res[len(res)-1]
	switch d.kind {
	case mapQuotEach:
		d.out.Set(k, top)
	case mapQuotFold:
		d.acc = top
	case mapQuotScan:
		d.acc = top
		d.out.Set(k, top)
	}
	return nil
}

func (d *mapQuotDriver) Finish() ([]Value, error) {
	switch d.kind {
	case mapQuotForEach:
		return nil, nil
	case mapQuotFold:
		return []Value{d.acc}, nil
	}
	return []Value{NewMap(d.out)}, nil
}

func (d *mapQuotDriver) Describe() string {
	return fmt.Sprintf("%s %d/%d", d.word, d.cur+1, len(d.keys))
}

func (d *mapQuotDriver) WrapError(it int, err error) error {
	return fmt.Errorf("%s: key %q: %w", d.word, d.keys[d.start+it], err)
}

// quotationInvoke is the Go-lane body invocation of a map quotation —
// runQuotationBody's rule: an EMPTY quotation is the identity on its inputs
// and runs nothing; otherwise the tokens run over the resolved inputs.
func quotationInvoke(reg *Registry, tokens []Value) LoopInvoke {
	return func(_ Value, inputs []Value) ([]Value, error) {
		if len(tokens) == 0 {
			return inputs, nil
		}
		return RunResolved(reg, inputs, tokens)
	}
}
