package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/easel/fizeau/internal/core"
)

// LoadSkillTool exposes a *Catalog to the agent loop as the `load_skill`
// tool. The tool returns the raw markdown body of the named skill on
// demand, supporting the progressive-disclosure pattern: only the
// frontmatter summary is in the system prompt; full instructions are
// loaded lazily.
type LoadSkillTool struct {
	Catalog *Catalog
}

// compile-time assertion that LoadSkillTool implements agent.Tool.
var _ agent.Tool = (*LoadSkillTool)(nil)

// Name returns the tool identifier.
func (t *LoadSkillTool) Name() string { return "load_skill" }

// Description returns the LLM-facing description.
func (t *LoadSkillTool) Description() string {
	return "Load the full instructions for a named skill. Returns the complete SKILL.md body as markdown."
}

// Schema returns the JSON schema for the tool's parameters.
func (t *LoadSkillTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"skill_name": {"type": "string", "description": "Name of the skill to load (matches the frontmatter \"name\" field)."}
		},
		"required": ["skill_name"]
	}`)
}

// Parallel reports that load_skill is safe to run concurrently — it only
// reads files.
func (t *LoadSkillTool) Parallel() bool { return true }

type loadSkillParams struct {
	SkillName string `json:"skill_name"`
}

// Execute returns the markdown body of the named skill, or an error
// listing the available names when the requested name is unknown.
func (t *LoadSkillTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p loadSkillParams
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("load_skill: invalid params: %w", err)
	}
	if p.SkillName == "" {
		return "", fmt.Errorf("load_skill: skill_name is required")
	}
	if t.Catalog == nil || t.Catalog.Len() == 0 {
		return "", fmt.Errorf("load_skill: no skills are configured")
	}
	if t.Catalog.ByName(p.SkillName) == nil {
		return "", fmt.Errorf("load_skill: skill %q not found; available: %s", p.SkillName, strings.Join(t.Catalog.Names(), ", "))
	}
	body, err := t.Catalog.LoadBody(p.SkillName)
	if err != nil {
		return "", fmt.Errorf("load_skill: %w", err)
	}
	return body, nil
}
