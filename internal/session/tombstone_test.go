package session //nolint:testpackage // internal package test

// 18-04 tombstone lifecycle battery (ACP-07 / D-07..D-09): the zero-byte
// marker write and the grace-expired GC sweep — grace boundary exactness
// (strictly-past-grace purges, exactly-at-grace does not), orphan-marker
// completion, the never-touch guarantees (live transcripts, the audit
// subtree, unrelated files, planted symlinks), and idempotence (a second
// sweep is a no-op).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sweepFixture writes one conforming transcript for sessionID, returning its
// path (the sweep's pair partner).
func sweepFixture(t *testing.T, dir, sessionID string) string {
	t.Helper()

	return writeListTranscript(t, dir, sessionID,
		standardListLines(t, sessionID, time.Now().UTC(), "sweep me"))
}

// sweepMarker writes the zero-byte marker for sessionID and pins its mtime —
// the grace clock (Pitfall 9: the undo window starts at tombstone creation).
func sweepMarker(t *testing.T, dir, sessionID string, at time.Time) string {
	t.Helper()

	marker := filepath.Join(dir, ".ass-guard", sessionID+tombstoneSuffix)

	err := os.WriteFile(marker, nil, filePermOwner)
	if err != nil {
		t.Fatalf("write marker %s: %v", marker, err)
	}

	err = os.Chtimes(marker, at, at)
	if err != nil {
		t.Fatalf("chtimes marker %s: %v", marker, err)
	}

	return marker
}

// assertGone fails when path still exists (any stat outcome but NotExist).
func assertGone(t *testing.T, path, what string) {
	t.Helper()

	if _, serr := os.Stat(path); !errors.Is(serr, os.ErrNotExist) {
		t.Errorf("%s still present after the sweep: %v", what, serr)
	}
}

// assertPresent fails when path cannot be Lstat'ed.
func assertPresent(t *testing.T, path, what string) {
	t.Helper()

	if _, serr := os.Lstat(path); serr != nil {
		t.Errorf("%s removed or unreachable: %v", what, serr)
	}
}

// TestTombstoneWrite pins the marker contract (D-07): a zero-byte 0600
// <id>.deleted beside the transcript, idempotent rewrites, the transcript
// untouched, and traversal-shaped ids rejected structurally before any path
// join (T-18-09).
func TestTombstoneWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sid := listUUID(1)

	if DefaultTombstoneGrace != 30*24*time.Hour {
		t.Errorf("DefaultTombstoneGrace = %v; want 30d (D-09)", DefaultTombstoneGrace)
	}

	transcript := sweepFixture(t, dir, sid)

	err := Tombstone(dir, sid)
	if err != nil {
		t.Fatalf("Tombstone: %v", err)
	}

	marker := filepath.Join(dir, ".ass-guard", sid+tombstoneSuffix)

	fi, serr := os.Stat(marker)
	if serr != nil {
		t.Fatalf("marker missing: %v", serr)
	}

	if fi.Size() != 0 {
		t.Errorf("marker size = %d; want zero bytes (D-07)", fi.Size())
	}

	if fi.Mode().Perm() != filePermOwner {
		t.Errorf("marker perms = %o; want %o", fi.Mode().Perm(), filePermOwner)
	}

	if _, terr := os.Stat(transcript); terr != nil {
		t.Errorf("transcript touched by the tombstone write: %v", terr)
	}

	err = Tombstone(dir, sid)
	if err != nil {
		t.Fatalf("second Tombstone (idempotent rewrite): %v", err)
	}

	fi, serr = os.Stat(marker)
	if serr != nil || fi.Size() != 0 {
		t.Errorf("marker after rewrite = (%v, %d bytes); want present, zero bytes", serr, fi.Size())
	}

	err = Tombstone(dir, "../../etc/passwd")
	if !errors.Is(err, ErrInvalidSessionID) {
		t.Errorf("Tombstone(traversal) error = %v; want ErrInvalidSessionID", err)
	}
}

// TestSweepRespectsGrace pins the grace clock (D-09): a 10d-old tombstoned
// pair survives the 30d grace, a 40d-old pair purges (transcript then
// marker), the removed id is returned — and the boundary convention is
// EXACT: a marker exactly at grace does NOT purge, one strictly past it
// does.
//
//nolint:funlen // one ordered grace-scenario assertion end-to-end
func TestSweepRespectsGrace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now().UTC()
	grace := 30 * 24 * time.Hour

	t10 := sweepFixture(t, dir, listUUID(10))
	m10 := sweepMarker(t, dir, listUUID(10), now.Add(-10*24*time.Hour))

	t40 := sweepFixture(t, dir, listUUID(40))
	m40 := sweepMarker(t, dir, listUUID(40), now.Add(-40*24*time.Hour))

	removed, err := SweepTombstones(dir, grace, now)
	if err != nil {
		t.Fatalf("SweepTombstones: %v", err)
	}

	if len(removed) != 1 || removed[0] != listUUID(40) {
		t.Errorf("removed = %v; want exactly [%s]", removed, listUUID(40))
	}

	assertGone(t, t40, "past-grace transcript")
	assertGone(t, m40, "past-grace marker")
	assertPresent(t, t10, "inside-grace transcript")
	assertPresent(t, m10, "inside-grace marker")

	// Boundary exactness, computed off the ACTUAL stored mtime so filesystem
	// timestamp precision cannot flip the convention.
	sweepFixture(t, dir, listUUID(30))

	mEdge := sweepMarker(t, dir, listUUID(30), now.Add(-grace))

	efi, gerr := os.Stat(mEdge)
	if gerr != nil {
		t.Fatalf("stat edge marker: %v", gerr)
	}

	removed, err = SweepTombstones(dir, grace, efi.ModTime().Add(grace))
	if err != nil {
		t.Fatalf("exactly-at-grace sweep: %v", err)
	}

	for _, id := range removed {
		if id == listUUID(30) {
			t.Error("exactly-at-grace marker purged; the convention is strictly-past-grace only")
		}
	}

	assertPresent(t, mEdge, "exactly-at-grace marker")

	removed, err = SweepTombstones(dir, grace, efi.ModTime().Add(grace+time.Second))
	if err != nil {
		t.Fatalf("just-past-grace sweep: %v", err)
	}

	if len(removed) != 1 || removed[0] != listUUID(30) {
		t.Errorf("just-past-grace removed = %v; want exactly [%s]", removed, listUUID(30))
	}
}

