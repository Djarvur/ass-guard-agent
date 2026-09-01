# The Permission Gate Pipeline

Phase 17 (ACP-01/ACP-02) — the ONE pipeline every tool call passes through:
**hook verdict → permission ask → execute**. This document is the contract
Phase 21's hooks join at (the GATE PIPELINE LOCK: hooks join this pipeline,
they never bolt on a second gate) and the reference for operators hand-editing
`permissions.yaml`. Every documented behavior cites its phase decision
(D-01..D-13 of `.planning/phases/17-permissions-elicitation/17-CONTEXT.md`).

---

## 1. The pipeline — hook verdict → permission ask → execute

### 1.1 The ONE chokepoint: `internal/session` `gateCall`

`gateCall` (internal/session/gate.go) is THE per-call permission pipeline. It
is invoked for EVERY tool call in `runTurn`'s loop, in BOTH dispatch branches:
the batch-eligible branch (beside `planModeBlocks`) and the subagent branch
(before `DispatchSubagent`). There is no second permission path — not for
subagents, not for engine/automation turns (D-05). A package-wide grep pins
this: `perm.RuleSet.Evaluate` appears in internal/session only inside gate.go.

Why the gate sits in `runTurn` and nowhere lower:

- `toolexec.DispatchBatch` wraps every dispatched call in a per-tool deadline
  (`executeBounded`, default 120s) — a human-scale dialog wait inside the
  executor would be deadline-killed. The gate runs ABOVE the wrap; an allowed
  call keeps its normal deadline.
- The executor Stub seam carries no call identity; the gate needs
  turnID + callID to record the pending ask.
- Subagent `Task` calls never reach `DispatchBatch` — an executor-level gate
  would miss them entirely.

### 1.2 The verdict chain, in order

`gateCall` evaluates, per call:

1. **Hook verdict head (D-04).** Hooks are checked BEFORE permission rules
   (CC parity, the blocking-hook precedent). **Phase 21 joins here, at this
   exact seam** — see §1.3. Today the head is an implicit allow (no hook
   surface exists at the turn loop yet).
2. **Permission rules — evaluated in BOTH modes (D-05).** "Ungated = no
   dialogs" never means "no evaluation": deny rules deny in the ungated
   default too (§2.1). A deny anywhere beats any allow, in both modes.
3. **The human-present decision (D-07).** An automation turn NEVER opens a
   dialog nobody would answer — ask-class calls decline loudly in BOTH modes
   (§2.3). Deny/allow rules were already enforced at step 2.
4. **The mode decision (D-05/D-06).** Ungated: unmatched ask-class calls
   execute with zero dialogs (the default, criterion 4). Gated: suspend for
   the native dialog (§2.2).
5. **The degraded-client guard (16-D-18).** A client that answered -32601
   once cannot answer permission asks at all — sticky decline, no new
   round-trip, never a silent allow (§5).

Verdicts: `gateExecute` (run the call), `gateDeny` (append the denial result,
never execute), `gateSuspend` (record the pending ask, enqueue, end the turn
with the ask marker), `gateDeclineAutomation` (append the decline note). The
suspension rides the AskBroker discipline: **the per-session turn mutex is
NEVER held across a human wait** — the turn ends, the ask queue's pump fires
the dialog, and the answer drives a resume that persists any always-rule and
executes the gated call before re-entering the SAME turn.

### 1.3 The Phase-21 hook-join contract

Phase 21's PreToolUse hooks join at the HEAD of `gateCall` (step 1), under
these locked rules (D-04 + PAR-03):

- **Deny-only authority from project scope.** A blocking hook deny stops the
  call BEFORE rule evaluation (`gateDeny`) — matching CC, where "a hook that
  exits with code 2 stops the tool call before permission rules are
  evaluated".
- **Repo-shipped files never grant allow.** Hooks can never allow a call past
  the rules: every allow case still falls through to rule evaluation. This is
  deliberately stricter than Claude Code, whose hooks can return `allow` —
  a project safety decision, not drift (§4).
