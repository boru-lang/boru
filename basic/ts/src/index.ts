// @boru-lang/basic — the boru base language layer, the TS twin of the
// basic/go module.
//
// Same dependency rule as its Go twin (ADR-013, as amended 2026-08-08): it
// depends on @boru-lang/core and NOTHING else. A word's analysis half
// belongs in core's carrier vocabulary, not in a dependency on the
// checker.
//
// PORT STATUS: this is the first increment — the stack vocabulary, minus
// the three full-stack words (depth / pick / roll) that need a FullStack
// knob core/ts does not yet have. The rest of basic/go's surface (the
// definition, control-flow and type-generics words, and the predefined
// content types) is not ported yet.

export { stackNatives } from './native-stack.ts'
export { controlNatives } from './native-control.ts'
