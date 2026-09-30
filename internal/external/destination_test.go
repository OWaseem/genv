package external

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ks1686/genv/internal/testutil"
)

func TestExpandDestinationBareTilde(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	got, err := expandDestination("~")
	if err != nil {
		t.Fatalf("expandDestination(~) error = %v, want nil", err)
	}
	if got != filepath.Clean(home) {
		t.Fatalf("expandDestination(~) = %q, want %q", got, home)
	}
}

func TestExpandDestinationTildeUserIsNotHome(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	// "~root/x" is another user's home; it is neither home-relative nor
	// absolute, so it is refused rather than silently joined under $HOME.
	if got, err := expandDestination("~root/x"); err == nil {
		t.Fatalf("expandDestination(~root/x) = %q, want an error", got)
	}
}

func TestLocalKeyRelativeResolvesAgainstSourceRoot(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	specDir := t.TempDir()
	keyRel := filepath.Join("files", "external-key-0.pub")
	keyAbs := filepath.Join(specDir, keyRel)
	if err := os.MkdirAll(filepath.Dir(keyAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyAbs, []byte("bundled-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	eng := Engine{SourceRoot: specDir}
	got, err := eng.localKey("", filepath.ToSlash(keyRel))
	if err != nil {
		t.Fatalf("localKey error = %v, want the bundled key to resolve", err)
	}
	if string(got) != "bundled-key" {
		t.Fatalf("localKey = %q, want bundled-key", got)
	}
}

func TestLocalKeyAbsoluteStillWorks(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	keyAbs := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyAbs, []byte("abs-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := (Engine{SourceRoot: "/nonexistent"}).localKey("", keyAbs)
	if err != nil {
		t.Fatalf("localKey error = %v", err)
	}
	if string(got) != "abs-key" {
		t.Fatalf("localKey = %q, want abs-key", got)
	}
}

func TestLocalKeyInlineWins(t *testing.T) {
	got, err := (Engine{}).localKey("inline-key", "relative/key")
	if err != nil || string(got) != "inline-key" {
		t.Fatalf("localKey = %q, %v", got, err)
	}
}
