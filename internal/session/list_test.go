package session //nolint:testpackage // internal package test

// 18-03 contract battery for the session enumeration engine (ListSessions):
// stable total order with (lastActivity desc, id asc) tie-break, composite
// opaque cursor pagination with no row re-emission, stat-only tombstone
// filter, lean header shape, bounds, and malformed-cursor rejection.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
)

const (
	listTestPageSize   = 3
	listTestSeven      = 7
	listTestStoreBig   = 120 // > clamp(100) so the bound is observable
	listTestLimitHuge  = 500
	listTestDefaultPg  = 50
	listTestClampPg    = 100
	listTestPromptLen  = 200
	listTestTitleCut   = 80
	listTestFillerSize = 70 * 1024 // filler pushing the prompt beyond the 64 KiB prefix
	listTestBaseYear   = 2026
	listTestFallback   = "(no prompt)"
)

// listUUID returns a sessIDPattern-valid id whose lexicographic order matches
// n (fixed-width hex tail) — ascending n is ascending id.
func listUUID(n int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012x", n)
}

// listStartLine builds the conforming first line every transcript carries.
func listStartLine(id string, at time.Time) Line {
	return Line{Type: TypeSessionStart, Timestamp: at, Text: id}
}

// listUserLine builds a user_message line in the real on-disk shape (content
// blocks, the AppendUserMessage envelope).
func listUserLine(t *testing.T, id string, at time.Time, prompt string) Line {
	t.Helper()

	raw, err := json.Marshal([]ContentBlock{{Type: blockText, Text: prompt}})
	if err != nil {
		t.Fatalf("marshal user content: %v", err)
	}

	return Line{Type: TypeUserMessage, TurnID: id + "-turn-001", Timestamp: at, Content: raw}
}

// standardListLines is the minimal conforming transcript: session_start plus
// one user prompt.
func standardListLines(t *testing.T, id string, createdAt time.Time, prompt string) []Line {
	t.Helper()

	return []Line{listStartLine(id, createdAt), listUserLine(t, id, createdAt, prompt)}
}

