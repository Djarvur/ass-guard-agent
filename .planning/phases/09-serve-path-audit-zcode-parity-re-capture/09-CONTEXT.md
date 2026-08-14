# Phase 9: Serve-Path Audit + zcode Parity Re-capture - Context

**Gathered:** 2026-08-14
**Status:** Ready for planning

<domain>
## Phase Boundary

Every `acp serve` session leaves a redacted, bounded audit trail that also records the engine's decisions (why it continued / ran a hook / asked / waited), and the zcode parity stability test is re-grounded on a newly pinned, divergence-prone capture session — closing v1.0's LOG-01 serve-path gap and the Phase-1 data-source carry-forward. Merged phase (audit + re-capture are adjacent in the operator's priority chain); lands the capturer factory seam in its final home BEFORE Phase 10's `internal/runtime` extraction moves the wiring — written once, not re-wired.

</domain>

<decisions>
## Implementation Decisions

### Audit verbosity & storage
- **D-01:** Audit events are **metadata-only by default** — correlation IDs (session/turn/request), shapes, and content hashes; full redacted bodies go to the **capped body store**, retrievable by hash when debugging. No inline bodies in events (~80 KB/turn × hands-off multiplication = unbounded growth otherwise). *[User-selected.]*

### Audit file layout
- **D-02:** The audit mirror lives as **per-session JSONL files under `.ass-guard/audit/`** — aligns with the per-session transcript model and gives natural retention/pruning boundaries per session. `--audit-log` accepts an operator override path; the default is per-session. 0600, append-only, rejects stdout as a target (locked by AUD-02). *[User-selected.]*

### Engine-decision events
- **D-03:** Each engine-decision event carries **full provenance**: action (continue/hook/ask/wait) + matched signal (which pattern/tool-call) + the matched text span + source turn + config source (which table entry fired). A developer answers "why did it continue" from the log alone, no transcript cross-referencing required. *[User-selected — the AUD-04 differentiator.]*

### zcode parity re-capture
- **D-04:** The re-capture workload is **scripted and divergence-prone** — subagent fan-outs, MCP attach/detach, tool variety — run once by the operator in real zcode; that session pins the stability test. NOT natural-usage + richest-session selection (research Pitfall 17: `PickRichestMain` is selection-biased toward a vacuous stability proof). zcode + extractor versions recorded; thresholds explicitly re-baselined; drift report committed before any profile update (locked by AUD-05). *[User-selected.]*

### Claude's Discretion
- The `CurrentTurnID()` accessor vs empty-TurnID fallback (research gap — decide in planning; empty-TurnID with append-ordering is the acceptable v1.1 fallback)
- Captured-provider helper shape (generalizing `tracerProvider`; the OpenAI side already has `WithOpenAIRequestCapture`); body-store cap size and eviction policy
- Audit sink rotation/pruning mechanics beyond per-session files
- The scripted workload's exact prompt sequence (runbook design from research Pitfalls 17/18, written verbatim into the plan)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### v1.1 research (ground truth, 2026-08-14)
- `.planning/research/SUMMARY.md` — Phase-9 section (capturer seam before runtime extraction; write-once rationale)
- `.planning/research/ARCHITECTURE.md` §audit — tracerProvider/`WithOpenAIRequestCapture`/TranscriptWriter (zero non-test callers today), redaction-existing findings
- `.planning/research/PITFALLS.md` — Pitfall 8 (divergent-audit anti-pattern; copy-`tracerProvider` banned), Pitfall 9 (header-capture redaction), Pitfalls 10 (growth/body_ref), 17-18 (re-capture rigor: selection bias, version recording)

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"Audit & Parity Re-capture" — AUD-01..05
- `.planning/ROADMAP.md` §"Phase 9" — goal, 5 success criteria, phase gate (mise ci + live-serve redacted-audit verification + stability test green + token/secret canary greps)

### Prior decisions that bind
- v1.0 Phase 7 decisions (STATE.md log): factory wiring, `(*ProviderFactory).Endpoint` accessor added for the LOG-01 tracer capturer, `tracerProvider` reconstruction path — this phase refactors those onto the single factory seam
- v1.0 Phase 2 D-03/D-20: transcript = human-investigation artifact, one-writer — the audit trail complements, never duplicates, the transcript writer
- `.planning/phases/08-slash-command-kickoff/08-CONTEXT.md` — Phase 8 precedes; AUD-04's engine-decision events observe Phase-8-hardened turns

### Code ground truth
- `cmd/ass-guard/main.go` — `tracerProvider` (the reconstruction to delete), `openAuditSink` usage
- `internal/provider` — both shapes' capture hooks (`WithOpenAIRequestCapture`; the Anthropic-side capturer)
- `internal/redact` — existing redaction (nothing to build; extend patterns only if canaries fail)
- `internal/profile/stability_test.go` — the test the pinned session unblocks (currently globs the absent `eea3dc48` capture)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/audit` + `openAuditSink` — 0600/stdout-rejecting sink already exists (v1.0)
- `internal/redact` — full redaction machinery (Bearer/sk-/UUID/URL patterns)
- `(*ProviderFactory).Endpoint` accessor — added in Phase 7 explicitly for the LOG-01 capturer
- `internal/provider` OpenAI `WithOpenAIRequestCapture` — the OpenAI-shape capture hook already exists; generalize the pattern to both shapes via the factory seam
- `internal/parity` — the A/B harness the re-baselined thresholds feed

### Established Patterns
- Single-sourced construction (the copy-`tracerProvider` shortcut is banned — Pitfall 8)
- Per-session artifacts under `.ass-guard/` (transcripts, learned settings — audit files join them)
- Loud-but-never-fatal auxiliary writes (engine degradation pattern extended to audit)

### Integration Points
- Factory seam: `BuildWithCapturer`-style construction both frontends later share (lands here, Phase 10's runtime extraction inherits it)
- TranscriptWriter: per-session writer on the serve path (currently zero non-test callers — this phase wires it)
- Stability test: pinned-session ID consumed from profile meta (loader change minimal)

</code_context>

<specifics>
## Specific Ideas

- User's audit philosophy (from the option selections): compact always-on logs with full bodies available on demand; the log alone must answer "why did the agent continue" — the engine-decision provenance is the feature, not a nice-to-have.

</specifics>

<deferred>
## Deferred Ideas

- OTLP export of audit events — Future Requirements
- Raw-body opt-in audit mode — Future Requirements
- Audit-driven parity fingerprints in production (per-turn shape fingerprints making drift visible live) — research differentiator, v1.1.x candidate
- Retention automation beyond per-session file boundaries (pruning jobs) — Claude's discretion in-plan, no separate requirement

</deferred>

---

*Phase: 9-Serve-Path Audit + zcode Parity Re-capture*
*Context gathered: 2026-08-14*
