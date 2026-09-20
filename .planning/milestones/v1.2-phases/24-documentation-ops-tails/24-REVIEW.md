---
phase: 24-documentation-ops-tails
reviewed: 2026-09-10T17:52:50Z
depth: standard
files_reviewed: 38
files_reviewed_list:
  - internal/modelrouting/outcomes.go
  - internal/modelrouting/outcomes_agg.go
  - internal/modelrouting/outcomes_test.go
  - internal/modelrouting/dispatch.go
  - internal/modelrouting/breaker.go
  - internal/modelrouting/safety.go
  - internal/modelrouting/dispatch_test.go
  - internal/modelrouting/breaker_test.go
  - internal/modelrouting/safety_test.go
  - .golangci.yml
  - internal/session/session_outcomes_test.go
  - internal/acpserve/config_surface_outcomes_test.go
  - internal/session/session.go
  - internal/session/subagent.go
  - internal/runtime/runtime.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/acp_serve.go
  - internal/modelroutingcmd/modelrouting.go
  - cmd/ass-guard/modelrouting.go
  - cmd/ass-guard/nightly_check.go
  - cmd/ass-guard/profile_check.go
  - cmd/ass-guard/parity.go
  - internal/profilecheckcmd/nightly_check.go
  - internal/profilecheckcmd/nightly_check_test.go
  - profiles/zcode/structure-pin.json
  - .github/workflows/nightly-parity.yml
  - internal/paritycli/parity.go
  - .mise.toml
  - internal/modesmatrix/matrix.go
  - internal/modesmatrix/matrix_test.go
  - internal/acpserve/modesmatrix_interactive_test.go
  - internal/session/modesmatrix_subagent_test.go
  - internal/runtime/modesmatrix_cron_test.go
  - internal/runtime/modesmatrix_wake_test.go
  - internal/ecosys/testdata/modes-matrix/plugin.json
  - internal/ecosys/testdata/modes-matrix/hooks/hooks.json
  - internal/ecosys/testdata/modes-matrix/commands/matrix-echo.md
  - internal/ecosys/testdata/modes-matrix/skills/matrix-skill/SKILL.md
findings:
  critical: 1
  warning: 7
  info: 6
  total: 14
status: issues_found
---

# Phase 24: Code Review Report

**Reviewed:** 2026-09-10T17:52:50Z
**Depth:** standard
**Files Reviewed:** 38 (runtime.go and config_surface.go reviewed via the phase-24 diff hunks — 4.5k/2k-line files with ~360 changed lines; docs/lsp-setup.md and README.md spot-checked for links/claims only, per config)
**Status:** issues_found

## Summary

Phase 24 delivers three tails: the outcome store + live recording + replay-demotion (24-01/24-02), the nightly parity gate + first GitHub Actions workflow (24-04), and the modes-matrix harness (24-05). The store itself is well-built (tolerant read, atomic single-line appends, self-gitignore, per-attempt recording verified end-to-end by real-path tests), the nightly check's exit contract (0/7/1) was verified live in this review (built binary exits 7 and writes the report), and the committed structure pin matches the committed bundle (7/7 digests). Phase-24 tests pass under `-race` in modelrouting, session, profilecheckcmd, modesmatrix, and runtime (the only acpserve failure, TestPermissionsE2E, is the documented pre-existing Phase-23 regression named in the workflow's skip list).

The core defect: the new breaker-demotion logic ignores the provider dimension of a `Target`. `resolveSubagentModel` returns a demoted fallback's **model slug only**, discarding which provider hosts it — so a cross-provider fallback rides the session provider's wire, exactly the "silent wrong-wire" the function's own doc (runtime.go:3130-3136) prohibits. Secondary concerns: the cost-replay half of the deliverable (`ReplayCostTracker`) has no live call site; user-initiated cancellation is recorded as provider-transient evidence; D-15 cross-provider subagent attempts are attributed to the wrong provider in the store; and ~15 files picked up an accidental executable bit this phase.

Key invariants verified: store failures never fail a turn (tested via EISDIR break, one loud note); stdout stays ACP-clean (stats human output → stderr, `--json` → stdout; nightly-check has no stdout mode); expired cooldowns are not resurrected (replay stamps `openedAt` from record timestamps, so `Allow(now)` admits after cooldown); structural/exhausted never feed breakers (replay skips them entirely, pinned by tests); workflow permissions are minimal (`contents: read` top-level, `issues: write` only on the drift job) and the issue body is composed from the report file in JS, never through shell interpolation.

