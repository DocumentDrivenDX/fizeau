# AR-2026-04-26 — Agent vs Pi on OMLX Vidar Qwen3.6-27B-MLX-8bit

Status: in progress (sections 1–2 complete; sections 3–5 pending benchmark run).

Governing artifact: [ADR-006: Manual Overrides Are Auto-Routing Failure Signals](../helix/02-design/adr/ADR-006-overrides-as-routing-failure-signals.md).

This Action Research note pairs the native `agent` harness against the
external `pi` harness on an identical local-inference backend
(`vidar` / `Qwen3.6-27B-MLX-8bit`) to determine which harness should be the
default surface for OMLX-backed beadbench runs. The comparison answers a
specific routing question: when an operator manually overrides to local
inference, is the native agent capable of competing with pi on the same
model, or is the override actually signaling that pi is the better harness
for this class of work?

## 1. Methodology

### 1.1 Paired design

Each beadbench task in the harness-parity slice runs on **both** arms
defined in `scripts/beadbench/manifest-v1.json`:

- `agent-omlx-vidar-qwen36` — native agent harness, provider `vidar-omlx`,
  model `Qwen3.6-27B-MLX-8bit`, effort `medium`.
- `pi-omlx-vidar-qwen36` — pi harness, provider `vidar`, model
  `Qwen3.6-27B-MLX-8bit`, effort `medium`.

Both arms hit the same physical endpoint (the MLX server reachable as
`vidar` on the LAN), differing only in the harness wrapping the model. This
removes model and provider variance and isolates harness behavior:
prompt construction, tool calling, retry/loop policy, and verifier
integration.

### 1.2 Reviewer pin

Both arms use a pinned reviewer:

- `review_harness: codex`
- `review_model: gpt-5.5`

The reviewer **must not** be claude-family. Using a claude reviewer to
score a comparison that includes the native agent (which is itself
claude-driven on its non-OMLX paths) introduces a same-family confound.
Codex/gpt-5.5 is a third-party judge for both arms.

### 1.3 Sample size

- Minimum: **N ≥ 8** paired tasks (each task contributes one agent run +
  one pi run = one pair).
- Target: **N = 12** paired tasks, drawn from the `harness-parity` tier in
  the manifest plus eligible carry-over tasks from prior OMLX sweeps.

### 1.4 Win condition

The native agent "wins" the parity check and remains the default OMLX
harness if **both** of the following hold:

1. **Success rate:** agent paired-success rate ≥ pi paired-success rate
   (no regression on outcomes).
2. **Cost-per-success:** agent cost-per-success ≤ **1.2 ×** pi
   cost-per-success (modest indirection budget).

If either fails, the override-to-pi signal is treated as legitimate per
ADR-006 and pi becomes the recommended harness for OMLX-backed tasks.

### 1.5 Tiebreaker

When agent and pi land within a **5 %** margin on both metrics, prefer the
**native agent**. Rationale: lower indirection (one fewer process boundary,
one fewer config surface, one fewer failure mode) is intrinsically
valuable when outcomes are statistically indistinguishable.

### 1.6 Out of scope for this AR

This AR does **not** evaluate:

- Other providers (`grendel`, `bragi`, `hel`).
- Other Qwen variants (35B-A3B, Coder-Next, etc.).
- Reasoning-budget sweeps (covered by
  [beadbench-omlx-qwen-reasoning-sweep-2026-04-24.md](beadbench-omlx-qwen-reasoning-sweep-2026-04-24.md)).
- Frontier-model comparisons.

## 2. Pi configuration evidence

### 2.1 Configuration fix that unblocked this AR

Prior to 2026-04-26, pi could not reach vidar because of two defects in
its `models.json` / provider config:

1. A trailing-comma JSON syntax error that prevented pi from loading the
   provider list at all.
2. Vidar's API was registered as `openai-completions`, but the MLX server
   on `vidar:1235` speaks the `anthropic-messages` protocol.

The fix landed on 2026-04-26 (see commit `5243c40 chore: unblock
harness-parity bead — pi configured for omlx vidar`):

- Removed the trailing comma.
- Switched the vidar entry to `api: anthropic-messages` with
  `baseUrl: http://vidar:1235`.

### 2.2 `pi --list-models` snapshot for vidar

Captured 2026-04-26 from the worktree host:

```
provider           model                                         context  max-out  thinking  images
vidar              gpt-oss-20b-MXFP4-Q8                          128K     16.4K    no        no
vidar              MiniMax-M2.5-MLX-4bit                         128K     16.4K    no        no
vidar              Qwen3-Coder-Next-MLX-4bit                     128K     16.4K    no        no
vidar              Qwen3.6-27B-MLX-8bit                          128K     16.4K    no        no
vidar              Qwen3.6-35B-A3B-4bit                          128K     16.4K    no        no
vidar              Qwen3.6-35B-A3B-nvfp4                         128K     16.4K    no        no
```

The arm under test (`Qwen3.6-27B-MLX-8bit`) is present and reachable
through pi's provider table.

### 2.3 Verified smoke test

Command:

```bash
pi --provider vidar --model Qwen3.6-27B-MLX-8bit -p 'In one sentence: what model are you?'
```

Output (captured 2026-04-26):

```
I'm the language model running inside the pi coding agent harness — I don't
have access to my own model name or version from within this environment.
```

The endpoint returned a coherent response over the
`anthropic-messages` transport, confirming that the post-fix pi
configuration successfully routes to the vidar MLX server. (The model's
inability to introspect its own name is expected — MLX does not surface
that metadata to the chat layer — and is not relevant to this AR.)

A second smoke test confirmed responsiveness on a trivial task:

```bash
pi --provider vidar --model Qwen3.6-27B-MLX-8bit -p 'Reply with the single word: pong'
# → pong
```

### 2.4 Catalog deltas vs. internal model catalog

Pi's vidar entry exposes the same six MLX models that the internal
provider catalog lists for the vidar endpoint. No models are missing on
the pi side, and pi does not advertise vidar models that the internal
catalog lacks. Context (128K) and max-output (16.4K) values match.
Pi reports `thinking: no` for all vidar models; the native agent's
internal catalog tracks the same. No catalog reconciliation is required
before running the paired benchmark.

## 3. Per-task pairwise outcome table

> **TBD — filled by agent-tbd-3 (child 3 of the harness-parity split).**
>
> This section will contain one row per task, with columns:
> task id, agent outcome, agent cost, agent wall-clock, pi outcome,
> pi cost, pi wall-clock, pair winner.

## 4. Aggregate metrics and winner declaration

> **TBD — filled by agent-tbd-3.**
>
> Aggregate paired-success rate, cost-per-success, and wall-clock-per-success
> for each arm. Apply the win condition from §1.4 and the tiebreaker from
> §1.5 to declare a winner.

## 5. Top-3 gaps

> **TBD — filled by agent-tbd-3 after data analysis.**
>
> The three highest-impact behavioral or routing gaps observed between the
> two arms, with concrete remediation pointers (file paths, ADRs, or
> follow-up beads).
