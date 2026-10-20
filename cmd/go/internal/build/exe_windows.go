package build

// exeSuffix is the extension a runnable program needs on this host: Windows
// finds executables by extension, so a default `boru build p.boru` writes
// p.exe there, as `go build` does (exe_other.go has none).
const exeSuffix = ".exe"
