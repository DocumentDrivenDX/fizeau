// Package ptytest provides test-only PTY cassette scenario assertions.
package ptytest

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/pty/cassette"
	"gopkg.in/yaml.v3"
)

type ReplayMode = cassette.ReplayMode

const (
	ReplayRealtime  = cassette.ReplayRealtime
	ReplayScaled    = cassette.ReplayScaled
	ReplayCollapsed = cassette.ReplayCollapsed
)

type Scenario struct {
	Name                  string                     `json:"name" yaml:"name"`
	CassettePath          string                     `json:"cassette_path" yaml:"cassette_path"`
	ExpectedManifestID    string                     `json:"expected_manifest_id" yaml:"expected_manifest_id"`
	ExpectedContentDigest string                     `json:"expected_content_digest" yaml:"expected_content_digest"`
	RequiredEmulator      *cassette.Emulator         `json:"required_emulator,omitempty" yaml:"required_emulator,omitempty"`
	ReplayMode            ReplayMode                 `json:"replay_mode" yaml:"replay_mode"`
	ReplayScale           float64                    `json:"replay_scale,omitempty" yaml:"replay_scale,omitempty"`
	TerminalSize          *TerminalSize              `json:"terminal_size,omitempty" yaml:"terminal_size,omitempty"`
	Environment           EnvironmentPolicy          `json:"environment,omitempty" yaml:"environment,omitempty"`
	ExpectedArtifacts     []string                   `json:"expected_artifacts,omitempty" yaml:"expected_artifacts,omitempty"`
	ResolutionMS          int64                      `json:"resolution_ms,omitempty" yaml:"resolution_ms,omitempty"`
	Groups                map[string][]AssertionSpec `json:"groups" yaml:"groups"`
}

type TerminalSize struct {
	Rows int `json:"rows" yaml:"rows"`
	Cols int `json:"cols" yaml:"cols"`
}

type EnvironmentPolicy struct {
	Home       string            `json:"home,omitempty" yaml:"home,omitempty"`
	ConfigRoot string            `json:"config_root,omitempty" yaml:"config_root,omitempty"`
	Allowlist  []string          `json:"allowlist,omitempty" yaml:"allowlist,omitempty"`
	Variables  map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
}

