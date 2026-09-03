package main

// 18-06 Task 1 tests (D-10, the CC-parity resume trio): flag parsing at both
// spellings and all three --resume forms (bare sentinel / = form / space
// form), cwd-scoped --continue resolution (newest NON-tombstoned row), the
// <id|name> resolution semantics (exact id pass-through, case-insensitive
// title prefix, ambiguity, miss, traversal rejection before any file access),
// and the root RunE delegation seam into the serve flow with the resolved
// target. The picker itself is Task 2 (TestPicker*); these tests inject the
// pick seam.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// flagResumeCLI is the --resume flag's literal (goconst: repeated across the
// parsing and delegation batteries).
const flagResumeCLI = "--resume"

// resumeFixture is one seeded store row: a conforming two-line transcript
// (session_start opener + user_message title) named for id, mtime-pinned to
// lastActivity (the list engine's LastActivity source), optionally
// tombstoned (D-07 marker beside the transcript).
type resumeFixture struct {
	id           string
	title        string
	lastActivity time.Time
	tombstone    bool
}

// writeResumeFixtureStore seeds dir/.ass-guard/ with the fixture rows.
func writeResumeFixtureStore(t *testing.T, dir string, rows ...resumeFixture) {
	t.Helper()

	store := filepath.Join(dir, ".ass-guard")

	err := os.MkdirAll(store, 0o750)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	for _, row := range rows {
		ts := row.lastActivity.UTC().Format(time.RFC3339Nano)

		title, merr := json.Marshal(row.title)
		if merr != nil {
			t.Fatalf("marshal title: %v", merr)
		}

		body := `{"type":"session_start","timestamp":"` + ts + `"}` + "\n" +
			`{"type":"user_message","turnID":"` + row.id + `-turn-001","timestamp":"` + ts + `",` +
			`"content":[{"type":"text","text":` + string(title) + `}]}` + "\n"

		path := filepath.Join(store, "transcript_"+row.id+".jsonl")

		werr := os.WriteFile(path, []byte(body), 0o600)
		if werr != nil {
			t.Fatalf("write transcript %s: %v", row.id, werr)
		}

		terr := os.Chtimes(path, row.lastActivity, row.lastActivity)
		if terr != nil {
			t.Fatalf("chtimes %s: %v", row.id, terr)
		}

		if row.tombstone {
			mkerr := os.WriteFile(filepath.Join(store, row.id+".deleted"), nil, 0o600)
			if mkerr != nil {
				t.Fatalf("write tombstone %s: %v", row.id, mkerr)
			}
		}
	}
}

// parseRootFlags builds the real root command, parses args into its flag set
// (no RunE execution), and returns the command plus the trio state read the
// way the RunEs read it.
func parseRootFlags(t *testing.T, args ...string) (*cobra.Command, resumeFlags) {
	t.Helper()

	root := newRootCmd()

	err := root.ParseFlags(args)
	if err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}

	return root, readResumeFlags(root, root.Flags().Args())
}

// interceptServeDelegate swaps runServeWithResumeTarget for a capture seam
// (the runACPServeCmd extraction precedent — the delegation is observable
// without spawning a real serve) and restores it at cleanup.
func interceptServeDelegate(t *testing.T) *resumeServeDelegate {
	t.Helper()

	orig := runServeWithResumeTarget

	captured := &resumeServeDelegate{}

	runServeWithResumeTarget = func(_ context.Context, d resumeServeDelegate) error {
		*captured = d

		return nil
	}

	t.Cleanup(func() { runServeWithResumeTarget = orig })

	return captured
}

// interceptPicker swaps the pickResumeSession composition edge for a capture
// + fixed-answer seam and restores it at cleanup.
func interceptPicker(t *testing.T, answer string) *[]session.SessionHeader {
	t.Helper()

	orig := pickResumeSession

	var seen []session.SessionHeader

	pickResumeSession = func(rows []session.SessionHeader) (string, error) {
		seen = rows

		return answer, nil
	}

	t.Cleanup(func() { pickResumeSession = orig })

	return &seen
}

