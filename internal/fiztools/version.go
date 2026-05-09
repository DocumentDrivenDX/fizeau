// Package fiztools versions the parts of fiz that affect agent behavior on
// benchmark tasks. Bump only when changing things that influence what the
// model sees or how it reasons:
//   - System prompts, tool schemas, agent-loop instructions
//   - Default sampling parameters baked into agent code
//   - Tool-call orchestration / message-flow logic
//   - Reasoning-mode handling
//
// Do NOT bump for: provider routing, retry/quota logic, test/build/CI
// plumbing, bench tooling, logging, metrics. Those don't change agent quality
// and shouldn't invalidate prior benchmark cells.
package fiztools

// Version is stamped on every benchmark report.json so cells can be grouped
// by a stable agent-behavior identity that survives unrelated fiz commits.
// Bump with a CHANGELOG entry explaining what changed.
const Version = 1
