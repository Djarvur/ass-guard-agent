package session

// Session enumeration engine (18-03, ACP-05 / D-04..D-07): an on-demand
// header scan over dir/.ass-guard/ transcripts — NO index file, nothing to
// drift, the append-only discipline preserved (D-04). Per transcript the scan
// touches ONLY the first line (createdAt from the opener line's Timestamp —
// the preferred session_start, or any known type under the G-18-1 legacy
// fallback) plus a bounded prefix (<= 64 KiB, stopping at the first
// user_message line) for the title, plus os.Stat for mtime, tombstones, and
// checkpoint availability — never a full transcript read.
//
// Ordering is a stable total order (D-05): lastActivity (mtime) descending,
// sessionId ascending as the deterministic tie-break. The opaque cursor
// encodes (lastActivity unixNano, sessionId) of the last emitted row, so the
// next page returns sessions strictly after that tuple and never re-emits a
// row across pages. PAGING STABILITY IS BEST-EFFORT (D-05 — picker utility
// over paging purity): a session that becomes active mid-paging can shift
// across pages, and a cursor pointing at a since-deleted session still
// returns a consistent page (the tuple is a comparator, not a row reference).
//
// Tombstones (D-07) are zero-byte <id>.deleted siblings filtered by os.Stat
// existence alone — zero transcript bytes and zero marker bytes read; the
// marker's single home (the write side) is internal/session/tombstone.go in
// 18-04. Headers stay lean (D-06): no token/cost fields (Phase 20's /cost).

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// List engine bounds (D-06 title cut, D-04 prefix cap, T-18-06 DoS guards).
const (
	defaultListPageSize = 50
	maxListPageSize     = 100
	listTitleMaxRunes   = 80
	listPrefixMaxBytes  = 64 * 1024
	listReadBufSize     = 8 * 1024
	listCursorMaxLen    = 256
	fallbackTitle       = "(no prompt)"
)

// Store-layout spellings (transcript.go openTranscript + the D-07 marker and
// the EARLY-01 checkpoint store's loose-ref directory).
const (
	storeDirName         = ".ass-guard"
	transcriptFilePrefix = "transcript_"
	transcriptFileSuffix = ".jsonl"
	tombstoneSuffix      = ".deleted"
	listCursorSep        = "|"
	checkpointRefsPath   = "checkpoints/shadow.git/refs/checkpoints"
	turnRefInfix         = "-turn-"
)

// ErrInvalidCursor is the typed rejection for a malformed list cursor
// (T-18-06): length-capped and shape-validated BEFORE any scan, so a hostile
// cursor string is never parsed, looped over, or turned into file reads.
var ErrInvalidCursor = errors.New("session: invalid list cursor")

