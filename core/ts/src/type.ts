// boru types form a slash-separated path lattice.
//
// A child type matches a parent pattern: Scalar/String/ProperString
// matches Scalar/String matches Scalar. The reverse is not true. The
// lattice and the short-name expansion table mirror the Go engine
// (eng/go/types.go + eng/go/typetable.go) so that a Type built here
// from "Integer" expands to "Scalar/Number/Integer" identically, and
// `typeof` / `pathof` render the same leaf names ("Float",
// "ProperString", …) as the reference engine.
//
// PARITY NOTE: this is a strict subset of the Go Type. It carries the
// path parts and supports matches / equal / specificity / toString /
// leaf. It does NOT implement: ID assignment, Behavior dispatch, Rank
// ordering, the Dependent-leaf bolt-on, metatype derivation, or
// MintType. User-minted types are handled separately where the specs
// reach them.

// builtinDecl mirrors core/go/typetable.go::builtinDecl (the subset of
// fields the TS port needs: path, the unified lattice rank, an optional
// friendly short alias for expansion, and the internal flag that keeps a
// type out of the user-facing name table).
interface BuiltinDecl {
  path: string;
  /**
   * The unified lattice Rank — the one integer the total order falls back
   * to for every cross-type comparison (core/go/compare_types.go). A child
   * ranks above its parent, siblings run least-to-most complex.
   */
  rank: number;
  alias?: string;
  internal?: boolean;
}