// TestResumeFlagParsing pins the trio's cobra surface on the ROOT command:
// bare --resume parses to the picker sentinel, the = form carries the target
// directly, the space form leaves the target in args (the NoOptDefVal
// contract), --continue/-c both set the bool, and combining any resume flag
// with --prompt is the typed combination error.
func TestResumeFlagParsing(t *testing.T) { //nolint:funlen // table battery + the conflict case, one subtest each
	t.Parallel()

	const id = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"

	t.Run("flag forms parse per the NoOptDefVal contract", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name       string
			args       []string
			wantResume string
			wantArgs   []string
			wantCont   bool
		}{
			{"bare --resume parses to the picker sentinel", []string{flagResumeCLI},
				pickSentinel, nil, false},
			{"--resume=<id> carries the id", []string{flagResumeCLI + "=" + id},
				id, nil, false},
			{"--resume <id> space form leaves the id in args", []string{flagResumeCLI, id},
				pickSentinel, []string{id}, false},
			{"--continue sets the bool", []string{"--continue"}, "", nil, true},
			{"-c sets the bool", []string{"-c"}, "", nil, true},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, rf := parseRootFlags(t, tc.args...)
				assertResumeFlags(t, rf, tc.wantResume, tc.wantCont, tc.wantArgs)
			})
		}
	})

	t.Run("--resume with --prompt is the typed combination error", func(t *testing.T) {
		t.Parallel()

		captured := interceptServeDelegate(t)

		root := newRootCmd()
		root.SetArgs([]string{flagResumeCLI, id, "--prompt", "hi"})

		err := root.Execute()
		if err == nil {
			t.Fatal("root RunE accepted --resume + --prompt; want the typed conflict")
		}

		if !errors.Is(err, errResumeWithPrompt) {
			t.Fatalf("combination error = %v; want errResumeWithPrompt", err)
		}

		for _, name := range []string{flagResumeCLI, "--continue", "--prompt"} {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("combination error %q does not name %s", err.Error(), name)
			}
		}

		if captured.target != "" {
			t.Errorf("delegation ran despite the conflict (target=%q)", captured.target)
		}
	})
}

// TestContinueResolvesMostRecentCwd pins --continue's cwd scoping (CC's
// "most recent interactive session in the current directory"): the newest
// NON-tombstoned row of THAT directory's store; an empty store is a clean
// not-found error naming the directory searched.
func TestContinueResolvesMostRecentCwd(t *testing.T) {
	t.Parallel()

	base := time.Now().Add(-time.Hour)

	const (
		oldID = "11111111-1111-4111-8111-111111111111"
		midID = "22222222-2222-4222-8222-222222222222"
		newID = "33333333-3333-4333-8333-333333333333"
	)

	dir := t.TempDir()

	writeResumeFixtureStore(t, dir,
		resumeFixture{id: oldID, title: "oldest session", lastActivity: base},
		resumeFixture{id: midID, title: "middle session", lastActivity: base.Add(10 * time.Minute)},
		resumeFixture{
			id: newID, title: "newest but deleted", lastActivity: base.Add(20 * time.Minute),
			tombstone: true,
		},
	)

	got, err := resolveResumeTarget(dir, resumeFlags{cont: true}, nil)
	if err != nil {
		t.Fatalf("--continue resolution: %v", err)
	}

	if got != midID {
		t.Errorf("--continue = %s; want the newest non-tombstoned %s", got, midID)
	}

	empty := t.TempDir()

	_, err = resolveResumeTarget(empty, resumeFlags{cont: true}, nil)
	if !errors.Is(err, errResumeNoSessions) {
		t.Fatalf("empty-store --continue error = %v; want errResumeNoSessions", err)
	}

	if !strings.Contains(err.Error(), empty) {
		t.Errorf("empty-store error %q does not name the searched directory %s", err.Error(), empty)
	}
}

