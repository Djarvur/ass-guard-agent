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
// This file is the ONE home of the marker write (Task 2) and of the sweep's
// os.Remove surface (Task 3) — no other production path may create markers
// or remove transcripts.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrInvalidSessionID is the typed structural rejection for a session id
// that fails the traversal-safe grammar: every path-building entry point of
// the tombstone surface validates BEFORE any filepath.Join (T-18-09 — a
// hostile id rejects structurally, it can never escape the store directory).
var ErrInvalidSessionID = errors.New("session: invalid session id")

// Tombstone writes the zero-byte <id>.deleted marker beside the session's
// transcript (D-07). Idempotent by construction: re-writing over an existing
// marker truncates it back to zero bytes. The transcript itself is never
// touched — the grace-expired sweep (18-04 Task 3) is the ONLY purger. The
// marker's mtime is the grace clock (Pitfall 9): the undo window starts at
// tombstone creation.
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
