package main

import (
	"regexp"
	"strings"
)

const (
	matrixInvalidQuota    = "invalid_quota"
	matrixInvalidAuth     = "invalid_auth"
	matrixInvalidSetup    = "invalid_setup"
	matrixInvalidProvider = "invalid_provider"
)

var (
	matrixInvalidQuotaPattern    = regexp.MustCompile(`(?i)(api_error_status:\s*429|insufficient[_\s-]*quota|out_of_credits|credits?\s+exhausted|usage\s+exhausted|rate\s*limit|too many requests|quota\s+exhausted|quota\s+exceeded)`)
	matrixInvalidAuthPattern     = regexp.MustCompile(`(?i)(unauthori[sz]ed|authentication failed|invalid api key|missing credentials?|not signed in|login required|account .*not .*authenticated|oauth.*failed|credential.*missing|account .*required|access denied)`)
	matrixInvalidSetupPattern    = regexp.MustCompile(`(?i)(binary not found|no such file or directory|exec format error|cannot execute binary file|wrong architecture|architecture mismatch|task dir not found|submodule not initialized|failed to start|wrapper startup|startup failed|docker.*(failed|error)|container.*(failed|error)|image.*(failed|error|not found)|operation not permitted|permission denied|sandbox.*failed|setup failed|preflight failure)`)
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
	if class := classifyMatrixInvalidText(matrixInvalidSignalBlob(report)); class != "" {
		return class
	}
	switch report.FinalStatus {
	case "graded_fail", "verifier_fail":
		return ""
	case "install_fail_permanent", "install_failed":
		return matrixInvalidSetup
	}
	if matrixHasMeaningfulAttempt(report) {
		return ""
	}
	return ""
}

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
	case matrixInvalidSetupPattern.MatchString(blob):
		return matrixInvalidSetup
	case matrixInvalidProviderPattern.MatchString(blob):
		return matrixInvalidProvider
	default:
		return ""
	}
}

func isMatrixKnownInvalidClass(status string) bool {
	switch status {
	case matrixInvalidQuota, matrixInvalidAuth, matrixInvalidSetup, matrixInvalidProvider:
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
