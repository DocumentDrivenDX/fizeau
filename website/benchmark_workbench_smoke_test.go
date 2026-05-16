package website

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const terminalBenchTaskBase = "https://www.tbench.ai/registry/terminal-bench-core/head/"

type benchmarkDataManifest struct {
	Artifacts []struct {
		Format string `json:"format"`
		Kind   string `json:"kind"`
		Rows   int    `json:"rows"`
	} `json:"artifacts"`
}

type workbenchReadyState struct {
	Failed bool   `json:"failed"`
	Ready  bool   `json:"ready"`
	Rows   string `json:"rows"`
	Status string `json:"status"`
}

type workbenchSnapshot struct {
	ActiveFilters           []string `json:"activeFilters"`
	CombinationRowCount     int      `json:"combinationRowCount"`
	CombinationTaskSort     string   `json:"combinationTaskSort"`
	CombinationTaskLinkHref string   `json:"combinationTaskLinkHref"`
	CompareA                string   `json:"compareA"`
	CompareB                string   `json:"compareB"`
	ComparisonGapText       string   `json:"comparisonGapText"`
	ComparisonGapSort       string   `json:"comparisonGapSort"`
	ComparisonRowCount      int      `json:"comparisonRowCount"`
	ComparisonTaskLinkHref  string   `json:"comparisonTaskLinkHref"`
	CurrentRoute            string   `json:"currentRoute"`
	LocationHash            string   `json:"locationHash"`
	LegacyLaneFilterPresent bool     `json:"legacyLaneFilterPresent"`
	ModelOptionCount        int      `json:"modelOptionCount"`
	ModelOptions            []string `json:"modelOptions"`
	ProviderOptionCount     int      `json:"providerOptionCount"`
	ProviderOptions         []string `json:"providerOptions"`
	ResultOptionCount       int      `json:"resultOptionCount"`
	Rows                    string   `json:"rows"`
	SummaryChartCount       int      `json:"summaryChartCount"`
	SummaryRows             string   `json:"summaryRows"`
	SettingsColumnCount     int      `json:"settingsColumnCount"`
	SettingsOpen            bool     `json:"settingsOpen"`
	Status                  string   `json:"status"`
	TaskOptionCount         int      `json:"taskOptionCount"`
	VisibleColumns          []string `json:"visibleColumns"`
}

