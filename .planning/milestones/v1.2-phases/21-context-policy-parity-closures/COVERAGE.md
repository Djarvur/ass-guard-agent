# API Coverage — Phase 21

> Deterministic detector result (2026-08-28): `{"detected":false,"signals":[]}` over the phase
> scope (21-CONTEXT.md + ROADMAP phase section + PLAN bodies).

No external API integration: Phase 21 extends in-repo seams only — the existing `ecosys.HookRunner`
(settings.json scope parsing + verdict resolution), the per-session profile-copy merge in
`runtime.sessionFor`, the Phase-16 thinking endpoints (`AppendRawThinking` / `ThoughtChunk`), and
`expandUserBlocks` ingress. The one new module (`golang.org/x/image`) is a pure-Go library
dependency (D-09), not an external API integration; provider request shaping rides the already-pinned
`anthropic-sdk-go` param types.
