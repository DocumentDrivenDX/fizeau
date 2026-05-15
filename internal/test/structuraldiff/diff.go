package structuraldiff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

var defaultTimeFieldNames = []string{
	"CapturedAt",
	"captured_at",
	"LastSuccessAt",
	"last_success_at",
	"Timestamp",
	"timestamp",
	"SnapshotCapturedAt",
	"snapshot_captured_at",
}

// Config controls structural JSON comparison for pinned fixtures.
type Config struct {
	// AdditivePaths are full dotted paths that may appear only in got.
	AdditivePaths []string
	// PresenceOnlyPaths are full dotted paths whose values are opaque:
	// both sides must carry the field, but their contents are not compared.
	PresenceOnlyPaths []string
	// TimeFieldNames are leaf field names compared under the "valid
	// RFC3339, present iff source was present" rule. When empty, the
	// default time-field set is used.
	TimeFieldNames []string
}

type normalizedConfig struct {
	additivePaths  map[string]struct{}
	presenceOnly   map[string]struct{}
	timeFieldNames map[string]struct{}
}

// CompareJSON unmarshals want and got, then compares them structurally.
func CompareJSON(want, got []byte, cfg Config) error {
	var wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		return fmt.Errorf("unmarshal want: %w", err)
	}
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		return fmt.Errorf("unmarshal got: %w", err)
	}
	return compareNode(nil, wantValue, gotValue, normalizeConfig(cfg))
}

func normalizeConfig(cfg Config) normalizedConfig {
	out := normalizedConfig{
		additivePaths:  make(map[string]struct{}, len(cfg.AdditivePaths)),
		presenceOnly:   make(map[string]struct{}, len(cfg.PresenceOnlyPaths)),
		timeFieldNames: make(map[string]struct{}),
	}
	for _, path := range cfg.AdditivePaths {
		out.additivePaths[normalizePath(path)] = struct{}{}
	}
	for _, path := range cfg.PresenceOnlyPaths {
		out.presenceOnly[normalizePath(path)] = struct{}{}
	}
	names := cfg.TimeFieldNames
	if len(names) == 0 {
		names = defaultTimeFieldNames
	}
	for _, name := range names {
		out.timeFieldNames[name] = struct{}{}
	}
	return out
}

func compareNode(path []string, want, got any, cfg normalizedConfig) error {
	if cfg.isPresenceOnly(pathString(path)) {
		return nil
	}
	if cfg.isTimeField(lastPathSegment(path)) {
		return compareTimeValue(pathString(path), want, got)
	}

	switch wantMap := want.(type) {
	case map[string]any:
		gotMap, ok := got.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: type mismatch: want object, got %T", pathString(path), got)
		}
		return compareMap(path, wantMap, gotMap, cfg)
	case []any:
		gotSlice, ok := got.([]any)
		if !ok {
			return fmt.Errorf("%s: type mismatch: want array, got %T", pathString(path), got)
		}
		if len(wantMap) != len(gotSlice) {
			return fmt.Errorf("%s: array length mismatch: want %d, got %d", pathString(path), len(wantMap), len(gotSlice))
		}
		for i := range wantMap {
			childPath := appendPath(path, fmt.Sprintf("[%d]", i))
			if err := compareNode(childPath, wantMap[i], gotSlice[i], cfg); err != nil {
				return err
			}
		}
		return nil
	default:
		if !reflect.DeepEqual(want, got) {
			return fmt.Errorf("%s: value mismatch: want %#v, got %#v", pathString(path), want, got)
		}
		return nil
	}
}

func compareMap(path []string, want, got map[string]any, cfg normalizedConfig) error {
	keys := make(map[string]struct{}, len(want)+len(got))
	for key := range want {
		keys[key] = struct{}{}
	}
	for key := range got {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	for _, key := range ordered {
		childPath := appendPath(path, key)
		childKey := pathString(childPath)
		wantValue, wantOK := want[key]
		gotValue, gotOK := got[key]
		switch {
		case !wantOK && gotOK:
			if cfg.isAdditive(childKey) {
				continue
			}
			return fmt.Errorf("%s: unexpected field", childKey)
		case wantOK && !gotOK:
			return fmt.Errorf("%s: missing field", childKey)
		case cfg.isPresenceOnly(childKey):
			continue
		default:
			if err := compareNode(childPath, wantValue, gotValue, cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func compareTimeValue(path string, want, got any) error {
	if want == nil || got == nil {
		if want == got {
			return nil
		}
		return fmt.Errorf("%s: time presence mismatch: want %#v, got %#v", path, want, got)
	}
	wantString, ok := want.(string)
	if !ok {
		return fmt.Errorf("%s: want time field must be a string, got %T", path, want)
	}
	gotString, ok := got.(string)
	if !ok {
		return fmt.Errorf("%s: got time field must be a string, got %T", path, got)
	}
	if _, err := parseRFC3339(wantString); err != nil {
		return fmt.Errorf("%s: want time field is not valid RFC3339: %q", path, wantString)
	}
	if _, err := parseRFC3339(gotString); err != nil {
		return fmt.Errorf("%s: got time field is not valid RFC3339: %q", path, gotString)
	}
	return nil
}

func parseRFC3339(value string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, nil
	}
	return time.Parse(time.RFC3339, value)
}

func appendPath(path []string, segment string) []string {
	out := make([]string, 0, len(path)+1)
	out = append(out, path...)
	out = append(out, segment)
	return out
}

func pathString(path []string) string {
	if len(path) == 0 {
		return "<root>"
	}
	var b strings.Builder
	for i, segment := range path {
		if strings.HasPrefix(segment, "[") {
			b.WriteString(segment)
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(segment)
	}
	return b.String()
}

func lastPathSegment(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

func normalizePath(path string) string {
	return strings.TrimSpace(strings.TrimPrefix(path, "."))
}

func (c normalizedConfig) isAdditive(path string) bool {
	_, ok := c.additivePaths[path]
	return ok
}

func (c normalizedConfig) isPresenceOnly(path string) bool {
	_, ok := c.presenceOnly[path]
	return ok
}

func (c normalizedConfig) isTimeField(name string) bool {
	_, ok := c.timeFieldNames[name]
	return ok
}
