package session

// Tombstone lifecycle (18-04, ACP-07 / D-07..D-09): the delete path's on-disk
// contract. A delete NEVER removes or rewrites the transcript — it writes a
// zero-byte <sessionID>.deleted marker beside the transcript (the shared
// tombstoneSuffix spelling the 18-03 list engine already stat-filters on),
// and the marker IS the delete until the grace-expired GC sweep purges the
// pair: reversible until the purge fires (remove the marker), audit-exempt
// forever (the audit subtree lives under a different branch of .ass-guard/
// and is never enumerated by any function in this file).
//
// This file is the ONE home of the marker write and of the sweep's os.Remove
// surface — no other production path may create markers or remove
// transcripts. Every os.Remove call site's arguments derive from a validated
// marker id joined under the store directory (T-18-02/T-18-09): the removal
// surface is structurally confined to tombstoned pairs inside .ass-guard/.

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTombstoneGrace is the default GC grace (D-09): 30 days — CC's
// cleanupPeriodDays parity. The grace window is the product's only undo for
// a mistaken delete (D-09 one-way door: the purge destroys the transcript
// permanently), so the default errs long.
const DefaultTombstoneGrace = 30 * 24 * time.Hour

// ErrInvalidSessionID is the typed structural rejection for a session id
// that fails the traversal-safe grammar: every path-building entry point of
// the tombstone surface validates BEFORE any filepath.Join (T-18-09 — a
// hostile id rejects structurally, it can never escape the store directory).
var ErrInvalidSessionID = errors.New("session: invalid session id")

// Tombstone writes the zero-byte <id>.deleted marker beside the session's
// transcript (D-07). Idempotent by construction: re-writing over an existing
// marker truncates it back to zero bytes. The transcript itself is never
// touched — the grace-expired sweep below is the ONLY purger. The marker's
// mtime is the grace clock (Pitfall 9): the undo window starts at tombstone
// creation.
func Tombstone(dir, sessionID string) error {
	if !sessIDPattern.MatchString(sessionID) {
		return fmt.Errorf("%w: %q (rejected before any path join)", ErrInvalidSessionID, sessionID)
	}

	store := filepath.Join(dir, storeDirName)

	err := os.MkdirAll(store, dirPerm)
	if err != nil {
		return fmt.Errorf("session: tombstone: mkdir store: %w", err)
	}

	marker := filepath.Join(store, sessionID+tombstoneSuffix)

	// A pre-existing non-regular marker (a planted symlink — T-18-03) refuses
	// the write: OpenFile would follow the link and truncate its target.
	fi, lerr := os.Lstat(marker)
	if lerr == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf( //nolint:err113 // dynamic path-carrying error
			"session: tombstone: marker path %s is not a regular file (refusing to follow a planted link)", marker)
	}

	f, err := os.OpenFile(marker, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePermOwner)
	if err != nil {
		return fmt.Errorf("session: tombstone: write marker: %w", err)
	}

	cerr := f.Close()
	if cerr != nil {
		return fmt.Errorf("session: tombstone: close marker: %w", cerr)
	}

	return nil
}

// SweepTombstones purges grace-expired tombstoned pairs under
// dir/.ass-guard/: for every marker whose mtime is STRICTLY past the grace
// (exactly-at-grace does not purge — the picked boundary convention), it
// removes the transcript FIRST, then the marker; an orphan marker (its
// transcript already gone) is removed unconditionally — the pair is already
// destroyed, and removing the marker also completes a sweep interrupted
// mid-purge (idempotent). Orphan aside, nothing else is ever touched: live
// transcripts, the audit subtree, unrelated files, and planted symlinks
// (marker or transcript — T-18-03: Lstat link-bit + regular-file checks
// before any removal, skipped loudly) all survive regardless of age.
//
// The grace clock is the MARKER's mtime (Pitfall 9). now is injected so
// tests pin the boundary exactly. Every removal logs one structured line
// (the loud-counter discipline); a file that cannot be removed is reported
// in the returned error (joined), never a panic — a partial sweep is
// completed by the next one.
func SweepTombstones(dir string, grace time.Duration, now time.Time) ([]string, error) {
	store := filepath.Join(dir, storeDirName)

	entries, err := os.ReadDir(store)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("session: sweep: read store: %w", err)
	}

	removed := []string{}

	var errs []error

	for _, e := range entries {
		id, ok := sweepMarkerID(e.Name())
		if !ok {
			continue // transcripts, the audit subtree, dotfiles — never enumerated
		}

		purged, perr := sweepPair(store, id, grace, now)
		if purged {
			removed = append(removed, id)
		}

		if perr != nil {
			errs = append(errs, perr)
		}
	}

	return removed, errors.Join(errs...)
}

