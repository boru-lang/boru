package native

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_fs.go holds the boru:io filesystem-structure words — stat, list, remove,
// move, and copy — plus the shared record/option helpers. Handlers route
// every filesystem touch through EffectiveFileOps(r) (so the in-memory
// backing engages under context.__sys.fs) and reuse extractPath for target
// routing. Mutating words call r.NoteEffect() (the effect ledger).

// mapBoolOpt reads one boolean key from an options map, returning def when the
// value is not a concrete map or the key is absent. It is the single option
// reader all of the io words share.
func mapBoolOpt(opts Value, key string, def bool) bool {
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return def
	}
	m, _ := AsMap(opts)
	if m == nil {
		return def
	}
	if v, ok := MapFieldBoolean(m, key); ok {
		return v
	}
	return def
}

// mtimeUnix renders a modification time as Unix seconds, mapping the zero
// time (an unset in-memory mtime) to 0 rather than a large negative number.
func mtimeUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// buildStatRecord turns a host FileInfo into the boru stat record: a Map with
// name/path/type/size/mode/mtime (and target for a symlink). `type` is a
// FileType atom so `(stat p).type is IO.FileType` holds.
func buildStatRecord(fi capabilities.FileInfo, path string, fileType *Type) Value {
	om := NewOrderedMap()
	om.Set("name", NewString(fi.Name))
	om.Set("path", NewString(path))
	om.Set("type", newFileTypeAtom(fileTypeName(fi), fileType))
	om.Set("size", NewInteger(fi.Size))
	om.Set("mode", NewInteger(int64(fi.Mode.Perm())))
	om.Set("mtime", NewInteger(mtimeUnix(fi.ModTime)))
	// owner / group are the uid/gid, or -1 where the backend cannot know
	// them (Windows, a mounted filesystem without owner fields).
	om.Set("owner", NewInteger(int64(fi.UID)))
	om.Set("group", NewInteger(int64(fi.GID)))
	if fi.Symlink {
		om.Set("target", NewString(fi.Target))
	}
	return NewMap(om)
}

// statShapeReturns is stat's check-mode result model: the same dynamic
// Map∪None union ReturnsDynUnion declared before it, with the Map
// alternative SHAPED — a record schema of buildStatRecord's fixed field
// set — so a field read through the union resolves its declared type
// (getNodeReturns' dynamic-disjunct schema branch) instead of widening
// at dynamic(Any). Conditional fields are declared too (target rides
// symlinks, xattr rides {xattr:true}): a schema read is GRADUAL
// (recordSchemaFieldReturns), so a field absent at run time reads
// optimistically and is discharged by a guard, never claimed strictly.
// Everything is built INSIDE the closure — fresh value identities per
// invocation (the def-slot aliasing lesson from #343's read-line
// incident).
func statShapeReturns(fileType *Type) ReturnsFunc {
	return func(_ []Value, _ *Registry) []Value {
		f := NewOrderedMap()
		f.Set("name", NewTypeLiteral(TString))
		f.Set("path", NewTypeLiteral(TString))
		f.Set("type", NewTypeLiteral(fileType))
		f.Set("size", NewTypeLiteral(TInteger))
		f.Set("mode", NewTypeLiteral(TInteger))
		f.Set("mtime", NewTypeLiteral(TInteger))
		f.Set("owner", NewTypeLiteral(TInteger))
		f.Set("group", NewTypeLiteral(TInteger))
		f.Set("target", NewTypeLiteral(TString))
		f.Set("xattr", NewTypeLiteral(TMap))
		return []Value{NewDynamicCarrierValue(NewDisjunct([]Value{
			NewRecordType(f), NewTypeLiteral(TNone),
		}))}
	}
}

