---
phase: 00-spike-re-verification
plan: 05
subsystem: infra
tags: [verification, verified-facts, sanitization, d-03, tier-b, mimicry, munged-cwd, zcode, jsonl, munc, completeness-gate, audit-trail]

# Dependency graph
requires:
  - phase: 00-spike-re-verification
    provides: "the four Wave-1 evidence files this plan folds into VERIFIED-FACTS.md — spikes/01-jsonl-capture/FINDING.md (Plan 01, items #1+#4), spikes/02-openai-toolschema/RESULT.md (Plan 02, item #2), spikes/03-acp-handshake/RESULT.md (Plan 03, item #3), spikes/05-stdout-collision/RESULT.md (Plan 04, item #5); plus the Task-1 gate script check-verified-facts.sh (committed at b3a85ce)"
provides:
  - ".planning/research/VERIFIED-FACTS.md — the post-spike source of truth (5 D-02 sections, all Status in enum, D-03 sanitized, Tier-B disposition recorded honestly, gate exit 0). STACK.md stays as the pre-spike recommendation unchanged (D-01). Phase 1's researcher reads this as MIMC-02 / PROF-03 / PROF-05 ground truth."
  - "The D-07 Tier-B finding for item #1 (#1 zcode JSONL path) resolved to research-predicted default option-a (revise-and-continue), applied by default after the user declined to override the checkpoint — recorded with the honest attribution (NOT 'user chose'). MIMC-02 path correction to be applied at Phase 1 planning."
affects: [phase-1-mimicry-mvp, phase-2-session-core-acp, phase-5-ecosystem-compatibility]

# Actuals (#2632) — pairs with the plan's estimate (30000 tokens, 3 tasks, low confidence)
actuals:
  tokens: 11239   # chars/4 over the realized VERIFIED-FACTS.md diff (44957 chars)
  tasks: 1         # Task 3 only (Task 1 done in a prior session at b3a85ce; Task 2 was the Tier-B checkpoint, resolved out-of-band)
  commits: 2       # 1 feat (VERIFIED-FACTS.md) + 1 docs (this metadata commit)

# Tech tracking
tech-stack:
  added: []  # no libraries; this is an integration/sanitization task over existing evidence
  patterns: ["D-01 single-writer file convention: one plan (00-05) owns VERIFIED-FACTS.md; Wave-1 plans wrote only per-spike FINDING.md/RESULT.md files so there is zero files_modified overlap", "D-03 sanitization as the phase's final secret-leak barrier (threat T-00-10): 8-point checklist over the WHOLE file + a self-grep for sk-/Bearer/*_API_KEY=/raw UUIDs//Users/ paths before commit", "completeness gate as a committed shell script (check-verified-facts.sh) that codifies RESEARCH.md §8/§9 — 5 sections, 5 Status-in-enum, 5 Evidence-not-followed-by-blank, no forbidden tokens, #4 STRUCTURALLY-MOOT, §3 path correction recorded; exits non-zero on any failure", "honest audit-trail attribution: a Tier-B resolution applied by default after the user declined to override is recorded verbatim as such, NOT falsified as 'user chose'"]

key-files:
  created:
    - .planning/research/VERIFIED-FACTS.md
    - .planning/phases/00-spike-re-verification/00-05-SUMMARY.md
  modified:
    - .planning/STATE.md
    - .planning/ROADMAP.md
  # NOTE: check-verified-facts.sh was created in Task 1 (commit b3a85ce); NOT created/modified by this plan's Task 3.