// builtinDecls mirrors core/go/typetable.go::builtinDecls ROW FOR ROW, in
// the same parent-first order and with the same ranks. It was a subset —
// thirteen nodes short (eight Microns, the three Weak flex nodes, KeyVal,
// Module) and rank-less — so `Module` was undefined_word here and a type
// literal in Go, and nothing could order two types the way Go does.
// The table is DATA the Go kernel owns: type-table.test.ts reads
// core/go/typetable.go and holds this copy to it row by row, as
// spec/tools/gen-typetable.sh does for the Lean model's.
const builtinDecls: BuiltinDecl[] = [
  // Degenerate roots.
  { path: "Any", rank: 11_000_000_000 },
  { path: "None", rank: 12_000_000_000 },
  { path: "Never", rank: 13_000_000_000 },
  { path: "Absent", rank: 14_000_000_000 },

  // Branch roots (sit under Any in the lattice; Path() stays the leaf).
  { path: "Scalar", rank: 20_000_000_000 },
  { path: "Node", rank: 30_000_000_000 },
  { path: "Ideal", rank: 40_000_000_000 },
  { path: "Word", rank: 50_000_000_000 },
  { path: "Type", rank: 60_000_000_000 },

  // Scalar branch.
  { path: "Scalar/Atom", rank: 20_100_000_000 },
  { path: "Scalar/Boolean", rank: 20_200_000_000 },
  { path: "Scalar/Number", rank: 20_300_000_000 },
  { path: "Scalar/Number/Integer", rank: 20_310_000_000 },
  { path: "Scalar/Number/Float", rank: 20_320_000_000 },
  { path: "Scalar/Number/BigInteger", rank: 20_330_000_000 },
  { path: "Scalar/Number/BigDecimal", rank: 20_340_000_000 },
  { path: "Scalar/String", rank: 20_400_000_000 },
  { path: "Scalar/String/EmptyString", rank: 20_410_000_000 },
  { path: "Scalar/String/ProperString", rank: 20_420_000_000 },
  // Scalar/Micron — the structured-scalar family. The TS port carries the
  // lattice rows only; the content (validators, grammars) lives in Go.
  { path: "Scalar/Micron", rank: 20_500_000_000 },
  { path: "Scalar/Micron/Pathon", rank: 20_510_000_000 },
  { path: "Scalar/Micron/Emailon", rank: 20_520_000_000 },
  { path: "Scalar/Micron/Urlon", rank: 20_530_000_000 },
  { path: "Scalar/Micron/Ipon", rank: 20_540_000_000 },
  { path: "Scalar/Micron/Hoston", rank: 20_550_000_000 },
  { path: "Scalar/Micron/Semveron", rank: 20_560_000_000 },
  { path: "Scalar/Micron/Cidron", rank: 20_570_000_000 },
  { path: "Scalar/Micron/Macon", rank: 20_580_000_000 },
  { path: "Scalar/Micron/Coloron", rank: 20_590_000_000 },
  { path: "Scalar/Micron/Mimon", rank: 20_600_000_000 },
  { path: "Scalar/Micron/Qion", rank: 20_610_000_000 },
  { path: "Scalar/Micron/Phonon", rank: 20_620_000_000 },

  // Node branch.
  { path: "Node/List", rank: 30_100_000_000 },
  { path: "Node/List/Args", rank: 30_110_000_000 },
  { path: "Node/List/FlexList", rank: 30_120_000_000 },
  { path: "Node/List/FlexList/WeakFlexList", rank: 30_121_000_000 },
  { path: "Node/Map", rank: 30_200_000_000 },
  { path: "Node/Map/Inspect", rank: 30_210_000_000 },
  { path: "Node/Map/FlexMap", rank: 30_220_000_000 },
  { path: "Node/Map/FlexMap/WeakFlexMap", rank: 30_221_000_000 },
  { path: "Node/Map/KeyVal", rank: 31_000_000_000 },
  { path: "Node/Xml", rank: 30_300_000_000 },
  { path: "Node/Xml/FlexXml", rank: 30_310_000_000 },
  { path: "Node/Xml/FlexXml/WeakFlexXml", rank: 30_311_000_000 },

  // Ideal branch.
  { path: "Ideal/Resource", rank: 40_120_000_000 },
  { path: "Ideal/Resource/Entity", rank: 40_121_000_000 },
  { path: "Ideal/Record", rank: 40_300_000_000 },
  { path: "Ideal/Options", rank: 40_400_000_000 },
  { path: "Ideal/Error", rank: 40_500_000_000 },
  { path: "Ideal/Store", rank: 40_600_000_000 },
  { path: "Ideal/Store/System", rank: 40_610_000_000 },
  { path: "Ideal/Table", rank: 40_700_000_000 },
  { path: "Ideal/Reach", rank: 40_800_000_000 },
  { path: "Ideal/Class", rank: 40_900_000_000 },
  { path: "Ideal/Surface", rank: 40_910_000_000 },
  { path: "Ideal/Module", rank: 41_000_000_000 },

  // Word branch — interpreter-internal markers (kept out of byName).
  { path: "Word/__FW", rank: 50_100_000_000, alias: "Forward", internal: true },
  { path: "Word/__OP", rank: 50_100_001_000, alias: "Paren", internal: true },
  { path: "Word/__CP", rank: 50_100_002_000, alias: "CloseParen", internal: true },
  { path: "Word/__ED", rank: 50_100_003_000, alias: "End", internal: true },
  { path: "Word/__PE", rank: 50_100_004_000, internal: true },
  { path: "Word/__IS", rank: 50_100_005_000, internal: true },
  { path: "Word/__XI", rank: 50_100_005_500, internal: true },
  { path: "Word/__RC", rank: 50_100_007_000, alias: "Returncheck", internal: true },
  { path: "Word/__MK", rank: 50_100_008_000, alias: "Mark", internal: true },
  { path: "Word/__MV", rank: 50_100_009_000, alias: "Move", internal: true },
  { path: "Word/__IN", rank: 50_100_011_000, internal: true },
  { path: "Word/__IN/__DC", rank: 50_100_011_001, internal: true },
  { path: "Word/__SP", rank: 50_100_012_000, alias: "Splice", internal: true },
  { path: "Word/__DM", rank: 50_100_013_000, alias: "DispatchMod", internal: true },
  { path: "Word/__SG", rank: 50_100_014_000, alias: "Sugar", internal: true },

  // Type (metatype) branch.
  { path: "Type/Function", rank: 60_100_000_000 },
  { path: "Type/FunctionSignature", rank: 60_200_000_000 },
  { path: "Type/Disjunct", rank: 60_300_000_000 },
  { path: "Type/Disjunct/Enum", rank: 60_310_000_000 },
  { path: "Type/Negation", rank: 60_400_000_000 },
  { path: "Type/Self", rank: 60_500_000_000 },
  { path: "Type/TypeParam", rank: 60_600_000_000 },
  { path: "Type/GenSpec", rank: 60_700_000_000 },
  { path: "Type/GenParam", rank: 60_800_000_000 },
];

