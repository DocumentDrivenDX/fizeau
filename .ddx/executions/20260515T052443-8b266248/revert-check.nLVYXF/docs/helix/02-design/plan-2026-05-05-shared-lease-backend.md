# Design Note: Shared Lease Backend for Sticky Route Leases

**Date**: 2026-05-05
**Status**: Proposed
**Scope**: Multi-machine sticky routing only; no backend implementation in this note

## Context

ADR-005 and FEAT-004 already establish that sticky route leases preserve
worker affinity and that server metrics are advisory. This note makes the
cross-process contract explicit so the routing docs do not have to repeat the
backend semantics in multiple places.

The key operational fact is that server metrics are sampled observations of a
single endpoint. They can help rank equivalent endpoints, but they cannot be
the authority for stickiness across processes:

- two processes can observe the same endpoint at different moments and make
  conflicting decisions
- a healthy-looking metric sample can race with a new request, so load derived
  only from metrics is stale as soon as it is read
- a process-local view cannot prevent two workers on different machines from
  assigning the same sticky key to different endpoints

For single-machine deployments, in-process route leases remain authoritative.
For multi-machine deployments, the system needs a shared lease backend so the
same sticky key resolves to one live owner across processes.

## Lease Record

A shared sticky lease record must include:

- `sticky_key` - the validated request sequence identity, normally the
  correlation ID or equivalent worker/session sequence key
- `provider` - the provider source or type
- `endpoint` - the concrete endpoint identity or selector
- `model` - the resolved concrete model string
- `owner` - the owning service instance or worker identity
- `expires_at` - the lease deadline
- `refreshed_at` - the last successful refresh timestamp

Recommended additional fields:

- `lease_token` - an opaque value returned on acquire and required for
  refresh/release
- `generation` - a monotonic compare-and-swap value for optimistic updates
- `reason` - optional diagnostic text for invalidation or release

The record is keyed by sticky route identity, not by model alone. That lets the
same model exist on multiple equivalent endpoints while preserving affinity for
one long-running worker.

## Atomic Semantics

The backend contract is intentionally small:

### Acquire

- Acquire succeeds only if no unexpired lease exists for the same
  `sticky_key`, or if the caller is replacing its own expired lease with a new
  token.
- Acquire writes the full lease record atomically and returns the lease token
  plus the new expiry.
- If another owner already holds an unexpired lease, acquire fails without
  partially updating the record.

### Refresh

- Refresh succeeds only when the caller presents the current lease token for
  the active owner.
- Refresh extends `expires_at` atomically and updates `refreshed_at`.
- A refresh after expiry is a miss, not a resurrection. The caller must
  acquire again.

### Release

- Release succeeds only when the caller owns the current lease token.
- Release is idempotent: releasing an already-missing lease is a no-op.
- Release removes the record or marks it vacant so a later acquire can claim it
  without ambiguity.

### Invalidation

- If the selected endpoint stops advertising the resolved model, the lease may
  be released early or allowed to expire, but a new acquire must not reuse it
  blindly.
- If the backend observes a conflicting owner, the stale lease loses on the
  next acquire or refresh attempt.

## Backend Choice

This note intentionally defers the concrete backend choice.

Either Redis or Postgres can satisfy the lease contract:

- Redis fits a short-lived lease with `SET ... NX PX`, compare-and-delete
  release, and token-checked refresh
- Postgres fits the same contract with row-level transactions and a unique key
  on `sticky_key`

The implementation bead should choose one backend only when the project is
ready to commit to operational tradeoffs such as deployment simplicity,
consistency guarantees, and observability expectations.

## Failure Behavior

If the shared lease backend is unavailable:

- single-machine routing continues with in-process leases only
- multi-machine routing treats cross-process stickiness as degraded rather
  than pretending it is authoritative
- new sticky assignments fall back to the best local process view, but the
  routing evidence must report that the shared backend was unavailable
- no endpoint should be marked unavailable solely because the shared lease
  backend is down

This preserves request progress while making the stickiness loss explicit. The
system is allowed to route, but it must not claim a cross-process lease it
cannot actually enforce.

## Follow-Up

If a concrete backend is selected, create implementation beads for:

- backend client and lease store
- atomic acquire/refresh/release paths
- routing integration and evidence reporting
- failure-mode tests for unavailable backend and lease conflicts