key-decisions:
  - "D-07 Tier-B resolution for item #1 (zcode JSONL path) = research-predicted default option-a (revise-and-continue), APPLIED BY DEFAULT after the user declined to override the checkpoint across multiple prompts — recorded with the honest attribution. The technical consequence is option-a's: Phase 1 MIMC-02 path wording corrected to ~/.zcode/cli/rollout/model-io-sess_<id>.jsonl; munged-cwd convention obsolete for zcode. Reversible at Phase 1 planning."
  - "D-01 honored: STACK.md is byte-identical to its pre-phase state (md5 5e4eecc8418f9ff61272702044da0f54 preserved; git diff empty). VERIFIED-FACTS.md is the post-spike source of truth; STACK stays as the pre-spike recommendation. What we guessed (STACK) never blurs into what we proved (VERIFIED-FACTS)."
  - "D-03 sanitization is the phase's final secret-leak barrier (threat T-00-10, high). The 8-point checklist ran over the WHOLE merged file; the self-grep is CLEAN for sk-/Bearer/xai-/*_API_KEY=/raw UUIDs//Users/ paths. JSON keys, HTTP header NAMES, enum values, and array lengths are PRESERVED (they ARE the mimicry target)."
  - "Phase 0 closes 5/5 STACK items (not 4/5 + a defer): #1 FAILED→revise-and-continue [Tier-B resolved], #2 PARTIAL [schema VERIFIED offline; live round-trip deferred pending operator keys], #3 VERIFIED [Tier-A session/new correction recorded], #4 STRUCTURALLY-MOOT [D-06], #5 VERIFIED [Tier-A handler-signature refinement recorded]."

patterns-established:
  - "Completeness gate script as a committed artifact: the phase's binary 'phase is done' check lives next to the doc it gates (check-verified-facts.sh), codifies RESEARCH.md §8/§9 in grep form, and exits non-zero on any structural/sanitization failure. Pattern reusable for any future phase whose product is a structured doc gated by field-completeness rules."
  - "Tier-B disposition attribution must be honest: when a blocking-human checkpoint is applied by default after the user declines to override, the record says exactly that — falsifying it as 'user chose' would corrupt the audit trail. The technical action is identical to option-a; only the attribution differs."
  - "Single-writer file convention for the integration doc: Wave-1 parallel plans write only their own per-spike evidence files; the Wave-3 integration plan (one writer) folds them all into the canonical committed doc. Zero files_modified overlap is what makes Wave-1 true-parallel safe."

