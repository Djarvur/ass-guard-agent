# Phase 22: Background Execution + Sandbox Reality - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

All long-lived process work converges on ONE lifecycle infrastructure: full background subagents (discriminated dispatch results, kind-detected task-notifications, output-file retrieval, cancellation), background Bash riding the same task-notification subsystem with process-group lifecycle, persistent-shell Bash via creack/pty, and the sandbox flag made real — landlock (Linux ≥5.13) / generated sandbox-exec profiles (macOS), probe-and-degrade loudly, default OFF.

</domain>

<decisions>
## Implementation Decisions

### Notification Delivery
- **D-01:** Background completion WAKES the model: an agent-initiated turn through the EXISTING automation-turn machinery (cron/steering already run turns without client prompts) with the notification block as the turn's input; inject-at-next-user-turn is the fallback while a client turn is active. Full CC parity (harness re-invokes on completion).
- **D-02:** Notification payload: task id, kind, exit status, duration, output TAIL (last ~N KB), pointer to the full output file. The model reacts immediately; full retrieval stays a Read away (PAR-07's output-file retrieval letter).
- **D-03:** Multiple completions while idle COALESCE into ONE wake turn: all pending notification blocks inject together, ordered by completion time. One model call per batch — no notification storm.
- **D-04:** Sandbox denies FS + NETWORK: confined tool processes get rw on workdir + tmp + `.ass-guard`, ro on system paths, network denied (landlock ABI v4 TCP rules / sandbox-exec network deny). Blocks curl-style exfil from confined tools.
- **D-05:** Sandbox confines SPAWNED tool processes only (Bash-class subprocesses; ruleset applied to children before exec). In-process tools and MCP host connections run unconfined — landlock is process-wide; confining ass-guard itself would break provider/MCP networking. The confinement boundary is documented honestly.
- **D-06:** Profiles come from embedded TEMPLATES with runtime path substitution (workdir/tmp/.ass-guard per session); landlock rulesets build symmetrically from the SAME policy struct — one policy definition, two backends. No runtime-generated profile artifacts on disk.
- **D-07:** ONE PTY per session (CC's model): all persistent-shell Bash calls share it; cd/export persist; concurrent persistent calls serialize on it. Non-persistent Bash stays independent processes.
- **D-08:** PTY lifetime = session lifetime: session/close drains and kills it (18-D-12's cancel-and-drain family); agent shutdown sweeps stragglers (PAR-08's no-orphans letter). A dead shell (EIO, child exit) restarts LAZILY on the next persistent call with a visible note — state loss acknowledged, never silent.
- **D-09:** Persistent shell is a per-call Bash tool argument (model opts in per invocation); default Bash stays stateless. Sandbox and persistence are orthogonal switches.
- **D-10:** Concurrent background subagents are CAPPED (default 8): over-cap dispatches QUEUE with a visible note (FIFO); foreground subagents unaffected. Bounded resources; no hard failure for being busy.
- **D-11:** Background Bash: same cap policy, own larger default (16); over-cap queues with a note. One discipline, two numbers.
- **D-12:** Both caps join the configOptions menu (Phase 16's rule — advertise all, apply-as-landed): `background.subagents=8`, `background.bash=16` defaults, operator-tunable.

### Claude's Discretion
- Output-tail size in the notification payload (D-02's ~N KB).
- The wake-turn's input framing (notification block syntax/markup).
- Landlock ABI negotiation order and the exact ro-system path set per distro.
- ANSI-strip scope (control sequences only vs full SGR), cd/export tracking granularity.
- Queue-note wording and the queued-task visibility surface.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §CC Parity Closures — PAR-07 (background dispatch discriminated results, kind-detected notifications, output-file retrieval, cancellation), PAR-08 (background Bash on the same subsystem; TERM-before-KILL, Pdeathsig Linux, startup stale-log sweep, no orphans), PAR-09 (creack/pty v1.1.24, Setsid + group-kill escalation, EIO-as-EOF, ANSI stripping, best-effort cd/export tracking) verbatim
- `.planning/REQUIREMENTS.md` §Sandbox Reality — SAND-01 (go-landlock v0.10.0 Linux ≥5.13, sandbox-exec targeted denies never deny-default, probe-and-degrade loudly, default OFF, `--sandbox=off`, bubbleware demoted to opt-in) verbatim
- `.planning/ROADMAP.md` §Phase 22 — goal, 4 success criteria; dependency note: Phase 20's per-agent routing MUST land first (mis-routed background launches otherwise)

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/20-built-in-commands-skills-per-agent-model/20-CONTEXT.md` — D-13..D-16 (per-agent model routing: frontmatter > dispatch > parent; cross-provider routing; resolvedModel reporting) — background subagents dispatch through this
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-02 (TurnEmitter ordered frames the notifications ride), task-notification kind (additive schema)
- `.planning/phases/18-session-family/18-CONTEXT.md` — D-12 (close = cancel-and-drain: PTY + background tasks die here), D-01 (replay reconstructs background-task state from transcript)
- `.planning/phases/17-permissions-elicitation/17-CONTEXT.md` — D-07 (automation turns: no human — background wake-turns behave like automation turns for ask-class tools)

### External References
- `pkg.go.dev/github.com/landlock-lsm/go-landlock/landlock` — RODirs/RWDirs ruleset shape; ABI v4 TCP restrictions (D-04's mechanism)
- `docs.kernel.org/userspace-api/landlock.html` — upstream ruleset semantics
- CC's background-task model — harness injects `<task-notification>` on completion and re-invokes the model; TaskOutput deprecated in favor of output-file Read (D-01/D-02's parity target)

### Code Anchors
- `internal/coreexec/bash.go` — process-group kill idiom (Setpgid, kill(-pid)); PAR-08 extends with TERM-before-KILL + Pdeathsig
- Automation-turn machinery (cron/steering turn runner) — D-01's wake-turn vehicle
- TurnEmitter + transcript kinds (Phase 16) — notification frames and durable records

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Process-group discipline (coreexec/bash.go): Setpgid + group-kill already proven; escalation ladder + Pdeathsig are additive.
- Automation-turn machinery: turns without client prompts already exist — the wake-turn is a new trigger on old rails.
- Subagent PARA machinery + 20's model routing: background dispatch reuses the foreground path plus async result discrimination.
- Config registration (16's configOptions): D-12 joins the standing menu.

### Established Patterns
- Graceful degradation + loud counters — sandbox probe-and-degrade (SAND-01 letter), dead-PTY restart notes (D-08), queue notes (D-10/D-11).
- Transcript-as-truth — task state reconstructs on resume (18-D-01); notification records are transcript lines.
- By-construction safety — one policy struct feeding both sandbox backends (D-06).

### Integration Points
- The task-notification subsystem: new module sitting between tool execution and the turn machinery — both subagent and Bash completions feed it.
- session/close (18-D-12) drains background tasks + PTY.
- Bash tool schema gains run-in-background + persistent-shell arguments.
- Startup sweep (stale logs) joins serve-start sequencing.

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- All-recommended sweep — wake-turn notifications, FS+network sandbox on spawned processes only, template-based profiles, one session PTY with lazy restart, per-call persistence opt-in, queued caps in configOptions.
- The confinement boundary honesty (D-05) matters: sandbox documents exactly what it does NOT confine.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 22-background-execution-sandbox-reality*
*Context gathered: 2026-08-27*
