// Package anchorstore tracks per-session line anchors for file reads.
package anchorstore

import (
	"sync"

	"github.com/easel/fizeau/internal/tool/anchorwords"
)

// Store keeps the current anchor assignment for each path.
type Store struct {
	mu    sync.RWMutex
	files map[string]fileState
}

// AnchorStore is the public store type used by anchor-aware tools.
type AnchorStore = Store

type fileState struct {
	anchors map[string]anchorState
	count   int
}

type anchorState struct {
	line      int
	ambiguous bool
	choices   []int
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		files: make(map[string]fileState),
	}
}

// Assign replaces path's anchor map for lines starting at fileOffset.
func (s *Store) Assign(path string, fileOffset int, lines []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureFiles()

	state := fileState{
		anchors: make(map[string]anchorState, len(lines)),
		count:   len(lines),
	}
	for i := range lines {
		line := fileOffset + i
		anchor := anchorwords.Anchors[positiveMod(line, len(anchorwords.Anchors))]
		current, ok := state.anchors[anchor]
		if ok {
			current.ambiguous = true
			current.line = -1
			current.choices = append(current.choices, line)
			state.anchors[anchor] = current
			continue
		}
		state.anchors[anchor] = anchorState{line: line, choices: []int{line}}
	}
	s.files[path] = state
}

// Lookup returns anchor's line for path.
//
// Results are encoded as:
//   - unique match: line, false
//   - ambiguous match: -1, true
//   - not found: -1, false
func (s *Store) Lookup(path string, anchor string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.files[path]
	if !ok {
		return -1, false
	}
	found, ok := state.anchors[anchor]
	if !ok {
		return -1, false
	}
	if found.ambiguous {
		return -1, true
	}
	return found.line, false
}

// Resolve returns all known candidate lines for anchor on path and the number
// of lines covered by the current assignment.
func (s *Store) Resolve(path string, anchor string) ([]int, int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.files[path]
	if !ok {
		return nil, 0, false
	}
	found, ok := state.anchors[anchor]
	if !ok {
		return nil, state.count, false
	}
	choices := append([]int(nil), found.choices...)
	return choices, state.count, true
}

// Invalidate clears path's anchor assignments.
func (s *Store) Invalidate(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.files, path)
}

func (s *Store) ensureFiles() {
	if s.files == nil {
		s.files = make(map[string]fileState)
	}
}

func positiveMod(n, d int) int {
	if d <= 0 {
		return 0
	}
	mod := n % d
	if mod < 0 {
		mod += d
	}
	return mod
}
