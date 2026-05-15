// Package cassette records and replays versioned direct-PTY evidence.
package cassette

import (
	"context"
	"encoding/json"
	"time"

	"github.com/easel/fizeau/internal/pty/session"
	"github.com/easel/fizeau/internal/pty/terminal"
)

const (
	Version = 1

	ManifestFile      = "manifest.json"
	InputFile         = "input.jsonl"
	OutputRawFile     = "output.raw"
	OutputEventsFile  = "output.jsonl"
	FramesFile        = "frames.jsonl"
	ServiceEventsFile = "service-events.jsonl"
	FinalFile         = "final.json"
	QuotaFile         = "quota.json"
	DiscoveryFile     = "discovery.json"
	ScrubReportFile   = "scrub-report.json"
)

type ReplayMode string

const (
	ReplayRealtime  ReplayMode = "realtime"
	ReplayScaled    ReplayMode = "scaled"
	ReplayCollapsed ReplayMode = "collapsed"
)

type Manifest struct {
	Version          int            `json:"version"`
	ID               string         `json:"id"`
	ContentDigest    ContentDigest  `json:"content_digest"`
	Harness          Harness        `json:"harness"`
	Command          Command        `json:"command"`
	Terminal         Terminal       `json:"terminal"`
	Timing           Timing         `json:"timing"`
	Provenance       Provenance     `json:"provenance"`
	RequiredFeatures []string       `json:"required_features,omitempty"`
	Optional         map[string]any `json:"optional,omitempty"`
}

type ContentDigest struct {
	SHA256 string `json:"sha256"`
}

type Harness struct {
	Name                string         `json:"name"`
	BinaryPathDigestSHA string         `json:"binary_path_digest_sha256,omitempty"`
	BinaryVersion       string         `json:"binary_version,omitempty"`
	Capability          map[string]any `json:"capability,omitempty"`
	AccountClass        string         `json:"account_class,omitempty"`
	CapturedAt          string         `json:"captured_at,omitempty"`
	FreshnessWindow     string         `json:"freshness_window,omitempty"`
}

type Command struct {
	Argv           []string `json:"argv,omitempty"`
	WorkdirPolicy  string   `json:"workdir_policy"`
	EnvAllowlist   []string `json:"env_allowlist,omitempty"`
	TimeoutMS      int64    `json:"timeout_ms,omitempty"`
	PermissionMode string   `json:"permission_mode,omitempty"`
}

type Terminal struct {
	InitialRows int            `json:"initial_rows"`
	InitialCols int            `json:"initial_cols"`
	Locale      string         `json:"locale,omitempty"`
	Term        string         `json:"term,omitempty"`
	PTYMode     map[string]any `json:"pty_mode,omitempty"`
	Emulator    Emulator       `json:"emulator"`
}

type Emulator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Timing struct {
	ResolutionMS  int64      `json:"resolution_ms"`
	ClockPolicy   string     `json:"clock_policy"`
	ReplayDefault ReplayMode `json:"replay_default"`
}

type Provenance struct {
	GitSHA          string `json:"git_sha,omitempty"`
	ContractVersion string `json:"contract_version,omitempty"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	RecordedAt      string `json:"recorded_at"`
	RecorderVersion string `json:"recorder_version"`
}

type Timed struct {
	Seq uint64 `json:"seq"`
	TMS int64  `json:"t_ms"`
}

type InputRecord struct {
	Timed
	Kind   session.EventKind `json:"kind"`
	Bytes  []byte            `json:"bytes,omitempty"`
	Key    session.Key       `json:"key,omitempty"`
	Size   *session.Size     `json:"size,omitempty"`
	Signal string            `json:"signal,omitempty"`
}

type OutputRecord struct {
	Timed
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256,omitempty"`
}

type FrameRecord struct {
	Timed
	Size         terminal.Size     `json:"size"`
	Text         []string          `json:"text"`
	Cells        [][]terminal.Cell `json:"cells,omitempty"`
	Cursor       terminal.Cursor   `json:"cursor"`
	Title        string            `json:"title,omitempty"`
	RawStart     int64             `json:"raw_start"`
	RawEnd       int64             `json:"raw_end"`
	ParserOffset int64             `json:"parser_offset"`
}

type ServiceEventRecord struct {
	Timed
	Payload json.RawMessage `json:"payload"`
}

type FinalRecord struct {
	Timed
	Exit           *session.ExitStatus `json:"exit,omitempty"`
	DurationMS     int64               `json:"duration_ms"`
	Metadata       map[string]any      `json:"metadata,omitempty"`
	Usage          map[string]any      `json:"usage,omitempty"`
	CostUSD        float64             `json:"cost_usd,omitempty"`
	RoutingActual  string              `json:"routing_actual,omitempty"`
	SessionLogPath string              `json:"session_log_path,omitempty"`
	FinalText      string              `json:"final_text,omitempty"`
}

type QuotaRecord struct {
	Source            string           `json:"source"`
	Status            string           `json:"status"`
	CapturedAt        string           `json:"captured_at,omitempty"`
	FreshnessWindow   string           `json:"freshness_window,omitempty"`
	StalenessBehavior string           `json:"staleness_behavior,omitempty"`
	AccountClass      string           `json:"account_class,omitempty"`
	Windows           []map[string]any `json:"windows,omitempty"`
	Metadata          map[string]any   `json:"metadata,omitempty"`
}

// DiscoveryRecord stores harness capability evidence captured from a live
// PTY or a documented CLI surface and replayed by tests without credentials.
type DiscoveryRecord struct {
	Source            string         `json:"source"`
	Status            string         `json:"status"`
	Models            []string       `json:"models,omitempty"`
	ReasoningLevels   []string       `json:"reasoning_levels,omitempty"`
	CapturedAt        string         `json:"captured_at,omitempty"`
	FreshnessWindow   string         `json:"freshness_window,omitempty"`
	StalenessBehavior string         `json:"staleness_behavior,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type ScrubReport struct {
	Status                 string         `json:"status"`
	Rules                  []string       `json:"rules"`
	HitCounts              map[string]int `json:"hit_counts"`
	EnvAllowlist           []string       `json:"env_allowlist,omitempty"`
	IntentionallyPreserved []string       `json:"intentionally_preserved,omitempty"`
}

type EventKind string

const (
	EventInput        EventKind = "input"
	EventOutput       EventKind = "output"
	EventFrame        EventKind = "frame"
	EventServiceEvent EventKind = "service-event"
	EventFinal        EventKind = "final"
)

type Event struct {
	Seq     uint64
	TMS     int64
	Kind    EventKind
	Input   *InputRecord
	Output  *OutputRecord
	Frame   *FrameRecord
	Service *ServiceEventRecord
	Final   *FinalRecord
}

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}
