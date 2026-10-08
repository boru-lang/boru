package basic

import "testing"

// Bytes opts into the const pool (core.ConstBakeable): immutable, shared
// zero-copy, so a pooled const shares as the interpreter does. Pinned here
// since the seventy-first increment: the stored-fn unit that used to bake
// mini-s3's delimiter reads it live now, so no corpus row reaches the
// method.
func TestBytesBakeableConst(t *testing.T) {
	if !(BytesBehavior{}).BakeableConst(NewBytes([]byte("x"))) {
		t.Fatal("Bytes is a bakeable const")
	}
}
