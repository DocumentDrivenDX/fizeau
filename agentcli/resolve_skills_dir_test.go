package agentcli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fizeau "github.com/DocumentDrivenDX/fizeau"
	agentConfig "github.com/DocumentDrivenDX/fizeau/internal/config"
	"github.com/DocumentDrivenDX/fizeau/internal/prompt"
)

func TestResolveSkillsDir_DefaultsToProjectDir(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "")
	cfg := &agentConfig.Config{}
	got := resolveSkillsDir(cfg, "/work")
	want := filepath.Join("/work", ".fizeau", "skills")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveSkillsDir_EnvOverride(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "/custom/skills")
	cfg := &agentConfig.Config{Skills: agentConfig.SkillsConfig{Dir: "/cfg/skills"}}
	if got := resolveSkillsDir(cfg, "/work"); got != "/custom/skills" {
		t.Fatalf("env override failed; got %q", got)
	}
}

func TestResolveSkillsDir_EnvDashDisables(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "-")
	cfg := &agentConfig.Config{Skills: agentConfig.SkillsConfig{Dir: "/cfg/skills"}}
	if got := resolveSkillsDir(cfg, "/work"); got != "" {
		t.Fatalf("env \"-\" must disable; got %q", got)
	}
}

func TestResolveSkillsDir_ConfigOverride(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "")
	cfg := &agentConfig.Config{Skills: agentConfig.SkillsConfig{Dir: "/cfg/skills"}}
	if got := resolveSkillsDir(cfg, "/work"); got != "/cfg/skills" {
		t.Fatalf("config override failed; got %q", got)
	}
}

func TestResolveSkillsDir_ConfigDashDisables(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "")
	cfg := &agentConfig.Config{Skills: agentConfig.SkillsConfig{Dir: "-"}}
	if got := resolveSkillsDir(cfg, "/work"); got != "" {
		t.Fatalf("config \"-\" must disable; got %q", got)
	}
}

func TestResolveSkillsDir_NilConfig(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "")
	got := resolveSkillsDir(nil, "/work")
	want := filepath.Join("/work", ".fizeau", "skills")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSkillsProgressiveDisclosureIntegration(t *testing.T) {
	t.Setenv("FIZEAU_SKILLS_DIR", "")
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".fizeau", "skills", "example")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	body := "# Example Skill\n\nUse the example workflow.\n"
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: example\ndescription: Example skill for integration testing.\n---\n"+body), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	catalog, warnings, err := fizeau.ScanSkillsDir(resolveSkillsDir(&agentConfig.Config{}, workDir))
	if err != nil {
		t.Fatalf("ScanSkillsDir: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}

	systemPrompt := prompt.New("Base.").WithSkillCatalog(catalog).Build()
	if !strings.Contains(systemPrompt, "# Available Skills") {
		t.Fatalf("system prompt missing skill catalog:\n%s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "- example: Example skill for integration testing.") {
		t.Fatalf("system prompt missing skill entry:\n%s", systemPrompt)
	}

	loader := fizeau.NewLoadSkillTool(catalog)
	if loader == nil {
		t.Fatal("NewLoadSkillTool returned nil")
	}
	params, err := json.Marshal(map[string]string{"skill_name": "example"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	got, err := loader.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("load_skill Execute: %v", err)
	}
	if got != body {
		t.Fatalf("load_skill body = %q, want %q", got, body)
	}
}
