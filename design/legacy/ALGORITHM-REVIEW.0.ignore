# Algorithm review and upgrade recommendations

**Status:** design report / review note. No code changes are proposed here.
This report was prepared from a source read of the Go implementation on the
current branch after the policy, string, dispatch, ordered-map, capture, and
query paths were reviewed.

## Summary

boru's core algorithms are generally straightforward and correct-first. The
highest-value improvements are not broad rewrites; they are targeted changes
where current implementations do repeated search work in hot or policy-sensitive
paths:

1. Memoize or compile policy glob matching.
2. Replace shell-pattern substring search with an automaton-style matcher.
3. Add a guarded signature-dispatch cache for hot words.
4. Add internal ordered-map iteration helpers to avoid defensive-copy churn.
5. Optimize Unicode-safe case-insensitive literal search without breaking byte
   offsets.
6. Consider fusing closure-capture body scans only after profiling.
7. Leave query column parsing mostly as-is unless the grammar grows.

## Findings

### 1. Policy glob matching

**Where:** `lang/go/policy/glob.go`.

The policy glob matcher supports literals, `?`, `*`, and `**`. It is used by
the permissions evaluator to match allow/deny rule names and string-valued
`where` predicates. The implementation is a recursive matcher that branches on
star tokens and tries candidate suffixes.

**Risk:** For today's short policy names this is fine. If users start supplying
larger policies, path-shaped names, or adversarial patterns, repeated suffix
exploration can revisit the same pattern/input state.

**Recommendation:** Replace recursive substring matching with an index-based
`match(pi, si)` routine and memoize `(pi, si)` results. If policies become more
static and high-volume, compile each glob pattern to a tiny automaton at policy
compile time.

**Expected benefit:** Predictable worst-case behavior in security-sensitive
checks, fewer substring operations, and a better foundation for larger policies.

### 2. Shell-pattern string search

**Where:** `lang/go/native/native_string_helpers.go`.

The shell search path currently finds occurrences by trying every candidate
substring and invoking `filepath.Match` on each one.

**Risk:** This is easy to understand but scales poorly: a long input with a
broad pattern may enumerate many substrings and repeatedly re-run the pattern
matcher.

**Recommendation:** Compile shell patterns to a small NFA/DP matcher and scan
the input while tracking accepting spans. Add literal-only and simple-prefix
fast paths before the general matcher.

**Expected benefit:** Better scaling for `match`/string search use cases and
less risk of surprising latency on large text.

### 3. Signature dispatch

**Where:** `eng/go/signature.go`.

Signature matching scans sorted signatures and returns the first compatible
match. This is semantically clean and preserves the existing overload ordering.
The cost is linear in the number of candidate signatures, plus type and pattern
unification cost.

**Risk:** Repeated hot calls with the same top-level argument types pay the same
scan cost over and over.

**Recommendation:** Add a versioned dispatch cache keyed by word/signature-set,
argument count, modifiers, and top-level argument type identities. For value
patterns, either cache only a candidate range or mark such signatures as
non-final-cacheable unless the pattern has been included safely in the key.

**Expected benefit:** Faster loops and native-word hot paths without changing
first-match-wins semantics.

### 4. Ordered-map key iteration

**Where:** `eng/go/value.go`, with callers in map unification and deep equality.

`OrderedMap.Keys()` returns a defensive copy. That is the right public API, but
internal algorithms often immediately iterate over the copy and discard it.

**Risk:** Large map comparisons or repeated unification can allocate avoidably.

**Recommendation:** Add an internal `Range` helper or explicitly documented
`KeysView` for trusted internal iteration. Keep the defensive `Keys()` API for
external callers.

**Expected benefit:** Lower allocation pressure in structural matching and deep
equality while preserving ordered-map semantics.

### 5. Unicode-safe case-insensitive literal search

**Where:** `lang/go/native/native_string.go`.

The current implementation correctly preserves byte offsets into the original
string by comparing under simple Unicode folding instead of lowercasing the
whole string and slicing with shifted offsets.

**Risk:** Correctness is good, but long inputs and repeated all-match searches
can approach `O(n*m)` behavior.

**Recommendation:** Keep the current implementation as the correctness baseline.
Add ASCII-only fast paths, and for the general Unicode case consider pre-decoded
rune/byte-offset buffers with a KMP-style folded-rune search.

**Expected benefit:** Faster search while retaining valid original-string byte
spans.

### 6. Closure-capture analysis

**Where:** `eng/go/fn_capture.go`.

Capture computation first collects body-local definitions and then walks body
words to decide which names come from an enclosing function scope.

**Risk:** Two traversals are simple and maintainable, but generated or very
large bodies pay repeated recursive walk cost.

**Recommendation:** Do not rewrite immediately. If benchmarks show closure
construction matters, fuse local-definition and word-reference collection into a
single traversal, preserving the current skip rules for quoted data and nested
function definitions.

**Expected benefit:** Lower construction overhead for large generated functions,
with the caveat that this path is correctness-sensitive.

### 7. Query column parsing

**Where:** `lang/go/native/query.go`.

Column specs are parsed by direct shape checks for atoms, strings, words, alias
pairs, aggregates, casts, and scalar subqueries.

**Risk:** Low. The current approach is readable and proportionate to the grammar
size.

**Recommendation:** Keep it. Move to a table-driven parser only if the query
mini-language grows enough that the direct switch becomes hard to audit.

## Suggested benchmark plan

Before implementing the higher-risk recommendations, add focused benchmarks:

- policy glob: repeated `*`/`**`, exact matches, and no-match cases;
- shell search: large input with broad pattern, narrow pattern, and no match;
- signature dispatch: many overloads, structural map patterns, and primitive hot
  calls;
- ordered maps: large-map deep equality and open map unification;
- closure capture: large generated function bodies with nested lists and
  interpolated expressions.

## Implementation order

1. Policy glob memoization.
2. Shell-pattern matcher rewrite.
3. Signature-dispatch cache prototype behind benchmarks.
4. Ordered-map internal iteration helper.
5. Unicode folded-search fast paths.
6. Closure-capture traversal fusion only if profiling warrants it.
7. Query parser refactor only if the grammar expands.
