package commands

import (
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// #213: `genv status` decided a package was installed purely from the lock, so
// a version-less entry left by a failed install reported "ok" for a package the
// manager had never installed. Reporting it as ok is self-perpetuating: the
// entry is the only reason the check passes.
func TestStatus_VersionlessLockEntryIsUnknownNotOK(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{
		ID:       "peaproxy",
		Managers: map[string]string{"scoop": "peaproxy"},
	}}}
	lf := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "peaproxy", Manager: "scoop", PkgName: "peaproxy"},
	}}
	live := map[string]map[string]bool{"scoop": {"jq": true}}

	got := StatusWithLive(f, lf, live)
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if got[0].Kind == StatusOK {
		t.Fatalf("Kind = %q, want not ok: a version-less entry proves nothing", got[0].Kind)
	}
	if got[0].Kind != StatusUnknown {
		t.Errorf("Kind = %q, want %q", got[0].Kind, StatusUnknown)
	}
}

// The version column keeps its own "*" versus "?" meaning: the StatusUnknown
// kind is what reports that genv cannot confirm the install, so the column
// does not have to collapse to a single placeholder.
func TestStatus_UnknownEntryKeepsVersionColumnMeaning(t *testing.T) {
	if got := (StatusEntry{ID: "a", Kind: StatusUnknown}).DisplayVersion(); got != "*" {
		t.Errorf("unconstrained DisplayVersion = %q, want *", got)
	}
	if got := (StatusEntry{ID: "b", Kind: StatusUnknown, SpecVersion: "1.2.*"}).DisplayVersion(); got != "?" {
		t.Errorf("constrained DisplayVersion = %q, want ?", got)
	}
	if got := (StatusEntry{ID: "x", Kind: StatusOK, InstalledVersion: "1.2.3"}).DisplayVersion(); got != "1.2.3" {
		t.Errorf("DisplayVersion = %q, want 1.2.3", got)
	}
}

// A recorded, satisfying version is still ok — the new kind must not fire on
// healthy entries.
func TestStatus_RecordedVersionStillOK(t *testing.T) {
	f := &schema.GenvFile{Packages: []schema.Package{{ID: "git"}}}
	lf := &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
	}}
	got := StatusWithLive(f, lf, map[string]map[string]bool{"brew": {"git": true}})
	if len(got) != 1 || got[0].Kind != StatusOK {
		t.Fatalf("entry = %+v, want kind ok", got)
	}
}
