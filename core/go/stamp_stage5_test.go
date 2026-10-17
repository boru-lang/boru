package core

// Stage-5 coverage: stamp_report.go. The stamp-attribution log is pure
// in-memory state (arming, recording, snapshotting, resetting), so every
// arm is drivable directly: nil receivers, unarmed registries, and an
// armed registry's full record/report/reset cycle.

import "testing"

func stage5Registry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
