# Phase 0: Spike + Re-verification - Context

**Gathered:** 2026-08-09
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 0 is a **verification phase**, not a feature phase. It mechanically re-verifies the 5 inherited facts recorded in `research/STACK.md` §"Open Verification Items (Phase-0 spike checklist)" (lines 484–492) — each ~5 weeks old as of 2026-08-09 — so that Phase 1 builds on grounded, dated evidence. It carries **no REQ-IDs** and produces **no feature code**. Its only products are (a) `.planning/research/VERIFIED-FACTS.md` — the trusted, dated record every downstream phase reads — and (b) throwaway spike programs under `spikes/` that prove the empirical claims.

**In scope:** the 5 STACK Phase-0 items, recorded as dated `{fact, source, verified_date, verified_against_version, status, evidence}` records; disposable proof for the items where a running process is the only honest evidence.

**Out of scope:** any feature code, the real Go module (`go.mod` at repo root starts in Phase 1), the ACP server skeleton (Phase 2), the mimicry profile itself (Phase 1), and — per a decision in this phase — any whisper.cpp bundling work (closed structurally; see D-06).

</domain>

<decisions>
## Implementation Decisions

### Verification registry (where the trusted facts live)
- **D-01:** Verified-fact records live in a **new file `.planning/research/VERIFIED-FACTS.md`**. `STACK.md` is left unchanged as the pre-spike recommendation — VERIFIED-FACTS.md is the post-spike source of truth, and the clean file boundary means "what we guessed Aug 2026" (STACK) never blurs into "what we proved" (VERIFIED-FACTS). Phase 1's researcher reads VERIFIED-FACTS.md as MIMC-02's ground truth.
- **D-02:** VERIFIED-FACTS.md uses **one section per fact** (not a single summary table). Each section carries the fields: `Fact`, `Source`, `Verified` (date), `Verified against` (version/commit/tag), `Status` (`VERIFIED` / `FAILED` / `PARTIAL` / `STRUCTURALLY-MOOT`), `Evidence`, `Notes`. The per-fact sections leave room for the JSONL schema specifics and spike observations that Phase 1 actually needs (a flat table would force Phase 1 to dig elsewhere).
- **D-03:** Captured samples (zcode JSONL excerpt, ACP frame dump, spike output) are embedded **inline in VERIFIED-FACTS.md** as fenced code blocks — 5–10 representative lines, not full dumps. Samples **must be sanitized/redacted** of prompts, tokens, file paths, and PII before commit, with the redaction noted in the `Evidence` field (e.g. "5 representative lines; prompt content redacted"). Rationale: a verification phase's evidence is inherently noisy/sensitive; inline representative excerpts keep the file self-contained and git-safe.

### Proof strength (does Phase 0 produce any code?)
- **D-04:** Phase 0 produces **throwaway spike code under a `spikes/` directory** — disposable Go programs that prove empirical claims. The spikes are explicitly **NOT the start of the real codebase**; they exist only to produce evidence for VERIFIED-FACTS.md. Item #5 (stdout collision) is an "integration test" per the roadmap and can only be honestly closed by a running process; item #2 (tool-calling schema fidelity) is empirical and needs a real round-trip.
- **D-05:** The `spikes/` directory has its **own isolated `go.mod`** (a distinct module, e.g. `github.com/djarvur/ass-guard-spikes`) and is **gitignored at the build-artifact level**: the spike `.go` source files + a `spikes/README.md` (how to run each) are committed, but `spikes/go.sum` and any built binaries are ignored. The **repo root stays clean** — no root `go.mod` until Phase 1. This preserves the "throwaway, not the codebase" intent and lets Phase 1 start the real module with clean, deliberate dependency decisions uncoupled from what the spikes happened to pin.
- **Per-item proof assignment:**
  - **#1 zcode JSONL path + schema** → **filesystem capture**, not Go code. Locate zcode's real on-disk transcript, extract a representative sample. The deliverable is the documented path + schema in VERIFIED-FACTS.md.
  - **#2 `sashabaranov/go-openai` tool-calling schema** → **spike**: a small program that performs a real tool-call round-trip against MiniMax M3 and Groq, recording the request/response schema. Pin the latest tag in the spike's go.mod.
  - **#3 ACP v1 method names** → **spike** (user chose to spike rather than doc-pin): a tiny ACP `initialize` handshake + one `session/prompt` round-trip against the canonical spec, confirming method names and wire shape empirically. (Doc-pinning from the spec repo is acceptable as supplementary evidence, but the user wants a running handshake.)
  - **#5 `go-telegram/bot` + ACP stdout collision** → **spike**: the integration test the roadmap explicitly calls for — both transports in one process, asserting no non-ACP frame ever reaches stdout.

