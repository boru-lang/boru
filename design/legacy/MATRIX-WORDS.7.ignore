# Matrix Operations Word Design for boru

## Context

boru has comprehensive scalar math builtins (add, sub, mul, div, pow, sqrt, trig, etc.) and planned dataframe words for tabular data. This design introduces matrix operations — a distinct domain covering linear algebra, signal processing, ML feature engineering, and scientific computing. While boru is a scripting/query language, a pragmatic subset of matrix operations enables useful workflows: computing correlations, transforming coordinates, solving linear systems, and preparing data for ML pipelines.

---

## Analysis: Go Platform Capabilities and Realistic Performance

### What Go Offers

| Capability | Details |
|---|---|
| **`gonum/mat`** | Production-quality dense matrix library. BLAS/LAPACK-backed with CGO; pure Go fallback otherwise. Dense matrices use flat `[]float64` slices (cache-friendly, no GC pressure). |
| **Pure Go performance** | Without CGO/OpenBLAS: ~2-10x slower than numpy for dense BLAS ops. With CGO+OpenBLAS: comparable to numpy. |
| **`math` stdlib** | All element-wise functions (sin, cos, exp, log, etc.) available; no external dependency needed. |
| **Goroutines** | Parallel element-wise operations trivial to implement; useful for matrices above ~1000 elements. |
| **Memory model** | Flat `[]float64` backing; no boxing overhead per element. A 1000x1000 matrix is ~8MB contiguous. |

### Key Design Decision: Dedicated Matrix Type vs. Nested Lists

**Recommendation: Add a dedicated `Scalar/Number/Matrix` type backed by `gonum/mat.Dense`.**

Reasons:

1. **Performance**: Nested `[]Value` lists box every float64 into a Value struct (ID + Type + interface{}). A 100x100 matrix as nested lists = 10,000 Value objects (~640KB overhead). A `mat.Dense` uses a single 80KB `[]float64` slice.
2. **Correctness**: Matrix multiply, transpose, decompositions all require contiguous storage. Reconstructing from nested lists on every operation is wasteful.
3. **Composability**: A distinct type enables clean signature dispatch. `mul` on `[Matrix Matrix]` triggers matrix multiply; `mul` on `[Matrix Number]` triggers scalar multiplication.
4. **Precedent**: The temporal design adds 6+ types under `Scalar/Time`. One type under `Scalar/Number` is conservative.

### Performance Target

Matrices up to ~5000x5000 should be responsive (sub-second for basic operations, a few seconds for decompositions). Well within gonum's capabilities.

---

## Type Hierarchy Addition

```
Scalar/
  Number/
    Integer
    Decimal
    Matrix       -- NEW: float64 dense matrix (gonum/mat.Dense backing)
```

---

## Word Design

### Notation

- `Mat` = Matrix, `I` = Integer, `D` = Decimal, `N` = Number (Integer or Decimal), `L` = List, `B` = Boolean, `S` = String

---

### 1. Construction (13 words)

#### `matrix` (NEW)
Create matrix from list of row-lists, or from flat list with dimensions.
```
Signatures:
  [list]                -> [matrix]
  [list integer integer] -> [matrix]

Examples:
  [[1 2] [3 4]] matrix          # 2x2 matrix
  [1 2 3 4 5 6] matrix 2 3     # 2x3 matrix from flat list
```

#### `mat-zeros` (NEW)
Zero matrix of given dimensions.
```
Signature:
  [integer integer] -> [matrix]

Example:
  mat-zeros 3 3                 # 3x3 zero matrix
```

#### `mat-ones` (NEW)
Matrix of all ones.
```
Signature:
  [integer integer] -> [matrix]

Example:
  mat-ones 2 4                  # 2x4 matrix of ones
```

#### `mat-eye` (NEW)
Identity matrix of given size.
```
Signature:
  [integer] -> [matrix]

Example:
  mat-eye 3                     # 3x3 identity matrix
```

#### `mat-diag` (NEW)
Diagonal matrix from list of values.
```
Signature:
  [list] -> [matrix]

Example:
  [1 2 3] mat-diag              # 3x3 diagonal matrix
```

