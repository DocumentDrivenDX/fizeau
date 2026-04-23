# Routing Override Precedence

Profiles supply defaults and constraints. Explicit pins narrow the candidate
set, but only inside the active profile's hard constraints.

## Rule

1. Explicit `Harness` pin wins within its allow-list. The `local` harness alias
   canonicalizes to `agent`.
2. Explicit `Model` pin wins within the chosen harness's allow-list. If the
   harness has a static allow-list and the model is absent, routing returns
   `ErrHarnessModelIncompatible`.
3. Explicit `Provider` pin wins within the chosen harness-model pair. In the
   current engine, provider is hard once `Harness` is also pinned; without a
   harness pin it is a provider-affinity preference because no harness-model
   pair has been fixed yet.
4. Profile supplies target model tier, provider preference, reasoning defaults,
   and placement preferences for any dimension not pinned.
5. If a pin violates a profile hard constraint, routing returns
   `ErrProfilePinConflict`; it does not silently substitute another harness,
   model, or provider.

## Worked Examples

1. `--profile default`

   No pin is present. The `default` profile supplies target `code-medium` and
   `local-first`; routing ranks eligible candidates using default/standard
   scoring, cost, locality, and name order.

2. `--profile smart --harness claude`

   The harness pin narrows routing to `claude`. Because `smart` is
   `subscription-first` with a subscription-only hard constraint and `claude` is
   a subscription harness, routing may choose `claude` if it is available, quota
   OK, and the resolved `code-high` model is allowed by that harness.

3. `--profile local --harness claude`

   This is contradictory. `local` is `local-only`, while `claude` is a
   non-local subscription harness. Routing returns `ErrProfilePinConflict` with
   `Profile=local`, `ConflictingPin=Harness=claude`, and
   `ProfileConstraint=local-only`.

4. `--profile smart --harness agent` with only local endpoints

   This is contradictory. `smart` has a subscription-only hard constraint, while
   `agent` is the local embedded harness. Routing returns
   `ErrProfilePinConflict` with `Profile=smart`,
   `ConflictingPin=Harness=agent`, and
   `ProfileConstraint=subscription-only`.

5. `--profile standard --model gpt-5.4`

   The exact model pin wins over the profile target tier. The profile still
   supplies `local-first` preference and all unpinned dimensions. If at least one
   eligible harness can serve `gpt-5.4`, routing ranks those candidates; if the
   model is only servable outside a hard profile constraint, the error is
   `ErrProfilePinConflict`.

6. `--profile standard --harness codex --model gpt-5.4`

   `codex` is selected first, then `gpt-5.4` must be in the `codex` allow-list.
   If it is present, routing evaluates only that harness-model path and applies
   profile preferences to remaining ties.

7. `--profile standard --harness codex --model sonnet-4.6`

   The harness pin wins, but the model must be valid for that harness. If the
   `codex` allow-list does not contain `sonnet-4.6`, routing returns
   `ErrHarnessModelIncompatible` rather than switching to `claude`.

8. `--profile cheap --harness agent --provider lmstudio --model qwen3.5-7b`

   The harness pin fixes `agent`; the model pin fixes `qwen3.5-7b`; the provider
   pin is hard inside that pair and rejects other configured agent providers.
   The `cheap` profile supplies local-first and economy-tier scoring, but cannot
   move the request away from the pinned tuple.

9. `--profile cheap --provider lmstudio`

   With no harness pin, provider is an affinity preference rather than a hard
   global filter. Matching `lmstudio` candidates receive a score bonus, but
   another candidate can still win if the profile score, gates, and tie-breaks
   rank it higher.

10. `--profile local --model opus-4.7`

    If `opus-4.7` is only servable by subscription harnesses in the current
    inputs, the model pin violates `local-only`. Routing returns
    `ErrProfilePinConflict` with `ConflictingPin=Model=opus-4.7`.

11. `--model gpt-5.4`

    With no profile, routing uses the exact model and the default
    `local-first` provider preference. No named profile hard constraint is
    applied.

12. `--profile does-not-exist`

    Unknown profiles are not aliases for `default`. The service returns
    `ErrUnknownProfile`.
