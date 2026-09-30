// Package pull contains helpers for materializing assets from a pulled spec
// repository.
package pull

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// BundleAssetSources returns the safe relative file asset sources referenced by
// a genv spec. Schema v8 specs collect assets from defaults and every target;
// earlier specs use the top-level files block. Hook scripts declared with
// `file` are included: they live next to the spec and a pulled spec without
// them loses its hooks.
func BundleAssetSources(f *schema.GenvFile) []string {
	if f == nil {
		return nil
	}
	seen := make(map[string]bool)
	addFiles := func(files *schema.FilesConfig) {
		if files == nil {
			return
		}
		for _, link := range files.Links {
			addBundleAssetSource(seen, link.Source)
		}
		for _, tmpl := range files.Templates {
			addBundleAssetSource(seen, tmpl.Source)
		}
	}
	addHooks := func(hooks *schema.HooksConfig) {
		for _, phase := range allHookPhases(hooks) {
			for _, h := range phase {
				addBundleAssetSource(seen, h.File)
			}
		}
	}

	if schema.IsPortableVersion(f.SchemaVersion) {
		if f.Defaults != nil {
			addFiles(f.Defaults.Files)
			addHooks(f.Defaults.Hooks)
		}
		targets := make([]string, 0, len(f.Targets))
		for target := range f.Targets {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		for _, target := range targets {
			if f.Targets[target] != nil {
				addFiles(f.Targets[target].Files)
				addHooks(f.Targets[target].Hooks)
			}
		}
	} else {
		addFiles(f.Files)
		addHooks(f.Hooks)
	}

	out := make([]string, 0, len(seen))
	for source := range seen {
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}

func allHookPhases(hooks *schema.HooksConfig) [][]schema.Hook {
	if hooks == nil {
		return nil
	}
	return [][]schema.Hook{
		hooks.PreApply, hooks.PostApply,
		hooks.PreAdd, hooks.PostAdd,
		hooks.PreRemove, hooks.PostRemove,
		hooks.PreUpgrade, hooks.PostUpgrade,
	}
}

// CopyBundleAssets copies safe relative files.links/templates sources from
// cacheDir into destDir, preserving each source's relative path. It returns the
// normalized relative paths copied.
func CopyBundleAssets(cacheDir, destDir string, f *schema.GenvFile) ([]string, error) {
	sources := BundleAssetSources(f)
	copied := make([]string, 0, len(sources))
	for _, source := range sources {
		if err := copyBundleAsset(cacheDir, destDir, source); err != nil {
			return copied, err
		}
		copied = append(copied, source)
	}
	return copied, nil
}

func addBundleAssetSource(seen map[string]bool, source string) {
	rel, ok := normalizeBundleAssetSource(source)
	if !ok {
		return
	}
	seen[rel] = true
}

func normalizeBundleAssetSource(source string) (string, bool) {
	if source == "" || isAbsoluteAssetSource(source) {
		return "", false
	}
	clean := path.Clean(strings.ReplaceAll(source, `\`, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	if shouldSkipBundlePath(clean) {
		return "", false
	}
	return clean, true
}

func isAbsoluteAssetSource(source string) bool {
	if filepath.IsAbs(source) || strings.HasPrefix(source, "/") {
		return true
	}
	slash := strings.ReplaceAll(source, `\`, "/")
	if strings.HasPrefix(slash, "//") {
		return true
	}
	// "~" and "~/..." are home-relative, not repo-relative: apply expands them
	// before reading, so they name a file outside the cache and must not be
	// bundled. "~name" is another user's home and is likewise not repo-local.
	if source == "~" || strings.HasPrefix(source, "~/") || strings.HasPrefix(source, `~\`) {
		return true
	}
	return len(slash) >= 2 && isASCIIAlpha(slash[0]) && slash[1] == ':'
}

func shouldSkipBundlePath(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		if segment == "secrets" || isLockfileName(segment) {
			return true
		}
	}
	return false
}

func isLockfileName(name string) bool {
	return name == "genv.lock.json" || strings.HasSuffix(name, ".lock.json")
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func copyBundleAsset(cacheDir, destDir, rel string) error {
	src := filepath.Join(cacheDir, filepath.FromSlash(rel))
	dst := filepath.Join(destDir, filepath.FromSlash(rel))
	if err := requireNoSymlinkComponents(cacheDir, src, rel); err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("copying bundle asset %s: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("copying bundle asset %s: symlinks are not allowed", rel)
	}
	if info.IsDir() {
		return copyBundleDir(src, dst, rel)
	}
	return copyBundleFile(src, dst, info.Mode().Perm())
}

// requireNoSymlinkComponents walks every path element from root to src. Only
// checking the final path missed an intermediate symlink: with "a" pointing
// outside the cache, the source "a/id_ed25519" passed the final Lstat and the
// copy still read from outside the cache.
func requireNoSymlinkComponents(cacheDir, src, rel string) error {
	relCheck, err := filepath.Rel(cacheDir, src)
	if err != nil {
		return fmt.Errorf("copying bundle asset %s: %w", rel, err)
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return fmt.Errorf("copying bundle asset %s: escapes the cache directory", rel)
	}
	current := filepath.Clean(cacheDir)
	for _, part := range strings.Split(filepath.ToSlash(relCheck), "/") {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("copying bundle asset %s: %w", rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("copying bundle asset %s: symlinks are not allowed in the path", rel)
		}
	}
	return nil
}

func copyBundleDir(srcRoot, dstRoot, relRoot string) error {
	return filepath.WalkDir(srcRoot, func(src string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, src)
		if err != nil {
			return err
		}
		nestedRel := relRoot
		if rel != "." {
			nestedRel = path.Join(relRoot, filepath.ToSlash(rel))
			if shouldSkipBundlePath(nestedRel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("copying bundle asset %s: symlinks are not allowed", nestedRel)
		}
		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyBundleFile(src, dst, info.Mode().Perm())
	})
}

func copyBundleFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
