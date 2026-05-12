package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fizRunResult struct {
	stdout   string
	stderr   string
	exitCode int
}

var (
	buildFizOnce sync.Once
	buildFizPath string
	buildFizErr  error
)

func buildFiz(t *testing.T) string {
	t.Helper()
	buildFizOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fiz-models-test-*")
		if err != nil {
			buildFizErr = err
			return
		}
		buildFizPath = filepath.Join(dir, "fiz")
		cmd := exec.Command("go", "build", "-o", buildFizPath, ".")
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildFizErr = errWithOutput(err, out)
		}
	})
	if buildFizErr != nil {
		t.Fatalf("build fiz: %v", buildFizErr)
	}
	return buildFizPath
}

func errWithOutput(err error, out []byte) error {
	if len(out) == 0 {
		return err
	}
	return &commandError{err: err, out: string(out)}
}

type commandError struct {
	err error
	out string
}

func (e *commandError) Error() string {
	return e.err.Error() + "\n" + e.out
}

func runFiz(t *testing.T, fixture fizFixture, args ...string) fizRunResult {
	t.Helper()
	exe := buildFiz(t)
	cmd := exec.Command(exe, append([]string{"--work-dir", fixture.workDir}, args...)...)
	cmd.Dir = fixture.workDir
	cmd.Env = fixture.env
	out, err := cmd.Output()
	stderr := ""
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = string(exitErr.Stderr)
		} else {
			t.Fatalf("run fiz: %v", err)
		}
	}
	return fizRunResult{stdout: string(out), stderr: stderr, exitCode: exitCode}
}

type fizFixture struct {
	workDir   string
	homeDir   string
	cacheRoot string
	env       []string
}

func newFizFixture(t *testing.T) fizFixture {
	t.Helper()
	root := t.TempDir()
	fixture := fizFixture{
		workDir:   filepath.Join(root, "work"),
		homeDir:   filepath.Join(root, "home"),
		cacheRoot: filepath.Join(root, "cache"),
	}
	if err := os.MkdirAll(filepath.Join(fixture.workDir, ".fizeau"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixture.homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "models.yaml")
	if err := os.WriteFile(catalogPath, []byte(testModelCatalogYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `
model_catalog:
  manifest: ` + catalogPath + `
providers:
  alpha:
    type: openai
    billing: fixed
    include_by_default: true
  beta:
    type: openrouter
    billing: fixed
    include_by_default: true
`
	if err := os.WriteFile(filepath.Join(fixture.workDir, ".fizeau", "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	writeDiscoveryCache(t, fixture.cacheRoot, "alpha", []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.3",
		"gpt-5.4-mini",
		"tiny-noise",
		"unknown-zero",
	})
	writeDiscoveryCache(t, fixture.cacheRoot, "beta", []string{
		"claude-opus-4.5",
		"gpt-5.5",
	})
	writeRuntimeCache(t, fixture.cacheRoot, "alpha", "available", 9, 75*time.Millisecond)
	writeRuntimeCache(t, fixture.cacheRoot, "beta", "exhausted", 0, 120*time.Millisecond)
	fixture.env = testEnv(fixture.homeDir, fixture.cacheRoot)
	return fixture
}

func testEnv(home, cacheRoot string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"HOME="+home,
		"FIZEAU_CACHE_DIR="+cacheRoot,
		"PATH=",
	)
	return env
}

func writeDiscoveryCache(t *testing.T, cacheRoot, source string, models []string) {
	t.Helper()
	dir := filepath.Join(cacheRoot, "discovery")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"captured_at": time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"models":      models,
		"source":      "test-fixture",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, source+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeRuntimeCache(t *testing.T, cacheRoot, provider, status string, quotaRemaining int, latency time.Duration) {
	t.Helper()
	dir := filepath.Join(cacheRoot, "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"provider":              provider,
		"status":                status,
		"quota_remaining":       quotaRemaining,
		"recent_p50_latency_ns": latency,
		"recorded_at":           time.Date(2026, 5, 12, 10, 1, 0, 0, time.UTC).Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, provider+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestModelsListDefaultColumns(t *testing.T) {
	fixture := newFizFixture(t)
	res := runFiz(t, fixture, "models")
	if res.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.exitCode, res.stderr, res.stdout)
	}
	for _, column := range []string{"PROVIDER", "MODEL", "FAMILY", "VERSION", "TIER", "POWER", "COST/M", "STATUS", "CATALOG QUOTA", "RUNTIME QUOTA", "AUTO"} {
		if !strings.Contains(res.stdout, column) {
			t.Fatalf("missing column %q in:\n%s", column, res.stdout)
		}
	}
	for _, want := range []string{"alpha", "beta", "gpt-5.5", "claude-opus-4.5"} {
		if !strings.Contains(res.stdout, want) {
			t.Fatalf("missing fixture entry %q in:\n%s", want, res.stdout)
		}
	}
}

func TestModelsListQuotaColumns(t *testing.T) {
	fixture := newFizFixture(t)
	res := runFiz(t, fixture, "models")
	if res.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.exitCode, res.stderr, res.stdout)
	}
	var alphaLine string
	for _, line := range strings.Split(res.stdout, "\n") {
		if strings.Contains(line, "alpha") && strings.Contains(line, "gpt-5.5") {
			alphaLine = line
			break
		}
	}
	if alphaLine == "" {
		t.Fatalf("missing alpha/gpt-5.5 row in:\n%s", res.stdout)
	}
	if !strings.Contains(alphaLine, "openai-frontier") {
		t.Fatalf("catalog quota pool not rendered in row:\n%s", alphaLine)
	}
	if !strings.Contains(alphaLine, "9") {
		t.Fatalf("runtime quota remaining not rendered in row:\n%s", alphaLine)
	}
}

func TestModelsListJSONEmitsSnapshot(t *testing.T) {
	fixture := newFizFixture(t)
	res := runFiz(t, fixture, "models", "--json")
	if res.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.exitCode, res.stderr, res.stdout)
	}
	var snapshot struct {
		Models []struct {
			Provider string
			ID       string
		}
		Sources map[string]any
	}
	if err := json.Unmarshal([]byte(res.stdout), &snapshot); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, res.stdout)
	}
	if len(snapshot.Models) == 0 {
		t.Fatalf("snapshot has no models: %s", res.stdout)
	}
	if !snapshotContains(snapshot.Models, "alpha", "gpt-5.5") {
		t.Fatalf("snapshot missing alpha/gpt-5.5: %s", res.stdout)
	}
	if snapshot.Sources["alpha"] == nil || snapshot.Sources["beta"] == nil {
		t.Fatalf("snapshot missing source metadata: %s", res.stdout)
	}
}

