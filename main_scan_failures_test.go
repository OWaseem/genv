package main

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
)

// scanBrokenAdapter fails its inventory the way a manager with an unparseable
// listing does — the npm case in #215, where the global package list could not
// be decoded and the section silently came out empty.
type scanBrokenAdapter struct{ scanManagerNameAdapter }

func (a *scanBrokenAdapter) ListInstalled() ([]string, error) {
	return nil, errStubScanList
}

var errStubScanList = errStub("parse npm global package list: unexpected shape")

type errStub string

func (e errStub) Error() string { return string(e) }

func withScanAdapters(t *testing.T, adapters ...adapter.Adapter) {
	t.Helper()
	origAll := adapter.All
	adapter.All = adapters
	t.Cleanup(func() { adapter.All = origAll })
	origScanGOOS := scanGOOS
	scanGOOS = runtime.GOOS
	t.Cleanup(func() { scanGOOS = origScanGOOS })
}

// #215: a manager whose inventory could not be read was reported on stderr and
// skipped, and scan still exited 0 — so a partial inventory read as a complete
// one. A hard failure must now fail the command, while a per-manager timeout
// stays a warning so a stalled winget source sync does not break every scan.
func TestScanCmd_HardListingFailureExitsNonZero(t *testing.T) {
	withScanAdapters(t,
		&scanBrokenAdapter{scanManagerNameAdapter{name: "brokenmgr"}},
		&scanManagerNameAdapter{name: "healthymgr", installed: []string{"jq"}},
	)

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", dir + "/genv.json", "--json", "--dry-run"})
	})
	if code == exitOK {
		t.Fatalf("scan exit = %d, want non-zero: one source could not be read (output: %s)", code, out)
	}

	var env struct {
		OK     bool `json:"ok"`
		Errors []string
		Data   struct {
			Unreadable []string `json:"unreadableManagers"`
		}
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("scan --json is not valid JSON: %v\noutput: %q", err, out)
	}
	if env.OK {
		t.Error("envelope ok = true, want false when a source failed")
	}
	if len(env.Errors) == 0 {
		t.Error("envelope errors empty, want the unreadable manager named")
	}
	if len(env.Data.Unreadable) != 1 || env.Data.Unreadable[0] != "brokenmgr" {
		t.Errorf("unreadableManagers = %v, want [brokenmgr]", env.Data.Unreadable)
	}
}

// The healthy manager's findings must still be reported: a partial inventory
// is incomplete, not useless.
func TestScanCmd_HardListingFailureStillReportsHealthyManagers(t *testing.T) {
	withScanAdapters(t,
		&scanBrokenAdapter{scanManagerNameAdapter{name: "brokenmgr"}},
		&scanManagerNameAdapter{name: "healthymgr", installed: []string{"jq"}},
	)

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var code int
	out := captureStdout(t, func() {
		code = run([]string{"scan", "--file", dir + "/genv.json", "--json", "--dry-run"})
	})
	if code == exitOK {
		t.Fatalf("scan exit = %d, want non-zero", code)
	}
	var env struct {
		Data struct {
			Packages []string `json:"packages"`
		}
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("scan --json is not valid JSON: %v", err)
	}
	if len(env.Data.Packages) != 1 || env.Data.Packages[0] != "jq" {
		t.Errorf("packages = %v, want [jq] from the healthy manager", env.Data.Packages)
	}
}
