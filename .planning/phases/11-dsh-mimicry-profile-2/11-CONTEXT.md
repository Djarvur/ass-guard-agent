# Phase 11: dsh Mimicry Profile #2 - Context

**Gathered:** 2026-08-14
**Status:** Ready for planning

<domain>
## Phase Boundary

DeepSeek-model turns through ass-guard are structurally indistinguishable from deepseek-harness (dsh) — captured from real wire traffic (recording proxy + zstd session-log harvest), riding the N-profile architecture with no target-specific code paths. The cross-harness mimicry thesis validated with a second profile. This is the roadmap's highest-research-depth phase (`--research-phase 11` recommended: recording-proxy runbook, zstd decode, system-message mapping form, DeepSeek dialect quirks — all answerable only from the capture).

</domain>

<decisions>
## Implementation Decisions

### Live endpoint target
- **D-01:** The live DeepSeek probe targets the **opencode subscription's gateway** — the operator's actual access path (MiniMax + DeepSeek via the opencode subscription, recorded at v1.0 close). Prerequisite verified during the phase: the gateway must expose an OpenAI-compatible endpoint (base URL + credential wired through the Phase-7 `ProviderConfig` machinery; scheduling.yaml's DeepSeek provider entry points there). DeepSeek's official API remains a config-swappable alternative — nothing hardcodes the gateway. *[User-selected.]*

### Profile lifecycle (dsh is developer-preview)
- **D-02:** **Pin + drift guard**: one capture against a pinned dsh commit (recorded in profile meta); `profile check dsh` guards against silent divergence; re-capture is a documented manual procedure run when dsh moves — NOT a scheduled ritual. *[User-selected.]*

### Parity bar
- **D-03:** **Full behavioral bar** — same standard as zcode: tool-call sequences through ass-guard-with-dsh-profile statistically indistinguishable from live dsh (the A/B parity harness, `internal/parity`). The cross-harness thesis gets proven, not assumed. *[User-selected.]*

### Profile selection (refines DSH-04)
- **D-04:** **The profile is specified per-model in config** — model entries in `scheduling.yaml` declare their profile (DeepSeek model entries specify `dsh`; GLM entries keep `zcode`). The explicit `--profile` flag remains as an override; per-turn switching stays a v1.2 non-goal. Requires a small schema addition (per-model `profile` field) riding the Phase-7 provider/model declaration machinery. *[User-selected — freeform: "profile is cpecified per-model in config".]*

### Claude's Discretion
- Recording-proxy implementation shape (local reverse proxy at a configured baseURL; dev-time tooling, not shipped runtime)
- zstd harvest tooling details (after the decode spike)
- The dsh capture workload's exact prompt sequence (mirrors the Phase-9 runbook discipline: divergence-prone)
- Redaction preserved-header genericization mechanics (profile-supplied lists)
- `strict: true` and DeepSeek dialect handling keyed by capability profile, never `if provider == "deepseek"`

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### v1.1 research (ground truth, 2026-08-14)
- `.planning/research/SUMMARY.md` — Phase-11 section (highest research depth; verification-gated)
- `.planning/research/STACK.md` — dsh repo layout (pnpm monorepo), wire shape from `llm-deepseek/serialize.ts` (OpenAI chat-completions style, always stream+include_usage, no tool_choice ever, `x-deepseek-harness-user-id` header), session JSONL = events-not-requests (zstd frames), klauspost/compress v1.19.2
- `.planning/research/PITFALLS.md` — Pitfalls 14-16 (dsh churn, extraction traps: source-reading is a guess wearing a costume; wire protocol from capture not docs; zcode-isms in shared code)

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"dsh Mimicry Profile #2" — DSH-01..05 (DSH-04 amended: per-model profile specification in config)
- `.planning/ROADMAP.md` §"Phase 11" — goal, 5 success criteria, phase gate (mise ci + zstd spike on real artifact + A/B parity green + live DeepSeek round-trip + full traceability)

### Prior decisions that bind
- Phase 9 context — capture-provenance pattern (recording-proxy wire truth, pinned-session discipline) made profile-agnostic; this phase generalizes it
- Phase 7 (v1.0) — `ProviderConfig`/`ResolveCredential`/`ProviderFactory`: the per-model `profile` field and the gateway provider entry ride this machinery
- v1.0 Phase 1 — the A/B parity methodology DSH's full behavioral bar reuses
- STATE.md decision log (2026-08-14) — opencode excluded as mimicry target; opencode subscription serves MiniMax + DeepSeek (the D-01 access path)

### Code ground truth
- `internal/profile/` — the profile-agnostic loader; `cmd/extract-profile` gains a dsh mode; `profile_check.go`'s hardcoded zcode path gets per-profile capture loaders
- `internal/provider/openai.go` — `buildRequest` silently drops `profile.System` (the DSH-02 fix site)
- `internal/scheduler/` — per-model `profile` field schema addition; gateway provider entry
- `internal/redact/` — the zcode-ism audit's first target (preserved-header list becomes profile-supplied)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/profile` + `internal/parity` — the N-profile architecture and A/B harness (profile #2 is data, not code)
- `internal/provider` openai-shape adapter — dsh's wire maps onto it (after the `profile.System` fix)
- Phase-7 provider/model config machinery — per-model profile field + gateway endpoint ride it
- Phase-9's capture-provenance pattern — generalized to dsh

### Established Patterns
- Captured-not-written profiles (the north star generalized: recording proxy = wire truth)
- Pinned-capture + drift-check discipline (zcode pattern extended per-profile)
- Capability-keyed dialect handling, never provider-name-keyed

### Integration Points
- `profiles/dsh/` bundle + sanitized seed in `internal/defaults` embed
- `scheduling.yaml` schema: per-model `profile` field; DeepSeek provider entry (opencode gateway baseURL)
- `cmd/extract-profile` dsh mode; `profile check dsh` per-profile capture loader

</code_context>

<specifics>
## Specific Ideas

- User's framing: the profile belongs to the model, not the invocation — config says which profile each model speaks, so DeepSeek turns are dsh-shaped wherever they're routed from.
- The gateway is the probe target because it's the real access path; everything stays base-URL-swappable regardless.

</specifics>

<deferred>
## Deferred Ideas

- Per-turn profile switching — v1.2 non-goal (locked)
- dsh Responses-API wire shape — only if verification demands it
- Scheduled re-capture automation — manual procedure per D-02; revisit if dsh stabilizes
- UI/plugin-runtime mimicry of dsh (only the model-wire surface is mimicked) — permanent non-goal

</deferred>

---

*Phase: 11-dsh Mimicry Profile #2*
*Context gathered: 2026-08-14*
