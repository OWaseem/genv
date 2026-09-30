package genvfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func newSpec() *schema.GenvFile {
	f := New()
	f.Defaults.Env = map[string]*schema.EnvVar{
		"TOKEN": {Value: "super-secret-value", Sensitive: true},
	}
	return f
}

func TestWrite_CreatesSpecPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "genv.json")
	if err := Write(path, newSpec()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("spec mode = %v, want no group/other access", perm)
	}
}

func TestWrite_DoesNotLoosenExistingPrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "genv.json")
	if err := Write(path, newSpec()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := Write(path, newSpec()); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("spec mode = %v, want the existing 0600 preserved", perm)
	}
}

func TestWrite_PreservesIntentionallyLooseMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "genv.json")
	if err := Write(path, newSpec()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := Write(path, newSpec()); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Fatalf("spec mode = %v, want the pre-existing 0644 left alone", perm)
	}
}

func TestWrite_LeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.json")
	for i := 0; i < 3; i++ {
		if err := Write(path, newSpec()); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "genv.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory contains %v, want only genv.json", names)
	}
}
