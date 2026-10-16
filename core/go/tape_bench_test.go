package core_test

import (
	"strconv"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The Tape against the slice it replaced, on the recursion shape that
// motivated it (design/legacy/RECURSION-PERFORMANCE.10.ignore). One
// simulated call level: splice a 12-token body at the cursor, do 14
// insert+remove pairs there, then leave 8 tokens pending and move on —
// the op mix measured from the real engine. The slice variant drags the
// pending tail on every edit; the Tape keeps the gap at the cursor so the
// tail never moves. (Moved here from eng/go when the compiled lane was
// removed, 2026-10-16; `make bench` runs it.)

func benchSliceLevel(s *[]core.Value, ptr int, body []core.Value) int {
	stackSplice(s, ptr, 0, body...)
	ptr += len(body)
	var v core.Value
	for i := 0; i < 14; i++ {
		stackInsert(s, ptr, v)
		stackRemove(s, ptr)
	}
	return ptr - 8
}

func BenchmarkTapeSliceRecursion(b *testing.B) {
	body := make([]core.Value, 12)
	for depth := 1000; depth <= 4000; depth *= 2 {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				s := make([]core.Value, 0, 64)
				ptr := 0
				for d := 0; d < depth; d++ {
					ptr = benchSliceLevel(&s, ptr, body)
				}
			}
		})
	}
}

func BenchmarkTapeGapRecursion(b *testing.B) {
	body := make([]core.Value, 12)
	for depth := 1000; depth <= 4000; depth *= 2 {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				tp := core.NewTape(nil, 64)
				ptr := 0
				var v core.Value
				for d := 0; d < depth; d++ {
					tp.Splice(ptr, 0, body...)
					ptr += len(body)
					for i := 0; i < 14; i++ {
						tp.Insert(ptr, v)
						tp.Remove(ptr)
					}
					ptr -= 8
				}
			}
		})
	}
}

// ---- former engine primitives (engine_stack.go), retained as the
// benchmark baseline after the engine moved to the gap-buffer Tape ----

func stackInsert(s *[]core.Value, i int, val core.Value) {
	*s = append(*s, core.Value{})
	copy((*s)[i+1:], (*s)[i:len(*s)-1])
	(*s)[i] = val
}

func stackRemove(s *[]core.Value, i int) {
	copy((*s)[i:], (*s)[i+1:])
	(*s)[len(*s)-1] = core.Value{}
	*s = (*s)[:len(*s)-1]
}

func stackSplice(s *[]core.Value, i, count int, replacements ...core.Value) {
	delta := len(replacements) - count
	oldLen := len(*s)
	newLen := oldLen + delta

	if delta > 0 {
		for cap(*s) < newLen {
			*s = append(*s, core.Value{})
		}
		*s = (*s)[:newLen]
		copy((*s)[i+len(replacements):], (*s)[i+count:oldLen])
	} else if delta < 0 {
		copy((*s)[i+len(replacements):], (*s)[i+count:])
		for j := newLen; j < oldLen; j++ {
			(*s)[j] = core.Value{}
		}
		*s = (*s)[:newLen]
	}
	copy((*s)[i:], replacements)
}
