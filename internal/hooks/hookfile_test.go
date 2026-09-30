package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// A pulled or exported spec keeps its hook scripts next to genv.json. Applying
// from another directory — or from the scheduler, whose cwd is unrelated —
// must still find them.
func TestResolveHookFile_RelativePathResolvesAgainstSourceRoot(t *testing.T) {
	specDir := t.TempDir()
	script := filepath.Join(specDir, "hooks", "pre.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	e := NewExecutor(nil, nil)
	e.SourceRoot = specDir

	got, err := e.resolveHookFile("hooks/pre.sh")
	if err != nil {
		t.Fatalf("resolveHookFile: %v", err)
	}
	if got != script {
		t.Fatalf("resolveHookFile = %q, want %q", got, script)
	}
}

func TestResolveHookFile_AbsoluteAndHomePathsUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	abs := filepath.Join(home, "scripts", "abs.sh")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A deliberately unrelated SourceRoot must not capture an absolute path.
	e := NewExecutor(nil, nil)
	e.SourceRoot = t.TempDir()

	if got, err := e.resolveHookFile(abs); err != nil || got != abs {
		t.Fatalf("absolute: got %q err %v, want %q", got, err, abs)
	}

	homeRel := filepath.Join(home, "scripts", "abs.sh")
	if got, err := e.resolveHookFile("~/scripts/abs.sh"); err != nil || got != homeRel {
		t.Fatalf("home-relative: got %q err %v, want %q", got, err, homeRel)
	}
}

func TestResolveHookFile_ReportsMissingScript(t *testing.T) {
	e := NewExecutor(nil, nil)
	e.SourceRoot = t.TempDir()
	if _, err := e.resolveHookFile("hooks/missing.sh"); err == nil {
		t.Fatal("expected an error for a missing hook script")
	}
}

func TestHookArgs_UsesResolvedPath(t *testing.T) {
	specDir := t.TempDir()
	script := filepath.Join(specDir, "pre.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(nil, nil)
	e.SourceRoot = specDir
	e.goos = "linux"

	args, err := e.hookArgs(schema.Hook{File: "pre.sh"})
	if err != nil {
		t.Fatalf("hookArgs: %v", err)
	}
	if len(args) == 0 || args[len(args)-1] != script {
		t.Fatalf("hookArgs = %v, want it to end with %q", args, script)
	}
}
