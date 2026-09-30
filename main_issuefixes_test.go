package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// filepath.Base let EDITOR=/tmp/evil/code pass the allowlist and then execute
// /tmp/evil/code, because the base name matched. With the directory off PATH
// the editor must be refused.
func TestBuildEditorCmd_QualifiedPathIsNotAllowlistedByBaseName(t *testing.T) {
	evilDir := t.TempDir()
	evil := filepath.Join(evilDir, "code")
	if err := os.WriteFile(evil, []byte("#!/bin/sh\necho pwned\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// PATH deliberately does NOT contain evilDir: that is the case genv can
	// defend. (An attacker who controls PATH is not this check's problem.)
	t.Setenv("PATH", t.TempDir())

	cmd, err := buildEditorCmd(evil, "/tmp/spec.json")
	if err == nil {
		t.Fatalf("buildEditorCmd(%q) = %v, want refusal", evil, cmd.Args)
	}
	if cmd != nil {
		t.Fatalf("buildEditorCmd(%q) returned a command (%v), want refusal", evil, cmd.Args)
	}
}

// A qualified editor name still works, but it resolves to the allowlisted
// binary on PATH rather than to the path the user typed.
func TestBuildEditorCmd_QualifiedPathResolvesToPATHBinary(t *testing.T) {
	pathDir := t.TempDir()
	legit := filepath.Join(pathDir, "code")
	if err := os.WriteFile(legit, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir)

	typed := filepath.Join(t.TempDir(), "code")
	if err := os.WriteFile(typed, []byte("#!/bin/sh\necho pwned\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd, err := buildEditorCmd(typed, "/tmp/spec.json")
	if err != nil {
		t.Fatalf("buildEditorCmd(%q) error = %v", typed, err)
	}
	if cmd.Path != legit {
		t.Fatalf("cmd.Path = %q, want the PATH-resolved %q", cmd.Path, legit)
	}
}

func TestBuildEditorCmd_BareNameStillAllowed(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "code")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	cmd, err := buildEditorCmd("code", "/tmp/spec.json")
	if err != nil {
		t.Fatalf("buildEditorCmd(code) error = %v", err)
	}
	if got := cmd.Args[len(cmd.Args)-1]; got != "/tmp/spec.json" {
		t.Fatalf("cmd args = %v", cmd.Args)
	}
}

func TestBuildEditorCmd_UnknownEditorRejected(t *testing.T) {
	if _, err := buildEditorCmd("rm", "/tmp/spec.json"); err == nil {
		t.Fatal("expected rm to be rejected")
	}
	if _, err := buildEditorCmd("vim --noplugin", "/tmp/spec.json"); err == nil {
		t.Fatal("expected an unsafe flag to be rejected")
	}
}

// A remote repo.url must not become the file source root: filepath.Clean turns
// "https://github.com/org/repo" into the relative path "https:/github.com/org/repo".
func TestSourceRootForSpec_RemoteRepoURLUsesSpecDir(t *testing.T) {
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "genv.json")

	for _, url := range []string{
		"https://github.com/org/repo",
		"ssh://git@github.com/org/repo",
		"git@github.com:org/repo.git",
	} {
		f := &schema.GenvFile{Repo: &schema.Repo{URL: url}}
		got := sourceRootForSpec(specPath, f)
		if got != specDir {
			t.Errorf("sourceRootForSpec with %q = %q, want %q", url, got, specDir)
		}
	}
}

func TestSourceRootForSpec_LocalRepoURLIsTheRoot(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "genv.json")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, tc := range []struct {
		url  string
		want string
	}{
		{url: "~/dotfiles", want: filepath.Join(home, "dotfiles")},
		{url: filepath.Join(home, "dotfiles"), want: filepath.Join(home, "dotfiles")},
		{url: "file:///srv/dotfiles", want: "/srv/dotfiles"},
		// file://host/path names another machine, not a local file.
		{url: "file://host/srv/dotfiles", want: filepath.Join(specPath, "..")},
	} {
		f := &schema.GenvFile{Repo: &schema.Repo{URL: tc.url}}
		got := sourceRootForSpec(specPath, f)
		if got != tc.want {
			t.Errorf("sourceRootForSpec(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestSourceRootForSpec_NoRepoUsesSpecDir(t *testing.T) {
	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "genv.json")
	if got := sourceRootForSpec(specPath, nil); got != specDir {
		t.Errorf("sourceRootForSpec(nil) = %q, want %q", got, specDir)
	}
	if got := sourceRootForSpec(specPath, &schema.GenvFile{}); got != specDir {
		t.Errorf("sourceRootForSpec(empty) = %q, want %q", got, specDir)
	}
}

// "confirm" built a fresh bufio.Reader per prompt, so a piped "y\ny\n" was
// consumed by the first prompt and the second read saw EOF.
func TestConfirm_SharedReaderConsumesPipedAnswers(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("y\ny\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	old := confirmReader
	confirmReader = bufio.NewReader(r)
	t.Cleanup(func() { confirmReader = old })

	if !confirm("first? ") {
		t.Fatal("first confirm = false, want true")
	}
	if !confirm("second? ") {
		t.Fatal("second confirm = false, want true (a fresh reader would have hit EOF)")
	}
}

func TestApplyJSON_WetRunRequiresYes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	// An env var with no lock entry is pending work, so the gate must fire.
	spec := `{"schemaVersion":"8","packages":[],` +
		`"defaults":{"env":{"GENV_TEST_VAR":{"value":"1"}}},` +
		`"targets":{"macos":{}}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := captureFD(t, &os.Stdout, func() {
		if got := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--host", "ci", "--json"}); got == exitOK {
			t.Errorf("apply --json without --yes returned exitOK")
		}
	})
	if !strings.Contains(stdout, "wet-run requires --yes") {
		t.Fatalf("output = %s, want the wet-run gate message", stdout)
	}
	// The gate is plan-only: nothing was written.
	if _, err := os.Stat(filepath.Join(dir, "env.sh")); !os.IsNotExist(err) {
		t.Fatal("apply --json without --yes still mutated the filesystem")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("apply --json without --yes still wrote the lock")
	}
}

func TestApplyJSON_WetRunWithYesMutates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	spec := `{"schemaVersion":"8","packages":[],` +
		`"defaults":{"env":{"GENV_TEST_VAR":{"value":"1"}}},` +
		`"targets":{"macos":{}}}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := captureFD(t, &os.Stdout, func() {
		if got := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--host", "ci", "--json", "--yes"}); got != exitOK {
			t.Errorf("apply --json --yes code = %d, want %d", got, exitOK)
		}
	})
	if strings.Contains(stdout, "wet-run requires --yes") {
		t.Fatalf("--yes should not trip the gate: %s", stdout)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("apply --json --yes did not write the lock: %v", err)
	}
}

// A dry run must not demand --yes, and an already-applied spec must not
// either (matching `genv upgrade --json`).
func TestApplyJSON_DryRunNeedsNoYes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	specPath := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	if err := os.WriteFile(specPath, []byte(`{"schemaVersion":"8","packages":[],"defaults":{},"targets":{"macos":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := captureFD(t, &os.Stdout, func() {
		if got := run([]string{"apply", "--file", specPath, "--lock-file", lockPath, "--host", "ci", "--json", "--dry-run"}); got != exitOK {
			t.Errorf("apply --json --dry-run code = %d, want %d", got, exitOK)
		}
	})
	if strings.Contains(stdout, "wet-run requires --yes") {
		t.Fatalf("dry run should not demand --yes: %s", stdout)
	}
}