// spaceShapeReturns is space's check-mode result model — the record
// schema of spaceHandler's filesystem-stats map, dynamic like every
// schema claim over a runtime-populated container. Built inside the
// closure for the same identity discipline as statShapeReturns.
func spaceShapeReturns() ReturnsFunc {
	return func(_ []Value, _ *Registry) []Value {
		f := NewOrderedMap()
		f.Set("total", NewTypeLiteral(TInteger))
		f.Set("free", NewTypeLiteral(TInteger))
		f.Set("available", NewTypeLiteral(TInteger))
		f.Set("bsize", NewTypeLiteral(TInteger))
		f.Set("type", NewTypeLiteral(TString))
		return []Value{NewDynamicCarrierValue(NewRecordType(f))}
	}
}

// xattrValue renders one attribute payload: a String when the bytes are
// valid UTF-8, Bytes otherwise — the mount read-payload convention.
func xattrValue(v []byte) Value {
	if utf8.Valid(v) {
		return NewString(string(v))
	}
	return NewBytesValue(v)
}

// boruToHostXattr / hostToBoruXattr map plain boru attribute names onto the
// host's namespace: Linux user xattrs live in the mandatory `user.`
// namespace, Darwin uses flat names (capabilities.XattrNamespacePrefix).
// The word layer owns this mapping, so boru programs spell `note` on every
// platform. hostToBoruXattr additionally reports whether the host name IS
// ours — attributes outside the namespace (security.selinux and friends)
// are not displayable or settable through the io words and are skipped.
func boruToHostXattr(name string) string {
	return capabilities.XattrNamespacePrefix + name
}

func hostToBoruXattr(host string) (string, bool) {
	return stripXattrNS(capabilities.XattrNamespacePrefix, host)
}

// stripXattrNS is hostToBoruXattr's platform-independent core, split out
// so BOTH prefix regimes (Linux's "user." and the flat "" of Darwin and
// the fallback platforms) are testable on any build.
func stripXattrNS(prefix, host string) (string, bool) {
	if prefix == "" {
		return host, true
	}
	if rest, ok := strings.CutPrefix(host, prefix); ok {
		return rest, true
	}
	return "", false
}

// attachXattrs adds the {xattr:{…}} sub-map to a stat record. A backend
// that does not support xattrs (the non-unix OS arm, an unextended
// mount) surfaces its compile failure as the stat error rather than a silent
// empty map — absence of support and absence of attributes must not
// look alike.
func attachXattrs(r *Registry, om Value, path string) error {
	names, err := EffectiveFileOps(r).XattrList(path)
	if err != nil {
		return err
	}
	xs := NewOrderedMap()
	for _, host := range names {
		name, ours := hostToBoruXattr(host)
		if !ours {
			continue
		}
		v, gerr := EffectiveFileOps(r).XattrGet(path, host)
		if gerr != nil {
			return gerr
		}
		xs.Set(name, xattrValue(v))
	}
	// om is always the freshly built concrete map from buildStatRecord, so
	// AsMutableMap cannot fail — the blank error is the provably-dead-guard
	// idiom (design/TEST-SEAMS.10.md).
	m, _ := AsMutableMap(om)
	m.Set("xattr", NewMap(xs))
	return nil
}

// statHandler implements IO.stat. It returns a stat record, or `none` when
// the path does not exist (folding in exists/access); other errors (e.g. a
// symlink cycle, a policy denial) surface loudly. follow defaults true
// (dereference a symlink; {follow:false} for lstat); {resolve:true} sets
// .path to the absolute resolved form.
func statHandler(args []Value, r *Registry, fileType *Type, hasOpts bool) ([]Value, error) {
	follow, resolve, xattr := true, false, false
	if hasOpts {
		follow = mapBoolOpt(args[1], "follow", true)
		resolve = mapBoolOpt(args[1], "resolve", false)
		xattr = mapBoolOpt(args[1], "xattr", false)
	}
	path := extractPath(args[0])
	fi, err := EffectiveFileOps(r).Stat(path, follow)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return nil, r.BoruError("stat_error", fmt.Sprintf("stat: %v", err), "stat")
	}
	displayPath := path
	if resolve {
		if rp, rerr := EffectiveFileOps(r).ResolvePath(path); rerr == nil {
			displayPath = rp
		}
	}
	rec := buildStatRecord(fi, displayPath, fileType)
	if xattr {
		if xerr := attachXattrs(r, rec, path); xerr != nil {
			return nil, r.BoruError("stat_error", fmt.Sprintf("stat: %v", xerr), "stat")
		}
	}
	return []Value{rec}, nil
}

