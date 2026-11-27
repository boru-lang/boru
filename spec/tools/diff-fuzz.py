#!/usr/bin/env python3
"""diff-fuzz — differential testing of the Lean model against core/go.

Generates random programs in the core/spec notation over the fixture
vocabulary, evaluates each on BOTH the Lean model (`lake exe corespec
--probe`) and the Go kernel (`TestCoreProbe`, the probe core/go already
ships for writing corpus rows), and reports every expression the two
answer differently.

This is a search tool, not a proof and not a gate: agreement on N random
programs refutes nothing beyond those N. Its value is the other
direction — a disagreement is a concrete expression where the
specification and the kernel part ways, and either the model or the
kernel (or the corpus, which did not pin the case) is wrong.

    tools/diff-fuzz.py [--n 2000] [--seed 1] [--max-len 8] [--out FILE]
"""
import argparse, os, random, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC = os.path.dirname(HERE)
REPO = os.path.dirname(SPEC)

WORDS = ["addq", "negq", "pairq", "sumq", "boomq", "patq", "dupq", "__pa", "undef",
         "qanyq", "bothq", "tyq", "tpatq", "unifyq", "meetq", "meetq", "depthq", "afn", "eachq"]
LITS = ["0", "1", "2", "5", "7", "-3", "'a'", "'x'", "true", "false", "none", "null"]
TYPES = ["Integer", "String", "Any", "None", "List", "Map", "Boolean", "Word", "Scalar/String"]
NAMES = ["x", "y", "n", "nosuchword"]
MARKS = ["(", ")", ";"]


def gen_item(rng, depth):
    r = rng.random()
    if depth > 0 and r < 0.16:
        kind = rng.choice(["[", "[q", "p(", "{", "{q", "d(", "[:", "{:"])
        n = rng.randint(0, 3)
        if kind in ("[", "[q"):
            return [kind] + sum((gen_item(rng, depth - 1) for _ in range(n)), []) + ["]"]
        if kind == "p(":
            # a bare `)` child would close the form early and leave the
            # rest as siblings — malformed inside a map
            kids = [k for k in (gen_item(rng, depth - 1) for _ in range(n)) if k != [")"]]
            return [kind] + sum(kids, []) + [")"]
        if kind == "d(":
            return [kind] + sum((gen_value(rng, depth - 1) for _ in range(n)), []) + [")"]
        if kind == "[:":
            return [kind] + gen_value(rng, 0) + sum((gen_value(rng, depth - 1) for _ in range(n)), []) + ["]"]
        if kind == "{:":
            out = [kind] + gen_value(rng, 0)
            for k in rng.sample(["a", "b", "c"], min(n, 3)):
                out += [k + ":"] + gen_value(rng, depth - 1)
            return out + ["}"]
        out = [kind]
        for k in rng.sample(["a", "b", "c"], min(n, 3)):
            out += [k + ":"] + gen_item(rng, depth - 1)
        return out + ["}"]
    if r < 0.50:
        w = rng.choice(WORDS)
        if w == "afn" and depth == 2 and rng.random() < 0.8:
            # mostly well-formed: a parameter name and a quoted body (at
            # the top level only — a map member is ONE item)
            body = rng.choice(["addq n 1", "n", "addq n x", "x", "n n", "", "afn m [q addq n m ]", "negq n"])
            return ["afn", "n", "[q"] + body.split() + ["]"]
        if w == "eachq" and depth == 2 and rng.random() < 0.85:
            # a driven loop: elements, and a body stepped once per element
            # with the element as its whole stack
            elems = sum((gen_value(rng, 1) for _ in range(rng.randint(0, 3))), [])
            body = rng.choice(["negq", "", "dupq", "addq 10", "depthq", "9", "boomq", "(", ")", ";",
                               "addq 1 ;", "addq", "pairq", "[ negq 5 ]", "negq/v", "x", "addq x",
                               "eachq [q 10 20 ] [q addq 1 ]", "afn n [q addq n 1 ]", "unifyq Integer",
                               "bothq", "n", "inc", "undef x", "( negq )", "negq ("])
            return ["eachq", "[q"] + elems + ["]", "[q"] + body.split() + ["]"]
        # call-site modifiers: /s stack only, /f forward only, /N arity,
        # /v the bound value, /u the function with its arguments reversed
        if rng.random() < 0.22:
            w += "/" + rng.choice(["s", "f", "1", "2", "0", "1s", "2f", "1f", "2s", "v", "v", "v",
                                   "u", "u", "u", "uv", "uv", "su", "2u", "fuv", "sv", "1v"])
        return [w]
    if r < 0.78:
        return [rng.choice(LITS)]
    if r < 0.86:
        return [rng.choice(TYPES)]
    if r < 0.92:
        n = rng.choice(NAMES + ["inc"])
        r2 = rng.random()
        return [n + "/v"] if r2 < 0.2 else [n + "/u"] if r2 < 0.3 else [n + "/uv"] if r2 < 0.35 else [n]
    return [rng.choice(MARKS)]


def gen_value(rng, depth):
    """An item that is data: no markers, no bare words."""
    r = rng.random()
    if depth > 0 and r < 0.2:
        n = rng.randint(0, 2)
        if rng.random() < 0.5:
            return ["[q"] + sum((gen_value(rng, depth - 1) for _ in range(n)), []) + ["]"]
        out = ["{q"]
        for k in rng.sample(["a", "b", "c"], n):
            out += [k + ":"] + gen_value(rng, depth - 1)
        return out + ["}"]
    if r < 0.66:
        return [rng.choice(LITS[:8])]
    if r < 0.70:
        # a function is a value too — and one with an IDENTITY: the same
        # text written twice is two functions
        return ["fn(", "n", "Integer", "Integer", "[", "n", "]", ")"]
    return [rng.choice(TYPES[:7])]