func TestBenchmarkWorkbenchSmoke(t *testing.T) {
	buildDir := t.TempDir()
	listener, baseURL := reserveListener(t)
	buildWebsiteWithBaseURL(t, buildDir, baseURL)
	expectedRows := readWorkbenchRowCount(t, buildDir)
	serveStaticDir(t, buildDir, listener)

	chromePath := ensureChromium(t)
	browserCtx := newBrowserContext(t, chromePath)

	workbenchURL := baseURL + "benchmarks/explorer/"
	waitForWorkbenchReady(t, browserCtx, workbenchURL)

	snapshot := captureWorkbenchSnapshot(t, browserCtx)
	if got := parseCount(t, snapshot.Rows); got != expectedRows {
		t.Fatalf("unexpected initial row count: got %d, want %d", got, expectedRows)
	}
	if snapshot.CurrentRoute != "summary" || snapshot.LocationHash != "#summary" {
		t.Fatalf("expected default summary route, got route=%q hash=%q", snapshot.CurrentRoute, snapshot.LocationHash)
	}
	if got := parseCount(t, snapshot.SummaryRows); got != expectedRows {
		t.Fatalf("unexpected summary row count: got %d, want %d", got, expectedRows)
	}
	if snapshot.SummaryChartCount == 0 {
		t.Fatal("expected summary charts to render")
	}
	if snapshot.Status == "" || !strings.Contains(snapshot.Status, "rows loaded") {
		t.Fatalf("expected loaded status, got %q", snapshot.Status)
	}
	if snapshot.ResultOptionCount < 4 {
		t.Fatalf("expected result-state options to populate, got %d", snapshot.ResultOptionCount)
	}
	if snapshot.TaskOptionCount < 2 {
		t.Fatalf("expected task picker options to populate, got %d", snapshot.TaskOptionCount)
	}
	if snapshot.ModelOptionCount < 2 || len(snapshot.ModelOptions) == 0 {
		t.Fatalf("expected model picker options to populate, got %d (%v)", snapshot.ModelOptionCount, snapshot.ModelOptions)
	}
	if snapshot.ProviderOptionCount < 2 || len(snapshot.ProviderOptions) == 0 {
		t.Fatalf("expected provider enum filter options to populate, got %d (%v)", snapshot.ProviderOptionCount, snapshot.ProviderOptions)
	}
	if snapshot.ComparisonRowCount == 0 {
		t.Fatal("expected pairwise comparison rows to render")
	}
	if snapshot.ComparisonGapText == "" || snapshot.ComparisonGapText == "-" {
		t.Fatalf("expected a rendered pairwise gap cell, got %q", snapshot.ComparisonGapText)
	}
	if snapshot.CompareA == "" || snapshot.CompareB == "" || snapshot.CompareA == snapshot.CompareB {
		t.Fatalf("expected distinct comparison families, got %q vs %q", snapshot.CompareA, snapshot.CompareB)
	}
	if snapshot.CombinationRowCount == 0 {
		t.Fatal("expected task combination aggregate rows to render")
	}
	if snapshot.CombinationTaskLinkHref == "" || !strings.HasPrefix(snapshot.CombinationTaskLinkHref, terminalBenchTaskBase) {
		t.Fatalf("expected aggregate task links to point at Terminal-Bench, got %q", snapshot.CombinationTaskLinkHref)
	}

	click(t, browserCtx, "[data-bw-route=\"data\"]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CurrentRoute == "data" && strings.HasPrefix(current.LocationHash, "#data") && len(current.VisibleColumns) > 0
	})
	snapshot = captureWorkbenchSnapshot(t, browserCtx)
	if len(snapshot.VisibleColumns) == 0 {
		t.Fatal("expected perspective viewer to expose visible columns")
	}
	expectedDefaultColumns := []string{
		"task",
		"task_category",
		"task_difficulty",
		"result_state",
		"model_display_name",
		"provider_type",
		"harness_label",
		"quant_display",
		"engine",
		"effective_gpu_model",
		"turns",
		"total_tokens",
		"cost_usd",
		"wall_seconds",
		"profile_ttft_p50_s",
		"profile_decode_tps_p50",
		"profile_timing_turns",
		"finished_at",
	}
	for _, column := range expectedDefaultColumns {
		if !contains(snapshot.VisibleColumns, column) {
			t.Fatalf("expected %s to be visible by default, got %v", column, snapshot.VisibleColumns)
		}
	}
	if len(snapshot.VisibleColumns) > len(expectedDefaultColumns) {
		t.Fatalf("expected focused default columns, got %d columns: %v", len(snapshot.VisibleColumns), snapshot.VisibleColumns)
	}
	if contains(snapshot.VisibleColumns, "terminalbench_task_url") {
		t.Fatalf("terminalbench_task_url must stay hidden by default, got %v", snapshot.VisibleColumns)
	}
	for _, column := range []string{"suite", "final_status", "invalid_class", "k_quant", "v_quant"} {
		if contains(snapshot.VisibleColumns, column) {
			t.Fatalf("%s must stay hidden by default, got %v", column, snapshot.VisibleColumns)
		}
	}
	if snapshot.LegacyLaneFilterPresent {
		t.Fatal("raw database filter UI must use profile terminology, not lane terminology")
	}

	click(t, browserCtx, "[data-bw-open-config]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.SettingsOpen && current.SettingsColumnCount > 0
	})
	clickViewerShadow(t, browserCtx, "#active-columns .column-selector-column span.is_column_active")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.SettingsOpen && len(current.VisibleColumns) == len(expectedDefaultColumns)-1
	})
	click(t, browserCtx, "[data-bw-open-config]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return !current.SettingsOpen
	})

	customColumns := []string{"task", "result_state"}
	setViewerColumns(t, browserCtx, customColumns)
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return sameStrings(current.VisibleColumns, customColumns)
	})

	setSelectValue(t, browserCtx, "[data-bw-model]", snapshot.ModelOptions[0])
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return parseCount(t, current.Rows) < expectedRows && sameStrings(current.VisibleColumns, customColumns)
	})

	click(t, browserCtx, "[data-bw-reset-view]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return contains(current.VisibleColumns, "profile_ttft_p50_s") && len(current.VisibleColumns) == len(expectedDefaultColumns)
	})
	click(t, browserCtx, "[data-bw-clear-filters]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return parseCount(t, current.Rows) == expectedRows && !hasFilterPrefix(current.ActiveFilters, "Model:")
	})

	setSelectValue(t, browserCtx, "[data-bw-compare-dimension]", "task")
	click(t, browserCtx, "[data-bw-route=\"compare\"]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CurrentRoute == "compare" &&
			strings.HasPrefix(current.LocationHash, "#compare") &&
			strings.Contains(current.LocationHash, "dim=task")
	})
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.ComparisonTaskLinkHref != ""
	})
	snapshot = captureWorkbenchSnapshot(t, browserCtx)
	if !strings.HasPrefix(snapshot.ComparisonTaskLinkHref, terminalBenchTaskBase) {
		t.Fatalf("expected pairwise task links to point at Terminal-Bench, got %q", snapshot.ComparisonTaskLinkHref)
	}
	click(t, browserCtx, "[data-bw-comparison] th:nth-child(4) button")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.ComparisonGapSort == "ascending" &&
			strings.Contains(current.LocationHash, "sort=gap_pp%3Aasc")
	})
	comparisonBookmark := captureWorkbenchSnapshot(t, browserCtx).LocationHash

	click(t, browserCtx, "[data-bw-route=\"combinations\"]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CurrentRoute == "combinations" && strings.HasPrefix(current.LocationHash, "#combinations")
	})
	click(t, browserCtx, "[data-bw-combinations] th:first-child button")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CombinationTaskSort == "ascending" &&
			strings.Contains(current.LocationHash, "sort=task%3Aasc")
	})

	click(t, browserCtx, "[data-bw-route=\"data\"]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CurrentRoute == "data"
	})
	setSelectValue(t, browserCtx, "[data-bw-model]", snapshot.ModelOptions[0])
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return parseCount(t, current.Rows) < expectedRows && hasFilterPrefix(current.ActiveFilters, "Model:")
	})

	click(t, browserCtx, "[data-bw-clear-filters]")
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return parseCount(t, current.Rows) == expectedRows && !hasFilterPrefix(current.ActiveFilters, "Model:")
	})

	setSelectValue(t, browserCtx, "[data-bw-filter-field=\"provider_type\"]", snapshot.ProviderOptions[0])
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return parseCount(t, current.Rows) < expectedRows &&
			hasFilterPrefix(current.ActiveFilters, "Provider:") &&
			strings.Contains(current.LocationHash, "f.provider_type=")
	})
	rawBookmark := captureWorkbenchSnapshot(t, browserCtx).LocationHash

	waitForWorkbenchReady(t, browserCtx, workbenchURL+comparisonBookmark)
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CurrentRoute == "compare" &&
			current.ComparisonGapSort == "ascending" &&
			current.ComparisonTaskLinkHref != "" &&
			strings.Contains(current.LocationHash, "dim=task")
	})

	waitForWorkbenchReady(t, browserCtx, workbenchURL+rawBookmark)
	waitForCondition(t, browserCtx, 30*time.Second, func(current workbenchSnapshot) bool {
		return current.CurrentRoute == "data" &&
			parseCount(t, current.Rows) < expectedRows &&
			hasFilterPrefix(current.ActiveFilters, "Provider:")
	})
}