// listEntry pairs a directory entry's metadata with its path relative to the
// listed root (the bare name for a shallow listing, a slash-joined path for a
// recursive walk).
type listEntry struct {
	info    capabilities.FileInfo
	relPath string
}

// collectEntries lists root's children (recursing into subdirectories when
// recursive is set), prefixing relative paths with prefix.
func collectEntries(r *Registry, root, prefix string, recursive bool) ([]listEntry, error) {
	infos, err := EffectiveFileOps(r).ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]listEntry, 0, len(infos))
	for _, fi := range infos {
		rel := fi.Name
		if prefix != "" {
			rel = prefix + "/" + fi.Name
		}
		out = append(out, listEntry{info: fi, relPath: rel})
		if recursive && fi.IsDir {
			child, cerr := collectEntries(r, root+"/"+fi.Name, rel, recursive)
			if cerr != nil {
				return nil, cerr
			}
			out = append(out, child...)
		}
	}
	return out, nil
}

// listHandler implements IO.list (readdir/walk). It returns a List of name
// strings, or of stat records with {detail:true}; {recursive:true} walks the
// whole subtree; {match:"*.txt"} keeps only entries whose relPath matches the
// glob (policy.Glob vocabulary: `*`/`?` stop at `/`, `**` spans components —
// so a recursive walk filters with `**/x.txt` shapes; character classes are
// NOT special, `[ab]` matches only the literal text `[ab]`).
func listHandler(args []Value, r *Registry, fileType *Type, hasOpts bool) ([]Value, error) {
	detail, recursive := false, false
	match, hasMatch := "", false
	if hasOpts {
		detail = mapBoolOpt(args[1], "detail", false)
		recursive = mapBoolOpt(args[1], "recursive", false)
		match, hasMatch = mapStrOpt(args[1], "match")
	}
	path := extractPath(args[0])
	entries, err := collectEntries(r, path, "", recursive)
	if err != nil {
		return nil, r.BoruError("list_error", fmt.Sprintf("list: %v", err), "list")
	}
	items := make([]Value, 0, len(entries))
	for _, e := range entries {
		if hasMatch && !policy.Glob(match, e.relPath) {
			continue
		}
		if detail {
			items = append(items, buildStatRecord(e.info, e.relPath, fileType))
		} else {
			items = append(items, NewString(e.relPath))
		}
	}
	return []Value{NewList(items)}, nil
}

// doRemoveWord implements IO.remove: {recursive:true} removes a directory
// tree, {force:true} ignores an absent path. Returns the target it removed.
func doRemoveWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	recursive, force := false, false
	if hasOpts {
		recursive = mapBoolOpt(args[1], "recursive", false)
		force = mapBoolOpt(args[1], "force", false)
	}
	path := extractPath(args[0])
	r.NoteEffect()
	err := EffectiveFileOps(r).Remove(path, recursive)
	if err != nil {
		if force && errors.Is(err, os.ErrNotExist) {
			return []Value{returnPath(args[0])}, nil
		}
		return nil, r.BoruError("remove_error", fmt.Sprintf("remove: %v", err), "remove")
	}
	return []Value{returnPath(args[0])}, nil
}

func ioRemoveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doRemoveWord(args, r, false)
}

func ioRemoveOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doRemoveWord(args, r, true)
}

// refuseOverwrite is the shared {overwrite:false} guard for copy and move:
// when the option is present-and-false and the destination exists, decline
// with an exists-class error. This is a handler-layer check (rename(2) has
// no portable no-replace flag), so it is best-effort against a concurrent
// creator — the TOCTOU window is documented, matching Java's
// StandardCopyOption posture rather than an atomicity promise.
func refuseOverwrite(r *Registry, opts Value, dst, word string) error {
	if mapBoolOpt(opts, "overwrite", true) {
		return nil
	}
	if _, err := EffectiveFileOps(r).Stat(dst, false); err == nil {
		return r.BoruError(word+"_error",
			fmt.Sprintf("%s: destination %q already exists (overwrite is false)", word, dst), word)
	}
	return nil
}

