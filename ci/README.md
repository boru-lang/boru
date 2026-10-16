# `ci/` — staged workflow update

This folder holds a change to `.github/workflows/` that must be applied by
hand. It was authored by an integration whose GitHub credentials lack the
`workflow` scope, so a push touching `.github/workflows/*` is rejected by
GitHub. Rather than drop the change, it is staged here for a maintainer
whose credentials carry the scope.

## What the change is

The compiler lane's removal (design/COMPILER-LANE-REMOVAL.0.md) deleted
the `borudebug` build tag and the differential gates the `borudebug` CI job
ran. The job still exists in `.github/workflows/ci.yml` and passes only
because `scripts/ci-steps.sh borudebug-gates` is kept as a retired no-op.

- [`ci/ci.yml`](./ci.yml) is the **full** intended contents of
  `.github/workflows/ci.yml`: the `borudebug` job is gone, and the header's
  description of the lanes names the direction lane's one remaining end
  state and the race gate's one package. Every other job is unchanged.
- [`ci/cover-gate.yml`](./cover-gate.yml) is the **full** intended contents
  of `.github/workflows/cover-gate.yml`: the merged gate's comment counts
  "every module" rather than the thirteen the lane's modules made.

The folder's previous content — a single-job workflow adding a per-PR
`make cover-gate` step — predated the parallel-job layout now on `main`,
which runs core's standalone gate on every PR and the merged ADR-008 gate
nightly in `cover-gate.yml`. It was superseded by that layout and is
replaced here; its manifest also still ran the `cover-gate-compiler` and
`cover-gate-eng` targets the removal deleted.

## How to apply

```bash
cp ci/ci.yml .github/workflows/ci.yml
cp ci/cover-gate.yml .github/workflows/cover-gate.yml
git rm -r ci
git add .github/workflows
git commit -m "ci: drop the borudebug job (the compiler lane is gone)"
git push
```

Then delete the `borudebug-gates` case in `scripts/ci-steps.sh` — its
comment names this step — in the same change or the next.
