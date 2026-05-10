package anchorstore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/easel/fizeau/internal/tool/anchorwords"
)

func TestAssignLookup(t *testing.T) {
	store := New()

	store.Assign("file.go", 0, []string{"first", "second", "third"})

	line, ambiguous := store.Lookup("file.go", anchorwords.Anchors[1])
	if ambiguous {
		t.Fatal("Lookup returned ambiguous for a unique anchor")
	}
	if line != 1 {
		t.Fatalf("Lookup line = %d, want 1", line)
	}
}

func TestLookupNotFound(t *testing.T) {
	store := New()
	store.Assign("file.go", 0, []string{"first"})

	line, ambiguous := store.Lookup("file.go", "NotAnAnchor")
	if line != -1 || ambiguous {
		t.Fatalf("Lookup missing anchor = (%d, %v), want (-1, false)", line, ambiguous)
	}

	line, ambiguous = store.Lookup("missing.go", anchorwords.Anchors[0])
	if line != -1 || ambiguous {
		t.Fatalf("Lookup missing path = (%d, %v), want (-1, false)", line, ambiguous)
	}
}

func TestInvalidate(t *testing.T) {
	store := New()
	store.Assign("file.go", 0, []string{"first"})
	store.Invalidate("file.go")

	line, ambiguous := store.Lookup("file.go", anchorwords.Anchors[0])
	if line != -1 || ambiguous {
		t.Fatalf("Lookup invalidated anchor = (%d, %v), want (-1, false)", line, ambiguous)
	}
}

func TestOffsetCorrectness(t *testing.T) {
	store := New()
	const offset = len(anchorwords.Anchors) + 37

	store.Assign("file.go", offset, []string{"line 37", "line 38"})

	line, ambiguous := store.Lookup("file.go", anchorwords.Anchors[37])
	if ambiguous {
		t.Fatal("Lookup returned ambiguous for offset anchor")
	}
	if line != offset {
		t.Fatalf("Lookup line = %d, want %d", line, offset)
	}

	line, ambiguous = store.Lookup("file.go", anchorwords.Anchors[38])
	if ambiguous {
		t.Fatal("Lookup returned ambiguous for second offset anchor")
	}
	if line != offset+1 {
		t.Fatalf("Lookup line = %d, want %d", line, offset+1)
	}
}

func TestAssignReplacesPathState(t *testing.T) {
	store := New()
	store.Assign("file.go", 0, []string{"first", "second"})
	store.Assign("file.go", 10, []string{"replacement"})

	line, ambiguous := store.Lookup("file.go", anchorwords.Anchors[0])
	if line != -1 || ambiguous {
		t.Fatalf("Lookup stale anchor = (%d, %v), want (-1, false)", line, ambiguous)
	}

	line, ambiguous = store.Lookup("file.go", anchorwords.Anchors[10])
	if ambiguous {
		t.Fatal("Lookup returned ambiguous for replacement anchor")
	}
	if line != 10 {
		t.Fatalf("Lookup replacement line = %d, want 10", line)
	}
}

func TestLargeFileWrappingIsAmbiguous(t *testing.T) {
	store := New()
	lines := make([]string, len(anchorwords.Anchors)+1)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}

	store.Assign("large.go", 0, lines)

	line, ambiguous := store.Lookup("large.go", anchorwords.Anchors[0])
	if line != -1 || !ambiguous {
		t.Fatalf("Lookup wrapped anchor = (%d, %v), want (-1, true)", line, ambiguous)
	}
}

func TestZeroValueStore(t *testing.T) {
	var store Store

	store.Assign("file.go", 0, []string{"first"})

	line, ambiguous := store.Lookup("file.go", anchorwords.Anchors[0])
	if ambiguous {
		t.Fatal("Lookup returned ambiguous for zero-value store assignment")
	}
	if line != 0 {
		t.Fatalf("Lookup line = %d, want 0", line)
	}
}

func TestConcurrentRead(t *testing.T) {
	store := New()
	lines := make([]string, len(anchorwords.Anchors))
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	store.Assign("file.go", 0, lines)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				idx := (worker + j) % len(anchorwords.Anchors)
				line, ambiguous := store.Lookup("file.go", anchorwords.Anchors[idx])
				if ambiguous {
					t.Errorf("Lookup(%d) unexpectedly ambiguous", idx)
					return
				}
				if line != idx {
					t.Errorf("Lookup(%d) line = %d, want %d", idx, line, idx)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