// Derived lookups, built once from builtinDecls.
const ROOTS = new Set<string>();
const BY_PATH = new Set<string>();
// leafIndex maps a leaf/alias name to its full path, or '' when the
// name is ambiguous (declared by more than one path). Mirrors
// TypeTable.leafIndex + ExpandShortName.
const LEAF_INDEX = new Map<string, string>();
// byName maps a user-facing leaf name to its full path (non-internal
// types only). Mirrors TypeTable.byName / TypeNameTable.
const BY_NAME = new Map<string, string>();

/**
 * indexDecl registers one declaration's derived lookups — the path and
 * root sets, the user-facing name, and the leaf/alias short-name index,
 * where a leaf name declared by more than one path collapses to ''
 * (ambiguous: newType's expansion refuses it). The one mutation path
 * for the lookup tables, mirroring TypeTable.RegisterType's index
 * maintenance in Go.
 */
export function indexDecl(d: {
  path: string;
  alias?: string;
  internal?: boolean;
}): void {
  const parts = d.path.split("/");
  const name = parts[parts.length - 1]!;
  BY_PATH.add(d.path);
  if (parts.length === 1) ROOTS.add(d.path);
  // Internal types stay OUT of the user-facing name table. Go's
  // TypeTable.byName is unfiltered, but the lookup stepWord consults is
  // not: `__ED` is undefined_word on both engines, which is what
  // establishes that the filter belongs here. (Measured — removing it
  // made core/ts resolve `__ED` to the End marker and render nothing.)
  if (!d.internal) BY_NAME.set(name, d.path);

  if (LEAF_INDEX.has(name)) {
    if (LEAF_INDEX.get(name) !== "") LEAF_INDEX.set(name, "");
  } else {
    LEAF_INDEX.set(name, d.path);
  }
  if (d.alias) LEAF_INDEX.set(d.alias, d.path);
}

for (const d of builtinDecls) indexDecl(d);

/**
 * The builtin declaration table, read-only — path, rank, alias and the
 * internal flag of every kernel lattice node, in declaration order. Exposed
 * so a test can hold it against core/go/typetable.go row by row: the table
 * is DATA the Go kernel owns, and a port that restates it by hand drifts
 * (it was thirteen nodes short when that test was written).
 */
export function builtinTypeDecls(): ReadonlyArray<Readonly<BuiltinDecl>> {
  return builtinDecls;
}

export class BoruType {
  readonly parts: readonly string[];

  constructor(parts: readonly string[]) {
    this.parts = parts;
  }

  /** True iff this type satisfies (conforms to) the given pattern. */
  matches(pattern: BoruType): boolean {
    if (pattern.parts.length === 1 && pattern.parts[0] === "Any") return true;
    return this.pathSubtype(pattern);
  }

  /** Strict path-prefix subtype: every part of pattern is a part of this at the same index. */
  pathSubtype(pattern: BoruType): boolean {
    if (this.parts.length < pattern.parts.length) return false;
    for (let i = 0; i < pattern.parts.length; i++) {
      if (this.parts[i] !== pattern.parts[i]) return false;
    }
    return true;
  }

  equal(other: BoruType): boolean {
    if (this.parts.length !== other.parts.length) return false;
    for (let i = 0; i < this.parts.length; i++) {
      if (this.parts[i] !== other.parts[i]) return false;
    }
    return true;
  }

  specificity(): number {
    return this.parts.length;
  }

  toString(): string {
    return this.parts.join("/");
  }

  leaf(): string {
    return this.parts[this.parts.length - 1] ?? "";
  }
}

/**
 * Build a Type from a slash-separated path. Short names like "Integer"
 * expand to their full hierarchy ("Scalar/Number/Integer") via the
 * leaf index, matching the Go engine's NewType + ExpandShortName.
 */