// writeListTranscript writes a hand-crafted transcript_<id>.jsonl under
// dir/.ass-guard/ through the real Line JSON envelope, returning its path.
func writeListTranscript(t *testing.T, dir, id string, lines []Line) string {
	t.Helper()

	store := filepath.Join(dir, ".ass-guard")

	err := os.MkdirAll(store, dirPerm)
	if err != nil {
		t.Fatalf("mkdir %s: %v", store, err)
	}

	var b bytes.Buffer

	for i := range lines {
		raw, merr := json.Marshal(lines[i])
		if merr != nil {
			t.Fatalf("marshal line: %v", merr)
		}

		b.Write(raw)
		b.WriteByte('\n')
	}

	path := filepath.Join(store, "transcript_"+id+".jsonl")

	err = os.WriteFile(path, b.Bytes(), filePermOwner)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

// setListMtime forces a transcript's mtime — deterministic lastActivity.
func setListMtime(t *testing.T, path string, at time.Time) {
	t.Helper()

	err := os.Chtimes(path, at, at)
	if err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// mustList runs ListSessions and fails the test on any error.
func mustList( //nolint:nonamedreturns // (headers, next) is the page pair
	t *testing.T, dir, cursor string, limit int,
) (headers []SessionHeader, next string) {
	t.Helper()

	headers, next, err := ListSessions(dir, cursor, limit)
	if err != nil {
		t.Fatalf("ListSessions(%q, %q, %d): %v", dir, cursor, limit, err)
	}

	return headers, next
}

// listIDs projects a header page down to its id sequence.
func listIDs(headers []SessionHeader) []string {
	ids := make([]string, len(headers))
	for i := range headers {
		ids[i] = headers[i].SessionID
	}

	return ids
}

// wantIDs asserts a page's id sequence.
func wantIDs(t *testing.T, headers []SessionHeader, want []string, what string) {
	t.Helper()

	if ids := listIDs(headers); !reflect.DeepEqual(ids, want) {
		t.Fatalf("%s: got %v, want %v", what, ids, want)
	}
}

// b64 encodes a raw cursor payload the way the engine's codec is expected to.
func b64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func listTestBase() time.Time {
	return time.Date(listTestBaseYear, time.August, 26, 12, 0, 0, 0, time.UTC)
}

func TestSessionListOrderingAndTiebreak(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := listTestBase()

	// ids 2 and 5 share base+1h — the adjacency edge resolved by the
	// composite cursor's ascending-id tie-break (D-05).
	forced := map[string]time.Time{
		listUUID(1): base.Add(3 * time.Hour),
		listUUID(2): base.Add(1 * time.Hour),
		listUUID(5): base.Add(1 * time.Hour),
		listUUID(3): base.Add(2 * time.Hour),
		listUUID(4): base,
	}

	for id, at := range forced {
		path := writeListTranscript(t, dir, id, standardListLines(t, id, base, "prompt "+id))
		setListMtime(t, path, at)
	}

	got1, next := mustList(t, dir, "", 0)
	if next != "" {
		t.Fatalf("nextCursor = %q, want empty", next)
	}

	// lastActivity desc; equal-mtime pair ascending by id; exactly once each.
	want := []string{listUUID(1), listUUID(3), listUUID(2), listUUID(5), listUUID(4)}
	wantIDs(t, got1, want, "order")

	for _, h := range got1 {
		if !h.LastActivity.Equal(forced[h.SessionID]) {
			t.Fatalf("lastActivity of %s = %v, want %v", h.SessionID, h.LastActivity, forced[h.SessionID])
		}
	}

	// identical call → identical slice: equal-mtime rows never flip (D-05).
	got2, _ := mustList(t, dir, "", 0)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatalf("identical calls differ:\n%v\n%v", got1, got2)
	}
}

func TestSessionListCursorPagination(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := listTestBase()

	for n := 1; n <= listTestSeven; n++ {
		id := listUUID(n)
		path := writeListTranscript(t, dir, id, standardListLines(t, id, base, "prompt "+id))
		setListMtime(t, path, base.Add(time.Duration(n)*time.Second))
	}

	// Single-page reference: 7 rows, mtime desc → ids 7..1.
	full, _ := mustList(t, dir, "", 0)

	wantFull := listIDs(full)
	if len(wantFull) != listTestSeven {
		t.Fatalf("full list has %d rows, want %d", len(wantFull), listTestSeven)
	}

	p1, c1 := mustList(t, dir, "", listTestPageSize)
	p2, c2 := mustList(t, dir, c1, listTestPageSize)
	p3, c3 := mustList(t, dir, c2, listTestPageSize)

	wantIDs(t, p1, wantFull[0:listTestPageSize], "page1")
	wantIDs(t, p2, wantFull[listTestPageSize:2*listTestPageSize], "page2")
	wantIDs(t, p3, wantFull[2*listTestPageSize:], "page3")

	if c1 == "" || c2 == "" {
		t.Fatalf("intermediate cursors must be non-empty: %q %q", c1, c2)
	}

	if c3 != "" {
		t.Fatalf("final cursor = %q, want empty", c3)
	}

	// Every row exactly once, page order tracking the full order.
	seen := append(listIDs(p1), listIDs(p2)...)
	seen = append(seen, listIDs(p3)...)

	if !reflect.DeepEqual(seen, wantFull) {
		t.Fatalf("paged sequence %v != full order %v", seen, wantFull)
	}

	// Stale cursor: delete the session c1 points at (page1's last row) — the
	// next page still returns a consistent page (best-effort shift, D-05).
	deadPath := filepath.Join(dir, ".ass-guard", "transcript_"+listUUID(5)+".jsonl")

	err := os.Remove(deadPath)
	if err != nil {
		t.Fatalf("remove stale row: %v", err)
	}

	p2b, c2b := mustList(t, dir, c1, listTestPageSize)
	wantIDs(t, p2b, wantFull[listTestPageSize:2*listTestPageSize], "page2 after deletion")

	if c2b != c2 {
		t.Fatalf("cursor after deletion = %q, want %q", c2b, c2)
	}
}

func TestSessionListTombstoneFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := listTestBase()

	liveID, deadID := listUUID(1), listUUID(2)
	livePath := writeListTranscript(t, dir, liveID, standardListLines(t, liveID, base, "live"))
	deadPath := writeListTranscript(t, dir, deadID, standardListLines(t, deadID, base, "dead"))

	setListMtime(t, livePath, base.Add(time.Hour))
	setListMtime(t, deadPath, base)

	// Zero-byte tombstone beside the transcript (D-07 spelling: <id>.deleted).
	marker := filepath.Join(dir, ".ass-guard", deadID+".deleted")

	err := os.WriteFile(marker, nil, filePermOwner)
	if err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// The tombstoned transcript is UNREADABLE: if the engine opened it, the
	// open failure would surface. Absence plus no error proves the stat filter
	// ran BEFORE any open — zero transcript bytes read (D-07 read side).
	err = os.Chmod(deadPath, 0)
	if err != nil {
		t.Fatalf("chmod dead transcript: %v", err)
	}

	got, _ := mustList(t, dir, "", 0)
	wantIDs(t, got, []string{liveID}, "tombstone filter")

	fi, err := os.Stat(marker)
	if err != nil {
		t.Fatalf("stat marker: %v", err)
	}

	if fi.Size() != 0 {
		t.Fatalf("marker size = %d, want 0", fi.Size())
	}
}

