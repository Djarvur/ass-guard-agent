---
phase: 0
slug: spike-re-verification
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-09
---

# Phase 0 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
>
> **Phase 0 is a verification phase, not a feature phase.** "Validation" here means "did we honestly close each of the 5 STACK Phase-0 items" — not "does a feature work." The sampling substrate is the spikes themselves (each a binary pass/fail program) plus a completeness grep over `VERIFIED-FACTS.md`. There are no REQ-IDs and no feature test suite; this document adapts the Nyquist template to that reality.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go run` for the Go spikes + shell/jq for the filesystem capture (spikes/ is an isolated Go module — `github.com/djarvur/ass-guard-spikes`; spikes #2/#3/#5 are `main` packages that run their assertion and exit non-zero on FAIL; spike #1 is a filesystem capture using `jq`/`python3 -m json.tool`, not a Go program — see CONTEXT.md D-04 "filesystem capture, not Go code") |
| **Config file** | `spikes/go.mod` (Plan 01 Task 1 creates it; pins `sashabaranov/go-openai@v1.42.0`, `go-telegram/bot@v1.23.0`) |
| **Quick run command** | `bash spikes/01-jsonl-capture/capture.sh && (cd spikes && go run ./02-openai-toolschema/ && go run ./03-acp-handshake/ && go run ./05-stdout-collision/)` |
| **Full suite command** | `bash spikes/01-jsonl-capture/capture.sh && (cd spikes && go vet ./... && go run ./02-openai-toolschema/ && go run ./03-acp-handshake/ && go run ./05-stdout-collision/) && bash .planning/phases/00-spike-re-verification/check-verified-facts.sh` |
| **Estimated runtime** | ~30-60 seconds (dominated by the #2 provider round-trip and #3 ACP handshake; #1 and #5 are sub-second) |

**Note on the "framework":** these are not unit tests in the `go test` sense. Spikes #2/#3/#5 are `main` programs whose process exit code IS the assertion result (0 = PASS, non-zero = FAIL), and whose stderr footer is the structured evidence the executor captures into `VERIFIED-FACTS.md`. Spike #1 is a filesystem capture (per D-04) — it locates the zcode rollout directory, parses one `model_io` line with `jq`, and writes a redacted schema excerpt to `spikes/01-jsonl-capture/FINDING.md`; its "assertion" is the exit code of the capture step + the presence of the FINDING.md. This shape was chosen in CONTEXT.md D-04 ("throwaway spike code… exists only to produce evidence") and RESEARCH.md §7.

---

## Sampling Rate

- **After every task commit:** Run the specific spike the task produced (e.g., after the #2 spike task, run `(cd spikes && go run ./02-openai-toolschema/)`; after the #1 capture task, run `bash spikes/01-jsonl-capture/capture.sh`). This is the fastest meaningful feedback.
- **After every plan wave:** Run the spikes built so far. Wave 1 = the #1 capture script + #4 structural note (Plan 01). Wave 2 = add the #2/#3/#5 spikes (Plans 02/03/04, which run in parallel once Plan 01's `spikes/go.mod` exists). Wave 3 = the VERIFIED-FACTS.md completeness gate (Plan 05).
- **Before `/gsd:verify-work`:** All spikes PASS (#2 may be `PARTIAL` if no API key — Tier A) + `VERIFIED-FACTS.md` has 5 complete sections (5 × `Status:`, 5 × `Evidence:`, no `TBD`/`TODO`) + the §3 Tier-B checkpoint resolved.
- **Max feedback latency:** ~60 seconds (the #2 provider round-trip is the long pole; everything else is sub-second).

---

## Per-Task Verification Map

Task IDs are provisional (the planner assigns final IDs). This map lists the validation each spike-producing task MUST carry.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 00-01-0x | 01 | 1 | (no REQ-ID; closes STACK item #1) | — | N/A — read-only filesystem capture | capture-script | `bash spikes/01-jsonl-capture/capture.sh` → exit 0; output: redacted JSONL schema excerpt in `spikes/01-jsonl-capture/FINDING.md` | ❌ W0 | ⬜ pending |
| 00-02-0x | 02 | 2 | (no REQ-ID; closes STACK item #2) | — | **API keys never logged to stdout** (transport discipline); redacted before VERIFIED-FACTS.md commit (D-03) | spike-program | `(cd spikes && go run ./02-openai-toolschema/)` → exit 0; output: redacted request/response JSON on stderr | ❌ W0 | ⬜ pending |
| 00-03-0x | 03 | 2 | (no REQ-ID; closes STACK item #3) | — | N/A — local ACP handshake (no secrets) | spike-program | `(cd spikes && go run ./03-acp-handshake/)` → exit 0; output: redacted ACP frame dump on stderr | ❌ W0 | ⬜ pending |
| 00-04-0x | 04 | 2 | (no REQ-ID; closes STACK item #4) | — | N/A — structural reasoning, no code | doc-only | `grep -F 'STRUCTURALLY-MOOT' .planning/research/VERIFIED-FACTS.md` → match found | ❌ W0 | ⬜ pending |
| 00-05-0x | 05 | 2 | (no REQ-ID; closes STACK item #5) | — | **Telegram bot logging routed to stderr, never stdout** (transport discipline — load-bearing, this is the spike's whole point) | spike-program | `(cd spikes && go run ./05-stdout-collision/)` → exit 0; output: byte-diff count (0 = clean) on stderr | ❌ W0 | ⬜ pending |
| 00-VERIF | 05 | 3 | (completeness gate + §3 Tier-B checkpoint) | — | All samples sanitized per D-03 checklist | shell-check | `bash .planning/phases/00-spike-re-verification/check-verified-facts.sh` → exit 0 | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

**"Wave 0" caveat for this phase:** Wave 0 in a normal phase means "stub test files before implementation." In Phase 0, the spikes ARE the tests; there is no separate test layer to stub. The `❌ W0` markers above indicate "the spike program does not exist yet" — creating it IS the task, and the spike's own exit code is its verification. The `spikes/go.mod` + `spikes/README.md` task (Plan 01 Task 1, the module skeleton) is the closest analog to a Wave 0 task — it lands in Wave 1 and is a hard dependency for Plans 02/03/04 (which need the module to compile).

---

## Wave 0 Requirements

- [ ] `spikes/go.mod` — isolated module `github.com/djarvur/ass-guard-spikes`, Go 1.25, pins `sashabaranov/go-openai@v1.42.0` + `go-telegram/bot@v1.23.0` (Plan 01 Task 1, Wave 1)
- [ ] `spikes/README.md` — how to run each spike (committed; per D-05) (Plan 01 Task 1)
- [ ] `.gitignore` — adds `spikes/go.sum` and `spikes/bin/` (built binaries gitignored; source committed; per D-05) (Plan 01 Task 1)
- [ ] `.planning/phases/00-spike-re-verification/check-verified-facts.sh` — the completeness gate (5 sections, 5 Status, 5 Evidence, no TBD/TODO) (Plan 05 Task 1, Wave 3)

**No `go test` stubs are needed** — the spikes are `main` programs (or, for #1, a shell capture script), not test suites. The "framework" is `go run` for Go spikes and `bash`/`jq` for the #1 capture.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The §3 finding (zcode JSONL path differs from STACK: `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl`, not `~/.claude/projects/...`) is surfaced for user sign-off, because it changes Phase 1's MIMC-02 path wording | (no REQ-ID; CONTEXT.md D-07 Tier B protocol) | This is a **human decision gate** (D-07 Tier B: revise-and-continue / halt-and-replan / accept-and-document), not an automated check. The spike can record the finding, but the *resolution* requires user judgment about whether the MIMC-02 requirement wording needs an upstream edit. | After the #1 spike completes, present the finding to the user with the three Tier-B options. Record the chosen resolution in VERIFIED-FACTS.md `Notes`. Default (if research prediction holds): revise-and-continue — record corrected path, flag MIMC-02 wording, proceed. |
| API keys for the #2 spike (MiniMax M3 / Groq) are provisioned by the operator | (no REQ-ID; closes STACK item #2) | The planner/executor cannot create provider accounts. If no key is available, the spike records `PARTIAL` (Tier A: try the other provider; if both unavailable, record PARTIAL + defer). | Operator sets `MINIMAX_API_KEY` / `GROQ_API_KEY` env vars before running the #2 spike. Spike reads from env, never logs the key, redacts it from any captured request before printing. |

*All other phase behaviors have automated verification (the spike exit codes + the completeness gate).*

---

## Validation Sign-Off

- [ ] All spike-producing tasks have an `<automated>` verify command (the spike's own `go run` + exit code)
- [ ] Sampling continuity: each wave has at least one runnable spike (no 3-task gap without automated verify — trivially satisfied; Phase 0 is ~6-8 tasks, most of which produce a spike)
- [ ] Wave 0 covers all MISSING references (the `spikes/go.mod` skeleton task creates the module every other spike task depends on)
- [ ] No watch-mode flags (spikes are one-shot `go run`, not watchers)
- [ ] Feedback latency < 60s (the #2 provider round-trip is the long pole)
- [ ] `nyquist_compliant: true` set in frontmatter (set after planner confirms the per-task map above matches the final PLAN.md task IDs)

**Approval:** pending