export function newType(path: string): BoruType {
  const rawParts = path.split("/");
  for (const p of rawParts) {
    // A LETTER that is not uppercase, as Go's NewType reads it
    // (unicode.IsLetter && !unicode.IsUpper): `é` is refused exactly as `e`
    // is, and a digit or `_` opens a part freely (`Word/__ED`).
    const ch = String.fromCodePoint(p.codePointAt(0) ?? 0x30);
    if (/\p{L}/u.test(ch) && !/\p{Lu}/u.test(ch)) {
      throw new Error(
        `boru: type part ${JSON.stringify(p)} in ${JSON.stringify(path)} must start with an uppercase letter`,
      );
    }
  }

  let parts = rawParts;
  const head = parts[0]!;
  if (!ROOTS.has(head) && !BY_PATH.has(path)) {
    const expanded = LEAF_INDEX.get(head);
    if (expanded !== undefined && expanded !== "") {
      const tail = parts.slice(1).join("/");
      const full = tail.length > 0 ? `${expanded}/${tail}` : expanded;
      parts = full.split("/");
    }
  }
  return new BoruType(parts);
}

// builtinRank returns a type's position in builtinDecls (parent-first
// order), which mirrors eng/go's positional lattice Rank closely enough
// to reproduce the Go engine's canonical disjunct ordering for the spec
// corpus. Types absent from the table sort last. Used to keep `tor`
// unions in the same tcmp order the Go kernel stores them in, so the
// cross-engine differential agrees.
const RANK_BY_PATH = new Map<string, number>(
  builtinDecls.map((d, i) => [d.path, i]),
);
export function builtinRank(t: BoruType): number {
  const r = RANK_BY_PATH.get(t.parts.join("/"));
  return r === undefined ? Number.MAX_SAFE_INTEGER : r;
}

const LATTICE_RANK = new Map<string, number>(
  builtinDecls.map((d) => [d.path, d.rank]),
);

/**
 * typeRank is a type's unified lattice Rank: its own when the kernel
 * declares it, else the nearest declared ancestor's — a minted type has no
 * positional slot and inherits its parent's (core/go/compare_types.go
 * `rankOf`). Zero for a type with no declared ancestor at all.
 */
export function typeRank(t: BoruType): number {
  for (let n = t.parts.length; n > 0; n--) {
    const r = LATTICE_RANK.get(t.parts.slice(0, n).join("/"));
    if (r !== undefined) return r;
  }
  return 0;
}

/**
 * compareTypes is the total order on types — the order two bare type
 * literals sort in, and what the value order falls back to across
 * families (core/go/compare_types.go `compareTypes`): Rank, then depth
 * (the more general type first), then leaf name.
 */
export function compareTypes(a: BoruType, b: BoruType): number {
  if (a.equal(b)) return 0;
  const byRank = typeRank(a) - typeRank(b);
  if (byRank !== 0) return byRank < 0 ? -1 : 1;
  const byDepth = a.parts.length - b.parts.length;
  if (byDepth !== 0) return byDepth < 0 ? -1 : 1;
  return compareText(a.leaf(), b.leaf());
}

/**
 * compareText orders two strings by CODE POINT, which is the order Go's
 * byte-wise strings.Compare gives UTF-8 text. JavaScript's own `<` compares
 * UTF-16 code units, and so puts an astral character (U+1F600, a surrogate
 * pair opening 0xD83D) BEFORE U+E000..U+FFFF where Go puts it after.
 */
export function compareText(a: string, b: string): number {
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i++) {
    const x = a.charCodeAt(i);
    const y = b.charCodeAt(i);
    if (x === y) continue;
    // Two differing units order as their code points do unless exactly one
    // of them is a surrogate: that one is half of an astral character, and
    // so the greater, whatever the unit values say.
    const xs = x >= 0xd800 && x <= 0xdfff;
    const ys = y >= 0xd800 && y <= 0xdfff;
    if (xs !== ys) return xs ? 1 : -1;
    return x < y ? -1 : 1;
  }
  return a.length === b.length ? 0 : a.length < b.length ? -1 : 1;
}