#### `mat-fill` (NEW)
Matrix filled with a constant value.
```
Signature:
  [integer integer number] -> [matrix]

Example:
  mat-fill 3 3 7.0              # 3x3 matrix of 7.0
```

#### `mat-rand` (NEW)
Matrix of uniform random values in [0,1).
```
Signature:
  [integer integer] -> [matrix]

Example:
  mat-rand 4 4                  # 4x4 random matrix
```

#### `mat-randn` (NEW)
Matrix of standard normal random values.
```
Signature:
  [integer integer] -> [matrix]

Example:
  mat-randn 100 10              # 100x10 standard normal matrix
```

#### `mat-linspace` (NEW)
List of N evenly spaced values from start to end.
```
Signature:
  [number number integer] -> [list]

Example:
  mat-linspace 0 1 5            # [0 0.25 0.5 0.75 1]
```

#### `mat-range` (NEW)
List of values from start to end with step.
```
Signature:
  [number number number] -> [list]

Example:
  mat-range 0 10 2              # [0 2 4 6 8]
```

#### `mat-from-cols` (NEW)
Create matrix from list of column lists.
```
Signature:
  [list] -> [matrix]

Example:
  [[1 3] [2 4]] mat-from-cols   # 2x2 matrix (columns are [1,3] and [2,4])
```

#### `mat-from-table` (NEW)
Extract numeric columns from a table as a matrix.
```
Signature:
  [table list] -> [matrix]

Example:
  people mat-from-table [age sales]
```

---

### 2. Shape & Info (6 words)

#### `mat-rows` (NEW)
Number of rows.
```
Signature:
  [matrix] -> [integer]

Example:
  m mat-rows                    # 3
```

#### `mat-cols` (NEW)
Number of columns.
```
Signature:
  [matrix] -> [integer]

Example:
  m mat-cols                    # 4
```

#### `mat-shape` (NEW)
Pushes rows then cols onto stack.
```
Signature:
  [matrix] -> [integer integer]

Example:
  m mat-shape                   # 3 4
```

#### `mat-size` (NEW)
Total element count (rows * cols).
```
Signature:
  [matrix] -> [integer]

Example:
  m mat-size                    # 12
```

#### `mat-square?` (NEW)
Is the matrix square?
```
Signature:
  [matrix] -> [boolean]

Example:
  m mat-square?                 # true/false
```

#### `mat-symmetric?` (NEW)
Is the matrix symmetric? (within float tolerance)
```
Signature:
  [matrix] -> [boolean]

Example:
  m mat-symmetric?              # true/false
```

---

### 3. Element Access (8 words)

#### `mat-at` (NEW)
Get element at row, col (0-indexed).
```
Signature:
  [matrix integer integer] -> [decimal]

Example:
  m mat-at 1 2                  # element at row 1, col 2
```

#### `mat-set` (NEW)
Set element, returns new matrix.
```
Signature:
  [matrix integer integer number] -> [matrix]

Example:
  m mat-set 1 2 99.0
```

#### `mat-row` (NEW)
Extract a row as a list.
```
Signature:
  [matrix integer] -> [list]

Example:
  m mat-row 0                   # first row as list
```

#### `mat-col` (NEW)
Extract a column as a list.
```
Signature:
  [matrix integer] -> [list]

Example:
  m mat-col 1                   # second column as list
```

#### `mat-diag-of` (NEW)
Extract main diagonal as a list.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-diag-of                 # diagonal elements
```

#### `mat-slice` (NEW)
Submatrix: row-start, row-end, col-start, col-end (0-indexed, exclusive end).
```
Signature:
  [matrix integer integer integer integer] -> [matrix]

Example:
  m mat-slice 0 2 0 2           # top-left 2x2 submatrix
```

#### `mat-set-row` (NEW)
Replace a row.
```
Signature:
  [matrix integer list] -> [matrix]

Example:
  m mat-set-row 0 [10 20 30]
```

#### `mat-set-col` (NEW)
Replace a column.
```
Signature:
  [matrix integer list] -> [matrix]

Example:
  m mat-set-col 1 [10 20 30]
```

---

### 4. Arithmetic (10 words — 4 extend existing)

#### `add` (EXTEND)
Element-wise addition of two matrices, or add scalar to all elements.
```
Signatures:
  [matrix matrix]  -> [matrix]
  [matrix number]  -> [matrix]

