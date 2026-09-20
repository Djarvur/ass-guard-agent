# Phase 21: Context & Policy Parity Closures - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

The content-path parity closures land as one coherent wave: settings.json hooks with PreToolUse deny (and scope-limited allow) join Phase 17's single gate pipeline at the executor chokepoint; AGENTS.md/CLAUDE.md auto-inject into system context via the per-session profile-copy merge (mtime-cached); thinking blocks stream live and round-trip byte-identical including cryptographic signatures; rich prompt content (base64 images with auto-downscale, @-mentions) enters with ingress validation and loud degradation.

</domain>

<decisions>
## Implementation Decisions

### Hook Authority & Merge
- **D-01:** Hook ALLOW authority is scope-split: USER-scope settings.json hooks may return allow (skips the permission ask); PROJECT-scope (repo-shipped) hooks are deny-only — an allow decision from project scope is ignored with a loud warning. PAR-03's letter exactly; CC's capability minus the repo-trust hole.
- **D-02:** Verdict contract = CC's `hookSpecificOutput` JSON (`permissionDecision` allow/deny/ask + `permissionDecisionReason`) AND legacy exit-2 deny — both parse; existing plugin hooks unchanged.
- **D-03:** All matching hooks run in deterministic scope order (project settings → user settings → plugin bundles); ANY deny wins over any allow (deny-wins, mirroring 17-D-02); the deny carries the first denying hook's reason. Fail-open applies to hook EXECUTION failure only (PAR-03 letter), never to conflicting verdicts.
- **D-04:** Full allow/deny/ask support: `ask` routes the call to the ask step even in ungated mode (opens a dialog) — the operator's explicit escalation lever. The only dialog a repo-shipped file can surface is an ask; never an allow.

### AGENTS.md Discovery
- **D-05:** Both AGENTS.md and CLAUDE.md recognized; same-directory collision → CLAUDE.md wins (CC-native name), AGENTS.md skipped with a note. Different levels accumulate.
- **D-06:** Discovery span: walk UP from cwd to the git repo root, collecting memory files at each level, plus the user global. Stops at the repo boundary — no walk-to-$HOME noise above the project (deliberately narrower than CC).
- **D-07:** User-level global: BOTH `~/.ass-guard/AGENTS.md|CLAUDE.md` and `~/.claude/CLAUDE.md` are checked; `~/.ass-guard` wins on conflict (operator choice — cross-tool sharing without ceding the home convention).
- **D-08:** Injected memory content is capped (per-file ~16–32 KB and a total budget across levels); over-cap files truncate with a loud note in the injection. Exact values Claude's discretion.

### Rich Content Rules
- **D-09:** Over-limit images AUTO-DOWNSCALE to fit the provider limit (operator choice): pure-Go image processing ONLY — the CGO_ENABLED=0 `mise ci` build gate stands. Original bytes preserved on disk; transcript provenance records the resize (original dims/size → scaled). — **Reversibility:** costly — the image-processing dependency becomes part of the build contract; removing it later means re-deciding oversize policy.
- **D-10:** @-mention: `@file` expands to file content (Read-tool rule gated per PAR-06 letter, provenance line); `@dir` expands to a one-level directory LISTING (names + sizes), no recursion. Unresolvable `@path` → loud note, turn proceeds.
- **D-11:** Provider without image support (ingress validation fails): drop the image block + ONE loud degrade note naming the provider; the turn proceeds with the text. Never silent, never a dead turn.

