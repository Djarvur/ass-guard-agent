# Phase 18: Session Family - Research

**Researched:** 2026-08-27
**Domain:** ACP session lifecycle (list/load/close/delete) + resume reconciliation + CC-parity CLI flags (Go)
**Confidence:** HIGH (codebase contracts read verbatim; ACP v1 schema and CC docs fetched from canonical domains this session)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Reconciliation Architecture**
- **D-01:** Transcript-as-truth: resumed live-state reconstructs from the transcript ALONE plus synthetic closures — no session-state sidecar to corrupt or drift under kill -9. Separately-persisted stores (schedule lastFired, learning) reload as today, outside session reconciliation. Any genuinely non-derivable state a planner encounters must justify a sidecar per-case through the deviation protocol. — **Reversibility:** one-way — the resume contract (single source of truth) becomes load-bearing for every later session feature; a sidecar added casually breaks kill-9-proofness.
- **D-02:** Synthetic closures carry provenance: replay shows them with an interrupted/synthetic marker — subtle in the UI, unambiguous on disk. Post-mortem of kill -9 cases stays possible (D-20 audit spirit).
- **D-03:** Reconcile-then-accept: session/load completes full replay + reconciliation BEFORE any new prompt is accepted (linearizable resume). No interleaving race between replayed frames and live turns; criterion 2's "no ghost state" is guaranteed by ordering.

**List & Pagination**
- **D-04:** On-demand header scan: session/list walks transcripts mtime-ordered with first-line reads — no index file, nothing to drift, append-only discipline preserved. Session counts are small; scan is milliseconds.
- **D-05:** Sort: lastActivity descending with a composite (lastActivity, id) cursor. Best-effort paging stability documented (a session that becomes active mid-paging can shift) — picker utility over paging purity.
- **D-06:** Lean header: sessionId, title (first user prompt, truncated), createdAt, lastActivity, state flags (checkpoint availability). Rich stats (tokens/cost) wait for /cost in Phase 20 rather than bloating the header.

