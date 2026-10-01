package pull

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// Only the final path was Lstat'd, so an intermediate symlink escaped:
// "a" -> outside the cache made "a/id_ed25519" pass and copy from outside.
func TestCopyBundleAssets_RejectsIntermediateSymlink(t *testing.T) {
	cacheDir := t.TempDir()
	destDir := t.TempDir()
	outside := t.TempDir()
	keyPath := filepath.Join(outside, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cacheDir, "keys")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"macos": {Files: &schema.FilesConfig{Links: []schema.FileLink{{
				Source: "keys/id_ed25519",
				Target: "~/.ssh/id_ed25519",
			}}}},
		},
	}
	if _, err := CopyBundleAssets(cacheDir, destDir, f); err == nil {
		t.Fatal("CopyBundleAssets followed an intermediate symlink")
	}
	if _, err := os.Stat(filepath.Join(destDir, "keys", "id_ed25519")); !os.IsNotExist(err) {
		t.Fatal("key outside the cache was copied into the destination")
	}
}

func TestCopyBundleAssets_AllowsNestedDirectories(t *testing.T) {
	cacheDir := t.TempDir()
	destDir := t.TempDir()
	nested := filepath.Join(cacheDir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Targets: map[string]*schema.TargetBundle{
			"macos": {Files: &schema.FilesConfig{Links: []schema.FileLink{{
				Source: "a/b/file.txt",
				Target: "~/.config/file.txt",
			}}}},
		},
	}
	copied, err := CopyBundleAssets(cacheDir, destDir, f)
	if err != nil {
		t.Fatalf("CopyBundleAssets: %v", err)
	}
	if len(copied) != 1 || copied[0] != "a/b/file.txt" {
		t.Fatalf("copied = %v", copied)
	}
	if _, err := os.Stat(filepath.Join(destDir, "a", "b", "file.txt")); err != nil {
		t.Fatalf("nested asset not copied: %v", err)
	}
}

func TestBundleAssetSources_IncludesHookScripts(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Defaults: &schema.TargetBundle{
			Hooks: &schema.HooksConfig{
				PreApply:    []schema.Hook{{Name: "a", File: "hooks/pre.sh"}},
				PostApply:   []schema.Hook{{Name: "b", Command: "echo inline"}},
				PreUpgrade:  []schema.Hook{{Name: "c", File: "hooks/pre-upgrade.sh"}},
				PostUpgrade: []schema.Hook{{Name: "d", File: "hooks/post-upgrade.sh"}},
			},
		},
		Targets: map[string]*schema.TargetBundle{
			"macos": {Hooks: &schema.HooksConfig{
				PreApply: []schema.Hook{{Name: "e", File: "hooks/macos.sh"}},
			}},
		},
	}
	got := BundleAssetSources(f)
	want := map[string]bool{
		"hooks/pre.sh":          true,
		"hooks/pre-upgrade.sh":  true,
		"hooks/post-upgrade.sh": true,
		"hooks/macos.sh":        true,
	}
	for _, w := range got {
		if !want[w] {
			t.Errorf("unexpected asset source %q", w)
		}
		delete(want, w)
	}
	for w := range want {
		t.Errorf("missing hook script %q in %v", w, got)
	}
}

func TestBundleAssetSources_SkipsHookPathsThatEscape(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version8,
		Defaults: &schema.TargetBundle{
			Hooks: &schema.HooksConfig{
				PreApply: []schema.Hook{
					{Name: "ok", File: "hooks/ok.sh"},
					{Name: "escape", File: "../outside.sh"},
					{Name: "abs", File: "/etc/genv/hook.sh"},
					{Name: "home", File: "~/hook.sh"},
				},
			},
		},
		Targets: map[string]*schema.TargetBundle{},
	}
	got := BundleAssetSources(f)
	if len(got) != 1 || got[0] != "hooks/ok.sh" {
		t.Fatalf("BundleAssetSources = %v, want only hooks/ok.sh", got)
	}
}
