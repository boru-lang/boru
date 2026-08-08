// Public entry point for @boru-lang/core — the TypeScript interpreter core.
//
// The TS twin of the core/go module (design/ENG-FOUR-PIECE.0.md): values,
// types, signatures, matching, the registry, and the step loop. What is NOT
// here is the point of the package — no check pass, no compiler, no VM, no
// parser, and no dependencies at all.
//
// core/go's rule applies verbatim: NO UPWARD IMPORTS. The check and compiler
// pieces reach core only through the seam tables core itself owns —
// AnalysisImpl (analysis-hooks.ts) and EmitRecorder (emit-recorder.ts) — each
// with a NAMED inactive default that a core-only build runs and a core-side
// test pins. Here that rule is enforced structurally rather than by
// convention: this package declares no dependency on @boru-lang/eng, so a
// core file reaching upward simply fails to resolve.

// The core surface is exported WHOLE, deliberately. The curated list this
// replaced was inherited from eng/ts, where consumers could reach past the
// barrel into './value.ts' directly; across a package boundary they cannot,
// so anything a consumer legitimately needs has to be on the barrel. A
// narrower list would only push callers back to deep imports, which is the
// coupling the package boundary exists to prevent.

export * from './error.ts'
export * from './type.ts'
export * from './decimal.ts'
export * from './value.ts'
export * from './canon.ts'
export * from './coretype.ts'
export * from './signature.ts'
export * from './match.ts'
export * from './registry.ts'
export * from './resolve.ts'
export * from './sugar.ts'
export * from './make.ts'
export * from './capability.ts'
export * from './engine.ts'
export * from './check-state.ts'
export * from './analysis-hooks.ts'
export * from './emit-recorder.ts'
