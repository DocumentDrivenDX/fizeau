package agentcli

import (
	"path/filepath"
	"testing"

	agentConfig "github.com/DocumentDrivenDX/fizeau/internal/config"
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
