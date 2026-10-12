package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct/go"
)

// The "getpath" word is registered via the consolidated Natives slice in
// natives.go.
//
// getpathHandler calls voxgigstruct.GetPath to retrieve a nested value.
func getpathHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	path, err := args[0].AsConcreteString()
	if err != nil {
		return nil, err
	}
	data := valueToAny(args[1])

	result := voxgigstruct.GetPath(path, data)

	val, err := structConvert(result)
	if err != nil {
		return nil, fmt.Errorf("getpath: %w", err)
	}
	return []Value{val}, nil
}

// getpathReachHandler reads a nested value through a Reach lens:
// `getpath $.a.b m` walks the reach's segments against m. Handled natively
// (not via voxgigstruct) so per-segment getr strictness and computed keys
// behave exactly as bare m.a.b — the same primitive `apply` uses.
func getpathReachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	info, err := AsReach(&args[0])
	if err != nil {
		return nil, fmt.Errorf("getpath: %w", err)
	}
	out, err := ApplyReach(r, info, args[1])
	if err != nil {
		return nil, err
	}
	return []Value{out}, nil
}
