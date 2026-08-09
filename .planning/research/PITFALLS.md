# Pitfalls Research

**Domain:** Go-based SDD-hosting AI coding agent with mimicry (outgoing request shape), multi-tier model scheduling, hook-DAG, learning mode, ACP + Telegram interfaces, and MCP hosting
**Researched:** 2026-08-09
**Confidence:** HIGH (inherited pitfalls grounded in predecessor's closed research; new-scope pitfalls grounded in current external sources where noted, project-specific reasoning where not)

This document is **prioritized for ass-guard's specific success criterion: mimicry is the north star.** New-scope pitfalls that compromise mimicry rank highest; pitfalls that merely compromise downstream features rank lower. Inherited pitfalls from `sdd-acp-agent` are summarized briefly (they are already researched in depth at `/Users/nil/DiskD/W/Djarvur/sdd-acp-agent/_bmad-output/planning-artifacts/research/technical-sdd-acp-agent-research-2026-07-02.md`) and separated from the new-scope deep dives.

Roadmap phases referenced below (matching PROJECT.md ordering):
- **Phase M** — Mimicry profile (zcode): profile extraction, request-shape parity, tool catalog fidelity
- **Phase S** — Model scheduling: tiers, time-windows, fallback chains, per-project overrides
- **Phase H** — Unified engine + Hook-DAG: post-turn decision engine, seeded hooks, learning mode
- **Phase I** — Interfaces: ACP primary, Telegram peer (text + voice)
- **Phase C** — MCP hosting + Claude-Code-compat drop-in
- **Phase A** — Audit log + distribution + greenfield discipline (cross-cutting)

---

## Critical Pitfalls — NEW SCOPE (deep dives, priority-ordered)

### Pitfall N1: Profile drift — target agent (zcode) silently updates after capture, profile goes stale

**Severity: CRITICAL (north-star killer)**

**What goes wrong:**
zcode ships a new build that changes its system prompt, renames a tool, alters a tool-call schema, adds a new metadata field to its outgoing requests, or reorders the tool catalog. The ass-guard profile was extracted from logs captured at time T0; at time T1 the profile no longer matches what the live zcode actually sends. ass-guard keeps emitting T0-shaped requests, the model behaves as it would for the old zcode, and mimicry silently degrades. There is no error — just behavioral divergence from the current zcode, which is the whole point.

**Why it happens:**
Profiles are static config derived from a one-time log extraction. There is no automatic mechanism to re-check whether the profile still matches reality, and zcode updates are out of ass-guard's control. The team treats "profile extracted once" as "profile validated forever."

**How to avoid:**
- Treat the profile as a **versioned artifact** with three fields: `{profile_version, target_capture_ref (zcode build hash / version string), captured_at}`. Refuse to load a profile that lacks a capture ref.
- Ship a **profile-drift detector** as a Phase M tool: given a profile and a fresh zcode log (re-capture on demand or via a watch on the zcode log directory), structurally diff the two and report drift in (system prompt hash, tool catalog names+schemas, identity fields, message-shape skeleton). Run this detector in CI on every profile update and surface it as a CLI command (`ass-guard profile check zcode`).
- Pin a **zcode version range** per profile (`target_capture_ref` field). On startup, if the installed zcode version is outside the validated range, log a loud stderr warning. Never silently proceed.
- Distinguish **cosmetic drift** (field ordering, optional fields absent) from **structural drift** (renamed tool, new required field, changed system-prompt skeleton). Cosmetic drift is acceptable per PROJECT.md's "structurally indistinguishable, not byte-identical" decision; structural drift is a hard fail.

**Warning signs:**
- Model behavior in ass-guard diverges from behavior in live zcode on the same prompt (the mimicry A/B test fails) — but only after a zcode update
- Profile's `target_capture_ref` is null or older than 30 days
- A new zcode tool appears in zcode's UI but is absent from ass-guard's catalog
- System prompt hash in the profile ≠ hash of zcode's current `AGENTS.md` + system-prompt sources

**Phase to address:** Phase M (build the drift detector alongside the extractor; the detector is not optional)

---

### Pitfall N2: Over-investing in byte-identical mimicry past the point of model indifference

**Severity: HIGH (wastes the scarce resource — team attention — on the wrong axis)**

**What goes wrong:**
The team chases binary-diff equality between ass-guard's outgoing requests and zcode's: exact field ordering in JSON, exact whitespace, exact serialization of optional fields, exact header capitalization, exact timestamp formats. This is explicitly **out of scope** per PROJECT.md ("structurally indistinguishable to the model" is the bar, not binary diff equality). The team spends weeks on cosmetic parity that has zero effect on model behavior, while structural drift (N1) goes undetected.

**Why it happens:**
Byte-identical is **measurable and satisfying** — `diff` gives a clean yes/no. Structural-indistinguishability is fuzzy and requires an empirical A/B test (does the model behave the same?). Engineers gravitate toward the measurable axis even when it's the wrong one. The predecessor's "Claude Code compatibility" framing also primes this — compat feels like it should mean identical.

**How to avoid:**
- Define **mimicry parity** as an empirical property, not a textual one: a fixed prompt suite, run through both ass-guard (with zcode profile) and live zcode, must produce statistically indistinguishable tool-call sequences (same tools called in the same order with the same arguments, within a tolerance band). This is the acceptance test for Phase M.
- Maintain a **parity impact triage** for every diff the drift detector surfaces: cosmetic (no action), structural-low (track), structural-high (block release). Resist fixing cosmetic diffs unless they're trivial.
- Normalize the **canonical-vs-wire** distinction in the codebase: store the profile as a canonical structure; serialize to wire format using the target's documented serialization rules. Don't try to make the canonical form byte-match — make the *serialization* match the target's serialization rules.
- Make PROJECT.md's "byte-identical is out of scope" decision a load-bearing comment at the top of the profile-serialization module.

**Warning signs:**
- PRs spend more lines on JSON field ordering than on tool-schema fidelity
- The parity test suite is textual-diff-based rather than behavioral
- A "mimicry 100%" green check relies on `diff` returning empty
- Team debates whitespace handling in serialized requests

**Phase to address:** Phase M (set the bar empirically before any other mimicry work)

---

### Pitfall N3: Incomplete capture — profile built from logs that don't represent zcode's full request surface

**Severity: HIGH (north-star compromise, harder to detect than N1)**

**What goes wrong:**
The log-extraction approach (PROJECT.md decision: "Profile content is log-extracted, not hand-written") is only as good as the logs. If the captured logs don't exercise every code path — every tool, every system-prompt variant, every conditional field — the profile silently omits them. The first time ass-guard encounters an unexercised path, it emits a request the model has never seen from "zcode," and mimicry breaks only on that path. Worse, the team doesn't know which paths are missing because the logs looked comprehensive.

**Why it happens:**
Log capture is opportunistic — you log what you happen to do. Rare tools (e.g. a specific MCP integration used once a week), conditional system-prompt sections (e.g. project-type-specific instructions), and edge-case message shapes (e.g. multi-turn tool chains, error-recovery paths) are under-represented in any single capture session. The extractor reports "tool catalog extracted: 18 tools" without saying "zcode actually ships 24 tools, your logs only exercised 18."

**How to avoid:**
- The extractor must produce a **coverage manifest** alongside the profile: for each section (system prompt, each tool's schema, each identity field, each message-shape variant), report `{observed: bool, observation_count: N, source_log_entries: [...]}`. A profile ships with this manifest; coverage <100% is a documented known-gap, not a silent hole.
- Cross-check the extracted tool catalog against **a non-log source**: zcode's own on-disk tool-definitions / plugin manifests / `AGENTS.md` (whichever is the source of truth for zcode's catalog). Any tool present in zcode's declared catalog but absent from logs is flagged "declared but unobserved" — and either gets a schema from the declaration or a TODO.
- Run a **capture-completeness script** that exercises every declared tool against a scratch project and logs the result; this becomes the canonical capture corpus rather than ad-hoc logs. Re-run when zcode updates.
- Mark sections with low observation count (`N < 3`) as "low-confidence" in the profile; treat them as candidates for re-capture.

**Warning signs:**
- Profile's coverage manifest shows tools with `observation_count: 1`
- A tool declared in zcode's config is absent from the profile
- Mimicry A/B test passes on common paths but fails the first time an uncommon tool is invoked
- The capture corpus is a single session's log file

**Phase to address:** Phase M (extractor must emit coverage manifest from day one; don't bolt it on)

---

### Pitfall N4: Tool-catalog drift between ass-guard's built-in catalog and the active profile

**Severity: HIGH (north-star compromise — the model calls a tool expecting one shape, ass-guard executes another)**

**What goes wrong:**
ass-guard ships its own built-in tool catalog (the claude-code-compat baseline). A profile (e.g. zcode) declares its own tool catalog with names and schemas. If the profile's declared tool `X` has schema v2 but ass-guard's built-in implementation of `X` expects schema v1, the model — believing it's talking to zcode — calls `X` with v2 arguments, and ass-guard's executor either fails, silently drops fields, or behaves differently from how zcode would. To the model this looks like zcode malfunctioning; to the user it looks like ass-guard malfunctioning; either way mimicry is broken.

**Why it happens:**
Two sources of truth (built-in implementation vs. profile declaration) drift independently. The built-in catalog evolves as ass-guard adds features; the profile is pinned to a capture date. There's no compile-time check that they agree.

**How to avoid:**
- Make the **profile's declared catalog the authoritative schema** at runtime: the model sees the profile's schemas; the executor dispatches by name to the built-in implementation; a **schema-adapter layer** translates between profile-declared schema and built-in-implementation schema. Drift is absorbed by the adapter, not exposed to the model.
- Add a **catalog-consistency check** in CI: for every tool in every shipped profile, verify the built-in implementation can satisfy the profile-declared schema (every required field is consumed; no field is silently ignored). Fail the build on inconsistency.
- When a tool's schema genuinely cannot be satisfied by the built-in implementation (e.g. the profile declares a field the implementation doesn't understand), surface this as a **profile-to-implementation gap** with an explicit TODO and a degraded-behavior log line at runtime — never silent.
- Version the built-in tool implementations (`tool_impl_version`) and record the tested-against profile version per tool.

**Warning signs:**
- Model calls a tool with arguments that include a field ass-guard's logs show it ignored
- CI doesn't have a catalog-consistency step
- A profile declares a tool with required fields that the built-in implementation doesn't read
- Tool-result shape sent back to the model differs from what zcode would send for the same tool

**Phase to address:** Phase M (catalog adapter is part of the mimicry MVP, not a follow-on)

---

### Pitfall N5: Fallback-chain cascading failure — one provider error triggers a chain that ends at the most expensive model

**Severity: HIGH (cost + reliability — directly undermines the scheduling delta)**

**What goes wrong:**
A fallback chain is configured (e.g. heavy → good → light, or provider-A → provider-B → provider-C). When the primary fails (rate limit, 5xx, timeout), the chain walks. But: (a) the failure is correlated (a region-wide outage hits A and B), so the chain burns time and retry budget on already-failing providers before landing on C; (b) the chain terminates at the most expensive option, so a transient blip on the cheap tier causes a permanent cost spike for the duration; (c) the chain and the time-window scheduler interact badly — fallback escapes the time-window's intent (peak-hours config says "use cheap model," but fallback walks to expensive model during peak hours anyway).

**Why it happens:**
Fallback chains are designed for **independent** failures (one provider flaky, others fine). Real failures are often correlated (upstream model outage, network partition, shared rate-limit quota across an org). And fallback chains compose badly with cost-aware scheduling because they're specified as availability mechanisms, not cost-bounded mechanisms.

**How to avoid:**
- Distinguish **transient** (429, 503, timeout — retry same tier with backoff) from **structural** (401, 404, schema error — fall through) failures. Don't fall through the chain on transient failures; back off and retry the same tier first. (Anthropic's docs: RPM/ITPM/OTPM rate limits plus a rolling weekly quota make 429s common and bursty — a retry-with-backoff absorbs most of them without chain-walking.)
- Bound the chain's **cost escalation**: each step records a cost ceiling; if falling through would exceed the per-turn cost budget, halt and surface a user-visible error rather than silently landing on the expensive model.
- Make the time-window scheduler **fallback-aware**: a fallback that escapes the active time-window's intended tier must either (a) ask the user, (b) wait for the window to rotate, or (c) proceed with a loud log — never silently. Default to "wait + log" during peak hours.
- Add **circuit breakers** per provider: N consecutive failures in a window marks the provider as unhealthy and short-circuits the chain to the next-healthy provider, skipping the retries on the known-bad one. Reset after a cooldown.
- Test the chain under **correlated failure** in CI (mock all providers returning 503 simultaneously) — assert the behavior is "fail fast with a clear error," not "burn 90 seconds of retries then land on the expensive model."