### Thinking Verification
- **D-12:** Thinking blocks are stored in the transcript as raw `json.RawMessage` (exact provider bytes) — the byte-identical round-trip holds BY CONSTRUCTION; no typed re-serialization can drift (extends 16-D-23's type-level bypass).
- **D-13:** Thinking streams live to the editor as `agent_thought_chunk` through the TurnEmitter (Phase 16's kinds) AND re-emits identically on session/load replay (Phase 18) — real history, no synthetic markers.
- **D-14:** Byte-identical round-trip is proven by golden fixtures from CAPTURED wire pairs (thinking + signature + redacted_thinking) through transcript → projector → outgoing request, plus an emitter replay test. Real bytes and signatures; drift fails loudly.

### Claude's Discretion
- Hook hard-timeout value (PAR-03 letter requires one; pick CC-compatible).
- settings.json schema details (hooks key layout, matcher regex dialect) — CC-compatible preferred.
- Memory file size caps' exact values (D-08) and the injection block's framing text.
- The pure-Go image library choice (researcher verifies candidates against CGO_ENABLED=0).
- @-mention parse details (whitespace, paths with spaces, quoting).
- Golden fixture capture mechanics (redaction of any sensitive bytes while preserving signature fields).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §CC Parity Closures — PAR-03 (hooks: settings.json scopes, PreToolUse deny at the chokepoint, bounded sync, hard timeout, fail-open, deny-only from project scope), PAR-04 (AGENTS.md/CLAUDE.md injection via profile-copy merge, mtime-cached), PAR-05 (thinking RawMessage passthrough, signature round-trip), PAR-06 (image blocks, @-mentions, Read-gating, ingress validation) verbatim
- `.planning/ROADMAP.md` §Phase 21 — goal, 5 success criteria

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/17-permissions-elicitation/17-CONTEXT.md` — D-04/D-05 (the ONE gate pipeline these hooks join at the head), D-02 (deny-wins grammar D-03 mirrors)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-23 (thinking type-level redaction bypass — D-12 extends), D-02 (TurnEmitter ordered stream — D-13 rides), D-20 (additive kinds)
- `.planning/phases/18-session-family/18-CONTEXT.md` — D-01/D-02 (replay re-emits thinking as real history)
- `.planning/phases/19-compaction-cache-control/19-CONTEXT.md` — thinking chains never rewritten mid-chain (compaction cut discipline interacts with D-12/D-13)

### External References
- `code.claude.com/docs/en/hooks` + hooks reference — hookSpecificOutput JSON verdict shape, exit-2 legacy, matcher dialect (D-02's parity target)
- `code.claude.com/docs/en/memory` + `code.claude.com/docs/en/claude-directory` — memory hierarchy (D-05..D-07's reference; we deliberately narrow the span)
- `platform.claude.com/docs/en/build-with-claude/thinking` — signature/redacted_thinking preservation rules (D-12/D-14's invariants)
- `platform.claude.com/docs/en/build-with-claude/vision` — image limits (8000×8000, 10 MB/5 MB classes, ~1568 px recommendation) for D-09's target

### Code Anchors
- `internal/ecosys/hooks.go` — the existing CC hook runner (exit-2, matchers, timeouts, stdin contract) PAR-03 extends with settings.json scopes + JSON verdicts
- `internal/runtime/runtime.go` gate chokepoint (post-17 landing zone) — where hook verdicts join permission evaluation
- Per-session profile copy (shaper) — PAR-04's injection vehicle (trailing System TextBlocks)
- `internal/shaper/shaper.go` — ingress validation + provider-shape mapping for images/mentions; thinking passthrough
- Captured corpus (`profiles/zcode/`, internal/profile fixtures) — D-14's golden source

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- HookRunner (ecosys/hooks.go): stdin-JSON contract, matcher regex, per-hook timeouts, exit-2 deny, CLAUDE_PLUGIN_ROOT env — PAR-03 adds scope parsing + the JSON verdict channel, not a new runner.
- Plugin precedence machinery (loader's tier merge) — D-03's deterministic scope order follows the same merge discipline.
- Advisory/engine note machinery — D-08's truncation notes and D-11's degrade notes join the family.
- Profile-copy merge seam (per-session) — PAR-04's mtime-cached injection lands exactly where the roadmap says.

### Established Patterns
- Graceful degradation + loud counters — D-01's ignored-allow warning, D-10's unresolvable-mention note, D-11's drop note.
- By-construction invariants — D-12's raw storage mirrors 16-D-23's redactor bypass and 18's transcript-as-truth.
- Captured-ground-truth testing — D-14's goldens follow the corpus-fixture family.

### Integration Points
- The ONE gate pipeline (Phase 17): hook verdict head → permission ask → execute (criterion 2's documented precedence becomes code).
- TurnEmitter kinds (Phase 16): agent_thought_chunk streaming.
- Request-build seam: memory injection + image ingress validation.
- configOptions: hook-relevant toggles if any emerge at planning (16's apply-as-landed rule).

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- Auto-downscale over reject — UX priority, accepted with the pure-Go/CGO-gate constraint pinned.
- Both home locations checked for user memory, ~/.ass-guard winning — cross-tool friendliness without ceding the convention.
- Full ask support from hooks (including ungated mode) — the escalation lever is worth the dialog cost.

</specifics>

<deferred>
## Deferred Ideas

- Full CC memory span (walk-to-$HOME chain + subdirectory-on-demand loading) — deliberately narrowed to cwd→git-root; revisit if monorepo/parent-context pain emerges.
- CLAUDE.local.md and import syntax (@file inside memory files) — CC legacy/deprecated surfaces; not carried.

</deferred>

---

*Phase: 21-context-policy-parity-closures*
*Context gathered: 2026-08-27*
