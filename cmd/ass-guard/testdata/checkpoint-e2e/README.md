# Checkpoint live rollback demonstration (EARLY-01, plan 14-01 Task 3)

The gated live leg proving the no-confirmation-tier agent's reversibility
backstop END TO END: a real model turn mutates a scratch git repo through the
FULL serve path (`sessionFor` → `Prompt` → real tool execution), then
`checkpoint restore` returns the workspace to byte-identical pre-turn state
with the user repo's `.git` untouched.

## What the test proves (all asserted in `TestCheckpointLiveRollback_Gated`)

1. **Serve wiring, default ON** — `sessionFor` opens the shadow store at
   `<workDir>/.ass-guard/checkpoints/shadow.git` and wires
   `Session.Checkpointer`; a store-open failure degrades loudly to a session
   WITHOUT checkpointing (never a serve refusal — the AUD-03 discipline).
2. **Snapshot at turn entry** — the turn's checkpoint is taken BEFORE any
   turn work, so restore means "undo this turn".
3. **A real mutating turn** — the model creates
   `<scratch>/notes-from-agent.md` via the Write tool.
4. **Byte-identical restore** — after `Store.Restore` of the turn's
   checkpoint, the FULL recursive workspace tree (paths + per-file bytes,
   `.ass-guard/` excluded) equals the pre-turn tree; the model-created file
   is GONE.
5. **The user's repo git state is untouched** — `.git/HEAD` bytes,
   `.git/index` bytes, and `git status --porcelain` output are identical
   before the turn and after the restore (the core EARLY-01 invariant).

## Environment gates (loud skip when absent)

```sh
export ZAI_API_KEY=<GLM Coding Plan key>        # real model creds
export ASSGUARD_CHECKPOINT_E2E=1                # opt into the live leg
```

Provider credentials resolve through the same layered
`config.yaml`/`ZAI_API_KEY` factory as every other gated E2E
(`setupProviderFactory` against the repo root).

## Run

From the repo root:

```sh
ASSGUARD_CHECKPOINT_E2E=1 go test ./cmd/ass-guard/... \
  -run TestCheckpointLiveRollback_Gated -count=1 -v
```

Expected: PASS with a final log line recording the evidence paths — the
scratch session's transcript and the restored ref, e.g.

```
checkpoint live evidence: transcript=<scratch>/.ass-guard/<session>.jsonl \
  restoredRef=refs/checkpoints/sess-ckpt-live-turn-001 entries=1
```

Ungated (`ASSGUARD_CHECKPOINT_E2E=0` or no key): the test SKIPs loudly,
naming BOTH env vars (exit 0).

## Operator terminal surface (the manual equivalent)

```sh
ass-guard checkpoint list --work-dir <workspace>
ass-guard checkpoint restore <sessionID-turn-NNN> --work-dir <workspace>
```

All CLI output goes to stderr; the store lives entirely under
`.ass-guard/checkpoints/`; the user's repository is never touched.

## Offline proof that stays green without gates

The store's invariant battery (`internal/checkpoint/store_test.go`) proves
byte-identity, user-git-untouched (snapshot AND restore), ordering,
empty-turn commits, idempotency, interruption safety, concurrency, retention,
perms, and ref-validation without any model: the gated leg above adds only
the REAL-turn dimension.