func reserveListener(t *testing.T) (net.Listener, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for static site: %v", err)
	}
	return listener, "http://" + listener.Addr().String() + "/"
}

func serveStaticDir(t *testing.T, dir string, listener net.Listener) {
	t.Helper()

	server := &http.Server{Handler: http.FileServer(http.Dir(dir))}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
}

func buildWebsiteWithBaseURL(t *testing.T, dest, baseURL string) {
	t.Helper()

	hugoPath, err := exec.LookPath("hugo")
	if err != nil {
		t.Fatalf("find hugo: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, hugoPath,
		"--quiet",
		"--baseURL", baseURL,
		"--destination", dest,
	)
	cmd.Dir = websiteRoot(t)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build website: %v\n%s", err, output)
	}
}

func readWorkbenchRowCount(t *testing.T, buildDir string) int {
	t.Helper()

	path := filepath.Join(buildDir, "data", "benchmark-data-manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read benchmark manifest: %v", err)
	}

	var manifest benchmarkDataManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode benchmark manifest: %v", err)
	}

	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == "cell_rows" && artifact.Format == "parquet" {
			return artifact.Rows
		}
	}

	t.Fatalf("find parquet cell_rows artifact in %s", path)
	return 0
}

func newBrowserContext(t *testing.T, chromePath string) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(chromePath),
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
		)...,
	)
	t.Cleanup(allocCancel)

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	t.Cleanup(browserCancel)
	return browserCtx
}

