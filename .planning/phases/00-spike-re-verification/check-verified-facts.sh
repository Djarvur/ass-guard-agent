#!/usr/bin/env bash
#
# check-verified-facts.sh -- Phase 0 completeness gate for VERIFIED-FACTS.md
#
# Codifies the 00-RESEARCH.md S8 completeness rules + S9 validation
# architecture for .planning/research/VERIFIED-FACTS.md. Exits non-zero on
# ANY failure; prints a one-line PASS/FAIL summary per check.
#
# This gates the DOC, not the code. The spikes' own process exit codes are
# their per-plan verification (per VALIDATION.md); this script only asserts
# VERIFIED-FACTS.md is complete, dated, sanitized, and structurally correct.
# It does NOT run any spike and does NOT depend on the spikes executing.
#
# VERIFIED-FACTS.md is authored by plan 00-05 Task 3; this gate is RUN only
# after that task (and is exercised as a syntax check -- `bash -n` -- by
# Task 1's own verify step, before the doc exists).
#
# Run from the repo root:
#   bash .planning/phases/00-spike-re-verification/check-verified-facts.sh

# --- Step 2: locate VERIFIED-FACTS.md; fail early if missing ---------------
VF="$PWD/.planning/research/VERIFIED-FACTS.md"
if [ ! -f "$VF" ]; then
  echo "FAIL: VERIFIED-FACTS.md not found at $VF"
  echo "      (it is authored by plan 00-05 Task 3; this gate is RUN only after that task)"
  exit 1
fi

# Track failures across all checks; run every check regardless of earlier
# outcomes so the operator sees the full picture in one invocation.
FAILURES=0

# --- Step 3: section count >= 5 (## #N. headings) --------------------------
# Per planner grep hygiene: counts "## #N." section markers; markdown
# headings are structural, so the comment-prose concern does not apply.
section_count=$(grep -c '^## #' "$VF")
if [ "$section_count" -ge 5 ]; then
  echo "PASS [sections]: $section_count section header(s) '## #N.' found (>=5 required)"
else
  echo "FAIL [sections]: only $section_count section header(s) '## #N.' found (>=5 required)"
  FAILURES=$((FAILURES + 1))
fi

# --- Step 4: Status fields >= 5, all within enum ---------------------------
# Count Status fields, then assert NONE are outside the enum by piping the
# matched Status lines through grep -Ev (inverted): any line that survives
# the -v filter lacks all four enum words and is therefore a violation.
status_count=$(grep -c '^- \*\*Status:\*\*' "$VF")
status_outside=$(grep -E '^- \*\*Status:\*\*' "$VF" \
  | grep -Ev 'VERIFIED|FAILED|PARTIAL|STRUCTURALLY-MOOT' \
  | grep -c .)
if [ "$status_count" -ge 5 ] && [ "$status_outside" -eq 0 ]; then
  echo "PASS [status]: $status_count Status field(s) found, all in enum {VERIFIED,FAILED,PARTIAL,STRUCTURALLY-MOOT} (>=5 required)"
else
  echo "FAIL [status]: $status_count Status field(s) found, $status_outside outside enum (>=5 required, 0 outside enum required)"
  FAILURES=$((FAILURES + 1))
fi

# --- Step 5: Evidence fields >= 5, none immediately followed by empty line -
evidence_count=$(grep -c '^- \*\*Evidence:\*\*' "$VF")
# awk: flag a violation when the line immediately after an Evidence field is
# empty. $0 == "" is a truly empty line (the S8 "none followed by an empty
# line" rule); prev_is_evidence carries the previous line's Evidence-ness.
empty_after=$(awk '
  {
    if (prev_is_evidence && $0 == "") { bad = 1 }
    prev_is_evidence = ($0 ~ /^- \*\*Evidence:\*\*/)
  }
  END { print (bad ? 1 : 0) }
' "$VF")
if [ "$evidence_count" -ge 5 ] && [ "$empty_after" -eq 0 ]; then
  echo "PASS [evidence]: $evidence_count Evidence field(s) found, none followed by an empty line (>=5 required)"
else
  echo "FAIL [evidence]: $evidence_count Evidence field(s) found, empty-after=$empty_after (>=5 required, no empty-after allowed)"
  FAILURES=$((FAILURES + 1))
fi

# --- Step 6: no forbidden tokens -------------------------------------------
# TBD / TODO / [fill in] / <empty> / pending spike completion must be GONE.
# (The Wave-1 placeholder "populated by plan 00-0X" is replaced by the time
# this gate runs; the literal forbidden set is what the S8 rule enumerates.)
forbidden=$(grep -nEi 'TBD|TODO|\[fill in\]|<empty>|pending spike completion' "$VF")
if [ -z "$forbidden" ]; then
  echo "PASS [forbidden-tokens]: no TBD/TODO/[fill in]/<empty>/pending spike completion found"
else
  echo "FAIL [forbidden-tokens]: forbidden placeholder token(s) found:"
  printf '%s\n' "$forbidden" | sed 's/^/        /'
  FAILURES=$((FAILURES + 1))
fi

# --- Step 7: item #4 is STRUCTURALLY-MOOT ----------------------------------
# Intent (plan step 7 + must_haves): item #4's section declares
# STRUCTURALLY-MOOT. The plan's literal "grep -A3 '^## #4\.'" window cannot
# reach the Status field under the S8 D-02 template, which places Status ~6
# lines below the section header (header, blank, Fact, Source, Verified,
# Verified against, Status). A literal -A3 would reject a correctly-structured
# file, violating must_haves truth #3 (gate passes exit 0). So the check is
# implemented as a section-scoped scan: extract item #4's full section (from
# its '## #4.' header up to the next '## ' header or EOF) and assert
# STRUCTURALLY-MOOT appears within it. (Rule 1 deviation: the gate mechanism
# is widened so a correct file passes; the asserted fact is unchanged.)
item4_section=$(awk '
  /^## #4\./ { in_sec = 1 }
  in_sec && /^## / && !/^## #4\./ { in_sec = 0 }
  in_sec { print }
' "$VF")
if printf '%s\n' "$item4_section" | grep -q 'STRUCTURALLY-MOOT'; then
  echo "PASS [item-4-moot]: item #4 section declares STRUCTURALLY-MOOT (D-06)"
else
  echo "FAIL [item-4-moot]: item #4 section does not declare STRUCTURALLY-MOOT (D-06 requires it)"
  FAILURES=$((FAILURES + 1))
fi

# --- Step 8: S3 path correction recorded -----------------------------------
# The corrected zcode JSONL path must appear verbatim in the doc.
if grep -q 'model-io-sess_<session-id>.jsonl' "$VF"; then
  echo "PASS [path-correction]: S3 corrected path 'model-io-sess_<session-id>.jsonl' recorded"
else
  echo "FAIL [path-correction]: S3 corrected path 'model-io-sess_<session-id>.jsonl' not found"
  FAILURES=$((FAILURES + 1))
fi

# --- Step 9: one-line summary + exit code ----------------------------------
echo "----"
if [ "$FAILURES" -eq 0 ]; then
  echo "RESULT: ALL CHECKS PASSED (exit 0)"
  exit 0
else
  echo "RESULT: $FAILURES CHECK(S) FAILED (exit 1)"
  exit 1
fi
