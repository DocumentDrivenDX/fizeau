---
ddx:
  id: FEAT-002
  depends_on:
    - helix.prd
---
# Feature Specification: FEAT-002 — Tool Set

**Feature ID**: FEAT-002
**Status**: Draft
**Priority**: P0
**Owner**: DDX Agent Team

## Overview

DDX Agent provides a structured tool surface for filesystem and shell work:
read, write, edit, bash, glob, grep, ls, patch, and task. The LLM uses these
tools to interact with the workspace, discover files, make precise changes,
and track work. Tools are the agent's hands. This implements PRD P0
requirement 2 and reflects the benchmark-driven navigation and task-tracking
capabilities already shipped.

## Problem Statement

- **Current situation**: Each agent CLI implements its own tools with different
  semantics (Claude Code has ~20 tools, pi has 4-7, codex has its own set).
  DDX Agent now ships a broader, benchmark-informed surface than the original
  four-tool minimum.
- **Pain points**: Tool behavior varies across agents. DDx can't predict what
  file operations an agent will perform or constrain them.
- **Desired outcome**: A small, well-defined tool set with consistent behavior
  that DDX Agent controls and DDx can audit.

## Requirements

### Functional Requirements

#### Core file and shell tools

1. `read` accepts path, optional line offset, and optional line limit
2. `read` resolves relative paths against the working directory
3. `read` returns file contents as string and errors when the file is missing
4. `write` accepts path and content, creates parent directories, and overwrites
   the file
5. `edit` accepts either multi-edit `edits[]` or legacy `old_string` + `new_string`
6. `edit` applies multi-edits atomically, from original content, with no overlap
7. `edit` fails when the match is missing or ambiguous
8. `bash` accepts a command and optional timeout, runs in the working directory,
   and captures stdout, stderr, and exit code
9. `bash` kills on timeout or context cancellation

#### Navigation, patching, and task-tracking tools

10. `glob` finds files by pattern for codebase navigation
11. `grep` searches file contents in a read-only way
12. `ls` lists directory contents without requiring a shell command
13. `patch` applies structured search-and-replace edits
14. `task` creates and updates task-tracking records for multi-step work
15. Navigation and patch tools reduce the need for shell `ls`, `find`, and
    `grep` anti-patterns in benchmark workloads

### Non-Functional Requirements

- **Security**: DDX Agent assumes it runs in a sandbox. File paths outside the
  working directory are allowed but logged. No path validation boundary.
- **Performance**: File operations complete in <10ms for files under 1MB.
  Bash tool adds <5ms overhead beyond the command's own execution time.
- **Reliability**: Tools never panic. All errors are returned as structured
  tool results that the model can interpret.

## Edge Cases and Error Handling

- **Symlink chains**: Resolve symlinks fully, log final target path
- **Binary file read**: Return error indicating binary content detected
- **Empty file write**: Allow (creates empty file)
- **Edit with empty old_string**: Reject (would match everything)
- **Bash command produces >1MB output**: Truncate with "[truncated]" marker
- **Bash command is interactive (reads stdin)**: Provide /dev/null as stdin

## Success Metrics

- All shipped tools pass acceptance tests with both local and cloud models
- All file operations are logged with resolved paths
- Bash timeout reliably kills runaway processes

## Constraints and Assumptions

- No network-access tool (bash can do network operations, but there's no
  dedicated fetch/curl tool — keep the surface area small)
- Tools are not extensible in P0. Custom tools are a P2 concern.

## Dependencies

- **Other features**: FEAT-001 (agent loop calls tools)
- **PRD requirements**: P0-2

## Out of Scope

- File watching or filesystem events
- Tool permission management (all tools are available; the caller controls
  scope via working directory)
- MCP tool integration