// doMoveWord implements IO.move (rename/move); {overwrite:false} declines an
// existing destination. Returns the destination.
func doMoveWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	src := extractPath(args[0])
	dst := extractPath(args[1])
	if hasOpts {
		if err := refuseOverwrite(r, args[2], dst, "move"); err != nil {
			return nil, err
		}
	}
	r.NoteEffect()
	if err := EffectiveFileOps(r).Rename(src, dst); err != nil {
		return nil, r.BoruError("move_error", fmt.Sprintf("move: %v", err), "move")
	}
	return []Value{returnPath(args[1])}, nil
}

func moveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doMoveWord(args, r, false)
}

func moveOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doMoveWord(args, r, true)
}

// doCopy copies src to dst: a symlink is recreated, a directory is copied
// recursively (requires recursive), a file's bytes are read and written.
func doCopy(r *Registry, src, dst string, recursive bool) error {
	ops := EffectiveFileOps(r)
	fi, err := ops.Stat(src, false)
	if err != nil {
		return err
	}
	switch {
	case fi.Symlink:
		// Overwrite parity with the file case (WriteFile truncates an existing
		// file): the {overwrite:false} guard already ran in doCopyWord, so an
		// existing destination is replaced rather than triggering EEXIST from
		// Symlink. Removing an existing non-empty directory still declines, the
		// same as writing a file over a directory.
		if _, sErr := ops.Stat(dst, false); sErr == nil {
			if rmErr := ops.Remove(dst, false); rmErr != nil {
				return rmErr
			}
		}
		return ops.Symlink(fi.Target, dst)
	case fi.IsDir:
		if !recursive {
			return fmt.Errorf("%q is a directory (use {recursive:true})", src)
		}
		return copyTree(r, src, dst)
	default:
		data, rerr := ops.ReadFile(src)
		if rerr != nil {
			return rerr
		}
		return ops.WriteFile(dst, data, fi.Mode.Perm())
	}
}

// copyTree recursively copies a directory, delegating each child to doCopy.
func copyTree(r *Registry, src, dst string) error {
	ops := EffectiveFileOps(r)
	if err := ops.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := ops.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := doCopy(r, src+"/"+e.Name, dst+"/"+e.Name, true); err != nil {
			return err
		}
	}
	return nil
}

// doCopyWord implements IO.copy. {recursive:true} copies a directory tree;
// {overwrite:false} declines an existing destination. Returns the destination.
func doCopyWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	src := extractPath(args[0])
	dst := extractPath(args[1])
	recursive := false
	if hasOpts {
		recursive = mapBoolOpt(args[2], "recursive", false)
		if err := refuseOverwrite(r, args[2], dst, "copy"); err != nil {
			return nil, err
		}
	}
	r.NoteEffect()
	if err := doCopy(r, src, dst, recursive); err != nil {
		return nil, r.BoruError("copy_error", fmt.Sprintf("copy: %v", err), "copy")
	}
	return []Value{returnPath(args[1])}, nil
}

func copyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doCopyWord(args, r, false)
}

func copyOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doCopyWord(args, r, true)
}

// mapIntOpt reads one integer key from an options map, reporting presence.
func mapIntOpt(opts Value, key string) (int64, bool) {
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return 0, false
	}
	m, _ := AsMap(opts)
	if m == nil {
		return 0, false
	}
	return MapFieldInteger(m, key)
}

// doLinkWord implements IO.link: a symbolic link (default) or a hard link
// ({hard:true}) at dst referring to src. Returns dst.
func doLinkWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	target := extractPath(args[0])
	link := extractPath(args[1])
	hard := false
	if hasOpts {
		hard = mapBoolOpt(args[2], "hard", false)
	}
	r.NoteEffect()
	var err error
	if hard {
		err = EffectiveFileOps(r).Link(target, link)
	} else {
		err = EffectiveFileOps(r).Symlink(target, link)
	}
	if err != nil {
		return nil, r.BoruError("link_error", fmt.Sprintf("link: %v", err), "link")
	}
	return []Value{returnPath(args[1])}, nil
}

func linkHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doLinkWord(args, r, false)
}

func linkOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doLinkWord(args, r, true)
}

// applyTouch is IO.touch's core: create the file if absent (Unix touch), then
// apply the metadata options {mode, mtime, atime, size}. It is a standalone
// function so its error branches are directly reachable in tests.
func applyTouch(ops capabilities.FileOps, path string, opts Value, hasOpts bool) error {
	if _, err := ops.Stat(path, false); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if werr := ops.WriteFile(path, []byte{}, 0o644); werr != nil {
			return werr
		}
	}
	if !hasOpts {
		return nil
	}
	if mode, ok := mapIntOpt(opts, "mode"); ok {
		if err := ops.Chmod(path, os.FileMode(mode)); err != nil {
			return err
		}
	}
	// {size} is applied BEFORE {mtime}/{atime}: truncate(2) itself updates
	// the file's mtime (on the real filesystem and, faithfully, in mem), so
	// an explicit {mtime} must land after the resize to win.
	if size, ok := mapIntOpt(opts, "size"); ok {
		if err := ops.Truncate(path, size); err != nil {
			return err
		}
	}
	mtime, hasM := mapIntOpt(opts, "mtime")
	atime, hasA := mapIntOpt(opts, "atime")
	if hasM || hasA {
		mt, at := time.Unix(mtime, 0), time.Unix(atime, 0)
		if !hasM {
			mt = at
		}
		if !hasA {
			at = mt
		}
		if err := ops.Chtimes(path, at, mt); err != nil {
			return err
		}
	}
	// {owner} / {group} re-own the entry (an omitted id stays unchanged —
	// the chown -1 convention); {xattr:{k:v}} sets extended attributes.
	owner, hasO := mapIntOpt(opts, "owner")
	group, hasG := mapIntOpt(opts, "group")
	if hasO || hasG {
		uid, gid := -1, -1
		if hasO {
			uid = int(owner)
		}
		if hasG {
			gid = int(group)
		}
		if err := ops.Chown(path, uid, gid, true); err != nil {
			return err
		}
	}
	if xm, ok := mapMapOpt(opts, "xattr"); ok {
		for _, name := range xm.Keys() {
			v, _ := xm.Get(name)
			var payload []byte
			if b, isB := AsBytesValue(v); isB {
				payload = b
			} else {
				s, serr := v.AsConcreteString()
				if serr != nil {
					return fmt.Errorf("touch: xattr %q value must be a String or Bytes, got %s", name, v.Parent)
				}
				payload = []byte(s)
			}
			if err := ops.XattrSet(path, boruToHostXattr(name), payload); err != nil {
				return err
			}
		}
	}
	return nil
}

// mapMapOpt reads one map-valued key from an options map, reporting presence.
func mapMapOpt(opts Value, key string) (ReadMap, bool) {
	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return nil, false
	}
	m, _ := AsMap(opts)
	if m == nil {
		return nil, false
	}
	v, ok := m.Get(key)
	if !ok {
		return nil, false
	}
	inner, err := AsMap(v)
	if err != nil || inner == nil {
		return nil, false
	}
	return inner, true
}

// doTouchWord implements IO.touch (the chmod/utimes/truncate/touch setter).
// Returns the path.
func doTouchWord(args []Value, r *Registry, hasOpts bool) ([]Value, error) {
	path := extractPath(args[0])
	var opts Value
	if hasOpts {
		opts = args[1]
	}
	r.NoteEffect()
	if err := applyTouch(EffectiveFileOps(r), path, opts, hasOpts); err != nil {
		return nil, r.BoruError("touch_error", fmt.Sprintf("touch: %v", err), "touch")
	}
	return []Value{returnPath(args[0])}, nil
}

func touchHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTouchWord(args, r, false)
}

func touchOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTouchWord(args, r, true)
}