func TestSessionListEmptyAndSingle(t *testing.T) {
	t.Parallel()

	base := listTestBase()

	// Empty store (.ass-guard exists, no transcripts).
	empty := t.TempDir()

	err := os.MkdirAll(filepath.Join(empty, ".ass-guard"), dirPerm)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	got, next := mustList(t, empty, "", 0)
	if len(got) != 0 || next != "" {
		t.Fatalf("empty store: got %d rows, next %q", len(got), next)
	}

	// Missing store dir entirely — same empty answer, not an error.
	missing := filepath.Join(t.TempDir(), "absent")

	got2, next2 := mustList(t, missing, "", 0)
	if len(got2) != 0 || next2 != "" {
		t.Fatalf("missing store: got %d rows, next %q", len(got2), next2)
	}

	// Single session: one row, empty nextCursor; the empty-string cursor IS
	// the first page.
	single := t.TempDir()
	only := listUUID(9)
	path := writeListTranscript(t, single, only, standardListLines(t, only, base, "only one"))
	setListMtime(t, path, base)

	got3, next3 := mustList(t, single, "", 0)
	wantIDs(t, got3, []string{only}, "single store")

	if next3 != "" {
		t.Fatalf("single store nextCursor = %q, want empty", next3)
	}
}

func TestSessionListHeaderShape(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := listTestBase()

	// Long multi-byte prompt: rune-safe truncation to 80 runes.
	longPrompt := strings.Repeat("é", listTestPromptLen)
	promptID := listUUID(1)
	ppath := writeListTranscript(
		t, dir, promptID, standardListLines(t, promptID, base, longPrompt))

	// No user_message at all → fallback literal, TitlePresent false.
	noneID := listUUID(2)
	npath := writeListTranscript(t, dir, noneID, []Line{
		listStartLine(noneID, base),
		{Type: TypeBoundary, Timestamp: base, Cause: "test-filler"},
	})

	// Prompt exists but BEYOND the 64 KiB bounded prefix → still the fallback.
	farID := listUUID(3)
	fpath := writeListTranscript(t, dir, farID, []Line{
		listStartLine(farID, base),
		{Type: TypeEngineDecision, Timestamp: base, Text: strings.Repeat("f", listTestFillerSize)},
		listUserLine(t, farID, base, "never read"),
	})

	// Checkpointed session: a real store snapshot (EARLY-01 layout).
	cpID := listUUID(4)
	cpath := writeListTranscript(t, dir, cpID, standardListLines(t, cpID, base, "checkpointed"))

	setListMtime(t, ppath, base.Add(3*time.Hour))
	setListMtime(t, npath, base.Add(2*time.Hour))
	setListMtime(t, fpath, base.Add(time.Hour))
	setListMtime(t, cpath, base)

	cp, err := checkpoint.Open(dir)
	if err != nil {
		t.Fatalf("checkpoint open: %v", err)
	}

	err = cp.Snapshot(context.Background(), cpID, cpID+"-turn-001")
	if err != nil {
		t.Fatalf("checkpoint snapshot: %v", err)
	}

	got, _ := mustList(t, dir, "", 0)

	by := map[string]SessionHeader{}
	for _, h := range got {
		by[h.SessionID] = h
	}

	if len(by) != 4 {
		t.Fatalf("rows = %d, want 4", len(by))
	}

	checkHeaderRows(t, by, base, promptID, noneID, farID, cpID, longPrompt)
}

