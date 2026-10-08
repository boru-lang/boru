// Unit campaign for check-state.ts and registry.ts — the mutable execution
// scopes. design/legacy/CORE-TS-COVERAGE.0.ignore stage 3.
//
// snapshot/restore is the pair worth testing hardest: a speculative
// check-mode compile mutates defs, capabilities, args and check state, then
// decides to fall back, and restore() has to put all four back. The contract
// its doc comment states — function REGISTRATIONS are deliberately shared
// because they are static infrastructure, while the runtime scopes are
// captured — is asserted here in both directions.
//
// The deep-copy property matters as much as the values: snapshot() copies
// each def stack and each args frame, so mutating the live registry after
// snapshotting must not reach into the snapshot.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import {
  CheckState,
  DEFAULT_CHECK_STEP_BUDGET,
  severityFor,
} from "./check-state.ts";
import { Registry } from "./registry.ts";
import { newInteger, newString } from "./value.ts";

describe("severityFor", () => {
  it("maps the known codes to their severities", () => {
    assert.equal(severityFor("arity_mismatch"), "error");
    assert.equal(severityFor("partial_dispatch"), "warning");
    assert.equal(severityFor("mixed_form_call"), "info");
  });

  it("defaults an unmapped code to info", () => {
    assert.equal(severityFor("no_such_code"), "info");
  });
});

describe("CheckState — begin and isActive", () => {
  it("is inactive until begin, and the returned fn disables it", () => {
    const cs = new CheckState();
    assert.equal(cs.isActive(), false);
    const end = cs.begin();
    assert.equal(cs.isActive(), true);
    end();
    assert.equal(cs.isActive(), false);
  });

  it("resets the per-pass state on begin", () => {
    const cs = new CheckState();
    cs.begin();
    cs.addDiagnostic({ code: "arity_mismatch", detail: "d", word: "w" });
    cs.recordDef("a");
    cs.stepCount = 7;
    cs.begin();
    assert.equal(cs.diagnostics.length, 0);
    assert.equal(cs.stepCount, 0);
    assert.equal(cs.budgetTripped, false);
    assert.equal(cs.defsInstalled.size, 0);
  });

  it("carries the default step budget", () => {
    assert.equal(DEFAULT_CHECK_STEP_BUDGET, 500_000);
  });
});

describe("CheckState — diagnostics", () => {
  it("defaults severity from the code and honours an override", () => {
    const cs = new CheckState();
    cs.addDiagnostic({ code: "arity_mismatch", detail: "d", word: "w" });
    assert.equal(cs.diagnostics[0]!.severity, "error");
    cs.addDiagnostic({
      code: "arity_mismatch",
      detail: "d",
      word: "w",
      severity: "info",
    });
    assert.equal(cs.diagnostics[1]!.severity, "info");
  });

  it("marks a diagnostic raised inside a fn body", () => {
    const cs = new CheckState();
    cs.fnBodyDepth = 1;
    cs.addDiagnostic({ code: "arity_mismatch", detail: "d", word: "w" });
    assert.equal(cs.diagnostics[0]!.fnBody, true);
  });

  it("suppresses only the two body-error codes while suppression is armed", () => {
    const cs = new CheckState();
    cs.suppressBodyErrors = 1;
    cs.addDiagnostic({ code: "no_signature", detail: "d", word: "w" });
    cs.addDiagnostic({ code: "undefined_word", detail: "d", word: "w" });
    assert.equal(cs.diagnostics.length, 0);
    // A different error code is NOT suppressed...
    cs.addDiagnostic({ code: "arity_mismatch", detail: "d", word: "w" });
    assert.equal(cs.diagnostics.length, 1);
    // ...nor is a non-error severity of a suppressed code.
    cs.addDiagnostic({
      code: "no_signature",
      detail: "d",
      word: "w",
      severity: "info",
    });
    assert.equal(cs.diagnostics.length, 2);
  });
});

describe("CheckState — unused-def analysis", () => {
  it("warns for an installed name that is never used", () => {
    const cs = new CheckState();
    cs.begin();
    cs.recordDef("a");
    cs.recordDef("b");
    cs.recordUse("b");
    cs.emitUnusedDefDiagnostics();
    assert.equal(cs.diagnostics.length, 1);
    assert.equal(cs.diagnostics[0]!.word, "a");
    assert.equal(cs.diagnostics[0]!.severity, "warning");
  });

  it("ignores an empty name and an underscore-led name", () => {
    const cs = new CheckState();
    cs.begin();
    cs.recordDef("");
    cs.recordDef("_private");
    cs.emitUnusedDefDiagnostics();
    assert.equal(cs.diagnostics.length, 0);
  });

  it("records nothing while check mode is off", () => {
    const cs = new CheckState();
    cs.recordDef("a");
    cs.recordUse("a");
    assert.equal(cs.defsInstalled.size, 0);
    assert.equal(cs.defsUsed.size, 0);
  });

  it("re-installing a name clears its earlier use", () => {
    const cs = new CheckState();
    cs.begin();
    cs.recordDef("a");
    cs.recordUse("a");
    cs.recordDef("a");
    cs.emitUnusedDefDiagnostics();
    assert.equal(cs.diagnostics.length, 1);
  });
});

