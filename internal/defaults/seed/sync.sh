#!/usr/bin/env bash
# sync.sh — regenerate internal/defaults/seed/ from canonical sources (T-06-01).
#
# The seed tree is embedded into the binary via go:embed (internal/defaults).
# Run this script after profiles/zcode or the scheduler default changes to keep
# the embedded seed in sync. The seed files themselves are committed (go:embed
# needs them at build time).
#
# Security: this script is the build-operator environment-leak barrier (T-06-01,
# HIGH). The dev profiles/zcode/ carries the operator's absolute paths in
# system/block-2.txt, meta.yaml, and coverage.yaml. This script copies ONLY the
# loader-required files + meta.yaml, sanitizes the absolute paths to placeholders
# / tilde-relative forms, and EXCLUDES coverage.yaml (a dev-only drift artifact
# with 6 absolute /Users/ paths the loader never reads). A trailing leak guard
# fails loudly if any absolute home path survives.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
SEED="$SCRIPT_DIR"

# (a) zcode profile — copy + sanitize build-operator paths (T-06-01).
ZCODE_SRC="$REPO_ROOT/profiles/zcode"
ZCODE_DST="$SEED/profiles/zcode"

mkdir -p "$ZCODE_DST/system"

# Copy the loader-required files (loader.go) + meta.yaml (target_capture_ref).
# coverage.yaml is intentionally EXCLUDED (dev-only drift artifact, 6 abs paths).
cp "$ZCODE_SRC/profile.yaml"      "$ZCODE_DST/profile.yaml"
cp "$ZCODE_SRC/tools.json"        "$ZCODE_DST/tools.json"
cp "$ZCODE_SRC/identity.yaml"     "$ZCODE_DST/identity.yaml"
cp "$ZCODE_SRC/thinking.json"     "$ZCODE_DST/thinking.json"
cp "$ZCODE_SRC/tool_choice.json"  "$ZCODE_DST/tool_choice.json"
cp "$ZCODE_SRC/meta.yaml"         "$ZCODE_DST/meta.yaml"
cp "$ZCODE_SRC/system/block-0.txt" "$ZCODE_DST/system/block-0.txt"
cp "$ZCODE_SRC/system/block-1.txt" "$ZCODE_DST/system/block-1.txt"

# block-2.txt carries the operator's absolute working directory in the
# "Primary working directory" line — sanitize to a placeholder before embedding.
sed 's#/Users/nil/DiskD/W/Djarvur/ass-guard-agent#<working_directory>#g' \
	"$ZCODE_SRC/system/block-2.txt" > "$ZCODE_DST/system/block-2.txt"

# meta.yaml target_capture_ref.sessions[].path is the operator's home rollout
# dir — make it tilde-relative (no /Users/ leak).
sed 's#/Users/nil/.zcode/cli/rollout/#~/.zcode/cli/rollout/#g' \
	"$ZCODE_DST/meta.yaml" > "$ZCODE_DST/meta.yaml.tmp" && mv "$ZCODE_DST/meta.yaml.tmp" "$ZCODE_DST/meta.yaml"

# (b) scheduling.yaml — byte-for-byte copy of the scheduler's embedded floor
# (DIST-03 zero-config floor). The drift-guard test in defaults_test.go asserts
# these two stay identical.
cp "$REPO_ROOT/internal/scheduler/defaults/scheduling.yaml" "$SEED/scheduling.yaml"

# (c) openspec.toml — authored by hand (schema skeleton); NOT regenerated here.
# Edit internal/defaults/seed/openspec.toml directly when the schema changes.

# Leak guard: fail loudly if any absolute home path survived sanitization. Only
# check the embeddable artifacts (sync.sh is not embedded — it legitimately
# contains the literal patterns it rewrites, so it is excluded).
if grep -rn '/Users/\|/home/' "$SEED/profiles" "$SEED/scheduling.yaml" "$SEED/openspec.toml"; then
	echo "ERROR: absolute home path leaked into the seed (T-06-01)" >&2
	exit 1
fi

echo "seed synced from canonical sources."
