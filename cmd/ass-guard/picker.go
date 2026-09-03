package main

// picker.go — the D-11 interactive session picker (18-06 Task 2): a numbered
// list of the most recent sessions rendered on STDERR, a number read
// line-wise from STDIN, ONE re-prompt on an invalid entry, and a typed error
// on the second invalid entry or EOF. No raw-mode/termios dependency exists
// anywhere in the path — the surfaces are io.Reader/io.Writer so the picker
// works identically over pipes, ssh, and plain TTYs. The CLI edge passes
// os.Stdin/os.Stderr at the composition point ONLY (acp_serve.go's
// pickResumeSession).

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// Picker bounds (D-11 + T-18-14: the DoS guard — an endless stdin of garbage
// gets exactly one retry, then the typed error) and the relative-time
// bucket edges (the display vocabulary — every leading unit is rounded).
const (
	pickerTitleMaxRunes = 60
	pickerReadBufSize   = 8 * 1024
	pickerHoursPerDay   = 24
	pickerDaysPerWeek   = 7
)

// errPickerInput is the typed rejection for a picker session that never
// produced a valid selection (two invalid entries, or EOF before one).
var errPickerInput = errors.New("no session selected")

// SelectSession renders headers as numbered rows `N) <title-60-runes>  (<rel
// time>)` on out, prints one selection prompt, and reads a line-wise number
// from in. An invalid entry (non-numeric, empty, out of range) re-prints the
// prompt ONCE; a second invalid entry — or EOF before any valid entry — is
// the typed errPickerInput. The caller passes the recency-ordered rows
// (ListSessions' first page); the top pickerMaxRows render.
func SelectSession(headers []session.SessionHeader, in io.Reader, out io.Writer) (string, error) {
	rows := headers[:min(len(headers), pickerMaxRows)]

	now := time.Now()

	for i, h := range rows {
		_, _ = fmt.Fprintf(out, "%d) %s  (%s)\n",
			i+1, truncateRunes(h.Title, pickerTitleMaxRunes),
			FormatRelativeTime(h.LastActivity, now))
	}

	br := bufio.NewReaderSize(in, pickerReadBufSize)

	const maxAttempts = 2

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, _ = fmt.Fprint(out, "Resume which session (1-"+strconv.Itoa(len(rows))+")? ")

		entry, rerr := br.ReadString('\n')
		if rerr != nil && entry == "" {
			// EOF (or a read error) with no partial data: no selection.
			return "", fmt.Errorf("%w: input closed", errPickerInput)
		}

		if n, ok := parsePickerEntry(entry, len(rows)); ok {
			return rows[n-1].SessionID, nil
		}

		if attempt == maxAttempts {
			break
		}
	}

	return "", fmt.Errorf("%w: expected a number in 1-%d", errPickerInput, len(rows))
}

// parsePickerEntry trims and parses one stdin entry; ok is false for empty,
// non-numeric, or out-of-range input (an empty line counts as invalid).
func parsePickerEntry(entry string, rowCount int) (n int, ok bool) { //nolint:nonamedreturns // pair-result clarity
	parsed, err := strconv.Atoi(strings.TrimSpace(entry))
	if err != nil || parsed < 1 || parsed > rowCount {
		return 0, false
	}

	return parsed, true
}

// truncateRunes cuts s to at most limit runes, unicode-safely (a multi-byte
// title never renders as a torn rune sequence). A fallback title ("(no
// prompt)") renders verbatim — the honest row for a session that never
// received a prompt.
func truncateRunes(s string, limit int) string {
	if runes := []rune(s); len(runes) > limit {
		return string(runes[:limit])
	}

	return s
}

// FormatRelativeTime renders t's age relative to now in coarse buckets —
// "just now" (< 1m), "Nm ago" (< 1h), "Nh ago" (< 24h), "Nd ago" (< 7d),
// "Nw ago" beyond — each leading unit rounded to nearest (90s reads as "2m
// ago"). A future t (clock skew) reads as just-now.
func FormatRelativeTime(t, now time.Time) string {
	d := now.Sub(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(roundUnit(d.Minutes())) + "m ago"
	case d < pickerHoursPerDay*time.Hour:
		return strconv.Itoa(roundUnit(d.Hours())) + "h ago"
	case d < pickerDaysPerWeek*pickerHoursPerDay*time.Hour:
		return strconv.Itoa(roundUnit(d.Hours()/pickerHoursPerDay)) + "d ago"
	default:
		return strconv.Itoa(roundUnit(d.Hours()/pickerHoursPerDay/pickerDaysPerWeek)) + "w ago"
	}
}

// roundUnit rounds a bucket's leading unit to nearest (90s -> 2m).
func roundUnit(v float64) int {
	return int(math.Round(v))
}