// checkHeaderRows asserts the four fixture rows' header shapes.
func checkHeaderRows(
	t *testing.T, by map[string]SessionHeader, base time.Time,
	promptID, noneID, farID, cpID, longPrompt string,
) {
	t.Helper()

	h := by[promptID]
	wantTitle := string([]rune(longPrompt)[:listTestTitleCut])

	if h.Title != wantTitle || !h.TitlePresent {
		t.Fatalf("title = %q (present %v), want %d-rune cut", h.Title, h.TitlePresent, listTestTitleCut)
	}

	if !h.CreatedAt.Equal(base) {
		t.Fatalf("createdAt = %v, want the session_start timestamp %v", h.CreatedAt, base)
	}

	if !h.LastActivity.Equal(base.Add(3 * time.Hour)) {
		t.Fatalf("lastActivity = %v, want mtime", h.LastActivity)
	}

	if h.HasCheckpoints {
		t.Fatalf("prompt session must have no checkpoints")
	}

	n := by[noneID]
	if n.Title != listTestFallback || n.TitlePresent {
		t.Fatalf("no-prompt title = %q (present %v), want fallback literal", n.Title, n.TitlePresent)
	}

	f := by[farID]
	if f.Title != listTestFallback || f.TitlePresent {
		t.Fatalf("beyond-prefix title = %q (present %v), want fallback literal", f.Title, f.TitlePresent)
	}

	c := by[cpID]
	if !c.HasCheckpoints {
		t.Fatalf("checkpointed session must report HasCheckpoints")
	}

	if !c.TitlePresent || c.Title != "checkpointed" {
		t.Fatalf("checkpointed title = %q (present %v)", c.Title, c.TitlePresent)
	}
}

func TestSessionListMalformedCursor(t *testing.T) {
	t.Parallel()

	base := listTestBase()

	// Store with an UNREADABLE transcript: a malformed cursor must be
	// rejected BEFORE any scan — if the engine touched the file, the open
	// failure would surface instead of the typed cursor error.
	dir := t.TempDir()
	writeListTranscript(t, dir, listUUID(1), standardListLines(t, listUUID(1), base, "ok"))
	unreadable := writeListTranscript(t, dir, listUUID(2), standardListLines(t, listUUID(2), base, "no"))

	err := os.Chmod(unreadable, 0)
	if err != nil {
		t.Fatalf("chmod: %v", err)
	}

	bad := []string{
		"!!not-base64url!!",
		b64("garbage-without-pipe"),
		b64("not-a-nano|" + listUUID(3)),
		b64("123|!!invalid-id"),
		b64("123|"),
		b64(strings.Repeat("y", 400)), // over the 256-byte cap (T-18-06)
	}

	for _, cursor := range bad {
		_, _, lerr := ListSessions(dir, cursor, 0)
		if !errors.Is(lerr, ErrInvalidCursor) {
			t.Errorf("cursor %q: want ErrInvalidCursor, got %v", cursor, lerr)
		}
	}

	// Well-formed cursor naming a session that does not exist: comparator
	// input, never an error. Far-future timestamp ranks every row after it.
	clean := t.TempDir()
	id := listUUID(1)
	path := writeListTranscript(t, clean, id, standardListLines(t, id, base, "clean"))
	setListMtime(t, path, base)

	got, next := mustList(t, clean, b64("9000000000000000000|"+listUUID(9)), 0)
	wantIDs(t, got, []string{id}, "far-future cursor page")

	if next != "" {
		t.Fatalf("far-future cursor next = %q, want empty", next)
	}
}

func TestSessionListCorruptFirstLine(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := listTestBase()

	good := listUUID(1)
	writeListTranscript(t, dir, good, standardListLines(t, good, base, "good"))

	// First line is garbage; a valid user_message follows. The session is
	// skipped without error (first line must be a conforming session_start).
	bad := listUUID(2)

	tail, err := json.Marshal(listUserLine(t, bad, base, "unreachable"))
	if err != nil {
		t.Fatalf("marshal tail: %v", err)
	}

	body := append([]byte("{not json\n"), tail...)
	body = append(body, '\n')

	corrupt := filepath.Join(dir, ".ass-guard", "transcript_"+bad+".jsonl")

	err = os.WriteFile(corrupt, body, filePermOwner)
	if err != nil {
		t.Fatalf("write corrupt transcript: %v", err)
	}

	got, _ := mustList(t, dir, "", 0)
	wantIDs(t, got, []string{good}, "corrupt first line")
}