- **No second gate.** Hook plumbing must call into `gateCall`'s pipeline, not
  wrap tool dispatch elsewhere. The precedence
  hook verdict → permission ask → execute is the whole contract; ROADMAP
  Phase 21 criterion 2 ("no second gate exists") audits against this
  document and the internal/session grep discipline.

### 1.4 The ask queue (D-11..D-13)

Every dialog firing goes through ONE queue (internal/session/askqueue.go):

- **One outstanding ask** — dialogs never stack; a second enqueue waits
  (D-11). Foreground turns preempt queued background asks; FIFO within a
  class; an open dialog is never preempted.
- **The queue is visible** — each queued enqueue emits a
  `session/update` note ("ask queued — N pending") and increments the
  enqueue counter (D-12).
- **Turn death drains** — `DrainTurn(turnID)` is the ONE shared drain, wired
  into all three teardown paths (session/cancel notification, logout, serve
  shutdown): the open dialog resolves cancelled through the queue-owned
  per-firing ctx (a registry-backed dialog cascades `$/cancel_request` per
  16-D-19) and that turn's queued-but-unfired asks drain cancelled-normal
  with zero fires (D-13). No orphaned dialogs, no zombie asks.

---

## 2. Mode semantics

`permissions.mode` is a real editor-facing config option
(`session/set_config_option`, `configId: "permissions.mode"`, values
`ungated | gated`). It persists via the project config layer (0600 atomic,
persist-then-apply) and a flip reaches the running session's very next tool
call — the gate reads the mode through a live per-call accessor, never a
boot-time snapshot. Boot resolution: project layer > global layer > ungated.

### 2.1 Ungated (the default — "available, not default")

Rules are evaluated; deny and allow are enforced; ask-class calls **execute
without any dialog** (D-05, criterion 4: zero new dialogs versus v1.1). A
hand-edited deny rule still denies. The E2E pins this: a mutating call on a
default session produces zero `session/request_permission` frames
(internal/acpserve/permissions_e2e_test.go, ungated-default scenario).

### 2.2 Gated

Ask-class calls **suspend** for the native permission dialog: one
`session/request_permission` frame carrying the tool-call card and exactly
four options — allow_once / allow_always / reject_once / reject_always
(optionId == kind, neutral symmetric labels). The dialog's outcome drives the
resume:

| Outcome | Behavior |
|---|---|
| allow_once | execute the gated call; no rule written; next call asks again |
| allow_always | persist the allow rule FIRST (persist-then-execute; a failed write downgrades to once-only, never silently widening), then execute |
| reject_once | denial result; no rule written |
| reject_always | persist the deny rule BEFORE the denial result (D-03); next matching call denies with no dialog |
| cancelled / turn death | cancelled-NORMAL transcript result (never an error) and no execution |

### 2.3 The ask class and automation turns (D-06/D-07)

Which calls ask is DERIVED by the rule grammar plus the existing tool
classes:

- An explicit `ask` rule routes to the dialog REGARDLESS of tool class.
- Unmatched calls: the default ask set IS today's mutating/serialized class
  (mutating tools and concurrency-unsafe tools — the same class
  `toolexec.isAloneInSlot` serializes). Everything else (read-only
  concurrent) allows.
- **Automation/cron turns (no human present): ask-class calls DECLINE with a
  client-visible transcript note + structured log — in BOTH modes.** Never a
  dialog nobody answers, never a silent allow. Deny and allow rules stay
  enforced. The decline note is the audit trail visible in the next
  foreground session (D-07).

---

## 3. permissions.yaml — the trust store

Location: `<project>/.ass-guard/permissions.yaml`. Created at session start
if absent (the zero-config floor: dirs 0750, file 0600). Loaded at session
start, consulted per call via a copy-on-read snapshot, written by dialog
clicks — and safe to hand-edit.

