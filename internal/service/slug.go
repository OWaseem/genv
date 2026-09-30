package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// serviceUnitSlug converts a service name into a token safe for a systemd unit
// filename, a launchd plist filename, and a schtasks task name.
//
// Service names are spec map keys and may contain a path separator (e.g.
// "apps/web"). These three functions previously used filepath.Base, so "a/x"
// and "b/x" both became "genv-x" and removing one service deleted the other.
//
// Sanitizing is lossy, so a name that needed changing gets a short digest of
// the original appended. Ordinary names keep their readable form, and names
// that differ only in replaced characters stay distinct.
func serviceUnitSlug(name string) string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(name, "\\", "/"), "/", " ")

	var b strings.Builder
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "default"
	}
	if slug != name {
		sum := sha256.Sum256([]byte(name))
		slug += "-" + hex.EncodeToString(sum[:])[:8]
	}
	return slug
}