// assertResumeFlags asserts one parsed trio state against its expectation.
func assertResumeFlags(t *testing.T, rf resumeFlags, wantResume string, wantCont bool, wantArgs []string) {
	t.Helper()

	if rf.resume != wantResume {
		t.Errorf("resume = %q; want %q", rf.resume, wantResume)
	}

	if rf.cont != wantCont {
		t.Errorf("cont = %v; want %v", rf.cont, wantCont)
	}

	if len(rf.args) != len(wantArgs) {
		t.Fatalf("args = %v; want %v", rf.args, wantArgs)
	}

	for i := range wantArgs {
		if rf.args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q; want %q", i, rf.args[i], wantArgs[i])
		}
	}
}

// assertInvalidTarget asserts the typed pre-scan rejection of one hostile
// target against the given store directory.
func assertInvalidTarget(t *testing.T, dir, hostile string) {
	t.Helper()

	_, err := resolveResumeTarget(dir, resumeFlags{resume: hostile}, nil)
	if !errors.Is(err, errResumeInvalidTarget) {
		t.Fatalf("target %q against %s: %v; want errResumeInvalidTarget", hostile, dir, err)
	}
}

// TestResumeTargetResolution pins the <id|name> semantics: pattern-valid ids
// pass through untouched, names match title prefixes case-insensitively,
// ambiguity lists every candidate, a miss names the directory searched, and
// traversal-shaped targets reject BEFORE any store scan.
func TestResumeTargetResolution(t *testing.T) { //nolint:funlen // the seven-case resolution battery, one subtest each
	t.Parallel()

	base := time.Now().Add(-time.Hour)

	const (
		fixID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		loginID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		alpha1  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		alpha2  = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)

	dir := t.TempDir()

	writeResumeFixtureStore(t, dir,
		resumeFixture{id: fixID, title: "unrelated title", lastActivity: base},
		resumeFixture{id: loginID, title: "Fix the Login Flow", lastActivity: base.Add(5 * time.Minute)},
		resumeFixture{id: alpha1, title: "alpha one", lastActivity: base.Add(6 * time.Minute)},
		resumeFixture{id: alpha2, title: "alpha two", lastActivity: base.Add(7 * time.Minute)},
	)

	t.Run("exact pattern-valid id resolves to itself", func(t *testing.T) {
		t.Parallel()

		const absent = "99999999-9999-4999-8999-999999999999"

		got, err := resolveResumeTarget(dir, resumeFlags{resume: absent}, nil)
		if err != nil {
			t.Fatalf("id resolution: %v", err)
		}

		if got != absent {
			t.Errorf("id %s resolved to %q; want itself (existence is the load engine's error)", absent, got)
		}
	})

	t.Run("case-insensitive title prefix resolves", func(t *testing.T) {
		t.Parallel()

		got, err := resolveResumeTarget(dir, resumeFlags{resume: "fix the log"}, nil)
		if err != nil {
			t.Fatalf("name resolution: %v", err)
		}

		if got != loginID {
			t.Errorf("name %q resolved to %s; want %s", "fix the log", got, loginID)
		}
	})

	t.Run("ambiguous prefix errors listing both candidates", func(t *testing.T) {
		t.Parallel()

		_, err := resolveResumeTarget(dir, resumeFlags{resume: "ALPHA"}, nil)
		if !errors.Is(err, errResumeAmbiguousName) {
			t.Fatalf("ambiguous error = %v; want errResumeAmbiguousName", err)
		}

		for _, want := range []string{alpha1, alpha2, "alpha one", "alpha two", dir} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ambiguous error %q does not list %q", err.Error(), want)
			}
		}
	})

	t.Run("miss errors naming the directory searched", func(t *testing.T) {
		t.Parallel()

		_, err := resolveResumeTarget(dir, resumeFlags{resume: "zzz-no-such"}, nil)
		if !errors.Is(err, errResumeUnknownName) {
			t.Fatalf("miss error = %v; want errResumeUnknownName", err)
		}

		if !strings.Contains(err.Error(), dir) {
			t.Errorf("miss error %q does not name the searched directory %s", err.Error(), dir)
		}
	})

	t.Run("traversal-shaped target rejects before any file access", func(t *testing.T) {
		t.Parallel()

		// The typed rejection against the populated store…
		assertInvalidTarget(t, dir, "../../../etc/passwd")

		// …and against a store directory that does not exist: the same typed
		// reject proves NO scan ran (a scan would have errored differently or
		// found nothing — never this typed reject).
		assertInvalidTarget(t, t.TempDir(), "../../../etc/passwd")
	})

	t.Run("bare sentinel funnels the store rows through the pick seam", func(t *testing.T) {
		t.Parallel()

		var seen []session.SessionHeader

		pick := func(rows []session.SessionHeader) (string, error) {
			seen = rows

			return rows[1].SessionID, nil
		}

		got, err := resolveResumeTarget(dir, resumeFlags{resume: pickSentinel}, pick)
		if err != nil {
			t.Fatalf("picker-mode resolution: %v", err)
		}

		if len(seen) == 0 {
			t.Fatal("pick seam saw no rows; want the cwd store's recency-ordered headers")
		}

		if want := seen[1].SessionID; got != want {
			t.Errorf("picker-mode resolution = %s; want the picked %s", got, want)
		}
	})

	t.Run("prompt combination is the typed conflict", func(t *testing.T) {
		t.Parallel()

		_, err := resolveResumeTarget(dir, resumeFlags{resume: "x", prompt: "y"}, nil)
		if !errors.Is(err, errResumeWithPrompt) {
			t.Fatalf("resolution with --prompt = %v; want errResumeWithPrompt", err)
		}
	})
}