func waitForWorkbenchReady(t *testing.T, ctx context.Context, pageURL string) {
	t.Helper()

	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1440, 1400, chromedp.EmulateScale(1)),
		chromedp.Navigate(pageURL),
	); err != nil {
		t.Fatalf("open benchmark workbench: %v", err)
	}

	deadline := time.Now().Add(90 * time.Second)
	var last workbenchReadyState
	for time.Now().Before(deadline) {
		if err := chromedp.Run(ctx, chromedp.Evaluate(workbenchReadyJS, &last)); err != nil {
			t.Fatalf("read workbench readiness: %v", err)
		}
		if last.Failed {
			t.Fatalf("benchmark workbench failed to load: %s", last.Status)
		}
		if last.Ready {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for benchmark workbench: status=%q rows=%q", last.Status, last.Rows)
}

func captureWorkbenchSnapshot(t *testing.T, ctx context.Context) workbenchSnapshot {
	t.Helper()

	var snapshot workbenchSnapshot
	if err := chromedp.Run(ctx, chromedp.Evaluate(workbenchSnapshotJS, &snapshot, awaitPromise)); err != nil {
		t.Fatalf("capture workbench snapshot: %v", err)
	}
	return snapshot
}

func waitForCondition(t *testing.T, ctx context.Context, timeout time.Duration, predicate func(workbenchSnapshot) bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last workbenchSnapshot
	for time.Now().Before(deadline) {
		last = captureWorkbenchSnapshot(t, ctx)
		if predicate(last) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for workbench condition: rows=%q filters=%v status=%q", last.Rows, last.ActiveFilters, last.Status)
}

func setSelectValue(t *testing.T, ctx context.Context, selector, value string) {
	t.Helper()

	script := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) {
			throw new Error("missing element: " + %q);
		}
		el.value = %q;
		el.dispatchEvent(new Event('change', { bubbles: true }));
		return el.value;
	})()`, selector, selector, value)

	var applied string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &applied)); err != nil {
		t.Fatalf("set %s to %q: %v", selector, value, err)
	}
	if applied != value {
		t.Fatalf("set %s to %q, got %q", selector, value, applied)
	}
}

func click(t *testing.T, ctx context.Context, selector string) {
	t.Helper()

	script := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) {
			throw new Error("missing element: " + %q);
		}
		el.click();
		return true;
	})()`, selector, selector)

	if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
		t.Fatalf("click %s: %v", selector, err)
	}
}

func clickViewerShadow(t *testing.T, ctx context.Context, selector string) {
	t.Helper()

	script := fmt.Sprintf(`(() => {
		const viewer = document.querySelector('[data-bw-viewer]');
		const el = viewer?.shadowRoot?.querySelector(%q);
		if (!el) {
			throw new Error("missing viewer shadow element: " + %q);
		}
		el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
		el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
		el.click();
		return true;
	})()`, selector, selector)

	if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
		t.Fatalf("click viewer shadow %s: %v", selector, err)
	}
}

func setViewerColumns(t *testing.T, ctx context.Context, columns []string) {
	t.Helper()

	raw, err := json.Marshal(columns)
	if err != nil {
		t.Fatalf("marshal columns: %v", err)
	}
	script := fmt.Sprintf(`(async () => {
		const viewer = document.querySelector('[data-bw-viewer]');
		if (!viewer) throw new Error('missing viewer');
		const config = await viewer.save();
		await viewer.restore({
			...config,
			columns: %s,
			sort: [],
			group_by: [],
			split_by: [],
			filter: [],
			settings: false,
		});
		viewer.dispatchEvent(new CustomEvent('perspective-config-update', { bubbles: true }));
		await viewer.flush?.();
		return (await viewer.save()).columns;
	})()`, string(raw))

	var applied []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &applied, awaitPromise)); err != nil {
		t.Fatalf("set viewer columns to %v: %v", columns, err)
	}
	if !sameStrings(applied, columns) {
		t.Fatalf("set viewer columns to %v, got %v", columns, applied)
	}
}

