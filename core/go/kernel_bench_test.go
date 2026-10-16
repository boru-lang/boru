package core_test

import (
	"strconv"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Kernel performance baseline (perf-baseline suite). One benchmark per
// kernel primitive that dominates interpreter hot paths: value
// comparison, unification, canonical rendering, and the OrderedMap the
// Map payload is built on. Run with:
//
//	go test -bench 'BenchmarkKernel' -benchmem -run '^$' .
//
// or `make bench` from the repo root. These pin the CURRENT cost of
// each primitive so an algorithmic regression (a new allocation in
// CompareValues, a quadratic walk in canon) shows up as a step change
// against the committed baseline rather than anecdote. (Moved here from
// eng/go when the compiled lane was removed, 2026-10-16.)

func benchScalarPairs() [][2]core.Value {
	return [][2]core.Value{
		{core.NewInteger(7), core.NewInteger(9)},
		{core.NewFloat(1.5), core.NewInteger(2)},
		{core.NewString("alpha"), core.NewString("beta")},
		{core.NewBoolean(true), core.NewBoolean(false)},
		{core.NewAtom("x"), core.NewAtom("y")},
	}
}

func benchList(n int) core.Value {
	elems := make([]core.Value, n)
	for i := range elems {
		elems[i] = core.NewInteger(int64(i))
	}
	return core.NewList(elems)
}

func benchMap(n int) core.Value {
	m := core.NewOrderedMap()
	for i := 0; i < n; i++ {
		m.Set("k"+strconv.Itoa(i), core.NewInteger(int64(i)))
	}
	return core.NewMap(m)
}

func BenchmarkKernelCompareScalars(b *testing.B) {
	pairs := benchScalarPairs()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := range pairs {
			_, _ = core.CompareValues(pairs[i][0], pairs[i][1])
		}
	}
}

func BenchmarkKernelCompareList64(b *testing.B) {
	a, c := benchList(64), benchList(64)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, _ = core.CompareValues(a, c)
	}
}

func BenchmarkKernelCompareMap64(b *testing.B) {
	a, c := benchMap(64), benchMap(64)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, _ = core.CompareValues(a, c)
	}
}

func BenchmarkKernelUnifyScalarType(b *testing.B) {
	v := core.NewInteger(42)
	t := core.NewTypeLiteral(core.TInteger)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, ok := core.Unify(v, t); !ok {
			b.Fatal("unify failed")
		}
	}
}

func BenchmarkKernelUnifyList32(b *testing.B) {
	a, c := benchList(32), benchList(32)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, ok := core.Unify(a, c); !ok {
			b.Fatal("unify failed")
		}
	}
}

func BenchmarkKernelCanonScalar(b *testing.B) {
	v := core.NewString("hello, world")
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = core.CanonValue(v)
	}
}

func BenchmarkKernelCanonNested(b *testing.B) {
	m := core.NewOrderedMap()
	m.Set("xs", benchList(16))
	m.Set("ys", benchMap(8))
	v := core.NewMap(m)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = core.CanonValue(v)
	}
}

func BenchmarkKernelOrderedMapSetGet(b *testing.B) {
	keys := make([]string, 64)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}
	val := core.NewInteger(1)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		m := core.NewOrderedMap()
		for _, k := range keys {
			m.Set(k, val)
		}
		for _, k := range keys {
			m.Get(k)
		}
	}
}
