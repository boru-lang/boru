/-
  corespec — the specification's runner for the kernel's parity corpus.

  core/spec/*.tsv is the declarative contract core/go and core/ts are
  both held to. This is the third runner: it evaluates every row on the
  Lean model and compares the result with the row's `expected` column.

      lake exe corespec ../core/spec          # every corpus file
      lake exe corespec --probe FILE          # print one result per line
-/
import Boru.Corespec.Notation

open Boru.Corespec

structure Row where
  file : String
  line : Nat
  cols : List String

def readRows (path : System.FilePath) : IO (List Row) := do
  let text ← IO.FS.readFile path
  let lines := text.splitOn "\n"
  let name := path.fileName.getD path.toString
  let mut rows := []
  let mut n := 0
  for l in lines do
    n := n + 1
    let t := l.trimAscii.toString
    if t.isEmpty || t.startsWith "#" then continue
    rows := { file := name, line := n, cols := l.splitOn "\t" } :: rows
  pure rows.reverse

def evalStr (expr : String) (fuel : Nat := Boru.Core.defaultFuel) : String :=
  match eval expr fuel with
  | .ok s => s
  | .error e => "NOTATION:" ++ e

/-- Every corpus file in the directory but the parity debt, in name
    order — read from the directory, as the Go and TypeScript runners read
    it, so a file added to the corpus is run here without being listed. -/
def specFiles (dir : System.FilePath) : IO (List String) := do
  let entries ← dir.readDir
  let names := entries.toList.map (·.fileName)
    |>.filter fun n => n.endsWith ".tsv" && n != "divergent.tsv"
  pure (names.mergeSort (· ≤ ·))

/-- The probe's step budget: the model's own, unless `CORESPEC_FUEL` names
    another. A program that does not terminate runs to the budget, and the
    model — which nests every call — pays for a long tape as it goes; the
    differential search asks for a smaller budget than the corpus needs,
    since it reads "ran out" as one answer whichever limit was met. -/
def probeFuel : IO Nat := do
  match (← IO.getEnv "CORESPEC_FUEL").bind String.toNat? with
  | some n => pure n
  | none => pure Boru.Core.defaultFuel

def runProbe (path : String) : IO UInt32 := do
  let text ← IO.FS.readFile path
  let fuel ← probeFuel
  for l in text.splitOn "\n" do
    if l.isEmpty then continue
    IO.println (evalStr l fuel)
  pure 0

def main (args : List String) : IO UInt32 := do
  match args with
  | ["--probe", path] => runProbe path
  | [dir] =>
    let mut total := 0
    let mut failed := 0
    let files ← specFiles (System.FilePath.mk dir)
    if files.isEmpty then
      IO.println s!"core/spec: no corpus files in {dir}"
      return 1
    for f in files do
      let rows ← readRows (System.FilePath.mk dir / f)
      let mut bad := 0
      for r in rows do
        let expr := r.cols.getD 0 ""
        let want := r.cols.getD 1 ""
        let got := evalStr expr
        total := total + 1
        if got != want then
          bad := bad + 1
          IO.println s!"FAIL {r.file}:{r.line}  {expr}\n  got  {got}\n  want {want}"
      failed := failed + bad
      IO.println s!"{f}: {rows.length - bad}/{rows.length}"
    -- the parity debt: which engine the specification agrees with
    let rows ← readRows (System.FilePath.mk dir / "divergent.tsv")
    let mut go := 0
    let mut ts := 0
    let mut neither := 0
    for r in rows do
      let got := evalStr (r.cols.getD 0 "")
      if got == r.cols.getD 1 "" then go := go + 1
      else if got == r.cols.getD 2 "" then ts := ts + 1
      else
        neither := neither + 1
        IO.println s!"NEITHER {r.file}:{r.line}  {r.cols.getD 0 ""}\n  spec {got}\n  go   {r.cols.getD 1 ""}\n  ts   {r.cols.getD 2 ""}"
    IO.println s!"divergent.tsv: {rows.length} rows — spec agrees with go on {go}, ts on {ts}, neither on {neither}"
    IO.println s!"core/spec: {total - failed}/{total} rows"
    -- a divergent row the specification answers like NEITHER engine has
    -- not been adjudicated: that fails the run too
    pure (if failed == 0 && neither == 0 then 0 else 1)
  | _ =>
    IO.eprintln "usage: corespec <core/spec dir> | corespec --probe <file>"
    pure 2