type AssertionSpec struct {
	Name          string  `json:"name,omitempty" yaml:"name,omitempty"`
	Type          string  `json:"type" yaml:"type"`
	AtMS          *int64  `json:"at_ms,omitempty" yaml:"at_ms,omitempty"`
	WithinMS      int64   `json:"within_ms,omitempty" yaml:"within_ms,omitempty"`
	StableForMS   int64   `json:"stable_for_ms,omitempty" yaml:"stable_for_ms,omitempty"`
	Text          string  `json:"text,omitempty" yaml:"text,omitempty"`
	BytesHex      string  `json:"bytes_hex,omitempty" yaml:"bytes_hex,omitempty"`
	Key           string  `json:"key,omitempty" yaml:"key,omitempty"`
	Rows          int     `json:"rows,omitempty" yaml:"rows,omitempty"`
	Cols          int     `json:"cols,omitempty" yaml:"cols,omitempty"`
	CursorRow     int     `json:"cursor_row,omitempty" yaml:"cursor_row,omitempty"`
	CursorCol     int     `json:"cursor_col,omitempty" yaml:"cursor_col,omitempty"`
	CursorVisible *bool   `json:"cursor_visible,omitempty" yaml:"cursor_visible,omitempty"`
	FG            *uint32 `json:"fg,omitempty" yaml:"fg,omitempty"`
	BG            *uint32 `json:"bg,omitempty" yaml:"bg,omitempty"`
	JSONPath      string  `json:"json_path,omitempty" yaml:"json_path,omitempty"`
	Equals        any     `json:"equals,omitempty" yaml:"equals,omitempty"`
	MetadataKey   string  `json:"metadata_key,omitempty" yaml:"metadata_key,omitempty"`
	ExitCode      *int    `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
	Signaled      *bool   `json:"signaled,omitempty" yaml:"signaled,omitempty"`
	Kind          string  `json:"kind,omitempty" yaml:"kind,omitempty"`
	BeforeKind    string  `json:"before_kind,omitempty" yaml:"before_kind,omitempty"`
	AfterKind     string  `json:"after_kind,omitempty" yaml:"after_kind,omitempty"`
	MinMS         *int64  `json:"min_ms,omitempty" yaml:"min_ms,omitempty"`
	MaxMS         *int64  `json:"max_ms,omitempty" yaml:"max_ms,omitempty"`
}

type Result struct {
	Scenario    Scenario
	Manifest    cassette.Manifest
	Events      []cassette.Event
	Failures    []Failure
	Clock       *VirtualClock
	Artifacts   map[string]string
	Environment map[string]string
}

type Failure struct {
	Group          string
	Assertion      AssertionSpec
	Message        string
	NearestTMS     int64
	ScreenExcerpt  string
	ServiceContext string
}

func (f Failure) Error() string {
	name := f.Assertion.Name
	if name == "" {
		name = f.Assertion.Type
	}
	return fmt.Sprintf("%s/%s: %s near t_ms=%d\nscreen:\n%s\nservice:\n%s", f.Group, name, f.Message, f.NearestTMS, f.ScreenExcerpt, f.ServiceContext)
}

func LoadScenario(path string) (Scenario, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- test scenarios are caller-selected fixtures.
	if err != nil {
		return Scenario{}, err
	}
	var scenario Scenario
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &scenario)
	default:
		err = json.Unmarshal(data, &scenario)
	}
	if err != nil {
		return Scenario{}, err
	}
	if scenario.CassettePath != "" && !filepath.IsAbs(scenario.CassettePath) {
		scenario.CassettePath = filepath.Join(filepath.Dir(path), scenario.CassettePath)
	}
	return scenario, nil
}

func RunScenario(ctx context.Context, scenario Scenario) (Result, error) {
	if scenario.CassettePath == "" {
		return Result{}, fmt.Errorf("ptytest: cassette_path is required")
	}
	openOpts := []cassette.OpenOption{}
	if scenario.ExpectedManifestID != "" || scenario.ExpectedContentDigest != "" {
		openOpts = append(openOpts, cassette.WithExpectedBinding(scenario.ExpectedManifestID, scenario.ExpectedContentDigest))
	}
	if scenario.RequiredEmulator != nil {
		openOpts = append(openOpts, cassette.WithRequiredEmulator(*scenario.RequiredEmulator))
	}
	reader, err := cassette.Open(scenario.CassettePath, openOpts...)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Scenario:  scenario,
		Manifest:  reader.Manifest(),
		Clock:     NewVirtualClock(),
		Artifacts: map[string]string{},
	}
	env, err := prepareEnvironment(scenario.Environment)
	if err != nil {
		return result, err
	}
	result.Environment = env
	if err := checkScenarioMetadata(reader, scenario); err != nil {
		return result, err
	}
	for _, name := range scenario.ExpectedArtifacts {
		path := filepath.Join(scenario.CassettePath, name)
		info, err := os.Stat(path)
		if err != nil {
			return result, fmt.Errorf("ptytest: expected artifact %s: %w", name, err)
		}
		result.Artifacts[name] = fmt.Sprintf("%d", info.Size())
	}
	sleeper := &virtualSleeper{clock: result.Clock}
	mode := scenario.ReplayMode
	if mode == "" {
		mode = cassette.ReplayCollapsed
	}
	err = reader.Replay(ctx, cassette.ReplayOptions{Mode: mode, Scale: scenario.ReplayScale, Sleeper: sleeper}, func(ev cassette.Event) error {
		result.Clock.Set(ev.TMS)
		result.Events = append(result.Events, ev)
		return nil
	})
	if err != nil {
		return result, err
	}
	for group, assertions := range scenario.Groups {
		for _, assertion := range assertions {
			if failure := evaluateAssertion(group, assertion, result.Events); failure != nil {
				result.Failures = append(result.Failures, *failure)
			}
		}
	}
	if len(result.Failures) > 0 {
		return result, result.Failures[0]
	}
	return result, nil
}

func checkScenarioMetadata(reader *cassette.Reader, scenario Scenario) error {
	manifest := reader.Manifest()
	if scenario.TerminalSize != nil {
		if manifest.Terminal.InitialRows != scenario.TerminalSize.Rows || manifest.Terminal.InitialCols != scenario.TerminalSize.Cols {
			return fmt.Errorf("ptytest: terminal size mismatch got=%dx%d want=%dx%d", manifest.Terminal.InitialRows, manifest.Terminal.InitialCols, scenario.TerminalSize.Rows, scenario.TerminalSize.Cols)
		}
	}
	if scenario.ResolutionMS > 0 && manifest.Timing.ResolutionMS != scenario.ResolutionMS {
		return fmt.Errorf("ptytest: resolution mismatch got=%d want=%d", manifest.Timing.ResolutionMS, scenario.ResolutionMS)
	}
	return nil
}

func prepareEnvironment(policy EnvironmentPolicy) (map[string]string, error) {
	env := map[string]string{}
	if policy.Home != "" {
		if err := os.MkdirAll(policy.Home, 0o700); err != nil {
			return nil, fmt.Errorf("ptytest: create HOME: %w", err)
		}
		env["HOME"] = policy.Home
	}
	if policy.ConfigRoot != "" {
		if err := os.MkdirAll(policy.ConfigRoot, 0o700); err != nil {
			return nil, fmt.Errorf("ptytest: create config root: %w", err)
		}
		env["XDG_CONFIG_HOME"] = policy.ConfigRoot
	}
	allow := map[string]bool{}
	for _, key := range policy.Allowlist {
		allow[key] = true
	}
	for key, value := range policy.Variables {
		if allow[key] {
			env[key] = value
		}
	}
	return env, nil
}

type VirtualClock struct {
	mu sync.Mutex
	ms int64
}

func NewVirtualClock() *VirtualClock { return &VirtualClock{} }

func (v *VirtualClock) Set(ms int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ms > v.ms {
		v.ms = ms
	}
}

func (v *VirtualClock) Advance(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ms += d.Milliseconds()
}

func (v *VirtualClock) NowMS() int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.ms
}

type virtualSleeper struct {
	clock *VirtualClock
}

func (v *virtualSleeper) Sleep(ctx context.Context, d time.Duration) error {
	v.clock.Advance(d)
	return ctx.Err()
}

func decodeHex(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return hex.DecodeString(s)
}

func assertEqual(got, want any) bool {
	if want == nil {
		return got == nil
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	return string(gotJSON) == string(wantJSON)
}

func matchJSONPath(payload json.RawMessage, path string) (any, bool) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, false
	}
	if path == "" || path == "$" {
		return value, true
	}
	path = strings.TrimPrefix(path, "$.")
	cur := value
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func noEventsError() error {
	return errors.New("no replay events captured")
}