// sweepPair processes ONE validated tombstoned pair (the sweep's loop body):
// the orphan marker (transcript already gone) is removed unconditionally —
// the pair is already destroyed and removing the marker completes a sweep
// interrupted mid-purge; a pair whose marker mtime is STRICTLY past the grace
// is purged (transcript first, then the marker — the ONLY transcript removal
// in the codebase, T-18-02); a non-regular transcript (a planted link) is
// skipped loudly with nothing removed (T-18-03). purged reports the id's
// addition to the sweep's removed set; err carries the pair's failure (the
// sweep continues with the next pair either way).
func sweepPair(store, id string, grace time.Duration, now time.Time) (bool, error) {
	marker := filepath.Join(store, id+tombstoneSuffix)
	transcript := filepath.Join(store, transcriptFilePrefix+id+transcriptFileSuffix)

	mfi, skip := sweepRemovableFile(marker)
	if skip {
		return false, nil
	}

	tfi, terr := os.Lstat(transcript)

	switch {
	case errors.Is(terr, os.ErrNotExist):
		// Orphan marker: remove it unconditionally.
		rerr := os.Remove(marker)
		if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return false, fmt.Errorf("session: sweep: remove orphan marker %s: %w", marker, rerr)
		}

		slog.Info("session: sweep removed orphan tombstone marker", "sessionID", id, "path", marker)

		return true, nil
	case terr != nil:
		return false, fmt.Errorf("session: sweep: stat transcript %s: %w", transcript, terr)
	case !tfi.Mode().IsRegular():
		slog.Warn("session: sweep skipping tombstoned pair — transcript is not a regular file (planted link?)", //nolint:lll // one structured log line
			"sessionID", id, "path", transcript)

		return false, nil
	case now.Sub(mfi.ModTime()) > grace:
		// Grace expired (strictly past): transcript first, then the marker.
		rerr := os.Remove(transcript)
		if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return false, fmt.Errorf("session: sweep: remove transcript %s: %w", transcript, rerr)
		}

		merr := os.Remove(marker)
		if merr != nil && !errors.Is(merr, os.ErrNotExist) {
			return false, fmt.Errorf("session: sweep: remove marker %s: %w", marker, merr)
		}

		slog.Info("session: sweep purged grace-expired tombstoned session",
			"sessionID", id, "age", now.Sub(mfi.ModTime()).Round(time.Second).String(), "path", transcript)

		return true, nil
	}

	return false, nil
}

// sweepMarkerID derives the session id from a <id>.deleted marker name;
// ok is false for every other store entry AND for a suffix-carrying name
// whose id fails the traversal-safe grammar (T-18-09 — the id is validated
// before it ever reaches a filepath.Join).
func sweepMarkerID(name string) (string, bool) {
	if !strings.HasSuffix(name, tombstoneSuffix) {
		return "", false
	}

	id := strings.TrimSuffix(name, tombstoneSuffix)
	if !sessIDPattern.MatchString(id) {
		return "", false
	}

	return id, true
}

// sweepRemovableFile enforces T-18-03 on one side of a pair: the path must
// be a REGULAR file — Lstat never follows a link, so a planted symlink (or
// any other non-regular entry) is skipped with a loud log instead of
// removed. skip is true when the caller must leave this pair alone.
func sweepRemovableFile(path string) (os.FileInfo, bool) {
	fi, err := os.Lstat(path)
	if err != nil {
		// Vanished between ReadDir and Lstat: nothing left to remove.
		return nil, true
	}

	if !fi.Mode().IsRegular() {
		slog.Warn("session: sweep skipping non-regular tombstone marker (planted link?)", "path", path)

		return nil, true
	}

	return fi, false
}
