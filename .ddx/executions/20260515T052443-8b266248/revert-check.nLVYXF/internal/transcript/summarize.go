package transcript

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ToolSummary struct {
	Action string
	Target string
}

type OutputSummary struct {
	Summary string
	Bytes   int
	Lines   int
	Excerpt string
}

func SummarizeToolCall(toolName string, input json.RawMessage) ToolSummary {
	toolName = strings.TrimSpace(toolName)
	payload := map[string]any{}
	if len(input) > 0 {
		_ = json.Unmarshal(input, &payload)
	}
	path := summaryString(payload, "path", "file")
	switch toolName {
	case "read":
		if path != "" {
			return ToolSummary{Action: "inspect " + lineRangeSummary(payload) + " in " + path, Target: path}
		}
		return ToolSummary{Action: "inspect file"}
	case "write":
		return ToolSummary{Action: "write file", Target: path}
	case "edit", "anchor_edit":
		return ToolSummary{Action: "edit file", Target: path}
	case "patch":
		action := "edit file"
		if op := summaryString(payload, "operation"); op != "" {
			action = op + " file"
		}
		return ToolSummary{Action: action, Target: path}
	case "grep":
		pattern := summaryString(payload, "pattern")
		target := firstNonEmpty(summaryString(payload, "dir"), summaryString(payload, "glob"))
		action := "search"
		if pattern != "" {
			action += " " + strconv.Quote(BoundedText(pattern, 32))
		}
		if target != "" {
			action += " in " + target
		}
		return ToolSummary{Action: action, Target: target}
	case "find":
		pattern := summaryString(payload, "pattern")
		target := summaryString(payload, "dir")
		action := "find files"
		if pattern != "" {
			action += " matching " + strconv.Quote(BoundedText(pattern, 32))
		}
		if target != "" {
			action += " in " + target
		}
		return ToolSummary{Action: action, Target: target}
	case "ls":
		target := summaryString(payload, "path")
		if target == "" {
			target = "."
		}
		return ToolSummary{Action: "list directory " + target, Target: target}
	case "bash":
		return SummarizeShellCommand(ExtractBashCommand(payload))
	default:
		if path != "" {
			return ToolSummary{Action: toolName, Target: path}
		}
		if toolName != "" {
			return ToolSummary{Action: toolName}
		}
		return ToolSummary{}
	}
}

func SummarizeShellCommand(command string) ToolSummary {
	command = NormalizeShellCommand(command)
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ToolSummary{}
	}
	switch fields[0] {
	case "sed":
		if len(fields) >= 4 && fields[1] == "-n" {
			target := fields[3]
			return ToolSummary{Action: "inspect " + shellLineRange(fields[2]) + " in " + target, Target: target}
		}
	case "cat", "head", "tail":
		if len(fields) >= 2 {
			target := fields[len(fields)-1]
			return ToolSummary{Action: "inspect " + target, Target: target}
		}
	case "rg", "grep":
		return summarizeSearchCommand(fields)
	case "go":
		if len(fields) >= 2 && fields[1] == "test" {
			target := strings.Join(fields[2:], " ")
			return ToolSummary{Action: strings.TrimSpace("test " + target), Target: target}
		}
	case "git":
		return summarizeGitCommand(fields)
	case "apply_patch":
		return ToolSummary{Action: "apply patch"}
	}
	return ToolSummary{Action: BoundedText(command, 96)}
}

func NormalizeShellCommand(command string) string {
	command = strings.TrimSpace(command)
	for _, prefix := range []string{"/bin/zsh -lc ", "zsh -lc ", "/bin/bash -lc ", "bash -lc ", "/bin/sh -lc ", "sh -lc "} {
		if !strings.HasPrefix(command, prefix) {
			continue
		}
		inner := strings.TrimSpace(strings.TrimPrefix(command, prefix))
		if unquoted, err := strconv.Unquote(inner); err == nil {
			command = unquoted
		} else {
			command = strings.Trim(inner, `"`)
		}
		break
	}
	for _, sep := range []string{" && ", " || ", " ; "} {
		if idx := strings.Index(command, sep); idx >= 0 {
			command = strings.TrimSpace(command[:idx])
			break
		}
	}
	return strings.Join(strings.Fields(command), " ")
}

func ExtractBashCommand(payload any) string {
	switch v := payload.(type) {
	case map[string]any:
		if s, ok := v["command"].(string); ok {
			return strings.TrimSpace(s)
		}
		if s, ok := v["cmd"].(string); ok {
			return strings.TrimSpace(s)
		}
	case string:
		return strings.TrimSpace(v)
	}
	return ""
}

