# Milestones

## v1.1 Kickoff & Peers (in progress)

**Phases:** 4 (8–11) · 1 complete
**Started:** 2026-08-14 (continues from v1.0's Phase 7 numbering)

- [x] Phase 8: Slash-Command Kickoff — completed 2026-08-16 (operator witness accepted; zero-continue product proof green — `/opsx:explore → propose → apply → archive` chained by the engine with zero manual continues, verified against the real openspec binary; findings 5+6 confirmed; known residuals: UAT check 3 hook live-leg + one stage-4 model-variance fail in the 2/3 zero-continue tally)
- [ ] Phase 9: Serve-Path Audit + zcode Parity Re-capture — executed; 09-04 blocked-on-harvest, operator to run the scripted re-capture workload (docs/recapture-runbook.md §3), then automated legs + verification
- [ ] Phase 10: Telegram Peer (Text + Voice)
- [ ] Phase 11: dsh Mimicry Profile #2 — needs replanning against source-analysis (dsh wire capture impossible; operator constraint 2026-08-15)

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
