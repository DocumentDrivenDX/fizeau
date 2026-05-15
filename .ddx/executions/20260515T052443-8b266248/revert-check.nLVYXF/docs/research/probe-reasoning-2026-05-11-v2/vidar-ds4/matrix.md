# fizeau-probe-reasoning

**Profile:** vidar-ds4  
**Model:** `deepseek-v4-flash`  
**Endpoint:** `http://vidar:1236/v1`  
**Wire format:** `openai_effort`  
**Timestamp:** 2026-05-11T22:45:33Z

| Reasoning | finish_reason | reasoning_tokens | approx | wire |
|-----------|---------------|------------------|--------|------|
| `off` | stop | 0 | false | `think:false` |
| `low` | stop | 57 | true | `reasoning_effort:"low"` |
| `medium` | stop | 75 | true | `reasoning_effort:"medium"` |
| `high` | stop | 65 | true | `reasoning_effort:"high"` |
| `4096` | stop | 75 | true | `reasoning_effort:"medium"` |
| `16384` | stop | 63 | true | `reasoning_effort:"high"` |

**Verdict:** recommended `reasoning_wire=effort`

> Named tiers vary but token budgets do not show clear proportional scaling. Use reasoning_wire=effort; investigate reasoning_wire=tokens if token precision is needed.
