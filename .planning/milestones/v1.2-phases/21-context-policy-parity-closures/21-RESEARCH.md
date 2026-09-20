# Phase 21: Context & Policy Parity Closures - Research

**Researched:** 2026-08-28
**Domain:** Claude-Code-parity policy gates (settings.json hooks), context injection (AGENTS.md/CLAUDE.md), thinking-block passthrough, rich prompt ingress (images/@-mentions) — Go 1.26, anthropic-sdk-go v1.63.0
**Confidence:** HIGH (in-repo seams verified by reading source this session; external contracts cited from official CC/Anthropic docs; one new pure-Go dependency registry-verified)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Implementation Decisions (LOCKED)

#### Hook Authority & Merge
- **D-01:** Hook ALLOW authority is scope-split: USER-scope settings.json hooks may return allow (skips the permission ask); PROJECT-scope (repo-shipped) hooks are deny-only — an allow decision from project scope is ignored with a loud warning. PAR-03's letter exactly; CC's capability minus the repo-trust hole.
- **D-02:** Verdict contract = CC's `hookSpecificOutput` JSON (`permissionDecision` allow/deny/ask + `permissionDecisionReason`) AND legacy exit-2 deny — both parse; existing plugin hooks unchanged.
- **D-03:** All matching hooks run in deterministic scope order (project settings → user settings → plugin bundles); ANY deny wins over any allow (deny-wins, mirroring 17-D-02); the deny carries the first denying hook's reason. Fail-open applies to hook EXECUTION failure only (PAR-03 letter), never to conflicting verdicts.
- **D-04:** Full allow/deny/ask support: `ask` routes the call to the ask step even in ungated mode (opens a dialog) — the operator's explicit escalation lever. The only dialog a repo-shipped file can surface is an ask; never an allow.

#### AGENTS.md Discovery
- **D-05:** Both AGENTS.md and CLAUDE.md recognized; same-directory collision → CLAUDE.md wins (CC-native name), AGENTS.md skipped with a note. Different levels accumulate.
- **D-06:** Discovery span: walk UP from cwd to the git repo root, collecting memory files at each level, plus the user global. Stops at the repo boundary — no walk-to-$HOME noise above the project (deliberately narrower than CC).
- **D-07:** User-level global: BOTH `~/.ass-guard/AGENTS.md|CLAUDE.md` and `~/.claude/CLAUDE.md` are checked; `~/.ass-guard` wins on conflict (operator choice — cross-tool sharing without ceding the home convention).
- **D-08:** Injected memory content is capped (per-file ~16–32 KB and a total budget across levels); over-cap files truncate with a loud note in the injection. Exact values Claude's discretion.

#### Rich Content Rules
- **D-09:** Over-limit images AUTO-DOWNSCALE to fit the provider limit (operator choice): pure-Go image processing ONLY — the CGO_ENABLED=0 `mise ci` build gate stands. Original bytes preserved on disk; transcript provenance records the resize (original dims/size → scaled). — **Reversibility:** costly — the image-processing dependency becomes part of the build contract; removing it later means re-deciding oversize policy.
- **D-10:** @-mention: `@file` expands to file content (Read-tool rule gated per PAR-06 letter, provenance line); `@dir` expands to a one-level directory LISTING (names + sizes), no recursion. Unresolvable `@path` → loud note, turn proceeds.
- **D-11:** Provider without image support (ingress validation fails): drop the image block + ONE loud degrade note naming the provider; the turn proceeds with the text. Never silent, never a dead turn.

#### Thinking Verification
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

### Deferred Ideas (OUT OF SCOPE)
- Full CC memory span (walk-to-$HOME chain + subdirectory-on-demand loading) — deliberately narrowed to cwd→git-root; revisit if monorepo/parent-context pain emerges.
- CLAUDE.local.md and import syntax (@file inside memory files) — CC legacy/deprecated surfaces; not carried.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PAR-03 | Hooks: settings.json parsing (project + user scopes), PreToolUse deny interceptor at the executor chokepoint — bounded sync execution, hard timeout, fail-open, structured verdicts, deny-only authority from project scope; joins the ONE gate pipeline locked in Phase 17 (hook verdict → permission ask → execute) | Existing `ecosys.HookRunner` (internal/ecosys/hooks.go) already implements stdin-JSON payload, matcher regex, per-hook timeout, exit-2 deny, sanitized env; PAR-03 adds settings.json scope parsing + the `hookSpecificOutput` JSON verdict channel + scope-split authority + deny-wins ordering, then joins 17-02's `gateCall` hook-verdict head (currently an implicit-allow no-op seam). See Phase-17 Dependency Map + Patterns 1–2. |
| PAR-04 | AGENTS.md/CLAUDE.md auto-injected into system context every session via dynamic merge into the per-session profile copy (trailing System TextBlocks), mtime-cached | The per-session profile-copy merge seam exists verbatim at internal/runtime/runtime.go:1046–1075 (skills listing + agent listing ride the identical trailing-TextBlock vehicle). D-05..D-08 define discovery; Pattern 3 gives the walker. |
| PAR-05 | Thinking blocks streamed to client: provider `"thinking"` chunks → raw `json.RawMessage` passthrough end-to-end (transcript, redactor excluded by construction, projector) — Anthropic signatures round-trip byte-identical | Phase 16 built the endpoints (`Manager.AppendRawThinking` manager.go:349; `EmitterHandle.ThoughtChunk` emitter.go:516); the MIDDLE is missing — `StreamChunk` has no thinking type, the SSE state machine ignores thinking deltas, the projector ignores raw_thinking lines, and `AppendRawThinking` has zero production callers (grep-verified). Pattern 4 gives the full chain. |
| PAR-06 | Rich prompt content: image blocks (base64) and @-file mentions expand with Read-tool rule gating and provenance; ingress capability validation per provider shape | `Prompt` already takes `[]ContentBlock` (text-only today); `expandUserBlocks` (runtime.go:395) is the ingress seam; shaper maps messages to SDK params; `ImageBlockParam` verified in the pinned SDK. Pattern 5 covers ingress + validation + downscale; D-09's pure-Go constraint resolved with golang.org/x/image/draw. |
</phase_requirements>

## Project Constraints (from AGENTS.md)

- **Tech stack:** Go single static binary; `go 1.26` pinned in go.mod (verified this session); CGO_ENABLED=0 in `mise` build task — D-09's pure-Go image constraint is a build-gate fact, not a preference.
- **Transport discipline:** stdout reserved exclusively for ACP JSON-RPC frames; ALL logging to stderr (`log.Printf`/`slog` conventions throughout internal/).
- **Compatibility:** Claude Code `.claude/` layout drop-in — settings.json hooks must read the SAME files CC reads (project `.claude/settings.json`, user `~/.claude/settings.json`), never clobber them.
- **Safety model:** no tool-execution confirmation tier; the hook table + gate pipeline is the only safety mechanism — this phase IS the policy surface, so the security section is load-bearing.
- **GSD workflow:** all edits ride GSD commands; research artifacts via this file.

## Summary

