package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSource expands source and, when it is relative, joins it under
// sourceRoot. It refuses paths that escape sourceRoot.
func ResolveSource(sourceRoot, source string) (string, error) {
	return resolveSource(sourceRoot, source)
}

// ExpandPath expands a leading ~ and $VAR references.
func ExpandPath(s string) (string, error) {
	return expandPath(s)
}

func resolveSource(sourceRoot, source string) (string, error) {
	if source == "" {
		return "", errors.New("source must not be empty")
	}
	expanded, err := expandPath(source)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return expanded, nil
	}
	if sourceRoot == "" {
		return "", errors.New("source is relative but SourceRoot is empty")
	}
	rootExpanded, err := expandPath(sourceRoot)
	if err != nil {
		return "", err
	}
	rootClean := filepath.Clean(rootExpanded)
	joined := filepath.Join(rootClean, expanded)
	rel, err := filepath.Rel(rootClean, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source %q escapes SourceRoot %s", source, rootClean)
	}
	return joined, nil
}

// expandPath expands a leading ~ to the home directory, then $VAR references.
// Only "~", "~/..." and "~\..." expand: "~name" is another user's home and
// must not silently resolve to a path beside ours.
func expandPath(s string) (string, error) {
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		s = filepath.Join(home, s[1:])
	}
	return os.Expand(s, os.Getenv), nil
}

// linkResolvesTo reports whether the symlink at target already points at
// source. readlink may return an absolute path, a path relative to the link's
// directory, or a path with redundant separators — all of which name the same
// file. Falls back to an inode comparison so a link whose text differs but
// resolves to the same file is still recognised.
func linkResolvesTo(target, readlink, source string) bool {
	if readlink == source {
		return true
	}
	resolved := readlink
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(target), resolved)
	}
	if filepath.Clean(resolved) == filepath.Clean(source) {
		return true
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		return false
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return false
	}
	return os.SameFile(targetInfo, sourceInfo)
}

func ensureParentDir(target string) error {
	parent := filepath.Dir(target)
	if parent == "" || parent == "." || parent == target {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parent, err)
	}
	return nil
}
