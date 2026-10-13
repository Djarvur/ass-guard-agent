---
phase: quick-261013-uhy
plan: 01
type: execute
wave: 2
depends_on: ["261013-uhx"]
files_modified:
  - kit/runtime/fakes_test.go
  - .github/workflows/nightly.yml
autonomous: true
must_haves:
  truths:
    - "The rescan concurrency stress test (TestRescanConcurrency, kit/runtime/rescan_test.go) passes under -race repeatedly — three consecutive -count runs — at the plan head"
    - "Every fakeCatalog accessor that reads or writes a canned table holds t.mu; the one remaining unguarded shared-state reader (Hooks read vs the setHooks writer) is closed per the ab7956b fakeLearned shape"
    - "The nightly -race leg runs the stress test again: the -skip regex and the reasons comment no longer name it, and every still-failing documented entry keeps its skip and its stated reason"
  artifacts:
    - "kit/runtime/fakes_test.go with the Hooks accessor mutex-guarded (t.mu) and the guard contract recorded in the fake's doc comment"
    - ".github/workflows/nightly.yml with the stress-test token removed from the -skip regex and the pre-existing-race clause dropped from the reasons comment"
  key_links:
    - "spawnMCP (kit/runtime/runtime.go:3073) -> liveCatalog().MCPLayers() -> fakeCatalog.MCPLayers (t.mu-guarded) — the read half of the original race, synchronized since the Phase-25 catalog injection"
    - "rescanAndSwap (kit/runtime/rescan.go:306) -> liveCatalog().Rediscover() (t.mu-guarded) -> installCatalog -> chainPtr.Store — the write half, synchronized + atomically swapped"
    - ".github/workflows/nightly.yml:69 -skip regex -> the entry this item discharges (WINDOWS #27; its nightly-parity.yml name is stale — the real committed file is nightly.yml)"
---

