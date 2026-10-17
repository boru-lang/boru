// specfix — the shared spec/check corpus harness.
//
// It is its OWN module, a sibling of eng rather than a package inside it,
// because BOTH eng/go's standalone corpus lanes and test/go's spec runners
// need it. It cannot live in test/go: test/go requires eng, so eng's tests
// importing it from there would close a cycle. And it does not belong in
// eng: it uses core, check and compiler symbols exclusively and not one
// symbol of the bytecode VM, so sitting inside eng only ever reached those
// three through the facade shim.
module github.com/boru-lang/boru/test/specfix

go 1.24.7

require (
	github.com/boru-lang/boru/check/go v0.0.0
	github.com/boru-lang/boru/core/go v0.0.0
	github.com/boru-lang/boru/parser/go v0.0.0
)

require (
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/tabnas/json/go v0.5.2 // indirect
	github.com/tabnas/jsonic/go v0.6.2 // indirect
	github.com/tabnas/parser/go v0.8.3 // indirect
)

replace github.com/boru-lang/boru/core/go => ../../core/go

replace github.com/boru-lang/boru/check/go => ../../check/go

replace github.com/boru-lang/boru/parser/go => ../../parser/go
