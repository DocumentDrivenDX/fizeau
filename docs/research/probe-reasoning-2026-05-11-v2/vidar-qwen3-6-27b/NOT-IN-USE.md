# vidar-qwen3-6-27b

This legacy oMLX lane was not re-probed in the v2 sweep. The profile is kept
for historical reruns, but the active vidar lane moved to ds4 on
`http://vidar:1236/v1`.

Probe attempt on 2026-05-12:

```text
curl --max-time 8 http://vidar:1235/v1/models
curl: (7) Failed to connect to vidar port 1235
```

Verdict: retired for current verification; use `vidar-ds4`.