func parseCount(t *testing.T, value string) int {
	t.Helper()

	clean := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	n, err := strconv.Atoi(clean)
	if err != nil {
		t.Fatalf("parse count %q: %v", value, err)
	}
	return n
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func hasFilterPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func awaitPromise(params *runtime.EvaluateParams) *runtime.EvaluateParams {
	return params.WithAwaitPromise(true)
}

const workbenchReadyJS = `(() => {
  const root = document.querySelector('[data-benchmark-workbench]');
  const status = root?.querySelector('[data-bw-status]')?.textContent?.trim() ?? '';
  const rows = root?.querySelector('[data-bw-metric="rows"]')?.textContent?.trim() ?? '';
  return {
    ready: status.includes('rows loaded') && rows !== '' && rows !== '-',
    failed: Boolean(root?.classList.contains('bench-workbench--error')) || status.startsWith('Workbench failed:'),
    rows,
    status,
  };
})()`

const workbenchSnapshotJS = `(async () => {
  const root = document.querySelector('[data-benchmark-workbench]');
  const text = (selector) => root?.querySelector(selector)?.textContent?.trim() ?? '';
  const options = (selector) => [...(root?.querySelector(selector)?.options ?? [])];
  const values = (selector) => options(selector).map((option) => option.value).filter(Boolean);
  const viewer = root?.querySelector('[data-bw-viewer]');
  const currentRoute = root?.querySelector('[data-bw-route][aria-current="page"]')?.dataset.bwRoute ?? '';
  const config = currentRoute === 'data' && viewer
    ? await Promise.race([
        viewer.save(),
        new Promise((resolve) => setTimeout(() => resolve({ columns: [] }), 1200)),
      ])
    : { columns: [] };
  const comparisonTaskLink = root?.querySelector('[data-bw-comparison] tbody tr td:first-child a');
  const combinationTaskLink = root?.querySelector('[data-bw-combinations] tbody a');

  return {
    activeFilters: [...(root?.querySelectorAll('[data-bw-active-filters] span') ?? [])].map((el) => el.textContent.trim()),
    combinationRowCount: root?.querySelectorAll('[data-bw-combinations] tbody tr').length ?? 0,
    combinationTaskSort: root?.querySelector('[data-bw-combinations] th:first-child')?.getAttribute('aria-sort') ?? '',
    combinationTaskLinkHref: combinationTaskLink?.href ?? '',
    compareA: root?.querySelector('[data-bw-compare-a]')?.value ?? '',
    compareB: root?.querySelector('[data-bw-compare-b]')?.value ?? '',
    comparisonGapText: text('[data-bw-comparison] tbody tr td:nth-child(4)'),
    comparisonGapSort: root?.querySelector('[data-bw-comparison] th:nth-child(4)')?.getAttribute('aria-sort') ?? '',
    comparisonRowCount: root?.querySelectorAll('[data-bw-comparison] tbody tr').length ?? 0,
    comparisonTaskLinkHref: comparisonTaskLink?.href ?? '',
    currentRoute,
    locationHash: window.location.hash,
    legacyLaneFilterPresent: Boolean(root?.querySelector('[data-bw-filter-field="lane_label"]')),
    modelOptionCount: options('[data-bw-model]').length,
    modelOptions: values('[data-bw-model]'),
    providerOptionCount: options('[data-bw-filter-field="provider_type"]').length,
    providerOptions: values('[data-bw-filter-field="provider_type"]'),
    resultOptionCount: options('[data-bw-result-state]').length,
    rows: text('[data-bw-metric="rows"]'),
    summaryChartCount: root?.querySelectorAll('.bench-workbench__pie, .bench-workbench__bar-row').length ?? 0,
    summaryRows: text('[data-bw-summary-metric="rows"]'),
    settingsColumnCount: viewer?.shadowRoot?.querySelectorAll('#active-columns .column-selector-column, #sub-columns .column-selector-column').length ?? 0,
    settingsOpen: Boolean(viewer?.hasAttribute('settings')),
    status: text('[data-bw-status]'),
    taskOptionCount: options('[data-bw-task]').length,
    visibleColumns: config?.columns ?? [],
  };
})()`
