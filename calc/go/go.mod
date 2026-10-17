module github.com/boru-lang/boru/calc/go

go 1.24.7

require github.com/boru-lang/boru/core/go v0.0.0

require (
	github.com/boru-lang/boru/parser/go v0.0.0
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/tabnas/json/go v0.5.2 // indirect
	github.com/tabnas/jsonic/go v0.6.2 // indirect
	github.com/tabnas/parser/go v0.8.3 // indirect
)

replace github.com/boru-lang/boru/core/go => ../../core/go

replace github.com/boru-lang/boru/check/go => ../../check/go

replace github.com/boru-lang/boru/parser/go => ../../parser/go
