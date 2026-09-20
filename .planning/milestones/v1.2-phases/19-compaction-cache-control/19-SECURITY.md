---
phase: "19"
slug: "compaction-cache-control"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: "2026-09-10"
---

# Phase 19 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| profile files → wire | operator-local trusted config becomes request-shaping flags (cache_control) | yaml scalars (trusted) |
| provider API → availability | breakpoint over-placement returns 400; non-2xx bodies parsed at the check site | HTTP error bodies (externally controlled) |
| provider endpoint → error path | the HTTP error body is externally-controlled input parsed at the new check site | bounded JSON envelope ≤8 KiB |
| transcript file → projector | on-disk lines (incl. marker summaries — model output over tool content) feed the model-visible window | JSONL lines (untrusted-at-rest) |
| marker payload → future requests | the summary becomes every later request's seed until the next compaction | model-generated text |
| tool-result content → summarizer input → durable seed | untrusted repo/file content shapes the summary that becomes every future request's seed | redacted transcript text |
| provider error text → retry decision | the overflow matcher gates a forced compaction + resend | error message strings |
| editor settings → config layers → session behavior | client-pushed values reach operator config files and the running engine | scalar config values over ACP |
| transcript (disk) → Projector (armed carve-out) | kill-9 replay, hand-crafted transcripts — the 19-06/19-07 tamper-safety gate | JSONL lines (untrusted-at-rest) |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-19-01 | Tampering | profiles/zcode/profile.yaml | low | accept | Operator-local trusted artifact (same trust class as model name / max_tokens) | closed |
| T-19-02 | DoS | shaper emission vs 4-breakpoint API cap | medium | mitigate | keep-last-4 over-cap degrade — internal/shaper/shaper.go:29,155 (maxCacheBreakpoints=4; pinned 3/4/5/6) | closed |
| T-19-03 | Tampering | error-body parser | medium | mitigate | Bounded read io.LimitReader 8 KiB (streaming.go:275,296); malformed bodies degrade to generic structural error | closed |
| T-19-04 | DoS | overflow matcher abused to force compaction loops | low | mitigate | Matcher only classifies; retry consumer bounded one attempt/turn (session.go:613,691-692) | closed |
| T-19-05 | Tampering | summary as durable seed (injection via tool output) | medium | mitigate | Marker rides the redacted append path (manager.go:393 AppendCompaction); extractive prompt; seed is data in a user-role message, never executed | closed |
| T-19-06 | Information Disclosure | secrets from earlier turns persisting in summary | medium | mitigate | Summarizer input is already-redacted transcript content; append-side redactor applies to the marker line | closed |
| T-19-07 | DoS | marker summary bloats every future request | low | mitigate | max_tokens 2048 cap + budget-fill accounting shrinks the tail when the summary is large (SetCompactionTailBudget) | closed |
| T-19-08 | Tampering | prompt injection steering the durable seed | medium | mitigate | Extractive summary prompt (compaction.go:426); redacted input; 2048 hard cap contains DoS | closed |
| T-19-09 | Information Disclosure | summary persisting secrets | medium | mitigate | Redacted-transcript input only; marker append on the redacted chokepoint | closed |
| T-19-10 | DoS | oversized summary bloating requests | medium | mitigate | CompactionSummaryMaxTokens=2048 (compaction.go:37); fill accounting; usage snapshot auditable in /cost | closed |
| T-19-11 | DoS | retry loops on persistent overflow | low | mitigate | overflowRetried once-per-turn guard (session.go:613,691); fail-twice test pinned; re-proven live in uat-harness-run-20260910 | closed |
| T-19-12 | Tampering | editor-pushed config values | medium | mitigate | Typed validation-first rejection, 1..100 bounds (config_surface.go:75-76); scalars only, no expansion surface | closed |
| T-19-13 | Tampering | value injection into arbitrary config keys | medium | mitigate | Writable key set whitelisted to menu ids (selectValues); 16-05 T-16-12 boundary extended, not reopened | closed |
| T-19-14 | DoS | threshold pushed to 1 forcing compaction every turn | low | mitigate | Each compaction costs one rate-visible summarizer call; operator resets via the same menu; not a crash/loop vector | closed |
| T-19-15 | Tampering | crafted transcript carrying a same-turn marker | high | mitigate | Carve-out requires the ENGINE-ARMED in-memory override (projector.go:95,116 SetRetryCompactedTurn), never transcript content; not-armed byte-identity pins | closed |
| T-19-16 | DoS | unbounded summarize input re-sent near-limit ×64/turn | high | mitigate | WR-03a span budget (compaction.go:49-65,290 spanBudgetChars) + WR-03b once-per-turn guard (compaction.go:219-223); forced-retry stamps the turn (session.go:698) | closed |
| T-19-17 | DoS | overflow retry storms | medium | mitigate | overflowRetried semantics unchanged and re-pinned — exactly one retry, no second | closed |
| T-19-18 | Information disclosure | span truncation drops oldest un-summarized content | low | accept | Loud truncation notice; chained compactions absorb prior summaries; strictly better than guaranteed-400 with no compaction | closed |
| T-19-SC | Tampering | npm/pip/cargo installs (plans 01-06) | high | accept | Zero new dependencies (19-01..19-06 legitimacy audits; SDK pinned v1.63.0, otherwise stdlib) | closed |
| T-19-07-01 | Tampering | Projector.Project armed carve-out branch | high | mitigate | Precedence flip keeps the engine gate: non-empty EXACT retryCompactedTurn == turnID match required (19-07 pins: not-armed DeepEqual, subagent-safe) | closed |
| T-19-07-02 | Tampering | Test fixtures (projector_test/compaction_test) | medium | mitigate | 19-07 Task 3 fixture-integrity audit — deletions confined to the deliberately inverted subtest | closed |
| T-19-07-SC | Tampering | package installs (19-07) | low | accept | Pure internal Go change; no package-manager installs | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-19-1 | T-19-01 | Operator-local trusted profile artifact; no untrusted input reaches the flag | plan 19-01 | 2026-09-06 |
| AR-19-2 | T-19-SC, T-19-07-SC | Zero package installs across all seven plans (legitimacy audits in each PLAN) | plan 19-01..19-07 | 2026-09-06 |
| AR-19-3 | T-19-18 | Bounded + announced truncation beats guaranteed-400 unbounded summarize | plan 19-06 | 2026-09-07 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-09-10 | 22 | 22 | 0 | gsd-manager verify:post (L1 grep-depth, ASVS 1; register authored at plan time — short-circuit per secure-phase.md Step 3) |

Additional live evidence 2026-09-10: the auto-UAT harness (uat-harness-run-20260910/) exercised the overflow-retry and once-per-turn guards end-to-end against a mock provider on a HEAD build — one drift threshold-compact + one forced compact per turn, retry projected post-marker, no loops.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-09-10
