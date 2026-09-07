package acpserve //nolint:testpackage // internal package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// TestCommandsNotifySeam (20-01/Task 3, ACP-04): the commands advertisement
// re-fire wiring at the COMPOSITION level. Two sessions started on one serve
// each receive a complete available_commands_update before their session/new
// response — v1 full-replacement semantics means EVERY fire (the session
// start today; the 20-05 rescan swap through the same NotifyAllAvailableCommands
// seam) carries the COMPLETE winner set, and consecutive fires never assume
// client-side merge.
//
//nolint:funlen // one composition scenario, one serve
func TestCommandsNotifySeam(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":` +
		`{"protocolVersion":1,"clientCapabilities":{},` +
		`"clientInfo":{"name":"test","version":"0"}}}
` +
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"x"}}
` +
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"x"}}
`)

	var (
		stdout threadlessBuffer
		stderr bytes.Buffer
	)

	ctx := t.Context()

	err := Run(ctx, in, &stdout, &stderr, &Options{
		Profile: profileZcode, MaxConcurrent: 6,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
	})
	if err != nil && !errors.Is(err, io.EOF) {
		t.Logf("serve returned %v (acceptable)", err)
	}

	out := stdout.String()

	// Collect every available_commands_update frame's winner set.
	type frameSet struct {
		sessionID string
		names     []string
	}

	var sets []frameSet

	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if !strings.Contains(line, acp.KindAvailableCommandsUpdate) {
			continue
		}

		var m struct {
			Params struct {
				SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
				Update    struct {
					Kind string                      `json:"sessionUpdate"`     //nolint:tagliatelle // ACP wire field
					Cmds []acp.AvailableCommandFrame `json:"availableCommands"` //nolint:tagliatelle // ACP wire field
				} `json:"update"`
			} `json:"params"`
		}

		jerr := json.Unmarshal([]byte(line), &m)
		if jerr != nil {
			t.Fatalf("decode advertisement frame %q: %v", line, jerr)
		}

		if m.Params.Update.Kind != acp.KindAvailableCommandsUpdate {
			continue
		}

		names := make([]string, 0, len(m.Params.Update.Cmds))
		for _, c := range m.Params.Update.Cmds {
			names = append(names, c.Name)
		}

		sets = append(sets, frameSet{sessionID: m.Params.SessionID, names: names})
	}

	if len(sets) < 2 {
		t.Fatalf("advertisement fires = %d; want >= 2 (one per session start); stdout:\n%s", len(sets), out)
	}

	// Every fire carries the COMPLETE winner set: the live builtins (status,
	// init) must be present in EACH frame.
	for _, s := range sets {
		has := make(map[string]bool, len(s.names))
		for _, n := range s.names {
			has[n] = true
		}

		for _, want := range []string{"status", "init"} {
			if !has[want] {
				t.Errorf("session %s advertisement missing builtin %q (full winner set per fire): %v",
					s.sessionID, want, s.names)
			}
		}
	}

	// Consecutive fires are each self-contained (identical complete sets —
	// discovery is stable within one serve).
	for i := 1; i < len(sets); i++ {
		if strings.Join(sets[i].names, ",") != strings.Join(sets[0].names, ",") {
			t.Errorf("fire %d winner set %v differs from fire 0 %v (full-replacement sets must each be complete)",
				i, sets[i].names, sets[0].names)
		}
	}
}

// threadlessBuffer is a plain bytes.Buffer alias (Run requires an io.Writer;
// tests read only after Run returns).
type threadlessBuffer = bytes.Buffer
