package resolver

import (
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// #213: a failed install left a lock entry with no version, and that entry was
// then believed forever. packageDrifted only compares a *recorded* version, so
// a version-less entry had nothing to compare, was never re-queued, and was
// written straight back into Unchanged on every subsequent apply. Once the
// manifest appeared, apply still reported "(up to date)" and the package was
// never installed.
func TestReconcileWith_VersionlessEntryAndManagerReportsAbsent_Reinstalls(t *testing.T) {
	desired := []schema.Package{{
		ID:       "peaproxy",
		Managers: map[string]string{"scoop": "peaproxy"},
	}}
	// The install failed, so the entry carries no version. scoop is
	// successfully inventoried and does not have the package.
	managed := []genvfile.LockedPackage{
		{ID: "peaproxy", Manager: "scoop", PkgName: "peaproxy"},
	}
	live := LiveSet{"scoop": {"jq": true}}

	got := ReconcileWith(desired, managed, map[string]bool{"scoop": true}, live)
	if len(got.ToInstall) != 1 {
		t.Fatalf("ToInstall = %d, want 1: a version-less entry whose manager reports it absent must be retried", len(got.ToInstall))
	}
	if got.ToInstall[0].Pkg.ID != "peaproxy" {
		t.Errorf("ToInstall[0].Pkg.ID = %q, want peaproxy", got.ToInstall[0].Pkg.ID)
	}
	if len(got.Unchanged) != 0 {
		t.Errorf("Unchanged = %v, want none: re-queued packages must not also be written back to the lock", got.Unchanged)
	}
}

// The absence of a version is only evidence of a problem when the manager was
// actually inventoried. A manager that could not be listed leaves its entries
// alone rather than re-queueing an install that may be perfectly satisfied.
func TestReconcileWith_VersionlessEntryAndManagerNotInventoried_StaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "git", Managers: map[string]string{"brew": "git"}}}
	managed := []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}}

	got := ReconcileWith(desired, managed, map[string]bool{"brew": true}, nil)
	if len(got.ToInstall) != 0 {
		t.Errorf("ToInstall = %d, want 0 with no live inventory to consult", len(got.ToInstall))
	}
	if len(got.Unchanged) != 1 {
		t.Errorf("Unchanged = %d, want 1", len(got.Unchanged))
	}
}

// A manager that was inventoried and simply has no such package is the same
// absence, even when it has other packages.
func TestReconcileWith_VersionlessEntryAndEmptyInventory_Reinstalls(t *testing.T) {
	desired := []schema.Package{{ID: "vortex", Managers: map[string]string{"winget": "NexusMods.Vortex"}}}
	managed := []genvfile.LockedPackage{
		{ID: "vortex", Manager: "winget", PkgName: "NexusMods.Vortex"},
	}
	live := LiveSet{"winget": {}}

	got := ReconcileWith(desired, managed, map[string]bool{"winget": true}, live)
	if len(got.ToInstall) != 1 {
		t.Fatalf("ToInstall = %d, want 1: an empty inventory means the package is absent", len(got.ToInstall))
	}
}

// A version-less entry whose manager *does* have the package is satisfied.
// genv just cannot name a version for it.
func TestReconcileWith_VersionlessEntryButPresentInInventory_StaysUnchanged(t *testing.T) {
	desired := []schema.Package{{ID: "git", Managers: map[string]string{"brew": "git"}}}
	managed := []genvfile.LockedPackage{{ID: "git", Manager: "brew", PkgName: "git"}}
	live := LiveSet{"brew": {"git": true}}

	got := ReconcileWith(desired, managed, map[string]bool{"brew": true}, live)
	if len(got.ToInstall) != 0 {
		t.Errorf("ToInstall = %d, want 0: the package is installed", len(got.ToInstall))
	}
	if len(got.Unchanged) != 1 {
		t.Errorf("Unchanged = %d, want 1", len(got.Unchanged))
	}
}

// A recorded version still wins: an entry with a version that no longer
// satisfies the spec is re-queued on version grounds alone, and the new
// absence rule must not change that path.
func TestReconcile_RecordedVersionStillDriftsWithoutLive(t *testing.T) {
	desired := []schema.Package{{ID: "git", Version: ">=3.0.0"}}
	managed := []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git", InstalledVersion: "2.43.0"},
	}
	got := Reconcile(desired, managed, map[string]bool{"brew": true})
	if len(got.ToInstall) != 1 {
		t.Fatalf("ToInstall = %d, want 1 on version drift", len(got.ToInstall))
	}
}
