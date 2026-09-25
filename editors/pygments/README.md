# boru for Pygments

`boru_lexer.py` is a [Pygments](https://pygments.org/) lexer for boru
(`.boru`) — a concatenative, strongly-typed query language. Use it to
syntax-highlight boru code blocks in documentation (MkDocs, Sphinx),
with `pygmentize`, or anywhere the Chroma-compatible token names are
consumed.

It highlights: line comments (`#` and `//`) and `/* … */` block
comments; single-, double- and backtick **template** strings (with
`\`-escapes and `${ … }` interpolation of embedded boru); decimal /
hex (`0x`) / binary (`0b`) / big-integer (`0d`) numbers and floats;
the constants `true false none inf nan`; the structural/query
**keywords** and the **builtin** library words; capitalised **type
names / module namespaces**; and the `/q /s /f /r /t /u` (and
`/<digit>` arity) dispatch **modifiers**.

## Install

The lexer is a one-module package that registers the
`pygments.lexers` entry point `boru = boru_lexer:BoruLexer`, so after
installing it `pygmentize -l boru` — and MkDocs / Sphinx — find it
automatically.

```bash
# from a checkout of the repo
cd editors/pygments
pip install .

# ...or straight from GitHub
pip install "git+https://github.com/boru-lang/boru#subdirectory=editors/pygments"
```

Verify the entry point is registered:

```bash
pygmentize -L lexers | grep -i boru       # -> * boru:  boru ...
```

## Use it

**Highlight a file (installed):**

```bash
pygmentize -l boru -f terminal256 path/to/program.boru      # ANSI, for a terminal
pygmentize -l boru -f html -O full -o out.html program.boru # standalone HTML page
```

**Without installing** — point Pygments straight at the module with
`-x` (the `file.py:ClassName` form):

```bash
pygmentize -x -l boru_lexer.py:BoruLexer -f terminal256 program.boru
```

Run this from the `editors/pygments/` directory (or add it to
`PYTHONPATH`) so `boru_lexer.py` is importable.

## MkDocs (`mkdocs-material` / `pymdownx.highlight`)

Install the lexer in the same environment as MkDocs, then reference the
language by its alias in fenced code blocks — no extra config needed
because the entry point makes `boru` a first-class Pygments language:

<pre><code>```boru
def new fn [[opts:Map] [Service] [ add {op: "create"} ]]
```</code></pre>

If you pin lexers explicitly, add `boru` under
`markdown_extensions` → `pymdownx.highlight`:

```yaml
markdown_extensions:
  - pymdownx.highlight:
      use_pygments: true
      # `boru` resolves via the installed entry point; nothing else to do.
  - pymdownx.superfences
```

## Sphinx

Install the lexer in Sphinx's environment. The entry point makes `boru`
usable directly:

```rst
.. code-block:: boru

   def new fn [[opts:Map] [Service] [ add {op: "create"} ]]
```

To make it the default for a whole page, set the `highlight` directive
or `highlight_language` in `conf.py`:

```python
# conf.py
highlight_language = "boru"
```

If you prefer not to install a package, register the class in
`conf.py` instead (put `boru_lexer.py` next to `conf.py` or on
`sys.path`):

```python
# conf.py
from sphinx.highlighting import lexers
from boru_lexer import BoruLexer
lexers["boru"] = BoruLexer()
```

## Token mapping

| boru construct                                   | Pygments token          |
| ----------------------------------------------- | ----------------------- |
| `# …`, `// …`                                    | `Comment.Single`        |
| `/* … */`                                        | `Comment.Multiline`     |
| `'…'` / `"…"` / `` `…` ``                         | `String.{Single,Double,Backtick}` |
| `\n \t \\ \' \" \` \uXXXX`                        | `String.Escape`         |
| `${ … }` in a template                           | `String.Interpol` (delimiters) + inner tokens |
| `0xFF`, `0b101`, `0d123`, `42`, `3.14e-2`        | `Number.{Hex,Bin,Integer→(0d)Number,Integer,Float}` |
| `true false none inf nan` (and `-inf -nan`)      | `Keyword.Constant`      |
| `def fn from where select …`                     | `Keyword`               |
| `add dup sort get set …`                         | `Name.Builtin`          |
| `Integer`, `Emailon`, `MathUtil`, user types     | `Name.Class`            |
| `def Foo …` / `def foo …` binding target         | `Name.Class` / `Name.Function` |
| `/q /s /f /r /t /u`, `/2`, combos like `/sq`     | `Operator`              |
| `. => : \| /` and `!.`                            | `Operator`              |
| `[] {} ()` `;` `@ $ % ,`                          | `Punctuation`           |
| any other word                                   | `Name`                  |

## Develop / self-check

```bash
python3 -m py_compile boru_lexer.py     # compiles clean
# with pygments installed, tokenise a file and assert no Error tokens:
python3 - <<'PY'
from boru_lexer import BoruLexer
from pygments.token import Error
src = open("../../design/examples/todo/todo.boru").read()
assert not any(t is Error for t, _ in BoruLexer().get_tokens(src))
print("ok: no Error tokens")
PY
```

The vocabulary (keywords / builtins / constants) is kept in sync with
the reference Emacs mode [`editors/emacs/boru-mode.el`](../emacs/boru-mode.el)
and with `boru describe`.
