package discoverycache

// Algorithm 6 from ADR-012: in-process singleflight composition.
//
// Within a process, singleflight.Group ensures at most one goroutine runs
// refreshAndCommit per source key. Across processes, the file-based marker
// (Algorithm 1) provides the same guarantee. The two layers compose: if
// goroutine A holds the singleflight key, goroutine B blocks on the same
// goroutine and gets the same result — no redundant file IO. If process B
// wins the singleflight but process A already holds the file marker, process B
// falls into waitForRefresh.

// Refresh forces a synchronous refresh regardless of TTL. If a refresh is
// already in flight (another goroutine or process), Refresh waits for it to
// complete. In-process callers share one goroutine via singleflight.
func (c *Cache) Refresh(s Source, fn Refresher) error {
	_, err, _ := c.sf.Do(s.key(), func() (interface{}, error) {
		return nil, c.refreshAndCommit(s, fn)
	})
	return err
}

// MaybeRefresh triggers a background refresh when the data is stale or absent,
// then returns immediately. The caller always gets data from Read; MaybeRefresh
// only schedules the replenishment. Multiple concurrent callers for the same
// source share one background goroutine via singleflight.
func (c *Cache) MaybeRefresh(s Source, fn Refresher) {
	res, err := c.Read(s)
	if err != nil || res.Fresh {
		return
	}
	go func() {
		_, _, _ = c.sf.Do(s.key(), func() (interface{}, error) {
			return nil, c.refreshAndCommit(s, fn)
		})
	}()
}
