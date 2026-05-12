package main

import (
	"regexp"
	"strings"
)

const (
	matrixInvalidQuota     = "invalid_quota"
	matrixInvalidAuth      = "invalid_auth"
	matrixInvalidSetup     = "invalid_setup"
	matrixInvalidProvider  = "invalid_provider"
	matrixInvalidLaneAbort = "lane_aborted"
)

var (
	matrixInvalidQuotaPattern = regexp.MustCompile(`(?i)(api_error_status:\s*429|insufficient[_\s-]*quota|out_of_credits|credits?\s+exhausted|usage\s+exhausted|rate\s*limit|too many requests|quota\s+exhausted|quota\s+exceeded)`)
	matrixInvalidAuthPattern  = regexp.MustCompile(`(?i)(unauthori[sz]ed|authentication failed|invalid api key|missing credentials?|not signed in|login required|account .*not .*authenticated|oauth.*failed|credential.*missing|account .*required|access denied)`)

	// Setup signals split into two tiers.
	//
	// matrixInvalidSetupDefinitivePattern: high-precision runtime errors
	// that are unambiguous config/setup mismatches even when emitted
	// during a real model attempt. Safe to trust on graded_fail with
	// turns > 0.
	//
	// matrixInvalidSetupBroadPattern: lower-precision substrings that
	// catch generic harness/container failures. Many of these false-
	// positive against the wrapper bash script that gets included in the
	// error blob (`permission denied` from chmod fallbacks, `failed to
	// start` from generic error fallbacks, `docker.*(failed|error)`
	// matches anywhere with a docker context). Only safe to apply when
	// there was no real model attempt.
	matrixInvalidSetupDefinitivePattern = regexp.MustCompile(`(?i)(binary not found|exec format error|cannot execute binary file|wrong architecture|architecture mismatch|task dir not found|submodule not initialized|wrapper startup|asyncio\.run\(\) cannot be called from a running event loop|preflight failure|reasoning=[^ ]* is not supported by provider type|reasoning_wire=none|qwen reasoning control is not supported|unsupported reasoning [^ ]* for harness)`)
	matrixInvalidSetupBroadPattern      = regexp.MustCompile(`(?i)(no such file or directory|failed to start|startup failed|docker.*(failed|error)|container.*(failed|error)|image.*(failed|error|not found)|harbor[\\/]+environments[\\/]+docker|_run_docker_compose_command|_start_environment_with_retry|docker compose command|operation not permitted|permission denied|sandbox.*failed|setup failed)`)

	matrixInvalidProviderPattern = regexp.MustCompile(`(?i)(connection refused|connection reset|socket hang up|fetch failed|tls handshake|dns|eof|timed out|timeout|stream closed|broken pipe|remote closed|upstream|service unavailable|bad gateway|gateway timeout|failed to connect|provider transport|network error)`)
)

