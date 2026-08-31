---
phase: 16
slug: acp-wire-foundation
status: verified
threats_open: 0
asvs_level: 1
created: 2026-09-01
---

# Phase 16 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| agent→editor stdout | session/update frames cross to the untrusted-slow client; ordering and bounded memory are the contract | protocol frames |
| bus→emitter | concurrent producers (turn, subagents, engine) share the lanes | protocol frames |
| provider→transcript | raw provider thinking bytes land on disk unredacted by design (D-23) | provider thinking bytes |
| disk→replay | transcript files may contain kinds a given binary does not know | transcript lines |
| client→registry (inbound responses) | untrusted frames claim to answer our requests; ids are the only authenticity | JSON-RPC responses |
| registry→client (outbound asks) | probe/ask payloads must not leak secrets or wedge on hostile clients | probe/ask payloads |
| editor-driven writes → operator config files | values arriving over the wire mutate files the operator owns | config values |
| editor→config surface | untrusted option ids/values arrive over the wire and target operator files | option id/value |
| _meta blob (initialize) → blobFills overlay | client-supplied defaults, in-memory only | model fills |
| config layers → runtime default model selection | the tier/time-window table steers which model answers turns | tier table |
| simulator→serve | the scripted client is a stand-in for a hostile-capable editor | scripted frames |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-16-01 | DoS | chatty emitters vs slow client | high | mitigate | Bounded lanes block producers (D-01); 256-slot Writer final boundary; stall detector logs loudly (D-03) — `internal/acp/metrics.go`, `TestTurnEmitterStall` | closed |
| T-16-02 | DoS | deadlock via lock-held enqueue | medium | mitigate | No lock spans an enqueue; -race stress suite (12 hits) exercises exactly this | closed |
| T-16-03 | Tampering | priority inversion through shared Writer buffer | medium | mitigate | Emitter drain is sole notification producer into Writer (≤1 in flight), pinned by tracer written-count assertions | closed |
| T-16-04 | Information Disclosure | raw_thinking lines on disk | high | mitigate | Type-scoped unredacted path (raw_thinking only, 26 refs); file perms 0600 (`filePermOwner`, 16 refs); zero-calls test pins boundary | closed |
| T-16-05 | Tampering | transcript replay with unknown kinds | medium | mitigate | Tolerant parse — unknown kinds/fields preserved in discriminators, never executed (15 refs); fixture-pinned (D-20) | closed |
| T-16-06 | Spoofing | response id collision / fabrication | high | mitigate | UUID v4 ids both directions (48 refs); unknown-id logged and dropped; exact-match map lookup | closed |
| T-16-07 | DoS | hostile client withholds responses | high | mitigate | D-14 ladder bounds every Call (one retry, then fallback); FAST-CONTROL ~10s bounds probes; shutdown drain releases waiters | closed |
| T-16-08 | Information Disclosure | probe payloads | medium | mitigate | Probe payload is minimal static form — no session content, no paths, no config values; stderr carries ids/methods only | closed |
| T-16-09 | Repudiation | capability degradation decisions | low | mitigate | Sticky cache plus structured logs record advertisement source vs probe outcome (D-16 counters, `server.go`) | closed |
| T-16-10 | Tampering | config file corruption on failed write | high | mitigate | Temp+rename atomic replace (5 refs); corrupt-layer and rename-failure tests prove original survives (D-07) | closed |
| T-16-11 | Elevation | loosened file permissions | medium | mitigate | Hard 0600 target, 0750 max dirs; `WarnLooseConfigPerm` police reads (5 refs) | closed |
| T-16-12 | Tampering | value injection into arbitrary config keys | medium | mitigate | Primitive only; 16-05 whitelists writable key set at the wire (cross-ref T-16-13) | closed |
| T-16-13 | Elevation | arbitrary config-key injection via optionId | high | mitigate | Menu-whitelist validation before any write (D-09); `_global/` prefix only on whitelisted ids; `validateSettableLocked` (3 refs) unreachable with arbitrary keyPath | closed |
| T-16-14 | Tampering | blob overriding operator config | high | mitigate | D-10 fills-unset in-memory only, never persisted (`blobFills`, 12 refs); explicit-file-wins asserted both directions | closed |
| T-16-15 | Information Disclosure | option values leaking config secrets | high | mitigate | Menu fixed to non-credential options; API keys env/file-only — no code path reads/writes credential fields | closed |
| T-16-16 | Tampering | half-applied live state | medium | mitigate | Persist-then-apply ordering with typed errors, unchanged state on failure (D-07); serialized apply | closed |
| T-16-17 | DoS | latent deadlocks/leaks invisible at ci scale | medium | mitigate | Adversarial emitter soak (`emitter-soak` task, `.mise.toml:49`); closing invariants: no leak, no drop, clean close, stall-detector-fired | closed |
| T-16-18 | Repudiation | live-editor claims asserted without an operator | medium | mitigate | Blocking checkpoint + two-valued disposition markers — unconfirmed claims cannot be recorded silently (UAT completed with operator, 2026-08-31) | closed |
| T-16-07-01 | DoS | TurnEmitter.Barrier wake | high | mitigate | Per-generation broadcast wake (CR-01 fix); `TestTurnEmitterBarrierConcurrentWaiters`, no ctx escape | closed |
| T-16-07-02 | DoS | Barrier wake rewrite | medium | mitigate | Waiters park on channel receive, never spin; snapshot-under-mu re-parks on fresh generation; -race suite + soak re-runnable | closed |
| T-16-08-01 | Tampering | Set idempotence basis | medium | mitigate | Scope-aware basis: global writes no longer dropped; project scope keeps combined comparison (D-10) | closed |
| T-16-08-02 | Tampering | _global twin display | medium | mitigate | Layer-true resolution (D-11); `TestConfigSurface_GlobalTwinsAdvertiseGlobalLayer` | closed |
| T-16-08-03 | Tampering | global-scoped value validation | low | mitigate | D-09 typed reject: values validated before scope routing; no new write surface | closed |
| T-16-09-01 | Tampering | defaultTurnModel config-driven default | medium | mitigate | Default rides the same heavy-tier resolution that built the session provider; static-binding fallback mirrors `resolveModelLocked` (8 refs) | closed |
| T-16-09-03 | Denial of Service | resolver error paths | low | mitigate | Every decline path falls down the documented ladder (resolver → static binding → profile slug); never a failed turn | closed |
| T-16-SC | Tampering | npm/pip/cargo installs (plans 16-01…16-06) | high | accept | Zero package installs — stdlib + existing deps only (yaml.v3); legitimacy gate not applicable | closed |
| T-16-07-03 | DoS | lane overflow behavior | low | accept | Unchanged: D-01 block-never-drop + D-03 stall detector already cover writer-wedge stalls | closed |
| T-16-07-SC | Tampering | package installs (plan 16-07) | low | accept | None; stdlib sync only | closed |
| T-16-08-04 | Information Disclosure | structured log lines | low | accept | Menu vocabulary carries no credential options; log lines name option ids/values of that vocabulary only | closed |
| T-16-08-SC | Tampering | package installs (plan 16-08) | low | accept | None; existing go.mod deps only | closed |
| T-16-09-02 | Repudiation | transcript model attribution | low | accept | Transcript keeps recording actual stamped model; attribution equals chain-resolved value | closed |
| T-16-09-SC | Tampering | package installs (plan 16-09) | low | accept | None; existing modelrouting dep only | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-16-01 | T-16-SC (×6, plans 01–06) | Zero package-manager installs this phase — stdlib + existing deps (yaml.v3, modelrouting, providerfactory); npm/pip/cargo legitimacy gate not applicable | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |
| AR-16-02 | T-16-07-03 | Lane overflow behavior unchanged by Barrier plan; D-01 block-never-drop + D-03 stall detector cover writer-wedge stalls | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |
| AR-16-03 | T-16-07-SC | No package installs in Barrier plan (stdlib sync only) | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |
| AR-16-04 | T-16-08-04 | Structured log lines name only the non-credential menu vocabulary | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |
| AR-16-05 | T-16-08-SC | No package installs in config-surface fix plan (existing go.mod deps only) | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |
| AR-16-06 | T-16-09-02 | Transcript model attribution now always equals chain-resolved value — auditability improved, no residual risk | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |
| AR-16-07 | T-16-09-SC | No package installs in default-model plan (existing modelrouting dep only) | Plan-time disposition (GSD secure-phase audit) | 2026-09-01 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-09-01 | 37 | 37 | 0 | GSD secure-phase (L1 grep verification, register authored at plan time) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-09-01
