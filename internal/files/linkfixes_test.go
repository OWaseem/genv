package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestExpandPath_TildeIsNotAPrefixForOtherUsers(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	cases := map[string]string{
		"~":         home,
		"~/foo":     filepath.Join(home, "foo"),
		"~/a/b.txt": filepath.Join(home, "a/b.txt"),
	}
	for in, want := range cases {
		got, err := expandPath(in)
		if err != nil {
			t.Fatalf("expandPath(%q) error = %v", in, err)
		}
		if got != want {
			t.Errorf("expandPath(%q) = %q, want %q", in, got, want)
		}
	}

	// ~name is another user's home. It must not become $HOME + "name".
	for _, in := range []string{"~root/x", "~someone"} {
		got, err := expandPath(in)
		if err != nil {
			t.Fatalf("expandPath(%q) error = %v", in, err)
		}
		if got == filepath.Join(home, strings.TrimPrefix(in, "~")) {
			t.Errorf("expandPath(%q) = %q, want the path left untouched", in, got)
		}
		if strings.HasPrefix(got, home+"name") || strings.HasPrefix(got, home+"root") ||
			strings.HasPrefix(got, home+"someone") {
			t.Errorf("expandPath(%q) = %q, must not expand under home %s", in, got, home)
		}
	}
}

func TestResolveSource_TildeUserNotJoinedUnderRoot(t *testing.T) {
	root := t.TempDir()
	// "~other/x" is not home-relative, so it must not silently become
	// <root>/~other/x either — it stays relative and lands under root only if
	// it does not escape. The point is that it is not home-expanded.
	got, err := resolveSource(root, "~other/x")
	if err != nil {
		t.Fatalf("resolveSource error = %v", err)
	}
	if strings.Contains(got, filepath.Join(root, "~other")) == false && got != filepath.Join(root, "~other/x") {
		t.Fatalf("resolveSource = %q, want it left relative under root", got)
	}
	if home, _ := os.UserHomeDir(); strings.HasPrefix(got, home) && got != filepath.Join(home, "x") {
		t.Fatalf("resolveSource = %q, unexpectedly under home %s", got, home)
	}
}

func TestApplyLink_RelativeSymlinkResolvingToSourceIsSkipped(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	targetDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	target := filepath.Join(targetDir, "simple.txt")
	// Relative link text naming the same file as the absolute source.
	if err := os.Symlink("../repo/simple.txt", target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &schema.FilesConfig{Links: []schema.FileLink{{
		Source: source,
		Target: target,
		Mode:   "link",
	}}}

	res, err := Apply(context.Background(), cfg, "any", ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != target {
		t.Fatalf("Skipped = %v, want [%s]", res.Skipped, target)
	}
	if len(res.Updated) != 0 {
		t.Errorf("Updated = %v, want none", res.Updated)
	}
	// The link text must not have been rewritten.
	cur, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if cur != "../repo/simple.txt" {
		t.Errorf("link was rewritten to %q", cur)
	}
}

func TestStatusLink_RelativeSymlinkIsOK(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	targetDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	target := filepath.Join(targetDir, "simple.txt")
	if err := os.Symlink("../repo/simple.txt", target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	cfg := &schema.FilesConfig{Links: []schema.FileLink{{
		Source: source,
		Target: target,
		Mode:   "link",
	}}}
	res, err := Status(cfg, "any")
	if err != nil {
		t.Fatalf("Status error = %v", err)
	}
	if !res.OK {
		t.Fatalf("Status not OK: %+v", res.Entries)
	}
	if res.Entries[0].Kind != "ok" {
		t.Errorf("Kind = %q, want ok", res.Entries[0].Kind)
	}
}

func TestAdopt_AlreadyLinkedRelativelyIsANoop(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	source := filepath.Join(home, "repo", "simple.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	targetDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	target := filepath.Join(targetDir, "simple.txt")
	if err := os.Symlink("../repo/simple.txt", target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	res, err := Adopt(source, target, AdoptOptions{})
	if err != nil {
		t.Fatalf("Adopt error = %v, want nil for an already-correct link", err)
	}
	if len(res.Steps) != 0 {
		t.Errorf("Steps = %v, want none", res.Steps)
	}
}

func TestCreateSymlinkAt_FailureLeavesLiveFileInPlace(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	// A directory whose parent is read-only makes the staging symlink fail
	// while the live file is still in place.
	dir := filepath.Join(home, "ro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(dir, "live.txt")
	if err := os.WriteFile(target, []byte("live content"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	source := filepath.Join(home, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("running as root: read-only directory is not enforced")
	}

	if _, err := createSymlinkAt(source, target, true); err == nil {
		t.Fatal("createSymlinkAt error = nil, want failure in a read-only directory")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("live file is gone: %v", err)
	}
	if string(got) != "live content" {
		t.Fatalf("live file = %q, want it untouched", got)
	}
}

func TestRenderTemplate_ForcedOverwriteKeepsTargetOnWriteFailure(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "src.tmpl")
	if err := os.WriteFile(src, []byte("rendered"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	dir := filepath.Join(home, "ro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dst := filepath.Join(dir, "target")
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("running as root: read-only directory is not enforced")
	}

	if err := RenderTemplate(src, dst, "", RenderOptions{Force: true}); err == nil {
		t.Fatal("RenderTemplate error = nil, want failure in a read-only directory")
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("target is gone: %v", err)
	}
	if string(got) != "stale" {
		t.Fatalf("target = %q, want it untouched", got)
	}
}

func TestRenderTemplate_NoLeftoverTmpFiles(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	src := filepath.Join(home, "source-template")
	if err := os.WriteFile(src, []byte("rendered"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	dst := filepath.Join(home, "target")
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := RenderTemplate(src, dst, "", RenderOptions{Force: true}); err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "rendered" {
		t.Errorf("target = %q err=%v, want rendered", got, err)
	}
}
