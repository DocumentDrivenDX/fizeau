# fizeau-probe-reasoning

**Profile:** sindri-llamacpp  
**Model:** `Qwen3.6-27B-UD-Q3_K_XL.gguf`  
**Endpoint:** `http://sindri:8020/v1`  
**Wire format:** `qwen`  
**Timestamp:** 2026-05-12T05:49:12Z

| Reasoning | finish_reason | reasoning_tokens | wall_time | think_hash | error |
|-----------|---------------|-----------------|-----------|------------|-------|
| `off` | stop | 0 | 4m10.448s | `—` | — |
| `low` | stop | 364 | 27.323s | `a2d5b416b52d617c` | — |
| `medium` | stop | 348 | 23.499s | `37fad692612b1ef3` | — |
| `high` | stop | 305 | 21.177s | `10d0d42790f44d12` | — |
| `4096` | stop | 287 | 21.166s | `e5d2c3630fbdb3c3` | — |
| `16384` | stop | 259 | 19.886s | `1dc25c7ca6299d2d` | — |

**Verdict:** recommended `reasoning_wire=effort`

> Named tiers vary but token budgets do not show clear proportional scaling. Use reasoning_wire=effort; investigate reasoning_wire=tokens if token precision is needed.
