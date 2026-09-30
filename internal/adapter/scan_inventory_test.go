package adapter

import (
	"reflect"
	"sort"
	"testing"
)

// #214: `genv scan` proposes "leaves, not the full dependency tree", but pacman
// proposed essentially every installed package (767 of 771 proposals on the
// reporter's host). paru and yay already list only explicit packages; pacman
// itself used the full -Qq inventory.
func TestPacman_ListForScanIsExplicitPackagesOnly(t *testing.T) {
	installFakeBinary(t, "pacman", `
case "$1" in
  -Qq)  printf 'glibc\nlinux\nvim\n' ;;
  -Qqe) printf 'linux\nvim\n' ;;
  *)    exit 1 ;;
esac
`)

	full, err := Pacman{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	sort.Strings(full)
	if want := []string{"glibc", "linux", "vim"}; !reflect.DeepEqual(full, want) {
		t.Fatalf("ListInstalled = %v, want %v (--all keeps the full inventory)", full, want)
	}

	scan, err := Pacman{}.ListForScan()
	if err != nil {
		t.Fatalf("ListForScan: %v", err)
	}
	sort.Strings(scan)
	if want := []string{"linux", "vim"}; !reflect.DeepEqual(scan, want) {
		t.Fatalf("ListForScan = %v, want %v (explicitly installed only, no glibc)", scan, want)
	}
}

// #216: the default scan proposed Ruby's own default gems. The `default:`
// marker in `gem list --local` is not a default-gem flag: RubyGems only prints
// it when a *second*, non-default version is also installed. A default gem
// with a single version appears unmarked, which is the abbrev/base64/csv case
// from the report.
func TestGem_ListForScanExcludesRubyDefaultGems(t *testing.T) {
	installFakeBinary(t, "gem", `
case "$1 $2" in
  "environment gemdir") printf '%s\n' "$GENV_TEST_GEMDIR" ;;
  "list --local")
cat <<'LIST'
*** LOCAL GEMS ***

abbrev (0.1.2)
base64 (0.3.0)
bundler (default: 4.0.20, 4.0.18)
csv (3.3.6, 3.3.5)
rake (13.4.2)
rubocop (1.75.0)
LIST
  ;;
  *) exit 1 ;;
esac
`)
	t.Setenv("GENV_TEST_GEMDIR", t.TempDir())

	orig := gemDefaultGems
	gemDefaultGems = func() map[string]bool {
		return map[string]bool{"abbrev": true, "base64": true, "bundler": true, "csv": true}
	}
	t.Cleanup(func() { gemDefaultGems = orig })

	scan, err := Gem{}.ListForScan()
	if err != nil {
		t.Fatalf("ListForScan: %v", err)
	}
	sort.Strings(scan)
	// rake is filtered by the bundled-gem map; rubocop is the only user choice.
	if want := []string{"rubocop"}; !reflect.DeepEqual(scan, want) {
		t.Fatalf("ListForScan = %v, want %v", scan, want)
	}

	// ListInstalled stays complete: apply, status and upgrade still need the
	// full inventory, including default gems.
	full, err := Gem{}.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(full) != 6 {
		t.Fatalf("ListInstalled = %v, want all 6 entries", full)
	}
}

// #215: `npm ls -g --json --depth=0` returns an object. When that object has
// no `dependencies` key the object branch was skipped and the array unmarshal
// ran instead, failing with "cannot unmarshal object into Go value of type
// []adapter.jsListPackage" for what is a successful parse of an empty object.
func TestParseJSListJSON_EmptyObjectIsNotAnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string
	}{
		{"no dependencies key", `{"name":"/usr/local/lib","problems":["peer dep missing"]}`, nil},
		{"empty dependencies", `{"name":"global","dependencies":{}}`, []string{}},
		{"populated", `{"name":"global","dependencies":{"npm":{"version":"11.0.0"}}}`, []string{"npm"}},
		{"legacy array", `[{"name":"global","dependencies":{"yarn":{"version":"1.22.0"}}}]`, []string{"yarn"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseJSListJSON([]byte(tc.in))
			if err != nil {
				t.Fatalf("parseJSListJSON(%s) error = %v, want nil", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseJSListJSON(%s) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
