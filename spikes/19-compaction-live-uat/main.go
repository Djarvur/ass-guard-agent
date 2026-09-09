// Command 19-compaction-live-uat is the automated stand-in for Phase 19's
// HUMAN-UAT test 1 (G-19-2 / SC-1c — "operator live-threshold retest").
//
// It does exactly what the operator would do in Zed, minus the human: builds
// the binary from HEAD, starts a local Anthropic-protocol mock provider,
// launches `ass-guard acp serve` against a scratch project dir, speaks ACP
// JSON-RPC over stdio, live-sets compaction-threshold to 50 via
// session/set_config_option, drives turns with ~420 KB replies until the
// FIRST and SECOND threshold compactions fire, exercises the overflow
// retry-once backstop (reject-by-size once, accept the compacted retry),
// restores the threshold to 80, and asserts quiescence — plus the on-disk
// evidence (transcript compaction markers, stderr compaction notes,
// persisted config layer, mock request log).
//
// Stdlib only. Run from the spikes module:
//
//	go run ./19-compaction-live-uat -repo /path/to/ass-guard-agent
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	bigReplyChars = 420_000 // ~105K tokens chars/4 — crosses the 50% line of a 200K window
	deltaChunk    = 16_000  // SSE text_delta chunk size
	summaryTag    = "AutoUAT compaction summary"
	turnTag       = "AutoUAT turn reply"
)

// --- mock provider ---

type mockReq struct {
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"` // "turn" | "summarize" | "turn-overflow-rejected"
	BodySize  int    `json:"bodySize"`
	Summaries []int  `json:"summariesPresent"` // which summary markers ride the body
	TurnNum   int    `json:"turnNum"`
}

type mock struct {
	mu               sync.Mutex
	srv              *http.Server
	seq              int
	summarizeN       int
	turnN            int
	rejectArm        bool // reject the next turn request once (overflow leg)
	rejectedBodySize int
	bigReplies       bool
	logFile          *os.File
	acceptedSizes    []int // body sizes of accepted turn requests
}

func (m *mock) start(logPath string) (string, error) {
	f, err := os.Create(logPath)
	if err != nil {
		return "", err
	}
	m.logFile = f

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", m.handle)

	m.srv = &http.Server{Handler: mux} //nolint:gosec // scratch harness on loopback

	go func() { _ = m.srv.Serve(ln) }()

	return "http://" + ln.Addr().String(), nil
}

func (m *mock) log(entry mockReq) {
	raw, _ := json.Marshal(entry)
	m.logFile.Write(append(raw, '\n')) //nolint:errcheck // evidence log, best effort
}

type wireMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func textOf(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			sb.WriteString(b.Text)
		}
		return sb.String()
	}

	return ""
}

func (m *mock) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var req struct {
		Model    string    `json:"model"`
		Messages []wireMsg `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)

	var allText strings.Builder
	for _, msg := range req.Messages {
		allText.WriteString(textOf(msg.Content))
		allText.WriteString("\n")
	}

	// Which summary markers ride this request (post-marker projection proof).
	fullText := allText.String()

	m.mu.Lock()
	m.seq++

	summaries := []int{}
	for i := 1; i <= m.summarizeN; i++ {
		if strings.Contains(fullText, fmt.Sprintf("%s #%d", summaryTag, i)) {
			summaries = append(summaries, i)
		}
	}

	entry := mockReq{Seq: m.seq, BodySize: len(body), Summaries: summaries}

	if strings.Contains(fullText, "Summarize the conversation below for a context-window compaction") {
		m.summarizeN++
		entry.Kind = "summarize"
		entry.TurnNum = m.turnN
		n := m.summarizeN
		m.log(entry)
		m.mu.Unlock()

		m.sse(w, fmt.Sprintf("%s #%d: task=phase19 live auto-UAT; state=compaction leg %d; decisions=[threshold fired]; open=[none]; files=[transcript markers].",
			summaryTag, n, n), len(body)/4, 40)

		return
	}

	m.turnN++
	entry.TurnNum = m.turnN

	if m.rejectArm {
		m.rejectArm = false
		m.rejectedBodySize = len(body)
		entry.Kind = "turn-overflow-rejected"
		m.log(entry)
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: ` +
			fmt.Sprintf("%d", len(body)/4) + ` tokens > 100000 maximum"}}`))

		return
	}

	entry.Kind = "turn"
	m.acceptedSizes = append(m.acceptedSizes, len(body))
	turn := m.turnN
	big := m.bigReplies
	m.log(entry)
	m.mu.Unlock()

	reply := fmt.Sprintf("%s %d. ", turnTag, turn)
	if big {
		filler := "context filler block with stable facts and file paths. "
		reps := bigReplyChars/len(filler) + 2
		reply += strings.Repeat(filler, reps)
		reply = reply[:bigReplyChars]
	}

	m.sse(w, reply, len(body)/4, len(reply)/4)
}

