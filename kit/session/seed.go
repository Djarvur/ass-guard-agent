package session

import (
	"strconv"
	"strings"
)

// Turn-id seeding (18-01, ACP-06 / 18-CONTEXT D-01 "continued id sequences
// from transcript maxima"): a resumed session's next turn id continues the
// on-disk sequence instead of restarting at 001 — replayed frames and new
// frames would collide otherwise (18-RESEARCH Pitfall 2).

// turnIDSuffixLen is the zero-padded width of the turn suffix in nextTurnID's
// fmt.Sprintf("%s-turn-%03d", …) shape (session.go). Suffixes wider than this
// (turn 1000 → "1000") are still valid: %03d is a minimum width, so the scan
// accepts any all-digit suffix.
const turnIDSuffixLen = 3

// MaxTurnCounter scans lines for TurnID values of the exact
// <sessionID>-turn-%03d shape this session mints (the format is fixed at
// Session.nextTurnID) and returns the largest suffix, 0 when none. Pure
// function, no I/O — resume seeds the session's turn counter from the result
// (SeedResume), so the next nextTurnID() is exactly one past the transcript's
// maximum. A line whose suffix is not all digits (or not positive) cannot have
// been minted by nextTurnID and is ignored.
func MaxTurnCounter(sessionID string, lines []Line) int64 {
	prefix := sessionID + "-turn-"

	var turnMax int64

	for i := range lines {
		id := lines[i].TurnID
		if !strings.HasPrefix(id, prefix) {
			continue
		}

		suffix := strings.TrimPrefix(id, prefix)
		if !isTurnSuffix(suffix) {
			continue
		}

		n, err := strconv.ParseInt(suffix, 10, 64)
		if err != nil || n <= 0 {
			continue
		}

		if n > turnMax {
			turnMax = n
		}
	}

	return turnMax
}

// isTurnSuffix reports whether s matches the %03d shape's alphabet: at least
// turnIDSuffixLen characters, all ASCII digits (ParseInt alone would accept
// "+5"/"-5", which nextTurnID can never produce).
func isTurnSuffix(s string) bool {
	if len(s) < turnIDSuffixLen {
		return false
	}

	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}
