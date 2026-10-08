package modules

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math/bits"

	"github.com/boru-lang/boru/lang/go/native"
)

// sign63Mask clears the sign bit of a 64-bit hash so the result is a
// non-negative boru Integer (0 … 2^63-1). Probabilistic structures index
// with `hash mod m`, and a negative dividend would produce a negative
// index — so the hash words deliberately return non-negative values.
const sign63Mask = 0x7FFFFFFFFFFFFFFF

// BuildBinaryModule creates the "boru:bin-util" native module — rotates,
// bit-counting, single-bit operators, and slice/construct routines.
// The core bitwise operators (band, bor, bxor, bnot, bsl, bsr, busr)
// are boru built-ins; this module covers the second tier.
//
// After import, words are accessed via dot notation: BinUtil.popcount,
// BinUtil.rotl, BinUtil.test, etc. The `b` prefix is dropped on module words
// because the `bin.` qualifier disambiguates.
//
// See design/BINARY-OPERATIONS.10.md.
func BuildBinaryModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newModuleRegistry("boru:bin-util", binaryModuleNatives)
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// The core bitwise operators (band/bor/bxor/bnot/bsl/bsr/busr) moved into
	// this module; register them in the sub-registry and export them too.
	for _, n := range native.BitwiseModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := delegatingExports(native.BitwiseModuleNatives, subReg)

	// Unary Integer -> Integer.
	for _, name := range []string{"popcount", "clz", "ctz", "bitlen", "mask", "reverse", "swap"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TInteger, native.TInteger))
	}

	// Unary Integer -> Boolean.
	exports.Set("parity", makeTypedFnDef("parity", subReg, native.TBoolean, native.TInteger))

	// Binary Integer Integer -> Integer.
	for _, name := range []string{"rotl", "rotr", "set", "clear", "toggle"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TInteger, native.TInteger, native.TInteger))
	}

	// Binary Integer Integer -> Boolean.
	exports.Set("test", makeTypedFnDef("test", subReg, native.TBoolean, native.TInteger, native.TInteger))

	// Ternary Integer Integer Integer -> Integer.
	exports.Set("extract", makeTypedFnDef("extract", subReg, native.TInteger, native.TInteger, native.TInteger, native.TInteger))

	// Quaternary Integer Integer Integer Integer -> Integer.
	exports.Set("insert", makeTypedFnDef("insert", subReg, native.TInteger, native.TInteger, native.TInteger, native.TInteger, native.TInteger))

	// Character codes: String -> Integer (`ord`) and Integer -> String
	// (`chr`). These replace the O(95) printable-ASCII alphabet trick that
	// every char-code-needing boru library otherwise has to roll by hand.
	// See §9.8 in the DX report.
	exports.Set("ord", makeTypedFnDef("ord", subReg, native.TInteger, native.TString))
	exports.Set("chr", makeTypedFnDef("chr", subReg, native.TString, native.TInteger))

	// Non-cryptographic string hashes: String -> Integer. FNV-1a, the
	// standard library's hash/fnv. `fnv32` returns the full 32-bit hash
	// (always a non-negative Integer); `fnv64` returns the 64-bit hash
	// with its sign bit cleared (non-negative, see sign63Mask) so it is
	// directly usable as `hash mod m` in bloom/sketch/dedup libraries.
	// See §9.9 in the DX report.
	exports.Set("fnv32", makeTypedFnDef("fnv32", subReg, native.TInteger, native.TString))
	exports.Set("fnv64", makeTypedFnDef("fnv64", subReg, native.TInteger, native.TString))

	// Binary-safe text encodings: String -> String. base64 (RFC 4648
	// standard alphabet, padded) and hex (lowercase). `*-encode` maps the
	// raw bytes of the input string to their textual form; `*-decode`
	// inverts it and errors on malformed input. These are the
	// most-universal stdlib batteries (16/20 TIOBE languages ship base64)
	// and the canonical way to carry binary data through text channels —
	// API tokens, content addressing, embedding bytes in JSON.
	// See design/legacy/BATTERIES-INCLUDED-REPORT.5.ignore (Phase 1, encoding).
	exports.Set("base64-encode", makeTypedFnDef("base64-encode", subReg, native.TString, native.TString))
	exports.Set("base64-decode", makeTypedFnDef("base64-decode", subReg, native.TString, native.TString))
	exports.Set("hex-encode", makeTypedFnDef("hex-encode", subReg, native.TString, native.TString))
	exports.Set("hex-decode", makeTypedFnDef("hex-decode", subReg, native.TString, native.TString))

	return moduleDesc(parent, "BinUtil", subReg, exports), nil
}