## Structural Findings (fallow)

No structural pre-pass was provided for this phase.

## Narrative Findings (AI reviewer)

### CR-01: Breaker demotion in resolveSubagentModel returns a cross-provider fallback with no same-provider guard — wrong-wire model slug

**File:** `internal/runtime/runtime.go:3170-3180` (with `internal/runtime/runtime.go:4196`, `internal/session/subagent.go:361-363`)
**Issue:** The pre-existing contract of `resolveSubagentModel` (runtime.go:3130-3163) is that the light-tier override applies **only** when the binding is on the session's provider — a different provider means `""` + a loud degrade note, because the returned slug is stamped onto a profile that rides the session provider's wire (`subagentProfile` blindly sets `prof.Model = plan.Model`; the plan at runtime.go:4196 carries `Model` only, no `Provider`). The new 24-02 demotion block calls `FirstAllowed(chain, ...)` over `[primary, fallbacks...]` and returns `pick.Model` for the first breaker-admitted candidate **without re-checking `pick.Provider == sessionProvider`**. Fallback chains are routinely cross-provider (the heavy chain in the test config is glm-5.2[anthropic] → minimax-m3[openai] → glm-4.6[anthropic]), so a replayed-open light primary demotes the subagent onto a model slug the session provider does not host — requests go out on the wrong wire (unresolvable model name, or worse a same-named model with the wrong shape/pricing). This is precisely the "silent wrong-wire" the function documents it must never do; the stderr note fires but does not fix the wire.
**Fix:** Filter the chain to the session provider before the walk, and treat a fully-denied or cross-provider-only chain as "keep the primary":

```go
if len(breakers) > 0 {
	sameProv := []modelrouting.Target{primary}
	for _, fb := range fallbacks {
		if fb.Provider == sessionProvider {
			sameProv = append(sameProv, fb)
		}
	}

	pick, demoted := modelrouting.FirstAllowed(sameProv, breakers, now)
	if demoted && pick.Model != "" {
		// ...note + return pick.Model (same provider by construction)
	}
}
```

## Warnings

### WR-01: Surface demotion advertises a model the live session can never apply (no provider guard)

**File:** `internal/acpserve/config_surface.go:743-795`
**Issue:** `demoteIfDeniedLocked` demotes the advertised/effective model to the first breaker-admitted fallback regardless of which provider hosts it. `applyTargetLocked` (config_surface.go:1324-1330) correctly refuses to live-apply a model bound to a different provider than `s.providerName`, so after a cross-provider demotion the options menu advertises a "current model" that (a) the live session is not using and (b) the editor cannot set — `Set` on that exact advertised value would be skipped as a cross-provider switch. The operator's model option becomes inconsistent with reality until the breaker recovers.
**Fix:** Restrict the demotion walk to candidates on `s.providerName` (same fix shape as CR-01), or fall back to the primary when the first allowed candidate is cross-provider, noting the degradation.

### WR-02: User-initiated cancellation is recorded as OutcomeTransient — pollutes breaker evidence and stats

**File:** `internal/session/session.go:700-701, 1321-1337`
**Issue:** `classifyStreamOutcome` maps any non-`*provider.ProviderError` — including `context.Canceled` from a user Stop / mid-stream cancel — to `OutcomeTransient`. The comment declares this deliberate ("conservative"), but the consequence chain is real: transient records are the only class that feeds breakers on replay (`ReplayBreakers`, outcomes_agg.go:76-96), so N=5 rapid user-cancels of a healthy provider, followed by a restart inside the cooldown window, trip the primary's replayed breaker and demote routing at both Resolve sites. The stats CLI also counts user cancels as provider failures. A cancel is not evidence about the provider.
**Fix:** Before classifying, check `errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded)`-per-attempt-context and either skip the record or record it under a distinct non-breaker class; at minimum exclude `context.Canceled` from the breaker-feeding class.

### WR-03: Cross-provider (D-15) subagent attempts are attributed to the session provider in the store

