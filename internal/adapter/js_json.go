package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
)

type jsPackageEntry struct {
	name    string
	version string
}

type jsListPackage struct {
	Version      string                   `json:"version"`
	Dependencies map[string]jsListPackage `json:"dependencies"`
}

func runJSONPackageList(cmd string, args ...string) ([]jsPackageEntry, error) {
	out, err := runProbe(cmd, args...)
	if err != nil {
		return nil, commandOutputError(err)
	}
	entries, err := parseJSListJSON(out)
	if err != nil {
		return nil, fmt.Errorf("parse %s global package list: %w", cmd, err)
	}
	return entries, nil
}

// parseJSListJSON reads `npm ls -g --json` output, which is an object keyed by
// "name"/"dependencies". Older output wrapped that object in a one-element
// array, so both shapes are accepted.
//
// The object shape is recognised by the JSON itself, not by whether a
// dependencies map happened to be present. An object with no `dependencies`
// key — npm emits one alongside "problems" when the tree is unsatisfiable —
// is a valid, empty listing. Treating its absence as "not the object shape"
// fell through to the array unmarshal and reported
// "cannot unmarshal object into Go value of type []adapter.jsListPackage",
// which reads as a parse failure when nothing was wrong, and made scan
// silently propose zero npm packages (#215).
func parseJSListJSON(data []byte) ([]jsPackageEntry, error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var root jsListPackage
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, err
		}
		return jsEntriesFromDependencies(root.Dependencies), nil
	}

	var roots []jsListPackage
	if err := json.Unmarshal(data, &roots); err != nil {
		return nil, err
	}
	if len(roots) == 0 || roots[0].Dependencies == nil {
		return nil, nil
	}
	return jsEntriesFromDependencies(roots[0].Dependencies), nil
}

func jsEntriesFromDependencies(deps map[string]jsListPackage) []jsPackageEntry {
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]jsPackageEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, jsPackageEntry{name: name, version: deps[name].Version})
	}
	return entries
}

func commandOutputError(err error) error {
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}
	return err
}

func entriesNames(entries []jsPackageEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names
}

func entriesVersions(entries []jsPackageEntry) map[string]string {
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		versions[entry.name] = entry.version
	}
	return versions
}

func findEntry(entries []jsPackageEntry, pkgName string) (jsPackageEntry, bool) {
	base := jsBasePackageName(pkgName)
	for _, entry := range entries {
		if entry.name == base {
			return entry, true
		}
	}
	return jsPackageEntry{}, false
}
