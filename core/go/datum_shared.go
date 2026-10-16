package core

// Shared datums (design/VALUE-NO-COPY.0.md §6 phase 4). A Datum is never
// written once a handle on it has been copied, so a value whose datum
// carries nothing per instance — a boolean, none, the empty string, a small
// integer, a payload-less structural marker — needs no datum of its own:
// every handle on it shares one, built at init, and minting it allocates
// nothing. A handle's occurrence state (position, identity, flags) is its
// own as ever, so sharing the datum changes nothing a holder can see.

// smallIntMin and smallIntMax bound the integers NewInteger shares: loop
// counters, indices, sizes and the arithmetic over them.
const (
	smallIntMin = -1024
	smallIntMax = 4095
)

var (
	trueDatum        = Datum{Parent: TBoolean, Data: BoolPayload{B: true}}
	falseDatum       = Datum{Parent: TBoolean, Data: BoolPayload{B: false}}
	noneDatum        = Datum{Parent: TNone, Data: NonePayload{}}
	emptyStringDatum = Datum{Parent: TStringEmpty, Data: StrPayload{S: ""}}
	openParenDatum   = Datum{Parent: TOpenParen}
	closeParenDatum  = Datum{Parent: TCloseParen}
	endDatum         = Datum{Parent: TEnd}

	// The collection kernel's probes: a value of the type a word WILL
	// become (true/false, an undefined word's atom), matched against a
	// slot without minting one. A probe is copied into a local handle
	// before it is handed out, so nothing writes the shared one.
	booleanProbeDatum = Datum{Parent: TBoolean}
	atomProbeDatum    = Datum{Parent: TAtom}
	// fnRefProbeDatum is a `/v` word's probe: one Function reference.
	fnRefProbeDatum = Datum{Parent: TFunction, Data: &FnDefInfo{}}

	// The dispatch words a reach lowers to (lowerReach): one `dot` or
	// `dotr` per segment of every evaluation.
	dotWordDatum  = Datum{Parent: TWord, Data: WordInfo{Name: "dot", ArgCount: -1}}
	dotrWordDatum = Datum{Parent: TWord, Data: WordInfo{Name: "dotr", ArgCount: -1}}

	smallIntDatums [smallIntMax - smallIntMin + 1]Datum
)

func init() {
	for i := range smallIntDatums {
		smallIntDatums[i].Parent = TInteger
		smallIntDatums[i].Data = IntPayload{N: int64(i) + smallIntMin}
	}
}

// sharedScalar is a handle on the shared datum d, given an identity as
// NewValueRaw gives a concrete value one: only inside a check or compile
// pass.
func sharedValue(d *Datum) Value {
	v := Value{d: d}
	if checkPassDepth.Load() > 0 {
		SetID(&v, GenerateID(IDPrefixForType(d.Parent)))
	}
	return v
}

// coDatum is a datum and its payload allocated as ONE object: the payload
// is held by pointer (P's pointer is the Payload), so boxing it costs no
// allocation of its own. The engine's per-step markers — forward, mark,
// move — are minted this way, and a marker that takes a new state takes a
// new coDatum (setCoPayload), so a holder that kept the old one keeps the
// old state, as a by-value payload gave it.
type coDatum[P any] struct {
	d Datum
	p P
}

// newCoDatum is a datum under parent whose payload is p, allocated with it.
func newCoDatum[P any, PP interface {
	*P
	Payload
}](parent *Type, p P) *Datum {
	x := &coDatum[P]{p: p}
	x.d.Parent = parent
	x.d.Data = PP(&x.p)
	return &x.d
}

// setCoPayload gives this holder a datum of its own built from its old one's
// parts with the payload p, allocated with it: cow and SetData in one
// object.
func setCoPayload[P any, PP interface {
	*P
	Payload
}](v *Value, p P) {
	x := &coDatum[P]{p: p}
	copyDatumParts(&x.d, v.d)
	x.d.Data = PP(&x.p)
	v.d = &x.d
}

// NewClosureValue is a Function value over the compiled closure cl, its
// payload allocated with its datum.
func NewClosureValue(cl ClosurePayload) Value {
	return NewClosureValueUnder(TFunction, cl)
}

// NewClosureValueUnder is NewClosureValue under the parent t (a closure that
// stands in for a fn value typed more narrowly than Function). Like the
// literal it replaces, it mints no identity: a closure's is its Ident.
func NewClosureValueUnder(t *Type, cl ClosurePayload) Value {
	return Value{d: newCoDatum(t, cl)}
}