// sessIDPattern is the session package's local copy of the traversal-safe id
// grammar (provenance: internal/coreexec/messaging.go — the captured zcode
// schema's sess_* branch OR ass-guard's own RFC 4122 v4 UUID branch; the
// per-package copy is the project convention, T-12-04-03). Both alternatives
// exclude path separators, so an id validated here cannot escape the store
// directory through any later filepath.Join.
var sessIDPattern = regexp.MustCompile(
	`^(sess_[A-Za-z0-9._-]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// SessionHeader is one lean list row (D-06). Title is the first user prompt
// truncated to 80 runes; TitlePresent distinguishes a real prompt from the
// fallback literal — downstream consumers (the 18-04 wire mapping, the 18-06
// picker) branch on the FLAG, never on string comparison against the literal.
type SessionHeader struct {
	SessionID      string
	Title          string
	TitlePresent   bool
	CreatedAt      time.Time
	LastActivity   time.Time
	HasCheckpoints bool
}

// listCursor is the composite pagination tuple (D-05): the (lastActivity,
// sessionId) of the last emitted row, opaque on the wire (base64url of
// "unixNano|sessionId").
type listCursor struct {
	lastActivity time.Time
	sessionID    string
}

// ListSessions enumerates the sessions recorded under dir/.ass-guard/ as
// lean headers ordered by (lastActivity desc, sessionId asc), one page at a
// time. cursor is the opaque continuation token a previous page returned
// ("" means first page); limit <= 0 selects the default page of 50 and any
// limit over 100 is clamped to 100 (T-18-06). The returned nextCursor is
// empty when no rows remain. A malformed cursor yields an error wrapping
// ErrInvalidCursor before any directory scan.
func ListSessions(dir, cursor string, limit int) ([]SessionHeader, string, error) {
	var after listCursor

	if cursor != "" {
		c, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}

		after = c
	}

	pageSize := listPageSize(limit)
	store := filepath.Join(dir, storeDirName)

	entries, err := os.ReadDir(store)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SessionHeader{}, "", nil
		}

		return nil, "", fmt.Errorf("session: list: read store: %w", err)
	}

	headers, serr := scanStoreHeaders(store, dir, entries)
	if serr != nil {
		return nil, "", serr
	}

	slices.SortFunc(headers, func(a, b SessionHeader) int {
		if !a.LastActivity.Equal(b.LastActivity) {
			return b.LastActivity.Compare(a.LastActivity) // lastActivity desc
		}

		return strings.Compare(a.SessionID, b.SessionID) // id asc tie-break
	})

	if cursor != "" {
		headers = filterAfterCursor(headers, after)
	}

	if len(headers) > pageSize {
		page := headers[:pageSize]
		last := page[pageSize-1]

		return page, encodeCursor(listCursor{lastActivity: last.LastActivity, sessionID: last.SessionID}), nil
	}

	return headers, "", nil
}

// listPageSize normalizes the caller's limit: <= 0 selects the default, and
// anything over the max is clamped (T-18-06 unbounded-paging guard).
func listPageSize(limit int) int {
	if limit <= 0 {
		return defaultListPageSize
	}

	return min(limit, maxListPageSize)
}

// scanStoreHeaders walks the store entries once, collecting the headers of
// every enumerable session (transcript-named, pattern-valid, regular,
// un-tombstoned, conforming opener). Open failures on structurally valid
// transcripts propagate (the SessionReader.read precedent) — a file that
// passed every structural check but cannot be read is an anomaly worth
// surfacing, not skipping.
func scanStoreHeaders(store, dir string, entries []os.DirEntry) ([]SessionHeader, error) {
	owners := checkpointOwners(dir)
	headers := make([]SessionHeader, 0, len(entries))

	for _, e := range entries {
		h, keep, err := scanStoreEntry(store, e, owners)
		if err != nil {
			return nil, err
		}

		if keep {
			headers = append(headers, h)
		}
	}

	return headers, nil
}

// scanStoreEntry classifies one store directory entry: derivable id, regular
// file (T-18-07 planted entries), no tombstone sibling (D-07), and a
// conforming opener. keep is false for every skipped shape.
func scanStoreEntry(store string, e os.DirEntry, owners map[string]struct{}) (SessionHeader, bool, error) {
	id, ok := listTranscriptID(e.Name())
	if !ok || !sessIDPattern.MatchString(id) {
		return SessionHeader{}, false, nil
	}

	if !e.Type().IsRegular() {
		return SessionHeader{}, false, nil // planted symlink/dir/fifo entries
	}

	_, terr := os.Stat(filepath.Join(store, id+tombstoneSuffix))
	if terr == nil {
		return SessionHeader{}, false, nil // tombstoned: stat alone (D-07)
	}

	fi, _ := e.Info() // vanished between ReadDir and Info: best-effort skip
	if fi == nil {
		return SessionHeader{}, false, nil
	}

	h, keep, rerr := readTranscriptHeader(filepath.Join(store, e.Name()), id)
	if rerr != nil {
		return SessionHeader{}, false, rerr
	}

	if !keep {
		return SessionHeader{}, false, nil
	}

	h.LastActivity = fi.ModTime()

	if _, has := owners[id]; has {
		h.HasCheckpoints = true
	}

	return h, true, nil
}

// listTranscriptID derives the session id from a transcript_<id>.jsonl file
// name; ok is false for every other store entry.
func listTranscriptID(name string) (string, bool) {
	if !strings.HasPrefix(name, transcriptFilePrefix) || !strings.HasSuffix(name, transcriptFileSuffix) {
		return "", false
	}

	return strings.TrimSuffix(strings.TrimPrefix(name, transcriptFilePrefix), transcriptFileSuffix), true
}

// readTranscriptHeader reads ONLY the first line (createdAt — the opener is
// the PREFERRED session_start line, or any KNOWN type with a valid timestamp
// under the G-18-1 legacy fallback for pre-fix transcripts) plus the bounded
// prefix that follows, stopping at the first user_message line (the title).
// keep is false when the transcript is non-conforming (corrupt first line,
// unknown opener type, zero timestamp) and the session is skipped without
// error.
func readTranscriptHeader(path, sessionID string) (SessionHeader, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionHeader{}, false, fmt.Errorf("session: list: open transcript: %w", err)
	}

	defer func() { _ = f.Close() }()

	// io.LimitReader pins the TOTAL bytes ever read from the file at the
	// 64 KiB prefix cap regardless of bufio's buffering (D-04).
	br := bufio.NewReaderSize(io.LimitReader(f, listPrefixMaxBytes), listReadBufSize)

	h, ok := readHeaderOpener(sessionID, br)
	if !ok {
		return SessionHeader{}, false, nil
	}

	readHeaderTitle(&h, br)

	return h, true, nil
}

// readHeaderOpener consumes lines up to and including the first non-empty one
// and validates it as the conforming opener: the preferred session_start
// line, or any KNOWN line type with a valid timestamp under the G-18-1
// legacy fallback (pre-fix transcripts began with user_message lines). ok is
// false for a corrupt opener, an unknown line type, a zero timestamp, or an
// empty file.
func readHeaderOpener(sessionID string, br *bufio.Reader) (SessionHeader, bool) {
	h := SessionHeader{SessionID: sessionID, Title: fallbackTitle}

	for {
		line, rerr := br.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")

		if conformingOpener(line, &h) {
			return h, true
		}

		if len(line) > 0 || rerr != nil {
			// A non-empty line that failed the opener check is corrupt or
			// non-conforming; EOF or a drained prefix ends the scan.
			return SessionHeader{}, false
		}
	}
}

// knownOpenerTypes is the bounded whitelist of the transcript vocabulary's
// own line kinds (the Type* constants in transcript.go — the same vocabulary
// the reader discipline already defines). G-18-1 legacy fallback: pre-fix
// transcripts begin with an ordinary line (user_message in practice) because
// nothing ever called AppendSessionStart, so enumeration accepts any KNOWN
// kind with a valid timestamp as the createdAt source — while an unknown type
// stays rejected: the tolerance is bounded to the vocabulary, never
// accept-anything (T-18-07-01).
var knownOpenerTypes = map[string]struct{}{ //nolint:gochecknoglobals // immutable whitelist table
	TypeSessionStart:      {},
	TypeUserMessage:       {},
	TypeRequestShaped:     {},
	TypeAgentMessageChunk: {},
	TypeAssistantMessage:  {},
	TypeToolCall:          {},
	TypeToolResult:        {},
	TypeBoundary:          {},
	TypeSubagentDispatch:  {},
	TypeSubagentResult:    {},
	TypeUsage:             {},
	TypeCanceled:          {},
	TypeEngineDecision:    {},
	TypeError:             {},
	TypeSessionEnd:        {},
	TypeCommandProvenance: {},
	TypeAskSuspended:      {},
	TypeRawThinking:       {},
	TypeLocalCommand:      {},
	TypeCompaction:        {},
	TypeMentionProvenance: {},
}

// knownOpenerType reports whether t is one of the transcript vocabulary's own
// line kinds (the knownOpenerTypes whitelist).
func knownOpenerType(t string) bool {
	_, ok := knownOpenerTypes[t]

	return ok
}

// conformingOpener parses line as the transcript's opener, recording the
// createdAt timestamp into h. TWO-TIER (G-18-1): the PREFERRED tier is the
// session_start line every post-fix transcript begins with (the shape
// sessionFor now writes); the LEGACY fallback tier accepts a first line of
// any OTHER known type with a non-zero timestamp, so pre-fix sessions stay
// enumerable forever. Still false for an empty, corrupt, unknown-type, or
// zero-timestamp line. The fixtures' hand-written session_start openers made
// the whole battery green against a shape real sessions never produced — the
// G-17-1 "tests modeled a fiction" lesson; do NOT re-tighten this to
// session_start-only. The legacy tier has a DOCUMENTED title consequence: the
// opener consumes line 1 as createdAt and readHeaderTitle then scans from
// line 2, so a legacy multi-prompt session's title is its SECOND user prompt
// and a single-prompt legacy session shows the fallback title — INTENTIONAL
// (the G-18-1 fix_direction kept the title scan unchanged); do NOT "fix"
// readHeaderTitle either.
func conformingOpener(line []byte, h *SessionHeader) bool {
	if len(line) == 0 {
		return false
	}

	var l Line

	if json.Unmarshal(line, &l) != nil {
		return false
	}

	if l.Timestamp.IsZero() {
		return false
	}

	// Preferred tier: the session_start opener.
	if l.Type == TypeSessionStart {
		h.CreatedAt = l.Timestamp

		return true
	}

	// Legacy fallback tier (G-18-1): any other known type with a valid
	// timestamp is a pre-fix transcript's de-facto opener.
	if knownOpenerType(l.Type) {
		h.CreatedAt = l.Timestamp

		return true
	}

	return false
}

// readHeaderTitle scans the bounded prefix after the opener for the first
// user_message line (the title). Never errors: non-conforming lines are
// skipped (the readers' discipline); EOF or a drained prefix keeps the
// fallback title with TitlePresent false.
func readHeaderTitle(h *SessionHeader, br *bufio.Reader) {
	for {
		line, rerr := br.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")

		if title, present, found := userTitleLine(line); found {
			h.Title, h.TitlePresent = title, present

			return
		}

		if rerr != nil {
			return
		}
	}
}

// userTitleLine parses one prefix line; found is true for a user_message
// line, carrying its title and TitlePresent flag.
func userTitleLine(line []byte) (title string, present, found bool) { //nolint:nonamedreturns // triple-result clarity
	if len(line) == 0 {
		return "", false, false
	}

	var l Line

	if json.Unmarshal(line, &l) != nil || l.Type != TypeUserMessage {
		return "", false, false
	}

	title, present = titleOfLine(&l)

	return title, present, true
}

// titleOfLine extracts the user_message title: the first text block's Text
// (the AppendUserMessage content shape), falling back to the line's flat
// Text field, truncated to listTitleMaxRunes (unicode-safe via []rune).
// present is false exactly when no prompt text was found (the caller keeps
// the fallback literal).
func titleOfLine(l *Line) (string, bool) {
	text := ""

	if len(l.Content) > 0 {
		var blocks []ContentBlock

		uerr := json.Unmarshal(l.Content, &blocks)
		if uerr == nil {
			for _, b := range blocks {
				if b.Type == blockText && b.Text != "" {
					text = b.Text

					break
				}
			}
		}
	}

	if text == "" {
		text = l.Text
	}

	if text == "" {
		return fallbackTitle, false
	}

	if runes := []rune(text); len(runes) > listTitleMaxRunes {
		return string(runes[:listTitleMaxRunes]), true
	}

	return text, true
}

// checkpointOwners probes the checkpoint store's on-disk layout (EARLY-01)
// for the sessions owning at least one turn ref: <dir>/.ass-guard/checkpoints/
// shadow.git/refs/checkpoints/ holds one LOOSE ref file per turn snapshot
// (<sessionID>-turn-<NNN>) because the store only ever creates refs via
// git update-ref and prunes via update-ref -d — it never packs. Pure ReadDir
// probe: no subprocess, and deliberately NOT checkpoint.Open (Open CREATES
// the store — a list scan must never write). Sessions the store never
// snapshotted (or whose refs were pruned) report HasCheckpoints false.
func checkpointOwners(dir string) map[string]struct{} {
	entries, err := os.ReadDir(filepath.Join(dir, storeDirName, filepath.FromSlash(checkpointRefsPath)))
	if err != nil {
		return nil
	}

	owners := make(map[string]struct{}, len(entries))

	for _, e := range entries {
		name := e.Name()

		i := strings.LastIndex(name, turnRefInfix)
		if i <= 0 || !isTurnSuffix(name[i+len(turnRefInfix):]) {
			continue
		}

		owners[name[:i]] = struct{}{}
	}

	return owners
}

// filterAfterCursor keeps the rows sorting strictly after the cursor tuple.
func filterAfterCursor(headers []SessionHeader, after listCursor) []SessionHeader {
	filtered := make([]SessionHeader, 0, len(headers))

	for _, h := range headers {
		if afterListCursor(&h, after) {
			filtered = append(filtered, h)
		}
	}

	return filtered
}

// afterListCursor reports whether h sorts strictly after the cursor tuple in
// the (lastActivity desc, sessionId asc) order: a smaller lastActivity, or
// an equal one with a greater id. The comparison is encoded once here so
// pagination and ordering cannot drift apart (D-05).
func afterListCursor(h *SessionHeader, c listCursor) bool {
	if !h.LastActivity.Equal(c.lastActivity) {
		return h.LastActivity.Before(c.lastActivity)
	}

	return h.SessionID > c.sessionID
}

// encodeCursor renders the composite tuple opaque: base64url of
// "unixNano|sessionId".
func encodeCursor(c listCursor) string {
	payload := strconv.FormatInt(c.lastActivity.UnixNano(), 10) + listCursorSep + c.sessionID

	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// decodeCursor validates and decodes an opaque cursor BEFORE any use: the
// raw string is length-capped (T-18-06), base64url-decoded, split on the
// tuple separator, the timestamp parsed, and the session id checked against
// the traversal-safe grammar. Any deviation is ErrInvalidCursor — never a
// parse loop, never a file read.
func decodeCursor(cursor string) (listCursor, error) {
	if len(cursor) > listCursorMaxLen {
		return listCursor{}, ErrInvalidCursor
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return listCursor{}, ErrInvalidCursor
	}

	nanoStr, id, found := strings.Cut(string(raw), listCursorSep)
	if !found {
		return listCursor{}, ErrInvalidCursor
	}

	nano, nerr := strconv.ParseInt(nanoStr, 10, 64)
	if nerr != nil || !sessIDPattern.MatchString(id) {
		return listCursor{}, ErrInvalidCursor
	}

	return listCursor{lastActivity: time.Unix(0, nano), sessionID: id}, nil
}
