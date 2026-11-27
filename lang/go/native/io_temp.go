package native

import (
	"fmt"
)

// io_temp.go — IO.temp (unique temp files/directories) and IO.space
// (volume/disk information), the stateless tail of the filesystem batch
// API. Both route through EffectiveFileOps like every io word, so the
// mem/overlay toggles and policy gates apply uniformly.

// doTempWord implements IO.temp: create a unique file (default) or
// directory ({dir:true}), optionally inside {in:Pathon}, named
// {prefix}…{suffix}. Returns the created path as a Pathon.
func doTempWord(args []Value, r *Registry) ([]Value, error) {
	dir, prefix, suffix := "", "", ""
	wantDir := mapBoolOpt(args[0], "dir", false)
	if p, ok := mapStrOpt(args[0], "prefix"); ok {
		prefix = p
	}
	if sfx, ok := mapStrOpt(args[0], "suffix"); ok {
		suffix = sfx
	}
	if m, mok := AsMap(args[0]); mok == nil && m != nil {
		if v, ok := m.Get("in"); ok && IsPathon(v) {
			dir = extractPath(v)
		}
	}
	pattern := prefix + "*" + suffix
	var path string
	var err error
	if wantDir {
		path, err = EffectiveFileOps(r).TempDir(dir, pattern)
	} else {
		path, err = EffectiveFileOps(r).TempFile(dir, pattern)
	}
	if err != nil {
		return nil, r.BoruError("temp_error", fmt.Sprintf("temp: %v", err), "temp")
	}
	return []Value{NewPathonFromString(path)}, nil
}

func tempOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doTempWord(args, r)
}

// spaceHandler implements IO.space: the volume holding a Pathon, as a
// map {total free available bsize type}. Values are backend-defined
// (the mem filesystem reports a synthetic 1 GiB volume) — shape is the
// contract, numbers are advisory.
func spaceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	fs, err := EffectiveFileOps(r).Statfs(path)
	if err != nil {
		return nil, r.BoruError("space_error", fmt.Sprintf("space: %v", err), "space")
	}
	om := NewOrderedMap()
	om.Set("total", NewInteger(int64(fs.TotalBytes)))
	om.Set("free", NewInteger(int64(fs.FreeBytes)))
	om.Set("available", NewInteger(int64(fs.AvailBytes)))
	om.Set("bsize", NewInteger(fs.BlockSize))
	om.Set("type", NewString(fs.Type))
	return []Value{NewMap(om)}, nil
}
