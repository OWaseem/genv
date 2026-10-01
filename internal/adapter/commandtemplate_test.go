package adapter

import (
	"reflect"
	"testing"
)

// The package name is one argv element. ValidPackageName deliberately allows
// shell metacharacters (";", "|", quotes, "$"), and built-in adapters always
// pass the name as its own argument, so a spec-defined adapter template must
// not let an id spill into neighbouring tokens — or into a `sh -c` string.
func TestSplitCommandTemplate_IdIsOneArgvElement(t *testing.T) {
	cases := []struct {
		name string
		tmpl string
		id   string
		want []string
	}{
		{
			name: "plain",
			tmpl: "tool install {{id}}",
			id:   "pkg",
			want: []string{"tool", "install", "pkg"},
		},
		{
			name: "semicolon stays inside one token",
			tmpl: "tool install {{id}}",
			id:   "foo;id",
			want: []string{"tool", "install", "foo;id"},
		},
		{
			name: "space does not split the id",
			tmpl: "tool install {{id}} --force",
			id:   "two words",
			want: []string{"tool", "install", "two words", "--force"},
		},
		{
			name: "embedded placeholder keeps surrounding text",
			tmpl: "tool install --id={{id}}",
			id:   "pkg",
			want: []string{"tool", "install", "--id=pkg"},
		},
		{
			name: "name placeholder is equivalent",
			tmpl: "tool add {{name}}",
			id:   "pkg",
			want: []string{"tool", "add", "pkg"},
		},
		{
			name: "sh -c keeps the id inside the script string as written",
			// The author opted into a shell here; genv does not add escaping.
			// What it must not do is split the id into extra argv entries.
			tmpl: `sh -c "tool install {{id}}"`,
			id:   "foo;id",
			want: []string{"sh", "-c", "tool install foo;id"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitCommandTemplate(tc.tmpl, tc.id)
			if err != nil {
				t.Fatalf("splitCommandTemplate(%q) error = %v", tc.tmpl, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitCommandTemplate(%q, %q) = %q, want %q", tc.tmpl, tc.id, got, tc.want)
			}
		})
	}
}

func TestPlanInstall_IdStaysSingleArgvElement(t *testing.T) {
	c := NewCommand("custom", CommandDef{Install: "tool install {{id}}"})
	got := c.PlanInstall("evil; rm -rf /")
	want := []string{"tool", "install", "evil; rm -rf /"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanInstall = %q, want %q", got, want)
	}
}

func TestSplitCommandTemplate_StillRejectsBadTemplates(t *testing.T) {
	if _, err := splitCommandTemplate(`tool "unclosed {{id}}`, "pkg"); err == nil {
		t.Fatal("expected an unclosed quote error")
	}
	if _, err := splitCommandTemplate("", "pkg"); err == nil {
		t.Fatal("expected an empty command error")
	}
}