// TestSweepNeverTouches pins the sweep's bounded removal surface: a live
// (non-tombstoned) transcript, audit-path artifacts, and unrelated store
// files survive REGARDLESS of age; planted symlinks (marker or transcript)
// are skipped loudly with nothing removed (T-18-03); an orphan marker with
// no transcript is removed — its pair is already gone, which also completes
// a sweep interrupted mid-purge.
//
//nolint:funlen,gocyclo,cyclop // the never-touch matrix in one scenario
func TestSweepNeverTouches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now().UTC()
	grace := 30 * 24 * time.Hour
	old := now.Add(-400 * 24 * time.Hour) // far past grace — irrelevant for non-tombstoned files

	live := sweepFixture(t, dir, listUUID(1))
	setListMtime(t, live, old)

	auditDir := filepath.Join(dir, ".ass-guard", "audit")

	err := os.MkdirAll(auditDir, dirPerm)
	if err != nil {
		t.Fatalf("mkdir audit dir: %v", err)
	}

	auditFile := filepath.Join(auditDir, "mirror.jsonl")

	err = os.WriteFile(auditFile, []byte("audit\n"), filePermOwner)
	if err != nil {
		t.Fatalf("write audit artifact: %v", err)
	}

	if cerr := os.Chtimes(auditFile, old, old); cerr != nil {
		t.Fatalf("chtimes audit artifact: %v", cerr)
	}

	unrelated := filepath.Join(dir, ".ass-guard", "notes.txt")

	err = os.WriteFile(unrelated, []byte("notes\n"), filePermOwner)
	if err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	if cerr := os.Chtimes(unrelated, old, old); cerr != nil {
		t.Fatalf("chtimes unrelated file: %v", cerr)
	}

	// Planted symlinks (T-18-03): a symlinked transcript under a valid
	// tombstone and a symlinked marker are both skipped — no removal of the
	// link, the pair, or the target.
	target := filepath.Join(dir, "outside.txt")

	err = os.WriteFile(target, []byte("outside\n"), filePermOwner)
	if err != nil {
		t.Fatalf("write symlink target: %v", err)
	}

	linkSid := listUUID(2)
	linkTranscript := filepath.Join(dir, ".ass-guard", "transcript_"+linkSid+".jsonl")

	err = os.Symlink(target, linkTranscript)
	if err != nil {
		t.Fatalf("plant transcript symlink: %v", err)
	}

	sweepMarker(t, dir, linkSid, now.Add(-90*24*time.Hour))

	linkMarkSid := listUUID(3)
	linkMarker := filepath.Join(dir, ".ass-guard", linkMarkSid+tombstoneSuffix)

	err = os.Symlink(target, linkMarker)
	if err != nil {
		t.Fatalf("plant marker symlink: %v", err)
	}

	// The orphan marker: FRESH (one minute old) — removed unconditionally,
	// its pair is already gone.
	orphan := listUUID(4)
	sweepMarker(t, dir, orphan, now.Add(-time.Minute))

	removed, serr := SweepTombstones(dir, grace, now)
	if serr != nil {
		t.Fatalf("SweepTombstones: %v", serr)
	}

	if len(removed) != 1 || removed[0] != orphan {
		t.Errorf("removed = %v; want exactly the orphan marker [%s]", removed, orphan)
	}

	assertPresent(t, live, "live transcript")
	assertPresent(t, auditFile, "audit-path artifact")
	assertPresent(t, unrelated, "unrelated store file")
	assertPresent(t, target, "symlink target")
	assertPresent(t, linkTranscript, "planted transcript symlink")
	assertPresent(t, linkMarker, "planted marker symlink")
}

// TestSweepIdempotent: running the sweep twice removes nothing the second
// time — the interrupted-mid-purge case converges and the steady state is a
// no-op returning an empty set.
func TestSweepIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now().UTC()

	for _, n := range []int{5, 6} {
		sweepFixture(t, dir, listUUID(n))
		sweepMarker(t, dir, listUUID(n), now.Add(-60*24*time.Hour))
	}

	removed, err := SweepTombstones(dir, 30*24*time.Hour, now)
	if err != nil {
		t.Fatalf("first sweep: %v", err)
	}

	if len(removed) != 2 {
		t.Fatalf("first sweep removed = %v; want 2 ids", removed)
	}

	removed, err = SweepTombstones(dir, 30*24*time.Hour, now)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("second sweep removed = %v; want nothing (idempotent no-op)", removed)
	}
}
