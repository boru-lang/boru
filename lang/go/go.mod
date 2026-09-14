module github.com/boru-lang/boru/lang/go

go 1.24.7

require (
	github.com/antchfx/xpath v1.3.6
	github.com/boru-lang/boru/basic/go v0.0.0
	github.com/boru-lang/boru/check/go v0.0.0
	github.com/boru-lang/boru/compiler/go v0.0.0
	github.com/boru-lang/boru/core/go v0.0.0
	github.com/boru-lang/boru/eng/go v0.0.0
	github.com/cockroachdb/apd/v3 v3.2.3
	github.com/creack/pty v1.1.24
	github.com/fsnotify/fsnotify v1.10.1
	github.com/itchyny/gojq v0.12.19
	github.com/ohler55/ojg v1.28.1
	github.com/rjrodger/aontu/go v0.1.6
	github.com/tabnas/abnf/go v0.4.2
	github.com/tabnas/csv/go v0.5.2
	github.com/tabnas/expr/go v0.5.2
	github.com/tabnas/feed/go v0.6.2
	github.com/tabnas/ini/go v0.5.2
	github.com/tabnas/json/go v0.5.2
	github.com/tabnas/json5/go v0.5.2
	github.com/tabnas/jsonc/go v0.5.2
	github.com/tabnas/jsonic/go v0.6.2
	github.com/tabnas/markdown/go v0.6.2
	github.com/tabnas/multisource/go v0.5.2
	github.com/tabnas/parser/go v0.8.3
	github.com/tabnas/toml/go v0.5.2
	github.com/tabnas/xml/go v0.7.2
	github.com/tabnas/yaml/go v0.5.2
	github.com/tabnas/zon/go v0.5.2
	github.com/voxgig/model/go v0.1.3-0.20260622172642-ee04212555c1
	golang.org/x/term v0.36.0
	golang.org/x/text v0.21.0
	modernc.org/sqlite v1.46.1
)

require (
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/tabnas/bnf/go v0.1.2 // indirect
	github.com/tabnas/directive/go v0.5.2 // indirect
	github.com/tabnas/hoover/go v0.3.2 // indirect
	github.com/tabnas/path/go v0.3.2 // indirect
)

require (
	github.com/boru-lang/boru/parser/go v0.0.0
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/voxgig/struct/go v0.1.2
	github.com/voxgig/udk/go v0.1.2
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sys v0.38.0
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/boru-lang/boru/eng/go => ../../eng/go

replace github.com/boru-lang/boru/basic/go => ../../basic/go

replace github.com/boru-lang/boru/core/go => ../../core/go

replace github.com/boru-lang/boru/check/go => ../../check/go

replace github.com/boru-lang/boru/compiler/go => ../../compiler/go

replace github.com/boru-lang/boru/parser/go => ../../parser/go