func (m *mock) sse(w http.ResponseWriter, text string, inTok, outTok int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	fl := w.(http.Flusher)

	write := func(event string, payload any) {
		raw, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
		fl.Flush()
	}

	write("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_autouat", "type": "message", "role": "assistant",
			"model": "GLM-5.3", "content": []any{},
			"usage": map[string]any{"input_tokens": inTok, "output_tokens": 1},
		},
	})
	write("content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})

	for off := 0; off < len(text); off += deltaChunk {
		end := min(off+deltaChunk, len(text))
		write("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text[off:end]},
		})
	}

	write("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	write("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": outTok},
	})
	write("message_stop", map[string]any{"type": "message_stop"})
}

func (m *mock) summarizeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summarizeN
}

func (m *mock) stats() (summarizes, turns, rejectedSize int, accepted []int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summarizeN, m.turnN, m.rejectedBodySize, append([]int(nil), m.acceptedSizes...)
}

func (m *mock) armReject() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rejectArm = true
}

func (m *mock) smallReplies() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bigReplies = false
}

// loadLog reads the mock's JSONL request log back for wire-order assertions.
func loadLog(path string) []mockReq {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var out []mockReq
	for _, ln := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var e mockReq
		if json.Unmarshal([]byte(ln), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func readLines(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(raw), "\n")
}

// --- ACP client ---

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *os.File

	wmu     sync.Mutex // serializes stdin writes (call frames vs probe replies)
	mu      sync.Mutex
	pending map[string]chan rpcMsg
	notes   map[string]int
}

func (c *client) writeFrame(frame []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.stdin.Write(append(frame, '\n'))
	return err
}

func (c *client) call(ctx context.Context, id int, method string, params any, timeout time.Duration) (rpcMsg, error) {
	raw, _ := json.Marshal(params)
	frame, _ := json.Marshal(rpcMsg{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprint(id)), Method: method, Params: raw})

	ch := make(chan rpcMsg, 1)
	c.mu.Lock()
	c.pending[string(frameID(id))] = ch
	c.mu.Unlock()

	if err := c.writeFrame(frame); err != nil {
		return rpcMsg{}, fmt.Errorf("write %s: %w", method, err)
	}

	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
		return rpcMsg{}, fmt.Errorf("%s: timeout after %s", method, timeout)
	case <-ctx.Done():
		return rpcMsg{}, ctx.Err()
	}
}

func frameID(id int) string { return fmt.Sprint(id) }

func (c *client) readLoop() {
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var msg rpcMsg
		if json.Unmarshal([]byte(line), &msg) != nil {
			continue
		}

		// Server→client REQUEST (probe etc.): politely unsupported.
		if msg.Method != "" && len(msg.ID) > 0 {
			resp, _ := json.Marshal(rpcMsg{
				JSONRPC: "2.0", ID: msg.ID,
				Error: &struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}{Code: -32601, Message: "method not found (autouat client)"},
			})
			_ = c.writeFrame(resp)
			continue
		}

		if msg.Method != "" { // notification
			kind := ""
			var p struct {
				Update *struct {
					SessionUpdate string `json:"sessionUpdate"`
				} `json:"update"`
			}
			if json.Unmarshal(msg.Params, &p) == nil && p.Update != nil {
				kind = p.Update.SessionUpdate
			}

			c.mu.Lock()
			c.notes[msg.Method+"/"+kind]++
			c.mu.Unlock()
			continue
		}

		if len(msg.ID) > 0 {
			c.mu.Lock()
			ch := c.pending[string(msg.ID)]
			delete(c.pending, string(msg.ID))
			c.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
		}
	}
}

// --- scenario ---

type check struct {
	name string
	ok   bool
	det  string
}