// TestRootResumeDelegation pins the root RunE branch: a resume flag set and
// no --prompt delegates to the serve flow (the testable seam) carrying the
// resolved target — direct id form, bare-picker form, and the cwd-scoped -c
// form.
//
//nolint:paralleltest // swaps package-level seam vars + t.Chdir — deliberately sequential
func TestRootResumeDelegation(t *testing.T) {
	// NOT parallel: the subtests swap package-level seams (delegation + picker).
	const id = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"

	t.Run("--resume <id> delegates with the resolved target", func(t *testing.T) {
		captured := interceptServeDelegate(t)

		root := newRootCmd()
		root.SetArgs([]string{flagResumeCLI, id})

		err := root.Execute()
		if err != nil {
			t.Fatalf("root Execute: %v", err)
		}

		if captured.target != id {
			t.Errorf("delegated target = %q; want %q", captured.target, id)
		}
	})

	t.Run("bare --resume delegates with the picked id", func(t *testing.T) {
		const picked = "77777777-7777-4777-8777-777777777777"

		chdirPickerFixture(t) // picker mode lists the cwd store first

		captured := interceptServeDelegate(t)
		interceptPicker(t, picked)

		root := newRootCmd()
		root.SetArgs([]string{flagResumeCLI})

		err := root.Execute()
		if err != nil {
			t.Fatalf("root Execute: %v", err)
		}

		if captured.target != picked {
			t.Errorf("delegated target = %q; want the picked %q", captured.target, picked)
		}
	})

	t.Run("-c delegates with the cwd-newest id", func(t *testing.T) {
		fresh := chdirContinueFixture(t)

		captured := interceptServeDelegate(t)

		root := newRootCmd()
		root.SetArgs([]string{"-c"})

		err := root.Execute()
		if err != nil {
			t.Fatalf("root Execute: %v", err)
		}

		if captured.target != fresh {
			t.Errorf("delegated target = %q; want the cwd-newest %q", captured.target, fresh)
		}
	})
}