func snapshotContains(models []struct {
	Provider string
	ID       string
}, provider, id string) bool {
	for _, model := range models {
		if model.Provider == provider && model.ID == id {
			return true
		}
	}
	return false
}

func TestModelsDetailCanonicalRef(t *testing.T) {
	fixture := newFizFixture(t)
	res := runFiz(t, fixture, "models", "alpha/gpt-5.5")
	if res.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.exitCode, res.stderr, res.stdout)
	}
	for _, want := range []string{"Canonical: alpha/gpt-5.5", "KnownModel:", "CatalogEntry:", "RawDiscoveryData:", "AutoRoutable: true"} {
		if !strings.Contains(res.stdout, want) {
			t.Fatalf("detail output missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestModelsDetailRuntimeFields(t *testing.T) {
	fixture := newFizFixture(t)
	res := runFiz(t, fixture, "models", "alpha/gpt-5.5")
	if res.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.exitCode, res.stderr, res.stdout)
	}
	for _, want := range []string{"RuntimeQuotaRemaining: 9", "RecentP50Latency: 75ms"} {
		if !strings.Contains(res.stdout, want) {
			t.Fatalf("detail output missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestModelsDetailShortformResolution(t *testing.T) {
	fixture := newFizFixture(t)

	unique := runFiz(t, fixture, "models", "gpt-5.4-mini")
	if unique.exitCode != 0 || !strings.Contains(unique.stdout, "Canonical: alpha/gpt-5.4-mini") {
		t.Fatalf("unique shortform failed: exit=%d stderr=%s stdout=%s", unique.exitCode, unique.stderr, unique.stdout)
	}

	ambiguous := runFiz(t, fixture, "models", "gpt")
	if ambiguous.exitCode != 1 {
		t.Fatalf("ambiguous shortform exit=%d, want 1 stdout=%s stderr=%s", ambiguous.exitCode, ambiguous.stdout, ambiguous.stderr)
	}
	for _, want := range []string{"ambiguous model ref", "alpha/gpt-5.5", "beta/gpt-5.5"} {
		if !strings.Contains(ambiguous.stderr, want) {
			t.Fatalf("ambiguous output missing %q:\n%s", want, ambiguous.stderr)
		}
	}

	missing := runFiz(t, fixture, "models", "gpt-6")
	if missing.exitCode != 1 {
		t.Fatalf("missing shortform exit=%d, want 1 stdout=%s stderr=%s", missing.exitCode, missing.stdout, missing.stderr)
	}
	for _, want := range []string{"no model matched", "Suggestions:", "alpha/gpt-5.5"} {
		if !strings.Contains(missing.stderr, want) {
			t.Fatalf("missing output missing %q:\n%s", want, missing.stderr)
		}
	}
}

func TestModelsListFiltersAndIncludeNoise(t *testing.T) {
	fixture := newFizFixture(t)

	defaultView := runFiz(t, fixture, "models")
	if defaultView.exitCode != 0 {
		t.Fatalf("default exit=%d stderr=%s", defaultView.exitCode, defaultView.stderr)
	}
	for _, hidden := range []string{"tiny-noise", "unknown-zero", "gpt-5.3"} {
		if strings.Contains(defaultView.stdout, hidden) {
			t.Fatalf("default view should suppress %q:\n%s", hidden, defaultView.stdout)
		}
	}

	noisy := runFiz(t, fixture, "models", "--include-noise")
	for _, shown := range []string{"tiny-noise", "unknown-zero", "gpt-5.3"} {
		if !strings.Contains(noisy.stdout, shown) {
			t.Fatalf("--include-noise missing %q:\n%s", shown, noisy.stdout)
		}
	}

	provider := runFiz(t, fixture, "models", "--provider", "beta")
	if strings.Contains(provider.stdout, "alpha") || !strings.Contains(provider.stdout, "beta") {
		t.Fatalf("--provider filter failed:\n%s", provider.stdout)
	}

	powerMin := runFiz(t, fixture, "models", "--power-min", "9")
	if !strings.Contains(powerMin.stdout, "gpt-5.5") || strings.Contains(powerMin.stdout, "gpt-5.4-mini") {
		t.Fatalf("--power-min filter failed:\n%s", powerMin.stdout)
	}

	powerMax := runFiz(t, fixture, "models", "--include-noise", "--power-max", "2")
	if !strings.Contains(powerMax.stdout, "tiny-noise") || strings.Contains(powerMax.stdout, "gpt-5.5") {
		t.Fatalf("--power-max filter failed:\n%s", powerMax.stdout)
	}
}

func TestCachePruneRemovesUnknownSources(t *testing.T) {
	fixture := newFizFixture(t)
	writeDiscoveryCache(t, fixture.cacheRoot, "stale-source", []string{"old-model"})
	writeRuntimeCache(t, fixture.cacheRoot, "stale-runtime", "available", 7, 44*time.Millisecond)

	res := runFiz(t, fixture, "cache", "prune")
	if res.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", res.exitCode, res.stderr, res.stdout)
	}
	for _, path := range []string{
		filepath.Join(fixture.cacheRoot, "discovery", "alpha.json"),
		filepath.Join(fixture.cacheRoot, "runtime", "beta.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("active source removed: %s: %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join(fixture.cacheRoot, "discovery", "stale-source.json"),
		filepath.Join(fixture.cacheRoot, "runtime", "stale-runtime.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale source still exists or stat failed: %s: %v", path, err)
		}
	}
}

func TestModelsRefreshFlags(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	cacheRoot := filepath.Join(root, "cache")
	if err := os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "models.yaml")
	if err := os.WriteFile(catalogPath, []byte(testModelCatalogYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5"}]}`))
	}))
	defer server.Close()
	config := `
model_catalog:
  manifest: ` + catalogPath + `
providers:
  alpha:
    type: openai
    base_url: ` + server.URL + `/v1
    billing: fixed
    include_by_default: true
`
	if err := os.WriteFile(filepath.Join(workDir, ".fizeau", "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := fizFixture{
		workDir:   workDir,
		homeDir:   filepath.Join(root, "home"),
		cacheRoot: cacheRoot,
		env:       testEnv(filepath.Join(root, "home"), cacheRoot),
	}
	noRefresh := runFiz(t, fixture, "models", "--no-refresh")
	if noRefresh.exitCode != 0 {
		t.Fatalf("--no-refresh exit=%d stderr=%s stdout=%s", noRefresh.exitCode, noRefresh.stderr, noRefresh.stdout)
	}
	if requests != 0 {
		t.Fatalf("--no-refresh triggered %d refresh requests", requests)
	}

	refresh := runFiz(t, fixture, "models", "--refresh")
	if refresh.exitCode != 0 {
		t.Fatalf("--refresh exit=%d stderr=%s stdout=%s", refresh.exitCode, refresh.stderr, refresh.stdout)
	}
	if requests == 0 {
		t.Fatal("--refresh did not synchronously call the discovery endpoint")
	}
	if !strings.Contains(refresh.stdout, "gpt-5.5") {
		t.Fatalf("--refresh did not render refreshed model:\n%s", refresh.stdout)
	}
}

func testModelCatalogYAML() string {
	return `
version: 5
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
providers:
  alpha:
    type: openai
    include_by_default: true
  beta:
    type: openrouter
    include_by_default: true
models:
  gpt-5.5:
    family: gpt
    status: active
    provider_system: openai
    quota_pool: openai-frontier
    power: 10
    cost_input_per_m: 1.25
    cost_output_per_m: 10.50
    context_window: 400000
    reasoning_levels: [low, medium, high]
  gpt-5.4:
    family: gpt
    status: active
    provider_system: openai
    quota_pool: openai-frontier
    power: 9
    cost_input_per_m: 1
    cost_output_per_m: 8
  gpt-5.3:
    family: gpt
    status: active
    provider_system: openai
    power: 8
    cost_input_per_m: 0.8
    cost_output_per_m: 6
  gpt-5.4-mini:
    family: gpt
    status: active
    provider_system: openai
    power: 6
    cost_input_per_m: 0.20
    cost_output_per_m: 0.80
  claude-opus-4.5:
    family: claude
    status: active
    provider_system: anthropic
    quota_pool: anthropic
    power: 10
    cost_input_per_m: 3
    cost_output_per_m: 15
  tiny-noise:
    family: tiny
    status: active
    provider_system: openai
    power: 2
    cost_input_per_m: 0.01
    cost_output_per_m: 0.02
`
}