### whisper.cpp / STT (item #4 resolution)
- **D-06:** **STT is ALWAYS an external utility — ass-guard NEVER bundles STT via cgo.** This holds for every backend: the OpenAI Whisper API is a network call; whisper.cpp (or any local STT) runs as an out-of-process binary (`whisper-cli`) the user installs, which ass-guard shells out to. Consequence: the Go binary stays pure-Go, so **goreleaser's macOS+Linux amd64+arm64 cross-compile matrix is unaffected by STT choice.** STACK item #4 ("whisper.cpp cross-compile impact") is therefore **`STRUCTURALLY-MOOT`** — its premise (cgo cross-compile risk) does not arise under this architecture. **Phase 0 closes 5/5 items** (not 4/5 + a defer). This decision propagates to v2 STT work: the external-utility rule is settled and won't be relitigated.

### Negative-finding protocol (what happens when a fact fails)
- **D-07:** **Severity-tiered failure response.** When a fact fails re-verification, the response depends on the failure's nature:
  - **Tier A — DOC/MINOR failure** (path moved to a sibling directory, version/tag bumped, field renamed, cosmetic drift): record `FAILED` in VERIFIED-FACTS.md **plus the corrected fact**, add a pointer from the affected STACK.md line, and **continue** the phase. These are exactly the kind of 5-week staleness Phase 0 exists to catch; they're updates, not alarms.
  - **Tier B — ARCHITECTURAL failure** (a load-bearing premise is wrong: e.g. go-openai cannot represent MiniMax's tool-calling schema, ACP v1 lacks a method the architecture assumed, zcode's JSONL does not carry the system prompt): record `FAILED`, **stop the phase**, and surface the finding to the user with three options — **(a) revise-and-continue** (if the fact can be re-scoped), **(b) halt-and-replan** (if it changes the roadmap), **(c) accept-and-document** (informed override). The finding is written to VERIFIED-FACTS.md before stopping so the evidence is never lost.
  - Rationale: matches the project's stop-and-replan culture (Phase 1 gates everything; PITFALLS N18 "grounded, dated evidence"; PROJECT.md Anti-Pattern 5) without overreacting to cosmetic drift. The north-star thesis (Phase 1 mimicry) is architectural; an architectural surprise during re-verification deserves a human decision, never silent propagation into Phase 1 planning.

### Claude's Discretion
- The exact contents/wording of VERIFIED-FACTS.md sections (beyond the field schema in D-02) are at the researcher/planner's discretion, guided by what Phase 1's researcher will actually need to read.
- Which specific prompt(s) the #2 tool-schema spike sends to MiniMax M3 / Groq — any minimal tool-calling request that exercises the schema is fine.
- The precise structure of the #5 stdout-collision test (goroutine layout, assertion mechanism) — any shape that reliably proves "no non-ACP frame on stdout when both transports run" is acceptable.
- Whether the #3 ACP handshake spike also pins the spec version via a spec-repo checkout or just hits a live handshake — supplementary doc-pinning is welcome but not required.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### The 5 facts to verify (the phase's entire scope)
- `.planning/research/STACK.md` §"Open Verification Items (Phase-0 spike checklist)" (lines 484–492) — the canonical list of the 5 items Phase 0 closes. Read the full surrounding STACK.md entries (the table rows + "Stack Patterns by Variant" + "Version Compatibility" + "What NOT to Use") for each item's load-bearing claim and recommended library/version.
- `.planning/ROADMAP.md` §"Phase 0: Spike + Re-verification" — the 4 success criteria; criterion #4 defines the `{fact, source, verified_date, verified_against_version}` record shape this phase produces.