<objective>
Close out the TestRescanConcurrency data race (WINDOWS #21) and unskip the test in the nightly parity gate (WINDOWS #27).

Planning-time ground truth (verified at master): the ORIGINAL race pair — spawnMCP reading the runner's MCP server set vs installRegistry writing it — no longer exists in kit form. Phase 25's D-17 catalog injection moved that state into the injected catalog: the read half is fakeCatalog.MCPLayers (t.mu-guarded), the write half is fakeCatalog.Rediscover (t.mu-guarded), and the chain swap is atomic (chainPtr.Store). Empirically green at master: five consecutive -race runs of the stress test plus one full-package -race run (46s). What remains of the race FAMILY in the test fakes is exactly one unsynchronized accessor — fakeCatalog.Hooks reads the hooks field while setHooks writes it under the mutex (a locked writer against an unlocked reader is still a data race when concurrent). This plan closes that gap following the sanctioned ab7956b shape (commit ab7956b, "mutex-guard the fakeLearned test fake" — mutex on the fake, never a production change), proves the stress test sustained-green under -race, and removes its entry from the nightly skip list.

Purpose: the hosted nightly -race leg currently skips the concurrency stress test, so the drift gate is blind to exactly the scenario the test exists to prove (Pitfall-1: concurrent resolution against discovery churn).
Output: guarded test fake + nightly skip-list entry removed.

Wave note: depends_on 261013-uhx is declared because sibling uhx's plan gates hardcode this test's entry being PRESENT in nightly.yml during its own edit ("the regex then begins with the TestRescanConcurrency token"; "exactly the five remaining documented entries") — only uhx-before-uhy keeps both plans' verify gates satisfiable. Both items edit .github/workflows/nightly.yml; this plan's diff stays narrow to its own entry.
</objective>

<execution_context>
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/workflows/execute-plan.md
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/templates/summary.md
</execution_context>

<context>
@kit/runtime/rescan_test.go
@kit/runtime/fakes_test.go
@.github/workflows/nightly.yml

Precedent: commit ab7956b (kit/runtime/engine_setup_test.go) — the sanctioned fix shape for a test-fake data race: add the mutex field, lock at every accessor head with defer unlock, no production change, validated by the existing suite under the race detector (no new test).

Item-text path correction: the item says internal/runtime/rescan_test.go; after the Phase-25 kit extraction the test lives at kit/runtime/rescan_test.go (internal/runtime/ no longer exists). Same for the original race report's installRegistry — renamed/restructured into SetCatalog/installCatalog + the catalog adapter's Rediscover (kit/runtime/commands.go:494, kit/runtime/rescan.go:306).
</context>

<tasks>

<task type="auto">
  <name>Task 1: Close the fakeCatalog mutex sweep — guard the Hooks reader (ab7956b shape)</name>
  <files>kit/runtime/fakes_test.go</files>
  <action>In kit/runtime/fakes_test.go, the fakeCatalog receiver set has exactly ONE remaining accessor that touches shared state without t.mu: Hooks reads the hooks field while setHooks writes it under the mutex — unsynchronized conflicting access whenever the two run concurrently (same family as the nightly race this item discharges). Guard it exactly like the ab7956b fakeLearned precedent: t.mu.Lock() + defer t.mu.Unlock() at the method head, nothing else in the method changes.

Audit, don't assume: sweep every other receiver method on fakeCatalog in this file and confirm t.mu coverage before declaring the sweep complete. The planning-time audit result: Rediscover, Entries, LookupCommand, SkillBody, Mutating, Agents, Counts, MCPLayers, SkillListing, AgentListing, SkillExecute, plantAgent, plantSkills, skill, agent, and setHooks are already guarded; MemoryInjection and MemoryFiles read only the filesystem (no shared maps) and stay lock-free DELIBERATELY — do not add locks there. Record the contract in the fakeCatalog doc comment with one sentence: every accessor touching the canned tables holds t.mu; the memory lenses are FS-only by design.

Add no new tests — the ab7956b precedent validated through the existing suite under the race detector, and so does this task. TDD heuristic ruling: test-fake synchronization is test glue, not business logic with defined I/O — standard task, no behavior block.

Hard scope rule: do NOT touch any production file. The original production pair is structurally gone (see objective); if any production edit starts to look necessary, stop — that contradicts the verified ground truth and the item's sanctioned fix shape.</action>
  <verify>
    <automated>test -z "$(gofmt -l kit/runtime/)" && go vet ./kit/runtime/ && go test -race -count=1 ./kit/runtime/</automated>
  </verify>
  <done>Every fakeCatalog accessor that reads or writes a canned table holds t.mu (Hooks included); gofmt clean, go vet clean, full kit/runtime package green under -race.</done>
</task>

<task type="auto">
  <name>Task 2: Prove the stress test sustained-green under -race, then unskip it in the nightly gate (WINDOWS #27)</name>
  <files>.github/workflows/nightly.yml</files>
  <action>Proof first: run the rescan concurrency stress test three consecutive times under the race detector — this is the acceptance evidence for unskipping (the test is the Pitfall-1 proof: sustained concurrent resolutions against a discovery-churn loop, with a mid-session agent that must stay dispatchable).

Then edit .github/workflows/nightly.yml (the real committed file — nightly-parity.yml in the item text and WINDOWS #27 is a stale path; do not create or hunt for it). Sibling item 261013-uhx lands first (declared dependency) and has already removed its own entry from the -skip regex at line 69 and from the reasons comment; re-read the file at execution time and work against its current text. Two touch points, nothing else: (1) remove the rescan-concurrency stress-test alternation token from the -skip regex; (2) in the reasons comment above it, drop the clause citing that test as a pre-existing race and reflow so the remaining entries' reasons read coherently, keeping the trailing REMOVE-entries-as-failures-are-fixed instruction intact. Leave the remaining documented entries (the acpserve simulator pair, the suite battery needing the openspec binary, the escalation full-suite flake) and every other line byte-identical. Do not touch .planning/WINDOWS.md — sibling item 261013-uhz owns the ledger, and no row discharge is assigned to this item.</action>
  <verify>
    <automated>go test -race -count=3 -run '^TestRescanConcurrency$' ./kit/runtime/ && test "$(grep -v '^[[:space:]]*#' .github/workflows/nightly.yml | grep -c 'RescanConcurrency')" -eq 0 && grep -q 'TestRunSuite_' .github/workflows/nightly.yml && ruby -ryaml -e 'YAML.load_file(".github/workflows/nightly.yml")' && echo NIGHTLY-UNSKIP-OK</automated>
  </verify>
  <done>The stress test passes -race -count=3; the nightly -skip regex no longer names it and the reasons comment no longer cites it; the other documented entries survive verbatim; git diff of the file shows only the regex token removal plus the comment reflow; the workflow parses as YAML.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| executor edits → .github/workflows/nightly.yml | the hosted drift gate's skip list — an over-broad edit silently ungates known-broken tests or breaks the workflow |
| test fake → production seams | the fake implements the kit CommandCatalog port; the fix must stay test-side and additive |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-uhy-01 | Tampering | nightly.yml -skip regex edit | low | mitigate | Removes exactly one entry whose underlying race the plan proves fixed (-count=3 under -race); the verbatim-survival grep on a remaining entry, the YAML-parse gate, and the narrow git-diff done criterion prevent over-removal or a malformed edit |
| T-uhy-02 | Denial of Service | kit/runtime test fakes | low | accept | Additive mutex on a test fake cannot change production behavior; the full-package -race gate (Task 1 verify) catches any regression the guard could introduce |
| T-uhy-SC | Tampering | package installs | none | accept | No package-manager installs in this item's scope — no supply-chain surface |
</threat_model>

<verification>
- Task 1 gates: gofmt clean on kit/runtime/, go vet clean, `go test -race -count=1 ./kit/runtime/` green.
- Sustained proof: `go test -race -count=3 -run '^TestRescanConcurrency$' ./kit/runtime/` green.
- Skip-list gates: the test's name absent from nightly.yml (comment-filtered count 0); a remaining documented entry still present; workflow parses as YAML via ruby.
- Scope: `git diff --name-only` at plan end shows only the two declared files (kit/runtime/fakes_test.go, .github/workflows/nightly.yml) — zero production files touched.
</verification>

<success_criteria>
The item's full letter is discharged: the race family behind the skipped test is closed in the test fake per the sanctioned mutex-guard shape (no production change — the original pair was already structurally removed by the Phase-25 catalog injection), the stress test is proven sustained-green under -race, and its entry is gone from the nightly skip list so the hosted drift gate runs the concurrency scenario again (WINDOWS #21/#27 pressure released for this test).
</success_criteria>

<output>
Create `.planning/quick/261013-uhy-fix-the-testrescanconcurrency-data-race-in-internal-runtime/261013-uhy-SUMMARY.md` when done
</output>
