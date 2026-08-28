#!/bin/sh
# eval-change-class.sh — the D-03 re-run gate detector (12-08, ACP-08).
#
# Reports whether the change set touches the LOCKED CHANGE CLASSES that
# require the behavioral-eval gate (mise eval-gate) before merge:
#   profile     — profiles/ (any captured-profile content)
#   model       — internal/provider + internal/shaper + scheduling surfaces
#                 (config.yaml / scheduling.yaml / internal/scheduler)
#   turn-behavior — internal/engine + internal/session + internal/coreexec +
#                 internal/runtime-equivalent wiring (cmd/ass-guard serve path)
#   kit/ equivalents for the SEED-001 extraction (Phase 25, D-20/Pitfall 1) sit beside the legacy paths — both retained.
#
# Protocol: exit 0 + the no-change report when nothing matches; exit 3 with
# the matched classes + the gate instruction when any matches.
#
# Base ref: the first of MERGE_BASE (explicit), @{push} (the branch's
# upstream), HEAD~1 (a single-commit change). Document your CI's base by
# exporting MERGE_BASE (the mise-shaped CI hook point):
#
#   # CI step (after checkout + before merge):
#   MERGE_BASE="$CI_MERGE_REQUEST_DIFF_BASE_SHA" mise eval-check-changed
#   # exit 3 => run: mise eval-gate
#
# Selftest: ./scripts/eval-change-class.sh --selftest

set -eu

CLASSES='
profile:profiles/
model:internal/provider
model:kit/provider
model:internal/shaper
model:kit/shaper
model:internal/scheduler
model:config.yaml
model:scheduling.yaml
turn-behavior:internal/engine
turn-behavior:kit/engine
turn-behavior:internal/session
turn-behavior:kit/session
turn-behavior:internal/coreexec
turn-behavior:kit/runtime
turn-behavior:cmd/ass-guard
turn-behavior:internal/evalharness
turn-behavior:internal/evalsuite
'

selftest() {
    # Fixture: the matcher itself (a tiny POSIX-sh harness — no git fixtures
    # needed; the pattern list is the contract).
    hits=0
    misses=0
    check() {
        if match_class "$1" >/dev/null 2>&1; then
            hits=$((hits + 1))
        else
            misses=$((misses + 1))
            echo "SELFTEST FAIL: expected class match for $1" >&2
            exit 1
        fi
    }
    check "profiles/zcode/tools.json"
    check "internal/engine/decide.go"
    check "internal/session/ask.go"
    check "cmd/ass-guard/acp_serve.go"
    # Phase 25 (D-20): the kit layout must fire the gate too — hypothetical
    # post-extraction paths for the three turn-behavior + one model entries.
    check "kit/engine/decide.go"
    check "kit/session/ask.go"
    check "kit/runtime/runner.go"
    check "kit/provider/stream.go"
    for clean in "docs/README.md" ".planning/STATE.md" "scripts/eval-change-class.sh"; do
        out=$(match_class "$clean")
        if [ -n "$out" ]; then
            echo "SELFTEST FAIL: clean path '$clean' matched class '$out'" >&2
            exit 1
        fi
    done
    echo "selftest ok: class matches ($hits) + docs-only no-match verified"
    exit 0
}

match_class() {
    # Emits the class name when $1 matches any locked pattern.
    echo "$CLASSES" | while IFS= read -r line; do
        [ -n "$line" ] || continue
        cls=${line%%:*}
        pat=${line#*:}
        case "$1" in
            "$pat"* ) echo "$cls"; break ;;
        esac
    done
}

[ "${1:-}" = "--selftest" ] && selftest

base="${MERGE_BASE:-}"
if [ -z "$base" ]; then
    if base=$(git rev-parse --symbolic-full-name "@{push}" 2>/dev/null); then
        :
    else
        base="HEAD~1"
    fi
fi

changed=$(git diff --name-only "$base"..."HEAD" 2>/dev/null || git diff --name-only "$base" 2>/dev/null || true)
[ -n "$changed" ] || changed=$(git diff --name-only HEAD~1 2>/dev/null || true)

if [ -z "$changed" ]; then
    echo "eval-change-class: no diff resolved against base '$base' — no change report"
    exit 0
fi

matched=$(echo "$changed" | while IFS= read -r f; do
    match_class "$f"
done | sort -u)

if [ -z "$matched" ]; then
    echo "eval-change-class: no locked change classes touched (base '$base')"
    exit 0
fi

echo "eval-change-class: LOCKED CHANGE CLASSES touched — the eval gate must run:"
echo "$matched" | sed 's/^/  - /'
echo "run: mise eval-gate"
exit 3