Examples:
  a b add                       # element-wise addition
  m 5 add                       # add 5 to all elements
```

#### `sub` (EXTEND)
Element-wise subtraction, or subtract scalar from all elements.
```
Signatures:
  [matrix matrix]  -> [matrix]
  [matrix number]  -> [matrix]

Examples:
  a b sub                       # element-wise subtraction
  m 2 sub                       # subtract 2 from all elements
```

#### `mul` (EXTEND)
Matrix multiplication (not element-wise), or scalar multiplication.
```
Signatures:
  [matrix matrix]  -> [matrix]
  [matrix number]  -> [matrix]

Examples:
  a b mul                       # matrix multiply (a @ b)
  m 3 mul                       # scalar multiply
```

#### `div` (EXTEND)
Element-wise division, or divide all elements by scalar.
```
Signatures:
  [matrix matrix]  -> [matrix]
  [matrix number]  -> [matrix]

Examples:
  a b div                       # element-wise division
  m 2 div                       # divide all elements by 2
```

#### `mat-emul` (NEW)
Element-wise (Hadamard) multiplication.
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-emul                  # element-wise multiply
```

#### `mat-ediv` (NEW)
Element-wise division (explicit name, same as `div` on two matrices).
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-ediv                  # element-wise divide
```

**Design note:** `mul` on two matrices performs matrix multiplication (the standard mathematical meaning, matching numpy `@`, MATLAB `*`, R `%*%`). Element-wise multiply gets the explicit name `mat-emul`. The `add`, `sub`, `div` words on two matrices are element-wise because that is their standard meaning (dimensions must match).

---

### 5. Element-wise Math — Existing Words Extended (22 overloads)

All existing unary and binary math builtins gain automatic `[matrix] -> [matrix]` or `[matrix matrix] -> [matrix]` overloads. Each applies the scalar operation element-wise to every element of the matrix. No new word names are introduced — these are additional signatures on existing words.

**Implementation:** Extend `registerUnaryNumOp` and `registerBinaryNumOp` in `registry.go` to add a `[TMatrix]` or `[TMatrix, TMatrix]` signature alongside the existing Integer/Decimal ones. This gives all registered unary/binary math ops matrix support with zero changes to individual `native_math_*.go` files.

#### Unary ops: `[matrix] -> [matrix]` (14 overloads)

| Word | Element-wise operation | Example |
|---|---|---|
| `abs` | Absolute value of each element | `m abs` |
| `negate` | Negate each element | `m negate` |
| `ceil` | Ceiling of each element | `m ceil` |
| `floor` | Floor of each element | `m floor` |
| `round` | Round each element | `m round` |
| `trunc` | Truncate each element | `m trunc` |
| `sqrt` | Square root of each element | `m sqrt` |
| `cbrt` | Cube root of each element | `m cbrt` |
| `exp` | e^x for each element | `m exp` |
| `log` | Natural log of each element | `m log` |
| `log2` | Base-2 log of each element | `m log2` |
| `log10` | Base-10 log of each element | `m log10` |
| `sin` | Sine of each element | `m sin` |
| `cos` | Cosine of each element | `m cos` |
| `tan` | Tangent of each element | `m tan` |
| `asin` | Arcsine of each element | `m asin` |
| `acos` | Arccosine of each element | `m acos` |
| `atan` | Arctangent of each element | `m atan` |

**Note:** `sign` is excluded — it returns Integer, not Decimal, so element-wise application would lose the matrix type. Use `mat-apply [sign]` instead.

#### Binary ops: `[matrix matrix] -> [matrix]` (4 overloads)

| Word | Element-wise operation | Example |
|---|---|---|
| `mod` | Modulo of corresponding elements | `a b mod` |
| `min` | Min of corresponding elements | `a b min` |
| `max` | Max of corresponding elements | `a b max` |
| `pow` | Power of corresponding elements | `a b pow` |

These also get `[matrix number] -> [matrix]` scalar broadcast overloads:

| Word | Scalar broadcast | Example |
|---|---|---|
| `mod` | Modulo each element by scalar | `m 3 mod` |
| `min` | Clamp each element to at most N | `m 100 min` |
| `max` | Clamp each element to at least N | `m 0 max` |
| `pow` | Raise each element to scalar power | `m 2 pow` |

`mat-apply` (section 11) remains available as the escape hatch for arbitrary quoted functions that aren't registered as builtins.

---

### 7. Transpose & Reshape (6 words)

#### `mat-t` (NEW)
Transpose.
```
Signature:
  [matrix] -> [matrix]