def gen_program(rng, max_len):
    toks = []
    if rng.random() < 0.25:
        toks += ["def", "x"] + gen_value(rng, 1) + [";"]
    if rng.random() < 0.08:
        toks += ["def", "y"] + gen_value(rng, 1) + [";"]
    # a global named like the parameter every generated function takes: a
    # call shadows it, and must leave it bound
    if rng.random() < 0.10:
        toks += ["def", "n"] + gen_value(rng, 0) + [";"]
    # a function that BUILDS a closure in its frame, and calls of it
    if rng.random() < 0.10:
        inner = rng.choice(["addq n x", "x", "n", "addq n y", "afn m [q addq m x ]", "addq x 1", "n x"])
        toks += ["def", "outer", "fn(", "x", rng.choice(["Integer", "Any"]), "Any",
                 "[", "afn", "n", "[q"] + inner.split() + ["]", "]", ")", ";"]
        if rng.random() < 0.8:
            toks += rng.choice([["bothq", "1", "(", "outer", "10", ")"], ["outer", "10"], ["(", "outer", "10", ")"]])
    if rng.random() < 0.10:
        body = rng.choice(["[ addq n 1 ]", "[ n ]", "[ negq n ]", "[ n n ]", "[ ]", "[ 'a' ]", "[ dupq n ]"])
        toks += ["def", "inc", "fn(", "n", rng.choice(["Integer", "Any"]),
                 rng.choice(["Integer", "Any"])] + body.split() + [")", ";"]
        if rng.random() < 0.8:
            toks += ["inc"]
    items = [gen_item(rng, 2) for _ in range(rng.randint(1, max_len))]
    # a function VALUE standing in the program: it dispatches where it stands
    if rng.random() < 0.08:
        body = rng.choice(["[ addq n 1 ]", "[ n ]", "[ negq n ]", "[ n n ]", "[ ]", "[ dupq n ]"])
        fnv = ["fn(", "n", rng.choice(["Integer", "Any"]), rng.choice(["Integer", "Any"])] + body.split() + [")"]
        items.insert(rng.randint(0, len(items)), fnv)
    # mostly keep the top-level paren MARKERS balanced: they are the
    # interesting shape. A `)` inside a bracket form is that form's own
    # closer and is never touched.
    if rng.random() < 0.8:
        depth, fixed = 0, []
        for it in items:
            if it == ["("]:
                depth += 1
            elif it == [")"]:
                if depth == 0:
                    continue
                depth -= 1
            fixed.append(it)
        items = fixed + [[")"]] * depth
    for it in items:
        toks += it
    return "run " + " ".join(toks)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--n", type=int, default=2000)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--max-len", type=int, default=8)
    ap.add_argument("--out", default="")
    ap.add_argument("--show", type=int, default=40)
    args = ap.parse_args()

    rng = random.Random(args.seed)
    progs = sorted({gen_program(rng, args.max_len) for _ in range(args.n)})
    tmp = tempfile.mkdtemp(prefix="boru-diff-")
    inp = os.path.join(tmp, "in.txt")
    with open(inp, "w") as f:
        f.write("\n".join(progs) + "\n")

    go_out = os.path.join(tmp, "go.txt")
    env = dict(os.environ, CORE_PROBE_IN=inp, CORE_PROBE_OUT=go_out)
    r = subprocess.run(["go", "test", "-run", "TestCoreProbe$", "-count=1", "."],
                       cwd=os.path.join(REPO, "core", "go"), env=env, capture_output=True, text=True)
    if r.returncode != 0 or not os.path.exists(go_out):
        sys.stderr.write("go probe failed:\n" + r.stdout[-3000:] + r.stderr[-3000:])
        return 2
    go = open(go_out).read().split("\n")

    r = subprocess.run(["lake", "exe", "corespec", "--probe", inp], cwd=SPEC, capture_output=True, text=True)
    if r.returncode != 0:
        sys.stderr.write("lean probe failed:\n" + r.stdout[-3000:] + r.stderr[-3000:])
        return 2
    lean = r.stdout.split("\n")

    # A program that does not terminate ends on a RESOURCE limit, and which
    # one it meets first is the engine's accounting, not the language: the
    # kernel has a step budget and a tape ceiling, the model only fuel. The
    # two engines agree on such a program when both ran out.
    resource = ("ERROR:evaluation_limit", "ERROR:tape_exhausted")
    diffs, unmodelled, exhausted = [], 0, 0
    for i, p in enumerate(progs):
        g = go[i] if i < len(go) else "<missing>"
        l = lean[i] if i < len(lean) else "<missing>"
        if l in ("ERROR:unmodelled",):
            unmodelled += 1
            continue
        if g in resource and l in resource:
            exhausted += 1
            continue
        if g != l:
            diffs.append((p, g, l))
    print(f"seed {args.seed}: {len(progs)} programs, {len(diffs)} differ, {unmodelled} outside the model"
          + (f", {exhausted} ran out of a resource in both" if exhausted else ""))
    diffs.sort(key=lambda d: len(d[0]))
    for p, g, l in diffs[:args.show]:
        print(f"  {p}\n     go   {g}\n     lean {l}")
    if args.out:
        with open(args.out, "w") as f:
            for p, g, l in diffs:
                f.write(f"{p}\t{g}\t{l}\n")
    return 1 if diffs else 0


if __name__ == "__main__":
    sys.exit(main())
