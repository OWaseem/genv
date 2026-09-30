package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #217: genv injected `. '/home/ks1686/.config/genv/env.sh'` into rc files —
// an absolute, host-specific path with no existence guard. On a fresh clone, or
// any host that has not applied since the fragments were rendered, every shell
// start printed "No such file or directory" until someone ran `genv apply`.
func TestInjectSourceLine_EmitsGuardedHomeRelativeSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	frag := filepath.Join(home, ".config", "genv", "env.sh")
	rc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rc, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine: %v", err)
	}
	got := readFileString(t, rc)

	// Guarded: a missing fragment must not produce an error on shell start.
	if !strings.Contains(got, `if [ -r `) {
		t.Errorf("injected line is not guarded, so a missing fragment errors on every shell start:\n%s", got)
	}
	// An `if`, not `[ -r x ] && . x`: the && form leaves a false exit status
	// when the fragment is missing, which breaks the non-interactive shells
	// this is meant to fix.
	if strings.Contains(got, "&&") {
		t.Errorf("guard uses && and will leave a false exit status when the fragment is missing:\n%s", got)
	}
	// $HOME-relative: the same committed rc template must be correct on every host.
	if !strings.Contains(got, `"$HOME/`) {
		t.Errorf("injected line is not $HOME-relative, so a shared rc template carries one host's path:\n%s", got)
	}
	if strings.Contains(got, home) {
		t.Errorf("injected line still embeds the absolute home path %q:\n%s", home, got)
	}
}

// A fragment outside home (a custom --state-dir) cannot be written as
// $HOME-relative, but it must still be guarded.
func TestInjectSourceLine_GuardsFragmentOutsideHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	frag := filepath.Join(t.TempDir(), "elsewhere", "env.sh")
	rc := filepath.Join(home, ".bashrc")

	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine: %v", err)
	}
	got := readFileString(t, rc)
	if !strings.Contains(got, `if [ -r `) {
		t.Errorf("fragment outside home must still be guarded:\n%s", got)
	}
	if !strings.Contains(got, frag) {
		t.Errorf("want the absolute path for a fragment outside home:\n%s", got)
	}
}

// #217, second half: genv recognised its own injected line only by exact path
// match, so a committed rc template carrying macOS's /Users/... never matched
// on a Linux host. genv appended a second block on first apply and the file
// stayed permanently dirty. An existing genv block must be *replaced* with the
// host-correct rendering, not duplicated.
func TestInjectSourceLine_ReplacesStaleBlockFromSharedTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".zshrc")
	// A shared template already carrying another host's rendering.
	stale := "\n# genv env\n. '/Users/someone-else/.config/genv/env.sh'\n"
	if err := os.WriteFile(rc, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	frag := filepath.Join(home, ".config", "genv", "env.sh")
	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine: %v", err)
	}
	got := readFileString(t, rc)

	if n := strings.Count(got, "# genv env"); n != 1 {
		t.Errorf("genv env marker appears %d times, want exactly 1:\n%s", n, got)
	}
	if strings.Contains(got, "/Users/someone-else") {
		t.Errorf("stale host path was left behind:\n%s", got)
	}
	if !strings.Contains(got, `"$HOME/`) {
		t.Errorf("want the host-correct $HOME-relative line:\n%s", got)
	}
}

// Repeated applies must stay idempotent.
func TestInjectSourceLine_IdempotentAcrossApplies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	frag := filepath.Join(home, ".config", "genv", "env.sh")
	rc := filepath.Join(home, ".bashrc")

	for i := range 3 {
		if err := InjectSourceLine(rc, frag); err != nil {
			t.Fatalf("InjectSourceLine (run %d): %v", i, err)
		}
	}
	got := readFileString(t, rc)
	if n := strings.Count(got, "# genv env"); n != 1 {
		t.Errorf("genv env marker appears %d times after 3 runs, want 1:\n%s", n, got)
	}
}

// Unrelated rc content must survive: the replacement is scoped to genv's own
// block, not the whole file.
func TestInjectSourceLine_PreservesSurroundingContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".bashrc")
	before := "export FOO=1\n# genv env\n. '/old/host/env.sh'\nexport BAR=2\n"
	if err := os.WriteFile(rc, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	frag := filepath.Join(home, ".config", "genv", "env.sh")
	if err := InjectSourceLine(rc, frag); err != nil {
		t.Fatalf("InjectSourceLine: %v", err)
	}
	got := readFileString(t, rc)
	if !strings.Contains(got, "export FOO=1") || !strings.Contains(got, "export BAR=2") {
		t.Errorf("surrounding rc content was lost:\n%s", got)
	}
	if strings.Contains(got, "/old/host/env.sh") {
		t.Errorf("stale source line was not replaced:\n%s", got)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
