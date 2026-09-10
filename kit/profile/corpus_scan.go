// corpus_scan.go (14-02, EARLY-02) is the MECHANICAL context-behavior census
// over zcode rollout JSONL. It answers, for any rollout stream: where
// cache_control appears (and in what placement class), which compact-shaped
// message forms exist (literal shape keys, never keyword interpretation), and
// what the request-window shape is (messagesKind census, per-request message
// counts, monotonic-offset continuation vs reset). It is the evidence
// generator behind docs/compaction-decision.md — the decision artifact's
// census numbers are this report over the named corpus sessions.

package profile

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// cacheControlKey is the wire field the census walks for.
const cacheControlKey = "cache_control"

// compactionShapePrefixes is the literal shape-key table for compact-shaped
// message forms: a message counts under a key iff its text content has the
// structural prefix. Ordinary conversation text that merely mentions the words
// "compact"/"summary" never matches — the extraction is literal, not
// interpreted (14-02 Task 1, Test 3).
var compactionShapePrefixes = []struct{ key, prefix string }{ //nolint:gochecknoglobals // immutable table
	// Mid-conversation harness-injected notice wrapper (zcode/claude-code form).
	{"system-reminder", "<system-reminder>"},
	// The canonical auto-compact continuation header a target emits when a
	// session resumes from a summarized/compacted prior conversation.
	{"compact-continuation-header", "This session is being continued from a previous conversation"},
}

// WindowShape classifies the request-window arithmetic of one rollout stream:
// per-request message counts, the messagesKind census, and whether the window
// bookkeeping shows monotonic continuation (rolling tail advancing) or a
// reset (history shrink / window-start regression — the eviction signature).
//
// Semantics (pinned by TestScanContextBehavior_WindowShape):
//   - messagesKind "full" re-anchors at messageOffset 0 by construction — a
//     recording-mode alternation, never an eviction; full records do not
//     participate in the offset-regression comparison as the LATER record.
//   - MonotonicOffsets is false iff some non-full record's offset is lower
//     than the previous offset-bearing record's offset.
//   - ResetObserved is true iff that regression occurs OR a record's
//     messageCount is below the running maximum (history shrink).
type WindowShape struct {
	Requests         int            // analyzed model_io records
	KindCounts       map[string]int // messagesKind -> record count
	TailLengths      map[int]int    // len(messages) -> request count
	MonotonicOffsets bool
	ResetObserved    bool
}

// ContextBehaviorReport is the mechanical census result of
// ScanContextBehavior over one rollout stream.
type ContextBehaviorReport struct {
	// CacheControlPlacements counts every cache_control occurrence by known
	// placement class: "system:index=N" (request.body.system block),
	// "tools:index=N" (request.body.tools decl), and
	// "message:role=R:block=T" (a request.messages[i].content[j] block of
	// block-type T on a role-R message).
	CacheControlPlacements map[string]int
	// CacheControlOther is the EXPOSED other bucket: cache_control occurrences
	// at paths outside the known classes, keyed by their literal JSON path.
	// Nothing is ever silently dropped (14-02 acceptance criterion).
	CacheControlOther map[string]int
	// CompactionMarkers counts compact-shaped message forms per literal shape
	// key (see compactionShapePrefixes) plus "inline-system-message" for
	// role=system messages carried inside request.messages.
	CompactionMarkers map[string]int
	// WindowShape carries the request-window arithmetic.
	WindowShape WindowShape
	// ParseErrors counts malformed lines; the scan always continues.
	ParseErrors int
	// ScannedLines counts analyzed model_io lines (comments/blanks/other
	// record types excluded).
	ScannedLines int
}

// scanMessage is one entry of the request-level messages array; only the
// census-relevant shape is modeled, content stays raw until needed.
type scanMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// texts returns the message's text payloads: string content directly, block
// arrays as their text blocks' text. Non-text shapes return nothing.
func (m scanMessage) texts() []string {
	if len(m.Content) == 0 {
		return nil
	}

	var s string

	err := json.Unmarshal(m.Content, &s)
	if err == nil {
		return []string{s}
	}

	var blocks []struct {
		Text string `json:"text"`
	}

	err = json.Unmarshal(m.Content, &blocks)
	if err == nil {
		out := make([]string, 0, len(blocks))

		for _, b := range blocks {
			if b.Text != "" {
				out = append(out, b.Text)
			}
		}

		return out
	}

	return nil
}

// windowTracker carries the cross-record window state (previous offset, running
// max messageCount) and folds each record's bookkeeping into the report.
type windowTracker struct {
	prevOffset *int
	maxCount   int
	anyCount   bool
}

// record applies the offset/count arithmetic to the report's WindowShape.
func (w *windowTracker) record(rep *ContextBehaviorReport, mio *ModelIO) {
	if mio.Request.MessageOffset != nil {
		off := *mio.Request.MessageOffset

		if w.prevOffset != nil && mio.Request.MessagesKind != "full" && off < *w.prevOffset {
			rep.WindowShape.MonotonicOffsets = false
			rep.WindowShape.ResetObserved = true
		}

		w.prevOffset = &off
	}

	if mio.Request.MessageCount != nil {
		cnt := *mio.Request.MessageCount

		if w.anyCount && cnt < w.maxCount {
			rep.WindowShape.ResetObserved = true
		}

		if !w.anyCount || cnt > w.maxCount {
			w.maxCount, w.anyCount = cnt, true
		}
	}
}

