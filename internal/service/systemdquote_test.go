package service

import (
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

// systemd expands % specifiers inside quoted ExecStart= and Environment=
// values. A literal % must be doubled or the process sees something else.
func TestSystemdQuoteArg_EscapesPercent(t *testing.T) {
	cases := map[string]string{
		`date +%s`:   `"date +%%s"`,
		`printf %d`:  `"printf %%d"`,
		`100%`:       `"100%%"`,
		`%Y-%m-%d`:   `"%%Y-%%m-%%d"`,
		`already %%`: `"already %%%%"`,
		`no-percent`: `"no-percent"`,
	}
	for in, want := range cases {
		if got := systemdQuoteArg(in); got != want {
			t.Errorf("systemdQuoteArg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSystemdQuoteArg_StillEscapesBackslashAndQuote(t *testing.T) {
	if got := systemdQuoteArg(`a\b"c`); got != `"a\\b\"c"` {
		t.Errorf("systemdQuoteArg = %q, want %q", got, `"a\\b\"c"`)
	}
}

func TestSystemdUnitContent_EscapesPercentInExecStart(t *testing.T) {
	svc := schema.Service{Start: []string{"/usr/bin/date", "+%s"}}
	content := SystemdUnitContent("stamp", svc)
	if !strings.Contains(content, `ExecStart="/usr/bin/date" "+%%s"`) {
		t.Fatalf("unit content missing escaped specifier:\n%s", content)
	}
	if strings.Contains(content, `"+%s"`) {
		t.Fatalf("unit content contains an unescaped %% specifier:\n%s", content)
	}
}

func TestRenderSystemdEnvironment_EscapesPercentInValues(t *testing.T) {
	out := renderSystemdEnvironment(map[string]string{
		"TEMPLATE": "date +%Y",
		"PLAIN":    "value",
	})
	if !strings.Contains(out, `Environment="TEMPLATE=date +%%Y"`) {
		t.Fatalf("environment not escaped:\n%s", out)
	}
	if !strings.Contains(out, `Environment="PLAIN=value"`) {
		t.Fatalf("plain value altered:\n%s", out)
	}
}

// launchd and macOS do not expand % the way systemd does, so only the systemd
// renderers may double it.
func TestLaunchdEnvironment_DoesNotDoublePercent(t *testing.T) {
	out := renderLaunchdEnvironment(map[string]string{"TEMPLATE": "date +%Y"})
	if !strings.Contains(out, "date +%Y") {
		t.Fatalf("launchd environment altered:\n%s", out)
	}
}

func TestLaunchdPlistContent_DoesNotDoublePercent(t *testing.T) {
	svc := schema.Service{Start: []string{"/usr/bin/date", "+%s"}}
	content := LaunchdPlistContent("stamp", svc)
	if !strings.Contains(content, "<string>+%s</string>") {
		t.Fatalf("plist content altered:\n%s", content)
	}
}