**Delete & Tombstones**
- **D-07:** Tombstone = zero-byte marker file beside the transcript (`<id>.deleted`). List filters by stat (no read); transcript bytes are never touched; unambiguous and reversible on disk.
- **D-08:** Delete covers transcript + checkpoints + session-scoped derived artifacts; the audit trail survives untouched — separate policy, D-20 invariant verbatim.
- **D-09:** GC after grace (operator chose auto-purge): 30-day default, configurable through the configOptions menu (joins Phase 16's rule: full menu, apply-as-landed). Tombstoned artifacts are physically purged when grace expires; audit NEVER purges; reversible until the purge fires (remove marker). — **Reversibility:** one-way — purge destroys the transcript permanently; the grace window is the only undo.

**Resume CLI + Close**
- **D-10:** Full CC trio: `--resume` (no args) opens the interactive picker; `--resume <id|name>` resumes directly; `--continue`/`-c` resumes the most recent session with zero ceremony. Parity with Claude Code's documented surface.
- **D-11:** Terminal picker = numbered list on stdin/stderr (N recent sessions: title + relative time; user types a number). No raw-mode TUI dependency — works over pipes, ssh, plain TTY.
- **D-12:** session/close = cancel-and-drain: in-flight turns cancelled via the existing cancel contract, queued asks drained cancelled (17-D-13), buffers flushed, then closed. Re-close is idempotent (already-closed = success). Criterion 4's "stops its work cleanly" verbatim.

### Claude's Discretion
- Kill -9 test matrix design (roadmap-flagged: enumerate the live-state inventory during planning — the inventory derives from the transcript kinds + ask/pending registries).
- Title truncation length, relative-time formatting in the picker.
- Tombstone-purge sweep trigger point (session-start sweep vs close-time check) within the 30d contract.
- `--continue`'s directory-scoping (CC scopes to cwd) vs global most-recent — planner checks CC behavior and matches.

### Deferred Ideas (OUT OF SCOPE)
- Dedicated session-restore surface (un-delete UI/CLI beyond removing the marker file) — post-v1.2 if wanted.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ACP-05 | User can list sessions from the editor via `session/list` (header-scan, cursor pagination) | v1 wire shapes pinned (ListSessionsRequest/Response, SessionInfo incl. `updatedAt`); existing enumeration surface `SessionReader.Sessions()` + first-line header discipline; composite cursor pattern below |
| ACP-06 | User can resume any past session via `session/load` — full replay through TurnEmitter plus live-state reconciliation (synthetic interrupted-closures for dangling expectations, continued id sequences from transcript maxima, orphaned in-flight tool_calls closed as failed, commands re-advertised); `--resume` anywhere | v1 load contract (top-level `loadSession` capability, response shape with NO sessionId field); Live-State Inventory below enumerates every dangling-expectation class from the 16 transcript kinds; kill -9 durability argument; CC `--resume` semantics fetched |
| ACP-07 | User can close or delete a session via `session/close` / `session/delete` with tombstoning (never rm — D-20 audit invariant); delete is spec-unstable → best-effort | v1 close/delete shapes + capability gating; MUST-language "treat it as if session/cancel was called" matches D-12; tombstone file mechanics + existing `closeOnce` drain chain; delete instability documented (RFD origin) |
</phase_requirements>

## Summary

Phase 18 turns the transcript family — already an append-only, human-greppable, single-writer store — into the backing store of the ACP session lifecycle. Three of the four RPC methods are thin reads over machinery that exists today: `session/list` is a header scan over the same `.ass-guard/` directory `SessionReader` already enumerates; `session/close` is the existing `Close`/cancel contract aimed at an explicit RPC; `session/delete` is a marker-file write plus artifact sweep. The genuinely new engineering is `session/load`: full replay through Phase 16's ordered TurnEmitter plus live-state reconciliation, where the correctness surface is exactly the set of transcript kinds that can dangle when a process dies mid-turn.

The reconciliation inventory (roadmap flag) is fully enumerable from the code: the transcript has 16 line kinds [VERIFIED: internal/session/transcript.go:18-49], and of these, seven carry pair-or-state semantics that can be left open by kill -9 (tool_call without tool_result, user_message without terminal line, ask_suspended without resolution, subagent_dispatch without subagent_result, dangling agent_message_chunk stream, plan_mode state, turn id sequence). Every one has a defined synthetic closure with provenance (D-02). kill -9 itself is the *easy* crash class for durability: `appendLine` writes complete lines via single `write(2)` to an `O_APPEND` fd — data in the page cache survives process death; only OS crash/power loss risks a torn final line (the readers already skip non-conforming lines).

The external contracts are pinned: ACP v1 gates `session/list`/`close`/`delete`/`resume` under a nested `sessionCapabilities` object while `session/load` stays under the top-level `loadSession` flag the server already advertises (currently `false` — this phase flips it); Claude Code's `--continue` is directory-scoped ("Resumes the most recent interactive session in the current directory") and `--resume <id>` works from any directory — resolving the D-10 discretion item in favor of cwd-scoped `--continue`, matching CC.

**Primary recommendation:** Build reconciliation as a pure transcript pass (scan → classify dangling expectations → synthesize closures with provenance → seed in-memory state) that completes BEFORE the loaded session enters the prompt-accepting map; reuse `SessionReader`'s enumeration and traversal-safe id pattern for list/delete; keep the tombstone a zero-byte sibling file filtered by `os.Stat`; and land the kill -9 test matrix as a scripted process-kill harness against a real serve process, not unit-level fixtures alone.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| session/list RPC + header scan | ACP handler layer (internal/acp) | Storage enumeration (internal/coreexec SessionReader or new internal/session list surface) | RPC surface belongs with the other handlers; the scan is a storage concern over `.ass-guard/` |
| Header extraction (title/createdAt/lastActivity) | Storage layer (internal/session) | — | First-line read of transcripts the Manager already owns; single-writer discipline |
| Replay + reconciliation engine | Session core (internal/session) | ACP handler (orchestrates, does not implement) | D-01: reconstruction is transcript semantics, not wire semantics; Projector precedent |
| Synthetic closure emission | Session core via Phase-16 TurnEmitter | ACP notification stream | Replay frames ride the same ordered emitter as live turns (16-D-02) |
| session/close cancel-and-drain | ACP handler | Session `Close` (closeOnce chain) | D-12 = existing cancel contract + 17-D-13 drain aimed at a session |
| session/delete + tombstone + GC sweep | Storage layer (internal/session) | configOptions menu (Phase 16 rule) for grace period | Filesystem lifecycle is store ownership; menu is configuration surface |
| --resume/--continue/--c flags + picker | CLI layer (cmd/ass-guard) | acp serve entrypoint | D-10/D-11 are terminal surfaces; they select a session BEFORE serve wiring |
| updatedAt / session_info_update pushes | ACP handler | Session callbacks | Optional freshness surface; emitter family |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (`os`, `path/filepath`, `regexp`, `sync`, `time`) | go.mod toolchain | Everything: enumeration, stat-filtering, marker files, cursor encoding, picker I/O | The whole phase is filesystem + wire plumbing; no external dependency is justified |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/spf13/cobra` | existing dep | `--resume`/`--continue`/`-c` flag registration on the CLI surface | Only for flag parsing; all logic internal |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Zero-byte tombstone file | Rename to `.deleted` suffix or trash dir | Rename touches the transcript path (breaks D-04 scan invariants and reversibility story); trash dir moves bytes — D-07 explicitly locks marker file |
| Composite string cursor | Base64 offset index / SQLite index | Index files drift (D-04's whole point); opaque composite tuple is stateless |

**Installation:** none — zero new dependencies.

**Version verification:** N/A (no new packages; cobra already in go.mod).

## Package Legitimacy Audit

**No external packages are installed by this phase.** All work is Go stdlib plus the existing cobra dependency. Audit not applicable.

## Architecture Patterns

### System Architecture Diagram

```
                       ┌───────────────────────────────────────────────┐
                       │                ACP client (Zed)               │
                       └───────┬───────────────┬──────────────┬────────┘
                 session/list  │     session/load │   session/close│delete
                               ▼                  ▼               ▼
                       ┌───────────────────────────────────────────────┐
                       │          internal/acp handlers                │
                       │  (list: header scan → SessionInfo[])          │
                       │  (load: reconcile-then-accept gate)           │
                       │  (close: cancel → drain asks → flush)         │
                       │  (delete: tombstone write → artifact sweep)   │
                       └───────┬───────────────┬──────────────┬────────┘
                               ▼               ▼              ▼
             ┌─────────────────────┐  ┌────────────────┐  ┌──────────────────┐
             │ .ass-guard/ scan    │  │ Session core   │  │ Storage layer    │
             │ transcript_*.jsonl  │  │ replay pass →  │  │ tombstone <id>.  │
             │ first-line headers  │  │ dangling-class │  │ deleted (0-byte) │
             │ mtime order, filter │  │ classification │  │ checkpoints/     │
             │ .deleted by stat    │  │ → synthetic    │  │ derived artifacts│
             │                     │  │ closures (D-02)│  │ GC after grace   │
             └─────────────────────┘  └───────┬────────┘  └──────────────────┘
                                              │ seed: turnCounter max, planMode
                                              │ from last plan_mode line,
                                              │ catalog/commands re-advertised
                                              ▼
                                    ┌────────────────────┐
                                    │ prompt-accepting   │
                                    │ session map (gated │
                                    │ on replay complete)│
                                    └────────────────────┘

  CLI path (parallel entry):
  ass-guard --resume|--resume <id|name>|--continue|-c
        │ picker (D-11 numbered list, stdin/stderr) or direct id/name
        ▼
  session selected BEFORE serve wiring → acp serve loads it (same load engine)
```

### Recommended Project Structure
```
internal/
├── acp/            # session/list|load|close|delete handlers, capability flips
├── session/        # header scan, reconciliation pass, tombstone+GC, replay seeding
├── coreexec/       # SessionReader (existing enumeration — reuse or supersede)
└── acpserve/       # Options gains resume-target selection from CLI flags
cmd/ass-guard/      # --resume/--continue/-c flags + numbered picker
```

### Pattern 1: Reconcile-then-accept gate (D-03)
**What:** session/load constructs the Session, runs the full replay + reconciliation, and only then registers the session as prompt-accepting.
**When to use:** always on load; the --resume/--continue CLI path funnels into the same engine.
**Why:** linearizable resume — a prompt arriving during replay either waits or errors, never interleaves with replayed frames.

### Pattern 2: Header scan, no index (D-04/D-06)
**What:** list = `os.ReadDir` + `os.Stat` filter (skip `<id>.deleted`) + first-line read of each transcript for sessionId/first-user-prompt/createdAt; lastActivity from mtime (cross-checked against last line's timestamp when needed).
**When to use:** every session/list call; picker.
**Why:** nothing to drift; append-only discipline preserved; milliseconds at project scale.

### Pattern 3: Composite opaque cursor (D-05)
**What:** cursor encodes `(lastActivity, sessionId)` of the last emitted row; the next page returns sessions strictly after that tuple in `(lastActivity desc, id asc/desc)` order. Base64/string-encoded, treated as opaque by clients.
**When to use:** session/list pagination.
**Why:** stable tie-break; documented best-effort stability when a session becomes active mid-paging.

### Pattern 4: Tombstone + grace GC (D-07/D-08/D-09)
**What:** delete = write zero-byte `<id>.deleted` beside the transcript; remove checkpoints + session-scoped derived artifacts; audit untouched. A sweep (trigger point at planner's discretion) physically purges tombstoned files older than the configurable grace (default 30d).
**Why:** reversible until purge; list filters by stat without reading transcript bytes.

### Pattern 5: Synthetic closures with provenance (D-02)
**What:** each dangling expectation discovered by the reconciliation pass appends (or emits, per planner's on-disk-vs-replay decision) a closure line whose payload carries an interrupted/synthetic marker.
**Why:** replay shows them subtly in the UI; the audit trail keeps them unambiguous on disk (D-20 spirit).

### Anti-Patterns to Avoid
- **Sidecar session state:** any JSON blob next to the transcript capturing live state. D-01 forbids it; under kill -9 it drifts from the transcript and becomes the corruption source.
- **rm on delete:** destroys reversibility and the grace window; the marker file IS the delete until GC fires (D-07/D-09).
- **Registering the session before replay completes:** reintroduces the interleaving race D-03 exists to prevent.
- **Reading whole transcripts for list:** D-04/D-06 is a first-line + stat scan; full reads belong to load/replay only.
- **Regenerating session/turn ids on resume:** ids continue from transcript maxima (`<sessionID>-turn-%03d` sequence), or replayed frames and new frames collide.

## Live-State Inventory (roadmap research flag)

The reconciliation surface derives from the transcript kinds plus the ask/pending registries. The complete line-type set is fixed in code [VERIFIED: internal/session/transcript.go:19-48]:

```go
	TypeSessionStart      = "session_start"
	TypeUserMessage       = userMessageType
	TypeRequestShaped     = "request_shaped"
	TypeAgentMessageChunk = "agent_message_chunk"
	TypeAssistantMessage  = "assistant_message"
	TypeToolCall          = "tool_call"
	TypeToolResult        = "tool_result"
	TypeBoundary          = kindBoundary
	TypeSubagentDispatch  = "subagent_dispatch"
	TypeSubagentResult    = "subagent_result"
	TypeUsage             = "usage"
	TypeCanceled          = "canceled"
	TypeEngineDecision    = "engine_decision"
	TypeError             = "error"
	TypeSessionEnd        = "session_end"
	TypeCommandProvenance = "command_provenance"
	TypeAskSuspended      = "ask_suspended"
```
plus `TypePlanMode = "plan_mode"` [VERIFIED: internal/session/planmode.go:67].

Class-by-class dangling-expectation matrix (what kill -9 mid-turn leaves on disk, and the required synthetic closure):

| # | Dangling class (on-disk symptom) | Synthetic closure (D-02 provenance) | In-memory seed after reconciliation |
|---|----------------------------------|--------------------------------------|--------------------------------------|
| 1 | `tool_call` without matching `tool_result` | failed tool_result with interrupted marker (ACP-06 verbatim: "orphaned in-flight tool_calls closed as failed") | none — closure completes the pair; Projector's pair-safety then folds it |
| 2 | `user_message` + turn lines without a terminal line (`canceled`/error/end-of-turn marker) for that turnID | synthetic canceled terminal with interrupted marker | turn is DEAD; no engine continuation for it |
| 3 | `ask_suspended` without the resolution that the operator reply or D-01 timer would append | synthetic cancelled-normal resolution (16-D-19 registry synthetic-cancel is the live-side twin) | ask registry for that turn cleared; no parked ask fires post-resume |
| 4 | `subagent_dispatch` without `subagent_result` | synthetic failed subagent_result with interrupted marker | task registry carries no live child (processes died with the parent); no zombie reaps |
| 5 | `agent_message_chunk` stream with no closing `assistant_message` | synthetic interrupted marker on replay (cosmetic — the model-visible window uses the Projector's folded view) | none |
| 6 | `plan_mode` state (in-memory today; last `plan_mode` line on disk) | n/a — reconstruction, not closure: seed planMode from the LAST `plan_mode` line's target state | planMode initialized to persisted state, not fresh default |
| 7 | turn id sequence | n/a — seed: scan max turn suffix from `<sessionID>-turn-%03d` lines and set `turnCounter` past it | next `nextTurnID()` continues the sequence without collision |
| 8 | `request_shaped` without closing `usage` | no model-visible closure needed — audit-only pair; note in replay provenance | usage/cost counters re-derived from complete `usage` lines only |
| 9 | missing `session_end` | synthetic session_end with interrupted marker opens the resume's transcript continuity (planner decides whether resume appends a fresh `session_start` or a resume marker — additive kinds allowed by 16-D-20) | none |
| 10 | pending permission asks (post-17: gate pipeline state) | Phase 17 must record pending-ask state on the transcript for reconciliation to see it — see Open Question Q2 | none |

Separately-persisted stores reload outside reconciliation per D-01: schedule `lastFired`, learning store, permissions.yaml (post-17). The catalog/command registry re-advertises via the existing discovery surface (ACP-06: "commands re-advertised").

**kill -9 durability argument:** `appendLine` appends complete lines through `os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermOwner)` [VERIFIED: internal/session/transcript.go:141-144, with `const filePermOwner = 0o600` at transcript.go:11] where `fname := "transcript_" + sessionID + ".jsonl"` [transcript.go:141]. A single `write(2)` to an `O_APPEND` fd is atomic-enough for process death: page-cache data survives kill -9 without fsync. Torn/partial final lines are an OS-crash/power-loss artifact only — and both readers already skip non-conforming lines (`readTranscriptFile` in manager.go; SessionReader.read in messaging.go). This is why kill -9 is the correct acceptance target and why the matrix below tests class coverage, not byte-level durability.

**Kill -9 test matrix (design for the planner):** a scripted harness spawns a real `acp serve` (or the runner seam) with a stubbed provider that pauses mid-turn at a chosen suspension point, sends SIGKILL, then resumes via session/load and asserts the reconciliation outcome. Rows = the ten inventory classes; each row asserts: (a) replay completes before prompt acceptance, (b) the synthetic closure carries provenance, (c) the next turn id continues the sequence, (d) no ghost state (no orphaned asks/tool_calls visible as live). Two additional rows: kill during ask suspension with queued asks (17-D-13 interplay), and kill during replay itself (load interrupted → second load succeeds; idempotent). Matrix cell commands must be runnable in CI (SIGKILL of a child process is portable on macOS+Linux).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Safe session-id → path resolution | Ad-hoc validation | Existing `sessIDPattern` | Traversal-safe by construction; reuse verbatim |
| Model-visible window reconstruction | A parallel replay builder | Existing Projector | Reads whole transcript per turn already; resume gets correct window by construction |
| Turn cancellation semantics | New cancel paths | Existing cancel contract + `closeOnce` chain | D-12 is the same machinery aimed at a session |
| Transcript enumeration | New directory walker | `SessionReader.Sessions()` | Already enumerates `transcript_*.jsonl` under `.ass-guard/` with bounds |
| Replay frame emission | Direct notification writes | Phase-16 TurnEmitter | 16-D-02: replay rides the same ordered emitter as live turns; 25-D-13/D-14 keep the seam kit-extractable |

**Key insight:** this phase is almost entirely *composition of existing correctness machinery behind new RPC/CLI surfaces* — the new correctness surface (reconciliation) is a read-only classification pass over the transcript, which cannot corrupt anything if it only appends provenance-marked closures.

## Common Pitfalls

### Pitfall 1: Registering the resumed session before replay finishes
**What goes wrong:** a `session/prompt` arriving mid-replay interleaves with replayed frames; ghost state.
**Why it happens:** handler wiring that inserts into the session map at construction.
**How to avoid:** the reconcile-then-accept gate — the map insert (or an `acceptingPrompts` flag) is the last step of load. **Warning signs:** any test that prompts concurrently with load.

### Pitfall 2: Regenerating ids on resume
**What goes wrong:** fresh sessionID (the current `sessionFor` default) or reset turnCounter → replayed frames and new frames collide; transcript maxima break.
**Why it happens:** `sessionFor` constructs a fresh CSPRNG id every call today [VERIFIED: internal/runtime/runtime.go sessionFor — fresh sessionID per construction].
**How to avoid:** load path must pass the transcript's sessionId AND seed `turnCounter` from the max `%03d` suffix of `<sessionID>-turn-NNN` lines (format fixed at [VERIFIED: internal/session/session.go:143] — `return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)`).
**Warning signs:** turn ids restarting at 001 on a resumed session.

### Pitfall 3: planMode reset on resume
**What goes wrong:** resumed session starts in default mode instead of the persisted plan-mode state.
**Why it happens:** planMode is in-memory (`session.NewPlanModeState()` at construction).
**How to avoid:** seed from the last `plan_mode` transcript line (the line type exists: [VERIFIED: internal/session/planmode.go:67]).

### Pitfall 4: Delete touching transcript bytes or the audit
**What goes wrong:** rm/rewrite breaks reversibility and D-20.
**Why it happens:** reaching for os.Remove for "cleanliness."
**How to avoid:** tombstone marker + stat filter; sweep is the ONLY purger, audit exempt unconditionally. **Warning signs:** any `os.Remove` on `transcript_*.jsonl` outside the grace-expired sweep.

### Pitfall 5: Cursor instability without tie-break
**What goes wrong:** same-lastActivity sessions flip pages across calls.
**Why it happens:** time-granularity collisions (mtime seconds).
**How to avoid:** composite `(lastActivity, sessionId)` cursor with deterministic secondary sort; document best-effort shift when a session becomes active mid-paging (D-05).

### Pitfall 6: --continue scoping mismatch
**What goes wrong:** global most-recent resumes a session from an unrelated project.
**Why it happens:** assuming "most recent" means globally.
**How to avoid:** match CC — `--continue` is cwd-scoped ("Resumes the most recent interactive session in the current directory" [VERIFIED: code.claude.com/docs/en/sessions, Resume a session table]); ass-guard transcripts are per-directory (`<dir>/.ass-guard/`), so cwd scoping is also the natural read of the store. `--resume <id>` may look beyond cwd (see Open Question Q1).

### Pitfall 7: session/load response shape mismatch
**What goes wrong:** emitting a `sessionId` field in LoadSessionResponse, or naming the mode field `currentMode`.
**Why it happens:** v2/RFD examples and training-data shapes differ from v1.
**How to avoid:** v1 LoadSessionResponse properties are exactly `_meta`, `configOptions` (`SessionConfigOption[] | null`), `modes` (`SessionModeState | null`) — NO sessionId [VERIFIED: agentclientprotocol.com/protocol/v1/schema.md, LoadSessionResponse ResponseFields]; the request's sessionId persists as THE session id. NewSessionResponse DOES carry `sessionId` [same source].

### Pitfall 8: Advertising session/load under the wrong capability key
**What goes wrong:** clients gating on `sessionCapabilities.*` never see load.
**Why it happens:** v1 splits gating — load is top-level.
**How to avoid:** `session/list|close|delete|resume` advertise under `sessionCapabilities: {list:{}, close:{}, delete:{}}`; `session/load` under top-level `loadSession: true` (flip of today's `false` [VERIFIED: internal/acp/handlers.go:50 — `"loadSession": false`]). Schema note: "session/load is still handled by the top-level load_session capability. This will be unified in future versions" [VERIFIED: agentclientprotocol.com/protocol/v1/schema, SessionCapabilities].

### Pitfall 9: GC sweeping live or fresh tombstones
**What goes wrong:** purge fires on a session the user just deleted by accident; or sweeps an in-use session.
**How to avoid:** grace clock starts at tombstone creation (marker mtime); sweep never touches non-tombstoned files; loud structured logging of what it removed (established pattern).

### Pitfall 10: Picker assuming a TTY
**What goes wrong:** raw-mode/termios dependency breaks pipes/ssh.
**How to avoid:** D-11's numbered list on stdin/stderr only; read line-wise numbers; EOF/invalid → error out cleanly. Out-of-scope table already forbids agent-side TUI pickers.

## Code Examples

### Enumeration + traversal safety (reuse verbatim)
```go
// [VERIFIED: internal/coreexec/messaging.go:175-186]
// sessIDPattern validates session ids in BOTH live vocabularies (12-11,
// G-12-4c): the captured zcode schema's sess_* branch (kept
// character-for-character — the schema is the provenance for that shape) OR
// ass-guard's own id branch — the RFC 4122 v4 UUID form internal/acp/handlers.go
// newSessionID mints and internal/session/transcript.go openTranscript writes as
// transcript_<uuid>.jsonl (T-12-04-03: the reader serves ass-guard's OWN store).
//
// Traversal-safe by construction (T-12-11-01): both alternatives exclude path
// separators, so the filepath.Join under .ass-guard/ in read() cannot escape
// the store directory — a hostile id rejects structurally BEFORE any file open.
var sessIDPattern = regexp.MustCompile(
	`^(sess_[A-Za-z0-9._-]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)
```
`SessionReader.Sessions()` enumerates via `os.ReadDir(filepath.Join(r.dir, ".ass-guard"))` [VERIFIED: internal/coreexec/messaging.go:241-242]. List/delete/session-id-from-client paths must route through this pattern.

### Transcript path + append discipline (the durability base)
```go
// [VERIFIED: internal/session/transcript.go:141-144]
	fname := "transcript_" + sessionID + ".jsonl"
	// ... path := filepath.Join(dir, ".ass-guard", fname)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermOwner)
```
(`filePermOwner = 0o600` at transcript.go:11.) Tombstones are zero-byte siblings named `<id>.deleted` beside these files (D-07) — i.e. `transcript_<id>.jsonl.deleted` or `<id>.deleted` per planner, but list filtering must be by `os.Stat` existence only.

### Turn-id continuation (Pitfall 2)
```go
// [VERIFIED: internal/session/session.go:139-143]
// nextTurnID returns a monotonically-increasing turn id for this session.
func (s *Session) nextTurnID() string { //nolint:funcorder // ordering groups related logic
	n := s.turnCounter.Add(1)
	// ...
	return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)
```
`turnCounter` is `atomic.Int64` (session.go:127). Resume seeds it from the transcript's max `turn-NNN` suffix (ACP-06: "continued id sequences from transcript maxima").

### Current load stub being replaced
```go
// [VERIFIED: internal/acp/handlers.go:184-192]
// handleSessionLoad is a NO-OP per D-09 (NO replay in v1). It returns a -32601
// method-not-supported error; loadSession is advertised false in initialize.
// There is deliberately NO replay code path (Pitfall 6 — scope-creep guard).
func (s *Server) handleSessionLoad(ctx context.Context, params json.RawMessage) (any, error) {
	return nil, &RPCError{
		Code:    CodeMethodNotFound,
		Message: "session/load not supported (loadSession is false; replay is out of v1 scope — D-09)",
	}
}
```
This phase replaces the body and flips the advertisement at handlers.go:48-50 (`ProtocolVersion: 1`, `"loadSession": false`).

### v1 wire shapes (authoritative, fetched this session)
```
# [VERIFIED: agentclientprotocol.com/protocol/v1/schema.md — ResponseField names]
ListSessionsRequest:  _meta, cursor (string | null), cwd (string | null)
ListSessionsResponse: _meta, nextCursor (string | null), sessions (SessionInfo[] required)
SessionInfo:          _meta, additionalDirectories (string[]),
                      cwd (string, required), sessionId (string, required),
                      title (string | null), updatedAt (string | null)
                      # updatedAt = "ISO 8601 timestamp of last activity"
LoadSessionRequest:   _meta, additionalDirectories (string[]),
                      cwd (string, required), mcpServers (McpServer[], required),
                      sessionId (string, required)
LoadSessionResponse:  _meta, configOptions (SessionConfigOption[] | null),
                      modes (SessionModeState | null)          # NO sessionId field
CloseSessionRequest:  _meta, sessionId (required)
CloseSessionResponse: _meta
DeleteSessionRequest: _meta, sessionId (required)
DeleteSessionResponse: _meta
```
SessionInfoUpdate (agent→client notification, keeps editor lists fresh): fields `title` (null to clear), `updatedAt` (null to clear) [VERIFIED: same source]. D-06's "lastActivity" concept maps to wire field `updatedAt`; the header's createdAt is list/picker-side only (no v1 wire slot — optional `_meta` is the escape hatch if ever needed).

### CC parity reference (D-10)
```
# [VERIFIED: code.claude.com/docs/en/sessions — Resume a session table]
`claude --continue`  → "Resumes the most recent interactive session in the current directory"
`claude --resume`    → "Opens the session picker"
`claude --resume <name>` → "Resumes the named session directly"
`claude --resume <session-id>` → works "from any directory"; cross-project search
                       resolves "only when exactly one other project holds a transcript"
`/resume`            → switch mid-session (out of Phase 18 scope — class-B command lands Phase 20)
```
Picker row content for D-11 calibration: "the session name if you set one, otherwise the AI-generated session title, conversation summary, or first prompt, along with time since last activity" [same source]. ass-guard's D-06 title is the mechanical truncation of the first user prompt (no LLM call) — documented deviation, acceptable: CC's LLM title is an implementation nicety, the contract is "first-prompt-derived handle." CC retention default is also 30 days (`cleanupPeriodDays`), matching D-09's 30-day default.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ACP v1: `session/load` under top-level `loadSession`; separate no-replay `session/resume` under `sessionCapabilities.resume` | ACP v2 (RFD stage): `session/resume` replaces BOTH (load+replay via `replayFrom`); list/delete/close promoted with required `cwd`/`updatedAt` | v2 in RFD/migration-notes stage now | Project speaks protocolVersion 1 [VERIFIED: internal/acp/handlers.go:48]; build against v1 shapes; the reconciliation engine is version-agnostic (transcript-side), so a future v2 `resume+replayFrom` reuses it |
| v1.1/v1.0: session/load explicitly out of scope (D-09 stub, `-32601`) | This phase: full load + replay | Phase 18 | The stub at handlers.go:184-192 is the seam being filled |
| CC pre-v2.1.223: `--resume <id>` cwd-only | CC current: cross-project unique-match search | CC v2.1.223 | Parity bar for `--resume <id>` "anywhere" — but ass-guard's store is per-directory; see Open Question Q1 |

**Deprecated/outdated:**
- v2 `SessionInfo.updatedAt` as a REQUIRED field with required `cwd` — do NOT copy into the v1 implementation; v1 marks both optional/nullable.
- Any `currentMode` response field name — v1 uses `modes` (`SessionModeState`).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Phase 16's TurnEmitter and Phase 17's ask/gate machinery will have landed with the contracts as written in their CONTEXT.md files (16-D-02/D-19/D-20, 17-D-13) | Live-State Inventory, Don't Hand-Roll | Replay emitter and ask-drain integration points shift; reconciliation classes 1/3 unchanged (they are transcript-driven) |
| A2 | `--resume anywhere` (ACP-06) means the CLI flag is available in any launch context (terminal, not only via editor RPC), with cwd-scoped default resolution; NOT a CC-style cross-project filesystem search | Pitfall 6, Open Questions | If operator intends CC's global-search parity, a project-directory registry (CC has `~/.claude/projects/`) becomes a new design need — descope decision required |
| A3 | `session/resume` (v1, no-replay variant) is NOT advertised in Phase 18 — D-03's reconcile-then-accept makes full load the only safe path; nothing in ACP-05/06/07 requires it | Pitfall 8, State of the Art | Zed falls back to load if resume capability absent (capability-gated); zero client impact expected, verify during execution |
| A4 | Tombstone filename interpretation `<id>.deleted` = beside the transcript, exact spelling planner's choice (D-07 says `(<id>.deleted)`; examples above note both readings) | Pattern 4, Code Examples | Cosmetic on-disk naming; both satisfy stat-filter + zero-byte contract; lock during planning |
| A5 | CC picker behaviors (Ctrl+A widen, branch grouping, rename) are NOT parity targets for D-11's numbered picker | Code Examples | Scope creep only; REQUIREMENTS out-of-scope table already bars agent-side TUI pickers |
| A6 | Synthetic closures are APPENDED to the transcript on load (on-disk provenance per D-02 "unambiguous on disk"), not merely emitted into replay | Pattern 5 | If planner chooses replay-only emission, audit post-mortem of kill -9 cases weakens; either satisfies the UI half of D-02 — decide in planning |

## Open Questions

1. **`--resume <id|name>` resolution scope beyond cwd**
   - What we know: CC resolves `<session-id>` across all projects (unique-match); ass-guard stores transcripts per project directory with no central registry.
   - What's unclear: whether ACP-06's "`--resume` anywhere" demands cross-directory search or only CLI-surface availability.
   - Recommendation: default to cwd-scoped resolution + clear "not found in this directory" error; treat global search as a follow-up needing a project registry (A2). Planner confirms with operator only if cheap; otherwise document the deviation.
2. **Phase-17 pending-permission state on disk (inventory class 10)**
   - What we know: reconciliation is transcript-only (D-01); permission asks post-17 suspend via AskBroker and 17-D-13 drains them on turn death.
   - What's unclear: whether Phase 17's plan records pending gate state as transcript lines (needed) or in-memory only (breaks class-10 reconciliation).
   - Recommendation: planner reads 17's PLAN.md when it lands; if in-memory only, Phase 18 must add the additive line kind (16-D-20 permits) or accept that pending permission asks resolve as cancelled-normal (class-3 treatment) — the latter is probably correct UX anyway.
3. **Picker/CLI landing surface**
   - What we know: root command is a one-shot tracer requiring `--prompt`; `acp serve` is the Zed entrypoint; D-10's trio implies an interactive conversational loop or at least a resume-then-serve flow.
   - What's unclear: whether `--resume` attaches to root, to `acp serve`, or to a new interactive mode; Phase 18's boundary says "CLI flag parsing in cmd (root/acp command) — --resume/--continue pre-serve selection," suggesting flags select a session then hand off to serve wiring.
   - Recommendation: flags on `acp serve` (and a thin root passthrough if criterion 3's "works anywhere" demands it) that resolve the target BEFORE `acpserve.Run` and inject it as the initial loaded session via the same load engine as session/load — one engine, two entrypoints.

## Environment Availability

Step 2.6: SKIPPED — no external dependencies beyond the Go toolchain already gating `mise ci`; kill -9 harness needs only POSIX signals (portable macOS+Linux per project platform scope).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + `-race` (project standard) |
| Config file | `.mise.toml` (tasks: vet, lint, build, test, ci) + `.golangci.yml` |
| Quick run command | `go test -race -count=1 ./internal/session/... ./internal/acp/...` |
| Full suite command | `mise ci` (vet + golangci-lint v2 + CGO_ENABLED=0 build + `go test -race`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ACP-05 | list returns SessionInfo[] header-scan, filters tombstones by stat, cursor pagination incl. tie-break | unit | `go test -race -count=1 ./internal/session/ -run TestSessionList` | ❌ Wave 0 |
| ACP-05 | list handler round-trip: capability-gated, cwd filter, nextCursor contract | unit (handler) | `go test -race -count=1 ./internal/acp/ -run TestHandleSessionList` | ❌ Wave 0 |
| ACP-06 | reconciliation classes 1-9: dangling input → synthetic closure with provenance + correct seed | unit (table-driven over fixture transcripts) | `go test -race -count=1 ./internal/session/ -run TestReconcile` | ❌ Wave 0 |
| ACP-06 | reconcile-then-accept: prompt before replay-complete rejected/blocked | unit (concurrent) | `go test -race -count=1 ./internal/acp/ -run TestLoadGate` | ❌ Wave 0 |
| ACP-06 | kill -9 matrix: real serve process, SIGKILL mid-suspension, resume asserts inventory classes | integration | `go test -race -count=1 ./internal/acpserve/ -run TestKill9Resume` (gated like eval suites if slow) | ❌ Wave 0 |
| ACP-06 | --continue/--resume/--c flag parsing + cwd scoping + picker I/O on pipes | unit | `go test -race -count=1 ./cmd/ass-guard/ -run TestResumeFlags` | ❌ Wave 0 |
| ACP-07 | close: cancel-and-drain, idempotent re-close | unit | `go test -race -count=1 ./internal/acp/ -run TestSessionClose` (extends close_test.go family) | ❌ Wave 0 (close_test.go exists for Session.Close — handler-level new) |
| ACP-07 | delete: tombstone written, list filters, artifacts swept, audit intact, GC respects grace | unit | `go test -race -count=1 ./internal/session/ -run TestTombstone` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./internal/session/... ./internal/acp/... ./cmd/ass-guard/...`
- **Per wave merge:** `mise ci`
- **Phase gate:** full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- `internal/session/list_test.go` — ACP-05 header scan/cursor/tombstone filter
- `internal/session/reconcile_test.go` — ACP-06 class matrix with fixture transcripts (one fixture per inventory row)
- `internal/acp/session_family_test.go` — list/load/close/delete handler round-trips + load gate
- `internal/acpserve/kill9_test.go` — SIGKILL harness (spawn → suspend → kill → resume → assert); reuse the PrepareServe/FinishServe callback seam landed in Phase 15
- `cmd/ass-guard/resume_flags_test.go` — flag trio + picker-on-pipe
- Fixture discipline: reconciliation fixtures are hand-written transcripts exercising each dangling class — keep them in testdata, one per class, named by inventory row number

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | stdio transport; out of phase scope |
| V3 Session Management | yes (session lifecycle semantics) | ACP session ids; idempotent close; capability-gated methods |
| V4 Access Control | partial | capability advertisement gates list/load/close/delete per client |
| V5 Input Validation | yes | `sessIDPattern` on every client-supplied sessionId (list cwd, load, close, delete) — traversal-safe by construction |
| V6 Cryptography | no | ids are existing CSPRNG UUIDs; nothing new |

### Known Threat Patterns for Go stdio agent + filesystem session store

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal via sessionId (list/load/delete) | Tampering / Elevation | `sessIDPattern` reject-before-open [VERIFIED: internal/coreexec/messaging.go:182-186]; apply to ALL new handlers |
| Destructive delete weaponized (delete arbitrary files) | Tampering | tombstone-only delete; purge sweep constrained to `.ass-guard/` tombstoned files past grace; audit exempt |
| Symlink planting in `.ass-guard/` | Tampering / Info disclosure | store dir 0750/file 0600 discipline (existing `filePermOwner`/`dirPerm`); planner considers lstat-vs-stat on marker checks |
| Replay poisoning via crafted transcript | Tampering | readers skip non-conforming lines (existing); reconciliation is read-only classification — closures appended, never rewrite |
| DoS via huge cursor/paging loops | DoS | bounded page size; opaque cursor validated before use |

## Sources

### Primary (HIGH confidence)
- Codebase (read this session): `internal/session/transcript.go`, `internal/session/manager.go`, `internal/session/session.go`, `internal/session/projector.go`, `internal/session/planmode.go` (line 67), `internal/acp/handlers.go`, `internal/coreexec/messaging.go`, `internal/runtime/runtime.go`, `cmd/ass-guard/main.go`, `cmd/ass-guard/acp_serve.go`, `.mise.toml`
- `.planning/phases/18-session-family/18-CONTEXT.md` (D-01..D-12 verbatim), `.planning/REQUIREMENTS.md` (ACP-05/06/07), `.planning/STATE.md`, 16/17/25-CONTEXT.md contracts

### Secondary (MEDIUM confidence)
- [VERIFIED: agentclientprotocol.com/protocol/v1/schema + schema.md] — v1 wire shapes, ResponseField names, capability gating, SessionInfoUpdate (fetched 2026-08-27, HTML + markdown cross-checked)
- [VERIFIED: code.claude.com/docs/en/sessions] — CC --resume/--continue contract, picker scoping, 30-day retention, title policy (fetched 2026-08-27)
- Context7 `/websites/agentclientprotocol` — v2/RFD migration context (session/resume replacing load; RFD provenance of delete/close/list)

### Tertiary (LOW confidence)
- None — no claim in this document rests on an unfetched source

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero new dependencies; stdlib composition over read code
- Architecture: HIGH — every integration point verified against source this session; Phase 16/17 contracts are written but unbuilt (A1 tracks the drift risk)
- Wire shapes: MEDIUM-HIGH — v1 schema fetched from the canonical domain twice (HTML + markdown); seam classify-confidence returns MEDIUM for context7/webfetch providers, mitigated by dual-source cross-check
- Pitfalls: HIGH — derived from code reads and canonical docs, not folklore

**Research date:** 2026-08-27
**Valid until:** 2027-01-27 (v1 schema stable; CC sessions doc fast-moving — re-verify CC flags before execution if >30 days elapse)