// Well-known types. Mirrors the var block in eng/go/types.go.
export const TAny = newType("Any");
export const TNone = newType("None");
export const TNever = newType("Never");
export const TAbsent = newType("Absent");
export const TScalar = newType("Scalar");
export const TString = newType("Scalar/String");
export const TStringProper = newType("Scalar/String/ProperString");
export const TStringEmpty = newType("Scalar/String/EmptyString");
export const TNumber = newType("Scalar/Number");
export const TInteger = newType("Scalar/Number/Integer");
export const TFloat = newType("Scalar/Number/Float");
export const TBigInteger = newType("Scalar/Number/BigInteger");
export const TBigDecimal = newType("Scalar/Number/BigDecimal");
export const TBoolean = newType("Scalar/Boolean");
export const TMicron = newType("Scalar/Micron");
export const TPathon = newType("Scalar/Micron/Pathon");
export const TEmailon = newType("Scalar/Micron/Emailon");
export const TUrlon = newType("Scalar/Micron/Urlon");
export const TAtom = newType("Scalar/Atom");
export const TNode = newType("Node");
export const TIdeal = newType("Ideal");
export const TList = newType("Node/List");
export const TMap = newType("Node/Map");
export const TInspect = newType("Node/Map/Inspect");
export const TXml = newType("Node/Xml");
export const TClass = newType("Ideal/Class");
export const TRecord = newType("Ideal/Record");
export const TOptions = newType("Ideal/Options");
export const TError = newType("Ideal/Error");
export const TStore = newType("Ideal/Store");
export const TTable = newType("Ideal/Table");
export const TReach = newType("Ideal/Reach");
export const TWord = newType("Word");
export const TType = newType("Type");
export const TFunction = newType("Type/Function");
export const TFnUndef = newType("Type/FunctionSignature");
export const TDisjunct = newType("Type/Disjunct");
export const TEnum = newType("Type/Disjunct/Enum");
export const TNegation = newType("Type/Negation");
export const TForward = newType("Word/__FW");
export const TOpenParen = newType("Word/__OP");
export const TCloseParen = newType("Word/__CP");
export const TEnd = newType("Word/__ED");
export const TMark = newType("Word/__MK");
export const TMove = newType("Word/__MV");
export const TSplice = newType("Word/__SP");
export const TDispatchMod = newType("Word/__DM");
export const TSugar = newType("Word/__SG");
export const TInterpString = newType("Word/__IS");
export const TParenExpr = newType("Word/__PE");
export const TXmlInterp = newType("Word/__XI");

/** Back-compat alias: the decimal scalar is named Float in the lattice. */
export const TDecimal = TFloat;

/**
 * Return the canonical lookup table of well-known type names. Mirrors
 * `eng.TypeNameTable()` — the user-facing leaf name → Type map the
 * parser and fn-param resolution consult for bare type-name words.
 */
/**
 * Resolve a bare type-name WORD, mirroring Go's ResolveBuiltinTypeName
 * (core/go/resolve.go:23): the user-facing LEAF table first, then the
 * full PATH form. core/ts had only the leaf half, so `Scalar/String` and
 * `Word/__ED` were undefined_word here and resolved there.
 *
 * The path arm admits only paths the builtin table actually declares —
 * Go's ResolveTypePath checks `Builtin.bypath[t.Path()] == t` — so an
 * invented path stays undefined rather than minting a type.
 */
export function resolveBuiltinTypeName(name: string): BoruType | undefined {
  const leaf = typeNameTable().get(name);
  if (leaf !== undefined) return leaf;
  if (!name.includes("/")) return undefined;
  // The path arm is NewType's: the first part may be a SHORT name
  // (`Number/Integer`, `FlexMap/WeakFlexMap`), expanded through the leaf
  // index, and a part opening with a lowercase letter is no type path at
  // all. core/ts tested the name as written against the declared paths, so
  // only the fully spelled form resolved.
  let t: BoruType;
  try {
    t = newType(name);
  } catch {
    return undefined;
  }
  return BY_PATH.has(t.toString()) ? t : undefined;
}

export function typeNameTable(): Map<string, BoruType> {
  const m = new Map<string, BoruType>();
  for (const [name, path] of BY_NAME) {
    m.set(name, newType(path));
  }
  return m;
}