// ScanContextBehavior reads a zcode rollout JSONL stream (io.Reader — one
// session or a concatenation) and returns the mechanical context-behavior
// census. Lines starting with '#' (fixture provenance headers) and blank
// lines are skipped. Malformed lines increment ParseErrors and never abort
// the scan; the only error return is a reader failure. Empty input yields a
// zero report with no error.
func ScanContextBehavior(r io.Reader) (ContextBehaviorReport, error) {
	rep := ContextBehaviorReport{
		CacheControlPlacements: map[string]int{},
		CacheControlOther:      map[string]int{},
		CompactionMarkers:      map[string]int{},
		WindowShape: WindowShape{
			KindCounts:       map[string]int{},
			TailLengths:      map[int]int{},
			MonotonicOffsets: true,
		},
	}

	sc := bufio.NewScanner(r)
	// Rollout lines carry the full wire body per line; same bound as extract.go.
	sc.Buffer(make([]byte, scannerBufInit*scannerBufSize), 16*scannerBufSize*scannerBufSize)

	tracker := windowTracker{}

	for sc.Scan() {
		trimmed := strings.TrimSpace(sc.Text())
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		var mio ModelIO

		err := json.Unmarshal(sc.Bytes(), &mio)
		if err != nil {
			rep.ParseErrors++

			continue
		}

		if mio.Type != "model_io" {
			continue
		}

		rep.ScannedLines++
		rep.analyzeRecord(&mio)

		// Generic cache_control walk over the raw record — counts occurrences
		// at ANY nesting depth, classifying by path (unknown paths land in the
		// exposed other bucket).
		var generic any

		if json.Unmarshal(sc.Bytes(), &generic) == nil {
			walkCacheControl(&rep, generic, "", "", "")
		}

		tracker.record(&rep, &mio)
	}

	err := sc.Err()
	if err != nil {
		return rep, fmt.Errorf("scan context behavior: %w", err)
	}

	return rep, nil
}

// analyzeRecord folds one model_io record's window census and compaction
// markers into the report.
func (rep *ContextBehaviorReport) analyzeRecord(mio *ModelIO) {
	rep.WindowShape.Requests++
	rep.WindowShape.KindCounts[mio.Request.MessagesKind]++

	var msgs []scanMessage

	if len(mio.Request.Messages) > 0 {
		_ = json.Unmarshal(mio.Request.Messages, &msgs) // best-effort; empty on malformed
	}

	rep.WindowShape.TailLengths[len(msgs)]++

	for _, m := range msgs {
		if m.Role == "system" {
			rep.CompactionMarkers["inline-system-message"]++
		}

		for _, text := range m.texts() {
			for _, p := range compactionShapePrefixes {
				if strings.HasPrefix(text, p.prefix) {
					rep.CompactionMarkers[p.key]++
				}
			}
		}
	}
}

// walkCacheControl recursively walks a decoded JSON value counting every
// cache_control key occurrence, classifying by its built path. The role/block
// context rides the recursion and is HOISTED per map level (map iteration
// order is random): a map's own "role"/"type" string values name the
// enclosing message role and content-block type for anything nested in that
// map (classification only consumes them at request.messages paths).
func walkCacheControl(rep *ContextBehaviorReport, v any, path, role, block string) {
	switch x := v.(type) {
	case map[string]any:
		cRole, cBlock := role, block

		if s, ok := x["role"].(string); ok {
			cRole = s
		}

		if s, ok := x["type"].(string); ok {
			cBlock = s
		}

		for k, child := range x {
			childPath := k

			if path != "" {
				childPath = path + "." + k
			}

			if k == cacheControlKey {
				placeCacheControl(rep, childPath, cRole, cBlock)
			}

			walkCacheControl(rep, child, childPath, cRole, cBlock)
		}
	case []any:
		for i, child := range x {
			walkCacheControl(rep, child, path+"."+strconv.Itoa(i), role, block)
		}
	}
}

// placeCacheControl classifies one occurrence by its literal path. Known
// classes: request.body.system.<i>, request.body.tools.<i>, and
// request.messages.<i>.content.<j>. Everything else lands in the exposed
// other bucket keyed by the path — never dropped.
func placeCacheControl(rep *ContextBehaviorReport, path, role, block string) {
	seg := strings.Split(path, ".")

	if len(seg) == 5 && seg[0] == "request" && seg[1] == "body" &&
		(seg[2] == "system" || seg[2] == "tools") && seg[4] == cacheControlKey {
		rep.CacheControlPlacements[seg[2]+":index="+seg[3]]++

		return
	}

	if len(seg) == 6 && seg[0] == "request" && seg[1] == "messages" &&
		seg[3] == "content" && seg[5] == cacheControlKey {
		rep.CacheControlPlacements["message:role="+role+":block="+block]++

		return
	}

	rep.CacheControlOther[path]++
}