// ---- handlers ----

func intArg(v native.Value) (int64, error) {
	return v.AsConcreteInteger()
}

// binaryModuleNatives holds the NativeFunc registrations for the
// module's words. Note the handler convention for binary ops:
// `value rotl count` → args[1]=value, args[0]=count.
var binaryModuleNatives = []native.NativeFunc{
	// --- bit counting (unary) ---
	{
		Name: "popcount",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.OnesCount64(uint64(x))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "clz",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.LeadingZeros64(uint64(x))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "ctz",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.TrailingZeros64(uint64(x))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "parity",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewBoolean(bits.OnesCount64(uint64(x))%2 == 1)}, nil
			}),
			Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name: "bitlen",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.Len64(uint64(x))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- slice / construct (unary) ---
	{
		Name: "mask",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				if n <= 0 {
					return []native.Value{native.NewInteger(0)}, nil
				}
				if n >= 64 {
					return []native.Value{native.NewInteger(-1)}, nil
				}
				return []native.Value{native.NewInteger(int64((uint64(1) << uint(n)) - 1))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "reverse",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.Reverse64(uint64(x))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "swap",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.ReverseBytes64(uint64(x))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- rotates (binary) ---
	{
		Name: "rotl",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				// bits.RotateLeft64 reduces n mod 64 internally and
				// accepts negative shifts (rotates the other way).
				return []native.Value{native.NewInteger(int64(bits.RotateLeft64(uint64(x), int(n%64))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "rotr",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.RotateLeft64(uint64(x), -int(n%64))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- single-bit ops (binary) ---
	{
		Name: "test",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.test: bit index out of range [0, 64): %d", n), "test")
				}
				return []native.Value{native.NewBoolean((uint64(x)>>uint(n))&1 != 0)}, nil
			}),
			Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name: "set",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.set: bit index out of range [0, 64): %d", n), "set")
				}
				return []native.Value{native.NewInteger(int64(uint64(x) | (uint64(1) << uint(n))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "clear",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.clear: bit index out of range [0, 64): %d", n), "clear")
				}
				return []native.Value{native.NewInteger(int64(uint64(x) &^ (uint64(1) << uint(n))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "toggle",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.toggle: bit index out of range [0, 64): %d", n), "toggle")
				}
				return []native.Value{native.NewInteger(int64(uint64(x) ^ (uint64(1) << uint(n))))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- slice / construct (ternary, quaternary) ---
	//
	// `BinUtil.extract value lo hi` → bits [lo, hi) of value.
	// Per §1.4 dispatch, `x op a b` lands as args[0]=a, args[1]=b,
	// args[2]=x — so args[0]=lo, args[1]=hi, args[2]=value.
	{
		Name: "extract",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				lo, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				hi, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[2])
				if err != nil {
					return nil, err
				}
				if lo < 0 || hi > 64 || lo > hi {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.extract: invalid bit range [%d, %d)", lo, hi), "extract")
				}
				width := uint(hi - lo)
				if width == 0 {
					return []native.Value{native.NewInteger(0)}, nil
				}
				var mask uint64
				if width >= 64 {
					mask = ^uint64(0)
				} else {
					mask = (uint64(1) << width) - 1
				}
				return []native.Value{native.NewInteger(int64((uint64(x) >> uint(lo)) & mask))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// `BinUtil.insert value lo hi bits` → value with bits at [lo, hi)
	// replaced by the low (hi - lo) bits of `bits`.
	// Per §1.4 dispatch: args[0]=lo, args[1]=hi, args[2]=bits, args[3]=value.
	{
		Name: "insert",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TInteger, native.TInteger, native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				lo, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				hi, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				bits_, err := intArg(args[2])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[3])
				if err != nil {
					return nil, err
				}
				if lo < 0 || hi > 64 || lo > hi {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.insert: invalid bit range [%d, %d)", lo, hi), "insert")
				}
				width := uint(hi - lo)
				if width == 0 {
					return []native.Value{native.NewInteger(x)}, nil
				}
				var fieldMask uint64
				if width >= 64 {
					fieldMask = ^uint64(0)
				} else {
					fieldMask = (uint64(1) << width) - 1
				}
				shifted := (uint64(bits_) & fieldMask) << uint(lo)
				clear := uint64(x) &^ (fieldMask << uint(lo))
				return []native.Value{native.NewInteger(int64(clear | shifted))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// --- character codes ---
	// `BinUtil.ord s` → the Unicode codepoint of the first rune of s.
	{
		Name: "ord",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TString},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				rs := []rune(s)
				if len(rs) == 0 {
					return nil, r.BoruError("range_error", "BinUtil.ord: empty string has no codepoint", "ord")
				}
				return []native.Value{native.NewInteger(int64(rs[0]))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// `BinUtil.chr n` → the single-rune string for codepoint n.
	{
		Name: "chr",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				if n < 0 || n > 0x10FFFF {
					return nil, r.BoruError("range_error",
						fmt.Sprintf("BinUtil.chr: codepoint %d out of range [0, 0x10FFFF]", n), "chr")
				}
				return []native.Value{native.NewString(string(rune(n)))}, nil
			}),
			Returns: []*native.Type{native.TString}, BarrierPos: -1,
		}},
	},
	// --- non-cryptographic string hashes (FNV-1a) ---
	// `BinUtil.fnv32 s` → the 32-bit FNV-1a hash of s (non-negative).
	{
		Name: "fnv32",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TString},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				h := fnv.New32a()
				_, _ = h.Write([]byte(s))
				return []native.Value{native.NewInteger(int64(h.Sum32()))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// `BinUtil.fnv64 s` → the 64-bit FNV-1a hash of s, sign bit cleared so the
	// result is a non-negative Integer usable directly as `hash mod m`.
	{
		Name: "fnv64",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TString},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				h := fnv.New64a()
				_, _ = h.Write([]byte(s))
				return []native.Value{native.NewInteger(int64(h.Sum64() & sign63Mask))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// --- binary-safe text encodings (RFC 4648) ---
	// `BinUtil.base64-encode s` → the standard-alphabet, padded base64 of
	// the raw bytes of s.
	{
		Name: "base64-encode",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TString},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewString(base64.StdEncoding.EncodeToString([]byte(s)))}, nil
			}),
			Returns: []*native.Type{native.TString}, BarrierPos: -1,
		}},
	},
	// `BinUtil.base64-decode s` → the bytes encoded by the standard-alphabet,
	// padded base64 string s, as a String. Malformed input errors.
	{
		Name: "base64-decode",

		// Pure codec: the DryPassReturns mirror flags a top-level decode
		// of a provably-malformed literal with the runtime's decode_error.
		Signatures: []native.Signature{{
			Args:      []*native.Type{native.TString},
			Impl:      native.Go(base64DecodeHandler),
			ReturnsFn: native.DryPassReturns(base64DecodeHandler, native.TString),
			Returns:   []*native.Type{native.TString}, BarrierPos: -1,
		}},
	},
	// `BinUtil.hex-encode s` → the lowercase hexadecimal of the raw bytes of s.
	{
		Name: "hex-encode",

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TString},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewString(hex.EncodeToString([]byte(s)))}, nil
			}),
			Returns: []*native.Type{native.TString}, BarrierPos: -1,
		}},
	},
	// `BinUtil.hex-decode s` → the bytes encoded by the hexadecimal string s,
	// as a String. Odd-length or non-hex input errors.
	{
		Name: "hex-decode",

		// Pure codec: see base64-decode.
		Signatures: []native.Signature{{
			Args:      []*native.Type{native.TString},
			Impl:      native.Go(hexDecodeHandler),
			ReturnsFn: native.DryPassReturns(hexDecodeHandler, native.TString),
			Returns:   []*native.Type{native.TString}, BarrierPos: -1,
		}},
	},
}

// The decode handlers are NAMED so the sigs wire the same function as both
// the runtime Impl and the check-mode DryPassReturns mirror.
func base64DecodeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, err
	}
	b, derr := base64.StdEncoding.DecodeString(s)
	if derr != nil {
		return nil, r.BoruError("decode_error",
			fmt.Sprintf("BinUtil.base64-decode: invalid base64: %v", derr), "base64-decode")
	}
	return []native.Value{native.NewString(string(b))}, nil
}

func hexDecodeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, err
	}
	b, derr := hex.DecodeString(s)
	if derr != nil {
		return nil, r.BoruError("decode_error",
			fmt.Sprintf("BinUtil.hex-decode: invalid hex: %v", derr), "hex-decode")
	}
	return []native.Value{native.NewString(string(b))}, nil
}
