package fizeau

// public_cli_api.go re-exports the minimal set of types and helpers that the
// `cmd/fiz` CLI binary needs. These exist so the binary can stay behind a
// strict service-boundary import allowlist (see
// cmd/fiz/service_boundary_test.go) while still using shared building
// blocks. Add re-exports here only when removing one would force the CLI to
// import an internal package directly.

import (
	"context"

	"github.com/easel/fizeau/internal/compaction"
	agentcore "github.com/easel/fizeau/internal/core"
	oaiProvider "github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/session"
	"github.com/easel/fizeau/internal/skill"
	"github.com/easel/fizeau/internal/tool"
	"github.com/easel/fizeau/internal/tool/anchorstore"
)

// Skill discovery and the load_skill tool.

type SkillCatalog = skill.Catalog

// ScanSkillsDir walks dir for SKILL.md files and returns the discovered
// catalog. A non-existent directory returns an empty catalog with no
// error so callers can opt in to skill discovery without branching.
func ScanSkillsDir(dir string) (*SkillCatalog, []string, error) {
	return skill.ScanDir(dir)
}

// NewLoadSkillTool returns a Tool exposing the catalog as the
// `load_skill` tool. Returns nil when the catalog is nil or empty so
// callers can append unconditionally.
func NewLoadSkillTool(cat *SkillCatalog) Tool {
	if cat == nil || cat.Len() == 0 {
		return nil
	}
	return &skill.LoadSkillTool{Catalog: cat}
}

// Compaction.

type CompactionConfig = compaction.Config

func DefaultCompactionConfig() CompactionConfig { return compaction.DefaultConfig() }

// Built-in tool wiring.

type BashOutputFilterConfig = tool.BashOutputFilterConfig
type AnchorStore = anchorstore.AnchorStore
type ReadTool = tool.ReadTool

func BuiltinToolsForPreset(workDir, preset string, bashFilter BashOutputFilterConfig) []Tool {
	return tool.BuiltinToolsForPreset(workDir, preset, bashFilter)
}

func NewAnchorStore() *AnchorStore {
	return anchorstore.New()
}

func NewReadTool(workDir string, anchors *AnchorStore) Tool {
	return &tool.ReadTool{WorkDir: workDir, AnchorStore: anchors}
}

func NewAnchorEditTool(workDir string, anchors *AnchorStore) Tool {
	return &tool.AnchorEditTool{WorkDir: workDir, AnchorStore: anchors}
}

// OpenAI-shaped model discovery and ranking.

type ScoredModel = oaiProvider.ScoredModel

func DiscoverModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	return oaiProvider.DiscoverModels(ctx, baseURL, apiKey)
}

func RankModels(candidates []string, knownModels map[string]string, pattern string) ([]ScoredModel, error) {
	return oaiProvider.RankModels(candidates, knownModels, pattern)
}

func NormalizeModelID(requested string, catalog []string) (string, error) {
	return oaiProvider.NormalizeModelID(requested, catalog)
}

// Session log inspection.

type (
	SessionEvent     = agentcore.Event
	SessionEventType = agentcore.EventType
	SessionStatus    = agentcore.Status
	SessionEndData   = session.SessionEndData
	SessionStartData = session.SessionStartData
	TokenUsage       = agentcore.TokenUsage
)

const (
	EventSessionStart = agentcore.EventSessionStart
	EventSessionEnd   = agentcore.EventSessionEnd
	StatusSuccess     = agentcore.StatusSuccess
)

func ReadSessionEvents(path string) ([]SessionEvent, error) {
	return session.ReadEvents(path)
}

// SessionLogger writes session log events. CLI tests construct one to seed
// log fixtures that the running CLI later reads back.
type SessionLogger = session.Logger

func NewSessionLogger(dir, sessionID string) *SessionLogger {
	return session.NewLogger(dir, sessionID)
}

func NewSessionEvent(sessionID string, seq int, eventType SessionEventType, data any) SessionEvent {
	return session.NewEvent(sessionID, seq, eventType, data)
}
