package acp //nolint:testpackage // internal package test — golden pins on wire internals

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- 20-01 Task 3: available_commands_update + user_message_chunk wire contract ---

// staticCommandSource is a fixed CommandSource (golden fixtures need
// deterministic frames, not live discovery).
type staticCommandSource struct {
	cmds []AvailableCommandFrame
}

func (s staticCommandSource) AvailableCommands() []AvailableCommandFrame {
	return s.cmds
}

// goldenServe builds a Server whose notifications drain into a buffer and
// returns it with the buffer. The emitter is armed by NewServer; the Barrier
// flushes deterministically without running Serve.
//
//nolint:wsl_v5 // helper compactness
func goldenServe(t *testing.T, opts ...ServerOption) (*Server, *syncBuffer) {
	t.Helper()

	out := &syncBuffer{}

	srv := NewServer(strings.NewReader(""), out, &syncBuffer{}, opts...)
	t.Cleanup(func() { srv.emitter.Stop() })

	return srv, out
}

// flushAndDecode drains the emitter AND the async Writer drain, then decodes
// every stdout line as a Message.
func flushAndDecode(t *testing.T, srv *Server, out *syncBuffer) []Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	srv.emitter.Barrier(ctx)

	// The Writer is a second async hop (framer.go D-05) — settle-poll on
	// the SYNCHRONIZED buffer until the byte count stabilizes.
	time.Sleep(25 * time.Millisecond)

	settled := len(out.String())

	for range 20 {
		got := len(out.String())
		if strings.Count(out.String(), "\n") > 0 && got == settled {
			break
		}

		settled = got

		time.Sleep(25 * time.Millisecond)
	}

	var msgs []Message

	for line := range strings.SplitSeq(strings.TrimRight(out.String(), "\n"), "\n") {
		if line == "" {
			continue
		}

		var m Message

		err := json.Unmarshal([]byte(line), &m)
		if err != nil {
			t.Fatalf("decode frame %q: %v", line, err)
		}

		msgs = append(msgs, m)
	}

	return msgs
}

// TestAvailableCommandsUpdateGolden pins the available_commands_update wire
// shape byte-for-byte against the schema-derived fixture (ACP-04): params
// {sessionId, update{sessionUpdate, availableCommands[]}} with
// name/description always present and input omitted-when-empty (RESEARCH
// §Wire Vocabulary, fetched from schema/v1).
func TestAvailableCommandsUpdateGolden(t *testing.T) {
	t.Parallel()

	srv, out := goldenServe(t, WithCommandSource(staticCommandSource{cmds: []AvailableCommandFrame{
		{Name: "explore", Description: "Explore the change"},
		{Name: "model", Description: "Model pick", Input: &AvailableCommandInputFrame{Hint: "pick target"}},
	}}))

	err := srv.NotifyAvailableCommands("sess-av")
	if err != nil {
		t.Fatalf("NotifyAvailableCommands: %v", err)
	}

	msgs := flushAndDecode(t, srv, out)
	if len(msgs) != 1 {
		t.Fatalf("frames = %d; want 1 (raw: %s)", len(msgs), out.String())
	}

	if msgs[0].Method != methodSessionUpdate {
		t.Fatalf("method = %s; want %s", msgs[0].Method, methodSessionUpdate)
	}

	// Struct field order (Name, Description, Input) is the serialized order —
	// the golden pins the schema-derived KEY SPELLINGS and the nesting.
	const golden = `{"sessionId":"sess-av","update":{"availableCommands":[` +
		`{"name":"explore","description":"Explore the change"},` +
		`{"name":"model","description":"Model pick","input":{"hint":"pick target"}}],` +
		`"sessionUpdate":"available_commands_update"}}`

	if string(msgs[0].Params) != golden {
		t.Fatalf("params golden mismatch:\n got: %s\nwant: %s", msgs[0].Params, golden)
	}
}

// TestAvailableCommandsEmptySet pins the degrade: an absent CommandSource
// advertises the EMPTY set — serialized as [] (never null; a null would
// break the v1 array contract and clients holding no merge state).
func TestAvailableCommandsEmptySet(t *testing.T) {
	t.Parallel()

	srv, out := goldenServe(t)

	err := srv.NotifyAvailableCommands("sess-empty")
	if err != nil {
		t.Fatalf("NotifyAvailableCommands: %v", err)
	}

	msgs := flushAndDecode(t, srv, out)
	if len(msgs) != 1 {
		t.Fatalf("frames = %d; want 1 (the degrade NEVER skips the frame)", len(msgs))
	}

	if !strings.Contains(string(msgs[0].Params), `"availableCommands":[]`) {
		t.Fatalf("empty set not serialized as []: %s", msgs[0].Params)
	}
}

