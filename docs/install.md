# Installing ass-guard

ass-guard ships as a single static Go binary (macOS + Linux, amd64 + arm64)
with **zero runtime dependencies** — no shared libraries, no system timezone
database needed (IANA `time/tzdata` is bundled). Every default the agent needs
to run a working OpenSpec workflow is embedded in the binary and materialized
on first launch.

## Quick start (team install)

### 1. Download the release archive

Grab the archive matching your OS and architecture from the
[releases page](https://github.com/Djarvur/ass-guard-agent/releases):

| OS | Arch | Archive |
|----|------|---------|
| macOS (Apple Silicon) | arm64 | `ass-guard_<version>_darwin_arm64.tar.gz` |
| macOS (Intel) | amd64 | `ass-guard_<version>_darwin_amd64.tar.gz` |
| Linux | arm64 | `ass-guard_<version>_linux_arm64.tar.gz` |
| Linux | amd64 | `ass-guard_<version>_linux_amd64.tar.gz` |

Each archive bundles the `ass-guard` binary, this project's `agent.json`
manifest, and the `README.md`.

### 2. Extract onto PATH

```sh
tar -xzf ass-guard_<version>_<os>_<arch>.tar.gz
chmod +x ass-guard
sudo mv ass-guard /usr/local/bin/   # or anywhere on your PATH
ass-guard --version                  # sanity check (prints to stderr)
```

### 3. Add it to Zed via the ACP registry

Open a project in [Zed](https://zed.dev), open the command palette, and run
**`zed: acp registry`** (the ACP registry browser). Select **ass-guard**. Zed
records the spawn configuration and lists ass-guard as an available agent.

The registry manifest (`agent.json`) declares the spawn with the canonical ACP
schema fields:

- `cmd`: `ass-guard`
- `args`: `["acp", "serve"]`

Equivalently, Zed spawns `ass-guard acp serve` as a subprocess in the opened
project's working directory (the manifest deliberately carries no `cwd` field —
that is implicit). There is no `command` / `command_args` / `cwd` field; the
spawn is `cmd` + `args`, not the superseded names.

### 4. First launch is zero-config

On first launch in a project, ass-guard detects that `.ass-guard/` does not
exist and seeds it with the embedded defaults (DIST-03):

- `profiles/zcode/` — the pre-seeded zcode mimicry profile (system prompts,
  tool catalog, identity headers, thinking + tool_choice).
- `openspec.toml` — the OpenSpec handoff patterns skeleton.
- `config.yaml` — the default model config (GLM-5.3 via the Z.ai
  Anthropic endpoint, the pinned zcode capture's wire slug; glm-5.2 stays
  as the declared heavy fallback).
- `.gitignore` — a self-gitignoring file (`*\n!.gitignore\n`) so the seeded
  tree does not pollute your project's git status.

The model scheduling config resolves in two operator layers on top of the
embedded default (later layers win on conflict):

1. the **embedded default** — the seeded `.ass-guard/config.yaml` content,
   always the floor;
2. the **global layer** — `~/.config/ass-guard-agent/config.yaml`, your
   machine-wide operator config;
3. the **project layer** — `<project>/.ass-guard/config.yaml`, seeded on
   first run and yours to edit (it overrides the global layer on conflict).

The YAML schema is scheduler-only (`providers` / `models` / `tiers` /
`time_windows` / `circuit_breaker` / `cost_ceiling`). Because the config may
carry a literal `api_key`, ass-guard warns at startup when either layer's
file is looser than `chmod 0600`.

Seeding is **non-clobbering**: if `.ass-guard/` already exists (including a
partial or operator-customized one), ass-guard leaves it untouched. Your edits
survive across restarts. The first-run decision is logged to stderr only
(stdout stays byte-clean for ACP frames).

The agent is immediately ready to run an OpenSpec workflow — no flags, no
config files, no network on first run. A live model turn still requires
`ZAI_API_KEY` (export it in your environment); the defaults + ACP handshake do
not.

## Listing ass-guard in the official ACP registry

The `agent.json` at the repo root is the manifest a team uses locally. To make
ass-guard discoverable via `zed: acp registry` for everyone (sourced from the
CDN at `https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`),
submit it to the community registry:

1. Fork the [agentclientprotocol/registry](https://github.com/agentclientprotocol/registry)
   repository on GitHub.
2. Create a directory named after the agent id: `ass-guard/` (lowercase,
   hyphens — matches the manifest `id`).
3. Add the release-generated `agent.json` (run `scripts/gen-agent-json.sh` with
   the release `VERSION` + `CHECKSUMS_FILE` so the per-target `sha256` fields
   are filled) and an `icon.svg` (16×16 recommended, optional).
4. Open a pull request against `agentclientprotocol/registry`.

Once merged, clients that fetch the registry CDN discover ass-guard
automatically; `zed: acp registry` lists it without any local setup.

## Building from source (dev / snapshot)

For local development or a from-source snapshot build, use
[goreleaser](https://goreleaser.com) (v2.17+):

```sh
goreleaser build --snapshot --clean   # 4 binaries under dist/, no token needed
```

The `.goreleaser.yml` is the single source of truth for the build matrix (4
targets: darwin/linux × amd64/arm64, `CGO_ENABLED=0`). The produced binaries are
statically linked, version-injected via ldflags (`internal/version.Version`),
and zero-config-first-run-ready.

## See also

- [`agent.json`](../agent.json) — the ACP registry manifest.
- [`.goreleaser.yml`](../.goreleaser.yml) — the build matrix.
- [`scripts/gen-agent-json.sh`](../scripts/gen-agent-json.sh) — regenerates
  `agent.json` with the release version + per-target sha256.
- [`internal/defaults/`](../internal/defaults) — the embedded zero-config
  defaults (D-01).
- [`internal/firstrun/`](../internal/firstrun) — the first-run seeding flow
  (D-04).
