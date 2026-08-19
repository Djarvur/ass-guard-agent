package profile_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// openContextBehaviorFixture opens a committed fixture under
// testdata/context-behavior/.
func openContextBehaviorFixture(t *testing.T, name string) *os.File {
	t.Helper()

	f, err := os.Open(filepath.Join(".", "testdata", "context-behavior", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	return f
}

// TestScanContextBehavior_CacheControlFixture pins cache_control counting and
// placement classification against the committed redacted corpus excerpt
// (14-02 Task 1, Test 1). Every occurrence counts exactly once; placements
// classify into system-block-index / tools-decl / message-content-block
// classes; unknown placements land in the EXPOSED other bucket — never
// silently dropped.
func TestScanContextBehavior_CacheControlFixture(t *testing.T) {
	t.Parallel()

	rep, err := profile.ScanContextBehavior(openContextBehaviorFixture(t, "cache-control.jsonl"))
	if err != nil {
		t.Fatalf("ScanContextBehavior: %v", err)
	}

	want := map[string]int{
		"system:index=0":                      2, // both records, block 0
		"system:index=1":                      2,
		"system:index=2":                      2,
		"tools:index=1":                       1, // synthetic probe, record 2
		"message:role=tool:block=tool_result": 1, // synthetic probe, record 2
	}

	for key, n := range want {
		if rep.CacheControlPlacements[key] != n {
			t.Errorf("CacheControlPlacements[%q] = %d, want %d", key, rep.CacheControlPlacements[key], n)
		}
	}

	if len(rep.CacheControlPlacements) != len(want) {
		t.Errorf("CacheControlPlacements has %d classes, want exactly %d (extra: %v)",
			len(rep.CacheControlPlacements), len(want), rep.CacheControlPlacements)
	}

	// The unknown body-nest probe must land in the EXPOSED other bucket, not
	// vanish and not pollute the known classes.
	if rep.CacheControlOther["request.body.metadata.cache_control"] != 1 {
		t.Errorf("CacheControlOther[request.body.metadata.cache_control] = %d, want 1",
			rep.CacheControlOther["request.body.metadata.cache_control"])
	}

	if len(rep.CacheControlOther) != 1 {
		t.Errorf("CacheControlOther has %d entries, want exactly 1: %v",
			len(rep.CacheControlOther), rep.CacheControlOther)
	}

	if rep.ScannedLines != 2 {
		t.Errorf("ScannedLines = %d, want 2", rep.ScannedLines)
	}

	if rep.ParseErrors != 0 {
		t.Errorf("ParseErrors = %d, want 0", rep.ParseErrors)
	}
}

// TestScanContextBehavior_WindowShape pins the request-window arithmetic
// (14-02 Task 1, Test 2): per-request message counts, messagesKind census,
// monotonic-offset continuation vs reset/eviction classification. The
// committed fixture mirrors the observed rolling 64-message tail and ends with
// a fabricated eviction-style probe (messageCount regression).
func TestScanContextBehavior_WindowShape(t *testing.T) {
	t.Parallel()

	rep, err := profile.ScanContextBehavior(openContextBehaviorFixture(t, "eviction-window.jsonl"))
	if err != nil {
		t.Fatalf("ScanContextBehavior: %v", err)
	}

	if rep.WindowShape.Requests != 6 {
		t.Errorf("Requests = %d, want 6", rep.WindowShape.Requests)
	}

	if rep.WindowShape.KindCounts["full"] != 4 || rep.WindowShape.KindCounts["tail"] != 2 {
		t.Errorf("KindCounts = %v, want full=4 tail=2", rep.WindowShape.KindCounts)
	}

	// Message-array length distribution: 7, 20, 64, 64, 64, 30.
	for length, n := range map[int]int{7: 1, 20: 1, 64: 3, 30: 1} {
		if rep.WindowShape.TailLengths[length] != n {
			t.Errorf("TailLengths[%d] = %d, want %d", length, rep.WindowShape.TailLengths[length], n)
		}
	}

	// The fabricated eviction probe (full record with messageCount 30 < the
	// running max 68) must classify as a reset — history shrink.
	if !rep.WindowShape.ResetObserved {
		t.Error("ResetObserved = false, want true (fabricated eviction-style history shrink)")
	}

	// No NON-FULL offset regression exists in the fixture (the shrink record is
	// kind=full, which re-anchors at offset 0 by construction), so the offset
	// chain itself stays monotonic — the two classifications are independent.
	if !rep.WindowShape.MonotonicOffsets {
		t.Error("MonotonicOffsets = false, want true (no non-full offset regression in fixture)")
	}

	// Continuation-only stream (inline): advancing offsets, growing counts —
	// no reset, monotonic. Proves the negative side of the classification.
	cont := strings.NewReader(strings.Join([]string{
		windowLine("full", 0, 4, 4),
		windowLine("tail", 2, 6, 4),
		windowLine("tail", 4, 8, 4),
	}, "\n") + "\n")

	crep, err := profile.ScanContextBehavior(cont)
	if err != nil {
		t.Fatalf("ScanContextBehavior (continuation): %v", err)
	}

	if !crep.WindowShape.MonotonicOffsets || crep.WindowShape.ResetObserved {
		t.Errorf("continuation stream: MonotonicOffsets=%v ResetObserved=%v, want true/false",
			crep.WindowShape.MonotonicOffsets, crep.WindowShape.ResetObserved)
	}

	// Offset-regression probe WITHOUT a count regression (count keeps
	// advancing): the window start moves backward — still a reset signal, and
	// the offset chain is no longer monotonic.
	reg := strings.NewReader(
		windowLine("tail", 2, 6, 4) + "\n" + windowLine("tail", 0, 8, 4) + "\n")

	rrep, err := profile.ScanContextBehavior(reg)
	if err != nil {
		t.Fatalf("ScanContextBehavior (regression): %v", err)
	}

	if rrep.WindowShape.MonotonicOffsets || !rrep.WindowShape.ResetObserved {
		t.Errorf("offset-regression stream: MonotonicOffsets=%v ResetObserved=%v, want false/true",
			rrep.WindowShape.MonotonicOffsets, rrep.WindowShape.ResetObserved)
	}
}

// TestScanContextBehavior_CompactionMarkers pins the literal shape-key
// extraction (14-02 Task 1, Test 3): compact-shaped message forms are counted
// per distinct shape key from STRUCTURAL prefixes — ordinary conversation text
// that merely mentions the word "compact" is never counted.
func TestScanContextBehavior_CompactionMarkers(t *testing.T) {
	t.Parallel()

	recordA := "{\"type\":\"model_io\",\"request\":{\"messagesKind\":\"full\",\"messageOffset\":0,\"messageCount\":2," +
		"\"messages\":[" +
		"{\"role\":\"user\",\"content\":\"<system-reminder>\\ncontext placeholder\\n</system-reminder>\"}," +
		"{\"role\":\"assistant\",\"content\":\"ph\"}]," +
		"\"body\":{\"system\":[]}}}"
	recordB := "{\"type\":\"model_io\",\"request\":{\"messagesKind\":\"full\",\"messageOffset\":0,\"messageCount\":3," +
		"\"messages\":[" +
		"{\"role\":\"user\",\"content\":\"This session is being continued from a previous conversation. ph\"}," +
		"{\"role\":\"user\",\"content\":\"lets talk about compaction and summary, ordinary text\"}," +
		"{\"role\":\"system\",\"content\":\"inline system placeholder\"}]," +
		"\"body\":{\"system\":[]}}}"

	input := strings.NewReader(recordA + "\n" + recordB + "\n")

	rep, err := profile.ScanContextBehavior(input)
	if err != nil {
		t.Fatalf("ScanContextBehavior: %v", err)
	}

	want := map[string]int{
		"system-reminder":             1,
		"compact-continuation-header": 1,
		"inline-system-message":       1,
	}

	for key, n := range want {
		if rep.CompactionMarkers[key] != n {
			t.Errorf("CompactionMarkers[%q] = %d, want %d", key, rep.CompactionMarkers[key], n)
		}
	}

	if len(rep.CompactionMarkers) != len(want) {
		t.Errorf("CompactionMarkers has %d keys, want exactly %d (extra: %v)",
			len(rep.CompactionMarkers), len(want), rep.CompactionMarkers)
	}
}

// TestScanContextBehavior_EmptyInput pins the degenerate inputs (14-02 Task 1,
// Test 4): an empty reader yields a zero report with no error; a malformed
// line is counted as a parse error and NEVER aborts the whole scan.
func TestScanContextBehavior_EmptyInput(t *testing.T) {
	t.Parallel()

	rep, err := profile.ScanContextBehavior(strings.NewReader(""))
	if err != nil {
		t.Fatalf("empty reader: %v", err)
	}

	if rep.ScannedLines != 0 || rep.ParseErrors != 0 || rep.WindowShape.Requests != 0 {
		t.Errorf("empty reader: ScannedLines=%d ParseErrors=%d Requests=%d, want all zero",
			rep.ScannedLines, rep.ParseErrors, rep.WindowShape.Requests)
	}

	if len(rep.CacheControlPlacements) != 0 || len(rep.CacheControlOther) != 0 || len(rep.CompactionMarkers) != 0 {
		t.Errorf("empty reader: maps not empty: %v %v %v",
			rep.CacheControlPlacements, rep.CacheControlOther, rep.CompactionMarkers)
	}

	mixed := strings.NewReader("{not json\n" + windowLine("full", 0, 1, 1) + "\n")

	mrep, err := profile.ScanContextBehavior(mixed)
	if err != nil {
		t.Fatalf("malformed-line scan: %v", err)
	}

	if mrep.ParseErrors != 1 {
		t.Errorf("ParseErrors = %d, want 1", mrep.ParseErrors)
	}

	if mrep.ScannedLines != 1 || mrep.WindowShape.Requests != 1 {
		t.Errorf("scan aborted on malformed line: ScannedLines=%d Requests=%d, want 1/1",
			mrep.ScannedLines, mrep.WindowShape.Requests)
	}
}

// windowLine builds one minimal rollout-shaped JSONL record for the window
// tests (placeholder payloads only).
func windowLine(kind string, offset, count, n int) string {
	msgs := make([]string, n)

	for i := range msgs {
		role := "user"

		if i%2 == 1 {
			role = "assistant"
		}

		msgs[i] = "{\"role\":\"" + role + "\",\"content\":\"ph-" + strconv.Itoa(i) + "\"}"
	}

	return "{\"type\":\"model_io\",\"request\":{\"messagesKind\":\"" + kind + "\"," +
		"\"messageOffset\":" + strconv.Itoa(offset) + ",\"messageCount\":" + strconv.Itoa(count) + "," +
		"\"messages\":[" + strings.Join(msgs, ",") + "],\"body\":{\"system\":[]}}}"
}
