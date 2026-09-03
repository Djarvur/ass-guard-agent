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
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/session"
)

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

	if err := os.MkdirAll(store, 0o750); err != nil {
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

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write transcript %s: %v", row.id, err)
		}

		if err := os.Chtimes(path, row.lastActivity, row.lastActivity); err != nil {
			t.Fatalf("chtimes %s: %v", row.id, err)
		}

		if row.tombstone {
			if err := os.WriteFile(filepath.Join(store, row.id+".deleted"), nil, 0o600); err != nil {
				t.Fatalf("write tombstone %s: %v", row.id, err)
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

	if err := root.ParseFlags(args); err != nil {
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
func TestResumeFlagParsing(t *testing.T) {
	t.Parallel()

	const id = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"

	t.Run("bare --resume parses to the picker sentinel", func(t *testing.T) {
		t.Parallel()

		_, rf := parseRootFlags(t, "--resume")

		if rf.resume != pickSentinel {
			t.Errorf("bare --resume = %q; want the picker sentinel %q", rf.resume, pickSentinel)
		}

		if len(rf.args) != 0 {
			t.Errorf("bare --resume args = %v; want none", rf.args)
		}
	})

	t.Run("--resume=<id> carries the id", func(t *testing.T) {
		t.Parallel()

		_, rf := parseRootFlags(t, "--resume="+id)

		if rf.resume != id {
			t.Errorf("--resume=%s = %q; want the id", id, rf.resume)
		}
	})

	t.Run("--resume <id> space form leaves the id in args", func(t *testing.T) {
		t.Parallel()

		_, rf := parseRootFlags(t, "--resume", id)

		if rf.resume != pickSentinel {
			t.Errorf("space-form --resume = %q; want the sentinel (NoOptDefVal contract)", rf.resume)
		}

		if len(rf.args) != 1 || rf.args[0] != id {
			t.Errorf("space-form args = %v; want [%s]", rf.args, id)
		}
	})

	t.Run("--continue and -c both set the bool", func(t *testing.T) {
		t.Parallel()

		_, long := parseRootFlags(t, "--continue")
		if !long.cont {
			t.Error("--continue = false; want true")
		}

		_, short := parseRootFlags(t, "-c")
		if !short.cont {
			t.Error("-c = false; want true")
		}
	})

	t.Run("--resume with --prompt is the typed combination error", func(t *testing.T) {
		t.Parallel()

		captured := interceptServeDelegate(t)

		root := newRootCmd()
		root.SetArgs([]string{"--resume", id, "--prompt", "hi"})

		err := root.Execute()
		if err == nil {
			t.Fatal("root RunE accepted --resume + --prompt; want the typed conflict")
		}

		if !errors.Is(err, errResumeWithPrompt) {
			t.Fatalf("combination error = %v; want errResumeWithPrompt", err)
		}

		for _, name := range []string{"--resume", "--continue", "--prompt"} {
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

// TestResumeTargetResolution pins the <id|name> semantics: pattern-valid ids
// pass through untouched, names match title prefixes case-insensitively,
// ambiguity lists every candidate, a miss names the directory searched, and
// traversal-shaped targets reject BEFORE any store scan.
func TestResumeTargetResolution(t *testing.T) {
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

		const hostile = "../../../etc/passwd"

		_, err := resolveResumeTarget(dir, resumeFlags{resume: hostile}, nil)
		if !errors.Is(err, errResumeInvalidTarget) {
			t.Fatalf("traversal error = %v; want errResumeInvalidTarget", err)
		}

		// Before ANY file access: the same typed rejection fires against a
		// store directory that does not exist (a scan would have errored
		// differently or found nothing — never this typed reject).
		_, err = resolveResumeTarget(t.TempDir(), resumeFlags{resume: hostile}, nil)
		if !errors.Is(err, errResumeInvalidTarget) {
			t.Fatalf("traversal against a missing store = %v; want the same typed reject (no scan ran)", err)
		}
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
func TestRootResumeDelegation(t *testing.T) {
	// NOT parallel: the subtests swap package-level seams (delegation + picker).
	const id = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"

	t.Run("--resume <id> delegates with the resolved target", func(t *testing.T) {
		captured := interceptServeDelegate(t)

		root := newRootCmd()
		root.SetArgs([]string{"--resume", id})

		if err := root.Execute(); err != nil {
			t.Fatalf("root Execute: %v", err)
		}

		if captured.target != id {
			t.Errorf("delegated target = %q; want %q", captured.target, id)
		}
	})

	t.Run("bare --resume delegates with the picked id", func(t *testing.T) {
		const picked = "77777777-7777-4777-8777-777777777777"

		captured := interceptServeDelegate(t)
		interceptPicker(t, picked)

		root := newRootCmd()
		root.SetArgs([]string{"--resume"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root Execute: %v", err)
		}

		if captured.target != picked {
			t.Errorf("delegated target = %q; want the picked %q", captured.target, picked)
		}
	})

	t.Run("-c delegates with the cwd-newest id", func(t *testing.T) {
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

		captured := interceptServeDelegate(t)

		root := newRootCmd()
		root.SetArgs([]string{"-c"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root Execute: %v", err)
		}

		if captured.target != fresh {
			t.Errorf("delegated target = %q; want the cwd-newest %q", captured.target, fresh)
		}
	})
}
