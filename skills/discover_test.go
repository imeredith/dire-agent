package skills_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/skills"
)

func TestDiscoverScopesAncestorCompatibilityAndPrecedence(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	project := filepath.Join(base, "repo", "work", "nested")
	global := filepath.Join(base, "global")
	plugin := filepath.Join(base, "plugin")
	mustMkdir(t, project)

	writeSkill(t, filepath.Join(global, "duplicate"), "duplicate", "global copy", "global body")
	writeSkill(t, filepath.Join(global, "alpha"), "alpha", "global alpha", "alpha body")
	writeSkill(t, filepath.Join(plugin, "plugin-skill"), "plugin-skill", "from plugin", "plugin body")
	writeSkill(t, filepath.Join(base, "repo", ".agents", "skills", "duplicate"), "duplicate", "project copy", "project body")
	writeSkill(t, filepath.Join(base, "repo", ".codex", "skills", "codex-compatible"), "codex-compatible", "codex", "codex body")
	writeSkill(t, filepath.Join(base, "repo", "work", ".pi", "skills", "pi-compatible"), "pi-compatible", "pi", "pi body")

	catalog, err := skills.Discover(skills.Config{
		ProjectDir: project, GlobalRoots: []string{global},
		PluginRoots: []skills.PluginRoot{{Name: "example", Path: plugin}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"alpha", "codex-compatible", "duplicate", "pi-compatible", "plugin-skill"}
	if len(catalog.Skills) != len(wantNames) {
		t.Fatalf("skills = %#v", catalog.Skills)
	}
	for index, want := range wantNames {
		if catalog.Skills[index].Name != want {
			t.Fatalf("skills[%d].Name = %q, want %q", index, catalog.Skills[index].Name, want)
		}
	}
	duplicate, ok := catalog.Find("duplicate")
	if !ok || duplicate.Scope != skills.ScopeProject || duplicate.Description != "project copy" {
		t.Fatalf("duplicate = %#v, found %v", duplicate, ok)
	}
	pluginSkill, ok := catalog.Find("plugin-skill")
	if !ok || pluginSkill.Scope != skills.ScopePlugin || pluginSkill.Plugin != "example" {
		t.Fatalf("plugin skill = %#v, found %v", pluginSkill, ok)
	}
	if !hasCode(catalog.Diagnostics, "duplicate-name") {
		t.Fatalf("diagnostics = %#v, want duplicate-name", catalog.Diagnostics)
	}
	loaded, err := catalog.Load("duplicate")
	if err != nil || loaded == "" || !strings.Contains(loaded, "project body") {
		t.Fatalf("Load(duplicate) = %q, %v", loaded, err)
	}
}

func TestDiscoverPathRulesKeepDisabledEntries(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "skills")
	firstDir := filepath.Join(root, "first")
	secondDir := filepath.Join(root, "second")
	writeSkill(t, firstDir, "first", "first skill", "body")
	writeSkill(t, secondDir, "second", "second skill", "body")

	catalog, err := skills.Discover(skills.Config{
		GlobalRoots: []string{root}, DisabledPaths: []string{root},
		PathRules: []skills.PathRule{{Path: secondDir, Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 2 || len(catalog.EnabledSkills()) != 1 {
		t.Fatalf("skills/enabled = %#v / %#v", catalog.Skills, catalog.EnabledSkills())
	}
	first, _ := catalog.FindAny("first")
	if first.Enabled || first.DisabledReason == "" {
		t.Fatalf("first = %#v, want disabled reason", first)
	}
	if _, found := catalog.Find("first"); found {
		t.Fatal("Find returned a disabled skill")
	}
	if _, err := catalog.Load("first"); !errors.Is(err, skills.ErrSkillDisabled) {
		t.Fatalf("Load(disabled) error = %v", err)
	}
	if second, found := catalog.Find("second"); !found || !second.Enabled {
		t.Fatalf("second = %#v, found %v", second, found)
	}
}

func TestDiscoverFollowsSymlinksOnlyWithExplicitExternalOptIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	t.Parallel()
	base := t.TempDir()
	root := filepath.Join(base, "root")
	external := filepath.Join(base, "external")
	mustMkdir(t, root)
	writeSkill(t, filepath.Join(external, "linked"), "linked", "linked skill", "linked body")
	if err := os.Symlink(filepath.Join(external, "linked"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	locked, err := skills.Discover(skills.Config{GlobalRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(locked.Skills) != 0 || !hasCode(locked.Diagnostics, "symlink-escape") {
		t.Fatalf("locked catalog = %#v / %#v", locked.Skills, locked.Diagnostics)
	}
	unlocked, err := skills.Discover(skills.Config{GlobalRoots: []string{root}, AllowExternalSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if skill, found := unlocked.Find("linked"); !found || skill.Name != "linked" {
		t.Fatalf("linked skill = %#v, found %v; diagnostics %#v", skill, found, unlocked.Diagnostics)
	}
}

func TestProjectSkillRootSymlinkCannotEscapeByDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	t.Parallel()
	base := t.TempDir()
	project := filepath.Join(base, "project")
	external := filepath.Join(base, "external")
	mustMkdir(t, filepath.Join(project, ".agents"))
	writeSkill(t, filepath.Join(external, "outside"), "outside", "outside skill", "body")
	if err := os.Symlink(external, filepath.Join(project, ".agents", "skills")); err != nil {
		t.Fatal(err)
	}
	catalog, err := skills.Discover(skills.Config{ProjectDir: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 0 || !hasCode(catalog.Diagnostics, "symlink-root-escape") {
		t.Fatalf("catalog = %#v", catalog)
	}
	trusted, err := skills.Discover(skills.Config{ProjectDir: project, AllowExternalSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := trusted.Find("outside"); !found {
		t.Fatalf("trusted catalog = %#v", trusted)
	}
}

func TestDiscoverReportsBadSourcesAndContinues(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	root := filepath.Join(base, "skills")
	writeSkill(t, filepath.Join(root, "good"), "good", "valid", "body")
	bad := filepath.Join(root, "bad")
	mustMkdir(t, bad)
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("not frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog, err := skills.Discover(skills.Config{GlobalRoots: []string{root, filepath.Join(base, "missing")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := catalog.Find("good"); !found {
		t.Fatalf("valid skill missing: %#v", catalog)
	}
	if !hasCode(catalog.Diagnostics, "missing-frontmatter") || !hasCode(catalog.Diagnostics, "root-unavailable") {
		t.Fatalf("diagnostics = %#v", catalog.Diagnostics)
	}
}

func writeSkill(t *testing.T, directory, name, description, body string) string {
	t.Helper()
	mustMkdir(t, directory)
	path := filepath.Join(directory, "SKILL.md")
	text := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
