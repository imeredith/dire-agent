package skills_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dire-kiwi/dire-agent/skills"
)

func TestDetectInvocations(t *testing.T) {
	t.Parallel()
	text := "Use $review-code and \\$escaped.\n  /skill:deploy staging --force\n$notes"
	got := skills.DetectInvocations(text)
	if len(got) != 3 {
		t.Fatalf("DetectInvocations() = %#v", got)
	}
	if got[0].Name != "review-code" || got[0].Syntax != skills.SyntaxDollar || text[got[0].Start:got[0].End] != "$review-code" {
		t.Fatalf("first invocation = %#v", got[0])
	}
	if got[1].Name != "deploy" || got[1].Syntax != skills.SyntaxCommand || got[1].Args != "staging --force" {
		t.Fatalf("command invocation = %#v", got[1])
	}
	if got[2].Name != "notes" || got[2].Syntax != skills.SyntaxDollar {
		t.Fatalf("last invocation = %#v", got[2])
	}
}

func TestResolveInvocationsCanonicalizesAndDiagnoses(t *testing.T) {
	t.Parallel()
	catalog := &skills.Catalog{Skills: []skills.Skill{
		{Name: "Review", Enabled: true},
		{Name: "deploy", Enabled: false, Path: "/skills/deploy/SKILL.md"},
	}}
	resolved, issues := catalog.ResolveInvocations("$review $missing\n/skill:deploy now")
	if len(resolved) != 1 || resolved[0].Name != "Review" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if !hasCode(issues, "unknown-invocation") || !hasCode(issues, "disabled-invocation") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestCatalogTextIsStableProgressiveAndCapped(t *testing.T) {
	t.Parallel()
	catalog := &skills.Catalog{}
	for index := 0; index < 200; index++ {
		catalog.Skills = append(catalog.Skills, skills.Skill{
			Name: "skill-" + threeDigits(index), Description: strings.Repeat("界", 80) + " useful metadata",
			Enabled: true,
		})
	}
	catalog.Skills = append(catalog.Skills, skills.Skill{Name: "disabled", Description: "must stay hidden", Enabled: false})
	text := catalog.CatalogText()
	if utf8.RuneCountInString(text) > skills.MaxCatalogChars {
		t.Fatalf("catalog has %d characters", utf8.RuneCountInString(text))
	}
	if !strings.HasPrefix(text, "<available_skills>") || !strings.Contains(text, "skill-000") {
		t.Fatalf("catalog text = %q", text[:min(200, len(text))])
	}
	if strings.Contains(text, "must stay hidden") || !strings.Contains(text, "more skill") {
		t.Fatalf("catalog disclosure/cap failed")
	}
	if got := catalog.CatalogTextLimit(32); utf8.RuneCountInString(got) != 32 {
		t.Fatalf("small catalog length = %d, want 32", utf8.RuneCountInString(got))
	}
}

func threeDigits(value int) string {
	return string([]byte{'0' + byte(value/100), '0' + byte(value/10%10), '0' + byte(value%10)})
}