// TestSessionListLegacyShapeOpener pins the transcript shape REAL sessions
// produce (G-18-1): pre-fix transcripts begin with a user_message line, not
// the hand-written session_start opener the fixtures modeled — the whole list
// battery was green against a shape real sessions never produce (the G-17-1
// fixture-fiction class). A first line of ANY known type with a valid
// timestamp is the legacy createdAt fallback; unknown types and zero
// timestamps stay skipped — the tolerance is a bounded known-type whitelist,
// never accept-anything.
func TestSessionListLegacyShapeOpener(t *testing.T) { //nolint:funlen // four shape pins in the battery's subtest idiom
	t.Parallel()

	base := listTestBase()

	t.Run("real shape lists", func(t *testing.T) {
		t.Parallel()

		// The ~/tmp/perm-uat shape: user_message first, assistant_message
		// second — MUST enumerate with createdAt from the FIRST line exactly.
		dir := t.TempDir()
		live := listUUID(2)
		path := writeListTranscript(t, dir, live, []Line{
			listUserLine(t, live, base, "first real prompt"),
			{Type: TypeAssistantMessage, TurnID: live + "-turn-001",
				Timestamp: base.Add(time.Minute), Text: "reply"},
		})
		setListMtime(t, path, base.Add(2*time.Hour)) // lastActivity distinct from createdAt

		got, _ := mustList(t, dir, "", 0)
		wantIDs(t, got, []string{live}, "real shape")

		if !got[0].CreatedAt.Equal(base) {
			t.Fatalf("createdAt = %v, want the user_message line's timestamp %v exactly",
				got[0].CreatedAt, base)
		}

		if !got[0].LastActivity.Equal(base.Add(2 * time.Hour)) {
			t.Fatalf("lastActivity = %v, want the pinned mtime", got[0].LastActivity)
		}
	})

	t.Run("single-prompt legacy title", func(t *testing.T) {
		t.Parallel()

		// Knock-on 4 (accepted consequence, G-18-1 fix_direction b): the
		// legacy opener consumes line 1 as the createdAt fallback, the title
		// scan starts at line 2, and a single-prompt legacy session shows the
		// fallback title with TitlePresent false.
		dir := t.TempDir()
		only := listUUID(3)
		writeListTranscript(t, dir, only, []Line{listUserLine(t, only, base, "the only prompt")})

		got, _ := mustList(t, dir, "", 0)
		wantIDs(t, got, []string{only}, "single-prompt legacy")

		if got[0].Title != listTestFallback || got[0].TitlePresent {
			t.Fatalf("title = %q (present %v), want fallback literal with TitlePresent false",
				got[0].Title, got[0].TitlePresent)
		}
	})

	t.Run("still skipped unknown type", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		good := listUUID(1)
		writeListTranscript(t, dir, good, standardListLines(t, good, base, "good"))

		// A VALID timestamp but a type outside the known vocabulary; the
		// conforming tail proves the engine honors ONLY line 1.
		future := listUUID(4)
		writeListTranscript(t, dir, future, []Line{
			{Type: "future_kind", Timestamp: base},
			listStartLine(future, base),
			listUserLine(t, future, base, "unreachable"),
		})

		got, _ := mustList(t, dir, "", 0)
		wantIDs(t, got, []string{good}, "unknown opener type still skipped")
	})

	t.Run("still skipped zero timestamp", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		good := listUUID(1)
		writeListTranscript(t, dir, good, standardListLines(t, good, base, "good"))

		// A KNOWN type with a zero timestamp; same conforming tail.
		zero := listUUID(5)
		writeListTranscript(t, dir, zero, []Line{
			{Type: TypeUserMessage, Timestamp: time.Time{}},
			listStartLine(zero, base),
			listUserLine(t, zero, base, "unreachable"),
		})

		got, _ := mustList(t, dir, "", 0)
		wantIDs(t, got, []string{good}, "zero-timestamp opener still skipped")
	})
}

func TestSessionListBounds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := listTestBase()

	for n := 1; n <= listTestStoreBig; n++ {
		id := listUUID(n)
		path := writeListTranscript(t, dir, id, standardListLines(t, id, base, "row"))
		setListMtime(t, path, base.Add(time.Duration(n)*time.Second))
	}

	// Full order: mtime desc → ids 120..1.
	want := make([]string, listTestStoreBig)
	for n := range want {
		want[n] = listUUID(listTestStoreBig - n)
	}

	// limit 0 → default page of 50 plus a continuation cursor.
	p, next := mustList(t, dir, "", 0)
	if len(p) != listTestDefaultPg || next == "" {
		t.Fatalf("limit 0: %d rows, next %q", len(p), next)
	}

	wantIDs(t, p, want[:listTestDefaultPg], "default page head")

	// limit > 100 clamps to 100.
	p2, next2 := mustList(t, dir, "", listTestLimitHuge)
	if len(p2) != listTestClampPg || next2 == "" {
		t.Fatalf("limit %d: %d rows, next %q", listTestLimitHuge, len(p2), next2)
	}

	wantIDs(t, p2, want[:listTestClampPg], "clamped page head")

	// Exhaustion under the clamp: 100 + 20, then an empty cursor.
	q2, c2 := mustList(t, dir, next2, listTestLimitHuge)
	if len(q2) != listTestStoreBig-listTestClampPg || c2 != "" {
		t.Fatalf("tail page: %d rows, cursor %q", len(q2), c2)
	}

	seen := append(listIDs(p2), listIDs(q2)...)
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("clamped paging did not cover the store exactly once")
	}
}
