package skills_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/skills"
)

func TestToolListsMetadataAndLoadsFullInstructions(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "skills")
	path := writeSkill(t, filepath.Join(root, "review"), "review", "Review changes", "SECRET FULL BODY")
	catalog, err := skills.Discover(skills.Config{GlobalRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	tool := skills.NewTool(catalog)
	var _ agentloop.Tool = tool
	if definition := tool.Definition(); definition.Name != "skill" || !json.Valid(definition.Parameters) {
		t.Fatalf("definition = %#v", definition)
	}
	listed, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil || !strings.Contains(listed, "Review changes") || strings.Contains(listed, "SECRET FULL BODY") {
		t.Fatalf("list = %q, %v", listed, err)
	}
	loaded, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"load","name":"review"}`))
	if err != nil || !strings.Contains(loaded, "SECRET FULL BODY") || !strings.Contains(loaded, "name: review") {
		t.Fatalf("load = %q, %v", loaded, err)
	}
	if loadedBytes, err := os.ReadFile(path); err != nil || loaded != string(loadedBytes) {
		t.Fatalf("loaded content differs from SKILL.md: %v", err)
	}
}

func TestToolRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	tool := skills.NewTool(&skills.Catalog{})
	tests := []string{
		`{"action":"load"}`,
		`{"action":"list","name":"extra"}`,
		`{"action":"remove","name":"x"}`,
		`{"action":"list","extra":true}`,
		`{"action":"list"} {}`,
	}
	for _, input := range tests {
		if _, err := tool.Execute(context.Background(), json.RawMessage(input)); err == nil {
			t.Fatalf("Execute(%s) unexpectedly succeeded", input)
		}
	}
}

func TestLoadDetectsMutationAndSizeLimit(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "skills")
	path := writeSkill(t, filepath.Join(root, "small"), "small", "Small skill", "body")
	catalog, err := skills.Discover(skills.Config{GlobalRoots: []string{root}, MaxSkillBytes: 256})
	if err != nil {
		t.Fatal(err)
	}
	changed := "---\nname: other\ndescription: Changed identity\n---\nbody\n"
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Load("small"); err == nil || !strings.Contains(err.Error(), "changed since discovery") {
		t.Fatalf("Load(mutated) error = %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 300)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Load("small"); err == nil || !strings.Contains(err.Error(), "exceeds 256 bytes") {
		t.Fatalf("Load(oversize) error = %v", err)
	}
	if _, err := catalog.Load("missing"); !errors.Is(err, skills.ErrSkillNotFound) {
		t.Fatalf("Load(missing) error = %v", err)
	}
}

func TestDiscoveryRejectsNULAndOversizeFiles(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "skills")
	nulDir := filepath.Join(root, "nul")
	largeDir := filepath.Join(root, "large")
	mustMkdir(t, nulDir)
	mustMkdir(t, largeDir)
	if err := os.WriteFile(filepath.Join(nulDir, "SKILL.md"), append([]byte("---\nname: nul\ndescription: bad\n---\n"), 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(largeDir, "SKILL.md"), []byte(strings.Repeat("x", 101)), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog, err := skills.Discover(skills.Config{GlobalRoots: []string{root}, MaxSkillBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 0 || !hasCode(catalog.Diagnostics, "invalid-text") || !hasCode(catalog.Diagnostics, "file-too-large") {
		t.Fatalf("catalog = %#v", catalog)
	}
}
