---
phase: 261013-uhz
plan: 01
type: execute
wave: 1
depends_on: []
files_modified: [".planning/WINDOWS.md"]
autonomous: true

must_haves:
  truths:
    - "WINDOWS.md rows #23 and #24 carry status fixed with dated discharge reasons citing the operator Phase-23 UAT 2/2 PASS (2026-09-20) and the archived evidence file"
    - "The 23-UAT stdout-discipline observation is recorded as ledger row #30 (kind deviation, phase 23), status fixed, with a reason in the ledger's dated fixed-resolution format (row #8 precedent) citing commit ed83044, the non-reproduction probe, and TestGitRun_PipesBothChildStdioFds"
    - "Frontmatter counts equal entries-derived counts (open 16, waived 0, fixed 14, total 30) and the markdown table is byte-identical to renderTable of the fenced JSON entries — the ledger passes the windows read validator and any future write-path drift guard"
    - "Rows #21, #22, and #27 remain open with empty reasons — untouched, since sibling items 261013-uhx and 261013-uhy own them"
  artifacts:
    - path: .planning/WINDOWS.md
      provides: "Discharged Phase-23 UAT windows (#23, #24), a recorded closure for the stdout-discipline observation (row #30), and a consistent three-way (frontmatter / fenced JSON / rendered table) ledger"
  key_links:
    - "row #30 reason → commit ed830444adf71996d73147d17401dd2561f8a592 (gitRun child-stdio piping property; pin test at kit/checkpoint/store_test.go:1475)"
    - "rows #23/#24 reasons → .planning/milestones/v1.2-phases/23-seed-gaps-close-out/23-UAT-evidence-2026-09-20.md (Test 1 and Test 2 PASS) + STATE.md Phase 23 entry"
    - "frontmatter counts ↔ fenced JSON entries ↔ rendered table (three-way agreement enforced by parseLedger and the write-path table drift guard)"
---

