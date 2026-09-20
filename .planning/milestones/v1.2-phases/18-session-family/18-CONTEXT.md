# Phase 18: Session Family - Context

**Gathered:** 2026-08-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Sessions become first-class objects the editor can enumerate and restore: session/list (header-scan, cursor pagination), session/load with full replay through the ordered TurnEmitter plus live-state reconciliation (synthetic interrupted-closures for dangling expectations), session/close/delete with tombstoning preserving the D-20 audit invariant, and CC-parity --resume/--continue CLI flags. The kill -9 mid-turn → resume with zero ghost state is the acceptance spine.

</domain>

<decisions>
## Implementation Decisions

### Reconciliation Architecture
- **D-01:** Transcript-as-truth: resumed live-state reconstructs from the transcript ALONE plus synthetic closures — no session-state sidecar to corrupt or drift under kill -9. Separately-persisted stores (schedule lastFired, learning) reload as today, outside session reconciliation. Any genuinely non-derivable state a planner encounters must justify a sidecar per-case through the deviation protocol. — **Reversibility:** one-way — the resume contract (single source of truth) becomes load-bearing for every later session feature; a sidecar added casually breaks kill-9-proofness.
- **D-02:** Synthetic closures carry provenance: replay shows them with an interrupted/synthetic marker — subtle in the UI, unambiguous on disk. Post-mortem of kill -9 cases stays possible (D-20 audit spirit).
- **D-03:** Reconcile-then-accept: session/load completes full replay + reconciliation BEFORE any new prompt is accepted (linearizable resume). No interleaving race between replayed frames and live turns; criterion 2's "no ghost state" is guaranteed by ordering.

### List & Pagination
- **D-04:** On-demand header scan: session/list walks transcripts mtime-ordered with first-line reads — no index file, nothing to drift, append-only discipline preserved. Session counts are small; scan is milliseconds.
- **D-05:** Sort: lastActivity descending with a composite (lastActivity, id) cursor. Best-effort paging stability documented (a session that becomes active mid-paging can shift) — picker utility over paging purity.
- **D-06:** Lean header: sessionId, title (first user prompt, truncated), createdAt, lastActivity, state flags (checkpoint availability). Rich stats (tokens/cost) wait for /cost in Phase 20 rather than bloating the header.

### Delete & Tombstones
- **D-07:** Tombstone = zero-byte marker file beside the transcript (`<id>.deleted`). List filters by stat (no read); transcript bytes are never touched; unambiguous and reversible on disk.
- **D-08:** Delete covers transcript + checkpoints + session-scoped derived artifacts; the audit trail survives untouched — separate policy, D-20 invariant verbatim.
- **D-09:** GC after grace (operator chose auto-purge): 30-day default, configurable through the configOptions menu (joins Phase 16's rule: full menu, apply-as-landed). Tombstoned artifacts are physically purged when grace expires; audit NEVER purges; reversible until the purge fires (remove marker). — **Reversibility:** one-way — purge destroys the transcript permanently; the grace window is the only undo.

### Resume CLI + Close
- **D-10:** Full CC trio: `--resume` (no args) opens the interactive picker; `--resume <id|name>` resumes directly; `--continue`/`-c` resumes the most recent session with zero ceremony. Parity with Claude Code's documented surface.
- **D-11:** Terminal picker = numbered list on stdin/stderr (N recent sessions: title + relative time; user types a number). No raw-mode TUI dependency — works over pipes, ssh, plain TTY.
- **D-12:** session/close = cancel-and-drain: in-flight turns cancelled via the existing cancel contract, queued asks drained cancelled (17-D-13), buffers flushed, then closed. Re-close is idempotent (already-closed = success). Criterion 4's "stops its work cleanly" verbatim.

### Claude's Discretion
- Kill -9 test matrix design (roadmap-flagged: enumerate the live-state inventory during planning — the inventory derives from the transcript kinds + ask/pending registries).
- Title truncation length, relative-time formatting in the picker.
- Tombstone-purge sweep trigger point (session-start sweep vs close-time check) within the 30d contract.
- `--continue`'s directory-scoping (CC scopes to cwd) vs global most-recent — planner checks CC behavior and matches.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §ACP completeness — ACP-05 (list, header-scan cursor pagination), ACP-06 (load/resume + reconciliation + `--resume` anywhere), ACP-07 (close/delete, tombstoning never rm, delete best-effort) verbatim
- `.planning/ROADMAP.md` §Phase 18 — goal, 4 success criteria; §Research Flags — Phase 18 flag: resume reconciliation is the densest correctness surface; enumerate the live-state inventory and design the kill -9 test matrix during planning

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-02 (ordered emitter replay rides), D-19 (registry synthetic-cancel), D-20 (additive transcript kinds — tombstone/audit interplay)
- `.planning/phases/17-permissions-elicitation/17-CONTEXT.md` — D-13 (turn-death ask drain — reused for close), D-05/D-06 (gate state in reconciliation)
- `.planning/phases/25-seed-001-kit-extraction-strictly-last/25-CONTEXT.md` — D-13/D-14: session family must not foreclose the kit Emitter/Requester seam (resume is a second Emitter consumer)

### External References
- `code.claude.com/docs/en/sessions` — CC's --resume/--continue contract (D-10's parity target)
- `agentclientprotocol.com/protocol/v1/schema` — session/list, session/load, session/close, session/delete shapes (research verifies; delete is spec-unstable → best-effort)

### Code Anchors
- Transcript manager (appendLine, first-line headers) — D-04/D-06's scan surface
- Session replay-on-restart machinery (v1.0 SESS) — the foundation D-01 hardens
- Cancel contract + ask drain (post-17) — D-12's close rides it

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Session replay on restart (v1.0): the existing load path becomes D-01's reconstruction engine — extended, not replaced.
- Transcript append-only manager: headers already first-line; D-06 extends leanly.
- Cancel-drain discipline at ACP level (v1.1) + 17-D-13 turn-scoped ask drain — close = the same machinery aimed at a whole session.
- CSPRNG session ids (newSessionID) — collision-free continuation from transcript maxima is a max() scan, not regeneration.

### Established Patterns
- Graceful degradation, loud counters — D-09's purge sweep logs what it removed.
- Append-only + marker discipline (schedule store's lastFired JSON) — D-07's marker file follows the family.
- Text-mode-friendly interaction (no raw-mode dependencies) — D-11's numbered picker.

### Integration Points
- session/list|load|close|delete handlers in internal/acp — new RPC surface.
- CLI flag parsing in cmd (root/acp command) — --resume/--continue pre-serve selection.
- configOptions menu (Phase 16 rule) gains the tombstone-grace key.
- TurnEmitter (Phase 16) — replay's frame source, identical to live turns.

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- GC-after-grace chosen deliberately over the no-GC recommendation — disk hygiene valued, with the 30d/audit-exempt parameters keeping D-20 absolute.
- Full CC trio for the CLI: the operator wants the daily-driver convenience (`-c`) at parity, not just the resumability primitive.

</specifics>

<deferred>
## Deferred Ideas

- Dedicated session-restore surface (un-delete UI/CLI beyond removing the marker file) — post-v1.2 if wanted.

</deferred>

---

*Phase: 18-session-family*
*Context gathered: 2026-08-26*
