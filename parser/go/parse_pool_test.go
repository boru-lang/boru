package parser

import (
	"fmt"
	"sync"
	"testing"
)

// Parse reuses configured parsers (parserPool). These pin what reuse must
// not change: a parser that has parsed anything — a valid program, a
// syntax error, an XML literal whose interpolation parsed a nested program —
// parses the next source exactly as a freshly built one does, and
// concurrent parses each get an instance of their own.

var poolSources = []string{
	`def fib fn n:Integer Integer [if (n lte 1) [n] [(fib (n sub 1)) add (fib (n sub 2))]] fib 10`,
	`{a: (1 add 2) b: [x y] c: 'z'}`,
	"`a ${1 add 2} b`",
	`<p class="x">${1 add 2}</p>`,
	`m.a.b/v ; x:Integer => [x mul 2]`,
	`[:Integer] {:String}`,
	`'unterminated`,
	`(1 add`,
}

// renderParse is a parse rendered for comparison: the values or the error.
func renderParse(src string) string {
	vals, err := Parse(src)
	if err != nil {
		return "ERR " + err.Error()
	}
	return fmt.Sprint(vals)
}

// renderFresh parses on a parser built for this call alone, bypassing the
// pool: the reference a reused parser must match.
func renderFresh(src string) string {
	j := newBoruParser()
	result, err := j.Parse(src)
	vals, cerr := convertParsed(result, err, src)
	if cerr != nil {
		return "ERR " + cerr.Error()
	}
	return fmt.Sprint(vals)
}

func TestParsePoolReuseMatchesAFreshParser(t *testing.T) {
	// Twice through, so every source after the first parses on a parser
	// that has already parsed the others, errors included.
	for round := 0; round < 2; round++ {
		for _, src := range poolSources {
			if got, want := renderParse(src), renderFresh(src); got != want {
				t.Errorf("round %d %q:\n  reused %s\n  fresh  %s", round, src, got, want)
			}
		}
	}
}

func TestParsePoolConcurrentParsesAgree(t *testing.T) {
	want := make([]string, len(poolSources))
	for i, src := range poolSources {
		want[i] = renderFresh(src)
	}
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < 20; k++ {
				i := (g + k) % len(poolSources)
				if got := renderParse(poolSources[i]); got != want[i] {
					errs <- fmt.Sprintf("goroutine %d %q: %s, want %s", g, poolSources[i], got, want[i])
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