Phase 21 lands four parity closures that share two vehicles that already exist. First, the policy vehicle: `ecosys.HookRunner` (internal/ecosys/hooks.go) already executes hooks with the CC stdin-JSON contract, matcher regexes, per-hook timeouts, capped output, and the exit-2 PreToolUse refusal — but only for plugin-bundle hooks (`hooks/hooks.json`), with a boolean `(proceed bool, message string)` verdict. PAR-03 extends this same runner with settings.json scope parsing (project + user), the `hookSpecificOutput` JSON verdict channel (`permissionDecision` allow/deny/ask + `permissionDecisionReason`), scope-split authority (D-01), and deny-wins ordering (D-03) — then joins the hook-verdict head of Phase 17's `gateCall`, which 17-02 explicitly reserves as an "implicit-allow no-op today, one documented seam comment naming Phase 21" (17-02-PLAN Task 1). Second, the context vehicle: the per-session profile copy in `Runner.sessionFor` (internal/runtime/runtime.go:1029–1075) already merges dynamic trailing System TextBlocks (cwd composition, skills listing, agent listing) — PAR-04's memory injection is a fourth merge of exactly that shape, with an mtime cache and the D-05..D-07 walker.

The thinking leg (PAR-05) is an end-to-end middle-fill: Phase 16 built both endpoints — the `raw_thinking` transcript line with the redactor-exempt append path (`Manager.AppendRawThinking`, manager.go:344–358) and the `agent_thought_chunk` emitter method (emitter.go:516) — but nothing produces or consumes them yet: `provider.StreamChunk` (internal/provider/provider.go:46–58) has no thinking type, the hand-rolled SSE state machine (internal/provider/streaming.go:265) only tracks tool-use blocks, and `AppendRawThinking` has zero production callers. PAR-06 rides the ingress seam `expandUserBlocks` (runtime.go:395) and the text-only `ContentBlock` struct (transcript.go:81–84), which gains an image variant; D-09's pure-Go downscale resolves to `golang.org/x/image/draw` (official golang.org/x repo, v0.45.0, CGO-free — verified via `go list -m -versions`).