func SummarizeOutput(output string) OutputSummary {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return OutputSummary{}
	}
	lineCount := strings.Count(trimmed, "\n") + 1
	byteCount := len([]byte(trimmed))
	parts := []string{fmt.Sprintf("out=%s", FormatByteCount(byteCount))}
	if lineCount == 1 {
		parts = append(parts, "1 line")
	} else {
		parts = append(parts, fmt.Sprintf("%d lines", lineCount))
	}
	excerpt := ""
	if byteCount > 40 {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			excerpt = BoundedText(RedactProgressOutput(line), 48)
			parts = append(parts, strconv.Quote(excerpt))
			break
		}
	}
	return OutputSummary{
		Summary: BoundedText(strings.Join(parts, " "), 96),
		Bytes:   byteCount,
		Lines:   lineCount,
		Excerpt: excerpt,
	}
}

func TokenThroughput(outputTokens int, durationMS int64) *float64 {
	if outputTokens <= 0 || durationMS <= 0 {
		return nil
	}
	v := float64(outputTokens) / (float64(durationMS) / 1000)
	return &v
}

func FormatByteCount(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

var sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|authorization|auth)\s*[:=]\s*[^,\s]+`)

func RedactProgressOutput(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return sensitiveAssignmentPattern.ReplaceAllString(s, `$1=[redacted]`)
}

func summarizeSearchCommand(fields []string) ToolSummary {
	pattern := ""
	target := ""
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			continue
		}
		if pattern == "" {
			pattern = strings.Trim(field, "'\"")
			continue
		}
		target = field
	}
	action := "search"
	if pattern != "" {
		action += " " + strconv.Quote(BoundedText(pattern, 32))
	}
	if target != "" {
		action += " in " + target
	}
	return ToolSummary{Action: action, Target: target}
}

func summarizeGitCommand(fields []string) ToolSummary {
	if len(fields) < 2 {
		return ToolSummary{Action: "git"}
	}
	switch fields[1] {
	case "add":
		target := strings.Join(fields[2:], " ")
		if target == "" {
			return ToolSummary{Action: "stage changes"}
		}
		return ToolSummary{Action: "stage changes", Target: target}
	case "commit":
		return ToolSummary{Action: "commit changes"}
	case "diff":
		return ToolSummary{Action: "inspect diff"}
	case "status":
		return ToolSummary{Action: "inspect git status"}
	case "log":
		return ToolSummary{Action: "inspect git log"}
	default:
		return ToolSummary{Action: "git " + fields[1]}
	}
}

func shellLineRange(expr string) string {
	expr = strings.Trim(strings.TrimSpace(expr), "'\"")
	expr = strings.TrimSuffix(expr, "p")
	if expr == "" {
		return "lines"
	}
	return "lines " + expr
}

func lineRangeSummary(payload map[string]any) string {
	offset := summaryInt(payload, "offset")
	limit := summaryInt(payload, "limit")
	if offset <= 0 && limit <= 0 {
		return "file"
	}
	if limit > 0 {
		return fmt.Sprintf("lines %d-%d", offset+1, offset+limit)
	}
	return fmt.Sprintf("from line %d", offset+1)
}

func summaryString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func summaryInt(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func SummarizeJSONValue(raw json.RawMessage) string {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return summarizeAnyValue(payload)
}

func summarizeAnyValue(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 3 {
			keys = keys[:3]
		}
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, summarizeValueForKey(key, x[key])))
		}
		if len(x) > len(keys) {
			parts = append(parts, "...")
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		return fmt.Sprintf("[%d item(s)]", len(x))
	case string:
		return fmt.Sprintf("%q", BoundedText(x, 32))
	case float64, bool, nil:
		return fmt.Sprint(x)
	default:
		return fmt.Sprintf("%T", x)
	}
}

func summarizeValueForKey(key string, v any) string {
	if IsSensitiveSummaryKey(key) {
		return "[redacted]"
	}
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("%q", BoundedText(x, 32))
	case map[string]any, []any:
		return summarizeAnyValue(x)
	case float64, bool, nil:
		return fmt.Sprint(x)
	default:
		return fmt.Sprintf("%T", x)
	}
}

func IsSensitiveSummaryKey(key string) bool {
	key = strings.ToLower(key)
	switch {
	case strings.Contains(key, "secret"):
		return true
	case strings.Contains(key, "token"):
		return true
	case strings.Contains(key, "password"):
		return true
	case strings.Contains(key, "passwd"):
		return true
	case strings.Contains(key, "api_key"):
		return true
	case strings.Contains(key, "apikey"):
		return true
	case strings.Contains(key, "key"):
		return true
	case strings.Contains(key, "auth"):
		return true
	default:
		return false
	}
}
