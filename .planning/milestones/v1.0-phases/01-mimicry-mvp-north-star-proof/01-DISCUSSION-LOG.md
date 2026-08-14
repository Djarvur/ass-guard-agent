# Phase 1: Mimicry MVP (north-star proof) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-09
**Phase:** 1-Mimicry MVP (north-star proof)
**Areas discussed:** A/B parity test design, Profile fidelity & coverage boundary, Profile Shaper architecture, Turn Loop MVP scope vs Phase 2

---

## Area selection

| Option | Description | Selected |
|--------|-------------|----------|
| A/B parity test design | MIMC-03 is THE gate; prompt suite / metric / threshold choices decide whether the phase can ship | ✓ |
| Profile fidelity & coverage boundary | PROF-05 manifest + PROF-03 capture ref + PROF-04 drift detector; which fields byte-faithful vs structural | ✓ |
| Profile Shaper architecture | MIMC-01 chokepoint composition; PROF-02 no-zcode-coupling enforcement | ✓ |
| Turn Loop MVP scope vs Phase 2 | Minimal loop for parity test vs Phase 2's Session Core; where throwaway scaffold ends | ✓ |

**User's choice:** All four areas selected.
**Notes:** All four were presented as the decisions that will most change how research + planning approach Phase 1.

---

## A/B parity test design

### Prompt suite source

| Option | Description | Selected |
|--------|-------------|----------|
| Curated divergence set (rec.) | 5–15 hand-authored prompts targeting tool-divergence-prone paths; highest signal-per-sample | ✓ |
| Replay from zcode transcripts | Replay user turns from rollout files; grounds in real workload but couples to capture data | |
| Synthetic coverage sweep | Auto-generated prompts across tool-combination patterns; max coverage but may not reflect real workload | |

**User's choice:** Curated divergence set.
**Notes:** Highest signal per dollar; team controls what's in the suite.

### Metric

| Option | Description | Selected |
|--------|-------------|----------|
| Tool-call sequence equality (rec.) | Compare ordered tool-name sequences; ignores arguments; cleanest MIMC-03/04 reading | |
| Sequence + argument structure (selected) | Sequence equality AND per-tool argument structural equality (keys present, value shapes — not exact string) | ✓ |
| Tool-frequency distribution test | Compare distributions across many runs; most rigorous but expensive + assumes variance is signal | |

**User's choice:** Sequence + argument structure (stricter than bare minimum).
**Notes:** Catches a shaper that picks the right tool but passes malformed args. Stops short of byte-diff (respects MIMC-04).

### Threshold

| Option | Description | Selected |
|--------|-------------|----------|
| 100% — all prompts must match (rec.) | Literal gate; honors stop-and-replan culture; achievable because team controls the suite | ✓ |
| ≥ 90% — allow 1–2 divergences | Allows stochastic variance but can mask a real shaper bug | |
| Per-prompt match rate over N runs | Accounts for variance but expensive and "how many runs" is its own question | |

**User's choice:** 100% threshold.
**Notes:** The only bar that makes "gate" literally true. Discipline shifts to ensuring the curated set genuinely probes divergence.

### Variance handling

| Option | Description | Selected |
|--------|-------------|----------|
| Deterministic: temp=0, single run (rec.) | Every mismatch is a real signal pointing at the shaper; reproducible | ✓ |
| Stochastic: N runs, set comparison | Captures variance but expensive and mismatch could be variance OR bug | |
| Lock model+temp, research model choice | Lock what we can, research what we can't | |

**User's choice:** Deterministic temp=0, single run.
**Notes:** Test config (model, temp, seed) recorded for reproducibility.

### zcode arm (follow-up)

| Option | Description | Selected |
|--------|-------------|----------|
| Replay zcode's captured output (rec.) | Rollout transcripts as reference; offline, reproducible, no live zcode at test time | ✓ |
| Live zcode process | Truest comparison but stochastic + operationally heavy + couples to zcode version | |
| Frozen golden set from MITM capture | Capture once, freeze, compare; one-time cost, no zcode at test time | |

**User's choice:** Replay zcode's captured output.
**Notes:** RESEARCH-FLAG-01 raised — transcripts feed both profile extraction and parity reference; researcher must evaluate held-out split.

### Tool execution (follow-up — user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Stub all tools (rec.) | Return canned results; hermetic; aligns with temp=0 determinism; thesis is about what model SEES | ✓ (Claude's discretion) |
| Execute tools against a fixture | More realistic but introduces nondeterminism breaking temp=0 | |
| Research decides per-prompt | Defer based on whether prompts are single-turn or multi-turn | |

**User's choice:** (no answer — Claude's discretion: stub all tools).
**Notes:** Reversible if research finds multi-turn reactions to real tool output needed.

---

## Profile fidelity & coverage boundary

### Fidelity tiers

| Option | Description | Selected |
|--------|-------------|----------|
| Three explicit tiers (rec.) | TIER-1 byte-faithful / TIER-2 structural / TIER-3 informational; drift flags TIER-1/2 only | ✓ |
| Two tiers: exact vs informational | Simpler but bundles load-bearing-but-invariant headers with pure noise | |
| Single tier (derive empirically) | Strictest but drift detector fires on every transient field — useless until tuned | |

**User's choice:** Three explicit tiers.
**Notes:** Maps onto how mimicry thesis works — model attends to system+tools, provider fingerprints via headers, audit cares about rest.

### Coverage manifest form