**File:** `internal/session/session.go:1300` (with `internal/session/subagent.go:205-216`)
**Issue:** `recordDispatchOutcome` always stamps `Provider: s.ProviderName`. The subagent path may dispatch through `plan.Provider` — a different, explicitly built provider (D-15 cross-provider routing, runtime.go:4223) — while still recording `prof.Model`. The evidence row then names (sessionProvider, model) for an attempt that ran on another provider; `ReplayBreakers` keys on exactly that pair, so replay feeds the wrong breaker and `model-routing stats` misreports spend/failures per provider.
**Fix:** Thread the effective provider slug into the record: have `SubagentDispatchPlan` carry the provider slug alongside the `Provider` instance (or derive it at plan time) and pass it to `recordDispatchOutcome` as an override of `s.ProviderName`.

### WR-04: ReplayCostTracker has no live call site — the "cost" half of the replay deliverable is unwired

**File:** `internal/modelrouting/outcomes_agg.go:113-130`
**Issue:** The phase contract describes replay "seeding breakers/cost at both Resolve sites," and 24-01's plan built `ReplayCostTracker` against the `CostTracker` seam with tests. Grep confirms zero production callers (only `safety.go`'s `InstallSafety`, itself test-only — no live composition constructs a `Scheduler` at all). Consequence: windowed spend accumulated before a restart does **not** degrade routing after restart; the exported function is dead code whose existence implies an enforcement that does not happen. (The breaker half is genuinely wired at both Resolve sites; only the cost half is missing.)
**Fix:** Either wire it — the resolution sites return only a model string, so the natural home is a composition-level `SetCostTracker` consumer (or fold a `Check` into the demotion walk) — or mark `ReplayCostTracker` explicitly as reserved-for-future-wiring in its doc comment and record the gap in deferred-items.md so the invariant claim is not read as enforced.

### WR-05: Accidental executable bit (100755) committed on ~15 source files this phase

**File:** `internal/session/session.go`, `internal/session/subagent.go`, `internal/acpserve/config_surface.go`, `internal/acpserve/simulator_e2e_test.go`, `internal/modelrouting/breaker.go`, `internal/modelrouting/dispatch.go`, `internal/modelrouting/safety.go`, `internal/modelrouting/breaker_test.go`, `internal/modelrouting/dispatch_test.go`, `internal/modelrouting/safety_test.go`, `internal/modelroutingcmd/modelrouting.go`, `cmd/ass-guard/modelrouting.go`, `cmd/ass-guard/modelrouting_test.go`, `internal/runtime/apply_model_test.go`, `.golangci.yml` (mode change 100644→100755 in commits 59952f8..HEAD)
**Issue:** The phase commits flipped these files to mode 100755 (verified via `git diff --summary` and `git ls-files -s`); the repo now carries 38 executable files, most of them .go sources and a YAML lint config. It is accidental (nothing executes them), pollutes future diffs, and leaks into release tarballs via goreleaser metadata.
**Fix:** `chmod 644` the affected files and commit the mode change (`git update-index --chmod=-u` path); consider a CI check rejecting mode-755 on non-executable paths.

### WR-06: `--zcode-bin` flag is parsed but never used (pre-existing)

**File:** `cmd/ass-guard/profile_check.go:30`
**Issue:** The flag declares "zcode binary (live path; operator-gated)" but `RunE` calls `profilecheckcmd.RunProfileCheck(args[0], profilesDir, captureFile)` — the value never reaches the live path (which resolves the binary itself). An operator's `--zcode-bin /custom/path` is silently ignored. Pre-existing before this phase, but the file is in scope and the flag is a live contract lie.
**Fix:** Pass it through (`RunProfileCheck` gains a `zcodeBin` parameter with the flag default as fallback) or delete the flag.

### WR-07: `half_open_probes` config knob is parsed, defaulted, and set by tests — but never enforced

**File:** `internal/modelrouting/breaker.go:94-114` (knob defined at `config.go:149`, defaulted at `init.go:30` / `load.go:151-152`)
**Issue:** `CircuitBreaker.Allow` returns true for every caller while `breakerHalfOpen` — there is no probe-count limiting anywhere in breaker.go; `HalfOpenProbes` is never read. Operators configuring `half_open_probes: 3` get silently unlimited concurrent probes, and the phase's own tests (`outcomes_test.go:299`, `breaker_test.go:66`) set the field as if it had effect. Under a burst of concurrent dispatches against a half-open breaker, all of them hit the recovering provider at once — the exact behavior the knob promises to bound.
**Fix:** Either implement probe admission (count admitted-but-unresolved probes in HalfOpen, deny beyond `cfg.HalfOpenProbes` until an outcome resolves) or remove the field from the config surface and tests until it does something.

