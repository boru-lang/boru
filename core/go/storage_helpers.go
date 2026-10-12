package core

import (
	"fmt"
	"strconv"
)

// GetKey extracts the key string from any key-typed value (Word,
// String, Atom, or any other value via Sprintf fallback). Exported
// so lang's accessor handlers (.dotted notation, getr, the
// production set / get handlers themselves) and any host plugin can
// reuse the same key-coercion rules as the kernel's container
// access.
//
// Numeric and Boolean values render via their canonical
// FormatFloat / FormatInt forms, matching the language's
// printing rules.
func GetKey(v Value) string {
	if IsWord(&v) {
		w, _ := AsWord(&v)
		return w.Name
	}
	if v.Parent.ConformsTo(TString) {
		s, _ := AsString(v)
		return s
	}
	if IsAtom(v) {
		a, _ := AsAtom(v)
		return a
	}
	if v.Parent.ConformsTo(TInteger) {
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	}
	if v.Parent.ConformsTo(TFloat) {
		f, _ := AsFloat(v)
		return FormatFloat(f)
	}
	if v.Parent.ConformsTo(TBoolean) {
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v.Data)
}