**Warning signs:**
- Cost spikes correlate with provider incidents
- A single 429 causes a 30+ second stall before the turn completes
- Per-turn cost variance is high under load (chain walking makes cost unpredictable)
- Fallback lands on the most expensive model during configured peak hours

**Phase to address:** Phase S (fallback semantics are part of the scheduling MVP, not a tuning knob added later)

---

### Pitfall N6: Time-window boundary bugs — timezone, DST, and "boundary minute" scheduling

**Severity: MEDIUM-HIGH (silent misrouting, hard to reproduce)**

**What goes wrong:**
The time-window scheduler routes turns to different models based on wall-clock time. Bugs cluster at boundaries: (a) timezone confusion — the config says "peak hours 9-17" but doesn't say which timezone; the server runs in UTC, the user expects local; (b) DST transitions — a window defined as "17:00-23:00" loses or gains an hour twice a year; (c) the boundary minute itself — a turn starting at 16:59:59 with the peak-hours config switching at 17:00:00: which model runs?; (d) per-project override going stale — the project override was set in winter, the team's working hours shifted, no one updated the override.

**Why it happens:**
Wall-clock time is deceptively simple until you have to pin it to a timezone and reason about boundaries. Go's `time` package is correct but unforgiving — `time.Now()` returns local time, parsing without a location defaults to UTC, and DST is handled per-location. Config authors reach for "17:00" without specifying a zone because it feels obvious.

**How to avoid:**
- Mandate **timezone-explicit** config: every time-window must specify an IANA timezone (`America/New_York`, not `EST` — `EST` ignores DST). Reject config that omits it; default to `UTC` only with a loud stderr warning.
- Document the **boundary semantics** once and test them: a turn uses the model in effect at turn-start; mid-turn time-window transitions do not switch models. Assert this in a test that starts a turn at 16:59:59.999 with a 17:00:00 transition.
- Load time-windows via Go's `time.LoadLocation` and fail loudly if the zone database isn't available (common on slim Docker images — `tzdata` not installed). The binary should either bundle `tzdata` (`import _ "time/tzdata"`) or refuse to start without it.
- For per-project overrides, record a **last-reviewed timestamp** and surface a stderr nag if it's older than 90 days. The override isn't "set once"; it's "set and revisited."
- Add a **dry-run scheduler CLI** (`ass-guard schedule show --at 2026-08-09T17:01:00-04:00`) that prints which tier/model a turn at that instant would use. This makes boundary behavior inspectable without running a turn.

**Warning signs:**
- Config uses bare hour ranges without a timezone
- Bug reports that say "the wrong model ran" cluster around DST transitions
- The `tzdata` package isn't in the binary's build tags
- No test exercises a turn starting within 1 second of a window boundary
- Per-project override files have no `last_reviewed` field

**Phase to address:** Phase S

---

### Pitfall N7: Tier mismatch across providers — "heavy" on provider X is not "heavy" on provider Y

**Severity: MEDIUM-HIGH (silent capability degradation when fallback walks providers)**

