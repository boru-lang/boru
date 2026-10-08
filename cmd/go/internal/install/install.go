// Package install implements `boru install <name>-x.y.z [-r <url>]`
// — download a published module zip from the registry, extract it
// into .boru/<name>/, update boru.jsonic deps, and re-prep.
package install

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/prep"
)

// moduleIDPattern matches <name>-<major>.<minor>.<patch>.
var moduleIDPattern = regexp.MustCompile(`^(.+)-(\d+\.\d+\.\d+)$`)

// Download / extraction bounds. A module is small; these caps keep a
// malicious or compromised registry from exhausting memory via an
// unbounded body or a zip bomb.
const (
	maxDownloadBytes int64 = 64 << 20 // whole archive
	maxEntryBytes    int64 = 64 << 20 // single extracted file
)

type cmd struct{}

// New returns the install subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "install" }
func (*cmd) Synopsis() string { return "download and install a module from a registry" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

// Run handles `boru install <name>-x.y.z [-r <url>]`.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)

	registryURL := fs.String("r", "http://localhost:8080", "registry server URL")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "error: usage: boru install <name>-x.y.z [-r <url>]\n")
		return 1
	}

	moduleID := fs.Arg(0)
	matches := moduleIDPattern.FindStringSubmatch(moduleID)
	if matches == nil {
		fmt.Fprintf(stderr, "error: invalid module identifier %q (expected <name>-x.y.z)\n", moduleID)
		return 1
	}
	name := matches[1]
	version := matches[2]

	boruJSON := filepath.Join(".boru", "boru.json")
	if _, err := os.Stat(boruJSON); err != nil {
		fmt.Fprintf(stderr, "error: not a valid module folder (missing .boru/boru.json)\n")
		return 1
	}

	url := strings.TrimRight(*registryURL, "/") + "/module/" + moduleID
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Fprintf(stderr, "error: module %q not found on registry (status %d)\n", moduleID, resp.StatusCode)
		return 1
	}

	// Bound the download so a malicious or compromised registry cannot
	// stream an unbounded body into memory. The cap mirrors the server's
	// own upload limit with headroom.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if int64(len(body)) > maxDownloadBytes {
		fmt.Fprintf(stderr, "error: module download exceeds %d bytes\n", maxDownloadBytes)
		return 1
	}

	destDir := filepath.Join(".boru", name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid zip: %s\n", err)
		return 1
	}

	for _, f := range zr.File {
		// Reject anything that is not a clean relative path contained in
		// destDir (zip-slip). filepath.IsLocal catches "..", absolute
		// paths, and (on Windows) drive/volume-relative names — a more
		// robust guard than a "contains .." substring test.
		if !filepath.IsLocal(f.Name) {
			fmt.Fprintf(stderr, "error: declining unsafe path in archive: %q\n", f.Name)
			return 1
		}
		destPath := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		rc, err := f.Open()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		// Bound each entry so a zip bomb cannot exhaust memory/disk.
		data, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes+1))
		rc.Close()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if int64(len(data)) > maxEntryBytes {
			fmt.Fprintf(stderr, "error: archive entry %q exceeds %d bytes\n", f.Name, maxEntryBytes)
			return 1
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	if err := updateDeps(name, version); err != nil {
		fmt.Fprintf(stderr, "error: updating boru.jsonic: %s\n", err)
		return 1
	}

	if _, err := prep.Do("."); err != nil {
		fmt.Fprintf(stderr, "error: prep: %s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "installed %s@%s -> .boru/%s/\n", name, version, name)
	return 0
}

// updateDeps reads boru.jsonic, adds/updates deps.<name>=<version>, writes back.
func updateDeps(name, version string) error {
	src := "boru.jsonic"
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	content := string(data)
	depEntry := fmt.Sprintf("%s: %s", name, version)

	if strings.Contains(content, "deps:") {
		lines := strings.Split(content, "\n")
		found := false
		inDeps := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "deps:") {
				inDeps = true
				if strings.Contains(trimmed, "{") {
					namePattern := regexp.MustCompile(regexp.QuoteMeta(name) + `:\s*[^\s}]+`)
					if namePattern.MatchString(trimmed) {
						lines[i] = namePattern.ReplaceAllString(line, depEntry)
						found = true
					} else {
						lines[i] = strings.Replace(line, "}", " "+depEntry+"}", 1)
						found = true
					}
					inDeps = false
				}
				continue
			}
			if inDeps && trimmed == "}" {
				inDeps = false
				continue
			}
			if inDeps {
				namePrefix := name + ":"
				if strings.HasPrefix(trimmed, namePrefix) {
					lines[i] = strings.Replace(line, trimmed, depEntry, 1)
					found = true
				}
			}
		}
		if found {
			content = strings.Join(lines, "\n")
		} else {
			content = strings.Replace(content, "deps: {", "deps: {"+depEntry+" ", 1)
		}
	} else {
		content = strings.TrimRight(content, "\n") + "\ndeps: {" + depEntry + "}\n"
	}

	return os.WriteFile(src, []byte(content), 0644)
}
