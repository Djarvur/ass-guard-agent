# API Coverage — Anthropic Messages API (provider surface touched by Phase 19)

> Full coverage by default. Opt-outs are explicit, reasoned decisions.

The provider integration itself predates this phase (Phases 01/08-09 built the
Anthropic client and streaming path). Phase 19 extends that existing surface
with three capabilities; the matrix below records this phase's decisions over
them.

| capability | decision | reason |
|---|---|---|
| cache_control ephemeral breakpoints on system blocks (incl. keep-last-4 degrade at the 4-breakpoint API cap) | INTEGRATE | |
| non-2xx stream rejection → ClassifyHTTP-typed error chunks (observable at turn loop) | INTEGRATE | |
| prompt-overflow detection (IsOverflow) for retry-once compaction trigger | INTEGRATE | |
| light-tier model call for compaction summary generation | INTEGRATE | |
| cache_control on non-system blocks (tools / messages) | OPT-OUT | corpus census (910/910) shows zcode emits breakpoints on system blocks only — parity forbids wider placement |
| explicit cache_control read/invalidation management API | OPT-OUT | no such surface in the mimicked agent's requests; nothing to mirror |
