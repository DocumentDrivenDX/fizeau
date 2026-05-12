# fizeau-probe-reasoning

**Profile:** fiz-openrouter-qwen3-6-27b  
**Model:** `qwen/qwen3.6-27b`  
**Endpoint:** `https://openrouter.ai/api/v1`  
**Wire format:** `openrouter`  
**Timestamp:** 2026-05-12T05:43:47Z

| Reasoning | finish_reason | reasoning_tokens | wall_time | think_hash | error |
|-----------|---------------|-----------------|-----------|------------|-------|
| `off` |  | 0 | 925ms | `—` | — |
| `low` | stop | 367 | 1.116s | `—` | — |
| `medium` | length | 515 | 360ms | `—` | — |
| `high` | stop | 277 | 871ms | `—` | — |
| `4096` |  | 0 | 751ms | `—` | — |
| `16384` | stop | 383 | 1.131s | `—` | — |

**Verdict:** recommended `reasoning_wire=effort`

> Named tiers vary but token budgets do not show clear proportional scaling. Use reasoning_wire=effort; investigate reasoning_wire=tokens if token precision is needed.