**Phase-ordering reality (the planner's central constraint):** Phase 17 is PLANNED, NOT EXECUTED — `internal/perm/` and `internal/session/gate.go` do not exist in the tree (verified by `ls` and grep this session), while Phase 16's artifacts are all present. Numeric execution order (15→25, STATE.md) means 17 lands before 21 executes, so Phase 21 plans write against the LOCKED 17-01/17-02 contracts (the `gateCall` seam signature, the `gateVerdict` enum, `internal/perm`'s `Verdict` vocabulary) with per-task `precondition` blocks, exactly as 17-02 itself preconditions on Phase 16's outputs. Three of the four legs decompose so that the bulk of the work is 17-independent (hook runner extension, memory injection, thinking pipeline, image ingress); only the thin JOIN tasks (hook verdicts into `gateCall`'s head; `ask` routing to the ask step) are 17-blocked and must be marked. See "Phase-17 Dependency Map" below — it enumerates exactly which legs are blocked.

**Primary recommendation:** Plan four plans in dependency order: (1) settings-scope hooks + JSON verdicts in `internal/ecosys` with deny-wins ordering solved INSIDE the runner (17-independent), (2) memory discovery + mtime cache + profile-copy merge (fully independent), (3) thinking pipeline provider→transcript→bus→emitter→projector with golden fixtures (independent of 17; respects 19's locked never-rewrite discipline), (4) rich-content ingress (independent except the Read-rule-gating consult, which delegates to the same gate seam). Then a thin TRACER plan joins hook verdicts to `gateCall`'s head — the only 17-blocked slice — mirroring how Phase 20's plans reserve/fall-through against Phase 19.

## Phase-17 Dependency Map (planning against a locked-but-unbuilt contract)

Phase 17's plans 17-01/17-02 ARE the contract. Verified verbatim from `.planning/phases/17-permissions-elicitation/17-01-PLAN.md` and `17-02-PLAN.md`:

| Locked artifact (17) | Contract Phase 21 codes against | Exists today? |
|---|---|---|
| `internal/session/gate.go` — `(*Session).gateCall(ctx, turnID, callID, tool string, input json.RawMessage) gateVerdict` (17-02 artifacts list) | The JOIN point: 17-02 Task 1: "gateCall runs the hook-verdict head FIRST (implicit-allow no-op today, one documented seam comment naming Phase 21 and D-04), then rule evaluation" | NO — `ls internal/session/gate.go` fails; grep for `gateCall` in internal/session returns nothing (verified) |
| `gateVerdict{action, payload}` with `gateExecute/gateDeny/gateSuspend/gateDeclineAutomation` (17-02 artifacts_this_phase_produce) | Hook verdicts map: deny→`gateDeny`, ask→`gateSuspend` (D-04's ask-even-ungated; 17-02's ungated ask evaluation becomes hook-driven suspension), allow→`gateExecute` | NO |
| `internal/perm` — `RuleSet.Evaluate(toolName, primaryArg) Verdict` with `VerdictDeny/VerdictAsk/VerdictAllow/Unmatched`; `MCPName/SplitMCPName` (17-01 line 81) | PAR-06's Read-rule gating consults the same rule-set provider seam (injected accessor; implicit allow pre-17) | NO — `ls internal/perm` fails (verified) |
| AskQueue + permission ask surface (17-02/17-03) | D-04's `ask` routes to "the ask step even in ungated mode" — the queued permission ask | NO |
| `AskBroker`, `s.Hooks *ecosys.HookRunner`, transcript kinds, TurnEmitter, expandUserBlocks | The seams Phase 21 extends | YES — all verified in-tree |

**How to mark blocked legs (the Phase-20 precedent):** Phase 20's plans reserve names and fall through to current behavior with loud documentation (20-01: reserved builtin names "fall through as plain text (current behavior) and become live entries in their own plan tasks"). Phase 21 inverts this: the substrate (hook runner, verdict parser) is Phase 21's own deliverable, and only the JOIN is 17's. Therefore:

- Plans 1–4 (runner extension, memory, thinking, rich content) carry NO 17 dependency — each task's tests run green without `gate.go` existing.
- The final TRACER plan (hook-join) carries `precondition:` blocks verbatim from 17-02's must_haves ("internal/session/gate.go exists with gateCall … Phase 16 executed" style), and its tasks REPLACE the implicit-allow no-op — a loud, grep-able seam comment swap, not a parallel gate. If 17's landed shape diverges from the locked contract, the executor degrades loudly (stop + report), never bolts a second gate (17-CONTEXT D-05 forbids a second path; 21-CONTEXT criterion 2: "no second gate exists").
- Scope-order reconciliation (D-03 vs the loader's current merge order) must land in plan 1, not the TRACER — see Pitfall 1.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| settings.json hook parsing + scope authority | API/Backend (`internal/ecosys`) | — | Hook discovery is config loading; the loader's tier-merge discipline already lives there |
| Hook verdict evaluation at PreToolUse | API/Backend (`internal/session` gate head, post-17) | executor seam (`internal/coreexec` ToolHooks) today | 17-CONTEXT D-05 locks ONE chokepoint at the session-level gate; the executor-level `withHooks` PreToolUse wrap becomes redundant (see Pitfall 2) |
| AGENTS.md/CLAUDE.md discovery + cache | API/Backend (`internal/ecosys` — memory.go NEW) | — | Filesystem discovery belongs with the other `.claude/`-layout discovery (skills, agents, hooks) |
| Memory injection into system context | API/Backend (`internal/runtime` sessionFor) | Shaper renders | The profile-copy merge site is runtime-owned by construction (runtime.go:1046–1075) |
| Thinking SSE parse → transcript → emitter | API/Backend (`internal/provider` → `internal/session` → `internal/acp`) | — | Wire parse in provider; transcript append in session; frame emission via the existing bus→forwarder→emitter chain |
| Thinking round-trip into outgoing requests | API/Backend (`internal/session` Projector + `internal/shaper`) | — | The projector builds mid-turn assistant messages; the shaper maps them to SDK params — thinking blocks ride as first-class blocks there |
| Image ingress (ContentBlock variant, downscale) | API/Backend (`internal/runtime` ingress + NEW pure-Go scale) | Shaper maps to `ImageBlockParam` | D-09's original-bytes-on-disk + provenance is an ingress concern, before transcript append |
| @-mention expansion | API/Backend (`internal/runtime` expandUserBlocks) | gate seam for Read rules | Same seam slash-commands use (CMDS-03 precedent); Read-rule consult delegates to the injected evaluator |
| Per-provider image capability validation | API/Backend (provider adapters) | — | Only the adapter knows its protocol's image support (Anthropic: yes; OpenAI-shape adapter: text-only today) |

## Standard Stack

### Core (all in-tree — no new frameworks)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| stdlib `encoding/json` | go1.26 | `hookSpecificOutput` verdict parse; raw_thinking `json.RawMessage` storage (D-12) | `json.RawMessage` verbatim passthrough is the by-construction byte-identity mechanism |
| stdlib `os/exec`, `regexp`, `time` | go1.26 | Hook execution (existing runner extends in place) | Already proven in hooks.go |
| `internal/ecosys` HookRunner | in-tree | The hook vehicle PAR-03 extends | "PAR-03 adds scope parsing + the JSON verdict channel, not a new runner" (21-CONTEXT Reusable Assets) |
| `internal/session` transcript + projector | in-tree | raw_thinking line (16-02) + replay into outgoing requests | 16-D-20 additive-only schema; projector is the context machinery |
| `anthropics/anthropic-sdk-go` | v1.63.0 (pinned in go.mod, verified) | `ThinkingBlockParam`, `RedactedThinkingBlockParam`, `ImageBlockParam`, `Base64ImageSourceParam` — the outgoing-request block shapes | First-party SDK; the exact param types verified in the module cache this session |

### Supporting (new dependency: exactly one)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/image` | v0.45.0 (latest, `go list -m -versions` verified) | `draw.CatmullRom`/`draw.ApproxBiLinear` scalers for D-09 auto-downscale; optional `webp` decode | Only when an ingress image exceeds provider limits (D-09) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `golang.org/x/image/draw` | `disintegration/imaging` | imaging is pure-Go with a friendlier API but is effectively unmaintained (no release since 2022-era); x/image is official golang.org/x governance — same family as the already-pinned golang.org/x/net. D-09 makes this dependency part of the build contract: governance wins. |
| Denying over-limit images | Auto-downscale (D-09) | LOCKED — operator chose UX over rejection. |
| Extending `go-openai` to multipart content for images | D-11 ingress-validation degrade | The OpenAI-shape adapter is text-only today (verified: no `MultiContent`/`image` in internal/provider/openai.go); D-11 explicitly blesses drop+loud-note for providers without support. Adding MultiContent is a larger parity surface than PAR-06's letter requires. |

**Installation:**
```bash
go get golang.org/x/image@v0.45.0
```

## Package Legitimacy Audit

> This phase installs exactly one external package. The `gsd-tools package-legitimacy check` seam supports npm|pypi|crates only — Go modules fall back to registry + governance verification per Step 2 of the gate.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `golang.org/x/image` | Go module proxy (`go list -m -versions` → v0.45.0 latest, verified this session) | ~10 yrs (x-repo since 2015) | n/a (Go) | github.com/golang/image (official golang.org/x) | OK | Approved — pure Go, CGO_ENABLED=0-safe (pkg.go.dev draw package: "superset of and a drop-in replacement for image/draw") |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none
*No npm/PyPI packages are installed by this phase; hook commands are operator-supplied config executed at runtime (existing trust tier), not dependencies.*

## Architecture Patterns

### System Architecture Diagram

```
 LEG A: HOOKS (PAR-03)                          LEG B: MEMORY (PAR-04)
 .claude/settings.json (project)                 ./AGENTS.md|CLAUDE.md … up to git root
 ~/.claude/settings.json (user)                  ~/.ass-guard/AGENTS.md|CLAUDE.md (wins)
        │  parse scopes                                  ~/.claude/CLAUDE.md
        ▼                                                        │ walk + collide-check
 ecosys settings-hook loader ──► HookConfig{Scope}               ▼
        │                          │                      memory cache (mtime-keyed)
        │            plugin hooks.json (existing, unchanged)   │
        ▼                          ▼                            ▼
 ┌──────────────────────────────────────────┐   runtime.sessionFor profile COPY
 │ PRETOOLUSE VERDICT RESOLVER (deny-wins)  │   ── trailing TextBlock append ──►
 │ project→user→plugin order (D-03)         │          system[] of outgoing request
 │ JSON verdict ∥ exit-2 legacy (D-02)      │
 │ user allow ∥ project deny-only (D-01)    │   LEG C: THINKING (PAR-05)
 └───────────────┬──────────────────────────┘   provider SSE stream
                 │ verdict                          │  content_block_delta:
                 ▼                                  │   thinking_delta / signature_delta
 [POST-17] session.gateCall head (replaces          ▼
 implicit-allow no-op) ─► gateDeny │               SSE state machine (streaming.go)
        gateSuspend(ask) │ gateExecute                     │ StreamChunk{Type:"thinking", Raw}
                 │                                                ▼
                 ▼ (post-17: rule eval → ask step)   session.streamAndEmit
           tool executes or denied                            │
                                                    ┌─────────┼──────────────┐
                                                    ▼         ▼              ▼
                                          AppendRawThinking  bus event   transcript
                                          (redactor-exempt,  AgentThought   (json.RawMessage
                                           manager.go:349)   Chunk           verbatim, D-12)
                                                              │                    │
 LEG D: RICH CONTENT (PAR-06)                                 ▼                    ▼
 user prompt []ContentBlock                        routeBusEvent → emit     Projector: raw_thinking
        │ .Text with @mentions / image block       .ThoughtChunk →          lines → assistant msg
        ▼                                          agent_thought_chunk      thinking/signature blocks
 runtime expandUserBlocks (runtime.go:395)         (emitter.go:516)         → shaper → outgoing req
        │ @file→content (Read-rule consult)                                 (bytes identical, D-14 goldens)
        │ @dir→one-level listing
        ▼
 image ingress: dims/bytes validate → over-limit?
        │ yes → x/image draw downscale (pure Go, D-09)
        │ no support? → drop + loud note (D-11)
        ▼
 transcript user_message (provenance: resize/mention records) → shaper → provider
```

### Pattern 1: Settings-scope hook loading joins the flat registry slice
**What:** Parse `hooks` from project `.claude/settings.json` and user `~/.claude/settings.json` with the SAME `hooks → event → matcher-group → {type,command,timeout}` shape the plugin `hooks.json` parser already handles (hooks.go:124–182), tagging each `HookConfig` with a scope enum. Do NOT restructure `Registry.Hooks`; add `Scope` to `HookConfig` and resolve D-03's firing order at resolution time.
**When to use:** Plan 1.
**Why in-place:** 21-CONTEXT: "HookRunner … PAR-03 adds scope parsing + the JSON verdict channel, not a new runner." The existing parse-and-skip-with-warning degradation discipline (hooks.go:141–146) is the established pattern for malformed settings.

CC settings shape (what the loader must accept) [CITED: code.claude.com/docs/en/hooks]:
```json
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Edit|Write",
        "hooks": [ { "type": "command", "command": "./deny-edits.sh", "timeout": 30 } ] }
    ]
  }
}
```

The JSON verdict channel (D-02's parity target) [CITED: code.claude.com/docs/en/hooks]:
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "Destructive command blocked by hook"
  }
}
```

### Pattern 2: Verdict resolution — deny-wins over an ordered hook list
**What:** A pure function `resolveVerdict([]scopedResult) Verdict` where scoped results arrive in D-03 order (project → user → plugin). ANY deny (JSON `permissionDecision:"deny"` OR legacy exit-2) wins, carrying the first denying hook's reason; user-scope `allow` applies only if no deny; project-scope `allow` is demoted to a loud ignored-allow warning (D-01); `ask` survives unless a deny exists (D-04). Hook EXECUTION failure (timeout, non-2 non-zero exit, spawn error) is fail-open: the hook contributes NO verdict (PAR-03 letter; CC parity: a timed-out PreToolUse command hook "does not block — the call proceeds through the normal permission flow" [CITED: code.claude.com/docs/en/hooks]).
**When to use:** Plan 1 (in `internal/ecosys`, fully testable without the gate).
**Key:** keep this resolver SEPARATE from the runner's `Fire` loop so the 17-join is a one-line delegation later.

### Pattern 3: Memory walker + mtime cache + fourth profile-copy merge
**What:** `internal/ecosys` gains a memory discovery func: walk cwd → git root (stop at `.git`), at each level collect `CLAUDE.md` (wins) or `AGENTS.md` (D-05 collision rule), then the user global pair with `~/.ass-guard` precedence (D-07). Cache file content keyed by path+mtime+size. Injection appends ONE trailing `profile.TextBlock` (framing text + per-level bodies, per-file cap ~16–32 KB + total budget, truncation notes loud — D-08) to the session profile copy at the established site:
```go
// internal/runtime/runtime.go:1056-1059 — the vehicle (verbatim pattern):
if listing := ecosys.SkillListing(r.reg); listing != "" {
    prof.System = append(append([]profile.TextBlock(nil), prof.System...),
        profile.TextBlock{Type: blockText, Text: listing})
}
```
[VERIFIED: internal/runtime/runtime.go:1056-1059 — quote above verbatim; the memory merge joins at this exact site]
**When to use:** Plan 2. The mtime cache lives package-level in the discovery func (sessions are constructed repeatedly against the same files); it is a pure optimization — a cache miss re-reads, never stales content within a session (session-start injection only, per criterion 3).

### Pattern 4: Thinking pipeline — five hops, D-12 invariant at each
1. **SSE state machine** (internal/provider/streaming.go:265 `drainSSE`): extend the existing tool-use lifecycle tracking with a thinking-block accumulator — `content_block_start` with `content_block.type == "thinking"` opens it; `thinking_delta` (`delta.thinking`) accumulates text; `signature_delta` (`delta.signature`) accumulates the signature; `content_block_stop` emits `StreamChunk{Type:"thinking", Raw: <assembled block JSON>}`. `redacted_thinking` arrives fully-formed in `content_block_start` (`data` field) — emit immediately. CC doc discipline: thinking streams as `thinking_delta` "followed by a single `signature_delta` event just before the block's `content_block_stop`" [CITED: platform.claude.com/docs/en/build-with-claude/thinking].
2. **StreamChunk** (provider.go:46–58): add a `"thinking"` Type + `Raw json.RawMessage` payload (Raw already exists on the struct for the done chunk).
3. **Session** (`streamAndEmit`, session.go:663–701): case `"thinking"` → `s.Manager.AppendRawThinking(turnID, model, chunk.Raw)` + `s.Bus.Publish(event.AgentThoughtChunk{...})` (new event kind, mirroring `event.AgentMessageChunk`).
4. **Forwarder** (runtime.go:648–661 `routeBusEvent`): new case → `emit.ThoughtChunk(messageID, content)` — the emitter method exists (`EmitterHandle.ThoughtChunk`, emitter.go:516, frame kind `agent_thought_chunk` emitter.go:23 [VERIFIED: internal/acp/emitter.go:23,516]).
5. **Projector** (internal/session/projector.go — no raw_thinking handling today, verified): raw_thinking lines project into the mid-turn assistant message as thinking blocks; the shaper maps them via `anthropic.ThinkingBlockParam{Signature, Thinking}` / `RedactedThinkingBlockParam{Data}`. Byte-identity contract, precisely: transcript stores the assembled block as `json.RawMessage` verbatim (D-12); the projector extracts the `thinking`/`signature`/`data` field VALUES untouched and the SDK re-serializes them — golden fixtures (D-14) pin field-value identity provider→transcript→projector→outgoing. The Anthropic preservation law making this load-bearing: "Modified thinking blocks are rejected with a 400 error"; redacted_thinking carries encrypted `data`, NO signature, and "must be passed back unchanged" — code that filters only `type == "thinking"` silently drops them and breaks the protocol [CITED: platform.claude.com/docs/en/build-with-claude/thinking].

SDK param types (pinned dependency, quoted from the module cache):
```go
// [VERIFIED: anthropic-sdk-go v1.63.0 message.go:6509-6516]
type ThinkingBlockParam struct {
	// "Thinking blocks must be passed back unmodified and in their original
	// order; a modified block results in a 400 `invalid_request_error`."
	Signature string `json:"signature" api:"required"`
	Thinking  string `json:"thinking" api:"required"`
	Type      constant.Thinking `json:"type" default:"thinking"`
	// ...
}
// [VERIFIED: anthropic-sdk-go v1.63.0 message.go:5192-5197]
type RedactedThinkingBlockParam struct {
	// "Opaque and encrypted; pass it back unchanged."
	Data string `json:"data" api:"required"`
	Type constant.RedactedThinking `json:"type" default:"redacted_thinking"`
	// ...
}
```

### Pattern 5: Rich-content ingress — validate, downscale, degrade loudly
1. `ContentBlock` (transcript.go:81–84) gains an image variant (e.g. `Type:"image"` + base64 `Data` + `MediaType` + provenance fields) — the transcript user_message line records the resize provenance D-09 requires.
2. `expandUserBlocks` (runtime.go:385–455, the seam CMDS-03 reuses) parses `@`-mentions from the first text block: `@file` → content (after the Read-rule consult via the injected evaluator — implicit allow until 17 lands), `@dir` → one-level listing with names+sizes (D-10); unresolvable → loud note, turn proceeds.
3. Image ingress: decode dimensions cheaply FIRST (`image.DecodeConfig` — no pixel allocation), validate against provider limits, downscale only when needed via `x/image/draw`, re-encode, store ORIGINAL bytes on disk, ship scaled bytes in the request.
4. Provider-shape validation: the Anthropic adapter accepts `ImageBlockParam`; the OpenAI-shape adapter has no image path today — D-11's drop + ONE loud note naming the provider fires there. Wire shape [CITED: platform.claude.com/docs/en/build-with-claude/vision; VERIFIED: anthropic-sdk-go v1.63.0 message.go:3319-3323,120-124]:
```json
{ "type": "image",
  "source": { "type": "base64", "media_type": "image/jpeg", "data": "<base64 bytes>" } }
```

Downscale sketch [CITED: pkg.go.dev/golang.org/x/image/draw]:
```go
import pcm "image"
import "golang.org/x/image/draw"

dst := pcm.NewRGBA(pcm.Rect(0, 0, newW, newH))
draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
```

### Anti-Patterns to Avoid
- **A second gate path:** evaluating hook verdicts anywhere except 17's `gateCall` head (17-CONTEXT D-05; 21 criterion 2). The runner RESOLVES verdicts; the gate CONSUMES them — one consumption site.
- **Typed re-serialization of thinking:** unmarshaling thinking bytes into a struct and re-marshaling for storage drifts bytes and widens the redactor surface. RawMessage in, RawMessage stored (D-12, 16-D-23). Field extraction happens ONLY at the projector, and values pass through untouched.
- **Filtering thinking by `type == "thinking"` only:** drops `redacted_thinking` (no signature, encrypted `data`) and breaks the round-trip protocol [CITED: platform.claude.com thinking docs].
- **Regex-only matcher parity claim:** see Pitfall 3 — the existing runner treats every matcher as a Go regex; CC's documented dialect has an exact/alternatives path first.
- **Blocking the turn on hook execution:** hooks are bounded-sync with hard timeout; a hung hook must time out and fail open, never wedge the tool loop (existing runner discipline, hooks.go:304–348).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Image downscaling | Custom pixel resampler | `golang.org/x/image/draw` Kernel scalers | Correct interpolation kernels, pure Go, official maintenance; D-09 makes the dep contractual |
| Image decode/encode | Format sniffing by magic bytes | stdlib `image/jpeg`, `image/png`, `image/gif` + `image.DecodeConfig` | Stdlib handles headers/formats; DecodeConfig reads dims WITHOUT decoding pixels (the image-bomb guard) |
| JSON verdict parsing | Custom scanner for hook stdout | `encoding/json` with the documented "first non-whitespace char is `{`" gate | CC's documented parse rule is trivially expressible; schema-invalid JSON → non-blocking (proceed) per CC docs |
| Raw thinking storage | A typed Thinking struct in the transcript | `json.RawMessage` (D-12) | By-construction byte identity; the type system already carries it (16-D-23 precedent) |
| Anthropic block params | Hand-marshaled JSON for thinking/image blocks | SDK `ThinkingBlockParam` / `RedactedThinkingBlockParam` / `ImageBlockParam` (v1.63.0, verified) | The SDK owns wire-shape correctness; hand-rolled blocks risk the 400 family |

**Key insight:** every new surface in this phase either (a) extends an existing runner/parser in place, or (b) maps to a verified SDK param type. The only genuinely new algorithm is the memory walker (~50 lines of filepath.WalkDir + collision rules) — everything else is composition.

## Common Pitfalls

### Pitfall 1: Scope order inversion — the loader fires plugin hooks FIRST today
**What goes wrong:** `loadAll` prepends plugin hooks before core-tree hooks (loader.go:108–111: `hooks = append(hooks, pluginHooks...); merged.Hooks = append(hooks, merged.Hooks...)`), and within the core tree the merge is user→project (project overlays user). D-03 requires project settings → user settings → plugin bundles.
**Why it happens:** the flat slice preserves MERGE order, not FIRING order; the existing order was never observable because all hooks were plugin-scope.
**How to avoid:** add `Scope` to `HookConfig` (values derivable from Path at load, or set explicitly by the settings parser) and sort/partition at runner construction — don't reorder the loader's merge (other consumers depend on overlay precedence).
**Warning signs:** a test asserting a project deny wins over a user deny for the same tool — inverting load order breaks the "first denying hook's reason" clause of D-03.

### Pitfall 2: Double hook firing — executor wrap AND gate head both run PreToolUse
**What goes wrong:** today PreToolUse fires inside `coreexec.withHooks` (register.go:72–88, wired at runtime.go:1106–1110). After 17 lands, `gateCall`'s hook-verdict head also consults hooks. If both stay active, every matching hook executes twice, and a deny surfaces through two different result forms.
**Why it happens:** 12-02 put the seam at the executor because there was no session-level gate; 17 moves the policy decision upstream without removing the old site.
**How to avoid:** the 17-join plan must consolidate: hook evaluation lives at the gate head; `withHooks`' PreToolUse leg is removed or reduced to a pass-through while PostToolUse observation stays executor-side. Grep audit post-join: exactly one `PreToolUse` consultation site (mirrors 17-05's chokepoint-straggler grep).
**Warning signs:** hook side effects (marker files, log lines) firing twice per call in the join plan's E2E tests.

### Pitfall 3: Matcher dialect divergence — regex vs CC's two-path syntax
**What goes wrong:** the runner compiles every non-empty matcher as a Go regex (hooks.go:276–284). Current CC docs define a two-path dialect: matchers containing only `[alphanumeric/_/-/space/,/|]` are EXACT-string-or-alternatives; anything else is an unanchored regex [CITED: code.claude.com/docs/en/hooks]. So CC's `"Edit"` matches only the tool named `Edit`, while the runner's regex `Edit` also matches `NotebookEdit`.
**Why it happens:** the 12-02 implementation predated the current documented dialect (and was grounded in the plugin-hook corpus where matchers were regex-intended).
**How to avoid:** settings.json parsing (Plan 1) implements the two-path dialect for the new scope hooks (exact path = anchored compare; alternatives split on `|`/`,`; regex path = unanchored compile — Go `regexp` is compatible for the regex path). Decide whether to migrate plugin hooks too or keep their legacy behavior (21-CONTEXT D-02: "existing plugin hooks unchanged" → keep plugin behavior, apply the dialect to settings hooks; document the divergence).
**Warning signs:** a settings hook with matcher `Task` firing on `TodoWrite`-class tools in substring collisions.

### Pitfall 4: Exit-0 silent hooks misread as allow
**What goes wrong:** CC semantics: "A handler that matches but stays silent (exit 0, no output) does not approve the call — the normal permission flow applies" [CITED: code.claude.com/docs/en/hooks]. A naive resolver treating exit-0 as allow would widen trust — the exact hole D-01 exists to close.
**How to avoid:** three-valued per-hook outcome: {deny, allow, ask, NO-DECISION}; exit-0-without-valid-JSON and schema-invalid JSON both yield NO-DECISION (proceed). Only explicit `permissionDecision:"allow"` from USER scope produces allow.
**Warning signs:** resolver unit test "silent hook + no rules → ask-class still asks (gated) / executes via ungated rule path — never via hook allow".

### Pitfall 5: Thinking dropped or reordered by the projector's reset-boundary logic
**What goes wrong:** the projector cuts at reset boundaries (`splitAtResetBoundary`, projector.go:123); raw_thinking lines landing on the wrong side of a boundary silently break the "consecutive thinking-block sequence must match what the model generated" requirement for the CURRENT assistant message, producing provider 400s.
**Why it happens:** thinking blocks belong to a specific assistant message; the projection window must keep the thinking WITH its assistant_message/tool batch or drop BOTH (never think-without-its-turn).
**How to avoid:** project raw_thinking as part of the assistant-message accumulation (`accumulateMidTurn`, projector.go:170) keyed by turnID, so boundary cuts take the whole turn unit; Phase 19's locked discipline (19-CONTEXT D-04: "thinking chains never rewritten") applies to compaction cuts — Phase 21 must not position thinking as separately-cuttable content.
**Warning signs:** golden-fixture test with a boundary in the middle of a thinking+tool_use turn fails with a 400-shaped mismatch.

### Pitfall 6: Image decode memory blowups (pixel bombs)
**What goes wrong:** a small PNG can decode to a giant pixel buffer; naive `image.Decode` before validation allocates the full RGBA (an 8000×8000 frame is ~256 MB) — in a session process, repeated per turn.
**How to avoid:** `image.DecodeConfig` FIRST (header-only), validate dimensions/format/byte-size against provider limits, and only then decode pixels for downscale. Cap accepted decode dimensions defensively even when within limits.
**Warning signs:** memory spikes in the ingress unit tests with adversarial fixtures; add one explicit oversized-dims fixture to the plan.

### Pitfall 7: CGO creep via the image dependency
**What goes wrong:** D-09 pins pure-Go processing because `CGO_ENABLED=0` is the build gate (mise `[tasks.build]`: `CGO_ENABLED=0 go build ./...` [VERIFIED: .mise.toml tasks.build]); any imaging library that wraps C breaks the static-binary claim.
**How to avoid:** only `golang.org/x/image` (+ stdlib image packages); never `disintegration/imaging`'s cgo variants or libvips bindings. The `mise ci` build task fails loudly if cgo is required — keep it that way.
**Warning signs:** build failure under the gate = dependency regression, remove immediately.

### Pitfall 8: Settings.json is live operator config read per session — parse failures must degrade loudly, not fatally
**What goes wrong:** a malformed project settings.json killing session construction would let a repo brick the agent (and the malformed-file attack equals the repo-trust concern D-01 mitigates).
**How to avoid:** follow the existing hooks.json discipline verbatim — malformed → `logPluginSkipf`-style stderr warning + skip (hooks.go:141–146 pattern); scope verdicts degrade fail-open.
**Warning signs:** any code path where settings parse errors return an error up through `Load`/sessionFor.

## Code Examples

### Hook stdin payload (already implemented — the contract settings hooks inherit)
```go
// [VERIFIED: internal/ecosys/hooks.go:382-392]
payload := map[string]any{
	"session_id":      r.SessionID,
	"transcript_path": r.TranscriptPath,
	"cwd":             r.WorkDir,
	"hook_event_name": event,
}
```
PreToolUse adds `tool_name` + `tool_input` via `fields` (hooks.go:232–241) — matching CC's documented input shape [CITED: code.claude.com/docs/en/hooks].

### Existing runner bounds (extend, don't re-decide)
```go
// [VERIFIED: internal/ecosys/hooks.go:91-97]
const (
	hookDefaultTimeoutSec = 60
	hookOutputCap         = 30000
	hookRefusalExitCode   = 2
)
```
CC's current documented command-hook default is 600s (30s for UserPromptSubmit-attached) [CITED: code.claude.com/docs/en/hooks]. The discrepancy is Claude's-discretion territory (timeout value); the plan should pick one and state it — keeping 60s bounds tool-loop latency; matching 600s matches CC.

### The 17-locked verdict vocabulary Phase 21 maps onto
```text
// [VERIFIED: .planning/phases/17-permissions-elicitation/17-02-PLAN.md artifacts list]
gateExecute / gateDeny / gateSuspend / gateDeclineAutomation   (gateVerdict actions)
// [VERIFIED: .planning/phases/17-permissions-elicitation/17-01-PLAN.md line 81]
VerdictDeny / VerdictAsk / VerdictAllow / Unmatched            (internal/perm RuleSet)
```
Hook mapping: deny→gateDeny (reason = first denying hook's reason, D-03); ask→gateSuspend even in ungated mode (D-04); allow (user scope only, D-01)→gateExecute. `Unmatched` defers to the mode+class decision exactly as 17-01 line 95 states.

### Transcript line for thinking (already built — producer and consumer are Phase 21's)
```go
// [VERIFIED: internal/session/transcript.go:50-58]
TypeRawThinking = "raw_thinking"
// payload in Content (json.RawMessage — never re-serialized) + provider model
// attribution in Model; the line takes the UNREDACTED append path (the
// Redactor is never invoked on thinking bytes) — exemption is TYPE-SCOPED
// (manager.go:106-112 appendLineUnredacted, sole caller AppendRawThinking:349).
```

### The stream chunk switch that grows a thinking case
```go
// [VERIFIED: internal/session/session.go:663-701 — existing cases]
switch chunk.Type {
case blockText:    // … text buffer + AgentMessageChunk publish
case blockToolUse: // … ToolCall publish
case "usage":      // … UsageUpdate publish
case stopDone:     // … finish reason + raw
case chunkErrorType: // … mid-stream abort
// PAR-05 adds: case "thinking" → AppendRawThinking + AgentThoughtChunk publish
}
```

### CC scope-precedence context for D-01 [CITED: code.claude.com/docs/en/hooks]
CC merges hook entries across settings levels (user/project/local/managed) rather than replacing, dedupes identical handlers, and gates repo-shipped hooks behind workspace trust. ass-guard has NO trust dialog — D-01's deny-only-from-project-scope IS the substitute mitigation; the plan's security notes should say so explicitly.

## Runtime State Inventory

> Not a rename/refactor/migration phase — omitted per protocol. (No strings are renamed; new state introduced — the mtime memory cache — is ephemeral in-process, rebuilt from disk on miss.)

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| go | everything | ✓ | 1.26 (mise pin [tools] go="1.26"; go.mod `go 1.26`) | — |
| golangci-lint | `mise ci` | ✓ | 2.x (mise pin) | — |
| testify | tests | ✓ | v1.11.1 (go.mod, verified) | stdlib testing |
| `golang.org/x/image` | D-09 downscale | ✗ (not yet in go.mod — verified `go list -m`) | v0.45.0 available | Deny-over-limit instead of downscale would drop the dep — but D-09 LOCKS auto-downscale, so `go get` lands in Wave 0 |
| `sh` (hook exec) | hook tests + runtime | ✓ (macOS/Linux per platform constraint) | — | — (Windows deferred project-wide) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** `golang.org/x/image` — one `go get` in the owning plan's first task (it becomes part of the build contract per D-09's reversibility note).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `stretchr/testify` v1.11.1; `-race` always (mise `[tasks.test]`) |
| Config file | none (Go convention; golangci-lint v2 config in repo root) |
| Quick run command | `go test -race -count=1 ./internal/ecosys/ ./internal/session/ ./internal/provider/ ./internal/shaper/ ./internal/runtime/` |
| Full suite command | `mise ci` (vet + lint + CGO_ENABLED=0 build + race test — the standing phase gate) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PAR-03a | settings.json scope parse (project+user), malformed-skip warnings | unit | `go test -race ./internal/ecosys/ -run TestSettingsHooks -count=1` | ❌ extend hooks_test.go (exists) |
| PAR-03b | JSON verdict + exit-2 both parse; silent exit-0 = no-decision | unit | `go test -race ./internal/ecosys/ -run TestHookVerdict -count=1` | ❌ Wave 0 (extend hooks_test.go) |
| PAR-03c | deny-wins ordering project→user→plugin; project allow ignored loud | unit | `go test -race ./internal/ecosys/ -run TestHookScopeOrder -count=1` | ❌ Wave 0 |
| PAR-03d | hook verdict joins gateCall head; ask routes to ask step (POST-17) | integration | `go test -race ./internal/session/ -run TestGateHookVerdict -count=1` | ❌ blocked-on-17 (extends 17-02's gate_test.go — precondition-marked) |
| PAR-04 | memory discovery (collision, repo-root stop, user-global precedence), mtime cache, trailing-block injection, caps | unit+integration | `go test -race ./internal/ecosys/ -run TestMemoryDiscovery && go test -race ./internal/runtime/ -run TestMemoryInjection -count=1` | ❌ Wave 0 (new memory_test.go / runtime test) |
| PAR-05a | SSE thinking_delta/signature_delta/redacted_thinking → StreamChunk | unit | `go test -race ./internal/provider/ -run TestStreamThinking -count=1` | ❌ extend streaming_test.go (exists) |
| PAR-05b | transcript append redactor-exempt (zero redactor calls); emitter thought-chunk forward | unit | `go test -race ./internal/session/ -run TestRawThinking && go test -race ./internal/runtime/ -run TestThoughtForward -count=1` | partial — manager_test.go covers append path (16-02); forwarder test ❌ Wave 0 |
| PAR-05c | golden fixtures: captured thinking+signature+redacted pairs through transcript→projector→outgoing request | golden | `go test -race ./internal/session/ -run TestThinkingGolden -count=1` | ❌ Wave 0 (testdata goldens from profiles/zcode corpus) |
| PAR-06a | @-mention parse/expand (@file content, @dir listing, unresolvable note) | unit | `go test -race ./internal/runtime/ -run TestMentionExpand -count=1` | ❌ Wave 0 |
| PAR-06b | image ingress validate + downscale + provenance; DecodeConfig bomb guard | unit | `go test -race ./internal/runtime/ ./internal/shaper/ -run TestImageIngress -count=1` | ❌ Wave 0 |
| PAR-06c | provider-shape validation: Anthropic accepts image block; unsupported provider drops + loud note | unit | `go test -race ./internal/provider/ ./internal/shaper/ -run TestImageCapability -count=1` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** the owning package's `-run` filter (each < 30 s)
- **Per wave merge:** `go test -race -count=1 ./...`
- **Phase gate:** `mise ci` green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/ecosys` — settings-hook fixtures (`testdata/settings-project.json`, `testdata/settings-user.json`) for PAR-03a–c
- [ ] `internal/session` — golden thinking fixtures under testdata (captured wire pairs; redaction mechanics per discretion) for PAR-05c
- [ ] `internal/runtime` — memory walker fixtures (nested-dir repo tree, both-globals tree) for PAR-04
- [ ] Image test fixtures (oversized JPEG + pixel-bomb PNG with big DecodeConfig dims) for PAR-06b
- [ ] Framework install: `go get golang.org/x/image@v0.45.0` in the image plan's first task
- The PAR-03d gate-join test file is 17-02's `gate_test.go` — created by Phase 17, EXTENDED by Phase 21 (precondition-marked, not Wave 0)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V4 Access Control | **YES — this phase's core** | Deny-wins verdict resolution; scope-split authority (D-01: repo-shipped files NEVER grant allow); ONE gate consumption site (17-D-05); fail-open on execution failure but NEVER on conflicting verdicts (D-03) |
| V5 Input Validation | YES | Hook JSON output schema validation (invalid → no-decision, never honored); image dims/bytes/media-type validation before decode; @-path resolution bounded to repo root + declared dirs |
| V2/V3 Auth/Session | no | No new auth/session surfaces (hook env stays sanitized: PATH+HOME+CLAUDE_PLUGIN_ROOT only — hooks.go:400–414 [VERIFIED]) |
| V6 Cryptography | no (consumer only) | Signature verification is provider-side; ass-guard's duty is byte-faithful passthrough — never parse or recompute signatures |
| V8 Data Protection | YES | raw_thinking is unredacted BY DESIGN (T-16-04 type-scoped exemption must never widen — 16-02 locked); image bytes in transcript inherit the redacted-path treatment; no secrets in hook payloads (existing sanitized env) |

### Known Threat Patterns for this phase

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Repo-shipped settings.json grants itself allow (malicious repo) | Elevation of Privilege | D-01 deny-only from project scope; ignored-allow emits a loud structured warning; test pins "project allow + no user hooks → ask/normal flow, never allow" |
| Hook verdict spoofing via exit-code games (exit 2 with allow JSON) | Tampering | CC-verified rule: exit 2 blocks regardless of JSON ("even a JSON `permissionDecision` of `"allow"` can't override it") [CITED: code.claude.com/docs/en/hooks] — mirror exactly |
| Chokepoint bypass via subagent branch | Elevation of Privilege | The gate runs in BOTH runTurn branches (17-02 T-17-04 family); the join plan inherits those tests and adds hook-verdict coverage to both branches |
| Image pixel bomb / decompression DoS | DoS | `image.DecodeConfig` before decode; dimension caps; per-image and per-request byte caps (10 MB/image class [CITED: platform.claude.com vision]) |
| Prompt injection via @file content or memory files | Tampering (content-level) | Expansion is gated by Read rules (PAR-06 letter); memory files capped (D-08) with truncation notes; provenance lines make injected content auditable in the transcript |
| Unresolvable `@path` probing filesystem | Information Disclosure | D-10: loud note + proceed; resolution bounded to repo root (the same boundary as the memory walk); no existence oracle beyond the note's wording |
| Hook env leakage | Information Disclosure | Existing sanitized env (PATH/HOME/CLAUDE_PLUGIN_ROOT only) unchanged |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | CC `permissionDecision:"ask"` routes the call to the user prompt (the third documented value beside allow/deny) — deny is fully doc-confirmed, allow doc-attested, but the ask value's exact doc text was not quotable from the truncated hooks page this session; D-02/D-04 lock it regardless | Pattern 2 / Code Examples | LOW — the locked decisions define ass-guard's behavior; only the parity framing (not the feature) depends on CC's exact wording |
| A2 | CC hook timeout default is 600s for command handlers (current docs) vs the runner's existing 60s default (code comment claims CC documented 60s) — the planner picks per discretion | Standard Stack / Pitfall notes | LOW — bounded either way; the only cost is parity fidelity in a log line |
| A3 | The `agent_thought_chunk` ThoughtChunk frame accepts an arbitrary content block (text today) sufficient for streamed thinking fragments | Pattern 4 | LOW — emitter method exists and takes ContentBlock; worst case the plan widens the frame payload additively (16-D-20 discipline) |
| A4 | Golden thinking fixtures can be sourced from the captured corpus (`profiles/zcode/`) or synthesized from real captured wire pairs at plan time | Pattern 4 / Validation | MEDIUM — if the corpus lacks thinking-bearing responses, fixtures must be captured fresh (a capture run) before the golden tests can exist; the plan should include a capture step as contingency |
| A5 | Per-file memory cap ~16–32 KB and total budget exact values are planner-chosen (D-08 discretion); suggested: 24 KB/file, 64 KB total | Pattern 3 | LOW — pure tuning |
| A6 | `StreamChunk.Type` string `"thinking"` and `event.AgentThoughtChunk` kind naming follow the existing string-const conventions (blockText/blockToolUse style) | Pattern 4 | LOW — naming only; goconst lint enforces the convention mechanically |

## Open Questions (RESOLVED — all four adopted by plans; resolutions recorded 2026-08-28 at plan revision)

1. **Executor-level PreToolUse disposal after the gate join**
   - What we know: `withHooks` fires PreToolUse at the executor today; post-join the gate head owns verdicts (Pitfall 2).
   - What's unclear: whether to delete the PreToolUse leg of `withHooks` in Phase 21 or keep it as a no-op seam for kit-extraction symmetry (Phase 25).
   - Recommendation: delete the PreToolUse consultation (keep PostToolUse), and let Phase 25 re-home if the kit needs the seam; a dead policy seam is a bypass risk, not a convenience.
   - **RESOLVED:** recommendation adopted — 21-06 Task 2 deletes the executor PreToolUse leg (keeping PostToolUse) with the single-consultation grep audit as its acceptance gate; Phase 25 re-homes if the kit needs the seam.

2. **Plugin-hook matcher dialect migration scope**
   - What we know: settings hooks should use CC's two-path dialect (Pitfall 3); D-02 says "existing plugin hooks unchanged."
   - What's unclear: whether "unchanged" covers matcher semantics or only the JSON verdict channel compatibility.
   - Recommendation: unchanged = plugin hooks keep the current regex-everything behavior; settings hooks get the dialect. Document as a known divergence in the hook-runner doc comment.
   - **RESOLVED:** recommendation adopted — 21-01 Task 1 applies the two-path dialect to scopeProject/scopeUser hooks only and pins the split with table rows (settings matcher "Edit" does not match "NotebookEdit"; plugin matcher "Edit" still does); the divergence is documented in the hook-runner doc comment.

3. **Where image bytes live in the transcript vs request**
   - What we know: D-09 requires original bytes preserved on disk + provenance in the transcript; the request carries (possibly downscaled) base64.
   - What's unclear: whether the transcript stores the base64 payload inline (transcript bloat: 10 MB/image) or a file reference + hash with the audit body store (the 09-05 request_shaped precedent: metadata-in-line, body-by-Ref).
   - Recommendation: follow the 09-05 discipline — transcript line carries dims/media-type/resize-provenance + Ref; full bytes live on disk (originals) and in the request only. Planner confirms against transcript-size budgets.
   - **RESOLVED:** 09-05 discipline adopted and confirmed against transcript-size budgets — 21-05 Task 1 persists original+scaled bytes under the session .ass-guard images dir (sha-keyed) and carries Ref + metadata + resize provenance in the transcript line, with an explicit no-base64-in-transcript test.

4. **`ask` in ungated mode before Phase 17 lands**
   - What we know: D-04 locks ask-routes-to-ask-step "even in ungated mode."
   - What's unclear: nothing semantically — but mechanically the ask step is 17-02's gateSuspend+queue, so the D-04 behavior cannot exist until the join lands.
   - Recommendation: the join plan implements D-04 fully; pre-join, the resolver emits the verdict but the consumption site (gate) doesn't exist — which is exactly why the join is the final, precondition-marked plan.
   - **RESOLVED:** recommendation adopted — 21-06 Task 1 (the final, Phase-17-preconditioned tracer) implements D-04 fully (ask → gateSuspend unconditionally, ungated included), with the hookless-ungated zero-dialog control pinned in the same battery.

## Sources

### Primary (HIGH confidence)
- In-repo source read this session: internal/ecosys/hooks.go (runner contract, bounds, payload, env sanitization); internal/coreexec/register.go (withHooks seam); internal/runtime/runtime.go:385-455/500-560/586-680/1000-1140 (expandUserBlocks, forwarder, routeBusEvent, sessionFor profile-copy + hook chokepoint); internal/session/session.go:176-234/349-580/644-712 (Prompt, runTurn, streamAndEmit); internal/session/transcript.go:40-156 (Line + ContentBlock + additive kinds); internal/session/manager.go:106-112/344-358 (appendLineUnredacted, AppendRawThinking); internal/session/projector.go (function map — no raw_thinking handling); internal/provider/provider.go (Provider iface, StreamChunk); internal/provider/streaming.go:256-385 (drainSSE state machine); internal/provider/openai.go (function map — no image support); internal/ecosys/loader.go:40-114 (tier merge); internal/ecosys/types.go:73-83 (Registry.Hooks); internal/shaper/shaper.go (Message, Shape, toMessageParams, ComposeRuntimeWorkDir); internal/profile/types.go:22-47 (Profile); internal/acp/emitter.go:23/413-521 (ThoughtChunk); internal/acp/types.go:171-176 (ThoughtChunkFrame); internal/event/bus.go; go.mod; .mise.toml
- anthropic-sdk-go v1.63.0 (pinned dependency, module-cache read): message.go:120-124 Base64ImageSourceParam, :3319-3345 ImageBlockParam + SourceUnion, :5192-5200 RedactedThinkingBlockParam, :6509-6521 ThinkingBlockParam, :4409-4460 ThinkingDelta/SignatureDelta — quoted verbatim in Code Examples
- `.planning/phases/17-permissions-elicitation/17-CONTEXT.md` (D-01..D-13), `17-01-PLAN.md` (internal/perm contract), `17-02-PLAN.md` (gateCall seam, verdict enum, preconditions)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` (D-01..D-23), `.planning/phases/18-session-family/18-CONTEXT.md` (D-01/D-02 transcript-as-truth), `.planning/phases/19-compaction-cache-control/19-CONTEXT.md` (D-04 thinking-never-rewritten)
- code.claude.com/docs/en/hooks — PreToolUse decision contract, exit codes, settings schema, matcher dialect, timeouts, multi-hook semantics (fetched in full via web reader)
- code.claude.com/docs/en/memory — memory hierarchy, walk span, AGENTS.md non-support, injection shape
- platform.claude.com/docs/en/build-with-claude/thinking — signature/redacted_thinking preservation law, 400-on-modification, streaming deltas
- platform.claude.com/docs/en/build-with-claude/vision — 8000×8000, 1568px, 10MB, formats, block JSON

### Secondary (MEDIUM confidence)
- pkg.go.dev/golang.org/x/image/draw — scaler docs (cross-checked against `go list` registry verification)
- Stack Overflow #22940724 + roeber.dev resize pattern — usage shape only (matches official example tests)

### Tertiary (LOW confidence)
- None — no claims rest on unverified web-only sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — one new dep, registry-verified, official governance; everything else is in-tree and read this session
- Architecture: HIGH — all four legs compose verified seams; the 17-dependent join is contract-locked verbatim in 17-01/17-02 plans
- Pitfalls: HIGH — each pitfall is grounded in read code (loader order, withHooks double-fire, matcher dialect) or doc-quoted external law (exit-2 precedence, silent-hook, redacted_thinking)
- External contracts: HIGH for hooks/thinking/vision/memory (official docs, full-page fetches); one LOW-confidence residue logged as A1

**Research date:** 2026-08-28
**Valid until:** 2026-09-27 (30 days — CC docs and SDK v1.63.0 are the moving parts; re-verify matcher dialect + hook timeout defaults if Phase 21 planning slips past September)