requirements-completed: [STACK-Phase0-#1, STACK-Phase0-#2, STACK-Phase0-#3, STACK-Phase0-#4, STACK-Phase0-#5]

# Coverage metadata (#1602) — one entry per shipped deliverable. The deliverables
# here are: the VERIFIED-FACTS.md doc itself (D1 — the integration product, gated
# by the check script), the Tier-B resolution recording (D2 — audit-trail
# honesty requires human visibility), and the STACK-unchanged invariant (D3).
coverage:
  - id: D1
    description: "VERIFIED-FACTS.md authored with 5 D-02 sections in correct order (#1 zcode JSONL FAILED/Tier-B, #2 go-openai PARTIAL, #3 ACP v1 VERIFIED, #4 STT STRUCTURALLY-MOOT, #5 go-telegram stdout VERIFIED), all Status in enum, §3 path correction recorded, gate exit 0"
    requirement: "STACK-Phase0-#1"
    verification:
      - kind: integration
        ref: ".planning/phases/00-spike-re-verification/check-verified-facts.sh — `bash check-verified-facts.sh` exits 0 (6/6 checks PASS: 5 sections, 5 Status-in-enum, 5 Evidence-not-followed-by-blank, no forbidden tokens, #4 STRUCTURALLY-MOOT, §3 path recorded)"
        status: pass
      - kind: integration
        ref: "plan Task 3 <verify> chain — `check-verified-facts.sh && ! grep sk-/Bearer/*_API_KEY= && test -z git-diff-STACK && grep Tier-B|option-` prints PHASE0_GATE_PASS"
        status: pass
      - kind: integration
        ref: "secret/PII grep over VERIFIED-FACTS.md — zero sk-[A-Za-z0-9]{10}|Bearer [A-Za-z0-9]{20}|xai-|*_API_KEY=|raw UUID|/Users/ matches"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-07 Tier-B resolution for item #1 recorded with HONEST attribution (research-predicted default option-a applied by default after the user declined to override — NOT 'user chose option-a') + option-a consequence verbatim"
    requirement: "STACK-Phase0-#1"
    verification:
      - kind: integration
        ref: ".planning/research/VERIFIED-FACTS.md item #1 Notes — grep 'applied by default after the user declined to override the checkpoint' FOUND; grep 'user chose option-a' count: 0; grep 'Phase 1 MIMC-02 path wording to be corrected' FOUND"
        status: pass
    human_judgment: true
    rationale: "Audit-trail honesty for a Tier-B disposition applied by default (the user declined to answer across multiple prompts) is a correctness claim a human must be able to read and verify against the orchestrator's checkpoint transcript. Auto-pass would mask a mis-attributed record."
  - id: D3
    description: "STACK.md byte-identical to its pre-phase state (D-01) — VERIFIED-FACTS.md is the post-spike source of truth; STACK stays as the pre-spike recommendation unchanged"
    requirement: "STACK-Phase0-#1"
    verification:
      - kind: integration
        ref: "`md5 .planning/research/STACK.md` == 5e4eecc8418f9ff61272702044da0f54 (pre-phase baseline); `git diff .planning/research/STACK.md` empty (no unstaged + no staged changes from this task)"
        status: pass
    human_judgment: false

# Metrics
duration: 8min
completed: 2026-08-09
status: complete
---

# Phase 0 Plan 05: Author VERIFIED-FACTS.md (Phase 0 Integration) Summary

**Folded the four Wave-1 evidence files into a complete, D-03-sanitized, gated `.planning/research/VERIFIED-FACTS.md` (5/5 STACK items closed: #1 FAILED→revise-and-continue [Tier-B resolved by default with honest attribution], #2 PARTIAL, #3 VERIFIED, #4 STRUCTURALLY-MOOT, #5 VERIFIED); STACK.md byte-identical to pre-phase (D-01); gate exits 0; Tier-B recorded as research-predicted default option-a applied by default — NOT 'user chose'.**

## Performance

- **Duration:** ~8 min (Task 3 only)
- **Started:** 2026-08-09 (Task 3)
- **Completed:** 2026-08-09
- **Tasks:** 1 (Task 3; Task 1 was done in a prior session at b3a85ce; Task 2 was the D-07 Tier-B checkpoint, resolved out-of-band with option-a applied by default)
- **Files modified:** 4 (1 created + 1 created [this SUMMARY] + 2 modified [STATE.md, ROADMAP.md]); plus VERIFIED-FACTS.md (the integration product)

## Accomplishments

- Authored `.planning/research/VERIFIED-FACTS.md` (D-01 — new file) by folding the four Wave-1 evidence files into five D-02 sections in strict order #1 → #5: items #1 and #4 ← `spikes/01-jsonl-capture/FINDING.md`; #2 ← `spikes/02-openai-toolschema/RESULT.md`; #3 ← `spikes/03-acp-handshake/RESULT.md`; #5 ← `spikes/05-stdout-collision/RESULT.md`. Each section carries all seven D-02 fields (Fact, Source, Verified, Verified against, Status, Evidence, Notes) with field labels preserved verbatim from the source evidence. Re-headed each copied block from `# #N.` to `## #N.` so the gate's `grep -c '^## #'` matches.
- Recorded the D-07 Tier-B resolution in item #1 Notes with the **honest attribution**: *"research-predicted default **option-a (revise-and-continue)** applied — confirmed by on-disk evidence (plans 00-01..00-04), documented as the prediction in 00-05-PLAN.md Task 2, and applied by default after the user declined to override the checkpoint. Reversible at Phase 1 planning."* + the option-a consequence verbatim: *"→ Phase 1 MIMC-02 path wording to be corrected to `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` during Phase 1 planning; munged-cwd convention obsolete for zcode."* Zero occurrences of the forbidden falsification "user chose option-a" — the audit trail records what actually happened (a default applied after the checkpoint went unanswered), not a fabricated choice.
- Ran the full D-03 sanitization pass (RESEARCH.md §8, all 8 points) over the ENTIRE merged file: prompt/system text redacted; ids (session/trace/turn/request/call/message) redacted; token counts redacted (keys kept); API keys/auth tokens redacted; file paths redacted; tool arguments redacted; JSON keys + HTTP header NAMES + enum values + array lengths PRESERVED (they ARE the mimicry target). Each item's Evidence field carries its redaction note.
- Confirmed the secret/PII grep is CLEAN: zero matches for `sk-[A-Za-z0-9]{10}`, `Bearer [A-Za-z0-9]{20}`, `xai-[A-Za-z0-9]{10}`, `MINIMAX_API_KEY=`, `GROQ_API_KEY=`, `OPENAI_API_KEY=`, raw session-id UUIDs (`[0-9a-f]{8}-...`), and absolute home paths (`/Users/`). This is the phase's final secret-leak barrier (threat T-00-10, high).
- Confirmed STACK.md is **byte-identical to its pre-phase state** (D-01): md5 `5e4eecc8418f9ff61272702044da0f54` preserved; `git diff .planning/research/STACK.md` empty (no unstaged + no staged changes). STACK stays as the pre-spike recommendation; VERIFIED-FACTS.md is the post-spike source of truth.
- Ran the completeness gate: `bash .planning/phases/00-spike-re-verification/check-verified-facts.sh` exits 0 (6/6 checks PASS). The plan's Task 3 `<verify>` chain prints `PHASE0_GATE_PASS`.
- Closed Phase 0 5/5 STACK items (not 4/5 + a defer): #1 FAILED→revise-and-continue [Tier-B resolved], #2 PARTIAL [schema VERIFIED offline; live round-trip deferred pending operator keys — `MINIMAX_API_KEY`/`GROQ_API_KEY` both unset], #3 VERIFIED [Tier-A session/new correction recorded], #4 STRUCTURALLY-MOOT [D-06], #5 VERIFIED [Tier-A handler-signature refinement recorded].

## Task Commits

Each task was committed atomically. Task 1 was done in a prior session; Task 2 was the D-07 Tier-B checkpoint (resolved out-of-band). This run executed Task 3 only:

1. **Task 1: Write the VERIFIED-FACTS.md completeness gate script** - `b3a85ce` (chore, prior session) — `check-verified-facts.sh` (6 checks; exit 0; gate from Task 1 existed before this run)
2. **Task 2: Surface the §3 Tier-B finding for user resolution (D-07)** - (checkpoint, resolved out-of-band) — research-predicted default **option-a (revise-and-continue)** applied by default after the user declined to override the checkpoint; recorded in Task 3 with honest attribution
3. **Task 3: Author VERIFIED-FACTS.md from the four evidence files + record resolution + sanitize + gate** - `4be1cfe` (feat) — VERIFIED-FACTS.md authored (5 sections, D-03 sanitized, Tier-B recorded honestly, gate exit 0)

**Plan metadata:** (this commit) (docs: complete plan — SUMMARY.md + STATE.md + ROADMAP.md)

## Files Created/Modified

- `.planning/research/VERIFIED-FACTS.md` (created, `4be1cfe`) - The post-spike source of truth. File header notes it is the post-spike record (STACK.md is pre-spike, unchanged) + verification date 2026-08-09. Five `## #N.` sections in order, each with the 7 D-02 fields. Tier-B disposition in #1 Notes uses the honest default-applied wording. D-03 sanitized across the whole file; gate exit 0.
- `.planning/phases/00-spike-re-verification/check-verified-facts.sh` (created in Task 1, `b3a85ce`, prior session) - The 6-check completeness gate (sections ≥5, Status-in-enum, Evidence-not-followed-by-blank, no forbidden tokens, #4 STRUCTURALLY-MOOT, §3 path recorded). Exits 0 against the final VERIFIED-FACTS.md.
- `.planning/phases/00-spike-re-verification/00-05-SUMMARY.md` (created, this commit) - This summary.
- `.planning/STATE.md` (modified, this commit) - Plan counter 4→5; Phase 0 marked complete; current phase advanced to Phase 1 readiness; Tier-B resolution recorded in Decisions Log with honest attribution.
- `.planning/ROADMAP.md` (modified, this commit) - Phase 0 plan progress 4/5 → 5/5; Phase 0 row marked complete with date.

## Decisions Made

- **D-07 Tier-B resolution attribution = honest.** The checkpoint went unanswered across multiple prompts; the research-predicted default (option-a revise-and-continue) was applied. The record says exactly that — *"applied by default after the user declined to override the checkpoint"* — NOT "user chose option-a". Falsifying the attribution would corrupt the audit trail; the technical action (option-a) is identical either way. This matches the `<tier_b_resolution>` directive verbatim.
- **The gate's "Evidence not followed by an empty line" check (§8) caught a real defect.** The initial draft of item #4's Evidence reasoning paragraph was followed by a blank line before `- **Notes:**` — the gate flagged it. The fix removed the blank line so the Evidence field flows directly into the Notes field. (This is the one Rule-1 deviation in this run — see Deviations.) All four other Evidence fields are followed by code fences (not blank lines), so they passed on the first run.
- **STACK.md left untouched (D-01) — the file boundary is load-bearing.** The pre-phase md5 (`5e4eecc8418f9ff61272702044da0f54`) was captured at run start and re-verified post-commit; the `git diff` is empty. "What we guessed (STACK)" never blurs into "what we proved (VERIFIED-FACTS)"; Phase 1's researcher reads VERIFIED-FACTS.md as MIMC-02 ground truth, not STACK.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Gate flagged item #4 Evidence field followed by an empty line**
- **Found during:** Task 3 (gate run #1)
- **Issue:** The gate's §8 check (`awk` flags any line where the line immediately after an `^- \*\*Evidence:\*\*` field is empty) failed on item #4: the Evidence reasoning-chain paragraph was followed by a blank line before `- **Notes:**`. The four other Evidence fields passed because each is followed by a code fence (a non-empty line). The plan's §8 rule ("none followed by an empty line") is a structural-completeness invariant the gate enforces.
- **Fix:** Removed the single blank line between the #4 Evidence paragraph and the `- **Notes:**` line, so the Evidence field flows directly into the Notes field. No content changed.
- **Files modified:** `.planning/research/VERIFIED-FACTS.md`
- **Verification:** gate re-run: PASS [evidence]: 5 Evidence field(s) found, none followed by an empty line; full chain prints PHASE0_GATE_PASS.
- **Committed in:** `4be1cfe` (Task 3 feat commit)

---

**Total deviations:** 1 auto-fixed (1 Rule-1 bug — gate-mechanics vs canonical field spacing, caught and fixed before the VERIFIED-FACTS.md commit)
**Impact on plan:** No scope creep. The fix is a one-line whitespace correction that makes the plan's own gate pass; content unchanged in substance.

### Notes on the Task 1 deviation (prior session, commit b3a85ce)

For completeness: Task 1's `check-verified-facts.sh` has its own documented Rule-1 deviation in source comments (the #4-STRUCTURALLY-MOOT check is implemented as a section-scoped scan rather than the plan's literal `grep -A3 '^## #4\.'` window, because the literal window cannot reach the Status field under the canonical D-02 field order and would reject a correctly-structured file). That deviation was committed at `b3a85ce` in the prior session and is NOT re-introduced here — it is part of the gate this run executes as-is.

## Issues Encountered

None beyond the one Rule-1 deviation above. The four Wave-1 evidence files were complete and D-02-shaped (Plans 00-01..00-04 had each drafted their section in the canonical 7-field form), so the fold was a verbatim copy with a one-`#` re-heading per block. The Tier-B resolution directive was unambiguous (apply option-a, record honestly), and the honest-wording check (zero occurrences of "user chose option-a") passed on the first authoring.

## User Setup Required

**Operator key provisioning remains the single open operator action to flip item #2 from PARTIAL → VERIFIED** (carried forward from Plan 00-02; this plan does not change that status). Export at least one of:
- `MINIMAX_API_KEY` — MiniMax platform console → API keys
- `GROQ_API_KEY` — Groq Console → API Keys
- `OPENAI_API_KEY` + `OPENAI_BASE_URL` + `OPENAI_MODEL` — any other OpenAI-shape endpoint

Then re-run `cd spikes && go run ./02-openai-toolschema/`. The schema fidelity (the load-bearing #2 question) stands VERIFIED offline regardless. (No USER-SETUP.md generated — the env vars are documented in `spikes/README.md` and in VERIFIED-FACTS.md item #2 Notes.)

No other external service configuration required for Phase 0's closure.

## Threat Flags

This plan introduces no NEW security-relevant surface beyond the plan's `<threat_model>` (T-00-10/T-00-11). Both are mitigated:

| Flag | File | Description |
|------|------|-------------|
| (covered) T-00-10 | .planning/research/VERIFIED-FACTS.md | Information disclosure — the final sanitization pass over the merged file. Mitigated: D-03 8-point checklist + secret/PII self-grep CLEAN (no sk-/Bearer/xai-/*_API_KEY=/raw UUID//Users/). The phase's final secret-leak barrier. |
| (covered) T-00-11 | .planning/research/VERIFIED-FACTS.md item #1 Notes | Repudiation — Tier-B resolution recording. Mitigated: the chosen option + date + consequence + honest default-applied attribution is recorded verbatim, so the disposition is auditable. |

No threat flags beyond the plan's register — both registered threats (T-00-10/11) are mitigated and verified.

## Next Phase Readiness

- **Plan 00-05 (this plan) is complete. Phase 0 is complete (5/5 plans).** VERIFIED-FACTS.md is the dated, sanitized, gated post-spike source of truth; STACK.md is the unchanged pre-spike recommendation; the one Tier-B finding has an explicit, recorded, honest resolution; the completeness gate exits 0.
- **Phase 1 (Mimicry MVP, north-star proof)** reads VERIFIED-FACTS.md item #1 as the ground truth for zcode profile extraction (MIMC-02, PROF-03, PROF-05). The corrected path (`~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl`) + the richer-than-STACK schema (full wire-level request/response; no MITM proxy for the request body; the 12 identity header names; the 77-tool Anthropic-shape catalog) unblock Phase 1's profile artifact design. **MIMC-02's path wording correction to `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` is a Phase-1-planning action** (per the recorded option-a consequence); the munged-cwd convention is obsolete for zcode.
- **Carried-forward operator action:** provision at least one of `MINIMAX_API_KEY` / `GROQ_API_KEY` and re-run `spikes/02-openai-toolschema/` to flip item #2 PARTIAL → VERIFIED. The schema fidelity is VERIFIED offline; only the provider-quirk live observations remain open (Tier A).
- **No blockers.** Phase 0's ROADMAP goal (the team can trust every inherited fact before building on it; PITFALLS N18 mitigation) is met.

## Self-Check: PASSED

- **Files created (2/2 FOUND):** `.planning/research/VERIFIED-FACTS.md`, `.planning/phases/00-spike-re-verification/00-05-SUMMARY.md`
- **Files modified (2/2 FOUND):** `.planning/STATE.md`, `.planning/ROADMAP.md`
- **Commits FOUND:** `b3a85ce` (Task 1 gate script, prior session), `4be1cfe` (Task 3 VERIFIED-FACTS.md feat), (this commit) (docs metadata)
- **STACK.md unchanged:** md5 `5e4eecc8418f9ff61272702044da0f54` preserved; `git diff` empty
- **Gate exit 0:** `bash check-verified-facts.sh` 6/6 PASS
- **Plan `<verify>` chain:** PHASE0_GATE_PASS
- **Secret/PII grep:** CLEAN (zero sk-/Bearer/xai-/*_API_KEY=/raw UUID//Users/ matches)
- **Tier-B attribution honest:** "user chose option-a" count: 0; "applied by default after the user declined to override" FOUND

---

*Phase: 00-spike-re-verification*
*Plan: 05*
*Completed: 2026-08-09*
