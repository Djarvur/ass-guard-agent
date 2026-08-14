#!/usr/bin/env sh
# openspec-stub.sh — a portable stand-in for the real openspec CLI. The test
# harness puts this script on PATH (under the name "openspec") so the Adapter
# exercises a real subprocess without depending on the real binary. The first
# argument selects the behavior:
#   echo      -> print remaining args to stdout, exit 0
#   fail      -> print ASSGUARD_STUB_STDERR (default "boom") to stderr, exit 1
#   sleep     -> sleep ASSGUARD_STUB_SLEEP seconds (default 10), exit 0
#   argv      -> print the full argv ($@) to stdout, exit 0
#   version   -> print "openspec stub 0.0.0" to stdout, exit 0
mode="${ASSGUARD_STUB_MODE:-$1}"
case "$mode" in
  echo)
    shift
    printf '%s\n' "$*"
    ;;
  fail)
    printf '%s\n' "${ASSGUARD_STUB_STDERR:-boom}" 1>&2
    exit 1
    ;;
  sleep)
    sleep "${ASSGUARD_STUB_SLEEP:-10}"
    ;;
  argv)
    printf '%s\n' "$*"
    ;;
  version|--version)
    printf 'openspec stub 0.0.0\n'
    ;;
  env)
    # print OPEN_SPEC_INTERACTIVE from the environment (T3 Test 7)
    printf 'OPEN_SPEC_INTERACTIVE=%s\n' "${OPEN_SPEC_INTERACTIVE:-unset}"
    ;;
  stdin)
    # observe stdin: immediate EOF prints "stdin=eof"; any data prints "stdin=data"
    if IFS= read -r line; then
      printf 'stdin=data:%s\n' "$line"
    else
      printf 'stdin=eof\n'
    fi
    ;;
  *)
    # default: env-controlled outcome (T2/T3) — print argv to stdout, then
    # honor ASSGUARD_STUB_EXIT (non-zero exits print ASSGUARD_STUB_STDERR to stderr)
    printf 'openspec stub invoked: %s\n' "$*"
    if [ -n "${ASSGUARD_STUB_EXIT:-}" ]; then
      [ -z "${ASSGUARD_STUB_STDERR:-}" ] || printf '%s\n' "$ASSGUARD_STUB_STDERR" 1>&2
      exit "$ASSGUARD_STUB_EXIT"
    fi
    ;;
esac
