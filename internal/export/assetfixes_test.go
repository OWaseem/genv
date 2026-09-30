package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func v8Spec(t *testing.T, bundle schema.TargetBundle) *schema.GenvFile {
	t.Helper()
	data, err := marshalSpec(t, bundle)
	if err != nil {
		t.Fatal(err)
	}
	f, _, parseErr := schema.ParseAndValidate(data)
	if parseErr != nil {
		t.Fatalf("test spec invalid: %v", parseErr)
	}
	return f
}

func marshalSpec(t *testing.T, bundle schema.TargetBundle) ([]byte, error) {
	doc := map[string]any{
		"schemaVersion": schema.Version8,
		"targets":       map[string]any{"macos": bundle},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// copyAsset joins baseDir with source before reading. Renaming the output does
// not stop "../.ssh/id_ed25519" from being read from outside the spec dir.
func TestCopyAsset_RefusesSourceEscapingSpecDir(t *testing.T) {
	specDir := t.TempDir()
	outDir := t.TempDir()
	secret := filepath.Join(filepath.Dir(specDir), "secret.txt")
	writeFile(t, secret, "private")

	rel, err := filepath.Rel(specDir, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyAsset(specDir, outDir, rel, "link", 0); err == nil {
		t.Fatal("copyAsset accepted a source escaping the spec directory")
	}
	// And nothing was written into the bundle.
	if _, err := os.Stat(filepath.Join(outDir, "files")); !os.IsNotExist(err) {
		t.Fatalf("bundle received the escaping asset")
	}
}

func TestCopyAsset_RefusesSymlinkedSource(t *testing.T) {
	specDir := t.TempDir()
	outDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "private")
	if err := os.Symlink(outside, filepath.Join(specDir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := copyAsset(specDir, outDir, "link.txt", "link", 0); err == nil {
		t.Fatal("copyAsset followed a symlinked source")
	}
	if _, err := os.Stat(filepath.Join(outDir, "files", "link.txt")); !os.IsNotExist(err) {
		t.Fatal("symlink target contents were copied into the bundle")
	}
}

func TestCopyPath_RefusesSymlinkInsideDirectory(t *testing.T) {
	src := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "private")
	writeFile(t, filepath.Join(src, "ok.txt"), "fine")
	if err := os.Symlink(outside, filepath.Join(src, "bad.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := copyPath(src, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("copyPath followed a symlink inside a directory tree")
	}
}

// "~/dotfiles/zshrc" is a real file outside the spec directory. It was joined
// under the spec dir, and the copy failed instead of being reported as a skip.
func TestIsExternalSourcePath(t *testing.T) {
	external := []string{
		"/etc/genv/config",
		`C:\Users\me\.gitconfig`,
		`\\server\share\file`,
		"~/dotfiles/zshrc",
		`~\dotfiles\zshrc`,
		"~",
		"$HOME/dotfiles/zshrc",
		"assets/$FILE",
	}
	for _, p := range external {
		if !isExternalSourcePath(p) {
			t.Errorf("isExternalSourcePath(%q) = false, want true", p)
		}
	}
	bundleable := []string{"assets/gitconfig", "config.tmpl", "a/b/c.txt", "no-dollar-here"}
	for _, p := range bundleable {
		if isExternalSourcePath(p) {
			t.Errorf("isExternalSourcePath(%q) = true, want false", p)
		}
	}
}

func TestExport_HomeRelativeSourceIsReportedNotFailed(t *testing.T) {
	specDir := t.TempDir()
	outDir := t.TempDir()
	f := v8Spec(t, schema.TargetBundle{
		Files: &schema.FilesConfig{Templates: []schema.FileTemplate{{
			Source: "~/dotfiles/zshrc",
			Target: "~/.zshrc",
		}}},
	})
	report, err := BuildWithOptions(f, "macos", outDir, Options{BaseDir: specDir})
	if err != nil {
		t.Fatalf("BuildWithOptions error = %v, want the source reported not a copy failure", err)
	}
	if !hasReportCode(report, "absolute-source") {
		t.Fatalf("report = %+v, want an absolute-source finding", report)
	}
}

func TestExport_BundlesHookScripts(t *testing.T) {
	specDir := t.TempDir()
	outDir := t.TempDir()
	writeFile(t, filepath.Join(specDir, "hooks", "pre.sh"), "#!/bin/sh\necho hi\n")
	f := v8Spec(t, schema.TargetBundle{
		Hooks: &schema.HooksConfig{
			PreApply: []schema.Hook{{Name: "pre", File: "hooks/pre.sh"}},
		},
	})
	if _, err := BuildWithOptions(f, "macos", outDir, Options{BaseDir: specDir}); err != nil {
		t.Fatalf("BuildWithOptions: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "files", "hooks", "pre.sh")); err != nil {
		t.Fatalf("hook script not bundled: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "genv.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "files/hooks/pre.sh") {
		t.Fatalf("exported spec does not point at the bundled hook:\n%s", data)
	}
}

func hasReportCode(report Report, code string) bool {
	for _, item := range report {
		if item.Code == code {
			return true
		}
	}
	return false
}
