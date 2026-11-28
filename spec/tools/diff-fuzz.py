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

Two disagreements are KNOWN and are the kernel's, recorded in NUR.md; the
model states the rule in both, and a seed may still draw them:

  NUR407  a callee that unbinds its own parameter, called LAST by a
          function of the same parameter name (tail-call elision shows)
  NUR412  an all-unnamed function value — a `fn(` of `_` parameters, or
          `name/u` of one — with its arguments on the stack in the order
          the one argument rule refuses (the kernel reads them bottom-up)
"""
import argparse, os, random, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC = os.path.dirname(HERE)
REPO = os.path.dirname(SPEC)

WORDS = ["addq", "negq", "pairq", "sumq", "boomq", "patq", "dupq", "__pa", "undef",
         "qanyq", "bothq", "tyq", "tpatq", "unifyq", "meetq", "meetq", "depthq", "afn", "eachq",
         "forq", "forq", "rangeq", "whileq", "breakq", "contq", "ifq", "ifq", "replayq", "callq", "callq",
         "argsq", "def", "bindq", "rawlq", "rawmq", "rawpq", "formq", "raw2q", "form2q",
         "mpatq", "rawmpatq", "lpatq", "rawlpatq"]
RAW = ("rawlq", "rawmq", "rawpq", "formq", "raw2q", "form2q", "mpatq", "rawmpatq", "lpatq", "rawlpatq")
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
        if w in ("def", "bindq") and depth == 2 and rng.random() < 0.9:
            # a binding made at RUN TIME: a name and a value. In a function's
            # body it is the frame's to undo — when the body names a word
            # the frame analysis knows (`def`), and not when it does not
            # (`bindq`)
            return [w, rng.choice(["x", "y", "n", "c", "i", "inc"])] + gen_value(rng, 1) + (
                [";"] if rng.random() < 0.5 else [])
        if w == "afn" and depth == 2 and rng.random() < 0.8:
            # mostly well-formed: a parameter name and a quoted body (at
            # the top level only — a map member is ONE item)
            # a body that is ONE bare container is the deferred residual: an
            # anonymous function leaves it unevaluated, and its consumer
            # evaluates it where the parameter is no longer bound
            body = rng.choice(["addq n 1", "n", "addq n x", "x", "n n", "", "afn m [q addq n m ]", "negq n",
                               "[ n ]", "{ a: n }", "[ n ] 1", "p( n )", "[ x ]", "[ addq n 1 ]", "[ c ]",
                               "[ breakq ]", "( [ n ] )", "[ n ] ;", "[q n ]", "[: Integer ]", "[ argsq ]"])
            return ["afn", "n", "[q"] + body.split() + ["]"]
        if w == "eachq" and depth == 2 and rng.random() < 0.85:
            # a driven loop: elements, and a body stepped once per element
            # with the element as its whole stack
            elems = sum((gen_value(rng, 1) for _ in range(rng.randint(0, 3))), [])
            body = rng.choice(["negq", "", "dupq", "addq 10", "depthq", "9", "boomq", "(", ")", ";",
                               "addq 1 ;", "addq", "pairq", "[ negq 5 ]", "negq/v", "x", "addq x",
                               "eachq [q 10 20 ] [q addq 1 ]", "afn n [q addq n 1 ]", "unifyq Integer",
                               "bothq", "n", "inc", "undef x", "( negq )", "negq (",
                               "def y", "def x ; x", "bindq y", "def y ; y negq"])
            return ["eachq", "[q"] + elems + ["]", "[q"] + body.split() + ["]"]
        if w in ("forq", "rangeq") and depth == 2 and rng.random() < 0.85:
            # a counted loop: `mark body… move` on the caller's own stack,
            # the index bound as `i`; its values are stepped where it stood
            body = rng.choice(["i", "i i", "", "7", "negq/s", "addq i 10", "addq i", "depthq", "sumq",
                               "dupq/s", "[ i ]", "{ a: i }", "[q i ]", "negq/v", "negq", "bothq 1",
                               "fn( n Integer Integer [ addq n 1 ] )", "fn( n Integer Integer [ addq n 1 ] ) i",
                               "forq 2 [q i ]", "i forq 2 [q i ] i", "eachq [q 5 6 ] [q i ]", "undef i",
                               "undef i i", "boomq", "(", ")", ";", "i ;", "( i )", "( i", "pairq i",
                               "bothq i", "afn n [q addq n i ]", "x", "addq x", "inc", "unifyq i",
                               "whileq [q false ] [q 1 ]", "i depthq", "dupq i", "i negq/s", "i/v",
                               "c undef c", "rangeq i 3 1 [q i ]", "meetq i Integer",
                               # the flow signals: the nearest loop ahead takes one, through
                               # a group, a literal, a call frame and a driven loop
                               "i breakq", "breakq i", "i contq 9", "contq", "( i breakq ) 7",
                               "i ( contq ) 7", "[ i breakq ] 7", "{ a: contq } 7", "i [ 5 contq ]",
                               "bothq 1 ( i breakq ) 7", "negq ( contq )", "i bothq 1 breakq",
                               "forq 2 [q i breakq ] 7", "forq 2 [q i contq 8 ] 7", "forq 2 [q i ] breakq 7",
                               "eachq [q 1 2 ] [q breakq ] 7", "i eachq [q 1 2 ] [q contq ] 7",
                               "inc breakq", "outer 1 breakq", "negq/s breakq", "negq/s contq",
                               "depthq breakq", "i breakq/s", "1 breakq/v", "i ; contq", "( breakq",
                               "afn n [q n breakq ]", "bothq 0 ( afn n [q n breakq ] ) i 7",
                               "whileq [q true ] [q i breakq ] 7", "whileq [q c ] [q undef c contq ] i",
                               "x breakq", "undef i contq", "i breakq contq", "ov i breakq", "ov i 1 contq",
                               "ifq [q i ] [q i ] [q 'z' ]", "ifq [q i ] [q breakq ] [q i ]",
                               "ifq [q i ] [q contq ] [q i ] 7", "ifq [q i breakq ] [q 1 ] [q 2 ]",
                               "i ifq [q contq ] [q 1 ] [q 2 ]", "replayq [q i ]", "replayq [q i breakq ]",
                               "ifq [q c ] [q undef c ] [q breakq ]", "ifq [q depthq ] [q 1 ] [q 2 ]",
                               "callq i [q 7 ]", "callq i [q breakq ]", "i callq 9 [q contq ] 7",
                               "callq i [q negq/v ]",
                               # a binding made in a round, the index's name among them
                               "def y i ;", "def y i ; y", "bindq y i ;", "def i 9 ; i", "i def i 9 ;",
                               "def x i ; x breakq", "def c false ; i", "def y i ; contq", "undef y i"])
            if w == "forq":
                head = ["forq", rng.choice(["0", "1", "2", "2", "3", "-1", "'a'", "x"])]
            else:
                head = ["rangeq", rng.choice(["0", "1", "5"]), rng.choice(["0", "3", "5", "-2"]),
                        rng.choice(["1", "2", "-1", "-2", "0"])]
                # the edge of the Integer range: the index's advance wraps
                if rng.random() < 0.06:
                    head = rng.choice([
                        ["rangeq", "9223372036854775805", "9223372036854775807", "1"],
                        ["rangeq", "9223372036854775806", "9223372036854775807", "2"],
                        ["rangeq", "-9223372036854775807", "-9223372036854775808", "-2"]])
            return head + ["[q"] + body.split() + ["]"]
        if w == "ifq" and depth == 2 and rng.random() < 0.85:
            # the `if` continuation: a condition region whose LAST value picks
            # the branch — a paren group — that takes the region's place
            cond = rng.choice(["true", "false", "c", "c", "x", "0", "1", "''", "none", "", "5 false",
                               "false 5", "None", "Any", "Integer", "[q ]", "[ 0 ]", "nosuchword",
                               "boomq", "negq/s", "sumq", "depthq", "dupq/s", "(", "1 ;", "bothq 1",
                               "breakq", "contq", "i", "n", "( c )", "inc", "ifq [q c ] [q true ] [q false ]"])
            arms = ["1", "2", "", "1 2 3", "negq/s", "addq 1", "boomq", "x", "c undef c", "breakq", "contq",
                    "fn( n Integer Integer [ addq n 1 ] )", "negq/v", "[ addq 1 2 ]", "(", ")", "i", "n",
                    "ifq [q c ] [q 2 ] [q 3 ]", "forq 2 [q i ]", "eachq [q 1 2 ] [q negq ]", "inc", "depthq"]
            return (["ifq", "[q"] + cond.split() + ["]", "[q"] + rng.choice(arms).split() + ["]",
                    "[q"] + rng.choice(arms).split() + ["]"])
        if w == "callq" and depth == 2 and rng.random() < 0.85:
            # a call region: a driven loop of one round, whose residual is the
            # word's results — stepped again where the word stood
            body = rng.choice(["", "negq", "negq/s", "7", "addq 1", "depthq", "sumq", "negq/v", "boomq",
                               "fn( n Integer Integer [ addq n 1 ] )", "[ addq 1 2 ]", "(", ")", ";",
                               "bothq 1", "callq 6 [q sumq ]", "callq 6 [q ]", "breakq", "contq", "x",
                               "addq x", "afn n [q addq n 1 ]", "ifq [q dupq/s ] [q 'y' ] [q 'n' ]",
                               "eachq [q 5 6 ] [q negq ]", "forq 2 [q i breakq ]", "inc", "dupq", "undef x",
                               "i", "c undef c", "pairq", "unifyq Integer", "negq/s negq/s"])
            return ["callq"] + gen_value(rng, 1) + ["[q"] + body.split() + ["]"]
        if w == "replayq" and depth == 2 and rng.random() < 0.85:
            # the plain mark and move: the body runs, and then runs again in
            # the place of everything its first run left
            body = rng.choice(["1 2", "", "c undef c", "negq/s", "addq 1 2", "boomq", "replayq [q 7 ]", "i",
                               "breakq", "depthq", "fn( n Integer Integer [ addq n 1 ] )", "(", "[ addq 1 2 ]",
                               "x", "inc", "ifq [q c ] [q 1 ] [q 2 ]", "forq 2 [q i ]", "contq", "n"])
            return ["replayq", "[q"] + body.split() + ["]"]
        if w == "whileq" and depth == 2 and rng.random() < 0.85:
            # a conditional loop: the condition region's LAST value decides.
            # `c` is a stack of bindings a body pops, so the loop can end
            cond = rng.choice(["c", "c", "c", "true", "true", "true breakq", "contq", "breakq", "c contq",
                               "false", "0", "none", "''", "[q ]", "{q }", "", "5 c",
                               "Integer", "None", "List", "x", "nosuchword", "null", "0.0", "c c",
                               "( c )", "negq 0", "[ c ]", "depthq", "true", "Any"])
            body = rng.choice(["7 breakq", "breakq 7", "undef c 7 contq 8", "undef c 7 breakq 8",
                               "6 undef c contq", "undef c [ 5 breakq ]", "undef c ( contq ) 7",
                               "undef c forq 2 [q i breakq ] 7", "undef c eachq [q 1 ] [q breakq ]",
                               "undef c 7", "undef c", "c undef c", "undef c [ c ]", "undef c i",
                               "undef c forq 2 [q i ]", "undef c negq/s", "undef c depthq",
                               "undef c fn( n Integer Integer [ n ] )", "undef c ;", "undef c (",
                               "undef c boomq", "undef c x", "7", ""])
            return ["whileq", "[q"] + cond.split() + ["]", "[q"] + body.split() + ["]"]
        if w in RAW and depth == 2 and rng.random() < 0.7:
            # a raw-capture word with operands its modes tell apart: a
            # pending list, a pending map, a paren expression, a word bound
            # or not — before the word, after it, or either side of it
            def operand():
                return rng.choice(["[ addq 1 2 ]", "{ a: p( addq 1 2 ) }", "p( addq 1 2 )", "x", "nosuchword",
                                   "[ x ]", "p( x )", "{ a: [ x ] }", "5", "negq", "[q addq 1 2 ]", "x/v",
                                   "( addq 1 2 )", "[ breakq ]", "p( breakq )", "[: Integer 1 ]", "negq/v",
                                   "inc", "p( )", "[ ]", "nosuchword/2", "c", "i", "n", "[ n ]",
                                   "fn( n Integer Integer [ n ] )", "rawlq", "formq", ";",
                                   # operands a contents-reading pattern judges: pending,
                                   # and evaluated at the match unless the slot is raw
                                   "{ a: 1 }", "{ a: p( addq 0 1 ) }", "{ a: x }", "{ a: 1 b: p( nosuchword ) }",
                                   "{ a: p( bindq y 1 1 ) }", "{ a: p( breakq ) }", "{ a: 2 }", "{q a: 1 }",
                                   "[ 1 2 ]", "[ addq 0 1 ]", "[ 'a' ]", "[ bindq y 1 1 ]", "[q 1 2 ]",
                                   "{ a: [ addq 0 1 ] }", "[ 1 addq 0 1 ]", "{ a: n }", "y"]).split()
            mod = rng.choice(["", "", "", "", "/u", "/u", "/s", "/f", "/1", "/2", "/v", "/uv"])
            before = sum((operand() for _ in range(rng.choice([0, 0, 0, 1, 2]))), [])
            after = sum((operand() for _ in range(rng.choice([0, 1, 1, 1, 2, 2, 3]))), [])
            return before + [w + mod] + after
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


def gen_spec(rng):
    """A function specification: `[q [q params ] [q returns ] [q body ] … ]`."""
    types = ["Integer", "Integer", "String", "Any", "Any", "Boolean", "List", "Map", "None", "Number", "Scalar",
             "Float", "Atom", "Function", "Scalar/String", "Node", "Nosuch", "integer"]
    out = ["[q"]
    for _ in range(rng.choice([1, 1, 1, 2])):
        params = []
        for name in rng.sample(["n", "m", "k"], rng.choice([0, 1, 1, 1, 2, 2, 3])):
            r = rng.random()
            if r < 0.45:
                params.append(f"{name}:{rng.choice(types)}")
            elif r < 0.62:
                params.append(f"{name}?:{rng.choice(types)}")
            elif r < 0.68:
                params.append(f"{name}:{rng.choice(['Atom/q', 'kw/q', 'Integer/q', 'Any/q'])}")
            elif r < 0.76:
                params.append(rng.choice(types))
            elif r < 0.84:
                params.append(rng.choice(["7", "'a'", "0", "''"]))
            elif r < 0.88:
                params += [f"{name}:{rng.choice(types)}", "?"]
            elif r < 0.92:
                params += ["|", f"{name}:{rng.choice(types)}"]
            else:
                params += rng.choice([["nosuch"], ["none"], ["[q", "1", "]"], ["d(", "Integer", "String", ")"],
                                      ["{q", "x:", "Integer", "}"], ["[:", "Integer", "]"], ["?"], ["|"],
                                      ["Bad:Integer"], ["n:"], [":Integer"], ["{q", "}"], ["{:", "Integer", "}"]])
        returns = []
        for _ in range(rng.choice([0, 1, 1, 1, 1, 2])):
            returns += rng.choice([[rng.choice(types)], [rng.choice(types)], ["d(", "Integer", "String", ")"], ["7"],
                                   ["'a'"], ["r:Integer"], ["none"], ["[q", "1", "]"], ["[:", "Integer", "]"],
                                   ["{q", "a:", "Integer", "}"], ["d(", ")"], ["d(", "7", "'a'", ")"], ["x"],
                                   ["'Integer'"], ["'ok'"], ["'Nosuch'"], ["Bad:Integer"]])
        body = rng.choice(["n", "n", "addq n 1", "m", "[ n m ]", "", "7", "'a'", "n m", "argsq", "negq/s", "true",
                           "depthq", "k", "n 'a'", "[q 1 ]", "none", "{ a: n }", "def y n ; y", "sumq",
                           "5", "n k m", "ifq [q n ] [q ( addq n -1 ) sf/s ] [q 0 ]"])
        # (no body that calls `sf` unguarded: with an optional parameter it
        # recurses without end, which the kernel's elision makes cheap and
        # the model's nesting makes slow — the answer is a resource limit
        # either way)
        # a part that is not a list stands for the list of itself
        ps = ["[q"] + params + ["]"] if (len(params) != 1 or rng.random() < 0.85) else params
        rs = ["[q"] + returns + ["]"] if (len(returns) != 1 or rng.random() < 0.8) else returns
        out += ps + rs + ["[q"] + body.split() + ["]"]
    if rng.random() < 0.04:
        out = out[:-rng.randint(1, 3)]  # a specification cut short
        depth = sum(1 for t in out if t in ("[q", "[", "d(", "{q", "[:", "{:")) - sum(1 for t in out if t in ("]", ")", "}"))
        out += ["]"] * max(depth - 1, 0)
    return out + ["]"]


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
    # a STACK of bindings of one name: a conditional loop's body pops it,
    # so its condition changes from round to round
    if rng.random() < 0.20:
        for v in rng.sample(["false", "true", "1", "0", "'a'", "none", "true", "2", "Any", "[q 1 ]",
                             "d( )", "d( 1 2 )", "[: Integer ]", "List", "String", "''", "{q }", "null"],
                            rng.randint(1, 4)):
            toks += ["def", "c", v, ";"]
    # an ALIAS of a binding word, bound while the program runs: a function
    # VALUE whose body names it is asked at the call whether its frame
    # records, and finds the alias; an installed function was asked before
    if rng.random() < 0.08:
        target = rng.choice(["def/v", "def/v", "undef/v", "afn/v", "bindq/v", "negq/v"])
        toks += ["0", "def", "al", target, ";"]
        body = rng.choice(["al y 3 ; n", "al y 3 ; y", "bindq y 3 ; al z n", "al m [q addq m n ]",
                           "al n 9 ; n", "al y n ; { a: y }", "al x n ; x", "bindq y n ; al y", "al y"])
        toks += rng.choice([["afn", "n", "[q"] + body.split() + ["]", "5"],
                            ["bothq", "0", "(", "afn", "n", "[q"] + body.split() + ["]", "5", ")", "7"],
                            ["eachq", "[q", "1", "2", "]", "[q", "al", "y", "]"],
                            ["al", "y", "4", ";"]])
        if rng.random() < 0.7:
            toks += [rng.choice(["y", "x", "al/v", ";"])]
    # a function that BUILDS a closure in its frame, and calls of it
    if rng.random() < 0.10:
        inner = rng.choice(["addq n x", "x", "n", "addq n y", "afn m [q addq m x ]", "addq x 1", "n x"])
        toks += ["def", "outer", "fn(", "x", rng.choice(["Integer", "Any"]), "Any",
                 "[", "afn", "n", "[q"] + inner.split() + ["]", "]", ")", ";"]
        if rng.random() < 0.8:
            toks += rng.choice([["bothq", "1", "(", "outer", "10", ")"], ["outer", "10"], ["(", "outer", "10", ")"]])
    if rng.random() < 0.10:
        body = rng.choice(["[ addq n 1 ]", "[ n ]", "[ negq n ]", "[ n n ]", "[ ]", "[ 'a' ]", "[ dupq n ]",
                           "[ n breakq ]", "[ n contq ]", "[ forq 2 [q i breakq ] n ]", "[ [ n breakq ] ]",
                           "[ ifq [q n ] [q 1 ] [q 2 ] ]", "[ ifq [q n ] [q n ] [q breakq ] ]", "[ replayq [q n ] ]",
                           "[ argsq ]", "[ __pa argsq ]", "[ __pa n ]", "[ n argsq ]", "[ eachq [q 1 ] [q argsq ] ]",
                           "[ afn m [q argsq ] ]", "[ argsq breakq ]", "[ __pa afn m [q addq m n ] ]",
                           # BODY-LOCAL definitions: what the body binds, the frame's close
                           # unbinds — in a frame that took a snapshot
                           "[ def y 3 ; addq y n ]", "[ def n 9 ; n ]", "[ bindq y 3 ; y ]", "[ bindq n 9 ; n ]",
                           "[ undef n n ]", "[ undef n 7 ]", "[ undef x n ]", "[ undef x def x 2 ; n ]",
                           "[ def y n ; afn m [q addq m y ] ]", "[ def x 2 ; n breakq ]", "[ def c false ; n ]",
                           "[ forq 2 [q def y i ; ] y ]", "[ def y n ; [ y ] ]", "[ def y n ; { a: y } ]",
                           "[ bindq y 3 ; undef z y ]", "[ def inc 3 ; inc ]", "[ def y 3 ; outer n ]",
                           "[ def x n ; x ]", "[ bindq x n ; x ]", "[ def y 3 ; boomq 1 ]", "[ def y n ; p( y ) ]",
                           "[ def y 3 ; def y 4 ; y ]", "[ def y 3 ; undef y undef y n ]", "[ ( def y n ; ) y ]",
                           "[ eachq [q 1 2 ] [q def y ] n ]", "[ callq n [q def y ] y ]", "[ def n ; n ]",
                           "[ ifq [q n ] [q def y 1 ; ] [q 0 ] n ]", "[ def y 3 ; contq ]", "[ bindq y 3 ; n breakq ]",
                           # TAIL CALLS: the function calls itself last, through a branch.
                           # The kernel closes each frame before the next opens; the model
                           # nests them; the answers must be one. (No `undef n` among
                           # these: that is NUR407, where they are not.)
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q 0 ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q argsq ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q depthq ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q x ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q breakq ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q 'a' ] ]",
                           "[ def y n ; ifq [q n ] [q ( addq n -1 ) inc/s ] [q y ] ]",
                           "[ bindq y n ; ifq [q n ] [q ( addq n -1 ) inc/s ] [q y ] ]",
                           "[ n ifq [q n ] [q ( addq n -1 ) inc/s sumq ] [q 0 ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc/s ] [q afn m [q addq m n ] ] ]",
                           "[ ifq [q n ] [q ( addq n -1 ) inc ] [q 0 ] ]",
                           "[ forq 1 [q ifq [q n ] [q ( addq n -1 ) inc/s ] [q 0 ] ] ]",
                           "[ outer n ]", "[ ( inc2 n ) ]", "[ inc2 n ]", "[ def y 3 ; inc2 n ]", "[ 7 inc2 n ]",
                           "[ al y 3 ; n ]", "[ al y n ; y ]"])
        toks += ["def", "inc", "fn(", "n", rng.choice(["Integer", "Any"]),
                 rng.choice(["Integer", "Any"])] + body.split() + [")", ";"]
        # a second function of the SAME parameter name, which `inc` may call
        # last: the kernel elides a tail call only when the callee binds
        # every name the caller's close removes
        if "inc2" in body or rng.random() < 0.2:
            body2 = rng.choice(["[ n ]", "[ addq n 1 ]", "[ argsq ]", "[ depthq ]", "[ y ]", "[ n n ]", "[ ]",
                                "[ def y n ; y ]", "[ 'a' ]", "[ boomq 1 ]", "[ n breakq ]", "[ x ]",
                                "[ afn m [q addq m n ] ]", "[ negq n ]"])
            toks += ["def", "inc2", "fn(", "n", rng.choice(["Integer", "Any"]),
                     rng.choice(["Integer", "Any"])] + body2.split() + [")", ";"]
        if rng.random() < 0.8:
            toks += rng.choice([["inc"], ["inc"], ["inc", "3"], ["inc", "2"], ["8", "inc", "1"]])
    # a function of UNNAMED parameters: its arguments are the frame's first
    # stack values, and up to that many values beneath its declared return
    # are let pass and dropped
    if rng.random() < 0.10:
        k = rng.randint(1, 2)
        body = rng.choice(["", "7", "7 8", "7 8 9", "negq/s", "negq/s 7", "dupq/s", "sumq", "depthq", "argsq",
                           "addq 1", "pairq/s", "( )", "[ 1 ]", "breakq", "x", "boomq", "fn( n Integer Integer [ n ] )",
                           "5 pairq/s", "__pa", "eachq [q 1 ] [q negq ]",
                           "def y ; y", "def y ;", "bindq y ; y", "def y 3 ;", "def x ; x x"])
        toks += ["def", "un", "fn("] + ["_", rng.choice(["Any", "Any", "Integer"])] * k + [
            rng.choice(["Any", "Any", "Integer"]), "["] + body.split() + ["]", ")", ";"]
        if rng.random() < 0.8:
            toks += [rng.choice(["un", "un", "un/u", "un/s", "un/v"])]
    # a function WRITTEN as a specification and bound by the program's own
    # `def`: the kernel's reader makes its signatures, and the install
    # compiles it under the name — optional parameters expanded, an older
    # definition it overlaps replaced
    if rng.random() < 0.18:
        toks += [";"]
        for _ in range(rng.choice([1, 1, 1, 2, 2, 3])):
            toks += ["def", "sf", "(", "fn"] + gen_spec(rng) + [")", ";"]
            if rng.random() < 0.3:
                toks += rng.choice([["sf", "5"], ["sf"], ["sf", "'a'"], ["sf/v"]])
        for _ in range(rng.randint(0, 3)):
            toks += rng.choice([["sf"], ["sf", "5"], ["sf", "'a'"], ["sf", "5", "6"], ["sf", "5", "'a'"], ["5", "sf"],
                                ["sf", "kw", "5"], ["sf", "hello"], ["sf/v"], ["sf/1", "5"], ["sf/s"], ["sf/u", "5", "6"],
                                ["sf", "true"], ["sf", "none"], ["sf", "7"], ["6", "sf", "5"], ["sf", "[q", "1", "]"],
                                ["undef", "sf"], ["def", "g", "sf/v", ";", "g", "5"], ["def", "g", "sf/v", ";", "g/v"],
                                ["sf", ";"], ["sf", "'a'", "5"], ["sf", "7", "7"]])
    if rng.random() < 0.04:
        # the function value as the reader leaves it, applied where it
        # stands — ONE signature of named scalar parameters. Beyond that the
        # kernel applies an uninstalled value by its legacy stack path
        # (NUR412: unnamed parameters read bottom-up, several signatures
        # tried in written order), which the model does not follow.
        ptype = rng.choice(["Integer", "Any", "String", "Scalar"])
        body = rng.choice(["n", "addq n 1", "7", "'a'", "n n", "", "argsq", "depthq", "def y n ; y"])
        ret = rng.choice([["Integer"], ["Any"], [], ["d(", "Integer", "String", ")"], ["7"]])
        toks += [";", "fn", "[q", "[q", f"n:{ptype}", "]", "[q"] + ret + ["]", "[q"] + body.split() + ["]", "]"]
        toks += rng.choice([["5"], [], ["'a'"], ["5", "6"], ["7"]])
    # a word with SEVERAL definitions, of one or two parameters: which one
    # a call takes is the dispatch order — more arguments first, then the
    # more specific type at the first position that differs, then the
    # newest definition
    overloaded = rng.random() < 0.15
    if overloaded:
        ptypes = ["Integer", "Any", "Scalar", "Number", "String", "Boolean", "List", "None"]
        for tag in range(1, rng.randint(2, 4) + 1):
            params = []
            for name in ["a", "b"][:rng.randint(1, 2)]:
                # `_` is an UNNAMED parameter: its argument enters the frame
                # as stack data instead of being bound
                params += [name if rng.random() < 0.8 else "_", rng.choice(ptypes)]
            body = rng.choice([[str(tag)], [str(tag)], ["a"], ["addq", "a", str(tag)], [], ["argsq"],
                               ["negq/s"], ["dupq/s"], ["depthq"], [str(tag), str(tag)], ["sumq"]])
            toks += ["def", "ov", "fn("] + params + ["Any", "["] + body + ["]", ")", ";"]
    items = [gen_item(rng, 2) for _ in range(rng.randint(1, max_len))]
    if overloaded:
        for _ in range(rng.randint(1, 2)):
            items.insert(rng.randint(0, len(items)),
                         [rng.choice(["ov", "ov", "ov", "ov/s", "ov/u", "ov/v", "ov/1", "ov/2"])])
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
    # a row's LEADING `def NAME item ;` clauses bind before the program
    # runs; a program that begins with the WORD needs something before it
    if items and items[0][:1] == ["def"]:
        items.insert(0, [rng.choice(LITS)])
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
    # a line the notation cannot read fails the probe's run, not its output
    if not os.path.exists(go_out):
        sys.stderr.write("go probe failed:\n" + r.stdout[-3000:] + r.stderr[-3000:])
        return 2
    go = open(go_out).read().split("\n")

    # The model nests every call, so a program that recurses without end
    # costs it a long tape before its budget runs out. The search reads
    # "ran out" as one answer whichever limit was met, so it gives the
    # model a smaller budget than the corpus runner's; a generated program
    # that terminates does so in a few hundred steps.
    lean_env = dict(os.environ, CORESPEC_FUEL=os.environ.get("CORESPEC_FUEL", "20000"))
    r = subprocess.run(["lake", "exe", "corespec", "--probe", inp], cwd=SPEC, capture_output=True, text=True,
                       env=lean_env)
    if r.returncode != 0:
        sys.stderr.write("lean probe failed:\n" + r.stdout[-3000:] + r.stderr[-3000:])
        return 2
    lean = r.stdout.split("\n")

    # A program that does not terminate ends on a RESOURCE limit, and which
    # one it meets first is the engine's accounting, not the language: the
    # kernel has a step budget and a tape ceiling, the model only fuel. The
    # two engines agree on such a program when both ran out.
    resource = ("ERROR:evaluation_limit", "ERROR:tape_exhausted")
    diffs, unmodelled, exhausted, malformed = [], 0, 0, 0
    for i, p in enumerate(progs):
        g = go[i] if i < len(go) else "<missing>"
        l = lean[i] if i < len(lean) else "<missing>"
        if l in ("ERROR:unmodelled",):
            unmodelled += 1
            continue
        if g in resource and l in resource:
            exhausted += 1
            continue
        # a program the notation cannot read is no program: each runner
        # says so in its own words
        if g.startswith("NOTATION") and l.startswith("NOTATION"):
            malformed += 1
            continue
        if g != l:
            diffs.append((p, g, l))
    print(f"seed {args.seed}: {len(progs)} programs, {len(diffs)} differ, {unmodelled} outside the model"
          + (f", {exhausted} ran out of a resource in both" if exhausted else "")
          + (f", {malformed} unreadable in both" if malformed else ""))
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
