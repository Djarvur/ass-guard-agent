# Milestones

## v1.1 ACP Early Adoption (Complete: 2026-08-21)

**Phases:** 5 (8, 9, 12, 14, 13) · 5 complete · **Re-scoped 2026-08-16** (operator — renamed "Kickoff & Peers" → "ACP Completion" → "Product Completion"; Telegram and dsh lowered to the v1.2 pool in favor of product completeness; the added phase then split into machinery + OpenSpec halves; v1.1 adds zero new dependencies) · **Re-ordered 2026-08-18** (operator — early adoption: Phase 14 added from the other-agents-analysis dispositions; Phase 12's waves split around an adoption line — 12-01/02/05/04/06 pre-adoption, 12-03/07/08 post; Phase 13 post-adoption; milestone completes when the operator begins daily ACP use)
**Started:** 2026-08-14 (continues from v1.0's Phase 7 numbering)
**Outcome:** adoption line crossed 2026-08-20 (the operator began daily ACP use) — product proof delivered: the unmodified OpenSpec toolkit runs hands-off end-to-end incl. through model asks; 34/34 plans across the 5 phases, every phase verification-closed (8 witness-accepted, 9 PASS, 12 8/9+disposition, 14 39/39, 13 3/3); verified milestone gates: mise ci green + eval-gate flagship green k=1 (manager-certified 2026-08-20)

- [x] Phase 8: Slash-Command Kickoff — completed 2026-08-16 (operator witness accepted; zero-continue product proof green — `/opsx:explore → propose → apply → archive` chained by the engine with zero manual continues, verified against the real openspec binary; findings 5+6 confirmed; known residuals: UAT check 3 hook live-leg + one stage-4 model-variance fail in the 2/3 zero-continue tally)
- [x] Phase 9: Serve-Path Audit + zcode Parity Re-capture — completed 2026-08-18 (6/6 plans, verifier PASS, UAT 3/4 accepted — parity re-baseline disposition accepted; the two routed follow-ups live in Phase 12 as 12-03/12-05)
- [x] Phase 12: Product Functional Completeness — completed 2026-08-20 (8/8 plans, verification 8/9 — ACP-08 PARTIAL by disposition: eval first-green deferred to Phase 13 exit and discharged there 2026-08-21, flagship green ×3; every catalog tool executes for real — the FULL completeness gate is a permanent regression test; result forms capture-pinned via the live zcode re-record; plugin-install discovery live-proven)
- [x] Phase 14: Adoption Readiness (Analysis Dispositions) — completed 2026-08-19 (6/6 plans, verification pass 39/39 — the four human items resolved in one operator session: gated live rollback E2E RUN+PASS with real GLM credentials [WINDOWS.md #4 → fixed, byte-identical restore + user .git untouched], pi↔shaper audit classifications accepted [zero-fix legitimate], compaction-decision routing accepted [cache_control emission → post-adoption queue TIER-1, cross-turn span → 12-05], MVP goal wording accepted as-is; UAT 4/4; residual WINDOWS #3+#5 open and routed: timer-resume client mirroring → 12-07, cache probe standing-FAIL flips green when the routed TextBlock fix lands)
- [x] Phase 13: OpenSpec Workflow Completion — completed 2026-08-21 (5/5 plans incl. the 13-00 wave-0 gap closure — the engine-visible ask resume per the manager Rule-4 route-1 ruling; 13-VERIFICATION 3/3 PASS with both zero-continue chain legs re-run green by the verifier against the real binary + real model; matrix E2E 12 legs, 12 per-command eval suites green k=1; WINDOWS #8/#9 discharged)

**Moved to the v1.2 pool (2026-08-16, operator):**

- Phase 10: Telegram Peer (Text + Voice) — plans preserved in `.planning/phases/10-telegram-peer-text-voice/`; carries go-telegram/bot + the `internal/runtime` extraction to v1.2 (the 2026-08-17 execution attempt died with its agent; partial WIP preserved at `47f10b4` on the phase branch, untrusted — resume or revert deliberately at the replan)
- Phase 11: dsh Mimicry Profile #2 — needs replanning against source-analysis (dsh wire capture impossible; operator constraint 2026-08-15); carries zstd to v1.2
- Steering / input queue during a running turn (IDEA-LANDSCAPE gap 4) — tagged onto the Phase 10 Telegram replan (prerequisite-quality for a chat peer)
- v1.2 pool also holds: SEED-001 (agent-creation kit library), ECOSYSTEM-AUDIT clusters PLUG (full lifecycle) / LSP / MEM

## v1.0 MVP (Shipped: 2026-08-14)

**Phases completed:** 8 phases (0–7), 36 plans
**Timeline:** 6 days (2026-08-09 → 2026-08-14) · 210 commits (60 feat) · ~33.5k LOC Go, 24 packages · 410 files / 65.8k insertions
**Git range:** `4bc4521` (initial) → v1.0 tag

**Delivered:** A single static Go binary that speaks ACP v1 (Zed-native), shapes model requests structurally indistinguishable from the mimicked agent (zcode profile, thesis proven), schedules models across tiers/time-windows/fallbacks, hosts unmodified OpenSpec with a zero-continue autocontinue engine + hook-DAG + learning store, loads Claude-Code ecosystem setups, and installs zero-config via the ACP registry — with multi-provider credentials resolving from config so editor-spawned processes authenticate with zero env vars.

**Key accomplishments:**

1. **Mimicry thesis proven (Phase 1)** — zcode profile extracted from real rollout logs (3 byte-identical system blocks, 103-tool catalog, identity headers, thinking/tool_choice); A/B parity test shows statistically indistinguishable tool-call sequences from live zcode. The north star held.
2. **All inherited load-bearing facts re-verified before building (Phase 0)** — 5/5 STACK flags closed via throwaway spikes; zcode JSONL path corrected to `~/.zcode/cli/rollout/`; ACP `session/new` lifecycle step caught as a Tier-A spec correction; go-telegram/bot + ACP stdout coexistence proven with byte-equality assertions.
3. **ACP v1 session core (Phase 2)** — Zed-spawnable stdio JSON-RPC server with token streaming, session replay, two-layer context (durable transcript + lean projected window reset at command boundaries), and parallel subagents as goroutine turn-loops.
4. **Model scheduling layer (Phase 3)** — tier abstraction with time-windowed substitution (IANA zones, bundled tzdata), per-project override, typed provider-error fallback chains, circuit breakers, and a dollars-per-window cost ceiling with degrade-then-stop.
5. **Unified engine + Hook-DAG + learning (Phase 4)** — post-turn observer (continue/hook/ask/wait), dual-signal handoffs, structural safety (unmatched ⇒ nothing — testing/quick + E2E proven), cancel-drain, hand-rolled ≤300-LOC DAG executor with provenance loop prevention, ask-once-remember learning store (`ass-guard learning list/revert`).
6. **Ecosystem + distribution (Phases 5–6)** — MCP subprocess hosting with zombie-proof lifecycle (process groups, reaper, per-connection tools/list re-fetch); `.claude/` discovery read-only with clean namespacing; goreleaser 4-target static binary + canonical ACP registry `agent.json` + go:embed zero-config first-run seed.
7. **Multi-provider config & credentials (Phase 7)** — two-shape ProviderFactory (anthropic/openai) at all construction sites; credential precedence flag > env > config with `${VAR}` expansion; lazy uncredentialed failure; 0600-perm hygiene warnings; proven live by a zero-env editor-spawned model turn authenticating from the config file.

### Known Gaps

- **Phase-4 UAT kickoff gap (major):** live OpenSpec workflows kick off via agent-executed command files (`/opsx:explore` etc., installed by `openspec init --tools claude`), but `internal/ecosys` command discovery is unwired into session/ACP and the Phase-4 adapter modeled the binary as stage driver with a command set mismatching openspec v1.5.0's real surface. 11 UAT checks deferred. Fix deferred to next milestone (operator decision 2026-08-14; full diagnosis in `04-UAT.md` → Gaps).
- **Phase-1 parity stability test blocked:** the pinned zcode capture session (`eea3dc48`) is absent on disk; operator must export `ZAI_API_KEY` + re-capture a divergence-prone session.
- **LOG-01:** `--audit-log` is not written on the `acp serve` path (tracer wired via main.go only).
- **Catalog drift:** `tools.json` carries 103 tools vs the plans' stale 77 (PROF-04 documented; seed mirrors source byte-for-byte).

**Known deferred items at close: 2** (see STATE.md Deferred Items — both UAT-related, dispositioned by operator)

---
