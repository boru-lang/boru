package modules

func init() {
	registerDocs("boru:io", map[string]string{
		"read":      "Read a file; {enc} decodes utf8/bytes/utf16le/utf16be/latin1.",
		"write":     "Write to a file, returning its path; {enc} encodes, {mode:'append'} appends, {atomic:true} replaces via a temp+rename.",
		"printstr":  "Write formatted text to output, leaving no value.",
		"stdin":     "Standard-input handle.",
		"stdout":    "Standard-output handle.",
		"stderr":    "Standard-error handle.",
		"trace":     "Run a list as a traced sub-program, returning its result.",
		"read-line": "Read the next line of a stream or File handle without its terminator; none at EOF.",
		"is-tty":    "Report whether a stream is a terminal (false when the host installed no probe).",
		"exit":      "Ask the driver to terminate with a status (0..125); raises the reserved boru/exit control error rather than calling os.Exit.",
		"env":       "Read one environment variable by name; none when it is unset (\"\" is a real value).",
		"env-all":   "The whole visible environment as a Map, sorted by name; empty when the host installed none.",
		"args":      "The script's command-line arguments (after the script path), as a List of Strings; empty when the host passed none.",
		"folder":    "Create a directory, returning its Path.",
		"stat":      "Describe a path (name/size/type/mode/mtime/owner/group; {xattr:true} attaches attributes), or none if absent.",
		"list":      "List a directory's entries; {detail} for records, {recursive} to walk, {match} to glob-filter.",
		"remove":    "Delete a path; {recursive} for a tree, {force} to ignore absent.",
		"move":      "Rename/move a path, returning the destination; {overwrite:false} refuses an existing one.",
		"copy":      "Copy a path to a destination; {recursive} copies a tree, {overwrite:false} refuses an existing one.",
		"link":      "Link dst to src: a symlink by default, a hard link with {hard}.",
		"touch":     "Create a path if absent and set {mode}/{mtime}/{atime}/{size}/{owner}/{group}/{xattr}.",
		"temp":      "Create a unique temp file (or {dir:true} directory) under {in}, named {prefix}…{suffix}; returns its path.",
		"space":     "Report the volume holding a path as {total free available bsize type}.",
		"open":      "Open a File handle on a path; {mode:read|write|append|rw, create, exclusive, truncate, perm}.",
		"seek":      "Move a File handle's cursor to n relative to {from:start|current|end}; returns the new offset.",
		"flush":     "Sync a File handle or flush a writable Mmap region, returning the handle.",
		"close":     "Close a resource handle — a File, Watcher, Lock, or Mmap.",
		"lock":      "Take an advisory lock on a path; {shared} for a read lock, {block:false} returns none on contention.",
		"unlock":    "Release an advisory Lock.",
		"mmap":      "Map a file's bytes into a Mmap region; {offset, length, writable}. read/write/flush/close drive it.",
		"watch":     "Run a body per change event on a path ({recursive match} opts); returns a Watcher.",
		"unwatch":   "Stop a Watcher, closing its event stream.",
		"mount":     "Install a filesystem: a handler-fn map (the FileOps bridge), or a .zip archive Pathon (read-only, {writable:true} for copy-on-write).",
		"unmount":   "Restore the filesystem that was active before mount.",
	})

	// Paths are Pathon values, not Strings, and that is the single thing
	// a caller most needs shown — a signature of [Path Any] does not say
	// how to build one. Results are from verified lang/spec/module-io.tsv
	// rows, which use the in-memory filesystem so they touch no disk.
	registerExamples("boru:io", map[string][]string{
		"write": {
			`IO.write (make Pathon "/tmp/a.txt") "hello"      ;# returns the Path, so it chains`,
			`;# The path is a Pathon, not a String: ` + "`make Pathon \"…\"`" + ` or the`,
			`;# literal form. "mem://…" writes to the in-memory filesystem.`,
		},
		"read": {
			`IO.read (make Pathon "/tmp/a.txt")               ;# 'hello'`,
			`IO.read (IO.write (make Pathon "/tmp/a.txt") "hello")   ;# write returns its Path`,
		},
		"stat": {
			`(IO.stat (make Pathon "/tmp/a.txt")).size        ;# 5`,
			`(IO.stat (make Pathon "/tmp/a.txt")).type        ;# file/q — or dir/q, link/q`,
		},
		"list":   {`IO.list (make Pathon "/tmp")                     ;# the entries, as Paths`},
		"folder": {`IO.folder (make Pathon "/tmp/build")             ;# create it, parents included`},
		"remove": {`IO.remove (make Pathon "/tmp/a.txt")`},
		"move":   {`IO.move (make Pathon "/tmp/a.txt") (make Pathon "/tmp/b.txt")`},
		"copy":   {`IO.copy (make Pathon "/tmp/a.txt") (make Pathon "/tmp/b.txt")`},
		"open": {
			`def h (IO.open (make Pathon "/tmp/big.log") {mode:"r"})`,
			`IO.seek h 0 "end"  ;# … then IO.read / IO.close h`,
			`;# For whole-file work IO.read/IO.write need no handle at all.`,
		},
		"printstr": {`IO.printstr "hi"                                 ;# writes to stdout with no newline`},
		"trace": {
			`IO.trace [1 add 2 mul 3]                         ;# 9, plus a step-by-step trace on stderr`,
			`;# The value passes through, so a trace can be dropped into a`,
			`;# pipeline without changing what it computes.`,
		},
		"stdout": {`IO.stdout                                        ;# stdout/q — a StreamKind, for the words that take one`},
		"args":   {`IO.args                                          ;# the argv this program was started with`},
		"temp":   {`def p (IO.temp)                                  ;# a fresh temporary Path`},
		"watch": {
			`IO.watch (make Pathon "/tmp") {match:"*.boru"}    ;# events until IO.unwatch`,
		},
	})
}
