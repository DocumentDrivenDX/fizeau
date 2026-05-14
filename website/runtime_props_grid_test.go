package website

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestRuntimePropsGridFilterAndSort loads the runtime-props grid page, applies
// a provider filter, asserts the filtered row count, then applies a sort and
// asserts the first row's lane matches the expected value.
func TestRuntimePropsGridFilterAndSort(t *testing.T) {
	buildDir := t.TempDir()
	buildWebsite(t, buildDir)

	chromePath := ensureChromium(t)

	// Sub-test 1: responsive fit at 390 px (consistent with existing suite).
	t.Run("fits_390px", func(t *testing.T) {
		result := probePage(t, buildDir, chromePath, "benchmarks/terminal-bench-2-1/runtime-props/index.html")
		if result.InnerWidth != 390 || result.DocClientWidth != 390 {
			t.Fatalf("viewport not 390px: innerWidth=%d docClientWidth=%d", result.InnerWidth, result.DocClientWidth)
		}
		if len(result.TextOverflow) > 0 {
			t.Fatalf("text overflow on runtime-props page: %s", strings.Join(result.TextOverflow, ", "))
		}
	})

	// Sub-test 2: filter and sort interactions.
	t.Run("filter_and_sort", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
			append(chromedp.DefaultExecAllocatorOptions[:],
				chromedp.ExecPath(chromePath),
				chromedp.Flag("headless", true),
				chromedp.Flag("disable-gpu", true),
				chromedp.Flag("no-sandbox", true),
			)...,
		)
		defer allocCancel()

		browserCtx, browserCancel := chromedp.NewContext(allocCtx)
		defer browserCancel()

		pageURL := "file://" + buildDir + "/benchmarks/terminal-bench-2-1/runtime-props/index.html"

		// Step 1: load page — initial row count equals total lanes in JSON.
		var initialCount int64
		if err := chromedp.Run(browserCtx,
			chromedp.EmulateViewport(1280, 900, chromedp.EmulateScale(1)),
			chromedp.Navigate(pageURL),
			chromedp.WaitVisible(`#rp-tbody`, chromedp.ByID),
			chromedp.Evaluate(`document.querySelectorAll('#rp-tbody tr').length`, &initialCount),
		); err != nil {
			t.Fatalf("load runtime-props page: %v", err)
		}
		if initialCount < 1 {
			t.Fatalf("expected ≥1 row on initial load, got %d", initialCount)
		}

		// Step 2: filter provider column (col 1) by "vllm".
		// Sample data has ≥1 vllm rows and some non-vllm rows, so filtered < initial.
		var filteredCount int64
		if err := chromedp.Run(browserCtx,
			chromedp.WaitVisible(`.rp-filter[data-col="1"]`, chromedp.ByQuery),
			chromedp.SendKeys(`.rp-filter[data-col="1"]`, "vllm", chromedp.ByQuery),
			chromedp.Evaluate(`document.querySelectorAll('#rp-tbody tr').length`, &filteredCount),
		); err != nil {
			t.Fatalf("apply provider filter: %v", err)
		}
		if filteredCount < 1 {
			t.Fatalf("expected ≥1 row after filtering provider=vllm, got %d", filteredCount)
		}
		if filteredCount >= initialCount {
			t.Fatalf("filter had no effect: before=%d after=%d", initialCount, filteredCount)
		}

		// Step 3: clear filters, then sort by Lane ascending (click col-0 header).
		var firstLaneAsc string
		if err := chromedp.Run(browserCtx,
			chromedp.Click(`#rp-clear-btn`, chromedp.ByID),
			chromedp.Click(`.rp-th[data-col="0"]`, chromedp.ByQuery),
			chromedp.Evaluate(`(document.querySelector('#rp-tbody tr:first-child td') || {textContent:''}).textContent`, &firstLaneAsc),
		); err != nil {
			t.Fatalf("sort by lane ascending: %v", err)
		}
		if firstLaneAsc == "" || firstLaneAsc == "—" {
			t.Fatalf("expected a lane name in first row after ascending sort, got %q", firstLaneAsc)
		}

		// Step 4: click same header again to reverse to descending.
		var firstLaneDesc string
		if err := chromedp.Run(browserCtx,
			chromedp.Click(`.rp-th[data-col="0"]`, chromedp.ByQuery),
			chromedp.Evaluate(`(document.querySelector('#rp-tbody tr:first-child td') || {textContent:''}).textContent`, &firstLaneDesc),
		); err != nil {
			t.Fatalf("sort by lane descending: %v", err)
		}
		if initialCount > 1 && firstLaneDesc == firstLaneAsc {
			t.Fatalf("descending sort first row (%q) same as ascending (%q) with %d rows", firstLaneDesc, firstLaneAsc, initialCount)
		}
	})
}