Example:
  m mat-t                       # transpose
```

#### `mat-reshape` (NEW)
Reshape to new dimensions (element count must match).
```
Signature:
  [matrix integer integer] -> [matrix]

Example:
  m mat-reshape 6 2             # reshape to 6x2
```

#### `mat-flatten` (NEW)
Flatten to a list of decimals (row-major order).
```
Signature:
  [matrix] -> [list]

Example:
  m mat-flatten                 # [1 2 3 4 5 6]
```

#### `mat-hstack` (NEW)
Horizontal concatenation (same row count).
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-hstack                # side by side
```

#### `mat-vstack` (NEW)
Vertical concatenation (same col count).
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-vstack                # stacked vertically
```

#### `mat-squeeze` (NEW)
If 1xN or Nx1, return list; if 1x1, return scalar.
```
Signature:
  [matrix] -> [list] or [decimal]

Example:
  m mat-squeeze                 # list or scalar depending on shape
```

---

### 8. Linear Algebra (7 words)

#### `mat-det` (NEW)
Determinant (square matrices only).
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-det                     # determinant
```

#### `mat-inv` (NEW)
Matrix inverse.
```
Signature:
  [matrix] -> [matrix]

Example:
  m mat-inv                     # inverse
```

#### `mat-trace` (NEW)
Sum of diagonal elements.
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-trace                   # trace
```

#### `mat-rank` (NEW)
Matrix rank (via SVD).
```
Signature:
  [matrix] -> [integer]

Example:
  m mat-rank                    # rank
```

#### `mat-norm` (NEW)
Matrix norm. Default is Frobenius; optionally specify "fro", "1", or "inf".
```
Signatures:
  [matrix]        -> [decimal]
  [matrix string] -> [decimal]

Examples:
  m mat-norm                    # Frobenius norm
  m mat-norm "1"                # 1-norm
```

#### `mat-cond` (NEW)
Condition number (ratio of largest to smallest singular value).
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-cond                    # condition number
```

---

### 9. Decompositions (6 words)

Each decomposition pushes results onto the stack.

#### `mat-lu` (NEW)
LU decomposition: pushes L, U, P (permutation).
```
Signature:
  [matrix] -> [matrix matrix matrix]

Example:
  m mat-lu                      # L U P on stack
```

#### `mat-qr` (NEW)
QR decomposition: pushes Q, R.
```
Signature:
  [matrix] -> [matrix matrix]

Example:
  m mat-qr                      # Q R on stack
```

#### `mat-svd` (NEW)
SVD: pushes U, S (diagonal as matrix), V^T.
```
Signature:
  [matrix] -> [matrix matrix matrix]

Example:
  m mat-svd                     # U S V^T on stack
```

#### `mat-chol` (NEW)
Cholesky decomposition (symmetric positive definite): lower triangular L.
```
Signature:
  [matrix] -> [matrix]

Example:
  m mat-chol                    # lower triangular L
```

#### `mat-eigen` (NEW)
Eigendecomposition: pushes eigenvalues (list), eigenvectors (columns of matrix).
```
Signature:
  [matrix] -> [list matrix]

Example:
  m mat-eigen                   # eigenvalues eigenvectors on stack
```

#### `mat-svd-vals` (NEW)
Singular values only (more efficient than full SVD).
```
Signature:
  [matrix] -> [list]

Example:
  m mat-svd-vals                # list of singular values
```

---

### 10. Solving (3 words)

#### `mat-solve` (NEW)
Solve Ax=B for x (A is square, B is matrix or column vector).
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-solve                 # x such that Ax = B
```

#### `mat-lstsq` (NEW)
Least-squares solution to Ax~B (overdetermined systems).
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-lstsq                 # least-squares x
```