// chdirPickerFixture seeds a one-session cwd and chdirs into it (the bare
// --resume delegation case: picker mode lists the cwd store first).
func chdirPickerFixture(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	writeResumeFixtureStore(t, dir,
		resumeFixture{id: "66666666-6666-4666-8666-666666666666", title: "the listed row",
			lastActivity: time.Now().Add(-time.Minute), tombstone: false})
	t.Chdir(dir)
}

// chdirContinueFixture seeds a two-session cwd (distinct mtimes) and chdirs
// into it, returning the newer session's id (the -c delegation case).
func chdirContinueFixture(t *testing.T) string {
	t.Helper()

	const (
		stale = "44444444-4444-4444-8444-444444444444"
		fresh = "55555555-5555-4555-8555-555555555555"
	)

	dir := t.TempDir()
	writeResumeFixtureStore(t, dir,
		resumeFixture{id: stale, title: "older", lastActivity: time.Now().Add(-2 * time.Hour)},
		resumeFixture{id: fresh, title: "newer", lastActivity: time.Now().Add(-1 * time.Hour)},
	)

	t.Chdir(dir) // --continue is cwd-scoped (D-10/CC parity)

	return fresh
}

// --- 18-06 Task 2: the D-11 numbered picker (pipe-safe terminal I/O) ---

// pickerRows builds three distinct header rows for the picker tests.
func pickerRows(now time.Time) []session.SessionHeader {
	return []session.SessionHeader{
		{SessionID: "sess_row_one", Title: "first session", TitlePresent: true,
			LastActivity: now.Add(-90 * time.Second)},
		{SessionID: "sess_row_two", Title: "second session", TitlePresent: true,
			LastActivity: now.Add(-3 * time.Hour)},
		{SessionID: "sess_row_three", Title: "third session", TitlePresent: true,
			LastActivity: now.Add(-3 * 24 * time.Hour)},
	}
}

// TestPickerSelectsByNumber: three rows on the writer as
// `N) <title>  (<rel time>)` plus ONE prompt line; stdin "2" selects the
// second id.
func TestPickerSelectsByNumber(t *testing.T) {
	t.Parallel()

	now := time.Now()
	rows := pickerRows(now)

	var out bytes.Buffer

	id, err := SelectSession(rows, strings.NewReader("2\n"), &out)
	if err != nil {
		t.Fatalf("SelectSession: %v", err)
	}

	if id != rows[1].SessionID {
		t.Errorf("selected %q; want the second row %q", id, rows[1].SessionID)
	}

	rendered := out.String()

	// One line per row plus one prompt line — exactly.
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("picker wrote %d lines; want %d rows + 1 prompt:\n%s", len(lines), len(rows), rendered)
	}

	for i, want := range []string{
		"1) first session  (", "2) second session  (", "3) third session  (",
	} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("row %d = %q; want prefix %q", i+1, lines[i], want)
		}
	}

	if !strings.Contains(lines[0], "m ago") { // 90s -> the minutes bucket
		t.Errorf("row 1 relative time = %q; want the minutes bucket", lines[0])
	}

	if !strings.Contains(lines[len(lines)-1], "?") {
		t.Errorf("last line %q is not a selection prompt", lines[len(lines)-1])
	}
}

