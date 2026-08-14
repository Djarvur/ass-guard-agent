# opsx-real fixture

This tree is generated from REAL `openspec init --tools claude` output — not a
hand-rolled lookalike (CMD-01). It pins the exact command layout the loader must
discover (`.claude/commands/opsx/*.md`, one-level namespaced) plus one generated
skill.

## Regenerate

```sh
WORK=$(mktemp -d) && mkdir -p "$WORK/proj" && cd "$WORK/proj" \
  && openspec init --tools claude --force
cp "$WORK/proj/.claude/commands/opsx/"*.md internal/ecosys/testdata/opsx-real/.claude/commands/opsx/
cp "$WORK/proj/.claude/skills/openspec-explore/SKILL.md" internal/ecosys/testdata/opsx-real/.claude/skills/openspec-explore/
```

- Probe date: 2026-08-14
- Binary version: `openspec --version` → 1.5.0 (`/usr/local/bin/openspec`)

The env-gated test `TestOpsxFixtureMatchesRealInit` (runs only with
`ASSGUARD_OPENSPEC_BIN=1` and the binary on PATH) re-runs the init and asserts
the committed command set equals the generated set, so this fixture cannot
silently rot across openspec upgrades.
