package agent

import "errors"

// ErrCompactionNoFit reports that compaction was needed but could not produce
// a message history that fits within the effective context window.
var ErrCompactionNoFit = errors.New("agent: compaction could not fit within the effective context window")

// ErrToolCallLoop reports that the agent produced identical tool calls for
// toolCallLoopLimit consecutive turns, indicating a non-converging loop.
var ErrToolCallLoop = errors.New("agent: identical tool calls repeated, aborting loop")