<objective>
Broken Windows ledger bookkeeping: discharge the two Phase-23 PENDING-OPERATOR-CONFIRMATION windows (#23, #24) that the operator UAT passed 2/2 on 2026-09-20, record the 23-UAT stdout-discipline observation as fixed by commit ed83044, and refresh the ledger counts so `/gsd-ship`'s window gate reads truth.

Purpose: the ledger (.planning/WINDOWS.md) feeds `workflow.windows_enforce` (ship blocks while open_count > 0) and the next milestone's planning; stale PENDING rows misstate v1.2's actual state, and the stdout observation currently lives only in STATE.md Blockers/Concerns with no ledger closure.

Output: an internally consistent .planning/WINDOWS.md with #23/#24 fixed-with-reason, new row #30 recorded fixed-with-reason, counts open 16 / waived 0 / fixed 14 / total 30, and table+JSON+frontmatter in three-way agreement. Rows #21/#22/#27 are NOT touched (siblings 261013-uhx/261013-uhy own them).
</objective>

<execution_context>
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/workflows/execute-plan.md
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/templates/summary.md
</execution_context>

<context>
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/STATE.md
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/WINDOWS.md

Verified facts this plan relies on (batch planning evidence, re-confirmed during planning):

1. **Ledger mechanics (gsd-core/bin/lib/broken-windows.cjs):** the fenced ````json block is the SOLE source of truth; the markdown table is a rendering of it. The CLI verbs `windows fixed <id>` and `windows append --kind K --phase N [--file F] [--line L] --description D` recompute frontmatter counts (open/waived/fixed/total + last_updated), re-render BOTH surfaces, and write atomically. The write path drift-guards: any table not byte-identical to renderTable(entries) aborts the command. `windows fixed` does NOT set a reason — resolution narratives are hand-filled in the JSON entry AND the matching table row cells (the ledger's own convention; row #8 is the fixed-resolution format precedent: dated prefix like `260821-close:` + narrative with commit hash and evidence). Row ids: #23 and #24 are open (phase 23 unmet-truth rows); there is NO existing row for the stdout observation — it lives only in STATE.md Blockers/Concerns, so it is recorded as new row #30 per the ledger's defect-register purpose, born-fixed with the resolution in `reason`. Valid kinds include `deviation` (already used by rows #1/#12/#16/#20/#21/#27).
2. **UAT discharge (rows #23/#24):** operator Phase-23 UAT passed 2/2 on 2026-09-20 — STATE.md Phase 23 entry ("operator UAT 2/2 passed after G-23-1 gap closure … steering + idle/mid-turn /undo + cross-session refusal-then-restore operator-confirmed") and the archived evidence file .planning/milestones/v1.2-phases/23-seed-gaps-close-out/23-UAT-evidence-2026-09-20.md, whose own headings read "Test 1 — Live-Zed steering + /undo operator UAT (WINDOWS #23): PASS" and "Test 2 — Two-live-session cross-session restore check (WINDOWS #24): PASS". Note the phase dirs were archived post-milestone: the rows' file columns keep their original recorded pointers (.planning/phases/23-seed-gaps-close-out/...-SUMMARY.md) — do NOT rewrite them (archived row #11 set the precedent).
3. **stdout-discipline closure (row #30):** commit ed830444adf71996d73147d17401dd2561f8a592 "fix(checkpoint): pin child-stdio piping behind a regression test (23-UAT stdout observation)", committed 2026-09-27 21:15:27 +0300 (= 2026-09-27T18:15:27Z), touching kit/checkpoint/store.go (+9) and kit/checkpoint/store_test.go (+23). On v1.2 release code the leak does not reproduce (fresh-store acp serve probe keeps stdout byte-clean); the production gitRun seam has piped both child fds since the store's first commit; the named regression pin TestGitRun_PipesBothChildStdioFds sits at kit/checkpoint/store_test.go:1475.
4. **Scope fence:** touch ONLY rows #23, #24, #30 and the frontmatter. #21/#22/#27 belong to sibling quick items (261013-uhy, 261013-uhx) — do not mark, edit, or reword them.

Tooling paths (executor cwd may reset — always absolute):
- Tools: /home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/bin/gsd-tools.cjs
- Repo root: /home/nil/DiskD/W/Djarvur/ass-guard-agent (pass `--cwd` with this absolute root to every windows command)
- Ledger: /home/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/WINDOWS.md (current: open 18 / waived 0 / fixed 11 / total 29, read path verified green)
</context>

<tasks>

<task type="auto">
  <name>Task 1: Tool-side ledger mutations — flip #23/#24 fixed, append and fix row #30</name>
  <files>.planning/WINDOWS.md</files>
  <action>
Run four gsd-tools windows commands against the repo root (absolute `--cwd`, absolute tools path). Order matters only in that `append` must precede `fixed 30`.

1. `windows fixed 23` and `windows fixed 24` — the tool sets status=fixed and resolved_at=now on each, recomputes counts, and re-renders the table + fenced JSON atomically. Use the tool rather than hand-editing because the write path enforces byte-exact table rendering and count agreement; hand-editing only the rendered table is rejected by design.
2. `windows append` with exactly: `--kind deviation --phase 23 --file kit/checkpoint/store.go --description` plus (as one quoted argument) this description text: `23-UAT stdout-discipline observation (2026-09-20, non-blocking): agent emitted a git-checkpoint store-init log line on stdout at startup — stdout is reserved for ACP frames; evidence 23-UAT-evidence-2026-09-20.md; also logged in STATE.md Blockers/Concerns`. The tool assigns id 30 (max+1), status=open, recorded_at=now, and recomputes counts. Text discipline: the description deliberately contains no pipe, backslash, or backtick characters — the renderer escapes only pipes/backslashes, so keeping them out makes the later hand-edits byte-exact by construction. Kind `deviation` fits: the observation is a transport-discipline deviation surfaced at UAT (same kind the ledger uses for rows #1/#12/#16/#20/#21/#27).
3. `windows fixed 30` — flips the new row to fixed with resolved_at=now (the narrative reason and the truthful ed83044 resolved_at are recorded in Task 2).

After the four commands the tool-recomputed frontmatter must already read: open_count 16, waived_count 0, fixed_count 14, total_count 30 (18 opens − 2 fixed + 1 appended − 1 fixed = 16; 11 fixed + 2 + 1 = 14). Do not touch rows #21/#22/#27 (scope fence, fact 4).
  </action>
  <verify>
    <automated>T=/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/bin/gsd-tools.cjs; R=/home/nil/DiskD/W/Djarvur/ass-guard-agent; [ "$(node "$T" windows status --cwd "$R" --pick ledger.total_count)" = 30 ] && [ "$(node "$T" windows status --cwd "$R" --pick ledger.open_count)" = 16 ] && [ "$(node "$T" windows status --cwd "$R" --pick ledger.fixed_count)" = 14 ] && grep -q '^| 30 | 23 | deviation | kit/checkpoint/store.go' "$R/.planning/WINDOWS.md" && echo TASK1-OK</automated>
  </verify>
  <done>Ledger has 30 entries; #23/#24/#30 all status fixed; frontmatter reads open 16 / waived 0 / fixed 14 / total 30; row #30 is kind deviation, phase 23, file kit/checkpoint/store.go; #21/#22/#27 untouched.</done>
</task>

<task type="auto">
  <name>Task 2: Record the three fixed-resolutions (reasons in JSON block + table rows, truthful resolved_at for #30)</name>
  <files>.planning/WINDOWS.md</files>
  <action>
The `windows fixed` verb cannot record reasons, so fill the resolution narratives by hand — ALWAYS in both surfaces (the fenced JSON entry field AND the identical table row cell), keeping them textually identical so the byte-exact table invariant of Task 3 holds. Continue avoiding pipe, backslash, and backtick characters in inserted text. Use scoped Edit replacements on the specific fields/rows only.

Reasons (dated fixed-resolution format per the row #8 precedent — YYMMDD tag prefix + narrative with commit hash and evidence pointers):

- Row #23 reason: prefix `260920-uat:` and state it was discharged by the operator Phase-23 UAT 2/2 PASS on 2026-09-20, Test 1 — live-Zed boundary steering mid-turn plus idle and mid-turn /undo including the auto-cancel-then-restore path, run with real Zed 1.20.2 and a Zed-shaped ACP driver against the shipped binary; cite the evidence path .planning/milestones/v1.2-phases/23-seed-gaps-close-out/23-UAT-evidence-2026-09-20.md and the STATE.md Phase 23 entry.
- Row #24 reason: prefix `260920-uat:` and state it was discharged by the same UAT, Test 2 — two-live-session cross-session restore check PASS (/undo from an idle session refuses naming the busy session); cite the same evidence path and STATE.md Phase 23 entry.
- Row #30 reason: prefix `260927-close:` and state: non-reproducing on v1.2 release code (fresh-store acp serve probe keeps stdout byte-clean) and the gitRun seam has piped both child fds since the store's first commit; closed by ed83044 (fix(checkpoint): pin child-stdio piping behind a regression test, 2026-09-27) which adds TestGitRun_PipesBothChildStdioFds at kit/checkpoint/store_test.go:1475 pinning the property.

resolved_at fields: leave #23/#24 as the tool set them (bookkeeping write time — matches row #29's resolved_at-equals-write-time precedent; the 2026-09-20 discharge date lives in the reasons). Set #30's resolved_at to the commit's true instant `2026-09-27T18:15:27Z` in BOTH the JSON entry and the table cell (git-verified commit time of ed83044 — the actual closure event). Finally bump frontmatter last_updated to the current UTC instant in the module's ISO format (obtain it with node -e printing new Date().toISOString()).

Do not alter any description, recorded_at, id, phase, kind, file, or line values; do not touch any row other than #23/#24/#30.
  </action>
  <verify>
    <automated>R=/home/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/WINDOWS.md; [ "$(grep -c '260920-uat:' "$R")" -ge 4 ] && [ "$(grep -c '260927-close:' "$R")" -ge 2 ] && [ "$(grep -c 'ed83044' "$R")" -ge 2 ] && [ "$(grep -c 'TestGitRun_PipesBothChildStdioFds' "$R")" -ge 2 ] && grep -q '"resolved_at": "2026-09-27T18:15:27Z"' "$R" && grep -E '^\| 30 \|' "$R" | grep -q '2026-09-27T18:15:27Z' && echo TASK2-OK</automated>
  </verify>
  <done>Rows #23/#24 carry 260920-uat discharge reasons and row #30 carries a 260927-close reason citing ed83044 + TestGitRun_PipesBothChildStdioFds — each present in both the fenced JSON and the rendered table; #30 resolved_at is 2026-09-27T18:15:27Z in both surfaces; last_updated refreshed.</done>
</task>

<task type="auto">
  <name>Task 3: Consistency gate — read-validator plus byte-exact table drift check</name>
  <files>.planning/WINDOWS.md</files>
  <action>
Prove the ledger is internally consistent on three axes, using the ledger module's own functions (exported from /home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/bin/lib/broken-windows.cjs) so the gate is exactly as strict as the tooling that consumes the file:

1. Read-path validation: run `windows status` (it invokes parseLedger, the same validator /gsd-ship's window gate runs) — it must exit 0. parseLedger throws on malformed frontmatter, unparseable JSON, invalid entry shapes, and any frontmatter-vs-entries count disagreement.
2. Byte-exact table drift check (the same comparison the write path performs before any future mutation): load the file, parseLedger it, then assert extractTableRegion(raw, ledger.total_count) equals renderTable(ledger.entries). Any hand-edit from Task 2 that broke byte-exactness (spacing, escaping, a surface missed) fails here — fix the offending row/cell to byte-match the rendered form and re-run.
3. Row-level assertions: 30 entries with open 16 / waived 0 / fixed 14; #23/#24/#30 fixed with non-empty reason and resolved_at; #30's reason mentions ed83044 and TestGitRun_PipesBothChildStdioFds; #21/#22/#27 still open (scope fence held).

If either command fails, correct .planning/WINDOWS.md and re-run until both pass — this gate is the plan's acceptance proof, not advisory.
  </action>
  <verify>
    <automated>T=/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/bin/gsd-tools.cjs; R=/home/nil/DiskD/W/Djarvur/ass-guard-agent; node "$T" windows status --cwd "$R" > /dev/null && node -e 'const m=require("/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/bin/lib/broken-windows.cjs");const fs=require("fs");const p="/home/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/WINDOWS.md";const raw=fs.readFileSync(p,"utf8");const L=m.parseLedger(raw);const a=m.extractTableRegion(raw,L.total_count);const e=m.renderTable(L.entries);if(a!==e)throw new Error("TABLE_DRIFT: rendered table disagrees with fenced JSON entries");const n=s=>L.entries.filter(x=>x.status===s).length;if(L.entries.length!==30||n("open")!==16||n("fixed")!==14||n("waived")!==0)throw new Error("counts wrong: "+L.entries.length+"/"+n("open")+"/"+n("fixed")+"/"+n("waived"));const by=i=>L.entries.find(x=>x.id===i);for(const i of[23,24,30]){const r=by(i);if(!r||r.status!=="fixed"||!r.reason||!r.resolved_at)throw new Error("row "+i+" not fixed-with-reason");}if(!by(30).reason.includes("ed83044")||!by(30).reason.includes("TestGitRun_PipesBothChildStdioFds"))throw new Error("row 30 closure narrative incomplete");for(const i of[21,22,27]){const r=by(i);if(!r||r.status!=="open")throw new Error("row "+i+" must remain open");}console.log("OK: ledger consistent — 30 entries, open=16 fixed=14; #23/#24/#30 fixed-with-reason; #21/#22/#27 untouched")' && echo TASK3-OK</automated>
  </verify>
  <done>windows status exits 0 (read path green for /gsd-ship), the rendered table is byte-identical to renderTable of the fenced JSON entries, counts are 16/0/14/30, rows #23/#24/#30 are fixed-with-reason, and rows #21/#22/#27 remain open and untouched.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| planning-docs → ship gate | .planning/WINDOWS.md feeds `workflow.windows_enforce`: /gsd-ship blocks while open_count > 0 and the ledger module fail-closes on malformed input. A corrupted, drifted, or miscounted ledger either lies about project state or bricks the gate. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-UHZ-01 | Tampering | .planning/WINDOWS.md frontmatter counts / fenced JSON / rendered table | medium | mitigate | All structural mutations go through the gsd-tools windows verbs (atomic write, count recompute, both-surface render); hand-edits limited to reason/resolved_at cells in both surfaces; Task 3 gates on parseLedger (the ship gate's own validator) plus a byte-exact renderTable drift check |
| T-UHZ-02 | Tampering (scope creep) | ledger rows #21/#22/#27 | medium | mitigate | Sibling items 261013-uhx/261013-uhy own those rows; Task 3 asserts they remain open with empty reasons, so any out-of-scope status flip fails the plan's acceptance gate |
</threat_model>

<verification>
- `gsd-tools windows status --cwd /home/nil/DiskD/W/Djarvur/ass-guard-agent` exits 0 (the ledger passes the same read validator the ship gate uses).
- The Task 3 node gate passes: table byte-identical to renderTable(entries); counts 16/0/14/30; #23/#24/#30 fixed-with-reason; #21/#22/#27 open.
- grep counts: `260920-uat:` ×4, `260927-close:` ×2, `ed83044` ×2, `TestGitRun_PipesBothChildStdioFds` ×2 (each narrative present in both surfaces).
</verification>

<success_criteria>
- Rows #23 and #24: status fixed, reasons in the dated 260920-uat format citing the 2026-09-20 operator UAT 2/2, the archived evidence file, and the STATE.md Phase 23 entry.
- Row #30 exists (kind deviation, phase 23, file kit/checkpoint/store.go), status fixed, reason in the dated 260927-close format citing commit ed83044, the non-reproduction probe, and TestGitRun_PipesBothChildStdioFds (kit/checkpoint/store_test.go:1475), resolved_at 2026-09-27T18:15:27Z.
- Frontmatter: open_count 16, waived_count 0, fixed_count 14, total_count 30, last_updated refreshed — in agreement with the fenced JSON entries.
- The markdown table is byte-identical to renderTable of the fenced JSON entries (future tool writes and the ship gate proceed without drift errors).
- Rows #21, #22, #27 untouched (still open, empty reasons) — reserved for sibling items 261013-uhx/261013-uhy.
</success_criteria>

<output>
No phase SUMMARY is produced for quick-batch items. On completion, report the ledger deltas (#23/#24/#30 + counts) back to the coordinating quick-batch workflow; the coordinator owns the item status update and any commit.
</output>
