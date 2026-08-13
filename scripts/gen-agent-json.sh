#!/usr/bin/env bash
# gen-agent-json.sh — regenerate agent.json, the ACP registry manifest (D-03).
#
# The manifest declares how Zed spawns ass-guard via `zed: acp registry`. It uses
# the CANONICAL ACP schema (https://raw.githubusercontent.com/agentclientprotocol/
# registry/main/agent.schema.json): top-level id/name/version/description/
# distribution, with spawn declared per-target as distribution.binary.<target>
# .{archive, cmd, args, sha256}. There is NO command/command_args/cwd field —
# those names in the original D-03 wording were superseded (Tier-A correction,
# T-06-05). cmd maps the spawn binary; args maps command_args; cwd is dropped
# (Zed runs the agent in the opened project's working directory implicitly).
#
# Usage (release): VERSION=v1.0.0 CHECKSUMS_FILE=dist/ass-guard_v1.0.0_checksums.txt \
#                  ./scripts/gen-agent-json.sh
# Usage (snapshot/dev): ./scripts/gen-agent-json.sh   # writes 0.0.0-dev placeholders
#
# Env:
#   VERSION        semver (default 0.0.0-dev — the committed dev manifest)
#   CHECKSUMS_FILE goreleaser checksums file (sha256sum format); optional
#   OUT            output path (default agent.json, relative to repo root)
set -euo pipefail

VERSION="${VERSION:-0.0.0-dev}"
CHECKSUMS_FILE="${CHECKSUMS_FILE:-}"
OUT="${OUT:-agent.json}"

export VERSION CHECKSUMS_FILE OUT

exec python3 - "$VERSION" "$CHECKSUMS_FILE" "$OUT" <<'PYEOF'
import json
import os
import sys

version = sys.argv[1]
checksums_file = sys.argv[2] if len(sys.argv) > 2 and sys.argv[2] else ""
out = sys.argv[3] if len(sys.argv) > 3 else "agent.json"

repo = "Djarvur/ass-guard-agent"
base = f"https://github.com/{repo}/releases/download/v{version}"

# ACP-schema target key → goreleaser os_arch (the archive filename component).
# The schema uses aarch64/x86_64 naming; goreleaser uses arm64/amd64.
TARGETS = {
    "darwin-aarch64": "darwin_arm64",
    "darwin-x86_64": "darwin_amd64",
    "linux-aarch64": "linux_arm64",
    "linux-x86_64": "linux_amd64",
}

# Parse goreleaser's checksums file (sha256sum format: "<hex>  <filename>").
shas = {}
if checksums_file and os.path.isfile(checksums_file):
    with open(checksums_file, encoding="utf-8") as f:
        for line in f:
            parts = line.split()
            if len(parts) >= 2:
                shas[parts[1].split("/")[-1]] = parts[0]

binary = {}
for target, os_arch in TARGETS.items():
    archive = f"ass-guard_{version}_{os_arch}.tar.gz"
    entry = {
        "archive": f"{base}/{archive}",
        "cmd": "ass-guard",
        "args": ["acp", "serve"],
    }
    if archive in shas:
        entry["sha256"] = shas[archive]
    binary[target] = entry

manifest = {
    "id": "ass-guard",
    "name": "ass-guard",
    "version": version,
    "description": "Mimicry-first ACP agent — structurally indistinguishable outgoing model requests",
    "repository": f"https://github.com/{repo}",
    "license": "MIT",
    "website": f"https://github.com/{repo}",
    "distribution": {"binary": binary},
}

with open(out, "w", encoding="utf-8") as f:
    json.dump(manifest, f, indent=2, ensure_ascii=False)
    f.write("\n")

filled = len(shas)
print(f"wrote {out} (version={version}, sha256-filled={filled})", file=sys.stderr)
PYEOF
