# Phase 22: Background Execution + Sandbox Reality - Research

**Researched:** 2026-08-28
**Domain:** Go long-lived-process lifecycle (background subagents, background Bash, PTY persistent shell) + OS sandboxing (Linux Landlock / macOS Seatbelt)
**Confidence:** HIGH (codebase anchors read this session; both sandbox backends tool-verified; platform facts live-verified on the dev host)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Notification Delivery**
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

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PAR-07 | Full subagents: background dispatch via discriminated results (completed/async_launched), structured task-notifications detected by kind (not text-match), output-file retrieval for running tasks, cancellation | Captured Agent schema already advertises `run_in_background` [VERIFIED: internal/toolcat/coretools.json:29-32]; DispatchSubagent is synchronous today [VERIFIED: internal/session/subagent.go:43-123]; discriminated-result + tracker + notification design in §Pattern 2/3 |
| PAR-08 | Background Bash: completion notifications ride the same task-notification subsystem; process-group lifecycle (TERM-before-KILL escalation, Pdeathsig on Linux, startup stale-log sweep) | TaskRegistry already owns background Bash (Start/Output/Stop/ReapAll) but has no notifications and SIGKILL-only [VERIFIED: internal/coreexec/background.go]; escalation ladder + Pdeathsig verified in §Standard Stack/§Code Examples |
| PAR-09 | Persistent-shell Bash option via creack/pty v1.1.24 (Setsid + group-kill escalation, EIO-as-EOF, ANSI stripping, best-effort cd/export tracking) | creack/pty v1.1.24 verified current (go module proxy), MIT, pure-syscall, CGO-free; StartWithAttrs signature verified; EIO-as-EOF + sentinel pattern in §Pattern 5 |
| SAND-01 | Sandbox flag real: landlock-lsm/go-landlock v0.10.0 (Linux, kernel ≥5.13) / sandbox-exec generated `.sb` profiles embedded (macOS, targeted denies never deny-default); probe-and-degrade loudly; default OFF; `--sandbox=off`; bubblewrap demoted to opt-in | go-landlock v0.10.0 verified (go module proxy + pkg.go.dev) AND CGO_ENABLED=0 GOOS=linux build verified this session; sandbox-exec live-verified on macOS 26.5.1 (network + write denies both enforced); ABI table + probe from kernel docs |
</phase_requirements>

## Project Constraints (from AGENTS.md / repo)

- **Tech stack:** Go; go.mod declares `go 1.26`; .mise.toml pins `go = "1.26"` [VERIFIED: go.mod, .mise.toml]
- **CGO_ENABLED=0 discipline:** static binary — every new dependency must compile cgo-free (verified for both new packages this session)
- **Transport discipline:** stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr — PTY/background child output must NEVER touch stdout
- **Compatibility:** Claude Code config layout drop-in; additions namespaced
- **Distribution:** no daemon, no network port; editor owns process lifecycle (background work dies with the process — ReapAll discipline)
- **Platform:** macOS + Linux, amd64 + arm64; Windows deferred → all Linux-only code must be build-tagged
- **Safety model:** no tool-execution confirmation tier; pattern/hook table + manual cancellation only

## Summary

The codebase already owns roughly half of this phase's machinery, and the captured tool catalog has been promising the other half on ass-guard's behalf since the 12-05 harvest. `internal/coreexec/background.go` is a working per-session `TaskRegistry` (start with immediate `exec_<hex>` id return, bounded output accumulation, progressive log under `.ass-guard/outputs/<id>.log`, whole-group Stop, ReapAll on session close, cap 16) [VERIFIED: internal/coreexec/background.go:30-318], and the Bash executor already branches on `run_in_background` [VERIFIED: internal/coreexec/bash.go:271-274]. The captured catalog's Agent tool carries `run_in_background` ("Set to true to run this agent in the background. You will be notified when it completes.") and Bash's description says "re-invokes you when it exits" [VERIFIED: internal/toolcat/coretools.json:29-32, 167] — **no schema changes are needed for the background legs; the executors just have to honor what the catalog already advertises.** What does not exist anywhere: completion notifications, background (async) subagent dispatch, output-tail delivery, TERM-before-KILL escalation, Pdeathsig, the stale-log sweep, and every line of the sandbox. `DispatchSubagent` is strictly synchronous (goroutine + select) [VERIFIED: internal/session/subagent.go:89-122], the session package never parses `run_in_background` [VERIFIED: grep of internal/session], and the only kill discipline is SIGKILL-the-group [VERIFIED: internal/coreexec/bash.go:95-138].

The two external dependencies check out cleanly. `creack/pty` v1.1.24 is the module's latest v1 tag (v2 exists; REQUIREMENTS pins v1.1.24), MIT-licensed, pure-syscall with darwin+linux files [VERIFIED: go module proxy + pkg.go.dev]. `landlock-lsm/go-landlock` v0.10.0 — published Aug 24, 2026, exactly the version REQUIREMENTS pins — compiles under `GOOS=linux CGO_ENABLED=0` (verified this session with a real build), brings presets V1–V10 including `RestrictNet` with TCP (V4) and brand-new UDP (V10) rules, and depends only on `golang.org/x/sys` plus the official `kernel.org/...psx` shim. On the macOS side, sandbox-exec was **live-verified on this dev host (macOS 26.5.1)**: `(allow default)` + `(deny network*)` broke a curl connect (exit 7) and `(deny file-write*)` made `touch` fail with EPERM — the exact "targeted denies never deny-default" mechanism SAND-01 specifies, still fully functional. Landlock's kernel contract (per-thread, clone-inherited, irreversible, `landlock_create_ruleset(NULL,0,VERSION)` probe, ENOSYS/EOPNOTSUPP error taxonomy, v4 TCP) was verified against docs.kernel.org, and it confirms D-05's framing: rulesets must be applied **in the child** before exec — since `os/exec` has no pre-exec hook, the standard Go shape is a tiny re-exec of ass-guard's own binary with a sentinel argument.

The hardest integration work is the ONE task-notification subsystem (D-01..D-03, D-10..D-12) and its wake-turn vehicle. The automation-turn machinery (`runAutomationTurn`, `sessionTurnMu` TryLock-queue semantics, `runOneTurn(ctx, sess, blocks)`) is the locked vehicle and needs only a new trigger + a coalescing pending-queue [VERIFIED: internal/runtime/cron_wiring.go:124-205]. Two planner-level tensions surfaced: (1) D-11's queue-on-overcap **changes** the TaskRegistry's current fail-with-error behavior [VERIFIED: internal/coreexec/background.go:145-149]; (2) PAR-09's per-call `persistent` argument cannot ride the captured Bash schema unmodified (`additionalProperties: false`, no such property [VERIFIED: internal/toolcat/coretools.json:168-196]) — a deliberate catalog-extension decision the planner must put in front of the operator. Also note `dangerouslyDisableSandbox` ("dangerously override sandbox mode and run commands without sandboxing" [VERIFIED: coretools.json:187-190]) currently parses as a documented no-op [VERIFIED: bash.go:58-64] — with SAND-01 the flag acquires real meaning and should be honored or explicitly re-documented.