describe("CheckState — clone and restoreFrom", () => {
  it("round-trips every field", () => {
    const cs = new CheckState();
    cs.begin();
    cs.addDiagnostic({ code: "arity_mismatch", detail: "d", word: "w" });
    cs.recordDef("a");
    cs.stepCount = 3;
    cs.fnBodyDepth = 2;
    cs.fnInflight.add("f");

    const cp = cs.clone();
    assert.equal(cp.diagnostics.length, 1);
    assert.equal(cp.stepCount, 3);
    assert.equal(cp.fnBodyDepth, 2);
    assert.equal(cp.fnInflight.has("f"), true);

    // The clone is DEEP: mutating the original must not reach it.
    cs.addDiagnostic({ code: "arity_mismatch", detail: "d2", word: "w2" });
    cs.fnInflight.add("g");
    assert.equal(cp.diagnostics.length, 1);
    assert.equal(cp.fnInflight.has("g"), false);

    cs.restoreFrom(cp);
    assert.equal(cs.diagnostics.length, 1);
    assert.equal(cs.fnInflight.has("g"), false);
  });
});

describe("Registry — the def stack", () => {
  it("pushes, tops, pops and reports depth", () => {
    const r = new Registry();
    assert.equal(r.hasDef("a"), false);
    assert.equal(r.defStackDepth("a"), 0);
    assert.equal(r.topOfDefStack("a"), undefined);

    r.pushDef("a", newInteger(1n));
    r.pushDef("a", newInteger(2n));
    assert.equal(r.defStackDepth("a"), 2);
    assert.equal(r.topOfDefStack("a")!.asInteger(), 2n);
    assert.equal(r.hasDef("a"), true);

    assert.equal(r.popDef("a"), true);
    assert.equal(r.topOfDefStack("a")!.asInteger(), 1n);
    assert.equal(r.popDef("a"), true);
    assert.equal(r.hasDef("a"), false);
  });

  it("reports false when popping an absent or exhausted name", () => {
    const r = new Registry();
    assert.equal(r.popDef("nope"), false);
    r.pushDef("a", newInteger(1n));
    r.popDef("a");
    assert.equal(r.popDef("a"), false);
  });

  it("lists the bound names", () => {
    const r = new Registry();
    r.pushDef("a", newInteger(1n));
    r.pushDef("b", newInteger(2n));
    assert.deepEqual(r.defNames().sort(), ["a", "b"]);
  });
});

describe("Registry — the args stack", () => {
  it("pushes, tops and pops frames", () => {
    const r = new Registry();
    assert.equal(r.topArgs(), undefined);
    assert.equal(r.popArgs(), undefined);
    r.pushArgs([newInteger(1n)]);
    r.pushArgs([newInteger(2n)]);
    assert.equal(r.topArgs()![0]!.asInteger(), 2n);
    assert.equal(r.popArgs()![0]!.asInteger(), 2n);
    assert.equal(r.topArgs()![0]!.asInteger(), 1n);
  });
});

describe("Registry — capabilities", () => {
  it("stores and reports presence", () => {
    const r = new Registry();
    assert.deepEqual(r.capability("x"), [undefined, false]);
    r.setCapability("x", 42);
    assert.deepEqual(r.capability("x"), [42, true]);
  });
});

describe("Registry — snapshot and restore", () => {
  it("restores defs, capabilities, args and check state together", () => {
    const r = new Registry();
    r.pushDef("a", newInteger(1n));
    r.setCapability("c", "v");
    r.pushArgs([newInteger(9n)]);
    r.check.begin();
    r.check.addDiagnostic({ code: "arity_mismatch", detail: "d", word: "w" });

    const snap = r.snapshot();

    r.pushDef("a", newString("mutated"));
    r.pushDef("b", newInteger(2n));
    r.setCapability("c2", "v2");
    r.pushArgs([newInteger(8n)]);
    r.check.addDiagnostic({ code: "arity_mismatch", detail: "d2", word: "w2" });

    r.restore(snap);

    assert.equal(r.defStackDepth("a"), 1);
    assert.equal(r.topOfDefStack("a")!.asInteger(), 1n);
    assert.equal(r.hasDef("b"), false);
    assert.deepEqual(r.capability("c"), ["v", true]);
    assert.deepEqual(r.capability("c2"), [undefined, false]);
    assert.equal(r.topArgs()![0]!.asInteger(), 9n);
    assert.equal(r.check.diagnostics.length, 1);
  });

  it("deep-copies each def stack and args frame", () => {
    // Mutating the live registry after snapshotting must not reach the
    // snapshot — a shallow copy of the Map would share the arrays.
    const r = new Registry();
    r.pushDef("a", newInteger(1n));
    r.pushArgs([newInteger(1n)]);
    const snap = r.snapshot();
    r.pushDef("a", newInteger(2n));
    r.topArgs()!.push(newInteger(2n));
    assert.equal(snap.defStacks.get("a")!.length, 1);
    assert.equal(snap.argsStack[0]!.length, 1);
  });

  it("shares function registrations across the snapshot, by design", () => {
    // Static infrastructure, per snapshot()'s contract: a fn registered
    // after the snapshot survives the restore.
    const r = new Registry();
    const snap = r.snapshot();
    r.registerNativeFunc({
      name: "f",
      signatures: [{ args: [], returns: [] }],
    } as never);
    r.restore(snap);
    assert.ok(
      r.lookup("f") !== undefined,
      "registration should survive restore",
    );
  });
});
