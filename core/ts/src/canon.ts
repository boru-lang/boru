// Canon renders values as canonical boru source — the string a spec row's
// `expected` column is compared against. Ported from eng/go/canon.go +
// eng/go/value.go::FormatFloat so the TS engine's output matches the Go
// reference row-for-row.
//
// PARITY NOTE: this covers the value kinds the eng/spec corpus reaches
// through the current TS engine (none, type literals, scalars, atoms,
// lists, fn defs). Map / BigInteger / Reach / Flex / DepScalar branches
// are added by their owning port increments.
import {
  TAtom,
  TBoolean,
  TDispatchMod,
  TFloat,
  TInspect,
  TInteger,
  TList,
  TMap,
  TPathon,
  TString,
  TEmailon,
  TUrlon,
} from "./type.ts";
import { Decimal } from "./decimal.ts";
import { urlonHref, type UrlonInfo } from "./value.ts";
import type { DispatchModInfo, FnDefInfo, SugarInfo, WordInfo, XmlElement } from "./value.ts";
import {
  ChildType,
  ErrorInfo,
  OptionsData,
  OrderedMap,
  Value,
  asReach,
  asSugar,
  isEnd,
  isReach,
  isSugar,
} from "./value.ts";

// canonXml renders an XML element, normalising an empty element to the
// self-closing form (<br></br> → <br/>).
// XML entity re-escaping for the canonical render — text and attribute
// values are stored DECODED (the parser runs unescapeXml), so rendering
// must re-escape. Mirrors eng/go/core_xml.go's xmlTextEscaper /
// xmlAttrEscaper exactly.
function escapeXmlText(s: string): string {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function escapeXmlAttr(s: string): string {
  return escapeXmlText(s).replaceAll('"', "&quot;");
}

export function canonXml(e: XmlElement): string {
  const attrs = e.attrs
    .map((a) => ` ${a.name}="${escapeXmlAttr(a.value)}"`)
    .join("");
  if (e.children.length === 0) return `<${e.tag}${attrs}/>`;
  const body = e.children
    .map((c) => (typeof c === "string" ? escapeXmlText(c) : canonXml(c)))
    .join("");
  return `<${e.tag}${attrs}>${body}</${e.tag}>`;
}

/**
 * canonString renders a string payload as parseable boru source. Plain
 * content uses single quotes; content with a single quote switches to
 * double quotes; content with both quote kinds, backslashes, or control
 * characters falls back to single quotes with backslash escapes.
 * Mirrors eng/go/canon.go::canonString.
 */
export function canonString(s: string): string {
  const hasSingle = s.includes("'");
  const hasEscape = /[\\\n\t\r]/.test(s);
  if (!hasSingle && !hasEscape) return `'${s}'`;
  if (!s.includes('"') && !hasEscape) return `"${s}"`;
  let b = "'";
  for (const r of s) {
    switch (r) {
      case "\\":
        b += "\\\\";
        break;
      case "'":
        b += "\\'";
        break;
      case "\n":
        b += "\\n";
        break;
      case "\t":
        b += "\\t";
        break;
      case "\r":
        b += "\\r";
        break;
      default:
        b += r;
    }
  }
  return b + "'";
}

/**
 * formatFloat renders a float with a guaranteed decimal point so it
 * stays visually distinct from Integer, using the shortest round-trip
 * representation. Mirrors eng/go/value.go::FormatFloat.
 */
export function formatFloat(f: number): string {
  if (Number.isNaN(f)) return "nan";
  if (f === Infinity) return "inf";
  if (f === -Infinity) return "-inf";
  // Negative zero is a distinct bit pattern (IEEE) and renders as -0.0.
  if (f === 0 && 1 / f === -Infinity) return "-0.0";

  if (f !== 0) {
    const a = Math.abs(f);
    if (a >= 1e21 || a < 1e-10) {
      return formatExponential(f);
    }
  }
  let s = toFixedShortest(f);
  if (!/[.eE]/.test(s)) s += ".0";
  return s;
}

// toFixedShortest renders a float in plain decimal ('f') form with the
// shortest precision that round-trips — the equivalent of Go's
// strconv.FormatFloat(f, 'f', -1, 64). JS's String() uses 'g'-style
// formatting (switching to exponent for large/small values), so coerce
// any exponent form back to plain decimal for the everyday range that
// FormatFloat keeps non-scientific.
function toFixedShortest(f: number): string {
  const s = String(f);
  if (!/[eE]/.test(s)) return s;
  // Expand the JS exponential form to plain decimal.
  return expandExponential(s);
}

// formatExponential renders the Go strconv 'e' form: a mantissa with a
// sign-and-at-least-two-digit exponent (e.g. 1e+21 → "1e+21").
function formatExponential(f: number): string {
  let s = f.toExponential();
  // JS gives "1e+21"; Go gives "1e+21" too. Normalise the exponent to a
  // leading sign (JS already includes it) — no zero-padding needed to
  // match Go's -1 precision 'e' form for the magnitudes the specs use.
  // Drop a trailing ".0"-style mantissa? Go keeps the shortest mantissa.
  s = s.replace(/e([+-])(\d)$/, "e$10$2");
  return s;
}

// expandExponential turns a JS exponential literal into plain decimal.
function expandExponential(s: string): string {
  const neg = s.startsWith("-");
  if (neg) s = s.slice(1);
  const m = /^(\d+)(?:\.(\d+))?[eE]([+-]?\d+)$/.exec(s);
  if (!m) return (neg ? "-" : "") + s;
  const intPart = m[1]!;
  const fracPart = m[2] ?? "";
  const exp = Number.parseInt(m[3]!, 10);
  const digits = intPart + fracPart;
  const pointPos = intPart.length + exp;
  // The decimal point always lands at or left of the digits here:
  // formatFloat routes |f| >= 1e21 through formatExponential, and below
  // that JS's String only goes exponential for |f| < 1e-6 (negative
  // exp). A positive pointPos would make the repeat throw — loudly.
  let out = "0." + "0".repeat(-pointPos) + digits;
  // Trim a trailing fractional zero run produced by the expansion.
  if (out.includes(".")) out = out.replace(/0+$/, "").replace(/\.$/, "");
  return (neg ? "-" : "") + out;
}

/**
 * formatBigInteger renders a BigInteger as the parseable literal `0d…`,
 * sign BEFORE the marker — Go's FormatBigInteger (core/go/value.go).
 * `-0d789`, not `0d-789`: the marker introduces the digits, so the sign
 * has to lead or the literal does not parse back.
 */
export function formatBigInteger(n: bigint): string {
  if (n < 0n) return "-0d" + (-n).toString();
  return "0d" + n.toString();
}

/**
 * formatBigDecimal renders a BigDecimal as the parseable literal `0d…`,
 * mirroring Go's FormatBigDecimal — apd's plain 'f' form with the sign
 * before the marker.
 *
 * Exact at every magnitude and scale since the payload became a Decimal
 * (decimal.ts): `0d1e400`, `0d1e-400` and `0d0.30` all round-trip, where
 * the previous binary64 payload rendered `0dInfinity`, `0d0` and `0d0.3`.
 */
export function formatBigDecimal(d: Decimal): string {
  const s = d.toString();
  return s.startsWith("-") ? "-0d" + s.slice(1) : "0d" + s;
}

/** Canon renders a stack of values as canonical boru source. */
export function canon(stack: Value[]): string {
  // The sequence rule (canonSeqParts): a group-modifier marker is spelled
  // after its group — core/go's CanonValues (NUR072). A result stack holds
  // no marker, so an evaluated stack renders exactly as before.
  return canonSeqParts(stack, canonValue).join(" ");
}

/** canonValue renders one value as canonical boru source. */
export function canonValue(v: Value): string {
  // The `none` value renders lowercase; a bare type literal (including
  // the `None` type) renders as its user-facing leaf name.
  if (v.isNone()) return "none";
  if (v.data === null) {
    return v.vType.leaf();
  }
  // The `/v` / `/q` group modifier the parser emits AFTER a paren or
  // dotted-path group. Canon had no arm for it, so each engine fell through
  // to a DIFFERENT debug spelling — TS to `word(undefined)` (the word arm
  // reading a DispatchModInfo as a WordInfo) and Go to
  // `word()({false true})`. Neither is source, and they disagreed, which is
  // what the parity probe caught on `m.k/q`.
  if (v.vType.equal(TDispatchMod)) {
    return (v.data as DispatchModInfo).val ? "/v" : "/q";
  }
  // A caught-error VALUE (the do escape hatch) — mirrors Go's render.
  if (v.data instanceof ErrorInfo) {
    return `error(${v.data.message})`;
  }
  if (v.vType.matches(TInteger)) {
    return v.asInteger().toString();
  }
  if (v.vType.matches(TFloat)) {
    return formatFloat(v.asFloat());
  }
  if (v.vType.matches(TString)) {
    return canonString(v.asString());
  }
  if (v.vType.matches(TBoolean)) {
    return v.asBoolean() ? "true" : "false";
  }
  if (v.vType.equal(TAtom)) {
    return `${v.asAtom()}/q`;
  }
  if (
    v.vType.equal(TPathon) &&
    v.data !== null &&
    typeof v.data === "object" &&
    "segments" in v.data
  ) {
    const p = v.data as { segments: string[]; abs: boolean };
    return (p.abs ? "/" : "") + p.segments.join("/");
  }
  if (
    v.vType.equal(TEmailon) &&
    v.data !== null &&
    typeof v.data === "object" &&
    "user" in v.data
  ) {
    const e = v.data as { user: string; host: string };
    return `${e.user}@${e.host}`;
  }
  if (
    v.vType.equal(TUrlon) &&
    v.data !== null &&
    typeof v.data === "object" &&
    "scheme" in v.data
  ) {
    return urlonHref(v.data as UrlonInfo);
  }
  if (v.data instanceof ChildType) {
    const ct = v.data;
    const open = v.isTypedMap() ? "{" : "[";
    const close = v.isTypedMap() ? "}" : "]";
    // A typed MAP carries its concrete pairs in `entries`, not `elements`,
    // and canon rendered only the latter — so `{:Integer a:1}` came out
    // `{:Integer}` with the entries silently gone. Both are rendered here,
    // mirroring Value.toString and core/go/canon.go's ChildTypeInfo arm.
    const parts = [
      `:${canonTypeTag(ct.child)}`,
      ...ct.elements.map(canonValue),
      ...ct.entries.map((e) => `${e.key}:${canonChild(e.value)}`),
    ];
    return `${open}${parts.join(" ")}${close}`;
  }
  if (v.vType.matches(TList) && Array.isArray(v.data)) {
    const body = `[${canonSeqParts(v.asList(), canonChild).join(" ")}]`;
    return v.quoted ? `(quote ${body})` : body;
  }
  if (v.data instanceof OptionsData) {
    const m = v.data.map;
    // D1: render in INSERTION order (design/FLEX-ATTRS.1.md §3), matching
    // Go's joinEntries which iterates Keys(). SortedKeys() stays for
    // order-insensitive equality only (coretype.ts valuesEqual).
    const parts = m.keys().map((k) => `${k}:${canonValue(m.get(k)!)}`);
    return `options{${parts.join(" ")}}`;
  }
  if (v.vType.equal(TMap) && v.data instanceof OrderedMap) {
    const m = v.data;
    const parts = m.keys().map((k) => `${k}:${canonChild(m.get(k)!)}`);
    return `{${parts.join(" ")}}`;
  }
  // An inspection map renders in insertion order with bare word values
  // (e.g. `kind:native`), unlike a plain map.
  if (v.vType.equal(TInspect) && v.data instanceof OrderedMap) {
    const m = v.data;
    const parts = m.keys().map((k) => {
      const val = m.get(k)!;
      const rendered = val.isWord() ? val.asWord().name : canonValue(val);
      return `${k}:${rendered}`;
    });
    return `{${parts.join(" ")}}`;
  }
  if (v.isXml()) {
    return canonXml(v.data as XmlElement);
  }
  if (isReach(v)) {
    return canonReach(v);
  }
  if (v.isWord()) {
    // A word renders as its NAME plus any `/`-modifier suffix — the source
    // that re-parses to this word value (ADR-015), the Go twin exactly. A
    // plain word used to keep the `word(foo)` spelling, which re-parses as
    // the `word` splice over a group and so was never source; bare `foo`
    // re-parses to this Word, and whether it is later dispatched is
    // evaluation, which canon does not model (NUR072).
    const w = v.asWord();
    return w.name + canonWordModifiers(w);
  }
  if (isSugar(v)) {
    // Sugar markers have a surface spelling of their own; without one they
    // fell through to the debug dump `sugar(angle Box [word(…)])` (NUR059).
    const info = asSugar(v);
    if (info !== undefined) {
      const out = canonSugar(info);
      if (out !== null) return out;
    }
    return v.toString();
  }
  if (v.isParenExpr() && Array.isArray(v.data)) {
    // `(1 add 2)` rather than `paren([1 word(add) 2])` (NUR059). The body
    // renders through canonReachTokens, which keeps words bare — inside a
    // group they are CODE, not atom data; an arrow fold renders bare
    // (canonParen, NUR072).
    return canonParen(v.data as Value[]);
  }
  if (v.isFnDef()) {
    return canonFnDef(v.asFnDef());
  }
  // Disjunct / Enum: alternatives joined by ' tor ' — the word form, so
  // the rendering re-reads as source (`tor` is commutative). Atoms render
  // bare, type literals as their leaf. A TOP-LEVEL union is left ungrouped
  // (it re-parses as-is); a union nested in a container is grouped by
  // canonChild so it does not fragment into extra tokens.
  if (v.isDisjunct()) {
    return v
      .asDisjunct()
      .alternatives.map((a) =>
        a.vType.equal(TAtom) && a.data !== null
          ? a.asAtom()
          : a.data === null
            ? a.vType.leaf()
            : canonValue(a),
      )
      .join(" tor ");
  }
  return v.toString();
}

// canonChild renders a value that sits INSIDE a composite canon (a map value,
// a list element, a record/childtype field). A union's `A tor B` renders with
// whitespace, so concatenated into a container it fragments into extra
// word-separated tokens (`{x:Integer tor String}` reads as broken map syntax)
// and breaks the source round-trip; grouping it in parens keeps the compound
// canon re-parseable (`{x:(Integer tor String)}`). A top-level union is left
// ungrouped by canonValue — it re-parses as-is and is a comparison ordering
// key. Mirrors eng/go/canon.go::canonChild.
function canonChild(v: Value): string {
  return v.isDisjunct() ? `(${canonValue(v)})` : canonValue(v);
}

// canonTypeTag renders a container's element-type tag in SOURCE form. The
// parser is type-name-opaque (ADR-012 rule 4), so the tag on an unevaluated
// literal is still a bare Word — `[:Integer]` holds word(Integer), not the
// Integer type node — and rendering it through the generic value path leaks
// the debug spelling `word(Integer)` into what is meant to be source.
// Mirrors core/go/canon.go canonTypeTag.
function canonTypeTag(v: Value): string {
  // The modifier suffix rides along (NUR059): an angle argument can be a
  // modifier-bearing word (`Box<a/v>`), and emitting the bare name would
  // drop it — the same silent loss one level down.
  if (v.isWord()) {
    const w = v.asWord();
    return w.name + canonWordModifiers(w);
  }
  return canonChild(v);
}

// canonFnDef renders a function value's discriminating canonical form.
// The TS FnDefInfo carries a single params/returns/body triple (the
// spec subset), so this renders that one signature. Mirrors the shape
// of eng/go/canon.go::canonFnDef for a single authored sig.
function canonFnDef(fd: FnDefInfo): string {
  const sigs = fd.sigs
    .map((s) => {
      const params = s.params
        .map((p) => `${p.name}:${p.type.toString()}`)
        .join(" ");
      const returns = s.returns.map((r) => r.toString()).join(" ");
      const body = canon(s.body);
      return `[${params}][${returns}][${body}]`;
    })
    .join(" ");
  return `fn [${sigs}]`;
}

// canonReach renders a Reach back to its dotted surface — m.a.b, m!.x,
// m.'k', m.(expr), (expr).k — the read-print round-trip, mirroring
// eng/go/canon.go::canonReach (words stay bare; a codequote-captured
// reach wraps so it round-trips).
function canonReachToken(v: Value): string {
  // Bare NAME plus any modifier suffix (NUR059). A reach SEGMENT never
  // carries one — the parser peels a trailing `/mod` off the final key and
  // applies it to the whole reach — so segments render exactly as before.
  // A word inside a PAREN body can carry one, and `(x/v)` losing its `/v`
  // would be the same defect this record removes.
  if (v.isWord()) {
    const w = v.asWord();
    return w.name + canonWordModifiers(w);
  }
  if (v.isParenExpr() && Array.isArray(v.data)) return canonParen(v.data as Value[]);
  if (isReach(v)) return canonReach(v);
  // The end marker renders as `;` inside a reach, never as `end`. A bare
  // `end` in reach-token position is a WORD — a field name in a segment, an
  // ordinary word in a paren body (NUR066) — so spelling the marker `end`
  // here would re-parse as that word and silently change the program. `;`
  // re-parses to this marker.
  if (isEnd(v)) return ";";
  return canonValue(v);
}

function canonReachTokens(toks: Value[]): string {
  return canonSeqParts(toks, canonReachToken).join(" ");
}

// canonParen renders a paren group's tokens as source — the Go twin's rule:
// a group of exactly `A => B` is the arrow's FOLD and renders without
// parens (it re-folds into this group wherever it is re-read), every other
// group keeps them (NUR072).
function canonParen(toks: Value[]): string {
  if (isLambdaFold(toks)) return canonReachTokens(toks);
  return "(" + canonReachTokens(toks) + ")";
}

function isLambdaFold(toks: Value[]): boolean {
  if (toks.length !== 3 || !isSugar(toks[1]!)) return false;
  return asSugar(toks[1]!)?.kind === "lambda";
}

// canonSeqParts renders a value SEQUENCE — a list's elements, a paren body,
// a parsed stream — the Go twin's rule: a group-modifier marker (`/u /s /f
// /N` on a paren or reach group) precedes its group in the stream and is
// spelled as the standalone modifier token AFTER the group — `(1 2) /s` —
// which re-parses to the same pair (NUR072).
function canonSeqParts(vals: Value[], render: (v: Value) => string): string[] {
  const parts: string[] = [];
  for (let i = 0; i < vals.length; i++) {
    const mod = groupModifierText(vals[i]!);
    if (mod !== null && i + 1 < vals.length) {
      parts.push(render(vals[i + 1]!) + " /" + mod);
      i++;
      continue;
    }
    parts.push(render(vals[i]!));
  }
  return parts;
}

function groupModifierText(v: Value): string | null {
  if (!isSugar(v)) return null;
  const info = asSugar(v);
  switch (info?.kind) {
    case "usurp":
      return "u";
    case "stack-args":
      return "s";
    case "forward-args":
      return "f";
    case "force-arity":
      return String(info.n ?? 0n);
  }
  return null;
}


function canonReach(v: Value): string {
  const info = asReach(v);
  if (info === undefined) return String(v);
  let b: string;
  if (info.receiver.length === 0) b = "$";
  else if (info.receiver.length === 1) b = canonReachToken(info.receiver[0]!);
  else b = "(" + canonReachTokens(info.receiver) + ")";
  for (const seg of info.segments) {
    b += seg.getr ? "!." : ".";
    if (seg.computed) b += "(" + canonReachTokens(seg.keyExpr ?? []) + ")";
    else b += canonReachToken(seg.keyLit!);
  }
  if (v.quoted) return "(codequote " + b + ")";
  return b;
}

/**
 * Render a concrete value as its scalar TEXT — the plain form, as against
 * canon's re-readable one. Mirrors Go ValToString.
 *
 * It lives here rather than with `make` (its first caller) because
 * `coerceBoolean` needs it: a non-String value that RENDERS as "false" is
 * an unresolved boolean literal and keeps its boolean reading. coretype
 * cannot import make — make imports coretype — and rendering is canon's
 * subject anyway.
 */
export function valToString(v: Value): string {
  if (!v.isConcrete()) return v.toString();
  if (v.vType.matches(TString)) return v.asString();
  if (v.vType.equal(TAtom)) return v.asAtom();
  if (v.vType.matches(TFloat)) return formatFloat(v.asFloat());
  if (v.vType.matches(TInteger)) return v.asInteger().toString();
  if (v.vType.matches(TBoolean)) return v.asBoolean() ? "true" : "false";
  if (v.isWord()) return v.asWord().name;
  return v.toString();
}

// canonWordModifiers renders the `/`-suffix a WordInfo carries, or "" when
// it carries none (NUR059).
//
// Canon's contract is that its output re-parses to the same value, and
// these modifiers used to break it SILENTLY rather than loudly: a word with
// a modifier fell through to the debug form, which spells every word
// `word(foo)` — so `foo/v` and `foo/2` both canon'd as `word(foo)` and the
// modifier was not merely mis-spelled but DROPPED.
//
// The letters may be written in any order at the call site, so canon picks
// one CANONICAL order — digits, then f|s, then u, then r — matching the Go
// twin exactly. A stable order also keeps canon a usable sort key.
//
// `/q` is not here: a quoted word is an ATOM by the time it is a value, and
// the Atom arm already renders `name/q`.
function canonWordModifiers(w: WordInfo): string {
  let out = "";
  if (w.argCount !== undefined && w.argCount >= 0n) out += String(w.argCount);
  if (w.forceForward) out += "f";
  else if (w.forceStack) out += "s";
  if (w.forceUsurp) out += "u";
  if (w.forceVal) out += "v";
  return out === "" ? "" : "/" + out;
}

// canonSugar renders a sugar marker back to the surface syntax it came
// from (NUR059, NUR072), or null when the kind has no spelling of its own —
// the Go twin (core/go canon.go) exactly. The modifier kinds have no
// spelling of their own: on a word the suffix rides on the word, and on a
// group the sequence rule (canonSeqParts) spells the marker after it, so a
// marker rendered alone keeps the fallback. The lambda marker is `=>`; a mini literal renders
// `+name'src'` in one canonical delimiter with the lexer's escapes
// (canonMiniSrc) — the delimiter the user wrote is not part of the value;
// the type bound renders `name/t` from the `[name/q]` list its Items hold.
// A marker no source can produce keeps the fallback.
function canonSugar(info: SugarInfo): string | null {
  switch (info.kind) {
    case "angle":
      return info.name + "<" + (info.items ?? []).map(canonTypeTag).join(" ") + ">";
    case "lambda":
      return "=>";
    case "mini": {
      const name = info.name ?? "";
      const src = info.src ?? "";
      if (!/^[a-z][a-z0-9-]*$/.test(name) || /[\n\r]/.test(src)) return null;
      return "+" + name + "'" + canonMiniSrc(src) + "'";
    }
    case "type-bound": {
      const items = info.items ?? [];
      if (items.length === 1 && Array.isArray(items[0]!.data)) {
        const inner = items[0]!.data as Value[];
        if (inner.length === 1 && inner[0]!.vType.equal(TAtom) && typeof inner[0]!.data === "string") {
          return inner[0]!.asAtom() + "/t";
        }
      }
      return null;
    }
  }
  return null;
}

// canonMiniSrc spells a mini literal's source between the canonical `'`
// delimiter with the lexer's own escapes — the Go twin's rule exactly: `\'`,
// `\ ` and `\<tab>` escaped, a backslash doubled wherever the lexer would
// otherwise read an escape (before `'`, `\`, a space or a tab, or at the
// end), every other backslash raw.
function canonMiniSrc(src: string): string {
  let out = "";
  for (let i = 0; i < src.length; i++) {
    const c = src[i]!;
    if (c === "'" || c === " " || c === "\t") {
      out += "\\" + c;
    } else if (c === "\\") {
      const next = src[i + 1];
      out += next === undefined || next === "'" || next === "\\" || next === " " || next === "\t" ? "\\\\" : "\\";
    } else {
      out += c;
    }
  }
  return out;
}