**Primary recommendation:** Build one `task` subsystem (tracker + kinds + pending-notification queue + wake-turn drain) that both background subagents and background Bash feed; extend `TaskRegistry` with the escalation ladder and queue-on-cap rather than bolting a parallel registry; confine children via a self re-exec (Linux) and in-memory `sandbox-exec -p` profiles (macOS) driven by one policy struct; gate everything behind `--sandbox` default OFF with a startup ABI probe and loud stderr degradation.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Background subagent dispatch (discriminated result) | API/Backend (session core `DispatchSubagent`) | Runtime Runner (tracker/queue) | The dispatch site is `session.go:443`; background mode returns a tool result there and hands lifecycle to a runner-level tracker |
| Task-notification subsystem (kinds, coalescing) | API/Backend (new `internal` module) | Runtime Runner (wake drain) | Sits between tool execution and the turn machinery (per CONTEXT code_context); completions from both legs feed it |
| Wake-turn on completion | Runtime Runner (automation rails) | — | D-01 locks the vehicle: `runAutomationTurn`'s mutex/queue/provenance pattern with `runOneTurn` |
| Background Bash lifecycle + escalation ladder | API/Backend (coreexec `TaskRegistry`) | — | The registry already owns start/stop/output; ladder and queue are additive there |
| Persistent shell via PTY | API/Backend (coreexec — new pty module) | Session close chain (drain) | D-07 one PTY per session; lifecycle joins `OnClose` where `ReapAll` already runs [VERIFIED: runtime.go:1238-1243] |
| Sandbox policy + backends | Process-execution tier (coreexec wiring) | Serve composition (flag + startup probe) | Policy struct + backends are execution-path; the flag/probe live at the composition root (`acp_serve.go` flags, `acpserve.Options`) |
| Caps in configOptions | Frontend Server (acpserve `ConfigSurface`) | — | D-12 joins the Phase-16 menu (`Options()`/`build()` pattern [VERIFIED: internal/acpserve/config_surface.go:136-149, 479-513]) |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/creack/pty` | v1.1.24 (latest v1 tag; published Oct 31 2024) | Persistent-shell PTY: `StartWithAttrs`/`StartWithSize`, `Setsize`/`InheritSize`, `Winsize` | PAR-09 letter pins this exact version [VERIFIED: go module proxy `go list -m github.com/creack/pty@latest` → v1.1.24; pkg.go.dev]. MIT, pure syscalls (no .c files; darwin+linux platform files), zero deps |
| `github.com/landlock-lsm/go-landlock` | v0.10.0 (published Aug 24 2026) | Linux sandbox backend: `RestrictPaths` (RODirs/RWDirs/ROFiles/RWFiles/PathAccess), `RestrictNet` (ConnectTCP/BindTCP V4; BindUDP/ConnectSendUDP new in v0.10.0) | SAND-01 letter pins v0.10.0 [VERIFIED: go module proxy — v0.10.0 is the latest tag]. Official landlock-lsm org. **Build-verified this session: `GOOS=linux CGO_ENABLED=0 go build` PASS.** Deps: `golang.org/x/sys` + `kernel.org/pub/linux/libs/security/libcap/psx` v1.2.77 (official; pure-Go path compiles cgo-free) |
| stdlib `syscall` / `os/exec` | go 1.26 | Process groups (`Setpgid`), `Pdeathsig` (linux-only field), `Cmd.Cancel`/`WaitDelay`, signals | Pdeathsig verified present on GOOS=linux SysProcAttr, absent on darwin → build-tag split [VERIFIED: `go doc syscall.SysProcAttr`] |
| `/usr/bin/sandbox-exec` | ships with macOS (26.5.1 live-verified) | macOS sandbox backend | Deprecated-in-man-page since ~2017 but never removed; used by Claude Code, OpenAI Codex, Apple containerization [CITED: github.com/apple/containerization issue #737; news.ycombinator.com/item?id=44283454]. **Live-verified this session:** targeted network + write denies enforced |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| stdlib `regexp`, `strings` | — | ANSI CSI/SGR stripping on PTY capture; sentinel-line parsing | Persistent-shell capture only — never on foreground Bash |
| `stretchr/testify` | v1.11.1 (already in go.mod) | Assertions for the new batteries | All new tests |
| internal `event.Bus` / `TurnEmitter` | (existing) | Live notification surfacing if desired; the durable record is the transcript | Wake-turn machinery already forwards server-driven turn output to the client via the session forwarder [VERIFIED: cron_wiring.go:223-286] |

### Deliberately NOT dependencies
| Need | Decision | Why |
|------|----------|-----|
| ANSI stripping | hand-rolled regex (~10 lines) | A dependency for a CSI regex violates the minimal-deps posture; scope is Claude's discretion |
| Task-notification subsystem | internal package | Domain logic; no library exists for "agent task notifications" |
| TERM-before-KILL ladder | stdlib signals | `syscall.Kill(-pid, SIGTERM)` + timer + SIGKILL is trivial; the group idiom already exists in-repo |
| bubblewrap | demoted to opt-in stronger-isolation (SAND-01 letter) | Out of scope for the default path; Ubuntu 24.04+ AppArmor userns clamp (REQUIREMENTS Out-of-Scope table) |

**Installation:**
```bash
go get github.com/creack/pty@v1.1.24
go get github.com/landlock-lsm/go-landlock@v0.10.0
```
(landlock import must live in linux-build-tagged files only — it builds on darwin as a dep but the runtime probe is meaningless there; keeping the import behind `//go:build linux` keeps the darwin build clean and documents intent.)

## Package Legitimacy Audit

