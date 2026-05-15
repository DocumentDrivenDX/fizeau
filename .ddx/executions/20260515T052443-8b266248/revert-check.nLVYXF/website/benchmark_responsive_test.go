package website

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type responsiveProbeResult struct {
	InnerWidth      int64    `json:"innerWidth"`
	DocClientWidth  int64    `json:"docClientWidth"`
	DocScrollWidth  int64    `json:"docScrollWidth"`
	BodyClientWidth int64    `json:"bodyClientWidth"`
	BodyScrollWidth int64    `json:"bodyScrollWidth"`
	TextOverflow    []string `json:"overflow"`
	Overlaps        []string `json:"overlaps"`
}

func TestBenchmarkPagesFitAt390px(t *testing.T) {
	buildDir := t.TempDir()
	buildWebsite(t, buildDir)

	chromePath := ensureChromium(t)
	pageURLs := []string{
		"benchmarks/terminal-bench-2-1/index.html",
		"benchmarks/terminal-bench-2-1/models/index.html",
		"benchmarks/terminal-bench-2-1/harnesses/index.html",
		"benchmarks/terminal-bench-2-1/providers/index.html",
	}

	for _, rel := range pageURLs {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			result := probePage(t, buildDir, chromePath, rel)

			if result.InnerWidth != 390 || result.DocClientWidth != 390 {
				t.Fatalf("viewport not set to 390px: innerWidth=%d docClientWidth=%d", result.InnerWidth, result.DocClientWidth)
			}
			if len(result.TextOverflow) > 0 {
				t.Fatalf("unexpected text overflow on %s: %s", rel, strings.Join(result.TextOverflow, ", "))
			}
			if len(result.Overlaps) > 0 {
				t.Fatalf("unexpected text overlap on %s: %s", rel, strings.Join(result.Overlaps, ", "))
			}
		})
	}
}

func buildWebsite(t *testing.T, dest string) {
	t.Helper()

	hugoPath, err := exec.LookPath("hugo")
	if err != nil {
		t.Fatalf("find hugo: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, hugoPath,
		"--quiet",
		"--baseURL", "file://"+dest+"/",
		"--destination", dest,
	)
	cmd.Dir = websiteRoot(t)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build website: %v\n%s", err, output)
	}
}

func ensureChromium(t *testing.T) string {
	t.Helper()

	if path, err := findChromiumBinary(); err == nil {
		return path
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npx", "--yes", "playwright", "install", "chromium")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install chromium browser: %v\n%s", err, output)
	}

	path, err := findChromiumBinary()
	if err != nil {
		t.Fatalf("find chromium after install: %v", err)
	}
	return path
}

func findChromiumBinary() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	patterns := []string{
		filepath.Join(home, ".cache", "ms-playwright", "chromium-*", "chrome-linux", "chrome"),
		filepath.Join(home, ".cache", "ms-playwright", "chromium_headless_shell-*", "chrome-linux", "headless_shell"),
	}

	for i, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			continue
		}

		sort.Strings(matches)
		if i == 0 {
			return matches[len(matches)-1], nil
		}
		// Fall back to headless_shell only if no standard Chromium binary exists.
		return matches[len(matches)-1], nil
	}

	return "", errors.New("no Playwright Chromium binary found")
}

func probePage(t *testing.T, buildDir, chromePath, rel string) responsiveProbeResult {
	t.Helper()

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

	url := "file://" + filepath.Join(buildDir, rel)

	var result responsiveProbeResult
	err := chromedp.Run(browserCtx,
		chromedp.EmulateViewport(390, 1200, chromedp.EmulateScale(1)),
		chromedp.Navigate(url),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(responsiveProbeJS, &result),
	)
	if err != nil {
		t.Fatalf("probe %s: %v", rel, err)
	}

	return result
}

func websiteRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get website working directory: %v", err)
	}
	return dir
}

const responsiveProbeJS = `(() => {
  const parents = [document.querySelector('.br-body'), ...document.querySelectorAll('.narrative')].filter(Boolean);
  const overflow = [];
  const overlaps = [];

  for (const parent of parents) {
    const kids = [...parent.children].filter((el) => !el.matches('table, .chart'));
    const visible = kids.filter((el) => {
      const r = el.getBoundingClientRect();
      const s = getComputedStyle(el);
      return r.width > 0 && r.height > 0 && s.display !== 'none' && s.visibility !== 'hidden';
    });

    for (let i = 0; i < visible.length; i++) {
      const el = visible[i];
      if (el.scrollWidth - el.clientWidth > 8 && !el.querySelector('table, .chart')) {
        overflow.push((el.className || el.tagName) + ':' + (el.scrollWidth - el.clientWidth));
      }

      if (i > 0) {
        const prev = visible[i - 1].getBoundingClientRect();
        const cur = el.getBoundingClientRect();
        if (cur.top < prev.bottom - 1) {
          overlaps.push((visible[i - 1].className || visible[i - 1].tagName) + ' -> ' + (el.className || el.tagName));
        }
      }
    }
  }

  return {
    innerWidth: window.innerWidth,
    docClientWidth: document.documentElement.clientWidth,
    docScrollWidth: document.documentElement.scrollWidth,
    bodyClientWidth: document.body.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
    overflow,
    overlaps,
  };
})()`