| Option | Description | Selected |
|--------|-------------|----------|
| Machine-readable manifest (rec.) | YAML/JSON sidecar; CI-style hard failure on missing field; feeds TOOL-03 too | ✓ |
| Human-readable doc | Easier to author but can't be mechanically enforced | |
| Manifest + generated doc | Both; manifest is source of truth, doc generated | |

**User's choice:** Machine-readable manifest.
**Notes:** One artifact, two gates (PROF-05 + TOOL-03).

### Drift cmd data source

| Option | Description | Selected |
|--------|-------------|----------|
| Fresh live capture + diff (rec.) | Run live zcode, capture, diff field-by-field; truest PROF-04; needs keys at check time | ✓ |
| Diff against recent transcripts | No live zcode but fuzzy freshness window | |
| Two modes: --live + --transcripts | CI uses transcripts, operator uses live | |

**User's choice:** Fresh live capture + diff.
**Notes:** Operator command, not CI. Accepts operational weight because PROF-04 is the killer feature.

---

## Profile Shaper architecture

### SDK driving (user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Drive SDK native types (rec.) | Populate anthropic-sdk-go Message/System/ToolDef; compile-time safety, streaming, retries | |
| Build raw request body | Max fidelity but re-implements SDK machinery; compiler won't catch shape bugs | |
| Hybrid: native + escape hatch (selected) | Native types for bulk + data-driven escape hatch for fields SDK doesn't model (12 identity headers) | ✓ (Claude's discretion) |

**User's choice:** (no answer — Claude's discretion: hybrid native + escape hatch).
**Notes:** Escape hatch is part of profile artifact, not code — preserves PROF-02.

### Schema resolution (user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Profile schema authoritative, adapter layer (rec.) | Shaper pulls schemas from PROFILE; catalog provides EXECUTION; TOOL-02 verbatim | ✓ (Claude's discretion) |
| Built-in catalog is sole source | Simpler but can't reproduce captured schema differences — breaks mimicry at schema layer | |
| Built-in default + profile override | Pragmatic but makes drift detection harder and contradicts TOOL-02 "authoritative" | |

**User's choice:** (no answer — Claude's discretion: profile schema authoritative, adapter layer).
**Notes:** Directly follows from TOOL-02 wording.

### Coupling enforcement (user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Synthetic-profile conformance test (rec.) | CI test loads non-zcode fixture; any zcode branch breaks it; structural guarantee | ✓ (Claude's discretion) |
| Extract a second real profile | Strongest proof but doubles extraction work + different schema | |
| Lint rule + review | Lightest weight but lint catches strings, not semantic coupling | |

**User's choice:** (no answer — Claude's discretion: synthetic-profile conformance test + lint).
**Notes:** Strongest lightweight enforcement without doubling profile work.

---

## Turn Loop MVP scope vs Phase 2

### Loop shape (user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Test-harness loop, no session mgmt (rec.) | Narrow interface Run(ctx, profile, prompt, toolStubs) -> []ToolCall; Shaper survives, loop replaced | ✓ (Claude's discretion) |
| Minimal real Session Manager | Less throwaway but risks committing to Phase 2 decisions + violates serialized deltas | |
| No loop — direct Shaper+adapter test | Zero throwaway but no integration seam; LOG-01 needs a hook | |

**User's choice:** (no answer — Claude's discretion: test-harness loop).
**Notes:** Respects serialized-deltas discipline; Shaper+adapters survive into Phase 2.

### Audit hook (user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Event-bus subscriber (rec.) | Emit RequestShaped event; async subscriber; satisfies LOG-02 from day one | ✓ (Claude's discretion) |
| Direct synchronous call | Simplest but couples loop to logger; logger in critical path | |
| Defer to Phase 2 | Cleanest Phase 1 but LOG-01 is explicitly Phase-1 REQ | |

**User's choice:** (no answer — Claude's discretion: event-bus subscriber).
**Notes:** Minimal Phase-1 bus (one event type); Phase 2 expands.

### Tools scope (user declined to answer; Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Full catalog (TOOL-01 verbatim) (rec.) | All 77 tools; profile declares all; large surface area | |
| Subset: parity-exercised only | Smaller but model sees full 77 — "not implemented" stubs could change selection | |
| Tiered: full for exercised, schema-stub for rest (selected) | All 77 faithful schemas; execution stubbed per A/B decision | ✓ (Claude's discretion) |

**User's choice:** (no answer — Claude's discretion: tiered — collapses with stub-all-tools decision).
**Notes:** Work is faithful schema declarations for 77 tools + dispatch skeleton; execution is stubbed in parity test anyway.

---

## Claude's Discretion

Seven decisions were marked Claude's discretion because the user declined to specify them (across multiple prompts). Each was chosen to follow from an existing locked decision or requirement, not as a new commitment. All are reversible at planning:
- D-09 SDK-driving (hybrid native + escape hatch)
- D-10 Schema-resolution (profile-authoritative adapter — follows from TOOL-02)
- D-11 PROF-02 enforcement (synthetic-profile conformance test + lint)
- D-12 Turn Loop shape (test-harness — follows from serialized deltas)
- D-13 Audit hook (event-bus subscriber — satisfies LOG-02)
- D-14 Tool catalog scope (all 77 declared — collapses with D-15)
- D-15 Tool execution in parity test (stub all — aligns with temp=0 determinism)

## Deferred Ideas

None raised — discussion stayed within Phase-1 mimicry scope. Adjacent implementation details (OpenAI-shape field translation, profile versioning mechanics, CLI subcommand tree) recorded in CONTEXT.md as researcher/planner details, not user decisions.

---

*Phase: 1-Mimicry MVP (north-star proof)*
*Discussion date: 2026-08-09*