### 3.1 File discipline

- **0600 hard file permission, 0750 dirs** (the repo's artifact-family
  discipline).
- **Atomic temp + rename saves**; in-memory state commits only after a
  successful save — a failed write leaves the file byte-intact AND the rule
  set unchanged.
- **Idempotent dialog writes** — an always-click on an already-recorded entry
  is a byte-level no-op.

### 3.2 The grammar (three ordered lists of rule strings)

```yaml
# <project>/.ass-guard/permissions.yaml
deny:
  - "Bash(rm *)"             # Tool(specifier); "rm -rf /tmp/x" matches
  - "Bash(aws *)"            # a broad deny beats any narrower allow
  - "mcp__puppeteer"         # whole-server deny (bare mcp__<server> name)
ask:
  - "Write"                  # bare tool = all uses of Write
  - "Bash(npm *)"            # specifier with trailing-star prefix match
allow:
  - "Bash(git *)"            # compound-safe: EVERY subcommand must match
  - "Write(*.go)"            # specifier on the file tools' primary argument
  - "mcp__github__get_*"     # allow globs only after literal mcp__<server>__
```

Evaluation semantics (D-02, Claude-Code parity — the verified CC rule model):

- **Order: deny → ask → allow; first match wins; specificity never reorders
  the order.** A broad `Bash(aws *)` deny blocks even when `Bash(aws s3 ls)`
  also matches; a matching ask prompts even when a more specific allow also
  matches.
- **A deny anywhere beats any allow** (cross-list, by the fixed order).
- **Bash compound commands are split** on `&&`, `||`, `;`, `|`, `|&`, `&`,
  and newlines; every subcommand is matched independently, and the verdicts
  cascade fail-safe: any deny subcommand denies, any explicit ask subcommand
  asks, an UNMATCHED subcommand forces the unmatched path — an allow must
  cover EVERY subcommand before a compound executes (`git status && curl
  evil` can never ride an allow on `git status`).
- **Wildcard syntax:** a trailing `*` is a prefix wildcard; the `:*` suffix
  is equivalent to a trailing ` *` and recognized only at pattern end (a
  mid-pattern colon-star is literal); every other star is literal.
- **MCP tools** live in the `mcp__<server>__<tool>` namespace (the session
  catalog already registers them there); a bare `mcp__<server>` rule selects
  the whole server.
- **Unanchored allow globs** (`*`, `B*`, `mcp__*`) are inert with a warning —
  allow globs require the literal `mcp__<server>__` prefix (CC asymmetry).
  The same globs DO match in the deny/ask lists. Malformed lines are skipped
  with one structured warning each (logged to stderr, retained via
  `Store.Warnings()`), never fatal.

### 3.3 The dialog's narrow write surface (D-01/D-03)

The dialog writes ONLY **simple tool entries** — bare names of letters,
digits, and underscores (covering the `mcp__` namespace) — as one-click
tool×project trust decisions, in BOTH directions:

- **Always allow** appends the tool to `allow` (persist BEFORE executing the
  gated call — the user's recorded intent can never be lost to a crash between
  click and write).
- **Always reject** appends the tool to `deny` (persist BEFORE the denial
  result).

The scope is tool×project: the entry lands in THIS project's
`permissions.yaml`; other projects re-ask. Richer rules — specifiers, globs,
path prefixes — exist in the FILE for hand-editing only; the dialog never
writes them, and dialog writes preserve hand-edited richer lines verbatim.
The store rejects non-simple entries at the API boundary
(`validSimpleEntry`), so the D-01 boundary is a store constraint, not caller
discipline.

All choices survive restarts: the file is loaded at session start and
consulted at the chokepoint before any mode decision (criterion 5's
persistence clause).

---

## 4. Deliberate Claude-Code divergences

Two places where ass-guard intentionally differs from CC's permission model —
recorded so the differences read as decisions, not drift:

| Topic | Claude Code | ass-guard | Why |
|---|---|---|---|
| Dialog persistence | "Yes, and don't ask again" writes `.claude/settings.local.json` for Bash commands; file-modification approvals are SESSION-ONLY | One always-click persists tool×project into `permissions.yaml`, in BOTH directions (allow and deny) | A blast-radius-scoped trust model: one click cannot unleash a tool everywhere (other projects re-ask), and the same obvious semantics cover both dialog directions (D-01/D-03) |
| Hook authority | Hooks can return `allow`/`ask` decisions; a blocking (exit-2) deny stops the call before rules | **Deny-only** hook authority from project scope: repo-shipped files can NEVER grant allow; a hook allow still falls through to rule evaluation | Project safety decision (PAR-03): nothing that ships inside the repo may escalate what runs; the blocking-hook precedence matches CC exactly (D-04) |

Everything else — the deny→ask→allow order, specificity-never-reorders,
compound splitting, the `:*` wildcard, the `mcp__` namespace, hooks-before-
rules — is deliberate CC parity.

---

## 5. Degradation and failure semantics

- **Permission ask on a pre-permission client (-32601):** the surface maps
  the error to a fail-safe DECLINE (never an allow) and marks the session
  degraded STICKY (16-D-18) — every later gated ask declines without a new
  round-trip. No retry storm, no silent widening.
- **Transient failures (timeout after the D-14 ladder, transport errors,
  malformed outcomes):** fail-safe decline, non-sticky — the next call asks
  again. An unknown dialog option id is untrusted input: fail-safe decline.
- **Elicitation on older clients:** the elicitation-form capability is
  negotiated at initialize (advertisement-first; else ONE bounded probe).
  A -32601 probe degrades the connection sticky; degraded question-family
  asks land on today's plain-text path VERBATIM (the v1.1 route is the
  fallback, not the primary), and engine asks degrade to their advisory
  note. A valid accept renders into the captured answered form byte-for-byte;
  an invalid accept re-asks exactly ONCE (D-10) then routes like a
  non-answer; decline/cancel never re-ask.
- **Turn death mid-dialog:** `session/cancel` (or logout, or serve shutdown)
  drains the session's asks — the open dialog resolves cancelled through the
  `$/cancel_request` cascade, the transcript carries the cancelled-NORMAL
  result (never an error), and queued asks of the dead turn drain with zero
  fires (D-13). The session stays responsive throughout; nothing holds the
  turn mutex across the wait.

---

## 6. Where the code lives

| Concept | Code |
|---|---|
| The chokepoint (`gateCall`) + verdicts | `internal/session/gate.go` |
| Suspension, outcome matrix, persist-then-execute resume | `internal/session/ask.go` (`resumePermissionAsk`) |
| Ask queue (one outstanding, notes, drain) | `internal/session/askqueue.go` |
| Rule grammar + evaluation | `internal/perm/rules.go` |
| permissions.yaml store (0600 atomic, both directions) | `internal/perm/store.go` |
| Wire frames (request_permission / elicitation) | `internal/acp/types.go` |
| Permission ask surface (frame build + registry fire) | `internal/acpserve/ask_surface.go` (`PermissionAsk`) |
| Elicitation surface (D-08 mapping, D-10 validator, fallback) | `internal/acpserve/ask_surface.go` (`ElicitationAsk`) |
| permissions.mode config option + live flip | `internal/acpserve/config_surface.go` |
| Composition (queue, fire callbacks, mode accessor) | `internal/runtime/runtime.go`, `internal/acpserve/acp_serve.go` |
| End-to-end proof (five-scenario simulator battery) | `internal/acpserve/permissions_e2e_test.go` |

**Drift guards:** the doc names `gateCall` because the code does; the
no-second-gate property is greppable (`perm.RuleSet.Evaluate` call sites in
internal/session appear in gate.go only, non-test sources); and the E2E
battery drives the real serve composition over pipes so the wire story cannot
drift from this document silently.