### Project-level context that shapes this phase
- `.planning/PROJECT.md` — the v1 cut-line (STT default = OpenAI Whisper API; Telegram deferred to v2; profiles = zcode only), the constraints (single static binary, stdout = ACP only), and Anti-Pattern 5 (the north-star mimicry thesis gates everything; stop-and-replan on failure). The "Constraints" and "v1 Cut-Line" sections are load-bearing for D-06 and D-07.
- `.planning/research/PITFALLS.md` N18 — "serialized deltas, grounded dated evidence." The reason Phase 0 exists and the reason D-07 tiers failures the way it does.
- `.planning/REQUIREMENTS.md` — Phase 0 carries no REQ-IDs, but the MIMC-02 / PROF-03 / PROF-05 requirements (zcode profile extracted from real transcripts) are what item #1's JSONL capture ultimately unblocks in Phase 1.

### Where Phase 0's output is consumed downstream
- Phase 1 reads VERIFIED-FACTS.md item #1 as the ground truth for the zcode profile extraction (MIMC-02, PROF-03). The researcher should know the JSONL path/schema details must be specific enough to drive the profile artifact design.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None. This is a greenfield repository — only planning documents (`.planning/`) and the agent config (`.zcode/`, `AGENTS.md`) exist. No `.go` files, no `go.mod`, no `src/`. Phase 0 creates the first Go code (the spikes) but it is throwaway and isolated.

### Established Patterns
- **Project discipline (load-bearing for spikes):** stdout is reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics go to stderr (PROJECT.md "Transport discipline"). The #5 stdout-collision spike exists to *prove* this discipline holds when `go-telegram/bot` and ACP coexist — so the spike itself must model the discipline (Telegram goroutine logs to stderr, never stdout).
- **Throwaway-code convention:** spikes are isolated under `spikes/` with their own module and gitignored build artifacts (D-05). The real module starts at Phase 1.

### Integration Points
- None yet. The spikes do not integrate into a codebase — they are standalone programs whose output becomes evidence in VERIFIED-FACTS.md.

</code_context>

<specifics>
## Specific Ideas

- The user's framing for #4 ("we can use external utility for STT, so no cross-compile needed") is the origin of D-06. It's not just a deferral — it's an **architectural rule**: STT is always out-of-process. This is stronger than STACK's existing "do not cgo-bind whisper.cpp" guidance; it generalizes to all STT backends and is what makes #4 structurally moot.
- The user chose to **spike #3 (ACP method names)** rather than doc-pin it. The recommended option was doc-pin (method names are a spec lookup), but the user wants empirical confirmation via a running `initialize` handshake. Worth honoring — it costs little and the ACP wire shape is load-bearing for Phase 2.
- The user wants VERIFIED-FACTS.md to be **self-contained** (samples inline, one section per fact) rather than a thin index pointing at external capture files — prioritizing reviewer readability over raw-evidence auditability, with the sanitization rule (D-03) as the safety net for sensitive content.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within the Phase 0 verification scope. No feature capabilities were proposed (correctly — this is a verification phase). The whisper.cpp *integration test* (actually running whisper-cli end-to-end through ass-guard's STT backend) is not deferred so much as obviated by D-06 for v1; should local STT be pulled into scope in v2+, that integration test lands then, but the cross-compile concern it was meant to address remains closed.

</deferred>

---

*Phase: 0-Spike + Re-verification*
*Context gathered: 2026-08-09*
