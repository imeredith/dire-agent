package skills_test

import (
	"strings"
	"testing"

	"github.com/dire-kiwi/dire-agent/skills"
)

func TestParseFrontmatterScalarsAndBlocks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		frontmatter string
		want        skills.Metadata
	}{
		{
			name:        "quoted",
			frontmatter: "---\nname: \"code-review\"\ndescription: 'Review code: carefully'\nlicense: MIT\n---\n# Body\n",
			want:        skills.Metadata{Name: "code-review", Description: "Review code: carefully"},
		},
		{
			name:        "folded description",
			frontmatter: "---\nname: planning\ndescription: >-\n  Plan work across\n  multiple stages.\n\n  Keep it practical.\nmetadata:\n  author: test\n---\n",
			want:        skills.Metadata{Name: "planning", Description: "Plan work across multiple stages.\nKeep it practical."},
		},
		{
			name:        "literal description",
			frontmatter: "---\nname: notes\ndescription: |\n  First line\n  second line\n---\n",
			want:        skills.Metadata{Name: "notes", Description: "First line\nsecond line"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			metadata, issues := skills.ParseFrontmatter([]byte(test.frontmatter), "SKILL.md")
			if hasSeverity(issues, skills.SeverityError) {
				t.Fatalf("ParseFrontmatter() issues = %#v", issues)
			}
			if metadata != test.want {
				t.Fatalf("metadata = %#v, want %#v", metadata, test.want)
			}
		})
	}
}

func TestParseFrontmatterDiagnostics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		code string
	}{
		{"missing delimiters", "name: nope\n", "missing-frontmatter"},
		{"unterminated", "---\nname: nope\n", "unterminated-frontmatter"},
		{"missing name", "---\ndescription: useful\n---\n", "missing-name"},
		{"missing description", "---\nname: useful\n---\n", "missing-description"},
		{"duplicate", "---\nname: one\nname: two\ndescription: useful\n---\n", "duplicate-field"},
		{"unsafe name", "---\nname: ../secret\ndescription: nope\n---\n", "invalid-name"},
		{"bad quote", "---\nname: \"oops\ndescription: nope\n---\n", "invalid-scalar"},
		{"bad utf8", string([]byte{0xff, 0xfe}), "invalid-utf8"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, issues := skills.ParseFrontmatter([]byte(test.text), "/tmp/SKILL.md")
			if !hasCode(issues, test.code) {
				t.Fatalf("diagnostics = %#v, want code %q", issues, test.code)
			}
		})
	}
}

func TestParseFrontmatterAcceptsCompatibilityNameWithWarning(t *testing.T) {
	t.Parallel()
	metadata, issues := skills.ParseFrontmatter([]byte("---\nname: Presentations\ndescription: Build slides\n---\n"), "SKILL.md")
	if metadata.Name != "Presentations" || hasSeverity(issues, skills.SeverityError) {
		t.Fatalf("metadata/issues = %#v / %#v", metadata, issues)
	}
	if !hasCode(issues, "nonstandard-name") {
		t.Fatalf("issues = %#v, want compatibility warning", issues)
	}
}

func TestDescriptionLengthUsesCharacters(t *testing.T) {
	t.Parallel()
	description := strings.Repeat("界", 1024)
	_, issues := skills.ParseFrontmatter([]byte("---\nname: unicode\ndescription: "+description+"\n---\n"), "SKILL.md")
	if hasSeverity(issues, skills.SeverityError) {
		t.Fatalf("1024 Unicode characters should be accepted: %#v", issues)
	}
}

func hasCode(issues []skills.Diagnostic, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasSeverity(issues []skills.Diagnostic, severity skills.Severity) bool {
	for _, issue := range issues {
		if issue.Severity == severity {
			return true
		}
	}
	return false
}