// TestPickerRetriesOnceThenFails: one invalid entry re-prompts; a SECOND
// invalid entry errors; immediate EOF errors; an empty stdin line counts as
// invalid (and a valid retry after it succeeds).
func TestPickerRetriesOnceThenFails(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("second invalid entry errors", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer

		_, err := SelectSession(pickerRows(now), strings.NewReader("abc\n9\n"), &out)
		if err == nil {
			t.Fatal("picker accepted two invalid entries; want the typed error")
		}

		if n := strings.Count(out.String(), "?"); n != 2 {
			t.Errorf("prompt printed %d times; want exactly 2 (initial + one re-prompt): %q", n, out.String())
		}
	})

	t.Run("EOF immediately errors", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer

		_, err := SelectSession(pickerRows(now), strings.NewReader(""), &out)
		if err == nil {
			t.Fatal("picker returned nil error on immediate EOF")
		}
	})

	t.Run("empty line counts as invalid then a valid retry succeeds", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer

		id, err := SelectSession(pickerRows(now), strings.NewReader("\n3\n"), &out)
		if err != nil {
			t.Fatalf("picker: %v", err)
		}

		if id != "sess_row_three" {
			t.Errorf("selected %q; want the third row after the retry", id)
		}
	})
}

// TestPickerRelativeTime pins FormatRelativeTime's buckets by word (the
// exact locale formatting is discretion; the BUCKET is the contract): 90s ->
// minutes, 3h -> hours, 3d -> days, 30d -> weeks, 0-age -> just-now.
func TestPickerRelativeTime(t *testing.T) {
	t.Parallel()

	now := time.Now()

	cases := []struct {
		age  time.Duration
		want string
	}{
		{0, "just now"},
		{30 * time.Second, "just now"},
		{90 * time.Second, "m ago"},
		{3 * time.Hour, "h ago"},
		{3 * 24 * time.Hour, "d ago"},
		{30 * 24 * time.Hour, "w ago"},
	}

	for _, tc := range cases {
		got := FormatRelativeTime(now.Add(-tc.age), now)
		if !strings.Contains(got, tc.want) {
			t.Errorf("FormatRelativeTime(-%v) = %q; want the %q bucket", tc.age, got, tc.want)
		}
	}
}

// TestPickerPipeSafe drives SelectSession through an io.Pipe reader and a
// bytes.Buffer writer — the exact surfaces `ass-guard --resume` uses over
// ssh/pipes (no *os.File anywhere in the signature or the drive path).
func TestPickerPipeSafe(t *testing.T) {
	t.Parallel()

	now := time.Now()
	rows := pickerRows(now)

	pr, pw := io.Pipe()

	go func() {
		_, _ = pw.Write([]byte("1\n"))
		_ = pw.Close()
	}()

	var out bytes.Buffer

	id, err := SelectSession(rows, pr, &out)
	if err != nil {
		t.Fatalf("pipe-driven SelectSession: %v", err)
	}

	if id != rows[0].SessionID {
		t.Errorf("selected %q; want the first row %q", id, rows[0].SessionID)
	}
}

// TestPickerTruncatesTitles: a 200-rune title renders as EXACTLY 60 runes
// (unicode-safe) followed by the two-space time bucket.
func TestPickerTruncatesTitles(t *testing.T) {
	t.Parallel()

	now := time.Now()
	long := strings.Repeat("α", 200) // multibyte — proves rune-safe truncation

	rows := []session.SessionHeader{{
		SessionID: "sess_long", Title: long, TitlePresent: true,
		LastActivity: now.Add(-90 * time.Second),
	}}

	var out bytes.Buffer

	_, err := SelectSession(rows, strings.NewReader("1\n"), &out)
	if err != nil {
		t.Fatalf("SelectSession: %v", err)
	}

	first, _, _ := strings.Cut(out.String(), "\n")

	// Row shape: "1) " + 60 runes + "  (" + bucket + ")".
	if !strings.HasPrefix(first, "1) ") {
		t.Fatalf("row = %q; want the numbered prefix", first)
	}

	rest := strings.TrimPrefix(first, "1) ")

	title, _, found := strings.Cut(rest, "  (")
	if !found {
		t.Fatalf("row %q lacks the two-space time bucket separator", first)
	}

	if got := len([]rune(title)); got != 60 {
		t.Errorf("title rendered as %d runes; want exactly 60", got)
	}

	if title != strings.Repeat("α", 60) {
		t.Errorf("title = %.60q…; want the first 60 α runes", title)
	}
}