// TestAvailableCommandsFullReplacement pins v1 full-replacement + the 20-05
// re-fire contract: NotifyAllAvailableCommands called TWICE delivers two
// frames that EACH carry the COMPLETE winner set (the client holds no merge
// state — every send is self-contained).
func TestAvailableCommandsFullReplacement(t *testing.T) {
	t.Parallel()

	full := []AvailableCommandFrame{
		{Name: "alpha", Description: "A"},
		{Name: "beta", Description: "B"},
		{Name: "gamma", Description: "C"},
	}

	srv, out := goldenServe(t, WithCommandSource(staticCommandSource{cmds: full}))

	srv.mu.Lock()
	srv.sessions["sess-multi"] = &sessionState{id: "sess-multi"}
	srv.mu.Unlock()

	for range 2 {
		err := srv.NotifyAllAvailableCommands()
		if err != nil {
			t.Fatalf("NotifyAllAvailableCommands: %v", err)
		}
	}

	msgs := flushAndDecode(t, srv, out)
	if len(msgs) != 2 {
		t.Fatalf("frames = %d; want 2 (one per fire)", len(msgs))
	}

	for i, m := range msgs {
		upd := updateOf(t, &m)

		if upd[keySessionUpdate] != KindAvailableCommandsUpdate {
			t.Fatalf("frame %d kind = %v; want %s", i, upd[keySessionUpdate], KindAvailableCommandsUpdate)
		}

		cmds, ok := upd[keyAvailableCommands].([]any)
		if !ok || len(cmds) != len(full) {
			t.Fatalf("frame %d carries %v; want the COMPLETE %d-entry set", i, upd[keyAvailableCommands], len(full))
		}
	}
}

// TestUserMessageChunkEcho pins the D-05 echo frame: a user_message_chunk
// ContentChunk (content + messageId — Pitfall 5: the bare "user_message"
// kind does NOT exist in the v1 union and this golden goes red if the
// constant ever flips to it), with two distinct messageIds rendering two
// distinct messages.
func TestUserMessageChunkEcho(t *testing.T) {
	t.Parallel()

	rec := &recordingSink{}

	em := NewTurnEmitter(rec, &syncBuffer{}, TurnEmitterConfig{})
	defer em.Stop()

	fg := em.ForegroundHandle("sess-echo").(ActivityEmitter) //nolint:forcetypeassert // full surface

	err := fg.UserMessageChunk("turn-1:echo", "/status deep-check")
	if err != nil {
		t.Fatalf("echo chunk: %v", err)
	}

	err = fg.UserMessageChunk("turn-2:echo", "/cost")
	if err != nil {
		t.Fatalf("echo chunk 2: %v", err)
	}

	waitFor(t, "echo frames written", 2*time.Second, func() bool {
		return em.WrittenNotifications() == 2
	})

	got := rec.recorded()
	if len(got) != 2 {
		t.Fatalf("frames = %d; want 2", len(got))
	}

	first := updateOf(t, got[0])

	// Pinned against the LITERAL schema spelling (not the constant) so a
	// mutation of updKindUserMessageChunk to any other value goes red here.
	if first[keySessionUpdate] != "user_message_chunk" {
		t.Fatalf("echo kind = %v; want the v1 ContentChunk spelling (never bare)",
			first[keySessionUpdate])
	}

	if first[keyMessageID] != "turn-1:echo" {
		t.Fatalf("echo messageId = %v; want turn-1:echo", first[keyMessageID])
	}

	content, ok := first[keyContent].(map[string]any)
	if !ok || content["type"] != blockText || content["text"] != "/status deep-check" {
		t.Fatalf("echo content = %v; want the typed command text block", first[keyContent])
	}

	second := updateOf(t, got[1])
	if second[keyMessageID] == first[keyMessageID] {
		t.Fatalf("two echoes shared messageId %v; distinct ids render distinct messages", first[keyMessageID])
	}
}