> The `gsd-tools package-legitimacy check` seam does not support the Go ecosystem (returns a usage error listing npm|pypi|crates only — verified this session). Per the Go-ecosystem equivalent: both packages were verified against the **Go module proxy** (authoritative registry for Go) and their **official source repositories**, plus compile/run verification for go-landlock.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `github.com/creack/pty` | Go module proxy | 9+ yrs (repo), v1.1.24 Oct 2024 | ubiquitous (docker, containerd, kubectl ecosystems) | github.com/creack/pty (author's canonical repo) | OK | Approved |
| `github.com/landlock-lsm/go-landlock` | Go module proxy | ~5 yrs (repo), v0.10.0 Aug 24 2026 | official LSM project; referenced from docs.kernel.org ecosystem | github.com/landlock-lsm/go-landlock (official `landlock-lsm` org) | OK | Approved |
| `kernel.org/.../libcap/psx` v1.2.77 | Go module proxy (indirect dep) | official kernel.org publication | — | kernel.org (official) | OK | Indirect; compiles cgo-free (verified) |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none
**Seam note:** `--ecosystem go` is unsupported by the legitimacy seam; the module-proxy + official-org + compile-verification evidence above substitutes. No package names were discovered from non-authoritative sources.

## Architecture Patterns

### System Architecture Diagram

```
                       ┌──────────────────────────────────────────────────────┐
                       │                    ass-guard process                  │
                       │                                                      │
 model ──Agent tool────┼─► session.go:443 dispatch site                       │
        run_in_background:true?                                │
        │ yes: discriminated result ──► tool_result {status:"async_launched",│
        │        task_id, output_file} returned IMMEDIATELY                  │
        │ no: existing synchronous path (unchanged)                          │
        │                                                                    │
 model ──Bash tool─────┼─► BashExecute ─ run_in_background? ─► TaskRegistry ──┤
                       │                                        │            │
                       │        ┌───────────────────────────────┘            │
                       │        ▼                                            │
                       │  ┌───────────────────┐   cap reached (subagents 8 /  │
                       │  │  TaskTracker       │   bash 16): FIFO QUEUE +     │
                       │  │ (one subsystem)    │   visible note (D-10/D-11)   │
                       │  │  kinds: bash /     │◄──── completion callbacks ───┤
                       │  │  subagent          │   (goroutines + Wait)        │
                       │  └────────┬──────────┘                              │
                       │           │ completion: kind, exit, duration,       │
                       │           │ tail(N KB), output-file pointer (D-02)  │
                       │           ▼                                         │
                       │  ┌───────────────────┐   client turn active (D-01   │
                       │  │ pending-notify     │   fallback): keep coalescing │
                       │  │ queue (per session)│   (D-03)                     │
                       │  └────────┬──────────┘                              │
                       │           │ drain: TryLock sessionTurnMu            │
                       │           ▼                                         │
                       │  runOneTurn(ctx, sess, [notification blocks])       │
                       │  (D-01: automation-turn rails; wake provenance)     │
                       │           │                                         │
                       │           ▼                                         │
                       │  model reacts → Read output file for full output    │
                       │  model cancels → TaskStop (bash + subagent tasks)   │
                       │                                                     │
                       │  Bash persistent:true ──► ONE PTY/session (D-07)    │
                       │   write cmd+\n+sentinel-echo; read master;          │
                       │   ANSI-strip; EIO-as-EOF; serialize calls           │
                       │                                                     │
                       │  Bash (sandboxed, --sandbox=on) ──► policy struct   │
                       │   ├─ linux:  re-exec self (sentinel) → go-landlock  │
                       │   │          RestrictPaths+RestrictNet → execve target
                       │   └─ darwin: sandbox-exec -p <rendered template>    │
                       │              sh -c <command>                        │
                       └──────────────────────────────────────────────────────┘
                                          │                   │
                             provider/MCP networking (UNCONFINED, D-05)
                             child process tree (CONFINED; group-killed)
```

### Recommended Project Structure
```
internal/
├── coreexec/
│   ├── background.go      (MOD: queue-on-cap D-11, escalation ladder, kind-notify hook)
│   ├── bash.go            (MOD: persistent arg, sandbox wrap, disable-sandbox semantics)
│   ├── ptty.go            (NEW: per-session PTY shell manager — D-07/D-08/D-09)
│   └── ansistrip.go       (NEW: CSI/SGR stripper, pure + table tests)
├── tasks/                 (NEW: the ONE task-notification subsystem)
│   ├── tracker.go         (kinds enum, task records, discriminated results, cancel)
│   ├── notify.go          (payload per D-02, pending queue, coalescing per D-03)
│   └── subagent.go        (async subagent leg: serve-ctx runner, output file, cancel)
├── sandbox/               (NEW)
│   ├── policy.go          (ONE policy struct → both backends, D-06)
│   ├── landlock_linux.go  (//go:build linux — probe, ABI negotiate, child rulesets)
│   ├── child_linux.go     (//go:build linux — re-exec sentinel entrypoint)
│   └── seatbelt_darwin.go (//go:build darwin — embedded template + `-p` render)
├── runtime/
│   ├── cron_wiring.go     (wake-drain: reuse TryLock/provenance; new trigger)
│   └── runtime.go         (wire tracker into sessionFor; OnClose drain; startup sweep)
└── acpserve/
    └── config_surface.go  (D-12: background.subagents / background.bash menu entries)
```

### Pattern 1: One task-notification subsystem, two producers
**What:** A runner-owned tracker records every background task (kind `bash` or `subagent`), accepts completion callbacks, and maintains a per-session pending-notification queue.
**When to use:** Always — PAR-07 and PAR-08 explicitly require ONE subsystem.
**Shape (kinds + payload — D-02 fields verbatim):**
```go
// TaskKind discriminates producers; detection is by kind, never text-matching.
type TaskKind string

const (
    KindBash     TaskKind = "bash"     // PAR-08
    KindSubagent TaskKind = "subagent" // PAR-07
)

// Notification is the D-02 payload: task id, kind, exit status, duration,
// output TAIL, pointer to full output file.
type Notification struct {
    TaskID     string   // existing exec_<hex> shape for bash (12-06); subagent minted same way
    Kind       TaskKind
    ExitStatus string   // "0" | nonzero | "killed" | "error"
    Duration   time.Duration
    Tail       string   // last ~N KB (discretion default ~8KB), ANSI-stripped
    OutputFile string   // .ass-guard/outputs/<id>.log (bash) / <id>.log (subagent)
}
```
**Key links:** the bash leg already persists full output to `.ass-guard/outputs/<id>.log` and tees a bounded in-memory copy [VERIFIED: background.go:120-206] — the notification's Tail + OutputFile come straight from those two buffers.

### Pattern 2: Discriminated background dispatch (PAR-07)
**What:** At the `session.go:443` dispatch site, parse `run_in_background` from the Agent input (the field EXISTS in the captured schema [VERIFIED: coretools.json:29-32]; the session package currently ignores it). If true, register the task, return immediately, and let the goroutine detach under a LONGER-LIVED context.
**Critical correctness point:** `DispatchSubagent` today selects on the CALLER's ctx — for background mode the nested loop must run under the session/serve-lifetime ctx (the turn ends the moment dispatch returns; a turn-scoped ctx would kill the subagent instantly). Cancellation then rides the tracker's per-task cancel func (same mechanism `TaskStop` uses for bash).
**Example result shape:**
```go
// foreground (unchanged): tool result IS the final text
// background (new):
{"status":"async_launched","task_id":"exec_...","output_file":"<workdir>/.ass-guard/outputs/exec_....log"}
// (a foreground dispatch that finishes inline may return {"status":"completed", ...} —
//  the discrimination is the status field, per PAR-07's completed/async_launched letter)
```

### Pattern 3: Wake-turn via the automation rails (D-01/D-03)
**What:** A drain goroutine (or completion-callback hook) does exactly what `fireDueAutomations` does: `TryLock` the session's `sessionTurnMu`; if held (client turn active), leave notifications pending (they coalesce) and retry on turn end / next tick; if acquired, render ALL pending notifications (ordered by completion time) as one user-turn block list and call `runOneTurn(ctx, sess, blocks)` with a wake provenance line mirroring the automation `EngineDecision` write [VERIFIED: cron_wiring.go:148-205].
**Why:** D-01 locks this vehicle; the rails already solve turn serialization, client-turn coexistence (WINDOWS #3 forwarder muting), and provenance.

### Pattern 4: Child-only sandbox confinement (D-04/D-05/D-06)
**What:** One policy struct → two backends → applied to the child, never to ass-guard.
```go
// Source: docs.kernel.org/userspace-api/landlock.html (verified 2026-08-28):
// "Every new thread resulting from a clone(2) inherits Landlock domain
// restrictions from its parent" + "Once a thread is landlocked, there is no
// way to remove its security policy". go-landlock's Restrict* applies to the
// whole process ⇒ apply IN THE CHILD.
type Policy struct {
    RWPaths    []string // workdir, tmp, .ass-guard (D-04)
    ROSysPaths []string // per-distro ro set (discretion)
    DenyNetwork bool    // always true for v1 (D-04)
}
```
- **Linux:** `os.Executable()` re-invoked with a sentinel env (e.g. `__ASS_GUARD_SANDBOX_CHILD=1`); the child main path builds the ruleset from the policy, applies it (RestrictPaths + RestrictNet), then `syscall.Exec`s the real target. `os/exec` has no pre-exec child hook (only `Cancel`/`WaitDelay` [VERIFIED: go doc os/exec.Cmd]) so re-exec is the standard Go shape. **This makes ass-guard's binary the sandbox loader — no extra artifact.**
- **macOS:** embedded `.sb` template with path placeholders, rendered IN MEMORY (D-06: no on-disk artifacts) and passed via `sandbox-exec -p '<rendered>'` (`-p` takes the profile as an argv string — verified pattern [CITED: danmackinlay.name/notebook/sandboxed_apps.html]):
```lisp
(version 1)
(allow default)
(deny network*)
(deny file-write* (subpath "/System") (subpath "/usr"))
;; … targeted denies; rw allowed via (allow default) + targeted denies only (SAND-01:
;; targeted denies NEVER deny-default)
```

### Pattern 5: PTY persistent shell (D-07/D-08/D-09)
**What:** One lazy PTY per session. Because a PTY has no command boundary, completion is detected with a sentinel echo — the standard pattern:
```go
// Source: pkg.go.dev/github.com/creack/pty (verified 2026-08-28)
pty, err := pty.StartWithAttrs(cmd, &pty.Winsize{Rows: 24, Cols: 80},
    &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0})
// per call: write "command\necho __ASS_GUARD_DONE_$?__\n" to the master,
// read until the sentinel line; strip ANSI from capture; EIO (linux) ==
// clean EOF after the child exits — treat both as EOF.
```
- Serialize concurrent persistent calls on one mutex (D-07). Non-persistent Bash stays the existing `sh -c` path.
- Dead shell (read EOF/EIO, or process gone): mark dead, restart lazily on next persistent call WITH a visible note (D-08).
- Drain on `OnClose` (where `taskRegistry.ReapAll()` already runs [VERIFIED: runtime.go:1238-1243]): TERM-then-KILL the shell's session group, close the master fd.

### Pattern 6: TERM-before-KILL + Pdeathsig escalation (PAR-08)
```go
// Linux-only fields ⇒ //go:build linux file. Verified from go doc:
// "Pdeathsig, if non-zero, is a signal that the kernel will send to the
//  child process when the creating thread dies. Note that the signal is
//  sent on thread termination, which may happen before process termination.
//  There are more details at https://go.dev/issue/27505."
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}

// escalation ladder (portable): TERM the group, grace window, then the
// existing SIGKILL group-kill + reapGroup (bash.go idiom).
_ = syscall.Kill(-pid, syscall.SIGTERM)
select {
case <-done:
case <-time.After(grace): // discretion, e.g. 5s
    _ = syscall.Kill(-pid, syscall.SIGKILL)
    reapGroup(pid)
}
```

### Pattern 7: Phase-20 dependency seam (delegation-seam / precondition-mark)
Phase 20 is PLANNED, not executed (STATE.md confirms current position at Phase 21; 20-03's routing contract is locked on paper). Background dispatch MUST resolve models through 20-03's resolver, not the 14-05 light-tier default it replaces:
- 20-03 commits to: precedence frontmatter > dispatch > session > tier (`inherit`/empty → parent), cross-provider routing via `ProviderFactory.BuildWithCapturer` cached per (provider, session), degrade-to-parent + exactly ONE loud warning, `resolvedModel` on the dispatch line [VERIFIED: 20-03-PLAN.md truths + Task 1-3].
- The background dispatch passes the SAME dispatch-time model input (the Agent schema's `model` field [VERIFIED: coretools.json:20-28 — enum sonnet/opus/haiku with "Takes precedence over the agent definition's model frontmatter" — note this schema text contradicts D-13's frontmatter-first precedence; D-13 wins as the locked decision]) into the runner's resolver exactly as the foreground path does. If 20-03 has not landed when Phase 22 executes, plan a precondition task or a marked delegation seam (Phase-21 gate-join precedent) — never a second resolver.

### Anti-Patterns to Avoid
- **Applying landlock in ass-guard itself:** process-wide restriction kills provider/MCP networking (D-05 names this explicitly). Confine children only.
- **Trusting `BestEffort()` as enforcement:** go-landlock docs: "A best-effort call to RestrictPaths() will succeed without error even when Landlock is not available at all on the current kernel" [CITED: pkg.go.dev/github.com/landlock-lsm/go-landlock/landlock]. Probe with the strict-mode error / ABI probe, log loudly, then degrade.
- **Text-matching notifications:** the "You will be notified" promise must be delivered as structured kinds (PAR-07 letter), not by grepping output.
- **Turn-scoped ctx for background subagents:** the dispatch returns; the turn's ctx dies. Background work needs session/serve-lifetime ctx + explicit cancel.
- **Blocking dispatch on the provider semaphore:** over-cap must QUEUE (D-10/D-11) — note this REVERSES the TaskRegistry's current fail-with-error [VERIFIED: background.go:145-149].
- **Killing only the direct child / SIGKILL-first:** wrapper shells fork grandchildren; SIGKILL-only is today's gap — add TERM-first + group semantics (PAR-08 letter).
- **Letting PTY output touch stdout:** capture goes to file + bounded buffer + stderr counters only (transport discipline).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PTY open/atexit/ioctl handling | raw posix_openpt/grantpt via syscalls | `creack/pty` v1.1.24 | Platform-specific ioctl zoo (TIOCGWINSZ, termios); the library carries darwin+linux+bsd files, is 9+ years battle-tested (docker/kubectl lineage) |
| Landlock syscall wrappers + thread sync | raw `landlock_create_ruleset`/`add_rule`/`restrict_self` + psx-style thread broadcast | `go-landlock` v0.10.0 | ABI negotiation, `no_new_privs`, TSYNC (v8) thread-sync semantics, and the psx cross-thread syscall problem are exactly the "deceptively complex" surface; hand-rolling invites silent non-enforcement |
| Sandbox policy duplication | two divergent policy definitions | ONE policy struct feeding both backends | D-06 locks symmetry; drift between backends would make "sandboxed" mean different things per OS |
| Background process reaping | ad-hoc wait/kill per call site | existing `TaskRegistry` + `killGroupOnCtx`/`reapGroup` idiom | The fork-vs-kill straggler race is already solved and tested in-repo [VERIFIED: bash.go:88-138] |

**Key insight:** the sandbox domain punishes hand-rolling with SILENT failures — a wrong landlock rule set or a missed TSYNC thread fails open (unconfined) without errors. Both libraries chosen here are the ones their ecosystems use precisely because the failure modes are subtle.

## Runtime State Inventory

Not a rename/refactor/migration phase — omitted per protocol (greenfield feature work on existing seams).

## Common Pitfalls

### Pitfall 1: BestEffort silently enforces nothing
**What goes wrong:** `landlock.V1.BestEffort().RestrictPaths(...)` returns nil on kernels with no Landlock at all — you believe tools are confined while they run free.
**Why it happens:** Best-effort is designed to opportunistically degrade; it cannot signal absence.
**How to avoid:** At serve-start (when `--sandbox=on`), probe: `landlock.CreateRuleset`-style version probe semantics (`landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION)`; `ENOSYS` = unsupported, `EOPNOTSUPP` = disabled [CITED: docs.kernel.org/userspace-api/landlock.html]). If below the required level (v4 for the network deny), log ONE loud stderr warning + counter and mark the sandbox unavailable — every run then reports it ran unconfined.
**Warning signs:** sandboxed curl succeeding in tests on an old kernel.

### Pitfall 2: MPTCP blind spot in network restriction
**What goes wrong:** go-landlock docs: "Since Go 1.24, Multipath TCP is the default for net.Listen, and therefore, net.Listen can not be restricted with Landlock" (MPTCP sockets are outside the TCP rules) [CITED: pkg.go.dev go-landlock].
**Why:** kernel bug tracked at landlock-lsm/linux#54.
**How to avoid:** For ass-guard v1 this mainly matters if a *confined child* is itself a Go ≥1.24 program that listens — document the boundary honestly (the D-05 honesty note); non-Go children (curl, npm) are unaffected on the connect side.
**Warning signs:** a confined helper binding a port it shouldn't.

### Pitfall 3: Pdeathsig fires on THREAD death, not process death
**What goes wrong:** the kernel delivers Pdeathsig when the creating OS thread exits — the Go scheduler may retire that thread before the process dies, killing the child prematurely (go.dev/issue/27505).
**How to avoid:** set Pdeathsig in the Start path (Go sets it at fork time from the forking thread); keep the Start call on a pinned goroutine (`runtime.LockOSThread` around `cmd.Start()`) if premature kills appear in tests; document the residual risk. Linux-only — build-tagged.
**Warning signs:** background children dying with SIGKILL while ass-guard is alive and healthy.

### Pitfall 4: EIO is not an error on the PTY master
**What goes wrong:** on Linux, reading the master after the child exits returns `EIO`, not `EOF` — treating it as an error marks healthy sessions failed and can spin a read loop.
**How to avoid:** map `errors.Is(err, syscall.EIO)` → clean EOF (PAR-09 letter "EIO-as-EOF"); darwin typically returns EOF. Handle both.
**Warning signs:** dead-shell detection firing with scary error logs on every normal exit.

### Pitfall 5: PTY master fd leak across session close
**What goes wrong:** one leaked master fd per session; the editor-owned process accumulates fds over long lives.
**How to avoid:** close the master in the `OnClose` chain (the established composition point), and drain via TERM→KILL on the shell's session group (Setsid makes the shell a session leader: `kill(-pid)` reaches it). `syscall.SetNonblock` if deadlines on reads are wanted [CITED: creack/pty README].
**Warning signs:** `lsof` fd growth across resume cycles (18's session/load multiplies this).

### Pitfall 6: Catalog schema is closed (`additionalProperties: false`)
**What goes wrong:** adding a `persistent` argument to the Bash executor while the captured schema lacks the property means strict clients/providers reject the call — the model can never legally pass it.
**How to avoid:** decide explicitly (Open Question 1): extend the captured Bash `input_schema` additively (a deliberate, documented waiver of the 08-05 byte-identical discipline, justified by the 2026-08-25 pivot away from the mimicry bar), or expose persistence another way. Note the Agent/Bash background fields need NO such change — they already exist [VERIFIED: coretools.json:29-32, 183-186].
**Warning signs:** tool-call validation errors in provider responses.

### Pitfall 7: Over-cap behavior regression
**What goes wrong:** D-11 wants queue-with-note; `TaskRegistry.Start` currently returns `errBgCap` ("Over-cap starts fail with the structured error naming the cap, T-12-06-01") [VERIFIED: background.go:145-149, comment at 140-141].
**How to avoid:** change the semantics deliberately (FIFO waiters drained on completion), keep a cap-error path only for pathological growth (queue length bound), and update the 12-06 tests rather than deleting the coverage.
**Warning signs:** old cap tests failing → they must be rewritten to the queue contract, like Phase 20 rewrote the 14-05 tier tests.

### Pitfall 8: Notification storms / duplicate wakes
**What goes wrong:** one wake turn per completion floods the provider; two drains racing produce duplicate model turns.
**How to avoid:** coalesce (D-03) — one pending queue per session, single drain point under `sessionTurnMu` (the TryLock pattern makes concurrent drains impossible by construction [VERIFIED: cron_wiring.go:153-160]), order by completion time, dedupe by task id.
**Warning signs:** multiple consecutive assistant turns with no user input; duplicated notification blocks.

### Pitfall 9: Subagent output-file lifecycle
**What goes wrong:** "output retrievable for running tasks" — if the subagent's transcript only lands at completion, retrieval mid-run fails; if the file is never written on cancel, the pointer dangles.
**How to avoid:** stream/append subagent progress (text chunks, tool results) to `.ass-guard/outputs/<id>.log` as produced (same family as bash logs); on cancel, append a killed marker line so Read always resolves.
**Warning signs:** Read of a running task's file returning empty; 404-style structured errors after cancel.

### Pitfall 10: Startup order — sweep before sessionFor
**What goes wrong:** the stale-log sweep runs after sessions open and races fresh task logs.
**How to avoid:** run the sweep in the serve-start pipeline (acpserve.Run's startup sequence, before scheduler start), scanning `.ass-guard/outputs/` for logs with no live owning task; mark or GC per discretion; never delete the directory.
**Warning signs:** fresh logs vanishing on restart.

## Code Examples

### Startup sandbox probe (Linux leg)
```go
// Source: docs.kernel.org/userspace-api/landlock.html — probe idiom:
//   abi = landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION);
//   ENOSYS = "Landlock is not supported by the current kernel."
//   EOPNOTSUPP = "Landlock is currently disabled."
// go-landlock equivalent: strict-mode error as the probe.
err := landlock.V4.RestrictNet() // requires ABI >= 4 (kernel >= 6.7) for TCP rules
if err != nil {
    // LOUD degrade: stderr + counter; mark sandbox unavailable; runs unconfined.
}
```

### go-landlock child ruleset (inside the re-exec sentinel)
```go
// Source: pkg.go.dev/github.com/landlock-lsm/go-landlock/landlock (v0.10.0)
err := landlock.V4.RestrictPaths(
    landlock.RODirs(policy.ROSysPaths...),
    landlock.RWDirs(policy.RWPaths...), // workdir, tmp, .ass-guard (D-04)
)
if e2 := landlock.V4.RestrictNet(); e2 != nil { /* v4 unavailable: net stays allowed → loud note */ }
// then: syscall.Exec(target, argv, env)
// NOTE: RWDirs does NOT grant refer/ioctl-dev/resolve-unix rights (WithRefer etc.) —
// cross-dir rename inside workdir needs ABI>=2 + WithRefer() on kernels >= 5.19.
```

### sandbox-exec render + invoke (darwin leg)
```go
// Live-verified on macOS 26.5.1 (this session):
//   (version 1)(allow default)(deny network*)  → curl connect refused (exit 7)
//   + (deny file-write*)                        → touch: Operation not permitted
argv := []string{"sandbox-exec", "-p", renderedProfile, "sh", "-c", command}
cmd := exec.Command(argv[0], argv[1:]...) // Setpgid as usual; -p = profile string,
// no temp file (D-06: no runtime-generated profile artifacts on disk)
```

### PTY call with sentinel + EIO handling
```go
// Source: pkg.go.dev/github.com/creack/pty v1.1.24 (verified)
// "StartWithAttrs assigns a pseudo-terminal tty os.File to c.Stdin, c.Stdout,
//  and c.Stderr, calls c.Start, and returns the File of the tty's corresponding pty."
ptm, err := pty.StartWithAttrs(sh, &pty.Winsize{Rows: 24, Cols: 80},
    &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0})
_, werr := ptm.WriteString(cmdText + "\necho __ASS_GUARD_DONE_$?__\n")
// read loop: on errors.Is(err, syscall.EIO) → treat as clean EOF (linux)
// sentinel line captured → parse exit status; ANSI-strip before persisting
```

### Escalation ladder + queue-on-cap (registry)
```go
// Existing, keep: Setpgid + kill(-pid, SIGKILL) + reapGroup [bash.go:95-138]
// Add: Pdeathsig (linux-only, see Pitfall 3) + TERM-first ladder (Pattern 6)
// Change (D-11): over-cap → append to FIFO waiters, return queued-note form;
// drain waiters as tasks complete; bounded queue length.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Landlock ABI v1–v3 (FS only) | v10 UDP restrictions; v8 TSYNC; v6 signal/abstract-socket scopes | v4: kernel 6.7 (Jan 2024); v10: 2025/26 kernels | go-landlock v0.10.0 exposes the full ladder; negotiate down to v4 floor for the D-04 network deny |
| `go-landlock` v0.9-era API | v0.10.0 presets V1–V10, `RestrictNet`, UDP rules | Aug 24 2026 | REQUIREMENTS' pin is the current release, not a stale one |
| creack/pty v1 | v2 major exists | v2 published after v1.1.24 | Keep the PAR-09-pinned v1.1.24 import path; do not chase v2 this phase |
| TaskOutput-style blocking retrieval | CC deprecates TaskOutput "in favor of `Read` on the task's output file path" | CC v2.1.x | ass-guard keeps TaskOutput working (it exists and is tested) and makes the output file the primary retrieval path (D-02 pointer) [CITED: code.claude.com/docs/en/tools-reference] |
| sandbox-exec presumed dying | still shipped + used (Claude Code, Codex, Apple containerization) | verified live Aug 2026 | Safe to build on with a loud degrade if it ever disappears (probe at first use) |
| bubblewrap as Linux default | landlock (REQUIREMENTS out-of-scope table: Ubuntu 24.04+ AppArmor userns clamp breaks unprivileged bwrap) | 2024+ | SAND-01 letter honored; bwrap stays opt-in stronger-isolation, not this phase's focus |

**Deprecated/outdated:**
- TaskOutput blocking-wait pattern (CC-side): output-file Read is the parity target.
- `go.lsp.dev/jsonrpc2`-era advice: irrelevant here (hand-rolled framing stands, per AGENTS.md stack).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | CC's exact `<task-notification>` wire payload/kind vocabulary (community-sourced: completed/failed observed; official docs don't document the block) | Standard Stack / Pattern 1 | LOW — D-02 locks ass-guard's own payload; parity is behavioral (wake + re-invoke), not byte-exact |
| A2 | The re-exec-self sentinel pattern for child confinement is the right Go shape (kernel inheritance verified; go-landlock docs do not document a child-confinement recipe) | Pattern 4 | MEDIUM — if rejected, alternative is a tiny internal helper binary; more surface, same semantics |
| A3 | ANSI-strip regex scope (CSI/SGR class `\x1b[` params finals) is sufficient for PTY capture | Standard Stack / Pitfall 4 | LOW — discretion item; table tests pin the chosen scope |
| A4 | `runtime.LockOSThread` around `cmd.Start()` mitigates the Pdeathsig thread-lifetime race | Pitfall 3 | MEDIUM — residual risk documented; worst case premature child kill, caught by tests |
| A5 | Sentinel-echo completion detection for the PTY shell (standard practice; CC's internal mechanism undocumented) | Pattern 5 | LOW — alternative (window-title/prompt sniffing) is strictly worse |
| A6 | darwin PTY master returns EOF (vs linux EIO) on child exit | Pattern 5 | LOW — both branches handled; tests run on the darwin host |
| A7 | `.sb` template needs only targeted denies over `(allow default)` for v1 (no deny-default) | Pattern 4 | MEDIUM — seatbelt rule semantics are undocumented by Apple; live tests on the dev host pin the profile actually used |
| A8 | ro-system path set (e.g. /usr, /bin, /System) suffices for typical tool children | Pattern 4 / Pitfall 1 | MEDIUM — discretion item; degrade-loudly covers gaps; may need per-distro tuning in verify phase |

## Open Questions

1. **Bash `persistent` argument vs the closed captured schema** (`additionalProperties: false`, no persistent property [VERIFIED: coretools.json:168-196])
   - What we know: D-09 locks per-call opt-in via a Bash tool argument; the 08-05 discipline pins captured schemas byte-identical; the project pivoted off the mimicry bar 2026-08-25.
   - What's unclear: does the operator sanction an additive schema extension (new optional property + description note), and does the consistency test suite need a waiver?
   - Recommendation: additive extension with a documented waiver + description text in CC's own voice; raise in discuss/plan checkpoint for operator sign-off.
2. **`dangerouslyDisableSandbox` real semantics under SAND-01**
   - What we know: captured description promises "dangerously override sandbox mode and run commands without sandboxing" [VERIFIED: coretools.json:187-190]; bash.go documents the current no-op as deliberate.
   - What's unclear: honor it per-call (unconfined run, loud stderr note) or keep the no-op?
   - Recommendation: honor it (it is the captured contract; CC parity) with a loud note + counter. Flag in plan.
3. **Background subagents hitting ask-class tools**
   - What we know: 17-D-07 declines asks on automation turns (no human) — a wake-turn is the same shape; a background subagent may also outlive the session that spawned it.
   - What's unclear: should background subagents inherit the decline-with-note rule (17-D-07) wholesale?
   - Recommendation: yes — parity with automation turns; document in plan.
4. **Sweep semantics for stale logs**
   - What we know: PAR-08 letter requires a startup stale-log sweep; logs live under `.ass-guard/outputs/`.
   - What's unclear: mark-stale vs GC; retention window.
   - Recommendation: mark `.stale` (rename) + count, delete on next sweep past a window — D-20 audit spirit (tombstone, never silent rm).
5. **Whether queued background subagents survive session close**
   - What we know: D-10 queues over-cap dispatches; 18-D-12 close = cancel-and-drain.
   - Recommendation: queued-but-unstarted tasks cancel silently at close (nothing started, nothing to kill); note in the close summary.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | everything | ✓ | go1.26.5 darwin/amd64 (toolchain pinned 1.26 in .mise.toml) | — |
| `/usr/bin/sandbox-exec` | macOS sandbox leg | ✓ | macOS 26.5.1 live-verified (denies enforced) | loud degrade (probe at first use) — feature is probe-and-degrade by design |
| Linux kernel Landlock (≥5.13; ≥6.7 for TCP v4) | Linux sandbox leg | ✗ on this dev host (darwin) | — | probe-and-degrade loudly IS the feature; linux CI/host needed for runtime verification |
| Docker (Linux runtime tests on darwin host) | landlock runtime tests | ✗ (docker not installed) | — | build-tagged tests: `GOOS=linux CGO_ENABLED=0 go build/vet` compile gate here; runtime tests need a Linux machine/CI |
| `mise` | ci gate | ✓ (repo standard; `.mise.toml` present) | — | raw `go test`/`golangci-lint` commands |

**Missing dependencies with no fallback:** none blocking — landlock runtime verification needs a Linux environment (CI or operator host); the compile gate works locally.
**Missing dependencies with fallback:** Docker → build-tag compile gate + operator Linux host for runtime verification.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `stretchr/testify` v1.11.1 (in go.mod), `-race` always |
| Config file | `.mise.toml` (tasks), `golangci-lint` v2 config |
| Quick run command | `go test -race -count=1 ./internal/coreexec/ ./internal/runtime/ ./internal/session/ ./internal/tasks/ ./internal/sandbox/` |
| Full suite command | `mise ci` (vet + lint + CGO_ENABLED=0 build + `go test -race -count=1 ./...`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PAR-07 | background Agent dispatch returns discriminated `async_launched` immediately; foreground path unchanged | unit (fake provider captures request; assert tool-result JSON status field) | `go test -race -count=1 ./internal/session/ -run 'TestDispatchBackground' -x` | ❌ Wave 0 |
| PAR-07 | notification arrives on completion, detected by kind (struct field), payload = D-02 fields | unit | `go test -race -count=1 ./internal/tasks/ -run 'TestNotification' -x` | ❌ Wave 0 (new pkg) |
| PAR-07 | output retrievable for running tasks (mid-run Read of output file non-empty) | unit | `go test -race -count=1 ./internal/tasks/ -run 'TestOutputRetrieval' -x` | ❌ Wave 0 |
| PAR-07 | cancellation kills nested loop cleanly; TaskStop works on subagent tasks | unit | `go test -race -count=1 ./internal/tasks/ -run 'TestCancel' -x` | ❌ Wave 0 |
| PAR-07 | wake-turn fires through automation rails on completion; coalesces (D-03); queued while client turn active (D-01 fallback) | unit/integration (runner battery, fake clock + TryLock) | `go test -race -count=1 ./internal/runtime/ -run 'TestWakeTurn' -x` | ❌ Wave 0 |
| PAR-08 | completion of background Bash enqueues notification on the SAME subsystem | unit | `go test -race -count=1 ./internal/coreexec/ -run 'TestBackgroundNotify' -x` | ❌ Wave 0 |
| PAR-08 | TERM-before-KILL ladder (TERM observed, then KILL after grace); group semantics kept | unit (real `sh -c 'trap' sleep` children) | `go test -race -count=1 ./internal/coreexec/ -run 'TestEscalation' -x` | ❌ Wave 0 |
| PAR-08 | Pdeathsig set on linux (build-tagged; compile-gated) | unit, `//go:build linux` | `GOOS=linux CGO_ENABLED=0 go vet ./internal/coreexec/ ./internal/sandbox/` | ❌ Wave 0 |
| PAR-08 | over-cap QUEUES with note (rewrites T-12-06-01 cap tests) + `background.bash` cap from config (D-11/D-12) | unit | `go test -race -count=1 ./internal/coreexec/ ./internal/acpserve/ -run 'TestBackgroundCap|TestConfigMenu' -x` | ⚠️ rewrite existing background_test.go cap cases (Wave 0) |
| PAR-08 | startup stale-log sweep marks orphans; live logs untouched | unit | `go test -race -count=1 ./internal/runtime/ -run 'TestStaleSweep' -x` | ❌ Wave 0 |
| PAR-09 | PTY shell: cd/export persist across calls in ONE pty; concurrent calls serialize | unit (real `sh` under pty, darwin+linux) | `go test -race -count=1 ./internal/coreexec/ -run 'TestPersistentShell' -x` | ❌ Wave 0 |
| PAR-09 | EIO-as-EOF; dead shell lazy-restarts with visible note (D-08) | unit | `go test -race -count=1 ./internal/coreexec/ -run 'TestPTYDeadRestart' -x` | ❌ Wave 0 |
| PAR-09 | ANSI strip (table: CSI/SGR/OSC cases) | unit (pure) | `go test -race -count=1 ./internal/coreexec/ -run 'TestAnsiStrip' -x` | ❌ Wave 0 |
| PAR-09 | master fd closed on session close (fd-count or Close-assert) | unit | `go test -race -count=1 ./internal/runtime/ -run 'TestPTYDrain' -x` | ❌ Wave 0 |
| SAND-01 | one policy struct → both backends produce symmetric denials (golden policy → landlock rules list / rendered .sb) | unit (pure, both OSes) | `go test -race -count=1 ./internal/sandbox/ -run 'TestPolicySymmetry' -x` | ❌ Wave 0 (new pkg) |
| SAND-01 | darwin: sandbox-exec with rendered profile denies network+write (live child probe test, `//go:build darwin`) | integration | `go test -race -count=1 ./internal/sandbox/ -run 'TestSeatbelt' -x` | ❌ Wave 0 (runs on this host) |
| SAND-01 | linux: probe returns loud-degrade on ENOSYS/EOPNOTSUPP paths (fake-probed); ruleset build compiles cgo-free | unit + compile gate, `//go:build linux` | `GOOS=linux CGO_ENABLED=0 go build ./... && go test -race -count=1 ./internal/sandbox/ -run 'TestLandlockDegrade' -x` | ❌ Wave 0 |
| SAND-01 | default OFF; `--sandbox=off` always escapes; flag plumbs to executors | unit | `go test -race -count=1 ./internal/acpserve/ ./internal/runtime/ -run 'TestSandboxFlag' -x` | ❌ Wave 0 |
| PAR-07/20-seam | background dispatch resolves model per 20-03 contract (frontmatter > dispatch > session > tier; degrade+one warning) | unit (extends TestDispatchModel battery) | `go test -race -count=1 ./internal/runtime/ -run 'TestDispatchModel' -x` | ⚠️ arrives with 20-03; precondition-mark |

### Sampling Rate
- **Per task commit:** quick run command (packages touched by this phase)
- **Per wave merge:** `mise ci` full suite
- **Phase gate:** full suite green + `GOOS=linux CGO_ENABLED=0 go build ./...` + `go vet` (compile gate for linux-tagged files) before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/tasks/` package skeleton + `tracker_test.go`, `notify_test.go` — covers PAR-07/PR-08 notification half
- [ ] `internal/sandbox/` package skeleton + `policy_test.go` — covers SAND-01 symmetry
- [ ] `internal/coreexec/ptty_test.go` + `ansistrip_test.go` — covers PAR-09 (darwin host executes PTY tests)
- [ ] `//go:build linux` files + `GOOS=linux` compile gate wired into the phase verification (runtime landlock/Pdeathsig tests stay linux-host-gated; skipped-by-tag on darwin)
- [ ] Rewrite `internal/coreexec/background_test.go` cap cases to the D-11 queue contract

*(Existing infrastructure — TaskRegistry tests, cron/automation tests, config_surface tests — covers the surrounding rails.)*

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V1 Safe Rendering / sandbox boundary | yes | OS sandbox (landlock / sandbox-exec) applied to children only; D-05 honesty note |
| V2 Authentication | no | — (no new auth surface; provider creds untouched) |
| V3 Session Management | no | — (session lifecycle rides Phase 18) |
| V4 Access Control | yes | FS scope: rw on workdir+tmp+`.ass-guard`, ro system, network deny (D-04); caps D-10/D-11 bound resource abuse |
| V5 Input Validation | yes | tool-arg parsing (existing best-effort discipline); profile path substitution is operator/session-controlled, never model-controlled |
| V6 Cryptography | no | — (task ids reuse crypto/rand hex idiom from 12-06 [VERIFIED: background.go:107-116]) |
| V14 Configuration | yes | `--sandbox` default OFF (SAND-01); configOptions caps operator-tunable (D-12) |

### Known Threat Patterns for Go process lifecycle + OS sandboxing

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| curl-style exfil from a confined tool | Information Disclosure | D-04: landlock `RestrictNet()` (v4 TCP deny-all) / sandbox-exec `(deny network*)` — both live-verified this session |
| Silent fail-open sandbox (BestEffort false confidence) | Elevation of Privilege | strict-mode probe at startup + loud degrade + per-run unconfined note (Pitfall 1) |
| Privilege regain via SUID inside sandbox | Elevation of Privilege | `PR_SET_NO_NEW_PRIVS` — go-landlock sets it with every Restrict* call [CITED: pkg.go.dev go-landlock; docs.kernel.org: enforcement requires no_new_privs or CAP_SYS_ADMIN] |
| Orphan process DoS across kill -9 | Denial of Service | Pdeathsig (linux), group TERM→KILL ladder, ReapAll on close, startup stale sweep (PAR-08) |
| Fork-bomb via background caps | Denial of Service | D-10/D-11 caps (8/16) + FIFO queue; existing per-session registry scoping |
| Model-authored command executes anything | (accepted) | T-8-33 posture unchanged: the model's command IS the product; sandbox + caps are the new bounded blast radius, not a confirmation tier |
| PTY output leaking secrets into transcript | Information Disclosure | notification tail + PTY capture ride the REDACTED transcript path (D-23 type-scoping family); never stdout |
| Symlink/rename escape inside workdir (REFER) | Tampering | `RWDirs` excludes refer rights; `WithRefer()` only with ABI≥2 — leave OFF for v1 (deny-by-default on cross-dir rename) |

## Sources

### Primary (HIGH confidence)
- Read this session: `internal/coreexec/bash.go`, `internal/coreexec/background.go`, `internal/coreexec/register.go`, `internal/session/subagent.go`, `internal/session/session.go` (dispatch site), `internal/session/transcript.go`, `internal/session/manager.go` (Append* signatures), `internal/runtime/cron_wiring.go`, `internal/runtime/runtime.go` (sessionFor/OnClose/runOneTurn), `internal/acpserve/acp_serve.go`, `internal/acpserve/config_surface.go`, `internal/event/events.go`, `internal/toolcat/coretools.json`, `go.mod`, `.mise.toml`, `.planning/phases/20-.../20-03-PLAN.md`, ROADMAP Phase 22
- Live verification (this session): `sandbox-exec` network + write denies on macOS 26.5.1; `GOOS=linux CGO_ENABLED=0 go build` of go-landlock v0.10.0; `go doc syscall.SysProcAttr` Pdeathsig text; `go list -m` versions for both packages
- pkg.go.dev/github.com/landlock-lsm/go-landlock/landlock — v0.10.0 API (RestrictPaths/RestrictNet/V1–V10/BestEffort caveats/MPTCP caveat)
- pkg.go.dev/github.com/creack/pty — v1.1.24 API (StartWithAttrs/Winsize/Setsize), MIT, platform files
- docs.kernel.org/userspace-api/landlock.html — ABI v1–v10 table, probe syscall + error taxonomy, per-thread/inheritance/irreversibility, no_new_privs
- code.claude.com/docs/en/tools-reference — run_in_background, TaskOutput deprecation ("in favor of `Read` on the task's output file path"), TaskStop, background subagents

### Secondary (MEDIUM confidence)
- code.claude.com docs + community reports (Reddit r/ClaudeCode, GitHub issues #21343/#21048/#18544) — `<task-notification>` harness re-invoke behavior, kinds completed/failed
- github.com/apple/containerization#737 + HN thread — sandbox-exec deprecation-vs-reality status
- danmackinlay.name / korben.info — `sandbox-exec -p` inline profile syntax (cross-checked with live test)

### Tertiary (LOW confidence)
- None outstanding — community-only claims are confined to A1 (CC's internal notification block), which D-02 supersedes for ass-guard.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — both libraries version-pinned, source-verified, compile/live-verified this session
- Architecture: HIGH — every seam named in CONTEXT was read in source this session; patterns map onto existing, tested machinery
- Pitfalls: HIGH for verified gotchas (Pdeathsig doc text, BestEffort caveat, MPTCP caveat, EIO); MEDIUM for Pdeathsig mitigation efficacy (A4) and seatbelt profile scope (A7/A8)

**Research date:** 2026-08-28
**Valid until:** 2026-09-27 (stable libraries; landlock/seatbelt platform facts re-check if macOS/kernel majors ship mid-phase — ROADMAP research flag for Phase 22)