#### `mat-pinv` (NEW)
Moore-Penrose pseudoinverse.
```
Signature:
  [matrix] -> [matrix]

Example:
  m mat-pinv                    # pseudoinverse
```

---

### 11. Aggregation (10 words)

#### `mat-sum` (NEW)
Sum of all elements.
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-sum
```

#### `mat-mean` (NEW)
Mean of all elements.
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-mean
```

#### `mat-min` (NEW)
Minimum element.
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-min
```

#### `mat-max` (NEW)
Maximum element.
```
Signature:
  [matrix] -> [decimal]

Example:
  m mat-max
```

#### `mat-row-sum` (NEW)
Sum of each row, returned as list.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-row-sum
```

#### `mat-col-sum` (NEW)
Sum of each column.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-col-sum
```

#### `mat-row-mean` (NEW)
Mean of each row.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-row-mean
```

#### `mat-col-mean` (NEW)
Mean of each column.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-col-mean
```

#### `mat-row-min` (NEW)
Min of each row.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-row-min
```

#### `mat-col-max` (NEW)
Max of each column.
```
Signature:
  [matrix] -> [list]

Example:
  m mat-col-max
```

---

### 12. Comparison & Logic (6 words)

#### `mat-eq?` (NEW)
Element-wise equality within tolerance (all elements match).
```
Signature:
  [matrix matrix] -> [boolean]

Example:
  a b mat-eq?
```

#### `mat-close?` (NEW)
All elements within given tolerance.
```
Signature:
  [matrix matrix decimal] -> [boolean]

Example:
  a b 1e-10 mat-close?
```

#### `mat-gt` (NEW)
Element-wise greater-than, returns 0/1 matrix.
```
Signature:
  [matrix number] -> [matrix]

Example:
  m 0 mat-gt                    # 1 where element > 0, else 0
```

#### `mat-lt` (NEW)
Element-wise less-than, returns 0/1 matrix.
```
Signature:
  [matrix number] -> [matrix]

Example:
  m 5 mat-lt                    # 1 where element < 5, else 0
```

#### `mat-any?` (NEW)
Any non-zero element?
```
Signature:
  [matrix] -> [boolean]

Example:
  m mat-any?
```

#### `mat-all?` (NEW)
All elements non-zero?
```
Signature:
  [matrix] -> [boolean]

Example:
  m mat-all?
```

---

### 13. Advanced (8 words)

#### `mat-dot` (NEW)
Dot product of two lists (vectors).
```
Signature:
  [list list] -> [decimal]

Example:
  [1 2 3] [4 5 6] mat-dot      # 32
```

#### `mat-cross` (NEW)
Cross product of two 3-element lists.
```
Signature:
  [list list] -> [list]

Example:
  [1 0 0] [0 1 0] mat-cross    # [0 0 1]
```

#### `mat-outer` (NEW)
Outer product of two lists.
```
Signature:
  [list list] -> [matrix]

Example:
  [1 2] [3 4 5] mat-outer      # 2x3 matrix
```

#### `mat-kron` (NEW)
Kronecker product.
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  a b mat-kron
```

#### `mat-apply` (NEW)
Apply element-wise function (quoted code) to each element.
```
Signature:
  [matrix list] -> [matrix]

Example:
  m mat-apply [sqrt]            # element-wise sqrt
```

#### `mat-map-row` (NEW)
Apply function to each row (as list), collect results as list.
```
Signature:
  [matrix list] -> [list]

Example:
  m mat-map-row [sum]           # sum of each row
```

#### `mat-map-col` (NEW)
Apply function to each column (as list), collect results.
```
Signature:
  [matrix list] -> [list]

Example:
  m mat-map-col [mean]          # mean of each column
```

#### `mat-conv` (NEW)
2D convolution (useful for image/signal processing kernels).
```
Signature:
  [matrix matrix] -> [matrix]

Example:
  image kernel mat-conv
