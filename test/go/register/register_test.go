// Integrity gates for the performance register (design/legacy/FULL-COMPILATION.0.ignore
// section 14).
//
// The register is a permanent, append-only record: rows are never edited
// or deleted, and a measurement discovered to be wrong is superseded by a
// new row rather than corrected in place. Nothing here asserts anything
// about the VALUES — the register records, it never gates, because
// execution time is too noisy to fail CI on and the deterministic alloc
// ceilings remain the only performance gates. What these tests protect is
// the thing that makes the record readable years later: that every line
// parses, that the schema is stable, and that every measurement names a
// host whose full physical spec is on file.
package register

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type hostRow struct {
	Host      string `json:"host"`
	Label     string `json:"label"`
	CPUModel  string `json:"cpu_model"`
	CPUCores  int    `json:"cpu_cores"`
	MemKB     int64  `json:"mem_kb"`
	Arch      string `json:"arch"`
	OSName    string `json:"os_name"`
	OSMajor   string `json:"os_major"`
	Kernel    string `json:"kernel"`
	Virt      string `json:"virt"`
	FirstSeen string `json:"first_seen"`
	IDFields  string `json:"id_fields"`
}

type measRow struct {
	TS        string  `json:"ts"`
	Commit    string  `json:"commit"`
	Host      string  `json:"host"`
	Surface   string  `json:"surface"`
	Workload  string  `json:"workload"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	N         int     `json:"n"`
	Benchtime string  `json:"benchtime"`
	Go        string  `json:"go"`
	OSVersion string  `json:"os_version"`
}

func registerPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "bench", "register", name)
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// Every host row parses, carries the spec fields the id was derived from,
// and appears exactly once. A host id is a hash over a tuple of physical
// facts, so a duplicate id with different specs means the tuple stopped
// identifying a machine — the failure the id_fields column exists to make
// detectable.
func TestHostsWellFormed(t *testing.T) {
	seen := map[string]hostRow{}
	for i, l := range lines(t, registerPath(t, "hosts.jsonl")) {
		var h hostRow
		if err := json.Unmarshal([]byte(l), &h); err != nil {
			t.Fatalf("hosts.jsonl:%d does not parse: %v", i+1, err)
		}
		switch {
		case !strings.HasPrefix(h.Host, "h:"):
			t.Errorf("hosts.jsonl:%d host id %q lacks the h: prefix", i+1, h.Host)
		case h.CPUModel == "" || h.Arch == "" || h.OSName == "":
			t.Errorf("hosts.jsonl:%d host %s is missing spec fields the id hashed", i+1, h.Host)
		case h.IDFields == "":
			t.Errorf("hosts.jsonl:%d host %s does not record which tuple its id hashed", i+1, h.Host)
		case h.FirstSeen == "":
			t.Errorf("hosts.jsonl:%d host %s has no first_seen stamp", i+1, h.Host)
		}
		if prev, dup := seen[h.Host]; dup {
			t.Errorf("hosts.jsonl:%d host id %s appears twice (%q and %q) — the id tuple no longer identifies a machine",
				i+1, h.Host, prev.Label, h.Label)
		}
		seen[h.Host] = h
	}
	if len(seen) == 0 {
		t.Error("hosts.jsonl is empty — the register cannot attribute any measurement")
	}
}

// Every measurement parses, names a surface the design defines, and
// resolves to a host on file. An unresolvable host is the one corruption
// that makes a row permanently meaningless: absolute values compare only
// within a host id, so a row whose host is unknown can never be read.
func TestMeasurementsWellFormed(t *testing.T) {
	hosts := map[string]bool{}
	for _, l := range lines(t, registerPath(t, "hosts.jsonl")) {
		var h hostRow
		if err := json.Unmarshal([]byte(l), &h); err == nil {
			hosts[h.Host] = true
		}
	}

	surfaces := map[string]bool{
		"check": true, "compile": true, "interp": true,
		"exec": true, "parse": true, "e2e": true,
	}

	rows := lines(t, registerPath(t, "measurements.jsonl"))
	if len(rows) == 0 {
		t.Fatal("measurements.jsonl is empty")
	}
	bySurface := map[string]int{}
	for i, l := range rows {
		var m measRow
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("measurements.jsonl:%d does not parse: %v", i+1, err)
		}
		if !hosts[m.Host] {
			t.Errorf("measurements.jsonl:%d names host %s, which is not in hosts.jsonl", i+1, m.Host)
		}
		if !surfaces[m.Surface] {
			t.Errorf("measurements.jsonl:%d has surface %q, not one the design defines", i+1, m.Surface)
		}
		if m.Workload == "" || m.Metric == "" || m.Unit == "" {
			t.Errorf("measurements.jsonl:%d is missing workload/metric/unit", i+1)
		}
		if m.Commit == "" || m.TS == "" {
			t.Errorf("measurements.jsonl:%d has no commit or timestamp — it cannot be placed in the series", i+1)
		}
		if m.OSVersion == "" || m.Go == "" {
			t.Errorf("measurements.jsonl:%d omits os_version/go — a host drifts under a stable id, so both ride on every row", i+1)
		}
		bySurface[m.Surface]++
	}
	t.Logf("performance register: %d rows across %d hosts, by surface %v", len(rows), len(hosts), bySurface)
}
