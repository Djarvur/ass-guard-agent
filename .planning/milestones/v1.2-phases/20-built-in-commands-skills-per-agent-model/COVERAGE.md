# API Coverage — Phase 20 external surface (provider usage/billing endpoints)

> Full coverage by default. Opt-outs are explicit, reasoned decisions.
>
> Detector result at plan time: `detected: false` over the pre-plan scope (ROADMAP phase section +
> RESEARCH/CONTEXT). Authored anyway because the PLAN bodies reference D-07's "usage/billing
> endpoint" (detector vocabulary: `endpoint`, `wiring`) — this matrix keeps the seal-time
> `api-coverage.verify-pre` gate satisfiable with a decided record instead of a re-run surprise.
>
> Scope note: `github.com/fsnotify/fsnotify` v1.10.1 is a Go library (npm/pypi/crates gate does not
> apply; legitimacy audited in 20-RESEARCH.md §Package Legitimacy Audit — Approved). ACP
> `available_commands_update` / `user_message_chunk` frames are the product's own wire protocol
> (internal/acp), not an external API integration.

## External API: provider usage/billing endpoints (D-07 — /cost live leg)

| capability | decision | reason |
|---|---|---|
| usage_summary_query | INTEGRATE | D-07 locks endpoint-first /cost numbers: on-demand fetch at invocation under the FAST-CONTROL timeout class (~10s, 16-D-17) |
| per-provider capability declaration | INTEGRATE | providers without a queryable endpoint are config-declared "none" — a capability seam, never a runtime surprise (20-RESEARCH Open Question 1 / Assumption A1) |
| cost_amount_normalization | INTEGRATE | D-07 output carries amount + source note; rides modelrouting Target.Pricing currency units |
| billing_history_query | OPT-OUT | not needed — D-07 scopes /cost to current usage numbers; history retrieval is not a locked surface |
| push/auto_refresh stream | OPT-OUT | not needed — /cost is on-demand at invocation; any `usage_update` session/update emission is explicitly optional (planner discretion, RESEARCH §Wire Vocabulary) and out of locked scope |

Fallback contract (locked, not an opt-out): on endpoint absence/timeout/error the number is derived
from transcript usage × modelrouting cost table WITH a source note naming which produced it (D-07).