func main() {
	repo := flag.String("repo", "", "ass-guard-agent repo root (default: cwd's git toplevel guess)")
	keep := flag.String("keep", "/tmp/compaction-autouat", "scratch dir for project + logs")
	flag.Parse()

	root, err := filepath.Abs(*repo)
	must(err)
	if root == "" || *repo == "" {
		must(fmt.Errorf("pass -repo /path/to/ass-guard-agent"))
	}

	scratch := *keep
	must(os.MkdirAll(scratch, 0o755))
	proj := filepath.Join(scratch, "proj")
	must(os.RemoveAll(proj))
	must(os.MkdirAll(proj, 0o755))

	reportPath := filepath.Join(scratch, "REPORT.txt")
	requestLog := filepath.Join(scratch, "requests.jsonl")
	stderrLog := filepath.Join(scratch, "agent-stderr.log")

	// 1. Build the binary from HEAD (the UAT pinned "a binary that contains
	// the engine" — HEAD contains 19-04 engine + 19-06/19-07 CR-01 fixes).
	bin := filepath.Join(scratch, "ass-guard")
	run := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			must(fmt.Errorf("%s: %w\n%s", name, err, out))
		}
		return string(out)
	}

	run("go", "build", "-o", bin, "./cmd/ass-guard")

	// 2. Mock provider.
	m := &mock{bigReplies: true}
	baseURL, err := m.start(requestLog)
	must(err)

	// 3. Project config layer: provider → mock; credential dummy.
	must(os.MkdirAll(filepath.Join(proj, ".ass-guard"), 0o755))
	cfgYAML := "providers:\n  anthropic:\n    base_url: \"" + baseURL + "\"\n    api_key: autouat-dummy\n"
	must(os.WriteFile(filepath.Join(proj, ".ass-guard", "config.yaml"), []byte(cfgYAML), 0o600))

	// 4. Spawn the agent (stderr → file: compaction notes land there).
	stderrF, err := os.Create(stderrLog)
	must(err)

	cmd := exec.Command(bin, "acp", "serve",
		"--work-dir", proj,
		"--profiles-dir", filepath.Join(root, "profiles"))
	cmd.Dir = proj
	cmd.Env = append(os.Environ(), "ZAI_API_KEY=autouat-dummy")
	cmd.Stderr = stderrF

	stdinW, err := cmd.StdinPipe()
	must(err)
	stdoutR, err := cmd.StdoutPipe()
	must(err)
	must(cmd.Start())

	cl := &client{
		cmd: cmd, stdin: stdinW, stdout: bufio.NewReaderSize(stdoutR, 1<<20),
		stderr: stderrF, pending: map[string]chan rpcMsg{}, notes: map[string]int{},
	}
	go cl.readLoop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var checks []check
	pass := func(name string, ok bool, det string) {
		checks = append(checks, check{name, ok, det})
		mark := "✓"
		if !ok {
			mark = "✗"
		}
		fmt.Printf("  %s %s — %s\n", mark, name, det)
	}

	nextID := 0
	call := func(method string, params any) rpcMsg {
		nextID++
		msg, err := cl.call(ctx, nextID, method, params, 120*time.Second)
		must(err)
		return msg
	}
	var sessID string
	prompt := func(text string) string {
		msg := call("session/prompt", map[string]any{
			"sessionId": sessID,
			"prompt":    []map[string]string{{"type": "text", "text": text}},
		})
		var res struct {
			StopReason string `json:"stopReason"`
		}
		_ = json.Unmarshal(msg.Result, &res)
		return res.StopReason
	}
	markersIn := func() int {
		raw, err := os.ReadFile(filepath.Join(proj, ".ass-guard", "transcript_"+sessID+".jsonl"))
		if err != nil {
			return -1
		}
		return strings.Count(string(raw), `"type":"compaction"`)
	}

	fmt.Println("### GSD ► AUTO-UAT: Phase 19 compaction (G-19-2 live retest, no human leg)")

	// initialize (advertise elicitation form → no probe round-trip).
	init := call("initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"elicitation": map[string]any{"form": true},
			"session":     map[string]any{"configOptions": map[string]any{"boolean": true}},
		},
	})
	pass("initialize handshake", init.Error == nil && strings.Contains(string(init.Result), `"protocolVersion":1`), "v1 agreed")

	// session/new.
	sn := call("session/new", map[string]any{"cwd": proj})
	var snr struct {
		SessionID string `json:"sessionId"`
	}
	must(json.Unmarshal(sn.Result, &snr))
	sessID = snr.SessionID
	pass("session/new", snr.SessionID != "", sessID)

	// optionValue reads one option's current value from a configOptions array.
	optionValue := func(raw []byte, id string) string {
		var res struct {
			ConfigOptions []struct {
				ID           string `json:"id"`
				CurrentValue any    `json:"currentValue"`
				Value        any    `json:"value"`
			} `json:"configOptions"`
		}
		if json.Unmarshal(raw, &res) != nil {
			return ""
		}
		for _, o := range res.ConfigOptions {
			if o.ID == id {
				if s, ok := o.CurrentValue.(string); ok {
					return s
				}
				if s, ok := o.Value.(string); ok {
					return s
				}
				return fmt.Sprint(o.CurrentValue)
			}
		}
		return ""
	}

	// Live-set compaction-threshold 50 (the wire's lowest select value).
	sc := call("session/set_config_option", map[string]any{
		"sessionId": sessID, "configId": "compaction-threshold", "value": "50",
	})
	pass("set compaction-threshold=50 (live)", sc.Error == nil && optionValue(sc.Result, "compaction-threshold") == "50",
		"current value="+optionValue(sc.Result, "compaction-threshold"))

	// T1: seed a huge assistant turn.
	sr := prompt("AutoUAT turn one: please produce the big filler reply.")
	pass("turn 1 completes (end_turn)", sr == "end_turn", "stopReason="+sr)

	// T2: estimate crosses 50% → FIRST threshold compaction before request 2.
	sumsBefore := m.summarizeCount()
	sr = prompt("AutoUAT turn two: continue working; compaction should fire first.")
	pass("turn 2 completes (end_turn)", sr == "end_turn", "stopReason="+sr)
	pass("FIRST compaction fired (threshold 50)", m.summarizeCount() == sumsBefore+1 && markersIn() == 1,
		fmt.Sprintf("summarizer calls=%d markers=%d", m.summarizeCount(), markersIn()))

	// T3: usage anchor still over 50% → SECOND compaction (backstop not once-per-session).
	sr = prompt("AutoUAT turn three: the session must survive its SECOND compaction.")
	pass("turn 3 completes (end_turn)", sr == "end_turn", "stopReason="+sr)
	pass("SECOND compaction fired", m.summarizeCount() == sumsBefore+2 && markersIn() == 2,
		fmt.Sprintf("summarizer calls=%d markers=%d", m.summarizeCount(), markersIn()))

	// Restore 80 BEFORE the overflow leg so the forced path is what fires there.
	sc = call("session/set_config_option", map[string]any{
		"sessionId": sessID, "configId": "compaction-threshold", "value": "80",
	})
	yamlAfter, _ := os.ReadFile(filepath.Join(proj, ".ass-guard", "config.yaml"))
	pass("restore compaction-threshold=80", sc.Error == nil && optionValue(sc.Result, "compaction-threshold") == "80",
		"current value="+optionValue(sc.Result, "compaction-threshold"))
	pass("threshold 80 persisted to project layer", strings.Contains(string(yamlAfter), "threshold_pct: 80"), "config.yaml carries the key")

	// Overflow leg: reject the next turn request once (content-class: size).
	// Expected wire order for turn 4 (pinned design): the head's threshold
	// check fires FIRST (stale anchor + the 105K a3 estimate cross 80% — a
	// drift compaction, one per turn per WR-03b), THEN the request is
	// rejected, THEN the FORCED compact runs (its summary is what the retry
	// must carry — the CR-01 ruling), THEN the shrunken retry.
	m.armReject()
	sumsBefore = m.summarizeCount()
	marksBefore := markersIn()
	sr = prompt("AutoUAT turn four: the provider will reject this once for being too long.")
	_, _, rejectedSize, accepted := m.stats()
	retrySize := 0
	if len(accepted) > 0 {
		retrySize = accepted[len(accepted)-1]
	}
	pass("overflow retry-once completes turn (end_turn)", sr == "end_turn", "stopReason="+sr)
	pass("turn-4 compaction: one threshold (drift) + one forced",
		m.summarizeCount() == sumsBefore+2 && markersIn() == marksBefore+2,
		fmt.Sprintf("summarizer calls=%d→%d markers=%d→%d", sumsBefore, m.summarizeCount(), marksBefore, markersIn()))
	pass("retry window shrank (CR-01 post-marker projection)",
		retrySize < rejectedSize && retrySize < rejectedSize/2,
		fmt.Sprintf("rejected=%dB retry=%dB", rejectedSize, retrySize))

	// Quiescence at 80: no further compaction.
	m.smallReplies()
	sumsBefore = m.summarizeCount()
	marksBefore = markersIn()
	sr = prompt("AutoUAT turn five: quiet turn at the restored threshold.")
	pass("turn 5 completes (end_turn)", sr == "end_turn", "stopReason="+sr)
	pass("no compaction at threshold 80", m.summarizeCount() == sumsBefore && markersIn() == marksBefore,
		fmt.Sprintf("summarizer calls=%d markers=%d", m.summarizeCount(), markersIn()))

	// Wire-order pin: the exact request sequence the mock served. The pinned
	// 19-03 position rule shows here — a threshold marker lands AFTER its
	// turn's user line, so the summary seed rides from the FOLLOWING turn's
	// request (req2 carries none, req3 carries #1, req4 #2); the armed retry
	// carries the FORCED marker's fresh summary #4.
	logEntries := loadLog(requestLog)
	kinds := make([]string, len(logEntries))
	for i, e := range logEntries {
		kinds[i] = e.Kind
	}
	wantKinds := []string{
		"turn", "summarize", "turn", "summarize", "turn",
		"summarize", "turn-overflow-rejected", "summarize", "turn", "turn",
	}
	pass("wire order exact", strings.Join(kinds, ",") == strings.Join(wantKinds, ","),
		strings.Join(kinds, ","))
	if len(logEntries) == len(wantKinds) {
		pass("summary seeding follows the pinned position rule",
			len(logEntries[2].Summaries) == 0 && // req2: own-turn marker → lean seed
				len(logEntries[4].Summaries) == 1 && logEntries[4].Summaries[0] == 1 && // req3 ← marker1
				len(logEntries[6].Summaries) == 1 && logEntries[6].Summaries[0] == 2 && // req4 ← marker2
				len(logEntries[8].Summaries) == 1 && logEntries[8].Summaries[0] == 4, // retry ← FORCED marker4
			fmt.Sprintf("req3=%v req4=%v retry=%v",
				logEntries[4].Summaries, logEntries[6].Summaries, logEntries[8].Summaries))
	} else {
		pass("summary seeding follows the pinned position rule", false,
			fmt.Sprintf("unexpected request count %d", len(logEntries)))
	}

	// Disk evidence: markers carry non-empty summaries; stderr carries the notes.
	stderrRaw, _ := os.ReadFile(stderrLog)
	markerSummaries := 0
	for _, ln := range readLines(filepath.Join(proj, ".ass-guard", "transcript_"+sessID+".jsonl")) {
		var tl struct {
			Type    string `json:"type"`
			Summary string `json:"summary"`
		}
		if json.Unmarshal([]byte(ln), &tl) == nil && tl.Type == "compaction" && tl.Summary != "" {
			markerSummaries++
		}
	}
	pass("transcript markers carry durable summaries",
		markerSummaries == 4,
		fmt.Sprintf("%d marker lines with non-empty summary", markerSummaries))
	pass("stderr compaction notes (pre-authorized degrade)",
		strings.Count(string(stderrRaw), "compaction: starting") == 4 &&
			strings.Count(string(stderrRaw), "compaction: complete") == 4,
		fmt.Sprintf("starting=%d complete=%d",
			strings.Count(string(stderrRaw), "compaction: starting"),
			strings.Count(string(stderrRaw), "compaction: complete")))

	// Agent-side message chunks streamed (live turn observable).
	cl.mu.Lock()
	chunks := cl.notes["session/update/agent_message_chunk"]
	cl.mu.Unlock()
	pass("agent_message_chunk streamed to editor", chunks > 0, fmt.Sprintf("%d chunks", chunks))

	// Shutdown.
	_ = stdinW.Close()
	_ = cmd.Wait()
	_ = stderrF.Close()

	failed := 0
	for _, c := range checks {
		if !c.ok {
			failed++
		}
	}

	verdict := "PASS"
	if failed > 0 {
		verdict = "FAIL"
	}

	var rep strings.Builder
	fmt.Fprintf(&rep, "AutoUAT Phase 19 compaction — %s (%d/%d checks)\nrun: %s\nbinary: HEAD build of %s\nscratch: %s\n\n",
		verdict, len(checks)-failed, len(checks), time.Now().Format(time.RFC3339), root, scratch)
	for _, c := range checks {
		fmt.Fprintf(&rep, "[%s] %s — %s\n", map[bool]string{true: "PASS", false: "FAIL"}[c.ok], c.name, c.det)
	}
	must(os.WriteFile(reportPath, []byte(rep.String()), 0o644))

	fmt.Printf("\n### VERDICT: %s (%d/%d)\nreport: %s\n", verdict, len(checks)-failed, len(checks), reportPath)

	if failed > 0 {
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "autouat:", err)
		os.Exit(1)
	}
}
