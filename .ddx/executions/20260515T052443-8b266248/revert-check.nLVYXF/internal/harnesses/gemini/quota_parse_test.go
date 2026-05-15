package gemini

import (
	"strings"
	"testing"
)

// The observed Gemini CLI 0.38.2 /model manage dialog laid out one row per
// tier with "<label> <percent>% used  Resets <time> (<remaining>)" fragments.
// The fixture inline here mirrors that layout so the parser is exercised
// against the real shape captured during PTY smoke testing.
const geminiModelManageFixture = `Model management

  Flash         4% used      Resets 9:13 PM (23h 46m)
  Flash Lite    0% used      Resets 9:27 PM (24h)
  Pro         100% used

Use arrow keys to switch models. Press enter to select.
`

func TestParseGeminiModelManage_CapturesAllThreeTiers(t *testing.T) {
	windows := ParseGeminiModelManage(geminiModelManageFixture)
	if len(windows) != 3 {
		t.Fatalf("want 3 tier windows, got %d: %#v", len(windows), windows)
	}

	flash := FindGeminiQuotaWindow(windows, "gemini-flash")
	if flash == nil {
		t.Fatal("missing gemini-flash window")
	}
	if flash.UsedPercent != 4 {
		t.Fatalf("flash used percent: got %v want 4", flash.UsedPercent)
	}
	if flash.State != "ok" {
		t.Fatalf("flash state: got %q want ok", flash.State)
	}
	if !strings.Contains(flash.ResetsAt, "9:13 PM") {
		t.Fatalf("flash reset text should include reset time: %q", flash.ResetsAt)
	}

	lite := FindGeminiQuotaWindow(windows, "gemini-flash-lite")
	if lite == nil {
		t.Fatal("missing gemini-flash-lite window")
	}
	if lite.UsedPercent != 0 {
		t.Fatalf("flash-lite used percent: got %v want 0", lite.UsedPercent)
	}
	if lite.State != "ok" {
		t.Fatalf("flash-lite state: got %q want ok", lite.State)
	}
	if !strings.Contains(lite.ResetsAt, "9:27 PM") {
		t.Fatalf("flash-lite reset text should include reset time: %q", lite.ResetsAt)
	}

	pro := FindGeminiQuotaWindow(windows, "gemini-pro")
	if pro == nil {
		t.Fatal("missing gemini-pro window")
	}
	if pro.UsedPercent != 100 {
		t.Fatalf("pro used percent: got %v want 100", pro.UsedPercent)
	}
	if pro.State != "exhausted" {
		t.Fatalf("pro state: got %q want exhausted (100%% used)", pro.State)
	}
}

func TestParseGeminiModelManage_FlashLitePrecedence(t *testing.T) {
	// Regression: "Flash Lite" must not be parsed as "Flash".
	text := "  Flash Lite    7% used      Resets Monday\n"
	windows := ParseGeminiModelManage(text)
	if len(windows) != 1 {
		t.Fatalf("want 1 window, got %d: %#v", len(windows), windows)
	}
	if windows[0].LimitID != "gemini-flash-lite" {
		t.Fatalf("label precedence failed: %#v", windows[0])
	}
	if windows[0].UsedPercent != 7 {
		t.Fatalf("flash-lite used percent: got %v want 7", windows[0].UsedPercent)
	}
}

func TestParseGeminiModelManage_SkipsRowsWithoutPercent(t *testing.T) {
	// A tier header alone (no "% used") should not mint a window, because
	// the "quota unknown" state is the correct answer when the CLI fails
	// to render usage.
	text := `Pro

Reset in 24h
`
	windows := ParseGeminiModelManage(text)
	if len(windows) != 0 {
		t.Fatalf("want 0 windows for tier-without-percent text, got %#v", windows)
	}
}

func TestParseGeminiModelManage_TolerantOfANSIAndCR(t *testing.T) {
	text := "\x1b[1m  Flash\x1b[0m    4% used      Resets 9:13 PM (23h 46m)\r\n" +
		"\x1b[1m  Pro\x1b[0m    100% used\r\n"
	windows := ParseGeminiModelManage(text)
	if len(windows) != 2 {
		t.Fatalf("want 2 windows from ANSI+CRLF text, got %d", len(windows))
	}
	if pro := FindGeminiQuotaWindow(windows, "gemini-pro"); pro == nil || pro.State != "exhausted" {
		t.Fatalf("pro window with ANSI should still be exhausted: %#v", pro)
	}
}

func TestTierLimitIDForModel(t *testing.T) {
	cases := map[string]string{
		"gemini-2.5-pro":        "gemini-pro",
		"gemini-2.5-flash":      "gemini-flash",
		"gemini-2.5-flash-lite": "gemini-flash-lite",
		"GEMINI-2.5-FLASH-LITE": "gemini-flash-lite",
	}
	for model, want := range cases {
		got, ok := TierLimitIDForModel(model)
		if !ok {
			t.Fatalf("TierLimitIDForModel(%q): not recognised", model)
		}
		if got != want {
			t.Fatalf("TierLimitIDForModel(%q) = %q, want %q", model, got, want)
		}
	}
	if _, ok := TierLimitIDForModel(""); ok {
		t.Fatal("empty model should not map to a tier")
	}
	if _, ok := TierLimitIDForModel("claude-sonnet-4-6"); ok {
		t.Fatal("non-gemini model must not map to a gemini tier")
	}
}