## Info

### IN-01: Dead import-keeper in breaker_test.go

**File:** `internal/modelrouting/breaker_test.go:260-262`
**Issue:** `var _ = strings.TrimSpace` exists solely to keep the `strings` import alive ("in case of future trim"). It is dead code the linter had to be reasoned around; the import is otherwise unused (assertions use `require.Contains`).
**Fix:** Delete the var and the import; re-add when actually needed.

### IN-02: stats CLI cwd-relative default store path and hard config dependency

**File:** `cmd/ass-guard/modelrouting.go:158-161, 174`
**Issue:** `--store` defaults to `.ass-guard/routing/outcomes.jsonl` relative to the process cwd, but the store is written under the session's WorkDir — running the CLI from any other directory silently prints "no outcomes recorded yet" (documented in help text, still a trap). Separately, `stats` hard-fails via `modelrouting.Load` when the scheduling config is invalid even though the config is only tier-label enrichment; a broken config blocks reading pure store evidence.
**Fix:** Resolve the default against the work-dir discovery the serve path uses, or accept `--work-dir`; downgrade a config load failure to "tier column omitted" with a stderr note.

### IN-03: Replay state asymmetry vs live breaker semantics is undocumented

**File:** `internal/modelrouting/outcomes_agg.go:86-96` (with `internal/modelrouting/breaker.go:118-128`)
**Issue:** Live, a success after a trip can only be recorded from HalfOpen (Allow gates the attempt), and closes the breaker. On replay, `RecordSuccess` from `breakerOpen` does not close (only HalfOpen→Closed is wired), so a store containing [trip … later ok] replays as Open until the next `Allow(now)` transitions it to HalfOpen. Functionally the candidate is still admitted (cooldown long elapsed — the no-resurrection invariant holds), but the replayed machine is one probe more conservative than the live one; the divergence is defensible and undocumented.
**Fix:** One sentence in the `ReplayBreakers` doc comment noting the asymmetry and why it is acceptable (or replay-close on a success newer than `openedAt`).

### IN-04: MountRealPlugin/copyTreePreservingMode trust the operator tree more than needed

**File:** `internal/modesmatrix/matrix.go:452-491, 552-570`
**Issue:** `copyTreePreservingMode` follows symlinked entries (`os.ReadFile` resolves them; only dirs recurse), copying arbitrary out-of-tree content into the temp project, and `MountRealPlugin` silently replaces an `installed_plugins.json` that fails to parse (comment says "tolerate and replace"). Test-only code over an operator-supplied path, so impact is bounded, but the copy is described as read-only-disciplined while a symlink escapes the stated boundary.
**Fix:** Skip or explicitly reject symlink entries (`e.Type()&fs.ModeSymlink != 0`) and log loudly when the registry is replaced rather than merged.

### IN-05: Duplicate, independently-timed replay plumbing in one process

**File:** `internal/acpserve/acp_serve.go:36-61, 314` and `internal/runtime/runtime.go:3746-3773`
**Issue:** The surface replays the store at serve startup; the Runner lazy-replays at the first subagent dispatch. Two stateful breaker maps are built from the same file at different times — outcomes appended in between are reflected in one map and not the other, and each map mutates independently on `Allow`. Behavior is currently benign (both demote conservatively), but it is two copies of one concept with a divergence window.
**Fix:** Replay once in `Run` and hand the same map to both consumers (the Runner gains an injectable map, mirroring `SetOutcomeBreakers`).

### IN-06: Nightly version gate is an exact string match between two differently-sourced strings

**File:** `internal/profilecheckcmd/nightly_check.go:194-199`
**Issue:** Parity requires `probeOutput == pin.ZcodeVersion` where the left side is the trimmed stdout of `zcode --version` and the right side is `meta.yaml`'s `zcode_version`. If the binary's output format ever differs from the manifest's recording convention (e.g. `zcode 0.16.3` vs `0.16.3`), every nightly run reports drift (exit 7) and opens an issue — a permanent false alarm the workflow cannot distinguish from real drift. Not provable without the live binary (absent in this environment), but the comparison has no normalization and the failure mode is noisy.
**Fix:** Normalize both sides (extract a leading/trailing semver via regexp) before comparing, or compare against a version recorded through the same probe at pin time.

---

_Reviewed: 2026-09-10T17:52:50Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
