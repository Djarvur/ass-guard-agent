// Package runtime owns the turn runner (Runner, D-18): the session-lifetime
// core that drives each session/prompt through the Session Core, the Phase-4
// engine chain (with its 13-00 park state + advisory dedupe), the per-session
// construction site (sessionFor — transcripts, checkpoint store, MCP host,
// core tool executors, ask broker, plan-mode state), and the cron firing
// family (cron_wiring.go — the scheduler goroutine, per-session turn
// serialization, and the server-driven-turn forwarder).
//
// Kit ambition (SEED-001): at Phase 25 this package is the reusable agent
// core of the ass-guard kit — an embeddable turn runner an operator can
// compose under any transport, not just the ACP serve path. The carve (Phase
// 15) is the first step: the runner leaves the CLI composition (cmd) and
// acpserve composes it as a library. The Phase-25 seam plan (recorded with
// the amended D-13/D-15 sign-off): cron_wiring.go is slated to grow an
// interface-based cron seam then, as design work rather than relocation
// collateral — today its nine methods stay byte-verbatim beside their
// receiver (Go forbids declaring methods outside their receiver's package,
// and the core↔cron calls are bidirectional).
//
// Non-goals:
//
//   - acpserve is NOT kit surface (it is the ACP transport composition, not
//     the agent core) — nothing here imports it;
//   - no ACP-specific words in API naming (Run/Runner/RunnerConfig speak in
//     turns and sessions, not frames — the acp.TurnRunner interface
//     satisfaction is structural, not nominal).
package runtime
