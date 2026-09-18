package vault

// The suite derives a scrypt key a few thousand times (every store open,
// every export, every keyslot). At the production work factor that is
// 293 s of pure KDF; at 2^10 it is under five seconds and every test
// exercises exactly the same code paths — the parameter is a cost, not a
// branch. A test that wants the production cost sets scryptN back for
// its own duration (none does today).
func init() { scryptN = 1 << 10 }
