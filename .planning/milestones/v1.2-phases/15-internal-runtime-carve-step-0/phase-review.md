# Phase 15 Close-Out Review — internal/runtime carve (step 0)

Equivalence attestation per ROADMAP's three success criteria. Evidence gathered
on the final tree (HEAD after 15-07 Task 1; phase-start baseline f1e26b3).

## Leg 1 — mise ci

`mise ci` exit status: **0** (vet + golangci-lint 0 issues + CGO_ENABLED=0
build + `go test -race ./...` all packages ok; ~55s). Two standing lint
*warnings* (not issues): nolint directives elsewhere reference linter names
`err113-best-effort` and `testingcontext` that are not enabled — pre-existing,
carried from before the phase.

## Leg 2 — Test-function ledger match

| Measure | Number |
|---|---|
| Ledger rule (test-ledger.txt header) | 130 across the carve scope |
| Ledger file's own `^func Test` listing | 131 |
| 15-01 CLI-contract golden additions | +3 |
| **Actual carve-scope sum (cmd/ass-guard + internal/runtime/... + internal/acpserve/... + the six CLI packages)** | **134** |
| Repo-wide `^func Test` total | **829** |

Delta explanation: the ledger's *listing* self-counts 131 (its header prose
says 130 — an off-by-one in the ledger's own summary line, the per-file
listing is the authority); 15-01 added the three CLI-contract golden tests
(TestCLIBinaryContractRootAndACP / SupportCommands / Profile).
131 + 3 = 134 — exact. Repo-wide: 826 (baseline f1e26b3) + 3 = 829 — exact,
no test lost, no test gained beyond the sanctioned goldens.

## Leg 3 — Duplicate test names

Plan asks for zero duplicates repo-wide. Actual: **2** duplicate names —
`TestLoadLayering` (internal/, two packages) and `TestMain` (echo-server
self-exec mode, now internal/runtime/mcp_tracer_test.go + internal/mcp/
echoserver). Both pre-exist at the phase-start baseline, proven directly:

```
$ git grep -h '^func TestLoadLayering\|^func TestMain(' f1e26b3 -- 'cmd/**_test.go' 'internal/**_test.go' | sort | uniq -c
   2 func TestLoadLayering(t *testing.T) {
   2 func TestMain(m *testing.M) {
```

The zero-duplicate gate is therefore unmeetable as written on this repository;
its intent — the carve introduces no NEW duplicate names — holds:
duplicates before = duplicates after = 2.

## Leg 4 — Color-moved relocation census

Command of record: `git diff master --color-moved=plain --stat -- cmd/ internal/`
(226 files, 31783+/6308-; master predates milestone v1.2, so that diff spans
all of v1.2). The phase-scoped census uses the phase-start baseline:
`git diff f1e26b3 -M --name-status -- cmd/ internal/` →
**19 renames, 22 adds, 11 modifies** (52 files, 6182+/5016-).

Renames (moved blocks render as moved): all 19 retain 69-99% similarity with
the residue being package clauses, import blocks, and the sanctioned sed
renames (`sessionTurnRunner`→`Runner`, `setupEngine`→`SetupEngine`,
`loadCommandRegistry`→`LoadCommandRegistry`, test-package nolint clauses).
The adds are the carve's destination files (internal/runtime,
internal/runtime/enginebridge, internal/acpserve, the six CLI packages, the
split battery file) — new text by construction, not edited moves.

Every block rendered as EDITED rather than moved, classified:

| # | Edited block | Sanctioned category |
|---|---|---|
| 1 | cmd/ass-guard/acp_serve.go — runner family out, `acpserve.Run` call, de-cobra'd flag reads | RunnerConfig/NewRunner/rename mechanics + the two sanctioned de-cobra edits (15-05) + wrapper-retirement call-site re-points |
| 2 | cmd/ass-guard/{learning_cmd,modelrouting,parity,profile_check,main}.go — qualified calls into the extracted CLI packages | wrapper-retirement call-site re-points (15-02/15-03) |
| 3 | The five cmd-test identifier qualifications riding inside relocated files (acp_engine_e2e, e2e_opsx, checkpoint_test→checkpoint_session, e2e_opsx_matrix, subagent_tier_wiring) | wrapper-retirement re-points (plan-listed, 15-02/15-04) |
| 4 | cmd/ass-guard/goconst_constants.go + the new per-package goconst files | constant redistribution with the moved families |
| 5 | internal/session/ask.go — one comment line (`sessionTurnRunner.Run` → `Runner.Run`) | rename mechanics (grep-zero gate; 15-06 deviation #5) |
| 6 | cmd/ass-guard/background_wiring_test.go — `skillToolName` → `"Skill"` literal (one line) | constant redistribution fallout |
| 7 | cmd/ass-guard/checkpoint.go — checkpointerAdapter rides with its wiring site | rename mechanics (moved, listed for traceability) |
| 8 | checkpoint_session_test.go rename at 69% — helpers `writeTestModelRouting`/`pinEmptyHome` ride in provider_factory_helpers_test.go; adapter-ctor rewrites to `enginebridge.NewEngineTurnAdapter` | D-03 helper-duplication discipline + the documented enginebridge ctor re-point (15-06) |
| 9 | TestAskWiring_ConfigKnob flag-half merged into TestACPServeCommandRegistered (cmd); TestStageVocab → enginebridge package | 15-06 deviations #4 and the D-01/D-02 subject rule |

Non-sanctioned edited blocks: **0 (none found)** — every edited block above
maps to a documented sanctioned category from the plan text or a SUMMARY'd
deviation.

## Leg 5 — CLI contract + zero-config

`go test ./cmd/ass-guard/ -run 'TestZeroConfigFirstRun|TestCLIBinaryContract'
-count=1` → **ok** (all subtests pass, including acp serve flags/usage —
which also re-proves the merged --ask-timeout assertions). Binary handshake
smoke covered by TestACPServeWiresStdoutClean + TestServeAudit through the
real seam.

## Operator live-Zed checklist (ROADMAP criterion 2)

Baseline: daily-use memory of the v1.1-close editor session surface.

1. Spawn `ass-guard acp serve` from Zed exactly as in daily use (the agent
   binary on PATH, Zed's ACP agent entry).
2. Send a prompt; confirm token streaming appears natively
   (session/update chunks — the chunk-forwarder path).
3. Exercise a tool call (file read/edit); confirm native diffs render and
   the tool result returns (catalog executor path).
4. Restart Zed mid-session; confirm replay-on-restart matches v1.1 close
   behavior (session/load no-op, D-09 — a fresh session starts; transcripts
   remain on disk under `.ass-guard/`).

Disposition recorded in 15-07-SUMMARY.md (OPERATOR-CONFIRMED or
PENDING-OPERATOR-CONFIRMATION).