func classifyMatrixInvalid(report matrixRunReport) string {
	if report.FinalStatus == "graded_pass" {
		return ""
	}
	if isMatrixKnownInvalidClass(report.FinalStatus) {
		return report.FinalStatus
	}
	if report.InvalidClass != "" {
		return report.InvalidClass
	}

	// "Invalid" means the run didn't work because of systemic issues — not
	// "we tried our best and didn't pass". A real attempt that hits the
	// verifier and gets reward=0 is a graded_fail, full stop. The only
	// signals strong enough to reclassify a real attempt are QUOTA and
	// AUTH (definitive provider/account state) — never SETUP or PROVIDER
	// transport, whose regexes match the wrapper bash script and verifier
	// output ("eof" matches heredoc <<EOF markers; "timeout" matches task
	// content; "permission denied" matches chmod fallbacks; etc.).
	hasAttempt := matrixHasMeaningfulAttempt(report)
	signal := matrixInvalidSignalBlob(report)
	if hasAttempt {
		// Only high-precision signals are trustworthy on real attempts.
		// Broad SETUP/PROVIDER patterns false-positive on wrapper bash
		// content and verifier output — see pattern doc-comments above.
		if matrixInvalidQuotaPattern.MatchString(signal) {
			return matrixInvalidQuota
		}
		if matrixInvalidAuthPattern.MatchString(signal) {
			return matrixInvalidAuth
		}
		if matrixInvalidSetupDefinitivePattern.MatchString(signal) {
			return matrixInvalidSetup
		}
	} else if class := classifyMatrixInvalidText(signal); class != "" {
		return class
	}
	switch report.FinalStatus {
	case "verifier_fail":
		return ""
	case "install_fail_permanent", "install_failed":
		return matrixInvalidSetup
	case "harness_crash":
		// The agent runtime / wrapper crashed before producing a graded
		// outcome. By definition systemic — usually per-trial timeout,
		// docker subnet exhaustion, or external cancellation. Tag as
		// invalid_setup so it doesn't pollute pass-rate denominators.
		return matrixInvalidSetup
	case "ran":
		// final_status="ran" + ungraded + no meaningful attempt means the
		// harbor wrapper exited cleanly but the trial never actually ran
		// the agent — typically docker image pull failure, environment
		// setup error, or other pre-agent infra issue. The actual exception
		// lives in a side-file (exception.txt), not in the report's error
		// blob (which only carries the harbor summary table), so the text
		// classifier can't see it. The structural signal is enough.
		if !hasAttempt && (report.GradingOutcome == "" || report.GradingOutcome == "ungraded") {
			return matrixInvalidSetup
		}
	case "graded_fail":
		// Conservative quality-attribution rule: a graded_fail with no
		// meaningful agent attempt (zero turns, zero output tokens) is
		// almost certainly a harness/provider/setup failure that Harbor
		// happened to verify cleanly with reward=0. Sub-classify so the
		// matrix tells these apart:
		//   - had_llm_request=true + no response → provider hung/errored
		//     (5xx, rate limit drop, stream closed before token). This is
		//     a *provider* failure, retriable via --retry-invalid. The
		//     model itself never got to try.
		//   - had_llm_request=false / unknown → never reached the model
		//     (setup error before request). Stays invalid_setup.
		if !hasAttempt {
			if report.HadLLMRequest != nil && *report.HadLLMRequest &&
				(report.TerminatedMidWork == nil) {
				return matrixInvalidProvider
			}
			return matrixInvalidSetup
		}
		// Retry-spam: fiz retries transient provider errors internally up to
		// ~5 times; each retry increments turns but produces no tokens. A real
		// LLM turn always consumes input_tokens > 0 (the prompt is sent).
		// Zero input+output tokens with had_llm_request=true and no response
		// means the provider was unreachable on every attempt — not a model
		// quality failure.
		if intValue(report.InputTokens) == 0 && intValue(report.OutputTokens) == 0 &&
			report.HadLLMRequest != nil && *report.HadLLMRequest &&
			report.TerminatedMidWork == nil {
			return matrixInvalidProvider
		}
		// Even with some turns, if output tokens are zero AND wall is
		// suspiciously short (<30s) we treat it as harness-level — the
		// model never produced usable tokens, so reward=0 isn't a model
		// quality signal. Threshold tuned so genuinely hard tasks where
		// the model thinks-then-fails-fast still graded_fail correctly.
		if intValue(report.OutputTokens) == 0 && intValue(report.Turns) <= 2 {
			if w := report.WallSeconds; w != nil && *w < 30 {
				return matrixInvalidSetup
			}
		}
		return ""
	}
	return ""
}

// classifyMatrixInvalidText is the no-meaningful-attempt path: when the
// agent never produced tokens, the broad SETUP / PROVIDER patterns become
// safe to apply (the error blob is dominated by infrastructure messages
// rather than wrapper bash + task content).
func classifyMatrixInvalidText(blob string) string {
	blob = strings.ToLower(strings.TrimSpace(blob))
	if blob == "" {
		return ""
	}
	switch {
	case matrixInvalidQuotaPattern.MatchString(blob):
		return matrixInvalidQuota
	case matrixInvalidAuthPattern.MatchString(blob):
		return matrixInvalidAuth
	case matrixInvalidSetupDefinitivePattern.MatchString(blob),
		matrixInvalidSetupBroadPattern.MatchString(blob):
		return matrixInvalidSetup
	case matrixInvalidProviderPattern.MatchString(blob):
		return matrixInvalidProvider
	default:
		return ""
	}
}

func isMatrixKnownInvalidClass(status string) bool {
	switch status {
	case matrixInvalidQuota, matrixInvalidAuth, matrixInvalidSetup, matrixInvalidProvider, matrixInvalidLaneAbort:
		return true
	default:
		return false
	}
}

func matrixHasMeaningfulAttempt(report matrixRunReport) bool {
	return intValue(report.Turns) > 0 ||
		intValue(report.ToolCalls) > 0 ||
		intValue(report.ToolCallErrors) > 0 ||
		intValue(report.InputTokens) > 0 ||
		intValue(report.OutputTokens) > 0 ||
		intValue(report.CachedInputTokens) > 0 ||
		intValue(report.RetriedInputTokens) > 0
}

func matrixInvalidSignalBlob(report matrixRunReport) string {
	parts := []string{
		report.Error,
		report.ProcessOutcome,
		report.FinalStatus,
		strings.Join(report.AdapterTranslationNotes, " "),
		strings.Join(report.Command, " "),
	}
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.ToLower(strings.Join(out, "\n"))
}