```

---

## Word Summary

| Category | Count | New | Extended |
|---|---|---|---|
| Construction | 13 | 13 | 0 |
| Shape & Info | 6 | 6 | 0 |
| Element Access | 8 | 8 | 0 |
| Arithmetic | 10 | 2 | 4 (add, sub, mul, div) |
| Element-wise Math | 22 overloads | 0 | 22 (18 unary + 4 binary, via registry helpers) |
| Transpose & Reshape | 6 | 6 | 0 |
| Linear Algebra | 7 | 7 | 0 |
| Decompositions | 6 | 6 | 0 |
| Solving | 3 | 3 | 0 |
| Aggregation | 10 | 10 | 0 |
| Comparison & Logic | 6 | 6 | 0 |
| Advanced | 8 | 8 | 0 |
| **Total** | **83 words + 22 overloads** | **75 new** | **26 extended** |

---

## Composable Workflow Examples

### Basic matrix creation and multiplication
```boru
set a ([[1 2] [3 4]] matrix)
set b (mat-eye 2)
a b mul
# result: [[1 2] [3 4]] (identity multiplication)
```

### Solving a linear system (2x + 3y = 8, x + y = 3)
```boru
set coeffs ([[2 3] [1 1]] matrix)
set rhs ([[8] [3]] matrix)
coeffs rhs mat-solve
# result: [[1] [2]] meaning x=1, y=2
```

### Computing column means from a table
```boru
set data (people mat-from-table [age sales])
data mat-col-mean
# list of means for each column
```

### PCA-style workflow (center, covariance, eigendecompose)
```boru
set X (data matrix)
set means (X mat-col-mean)
# center the data: subtract column means
set centered (X [1] matrix means matrix sub)
# covariance matrix
set cov (centered mat-t centered mul)
set n (X mat-rows sub 1)
cov n div
# eigendecomposition
cov mat-eigen
# eigenvalues and eigenvectors now on stack
```

### Element-wise math (existing words, no mat-apply needed)
```boru
set m ([[1 4 9] [16 25 36]] matrix)
m sqrt                          # [[1 2 3] [4 5 6]]
m log                           # element-wise natural log
m 2 pow                         # square each element
m abs                           # absolute value of each element
m 0 max                         # clamp negatives to zero (ReLU)
```

### Element-wise function application (for arbitrary quoted code)
```boru
set m ([[1 4 9] [16 25 36]] matrix)
m mat-apply [sqrt]
# [[1 2 3] [4 5 6]]
```

### Composing with dataframe words
```boru
# Correlation matrix from table columns
set data (sales-data mat-from-table [price volume returns])
set means (data mat-col-mean)
# ... center, normalize, compute correlation matrix
```

---

## Implementation Priority

### Phase 1 — Core (essential, enables basic workflows)

| Words | Count |
|---|---|
| `matrix`, `mat-zeros`, `mat-ones`, `mat-eye`, `mat-diag`, `mat-fill` | 6 |
| `mat-rows`, `mat-cols`, `mat-shape`, `mat-size` | 4 |
| `mat-at`, `mat-row`, `mat-col`, `mat-slice` | 4 |
| `add`/`sub`/`mul`/`div` extensions (Mat-Mat, Mat-N), `mat-emul` | 5 |
| `mat-t`, `mat-reshape`, `mat-flatten` | 3 |
| `mat-sum`, `mat-mean`, `mat-min`, `mat-max` | 4 |
| `mat-dot` | 1 |
| Element-wise math overloads via `registerUnaryNumOp`/`registerBinaryNumOp` (abs, negate, sqrt, sin, cos, etc.) | 22 overloads |
| **Phase 1 Total** | **27 words + 22 overloads** |

### Phase 2 — Linear Algebra (decompositions and solving)

| Words | Count |
|---|---|
| `mat-det`, `mat-inv`, `mat-trace`, `mat-rank`, `mat-norm` | 5 |
| `mat-solve`, `mat-lstsq` | 2 |
| `mat-lu`, `mat-qr`, `mat-svd`, `mat-eigen` | 4 |
| **Phase 2 Total** | **11 words** |

### Phase 3 — Extended Operations

| Words | Count |
|---|---|
| `mat-rand`, `mat-randn`, `mat-linspace`, `mat-range` | 4 |
| `mat-hstack`, `mat-vstack`, `mat-squeeze` | 3 |
| `mat-row-sum`, `mat-col-sum`, `mat-row-mean`, `mat-col-mean`, `mat-row-min`, `mat-col-max` | 6 |
| `mat-set`, `mat-set-row`, `mat-set-col`, `mat-diag-of` | 4 |
| `mat-square?`, `mat-symmetric?` | 2 |
| **Phase 3 Total** | **19 words** |

### Phase 4 — Advanced

| Words | Count |
|---|---|
| `mat-eq?`, `mat-close?`, `mat-gt`, `mat-lt`, `mat-any?`, `mat-all?` | 6 |
| `mat-cross`, `mat-outer`, `mat-kron`, `mat-conv` | 4 |
| `mat-chol`, `mat-svd-vals`, `mat-pinv`, `mat-cond` | 4 |
| `mat-apply`, `mat-map-row`, `mat-map-col` | 3 |
| `mat-from-cols`, `mat-from-table`, `mat-ediv` | 3 |
| **Phase 4 Total** | **20 words** |

---

## Files to Create

Following the existing `native_*.go` one-file-per-category pattern:

- `lang/go/internal/engine/native_matrix_construct.go` — matrix, mat-zeros, mat-ones, mat-eye, mat-diag, mat-fill, mat-rand, mat-randn, mat-linspace, mat-range, mat-from-cols, mat-from-table
- `lang/go/internal/engine/native_matrix_info.go` — mat-rows, mat-cols, mat-shape, mat-size, mat-square?, mat-symmetric?
- `lang/go/internal/engine/native_matrix_access.go` — mat-at, mat-set, mat-row, mat-col, mat-diag-of, mat-slice, mat-set-row, mat-set-col
- `lang/go/internal/engine/native_matrix_arithmetic.go` — mat-emul, mat-ediv, plus add/sub/mul/div signature extensions
- `lang/go/internal/engine/native_matrix_transform.go` — mat-t, mat-reshape, mat-flatten, mat-hstack, mat-vstack, mat-squeeze
- `lang/go/internal/engine/native_matrix_linalg.go` — mat-det, mat-inv, mat-trace, mat-rank, mat-norm, mat-cond
- `lang/go/internal/engine/native_matrix_decomp.go` — mat-lu, mat-qr, mat-svd, mat-svd-vals, mat-chol, mat-eigen
- `lang/go/internal/engine/native_matrix_solve.go` — mat-solve, mat-lstsq, mat-pinv
- `lang/go/internal/engine/native_matrix_aggregate.go` — mat-sum, mat-mean, mat-min, mat-max, mat-row-sum, mat-col-sum, mat-row-mean, mat-col-mean, mat-row-min, mat-col-max
- `lang/go/internal/engine/native_matrix_compare.go` — mat-eq?, mat-close?, mat-gt, mat-lt, mat-any?, mat-all?
- `lang/go/internal/engine/native_matrix_advanced.go` — mat-dot, mat-cross, mat-outer, mat-kron, mat-conv, mat-apply, mat-map-row, mat-map-col

## Files to Modify

- `lang/go/internal/engine/types.go` — add `"Scalar/Number/Matrix": 38` to `builtinTypeIDs`, add `TMatrix` well-known constant, add `"Matrix": "Scalar/Number/Matrix"` to `typeAncestry`
- `lang/go/internal/engine/value.go` — add `MatrixData` struct, `NewMatrix()` constructor, `AsMatrix()` accessor, matrix display in `String()` method
- `lang/go/internal/engine/registry.go` — add `registerMatrix*` calls in `registerBuiltins()`; extend `registerUnaryNumOp` to add `[TMatrix] -> [TMatrix]` overload; extend `registerBinaryNumOp` to add `[TMatrix, TMatrix] -> [TMatrix]` and `[TMatrix, TNumber] -> [TMatrix]` overloads
- `lang/go/internal/engine/native_math_add.go` — extend with `[TMatrix, TMatrix]` and `[TMatrix, TNumber]` signatures
- `lang/go/internal/engine/native_math_sub.go` — same pattern
- `lang/go/internal/engine/native_math_mul.go` — extend with matrix multiply `[TMatrix, TMatrix]` and scalar `[TMatrix, TNumber]`
- `lang/go/internal/engine/native_math_div.go` — extend with element-wise `[TMatrix, TMatrix]` and scalar `[TMatrix, TNumber]`
- `lang/go.mod` — add `gonum.org/v1/gonum` dependency
