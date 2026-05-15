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
// only schedules the replenishment. Claiming happens synchronously so repeated
// stale reads do not fan out duplicate background goroutines.
func (c *Cache) MaybeRefresh(s Source, fn Refresher) {
	res, err := c.Read(s)
	if err != nil || res.Fresh {
		return
	}
	claim, err := c.claimRefresh(s)
	if err != nil || !claim.claimed {
		return
	}
	go func() {
		_ = c.refreshWithClaim(s, fn)
	}()
}

// MaybeRefreshSync refreshes synchronously only when the cache is stale or
// absent. Returns the refresh error (nil if data was fresh or the refresh
// succeeded). Composes with single-flight: concurrent callers share one
// refreshAndCommit, both in-process (singleflight.Group) and cross-process
// (the file marker). This is for explicit refresh/preflight surfaces; route
// hot paths use MaybeRefresh so stale providers cannot block scoring.
func (c *Cache) MaybeRefreshSync(s Source, fn Refresher) error {
	res, err := c.Read(s)
	if err != nil {
		return err
	}
	if res.Fresh {
		return nil
	}
	if state, stateErr := c.RefreshState(s); stateErr == nil && state.Failed && state.InFlight {
		return nil
	}
	return c.Refresh(s, fn)
}
