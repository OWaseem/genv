package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v8Doc wraps a defaults block in an otherwise valid schemaVersion 8 spec.
func v8Doc(defaults string) string {
	return `{"schemaVersion":"8","defaults":` + defaults + `,"targets":{"macos":{}}}`
}

func validateDoc(t *testing.T, doc string) []ValidationError {
	t.Helper()
	_, errs, parseErr := ParseAndValidate([]byte(doc))
	if parseErr != nil {
		t.Fatalf("ParseAndValidate: %v", parseErr)
	}
	return errs
}

func TestValidate_AliasValueCannotClosePowerShellWrapper(t *testing.T) {
	cases := map[string]string{
		"closing brace": `} true`,
		"opening brace": `Get-ChildItem {`,
		"newline":       "Get-ChildItem\nRemove-Item foo",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			doc := v8Doc(`{"shell":{"aliases":{"evil":{"value":` + quoteJSON(value) + `,"shell":"powershell"}}}}`)
			errs := validateDoc(t, doc)
			if !hasErrFieldContaining(errs, "aliases.evil") {
				t.Fatalf("expected an alias validation error, got %v", errs)
			}
		})
	}
}

// POSIX aliases are emitted single-quoted (`alias g='for i in {1..10}'`), so a
// brace cannot escape the wrapper there. These are ordinary working aliases and
// must keep validating.
func TestValidate_PosixAliasBracesStillPass(t *testing.T) {
	doc := v8Doc(`{"shell":{"aliases":{` +
		`"zbench":{"value":"for i in {1..10}; do date; done","shell":"bash"},` +
		`"zdot":{"value":"cd ${ZDOTDIR:-~}","shell":"zsh"},` +
		`"zshrc":{"value":"${EDITOR:-nvim} \"${ZDOTDIR:-$HOME}\"/.zshrc","shell":"zsh"}` +
		`}}}`)
	if errs := validateDoc(t, doc); len(errs) != 0 {
		t.Fatalf("validation errors = %v, want none for ordinary POSIX aliases", errs)
	}
}

// The same value IS rejected for the PowerShell form, where genv cannot quote it.
func TestValidate_PowerShellAliasBracesRejected(t *testing.T) {
	doc := v8Doc(`{"shell":{"aliases":{"evil":{"value":"for i in {1..10}","shell":"powershell"}}}}`)
	errs := validateDoc(t, doc)
	if !hasErrFieldContaining(errs, "aliases.evil") {
		t.Fatalf("expected an alias validation error, got %v", errs)
	}
}

func TestValidate_OrdinaryAliasValuesStillPass(t *testing.T) {
	doc := v8Doc(`{"shell":{"aliases":{` +
		`"ll":{"value":"Get-ChildItem","shell":"powershell"},` +
		`"g":{"value":"git log --graph --oneline","shell":"bash"},` +
		`"p":{"value":"ps aux | grep go","shell":"bash"}` +
		`}}}`)
	if errs := validateDoc(t, doc); len(errs) != 0 {
		t.Fatalf("validation errors = %v, want none", errs)
	}
}

func TestValidate_FunctionBodyCannotCloseWrapper(t *testing.T) {
	doc := v8Doc(`{"shell":{"functions":{"evil":{"body":` + quoteJSON("} true") + `,"shell":"bash"}}}}`)
	errs := validateDoc(t, doc)
	if !hasErrFieldContaining(errs, "functions.evil.body") {
		t.Fatalf("expected a function body validation error, got %v", errs)
	}
}

func TestValidate_HookCommandMustNotContainNewlines(t *testing.T) {
	doc := v8Doc(`{"hooks":{"preApply":[{"name":"x","command":` +
		quoteJSON("echo hi\necho second") + `}]}}`)
	errs := validateDoc(t, doc)
	if !hasErrFieldContaining(errs, ".command") {
		t.Fatalf("expected a hook command validation error, got %v", errs)
	}
}

func TestValidate_HookFileStillRejectsNewlines(t *testing.T) {
	doc := v8Doc(`{"hooks":{"preApply":[{"name":"x","file":` +
		quoteJSON("scripts/hook.sh\n") + `}]}}`)
	errs := validateDoc(t, doc)
	if !hasErrFieldContaining(errs, ".file") {
		t.Fatalf("expected a hook file validation error, got %v", errs)
	}
}

func TestValidate_HookSingleLineCommandPasses(t *testing.T) {
	doc := v8Doc(`{"hooks":{"preApply":[{"name":"x","command":"echo hi"}]}}`)
	if errs := validateDoc(t, doc); len(errs) != 0 {
		t.Fatalf("validation errors = %v, want none", errs)
	}
}

func TestValidate_TildeUserPathIsNotHomeExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}

	got, err := expandPath("~root/x")
	if err != nil {
		t.Fatalf("expandPath: %v", err)
	}
	// The old bug produced $HOME + "root/x"; leaving it relative is correct.
	if got == filepath.Join(home, "root/x") || got == home+"root/x" {
		t.Fatalf("expandPath(~root/x) = %q, want it not expanded under home %s", got, home)
	}
	if want := "~root/x"; got != want {
		t.Errorf("expandPath(~root/x) = %q, want %q left relative", got, want)
	}

	// The forms that are genuinely home-relative still expand.
	for in, want := range map[string]string{
		"~":     home,
		"~/foo": filepath.Join(home, "foo"),
	} {
		if got, err := expandPath(in); err != nil || got != want {
			t.Errorf("expandPath(%q) = %q (err %v), want %q", in, got, err, want)
		}
	}
}

func hasErrFieldContaining(errs []ValidationError, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Field, substr) {
			return true
		}
	}
	return false
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