**What goes wrong:**
The tier abstraction (heavy/good/light) is meant to decouple capability from concrete model. But capability is provider-relative: provider-X's "heavy" model (say, glm-5.2-analog) and provider-Y's "heavy" model (say, some other vendor's flagship) differ in context-window size, tool-calling reliability, instruction-following, and output quality. A fallback chain that walks from X-heavy to Y-heavy preserves the *tier label* but not the *capability*. A turn that needed X-heavy's specific strengths gets Y-heavy, which is technically "heavy" but practically insufficient. The scheduling layer reports success; the user sees worse results.

**Why it happens:**
"Tier" is a useful lie — it abstracts away exactly the differences that matter at the edges. Teams adopt the abstraction and then treat tier-equality as capability-equality, which holds in the middle of the capability distribution and breaks at the edges (especially for novel tasks, long-context tasks, or complex tool chains).

**How to avoid:**
- **Don't pretend tier is capability-portable across providers.** Document per-(provider, tier) the qualitative capability profile (context window, tool-call reliability, known weaknesses). The fallback chain can walk providers within a tier, but the capability profile is visible to the operator.
- Allow **tier-pin in the prompt**: a command/skill/subagent can declare "this turn requires X-heavy specifically" (not just "heavy"), bypassing cross-provider fallback for that turn. Use sparingly — only when the turn genuinely depends on a capability the cross-provider substitute lacks.
- Track **per-(provider, tier) success telemetry** (did the turn complete without retry, did the model call the expected tools). A provider-tier with degraded observed capability gets flagged in the operator's view.
- When walking a fallback chain across providers, **log the capability delta** ("falling back from X-heavy to Y-heavy; Y-heavy has 2x smaller context window"). Make the capability compromise visible, not silent.

**Warning signs:**
- Fallback across providers succeeds (no error) but produces worse results than the primary would have
- Telemetry shows one (provider, tier) combo has a much higher retry rate than others at the same tier
- No documentation of which concrete models back which tier on which provider
- Turns that depend on long context fail intermittently depending on which provider served them

**Phase to address:** Phase S

---

### Pitfall N8: Hook-DAG infinite loops — a hook's output matches the trigger for the next stage, which triggers the hook again

**Severity: HIGH (runaway agent, cost + state corruption)**

**What goes wrong:**
The unified engine fires after turn-complete (dual-signal: text-pattern OR known handoff tool-call). A hook (say, post-implement: run tests) emits output that itself matches the text-pattern for the next stage (say, "tests passing — proceed to apply"). The engine fires the next stage. That stage's hooks emit output that matches the implement-trigger pattern. The loop runs forever, burning model calls and mutating the project state, until the user notices and cancels — or until the rate limiter kicks in, at which point the fallback chain (N5) walks to the expensive model and the cost spike gets worse.

**Why it happens:**
Pattern matching is a leaky trigger. The patterns were authored to detect *user-toolkit handoff language*, but the same language can appear in hook output (hooks that summarize, hooks that report test results, hooks that propose next steps). The engine can't distinguish "toolkit said this" from "my own hook said this." The dual-signal design (pattern OR tool-call) makes it worse — either signal fires the engine, so even well-curated patterns are vulnerable to a stray tool-call from a hook.

**How to avoid:**
- **Tag every turn with its provenance**: `{user, model, hook, autocontinue}`. The engine only fires autocontinue/hooks on turns whose provenance is `user` or `model` (i.e. genuine user-driven or toolkit-driven turns), never on turns whose provenance is `hook` or `autocontinue` (engine-generated turns). This breaks the loop structurally.
- Maintain an **injection-depth counter** per scenario: a turn injected by the engine increments it; if depth exceeds a small bound (say, 5), halt and surface a "possible loop" warning. The counter resets on a genuine user turn.
- Make the **dual-signal require BOTH** signals in ambiguous cases, not OR — or, less conservatively, require the text-pattern to match on turns that did NOT contain a known handoff tool-call, and require a known handoff tool-call to match on turns that did. Keep them as independent triggers only when the toolkit's protocol is well-understood.
- Add a **loop-detector test fixture**: a recorded hook-output sequence that's known to match stage-trigger patterns; assert the engine does not loop on it. This is part of the engine's acceptance test.
- The pattern table must be **hook-output-aware**: when authoring a pattern, the author must check it doesn't match any of the seeded hooks' outputs. CI can verify this by running every pattern against every seeded hook's output corpus.

**Warning signs:**
- The same stage runs more than twice without user input
- Injection-depth counter hits its bound regularly in normal use (means the bound is too low OR there are real loops)
- A hook's output textually resembles toolkit handoff language
- Cost telemetry shows multi-turn "runs" that are longer than the toolkit's defined workflow length

**Phase to address:** Phase H (provenance tagging is part of the engine MVP; this cannot be a retrofit)

---

### Pitfall N9: Hook failures blocking the workflow — fail-loud when fail-soft was intended, or vice versa

**Severity: MEDIUM (workflow paralysis or silent corruption — opposite failure modes, same root cause)**

**What goes wrong:**
A hook in the DAG fails (command exits non-zero, prompt errors out, fresh-context spawn fails). Two opposite bad outcomes: (a) **fail-loud** — the whole SDD scenario halts because the post-implement lint hook returned non-zero on a warning; the user has to intervene on every minor issue, defeating the "hands-off" value proposition; (b) **fail-soft** — a critical hook (run tests before apply) silently fails, the workflow proceeds, and broken code gets applied. The same mechanism produces both bad outcomes depending on how failure semantics were (not) specified per hook.

**Why it happens:**
Default failure semantics for DAG steps are usually "halt on error" (safe for unknown steps) or "log and continue" (convenient for known-noisy steps), and either default is wrong for half the hooks. Without per-hook explicit failure policy, the DAG inherits whatever default the framework chose.

**How to avoid:**
- **Every hook declares its failure policy** explicitly: `{on-failure: halt | continue | ask}`. No default — config validation rejects a hook without it. This forces the author to think about it once, at authoring time.
- Map the policies to user intent: `halt` for "this hook's success is a precondition for proceeding" (tests before apply); `continue` for "this hook is advisory" (lint warnings, memory updates); `ask` for "this hook's failure is ambiguous and the user should decide."
- The **seeded hook set** (post-implement: test+lint+review+memory; post-phase: improvement proposals) ships with curated failure policies. Tests = halt (broken tests block apply). Lint = continue (advisory). Review = ask (the user should see what review found). Memory = continue (don't block on memory failure). Improvement = continue.
- Log **every hook failure with its policy decision** in the audit log, so post-hoc analysis can tell whether a workflow halted-by-design or halted-by-accident.
- Provide a **hook-failure dashboard** (even just stderr-aggregated) so the user sees accumulation of `continue`-policy failures they might otherwise miss.

**Warning signs:**
- The same hook failure halts the workflow repeatedly and the user works around it manually each time
- A `continue`-policy hook fails silently and the downstream stage produces broken output
- Hooks lack an explicit `on-failure` field
- No audit-log entry distinguishes "halted by hook policy" from "halted by user"

**Phase to address:** Phase H

---

### Pitfall N10: Concurrent input from ACP and Telegram — race conditions and session state divergence

**Severity: MEDIUM-HIGH (silent state corruption, hard to reproduce)**

**What goes wrong:**
ass-guard has two full-peer interfaces (ACP and Telegram). A user starts an SDD scenario from ACP, then sends a Telegram message mid-turn ("actually skip the tests"). Two failure modes: (a) **race** — the Telegram message arrives while the ACP turn is mid-execution; the engine queues or interleaves it incorrectly; (b) **state divergence** — the ACP-visible transcript and the Telegram-visible conversation drift apart; the user sees one state in the IDE and another on their phone, and edits based on stale state.

**Why it happens:**
Two interfaces to one core is the design (PROJECT.md: "Both are full-capability fronts to one core"). But "one core" must mean one **serialized session state**, not two loosely-coupled ones. Without explicit serialization, each interface's request handler races against the other for the shared session.

**How to avoid:**
- **Serialize all input through a single session mailbox.** Both ACP and Telegram deliver messages to one channel; the turn loop drains it serially. No two inputs are ever processed concurrently for the same session.
- Define **turn preemption semantics** explicitly and document them: can a Telegram message interrupt an in-flight ACP turn? PROJECT.md should answer this (recommendation: no — queue the message for the next turn boundary; mutating toolkit commands are boundaries by design). Surface "your message will be applied after the current stage" to the Telegram user.
- Make the **durable transcript the single source of truth** for both interfaces. ACP reads it for replay; Telegram reads it for context. Neither interface maintains its own session state — both project from the transcript.
- Add a **session-ownership lock** with a TTL: when ACP starts a turn, it holds the lock; Telegram messages during the lock are queued, not dropped. The lock releases at turn boundary. If a turn hangs, the TTL expires and the lock releases with a warning.
- Test the **interleaving matrix** in CI: ACP-starts + Telegram-interrupts, Telegram-starts + ACP-interrupts, both-start-simultaneously. Assert the transcript ends in a consistent state in all three.

**Warning signs:**
- A user reports edits made via Telegram "didn't take" or appeared in the wrong place
- The ACP transcript and the Telegram conversation show different recent history for the same session
- No lock or mailbox sits between the interfaces and the turn loop
- Tests don't exercise concurrent input from both interfaces

**Phase to address:** Phase I (build the mailbox from the start; bolting serialization onto two existing interfaces is much harder)

---

### Pitfall N11: Long-running SDD turn blocks the other interface — responsiveness asymmetry

**Severity: MEDIUM (UX erosion, the secondary interface feels broken)**

**What goes wrong:**
An SDD scenario running on ACP takes 10 minutes (multi-stage, lots of hooks, lots of model calls). During those 10 minutes, Telegram is either (a) blocked — messages queue and don't get responses until the ACP scenario finishes; or (b) silently dropped. The user, on their phone, thinks Telegram is broken. The inverse happens too: a long Telegram-driven turn blocks ACP.

**Why it happens:**
If the session mailbox (N10) is serialized without fairness, a long turn starves the other interface. If it's not serialized at all, you get N10's races instead. The right answer is serialized-but-responsive, which requires explicit design.

**How to avoid:**
- **Distinguish "input" from "status."** Even while a turn is in flight and input is queued, both interfaces must serve status queries (what's running, how far along, ETA) immediately. Status reads from the transcript, which is always consistent.
- Provide a **cancel-via-either-interface** path: Telegram can cancel an ACP-started turn and vice versa. Cancellation drains queued injections (per PROJECT.md's safety model).
- Surface **turn-progress notifications** to the non-driving interface (Telegram gets a "stage 3 of 5 complete" notification; ACP gets nothing extra because it's already driving). This makes the blocking visible rather than mysterious.
- Set a **per-turn wall-clock budget** with a configurable ceiling; turns that exceed it surface a "this is taking unusually long" notification to both interfaces, not just the driving one.
- The serialized mailbox must have **fairness**: a long turn cannot hold the lock indefinitely; at turn boundaries (which happen often within a multi-stage scenario), pending input from the other interface gets a turn-bounded chance to interject.

**Warning signs:**
- Telegram users report the bot "freezes" during long ACP runs
- No status query is possible while a turn is in flight
- A multi-hour ACP run holds the session lock the entire time
- Cancellation only works from the interface that started the turn

**Phase to address:** Phase I

---

### Pitfall N12: Voice transcription errors compounding through the agent

**Severity: MEDIUM (quality erosion, hardest to detect)**

**What goes wrong:**
A voice message is transcribed to text via a configurable STT backend. The transcription is wrong in a way that changes meaning: "skip the tests" becomes "keep the tests," "apply to main" becomes "apply to mane," "open spec" (the toolkit name) becomes "open spice." The agent acts on the wrong instruction. Because the user spoke (not typed), they have no visual feedback of what the agent heard until the response comes back — and by then the wrong action may have been taken.

**Why it happens:**
STT is probabilistic; the agent treats its output as authoritative; the agent's downstream actions (tool calls, file edits) commit before the user can correct. Compounding: the model may further interpret the misheard phrase in a plausible-but-wrong direction.

**How to avoid:**
- **Echo the transcription back before acting** on voice input. Telegram gets a "I heard: 'skip the tests' — proceeding" message with a brief window (say, 3 seconds) to cancel or correct. This converts voice into a typed-confirmation surface.
- Run a **secondary confirmation pass for high-stakes actions** detected in voice input: any phrase matching a mutating-toolkit-command pattern (apply, implement, delete, merge) gets an explicit yes/no confirmation, even if typed input wouldn't.
- Pick the **STT backend's domain carefully**: a generic STT will mangle domain terms (OpenSpec, SDD, ACP, hook-DAG, model names). Either fine-tune, post-process with a domain-term dictionary, or pick a backend that handles technical vocabulary. Maintain a **domain-term dictionary** the STT post-processor uses to correct common confusions ("open spice" → "OpenSpec").
- Log the **raw transcription alongside the action taken**, so post-hoc audit can reveal "the user said X, STT heard Y, agent did Z." This is the only way to detect compounding errors at scale.
- Track **STT-confidence if the backend exposes it**; below a threshold, force the echo-back-then-confirm path even for low-stakes phrases.

**Warning signs:**
- User reports the agent "did the opposite of what I said" — classic STT-error signature
- No transcription echo appears before action on voice input
- Domain terms (toolkit names, model names) are frequently mis-transcribed
- The audit log doesn't record the raw transcription, only the post-correction text

**Phase to address:** Phase I

---

### Pitfall N13: MCP server subprocess zombies and crashes — unbounded resource accumulation

**Severity: MEDIUM-HIGH (resource leak → process death → agent down)**

**What goes wrong:**
ass-guard hosts MCP servers as subprocesses (stdio JSON-RPC, mirroring the ACP transport). Failure modes: (a) ass-guard crashes or is killed (Ctrl-C from the IDE) without sending `SIGTERM` to spawned MCP children — they orphan, reparent to init/PID-1, and keep running; over days, dozens accumulate, eating RAM (a documented real-world case: ~14GB consumed by orphaned Claude Code MCP servers and headless Chrome processes on a single laptop). (b) An MCP server crashes mid-turn; ass-guard's tool call hangs or errors confusingly. (c) An MCP server's stdio pipe closes but the process lingers — partial zombie. (d) In a container, ass-guard-as-PID-1 doesn't reap children, and zombies accumulate in the process table.

**Why it happens:**
Subprocess lifecycle is easy to start and hard to clean up. Go's `os/exec` doesn't automatically send signals to process groups; the parent has to opt in. Crash paths (panic, OOM-kill, IDE-killed-subprocess) bypass `defer cleanup`. stdio transport coupling is implicit — there's no explicit "the pipe closed, kill the child" wiring unless you write it.

**How to avoid:**
- **Spawn every MCP server in its own process group** (`SysProcAttr{Setpgid: true}` on Unix). On shutdown, signal the **process group** (`syscall.Kill(-pgid, sig)`), not just the child PID — this catches grandchildren the MCP server itself spawned.
- Wire a **transport-coupled kill**: when the MCP server's stdout pipe closes (EOF on the read loop), treat it as "server gone" and `SIGTERM`/`SIGKILL` the process group after a short grace period. Don't leave the process around just because the pipe closed.
- Register a **signal handler** for `SIGINT`/`SIGTERM`/`SIGHUP` that walks all live MCP children and shuts them down before ass-guard exits. Combine with a `context.Context` cancellation that propagates to all child handlers. Don't rely on `defer` alone — `defer` doesn't run on `os.Exit` or signal death.
- `Wait()` on every spawned process from a dedicated **reaper goroutine** to avoid zombie accumulation even when the child has exited. (On Linux in containers, consider `prctl(PR_SET_PDEATHSIG)` so children die automatically if ass-guard dies — Go library: `go-prcind` or equivalent.)
- Add a **liveness check** per MCP server (periodic ping over JSON-RPC; or just track time-since-last-message). A server unresponsive past a threshold is killed and restarted (or marked degraded).
- Test the **crash-recovery path** in CI: kill ass-guard with `SIGKILL` mid-turn, then restart, and assert no orphan MCP processes remain (the test re-runs ass-guard, which should detect and clean up strays from the prior instance — or at minimum log them loudly).
- Document the **container-PID-1 caveat**: if ass-guard runs as PID-1 (unusual for an IDE-spawned subprocess, but possible in some Telegram-bot-only deployments), recommend `tini`/`dumb-init` as the parent.

**Warning signs:**
- `ps aux | grep mcp` shows growing counts of MCP processes over days
- RAM usage climbs steadily even when ass-guard is idle
- An MCP tool call hangs forever after the MCP server crashed silently
- No reaper goroutine exists in the MCP host code
- Tests don't exercise the signal-shutdown path

**Phase to address:** Phase C (subprocess hygiene is part of the MCP-hosting MVP)

*Sources: [I Built a Zombie Process Killer Because Claude Code Ate 14GB of My RAM (dev.to)](https://dev.to/thestack_ai/i-built-a-zombie-process-killer-because-claude-code-ate-14gb-of-my-ram-1deg); [hermes-agent gateway zombie-process bug](https://github.com/NousResearch/hermes-agent/issues/15012); [How to Fix Zombie Process Issues (OneUptime, 2026-01)](https://oneuptime.com/blog/post/2026-01-24-fix-zombie-process-issues/view); [Zombie process (Wikipedia)](https://en.wikipedia.org/wiki/Zombie_process).*

---

### Pitfall N14: MCP tool-schema drift between server versions — "works in Claude Code, breaks here"

**Severity: MEDIUM (compat gap, hardest to detect before runtime)**

**What goes wrong:**
ass-guard and Claude Code both host the same MCP server, but different versions of the server expose different tool schemas (a tool added a required field, renamed an argument, changed a return shape). ass-guard was tested against version A; the user has version B installed. The tool works in Claude Code (which tracks the server's current schema via the protocol's `tools/list`) but ass-guard was pinned to version A's schema and either fails or silently misbehaves.

**Why it happens:**
MCP's `tools/list` is dynamic — the server advertises its current schema at connection time. But if ass-guard caches the schema, bakes it into a profile, or otherwise freezes it, drift is invisible until runtime. Claude Code avoids this by re-fetching; ass-guard must too.

**How to avoid:**
- **Never cache MCP tool schemas across runs.** On every MCP-server connection, call `tools/list` and use the advertised schemas for that run. Re-fetch on reconnect after a crash.
- Within a run, **watch for `notifications/tools/list_changed`** (MCP spec) and re-fetch when the server signals a change. Don't assume the schema is stable for the session.
- Treat MCP tool schemas as **profile-overlay, not profile-baseline**: the zcode profile declares its built-in tools; MCP tools are layered on top at runtime from the live server's advertisement. The mimicry property (N1-N4) applies to the profile-baseline; MCP tools are explicitly "extra" and need not match anything.
- Add a **compat test matrix** in CI: install the N most common MCP servers at their latest versions, connect ass-guard, assert all advertised tools are callable with a smoke-test argument. Track which versions were tested; surface in release notes.
- When a tool-schema mismatch causes a call failure, surface a **clear error**: "MCP server X advertised tool Y with schema Z; ass-guard called it with schema W. Likely server version drift. Reconnecting to re-fetch." Auto-reconnect-once before surfacing as a hard error.

**Warning signs:**
- "Works in Claude Code, breaks in ass-guard" reports concentrate on MCP-served tools (not built-in tools)
- Tool schemas are cached to disk between runs
- No `tools/list_changed` handler exists
- The compat test matrix doesn't exist or is stale

**Phase to address:** Phase C

---

### Pitfall N15: MCP security — MCP servers run arbitrary code; ass-guard drives them ungated

**Severity: MEDIUM-HIGH in single-user, HIGH in multi-user (compounds with Telegram)**

**What goes wrong:**
PROJECT.md's safety model: "tools run ungated; the pattern/hook table + manual cancellation is the only safety mechanism." MCP servers are arbitrary executables (filesystem access, network, shell). An MCP server (malicious, compromised, or just buggy) can do anything the user can do. ass-guard drives MCP tools without confirmation. Compounded by Telegram: a voice command (potentially mis-transcribed, N12) could invoke an MCP tool that runs shell commands. The blast radius is the user's full system access, mediated by a voice interface.

**Why it happens:**
The "ungated tools" decision is reasonable for a single-developer IDE workflow where the developer reads every action before it commits. It stops being reasonable when (a) MCP servers from third parties are installed without audit, (b) Telegram enables remote driving, (c) STT errors inject unintended commands. The decision's threat model is the IDE-single-user case; the project's surface area has grown beyond it.

**How to avoid:**
- Document the **threat model per interface**: ACP = single-user, local, IDE-mediated (ungated tools acceptable, matching Claude Code's model). Telegram = potentially remote, voice-mediated, lower-trust surface. Different default policies per interface.
- For Telegram, **gate mutating MCP tools behind a per-tool allowlist** that defaults to empty. The user explicitly opts each MCP tool into Telegram-driveable. (This is a narrower gate than "every tool execution," which PROJECT.md rules out — it's "remote/voice-driven tool execution specifically.")
- Maintain an **MCP-server provenance record**: where each server was installed from (URL, package, hash), when, by whom (which interface). Surface this in a `ass-guard mcp audit` command so the operator can review what's running.
- For high-stakes MCP tool calls (filesystem write outside the project, shell execution, network egress to non-allowlisted hosts), surface a **pre-execution notification** to all interfaces (not just the driving one) — "about to run MCP tool X with arguments Y on host Z." This isn't a gate; it's visibility, compatible with the ungated model.
- Strongly recommend **MCP server sandboxing** in docs (containers, seatbelt/pledge, macOS Sandbox) for any non-first-party server. Don't require it (compatibility), but document it as the safe path.

**Warning signs:**
- MCP servers are installed from URLs without hash verification
- Telegram can drive every MCP tool by default
- No provenance record of where MCP servers came from
- Pre-execution notifications don't exist for any tool class
- The threat model document treats ACP and Telegram identically

**Phase to address:** Phase C (MCP hosting) and Phase I (Telegram gating — must be in place before Telegram is a peer)

---

### Pitfall N16: Audit log volume explosion and sensitive-data leakage

**Severity: MEDIUM (operational — disk fill, key leak, performance)**

**What goes wrong:**
PROJECT.md requires a "full audit log: user input, model requests, tool calls, and everything needed to reconstruct the exact sequence of actions." At realistic SDD workload (multi-stage scenarios, parallel subagents, long-running sessions), this is a lot of data. Failure modes: (a) **disk fill** — unbounded log growth crashes the agent or the system; (b) **sensitive data in logs** — model requests and tool calls include API keys (provider auth headers), source code (private/proprietary), secrets in env vars (Bash tool), and user PII (Telegram messages); (c) **performance overhead** — synchronous logging on the turn critical path slows the agent; (d) **reconstruction gap** — the log records what was sent but not enough to actually replay (missing intermediate state, missing model randomness seed, missing timing).

**Why it happens:**
"Log everything for reconstruction" is the right instinct but underspecified. Without rotation policy, redaction policy, and async-write design, the audit log becomes either a disk-filling firehose or a privacy liability.

**How to avoid:**
- **Async, bounded log writes.** The audit log is written through a buffered channel to a background goroutine; the turn critical path never blocks on log I/O. If the channel fills (slow disk), drop with a counter and log the drop count — never block the turn.
- **Redaction layer with explicit rules.** Every log entry passes through a redactor that strips: HTTP `Authorization` headers, known-secret env-var values (matched by name pattern — `*_TOKEN`, `*_KEY`, `*_SECRET`, `*_PASSWORD`), file contents matching a secret-detector regex (AWS keys, JWTs, private keys). Redacted entries record `redacted: [reason]`, never the value.
- **Rotation + retention policy** in config: max log size per session, max total log size, retention window (default 30 days). Rotate per-session (one file per session ID) so partial reads don't lock the whole log.
- **Reconstruction sufficiency test.** A CI test takes a recorded audit log and asserts it contains enough to replay the session deterministically against a mock model + mock tools: every model request, every tool call + result, every engine decision, every hook execution. If the replay can't reproduce the recorded transcript, the log is insufficient — and the test names the missing field.
- **Source code in logs is a policy decision.** Default: log tool-call arguments including file paths but redact file *contents* (Read/Edit/Bash results) unless an opt-in `--audit-include-content` flag is set. Document the tradeoff (reconstruction fidelity vs. privacy/disk).
- **Telegram input redaction:** Telegram messages may contain PII; log the message ID and a length, redact the body by default. Voice transcriptions likewise.
- **Performance budget:** add an audit-log overhead assertion to the turn-loop benchmarks; if logging exceeds (say) 2% of turn time, the redactor/writer is on the wrong path.

**Warning signs:**
- Audit logs are the largest files on disk after a week
- `grep -i key *.log` returns plaintext credentials
- Turn-loop benchmarks show log I/O in the critical path
- No CI test exercises log-driven replay
- A post-incident review can't reconstruct what happened from the logs alone
- Telegram message bodies appear in plaintext in logs

**Phase to address:** Phase A (audit log design is foundational — every other phase depends on it being usable)

---

### Pitfall N17: Learning mode proposes bad hooks or learns the wrong thing

**Severity: MEDIUM (quality erosion + user annoyance, slow to manifest)**

**What goes wrong:**
Learning mode (PROJECT.md delta 4) asks the user how to handle unfamiliar launch situations and proposes new hooks from the work log. Failure modes: (a) **learns the wrong thing** — user, annoyed by repeated prompts, accepts a wrong suggestion to make the prompt stop; the wrong hook is now persisted and fires forever; (b) **proposes bad hooks** — the work-log pattern the engine detected was coincidental, not causal, but the engine proposes a hook that codifies the coincidence; (c) **annoyance** — the engine asks too often, the user learns to dismiss without reading, and learning degrades to noise; (d) **rigidity** — the engine asks once and never re-asks, even though the right answer changed; (e) **persistence conflicts** — learned hooks conflict with each other or with the seeded set, producing N8-style loops.

**Why it happens:**
Learning is fundamentally inductive (pattern → proposed rule), and induction is wrong exactly when the pattern is coincidental. Annoyance-vs-accuracy is a hard UX tradeoff — ask too little, learn nothing; ask too much, learn noise. And learned rules ossify if there's no revisit mechanism.

**How to avoid:**
- **Confidence threshold for proposals** — the engine only proposes a hook if it has observed the pattern >= N times (say, 3) across separate sessions. Single-observation patterns are tracked but not proposed. This kills the coincidence trap.
- **Every learned hook ships with an expiry/review date** (say, 30 days). At expiry, the engine surfaces "you learned this hook a month ago; still right?" — converting one-shot learning into periodic re-validation.
- **Propose with rationale and counter-examples.** "I noticed you ran tests after implement 4 times; propose hook `post-implement: test`. Counter-example observed: 1 time you skipped tests; want to gate on a condition?" Surfacing counter-examples forces the engine to be honest about its confidence.
- **Announce mode** as the default for learning: the engine *logs* what it would have proposed, and surfaces a daily/weekly digest ("this week I would have proposed 3 hooks; review?") rather than interrupting inline. Inline prompts are reserved for the immediate "I don't know how to launch this — what do you want?" case, which is genuinely blocking.
- **Learned hooks are versioned and revertible.** Every accepted proposal creates a new version of the hook set; the user can roll back. This limits blast radius of a bad accept.
- **Conflict detection at acceptance time.** When the user accepts a proposed hook, the engine runs the loop-detector (N8) against the new hook set before committing. If the new hook creates a loop or conflicts with a seeded hook, surface the conflict and require explicit resolution.
- **Persistence-merge: append-only with override semantics.** Learned hooks don't edit seeded hooks; they layer on top with explicit precedence. Conflicts produce a documented resolution, not a silent clobber.

**Warning signs:**
- User dismisses learning prompts without reading (telemetry: dismiss-without-read rate > 50%)
- A learned hook fires on inputs the user didn't intend it to
- No counter-example surfaced with proposals
- Learned hooks have no review/expiry date
- A bad-accept requires manual editing of config files to undo
- Two learned hooks conflict in production and the engine didn't warn at accept time

**Phase to address:** Phase H (learning mode lives in the unified engine; the confidence/counter-example/conflict machinery ships with the first learning-mode version, not after)

---

### Pitfall N18: Greenfield-from-reference — copying predecessor code despite the no-port decision, and "we know this works" skip-verification

**Severity: MEDIUM-HIGH (silent foundation rot — compounds across all phases)**

**What goes wrong:**
Two related failure modes from the greenfield-from-reference decision. (a) **Copy temptingly close predecessor code** despite PROJECT.md's "fresh build; predecessor is reference-only (no code port)" decision. A function in `sdd-acp-agent` looks generic enough ("clearly this ACP frame parser is fine, just grab it"), gets copied, and now carries forward a decision that didn't fit ass-guard's scope — invisibly, because the git history doesn't show it as a deliberate decision. (b) **"We know this works, skip verification" on inherited facts.** The predecessor's research (2026-07-02) validated load-bearing facts: ACP is JSON-RPC over stdio, two provider shapes collapse, context-drop resolves to two-layer model. Those facts are 5 weeks old at project start. ACP may have shipped a spec revision; providers may have changed endpoints; the two-layer model may have a hole the predecessor didn't hit. ass-guard skips re-verification and discovers the drift in Phase M or later, when it's expensive.

**Why it happens:**
The predecessor's artifacts are first-class reference and live nearby on disk — copying is mechanically easy and feels safe because the code "already works." Re-verification feels wasteful because the predecessor "already proved it." Both impulses save time in the short term and cost time in the long term.

**How to avoid:**
- **Hard rule, enforced in review:** no code is copied from `sdd-acp-agent`. If a function looks generic enough to copy, rewrite it from understanding. The PROJECT.md decision is the authority; reviewer rejects copy-paste PRs. (One sentence in CONTRIBUTING: "Predecessor is reference-only; rewriting-from-understanding is mandatory even when copy-paste would compile.")
- **Re-verify every load-bearing inherited fact** in Phase 0 / Phase M kickoff, before building on it. Specifically: (1) re-confirm ACP spec version and method names against the canonical spec repo — community impls lag, and 5 weeks is enough for a revision; (2) re-confirm the two provider-shapes claim by hitting the live endpoints (Z.ai's Anthropic-compatible endpoint, MiniMax's OpenAI-shape endpoint); (3) re-confirm the two-layer context model satisfies the current ACP replay contract, not the 2026-07-02 version.
- **Document the re-verification** in the project's own research file (this one, or a successor). Each inherited fact gets `{fact, source, verified_date, verified_against_version}`. A fact verified 2026-07-02 and not re-verified is treated as **unverified** for ass-guard's purposes.
- **Resist scope creep from combining six deltas at once.** PROJECT.md's six deltas are individually coherent; their combination is the novelty. The roadmap must serialize them (mimicry first — it's the north star — then scheduling, then hooks, then interfaces), not attempt all six in parallel. A "let's build a thin slice of all six in Phase 1" plan feels like risk-reduction but is actually risk-multiplication: when (not if) the thin slice breaks, you can't tell which delta is at fault.
- **Reference, not oracle.** The predecessor's architecture spine (11 decisions) and epic breakdown (5 epics / 27 stories) are *input* to ass-guard's planning, not *constraints*. Decisions that don't fit the new scope (e.g. per-command flat routing → multi-tier scheduling) are explicitly superseded, not silently inherited.

**Warning signs:**
- A PR's diff includes code that's byte-identical or near-identical to `sdd-acp-agent` source
- An inherited fact is cited as "we know this works" with no ass-guard-era verification
- The roadmap attempts multiple deltas in the same phase rather than serializing
- The team refers to predecessor decisions as binding rather than as input
- ACP method names haven't been checked against the canonical spec since 2026-07-02

**Phase to address:** Phase 0 / Phase M (re-verification before any building; copy-ban enforced from project start)

---

## Critical Pitfalls — INHERITED (brief, see predecessor research for depth)

These carry forward from `sdd-acp-agent`'s Risk Assessment & Mitigation. They are still load-bearing for ass-guard; they are not re-researched here. Mitigation summaries are inherited; verify the underlying facts haven't drifted (N18).

### Pitfall I1: ACP spec drift / method-name mismatch

**Severity: HIGH (foundation).** Community ACP implementations lag the canonical spec; method names can drift between spec versions. **Inherited mitigation:** pin to a spec version; verify against canonical repo early; use schema types, not hand-rolled. **ass-guard note:** re-verify in Phase M kickoff — the predecessor's verification is 2026-07-02; 5 weeks is enough for a revision. **Phase: Phase M kickoff (re-verification).**

### Pitfall I2: DDG scrape-surface volatility

**Severity: MEDIUM.** The WebSearch backend (DDG scrape) breaks when DDG changes its HTML. **Inherited mitigation:** isolate DDG parsing behind an interface; tolerate partial results. **ass-guard note:** ass-guard generalizes the hardcoded DDG into configurable backends (PROJECT.md delta 3), which structurally mitigates this — but the default DDG backend still has the problem. **Phase: Phase M (or whenever WebSearch ships).**

### Pitfall I3: Provider tool-call shape edge cases

**Severity: MEDIUM.** Anthropic `tool_use` vs OpenAI `function_call` translation is fiddly; the most common integration-failure source. **Inherited mitigation:** adapter unit tests with recorded fixtures; default-model fallback. **ass-guard note:** compounded by N4 (catalog drift) and N7 (tier mismatch across providers). **Phase: Phase S.**

### Pitfall I4: Autocontinue false-positive pattern matches

**Severity: HIGH in predecessor, HIGH in ass-guard.** A pattern matches text that wasn't intended as a handoff signal. **Inherited mitigation:** v1 fires on turn-complete only; pattern table is exact-or-strict-regexp; curated per toolkit. **ass-guard note:** the unified engine (delta 5) widens this risk — see N8 (hook-DAG loops) for the structural mitigation (provenance tagging). **Phase: Phase H.**

### Pitfall I5: Context-drop breaking ACP replay

**Severity: HIGH.** ACP's replay-on-load contract requires the agent to reconstruct the session; aggressive context-drop breaks this. **Inherited mitigation:** two-layer model (durable replayable transcript + lean projected window), already designed out. **ass-guard note:** still load-bearing — the two-layer model must be present from day one; retrofitting is painful. Re-verify it satisfies the current ACP replay contract, not the 2026-07-02 version. **Phase: Phase M (foundational).**

### Pitfall I6: Pattern-table authoring friction for teams

**Severity: MEDIUM (adoption).** Teams struggle to author per-toolkit pattern tables; adoption stalls. **Inherited mitigation:** ship curated tables for OpenSpec; document the format; offer a learn mode later. **ass-guard note:** ass-guard's learning mode (delta 4) is the "learn mode later" made concrete — see N17 for its pitfalls. Seeded hook set (delta 5) further reduces authoring burden. **Phase: Phase H.**

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Hardcoding the zcode profile inline rather than as config | Faster Phase M demo | Forces a rewrite when profile #2 (non-zcode) arrives; PROJECT.md says architecture supports N from day one | Never — the N-profile architecture is cheap to build once and expensive to retrofit |
| Caching MCP tool schemas to disk for fast startup | Saves a `tools/list` round-trip per session | Silent schema drift (N14); "works in Claude Code, breaks here" reports | Never — re-fetch on every connection |
| Logging model requests synchronously on the turn critical path | Simpler implementation, no async machinery | Turn-loop latency under load; audit log becomes a bottleneck (N16) | Never — async logging is a small one-time cost |
| Treating ACP and Telegram input handlers symmetrically (same code path) | Less code; one input pipeline | Race conditions and state divergence (N10); no per-interface threat model (N15) | Never — serialize through a single mailbox; differentiate per interface downstream |
| Single-observation learning-mode proposals (propose on first sight) | Learning feels responsive; user gets value immediately | Codifies coincidences; bad hooks accumulate (N17) | Never — confidence threshold of >= 3 observations is the floor |
| Skipping profile-coverage manifest (extract profile, ship it) | Faster profile turnaround | Silent incomplete-capture holes (N3); mimicry fails on un-exercised paths weeks later | Never — coverage manifest is part of the profile artifact |
| Copying predecessor code that "obviously works" | Saves implementation time | Carries forward decisions that don't fit ass-guard's scope; invisible from git history (N18) | Never — rewrite from understanding; PROJECT.md decision |
| Fallback chain without circuit breakers or cost ceiling | Simpler chain implementation | Cascading failures, cost spikes during incidents (N5) | Acceptable in early Phase S demo only; must add before production use |
| Pattern-matching autocontinue without provenance tagging | Faster engine MVP | Hook loops (N8); runaway agent | Never — provenance is structural, not an optimization |
| Time-window config without explicit timezone | Convenient config; "obviously local" | Boundary bugs, DST issues, silent misrouting (N6) | Never — reject config without timezone |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Anthropic Messages API (Z.ai endpoint) | Assuming the Anthropic-compatible endpoint supports *every* Anthropic feature; some are silently dropped | Maintain a per-endpoint feature matrix; test each feature against the live endpoint; degrade gracefully where unsupported |
| OpenAI-shape providers (MiniMax M3, etc.) | Assuming the OpenAI shape implies OpenAI tool-calling semantics; argument-parsing quirks vary | Recorded-fixture tests per provider; never assume shape-compat = behavior-compat |
| ACP (editor as client) | Writing logs to stdout "just once" during debugging | Stdout is reserved for JSON-RPC frames (LSP-style discipline, non-negotiable); stderr-only logging enforced via a wrapper that fails fast on stdout writes outside the frame writer |
| MCP servers (subprocess stdio) | Not sending SIGTERM to the process group on shutdown; orphaning children | Process-group spawn + group-signal on shutdown; transport-coupled kill; reaper goroutine (N13) |
| Telegram Bot API | Sending messages >4096 chars and treating the resulting 400 error as transient | Chunk long messages ≤4096 chars (recommend ~1000 for safety with markdown); Markdown escaping eats into the budget — compute length after escaping |
| Telegram voice (STT) | Trusting transcription as authoritative | Echo back before acting; domain-term dictionary; confidence threshold (N12) |
| Z.ai / GLM endpoints (rate limits) | Treating 429 as "fall through to next provider" | Transient failures retry-with-backoff first; fall-through is for structural failures only (N5). Note the rolling weekly token quota, not just per-minute limits |
| Claude Code config (`.claude/`) | Clobbering Claude Code's files with ass-guard's additions | Namespace cleanly under `.claude/ass-guard/` (or per PROJECT.md's namespacing decision); never overwrite `CLAUDE.md`/`AGENTS.md`/`settings.json` |

*Sources: [Telegram 4096-char limit (node-telegram-bot-api Issue #165)](https://github.com/yagop/node-telegram-bot-api/issues/165); [Anthropic rate limits (Claude Platform Docs)](https://platform.claude.com/docs/en/api/rate-limits); [Anthropic rate-limit approach (support article)](https://support.claude.com/en/articles/8243635-our-approach-to-rate-limits-for-the-claude-api).*

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Synchronous audit-log writes on the turn path | Turn latency correlates with disk speed; agent feels sluggish under load | Async buffered channel + background writer; drop-with-counter on overflow | Any non-trivial session, immediately under load |
| Per-turn MCP server re-fetch of `tools/list` blocking the turn start | Turn-start latency spikes; user-visible delay | Fetch in background at session start; cache for the session with `tools/list_changed` invalidation | Many MCP servers connected |
| Pattern matching run on every streaming token (predecessor's open question) | CPU spike during long model outputs; false positives mid-token | Match on turn-complete only (predecessor's v1 decision; carry forward) | Long outputs, fast streaming |
| Audit log growing without rotation | Disk fill over weeks; log reads slow | Per-session rotation; retention window; max-total-size cap | Week-scale usage |
| Fallback chain retrying known-bad providers on every turn | Per-turn latency variance; cascading stalls under incidents | Per-provider circuit breakers; short-circuit unhealthy providers within a cooldown window | Provider incidents |
| Profile serialization recomputing canonical→wire mapping per request | Request-prep latency; CPU waste | Cache the serialized profile; invalidate on profile update | High request rate |
| Subagent goroutines each loading full tool catalog | Memory growth with parallelism; startup latency per subagent | Subagents get a restricted tool subset (per PROJECT.md); share read-only catalog state | Many parallel subagents |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Telegram can drive every MCP tool by default | Remote code execution via voice command (compounded by STT errors, N12+N15) | Per-interface MCP-tool allowlist for Telegram; default deny for mutating tools on remote interfaces |
| API keys logged in plaintext (provider auth headers in audit log) | Credential leak via log file (disk, backups, log shipping) | Redaction layer; strip `Authorization` headers and secret-named env vars by default (N16) |
| MCP servers installed from URLs without hash verification | Supply-chain compromise; arbitrary code execution at install time | Provenance record (URL, hash, installer, timestamp); warn on unsigned/unhashed installs (N15) |
| Voice transcription trusted as authoritative | Misheard mutating commands ("apply to main" / "apply to mane") execute without visual confirmation | Echo-back-before-acting; secondary confirmation for mutating commands (N12) |
| No threat-model differentiation between ACP (local, single-user) and Telegram (potentially remote, voice) | Threats meant for one surface bleed into the other; per-interface policy absent | Document per-interface threat model; default-deny for high-stakes actions on the remote interface (N15) |
| Source-code contents in audit log by default | Proprietary code leaks via log file; privacy liability for user PII | Default redact file contents; opt-in `--audit-include-content`; redact Telegram message bodies (N16) |
| MCP server inherits ass-guard's full filesystem/network access | Malicious or buggy MCP server exfiltrates code or secrets | Recommend sandboxing (containers, seatbelt) for non-first-party servers; document as the safe path (N15) |
| Cancellation signal doesn't drain queued injections | User cancels, but a queued (e.g. autocontinue) injection still fires and runs to completion | Cancellation drains the queue per PROJECT.md's safety model; test the drain path explicitly |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Silent fallback to expensive model during peak hours | Bill shock; user feels the agent is "out of control" cost-wise | Surface the fallback decision; bound cost escalation; wait+log during peak hours rather than silently escalating (N5) |
| Learning mode prompts on every unfamiliar pattern | Annoyance; user dismisses without reading; learning becomes noise | Announce/digest mode default; inline prompts only for genuinely-blocking "I don't know how to launch this" cases (N17) |
| Long ACP run freezes Telegram with no feedback | User on phone thinks Telegram is broken; loses trust in the secondary interface | Status queries always available; progress notifications to the non-driving interface (N11) |
| Voice command acts without echoing transcription | User said X, agent heard Y, did Y, user can't undo in time | Echo "I heard: …" with a brief cancel window before acting (N12) |
| Profile drift silently degrades mimicry | User perceives "the agent got worse" but can't tell why; no actionable signal | Profile-drift detector; loud warning when installed zcode is outside the profile's validated range (N1) |
| Workflow halts on every minor hook failure | Hands-off value proposition defeated; user intervenes constantly | Per-hook failure policy (`halt` / `continue` / `ask`); seeded set curated to favor `continue` for advisory hooks (N9) |
| Audit log can't reconstruct what happened | Post-incident review is impossible; user loses trust | Reconstruction-sufficiency test in CI; replay-the-session-from-log as an acceptance criterion (N16) |
| Tier mismatch across providers produces silently worse results | Fallback "succeeds" but output quality drops; user can't pin why | Capability delta logged on cross-provider fallback; per-(provider, tier) telemetry (N7) |

## "Looks Done But Isn't" Checklist

- [ ] **Mimicry profile:** Often missing the **coverage manifest** — verify every tool/section has an observation count and a source-log reference (N3)
- [ ] **Mimicry profile:** Often missing the **target_capture_ref** (zcode version) — verify the profile records what it was captured from and refuses to load without it (N1)
- [ ] **Mimicry parity:** Often "passes" via textual diff — verify the acceptance test is **behavioral** (same tools called in same order on a fixed prompt suite), not `diff`-based (N2)
- [ ] **Tool catalog:** Often missing the **profile-to-implementation consistency check** — verify CI rejects profiles whose declared schemas the built-in implementations can't satisfy (N4)
- [ ] **Fallback chain:** Often missing **circuit breakers and cost ceiling** — verify correlated-failure CI test (all providers 503) fails fast, doesn't burn the retry budget (N5)
- [ ] **Time-window scheduler:** Often missing **timezone** in config — verify every window has an IANA zone and the binary bundles `tzdata` (N6)
- [ ] **Unified engine:** Often missing **provenance tagging** on turns — verify the engine never fires on `hook`-provenance turns (loop-detector fixture passes) (N8)
- [ ] **Hook-DAG:** Often missing per-hook **failure policy** — verify config validation rejects hooks without `on-failure` (N9)
- [ ] **ACP + Telegram:** Often missing the **single session mailbox** — verify both interfaces drain through one serialized channel (N10)
- [ ] **Telegram voice:** Often missing **echo-back-before-acting** — verify a voice command produces a "I heard: …" message with a cancel window before any tool executes (N12)
- [ ] **MCP hosting:** Often missing the **process-group spawn + reaper goroutine** — verify killing ass-guard with `SIGKILL` mid-turn leaves no orphan MCP processes (N13)
- [ ] **MCP hosting:** Often missing the **`tools/list_changed` handler** — verify schema re-fetch works after a server signals a change (N14)
- [ ] **Audit log:** Often missing the **reconstruction-sufficiency test** — verify a recorded log can drive a deterministic replay against mock model + mock tools (N16)
- [ ] **Audit log:** Often missing the **redaction layer** — verify `grep -i key *.log` returns no credentials after a real session (N16)
- [ ] **Learning mode:** Often missing the **confidence threshold and counter-example surfacing** — verify no proposal fires on single-observation patterns (N17)
- [ ] **Greenfield discipline:** Often missing **ass-guard-era re-verification of inherited facts** — verify ACP spec, provider shapes, and two-layer context model have been checked against current sources, not 2026-07-02 sources (N18)

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Profile drift detected (N1) | LOW | Re-run extractor against fresh zcode logs; diff against shipped profile; release new profile version; pin new `target_capture_ref` |
| Profile shipped with incomplete capture (N3) | LOW-MEDIUM | Identify gaps from coverage manifest; run targeted capture session for un-exercised tools; re-extract; ship new profile version |
| Tool-catalog drift broke mimicry (N4) | MEDIUM | Add adapter translation for the drifted schema; backfill CI catalog-consistency check; release; audit existing profiles for the same drift pattern |
| Fallback chain cascaded, cost spike (N5) | MEDIUM (cost already incurred) | Add circuit breakers; add cost ceiling; review the time-window scheduler's fallback-escape behavior; refund/budget-adjust if possible |
| Hook-DAG infinite loop ran (N8) | HIGH (state may be corrupted) | Cancel; rollback project state via git; add provenance-tagging fix; add the specific loop pattern to the loop-detector fixture; audit log review for what was mutated |
| Bad hook accepted via learning mode (N17) | LOW | Roll back to prior hook-set version; add counter-example to the proposal's audit trail; tighten confidence threshold |
| MCP zombies accumulated (N13) | LOW | Kill stray processes (manual or via a cleanup script); add process-group spawn + reaper; add the crash-recovery test; consider a startup sweep that detects and cleans strays from prior instances |
| Audit log leaked credentials (N16) | HIGH | Treat as a credential compromise: rotate every key whose header appeared in the log; secure/destroy the leaked log files; add the redaction rule that was missing; audit what was exfiltrated |
| Telegram drove an unwanted mutating tool (N15) | HIGH (depends on tool) | Stop the tool if still running; rollback project state via git; add the tool to the Telegram-deny list; review the threat model |
| Voice command mis-transcription executed (N12) | MEDIUM | Rollback the action (often git-revertible); add the confusion to the domain-term dictionary; tighten confidence threshold for the offending phrase |
| Inherited fact found drifted (N18) | HIGH (foundational) | Stop building on the drifted fact; re-verify against current sources; update the project's own research; revise downstream plans that assumed the old fact |
| ACP and Telegram state diverged (N10) | HIGH | Reconcile from the durable transcript (single source of truth); discard the divergent interface's local state; add the serialization mailbox if missing; add the interleaving test |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| N1 — Profile drift | Phase M | Drift detector exists; CI runs it on every profile change; profile-version pinning enforced |
| N2 — Over-investing in byte-identical | Phase M | Acceptance test is behavioral (A/B prompt suite), not textual diff; cosmetic-vs-structural triage documented |
| N3 — Incomplete capture | Phase M | Every shipped profile has a coverage manifest; coverage <100% is a documented known-gap |
| N4 — Tool-catalog drift | Phase M | CI catalog-consistency check rejects profiles whose schemas built-ins can't satisfy |
| N5 — Fallback chain cascading | Phase S | Correlated-failure CI test (all providers 503) fails fast within cost budget |
| N6 — Time-window boundaries | Phase S | Timezone-explicit config enforced; `tzdata` bundled; boundary-minute test passes; dry-run scheduler CLI works |
| N7 — Tier mismatch across providers | Phase S | Per-(provider, tier) capability profile documented; cross-provider fallback logs capability delta |
| N8 — Hook-DAG infinite loops | Phase H | Provenance-tagging enforced; loop-detector fixture passes; injection-depth counter exists |
| N9 — Hook failure semantics | Phase H | Config validation rejects hooks without `on-failure`; seeded set curated with policies |
| N10 — ACP/Telegram concurrency | Phase I | Single mailbox sits between interfaces and turn loop; interleaving matrix test passes |
| N11 — Long-turn blocking | Phase I | Status query always available; progress notifications to non-driving interface; cancel-via-either works |
| N12 — Voice transcription errors | Phase I | Echo-back-before-acting exists; domain-term dictionary exists; confidence threshold enforced |
| N13 — MCP subprocess zombies | Phase C | Process-group spawn + reaper goroutine; `SIGKILL` test leaves no orphans |
| N14 — MCP schema drift | Phase C | `tools/list` re-fetched every connection; `tools/list_changed` handler exists; compat test matrix runs in CI |
| N15 — MCP security | Phase C + Phase I | Per-interface threat model documented; Telegram mutating-tool allowlist defaults empty; provenance record per MCP server |
| N16 — Audit log volume/leak | Phase A | Async writes; redaction layer; rotation policy; reconstruction-sufficiency test passes |
| N17 — Learning mode bad proposals | Phase H | Confidence threshold (>=3 observations); counter-example surfacing; conflict detection at accept; review/expiry dates |
| N18 — Greenfield-from-reference | Phase 0 / Phase M kickoff | No predecessor code in diffs; every inherited fact has ass-guard-era re-verification record |
| I1 — ACP spec drift | Phase M kickoff (re-verify) | Spec version pinned; method names verified against canonical repo since 2026-08-xx |
| I2 — DDG scrape volatility | Phase M (or WebSearch phase) | DDG parsing behind interface; partial-results tolerated; configurable backend abstraction in place |
| I3 — Provider tool-call shape edge cases | Phase S | Adapter tests with recorded fixtures; default-model fallback works |
| I4 — Autocontinue false positives | Phase H | Pattern table curated per toolkit; turn-complete-only firing; provenance tagging (N8) layered on top |
| I5 — Context-drop vs ACP replay | Phase M (foundational) | Two-layer model present from day one; replay contract re-verified against current ACP spec |
| I6 — Pattern-table authoring friction | Phase H | Curated OpenSpec patterns shipped; learning mode (N17) reduces authoring burden |

## Sources

### External (cited inline above)
- [I Built a Zombie Process Killer Because Claude Code Ate 14GB of My RAM (dev.to)](https://dev.to/thestack_ai/i-built-a-zombie-process-killer-because-claude-code-ate-14gb-of-my-ram-1deg) — MCP server orphan/zombie accumulation, real-world
- [hermes-agent gateway zombie-process bug (GitHub)](https://github.com/NousResearch/hermes-agent/issues/15012) — PID-1 / process-reaping failure mode for MCP-hosting gateways
- [How to Fix Zombie Process Issues (OneUptime, 2026-01)](https://oneuptime.com/blog/post/2026-01-24-fix-zombie-process-issues/view) — zombie prevention patterns
- [Zombie process (Wikipedia)](https://en.wikipedia.org/wiki/Zombie_process) — zombie vs orphan distinction
- [How to kill (or avoid) zombie processes with subprocess module (Stack Overflow)](https://stackoverflow.com/questions/2760652/how-to-kill-or-avoid-zombie-processes-with-subprocess-module) — `wait()`/`waitpid()` patterns
- [Telegram Bot API 4096-char message limit (node-telegram-bot-api Issue #165)](https://github.com/yagop/node-telegram-bot-api/issues/165) — hard limit, bot-side chunking required
- [Telegram Bugs Tracker #1423](https://bugs.telegram.org/c/1423) — bot vs client behavior on long messages
- [Anthropic rate limits (Claude Platform Docs)](https://platform.claude.com/docs/en/api/rate-limits) — RPM/ITPM/OTPM per tier
- [Our approach to rate limits for the Claude API (Anthropic Support)](https://support.claude.com/en/articles/8243635-our-approach-to-rate-limits-for-the-claude-api) — tier-based limits, weekly rolling quota
- [Claude API Token Limits Just Jumped 10x — Tier Breakdown (MindStudio)](https://www.mindstudio.ai/blog/claude-api-token-limits-increase-tier-breakdown) — recent per-tier limit changes; OTPM remains a bottleneck

### Project-internal (predecessor research)
- `/Users/nil/DiskD/W/Djarvur/sdd-acp-agent/_bmad-output/planning-artifacts/research/technical-sdd-acp-agent-research-2026-07-02.md` — predecessor's Risk Assessment & Mitigation section; source of the inherited pitfalls (I1-I6) and several load-bearing facts (ACP wire protocol, two provider shapes, two-layer context model)
- `/Users/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/PROJECT.md` — ass-guard scope, six deltas, out-of-scope (byte-identical mimicry, confirmation tier, predecessor code port), key decisions

### Reasoning-source (project-specific; not externally citable)
- N1/N3/N4 — mimicry-specific reasoning grounded in PROJECT.md's mimicry decisions (log-extracted profiles, structurally-indistinguishable bar, tool-catalog fidelity)
- N6 — timezone/boundary reasoning grounded in Go's `time` package semantics
- N8 — provenance-tagging mitigation grounded in the unified-engine decision (delta 5)
- N10/N11 — concurrency and serialization reasoning grounded in the two-peer-interface decision (delta 6)
- N16 — audit-log reasoning grounded in PROJECT.md's full-audit-log requirement
- N17 — learning-mode reasoning grounded in delta 4
- N18 — greenfield discipline grounded in the "fresh build, predecessor reference-only" key decision

---
*Pitfalls research for: ass-guard-agent (Go-based SDD-hosting AI coding agent with mimicry, scheduling, hook-DAG, learning, ACP+Telegram, MCP hosting)*
*Researched: 2026-08-09*
