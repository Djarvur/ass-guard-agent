package acp //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// The 16-05 config-options wire surface tests (ACP-08): advertisement with
// effective currentValues (D-11), the validated persist-then-apply
// set_config_option handler (D-07/D-09), and honest pending-handler no-ops
// (D-05). The fake ConfigSurface carries the PRODUCTION surface contract
// (validate → persist → apply, pending/idempotent semantics) so the handler
// tests pin the same behavior the real acpserve surface implements.

// Config option ids of the locked v1.2 menu (D-06 enumeration — the four
// options and their _global/ twins; A7 scope namespace).
const (
	optModel              = "model"
	optTier               = "tier"
	optPermissionsMode    = "permissions.mode"
	optCompactionThresh   = "compaction-threshold"
	optGlobalPrefix       = "_global/"
	testModelPrimary      = "GLM-5.3"
	testModelFallback     = "glm-5.2"
	testTierHeavy         = "heavy"
	testPermUngated       = "ungated"
	testPermGated         = "gated"
	testCompactionDefault = "80"
)

var errFakeDiskFull = errors.New("fake: disk full")

// fakeConfigSurface is the recording ConfigSurface fake: known eight-entry
// menu, recording Set calls, injectable persist failure, and an apply-path
// recorder so the persist-before-apply ordering (D-07) is observable.
type fakeConfigSurface struct {
	mu        sync.Mutex
	current   map[string]string // option id → effective currentValue
	failSet   error             // when set, Set fails typed-persist AFTER recording the call
	setCalls  []fakeSetCall
	applied   []string // the surface's apply path — reached ONLY after persist succeeds
	logBuf    strings.Builder
	blobCalls int
}

type fakeSetCall struct {
	sessionID string
	optionID  string
	value     any
}

func newFakeConfigSurface() *fakeConfigSurface {
	defaults := map[string]string{
		optModel:            testModelPrimary,
		optTier:             testTierHeavy,
		optPermissionsMode:  testPermUngated,
		optCompactionThresh: testCompactionDefault,
	}

	f := &fakeConfigSurface{current: map[string]string{}}

	for _, id := range fakeOptionIDs() {
		bare := strings.TrimPrefix(id, optGlobalPrefix)
		f.current[id] = defaults[bare]
	}

	return f
}

// fakeOptionIDs is the fake's menu membership (the four options + twins).
func fakeOptionIDs() []string {
	return []string{
		optModel, optTier, optPermissionsMode, optCompactionThresh,
		optGlobalPrefix + optModel, optGlobalPrefix + optTier,
		optGlobalPrefix + optPermissionsMode, optGlobalPrefix + optCompactionThresh,
	}
}

func (f *fakeConfigSurface) Options() []ConfigOptionFrame {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.menu()
}

// Set mirrors the production surface contract: record the call (the handler
// RELAYS — it always reached the surface), validate id then value (D-09 typed
// reject), fail typed-persist when injected (apply NEVER reached — D-07
// ordering), pending ids log one line and return the set UNCHANGED, real ids
// update the effective value and record the apply.
func (f *fakeConfigSurface) Set(sessionID, optionID string, value any) ([]ConfigOptionFrame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setCalls = append(f.setCalls, fakeSetCall{sessionID: sessionID, optionID: optionID, value: value})

	val, ok := value.(string)
	if !ok {
		return nil, &ConfigViolationError{OptionID: optionID, Violation: "value must be a string option id"}
	}

	known, found := f.optionByID(optionID)
	if !found {
		return nil, &ConfigViolationError{OptionID: optionID, Violation: "unknown option id"}
	}

	if !f.isMember(known.Options, val) {
		return nil, &ConfigViolationError{
			OptionID:  optionID,
			Violation: fmt.Sprintf("value %q is not one of the offered options", val),
		}
	}

	if f.failSet != nil {
		// D-07: the persist failed — the apply path is NEVER reached.
		return nil, &ConfigPersistError{OptionID: optionID, Err: f.failSet}
	}

	if f.isPending(optionID) {
		// D-05 pending-handler no-op: one structured line, currentValue unchanged.
		fmt.Fprintf(&f.logBuf, "config option %q: pending handler (value %q logged, not applied)\n", optionID, val)

		return f.menu(), nil
	}

	f.current[optionID] = val
	f.applied = append(f.applied, optionID+"="+val)

	return f.menu(), nil
}

func (f *fakeConfigSurface) ApplyBlobDefaults(_ map[string]json.RawMessage) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.blobCalls++

	return false, nil
}

// menu builds the fake's eight entries with the CURRENT values (D-11 shape:
// every currentValue is what effective resolution would return right now).
func (f *fakeConfigSurface) menu() []ConfigOptionFrame {
	build := func(id, name, category string, values []string) ConfigOptionFrame {
		opts := make([]ConfigOptionValue, 0, len(values))
		for _, v := range values {
			opts = append(opts, ConfigOptionValue{Value: v, Name: v})
		}

		return ConfigOptionFrame{
			ID: id, Name: name, Category: category,
			Type: ConfigOptionTypeSelect, CurrentValue: f.current[id], Options: opts,
		}
	}

	return []ConfigOptionFrame{
		build(optModel, "Model", "model", []string{testModelPrimary, testModelFallback}),
		build(optTier, "Session tier", "model_config", []string{testTierHeavy}),
		build(optPermissionsMode, "Permission mode", "mode", []string{testPermUngated, testPermGated}),
		build(optCompactionThresh, "Compaction threshold", "_custom",
			[]string{"off", "50", "65", testCompactionDefault, "95"}),
		build(optGlobalPrefix+optModel, "Model (global)", "model", []string{testModelPrimary, testModelFallback}),
		build(optGlobalPrefix+optTier, "Session tier (global)", "model_config", []string{testTierHeavy}),
		build(optGlobalPrefix+optPermissionsMode, "Permission mode (global)", "mode",
			[]string{testPermUngated, testPermGated}),
		build(optGlobalPrefix+optCompactionThresh, "Compaction threshold (global)", "_custom",
			[]string{"off", "50", "65", testCompactionDefault, "95"}),
	}
}

func (f *fakeConfigSurface) optionByID(id string) (ConfigOptionFrame, bool) {
	for _, o := range f.menu() {
		if o.ID == id {
			return o, true
		}
	}

	return ConfigOptionFrame{}, false
}

func (f *fakeConfigSurface) isMember(vals []ConfigOptionValue, val string) bool {
	for _, v := range vals {
		if v.Value == val {
			return true
		}
	}

	return false
}

func (f *fakeConfigSurface) isPending(optionID string) bool {
	bare := strings.TrimPrefix(optionID, optGlobalPrefix)

	return bare == optPermissionsMode || bare == optCompactionThresh
}

// jsonUnmarshalStrict decodes a response result (shared helper for the config
// tests — json.Unmarshal with error propagation).
func jsonUnmarshalStrict(raw []byte, v any) error {
	return json.Unmarshal(raw, v) //nolint:wrapcheck // test helper passthrough
}

func TestConfigAdvertise(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))

	// initialize: the advertisement carries the full menu with the fake's
	// effective values, and loadSession is true (18-01 — the replay spine is
	// live).
	h.send(t, newRequest(0, methodInitialize, zedLikeInitializeParams()))
	initResp := h.readFrame(t)

	if initResp.Error != nil {
		t.Fatalf("initialize errored: %+v", initResp.Error)
	}

	var ires struct {
		ProtocolVersion   int                 `json:"protocolVersion"`   //nolint:tagliatelle // ACP wire field
		AgentCapabilities map[string]any      `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
		ConfigOptions     []ConfigOptionFrame `json:"configOptions"`     //nolint:tagliatelle // ACP wire field
	}

	uerr := jsonUnmarshalStrict(initResp.Result, &ires)
	if uerr != nil {
		t.Fatalf("unmarshal initialize result: %v (raw=%s)", uerr, string(initResp.Result))
	}

	ls, ok := ires.AgentCapabilities["loadSession"].(bool)
	if !ok || !ls {
		t.Errorf("agentCapabilities.loadSession = %v; want present and true (18-01/ACP-06)",
			ires.AgentCapabilities["loadSession"])
	}

	assertEightEntries(t, ires.ConfigOptions, f, "initialize")

	// session/new: the SAME builder feeds the session advertisement.
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readResultFrame(t)

	if snew.Error != nil {
		t.Fatalf("session/new errored: %+v", snew.Error)
	}

	var sres struct {
		SessionID     string              `json:"sessionId"`     //nolint:tagliatelle // ACP wire field
		ConfigOptions []ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
	}

	uerr = jsonUnmarshalStrict(snew.Result, &sres)
	if uerr != nil {
		t.Fatalf("unmarshal session/new result: %v (raw=%s)", uerr, string(snew.Result))
	}

	if sres.SessionID == "" {
		t.Fatal("session/new returned an empty sessionId")
	}

	assertEightEntries(t, sres.ConfigOptions, f, "session/new")
}

// assertEightEntries verifies the D-06 enumeration: exactly eight entries in
// the v1 select shape, each currentValue equal to the surface's effective
// value for that id at response time (D-11).
func assertEightEntries(t *testing.T, got []ConfigOptionFrame, f *fakeConfigSurface, where string) {
	t.Helper()

	if len(got) != 8 {
		t.Fatalf("%s: configOptions length = %d; want 8 (the four options + _global/ twins)", where, len(got))
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	seen := map[string]bool{}

	for _, o := range got {
		seen[o.ID] = true

		if o.ID == "" || o.Name == "" {
			t.Errorf("%s: entry %q missing id/name (v1 required fields)", where, o.ID)

			continue
		}

		if o.Type != ConfigOptionTypeSelect {
			t.Errorf("%s: option %q type = %q; want select", where, o.ID, o.Type)
		}

		if o.CurrentValue == "" {
			t.Errorf("%s: option %q carries an empty currentValue (D-11 requires the EFFECTIVE value)", where, o.ID)
		}

		if len(o.Options) == 0 {
			t.Errorf("%s: option %q carries no options list", where, o.ID)
		}

		if o.Category == "" {
			t.Errorf("%s: option %q missing category", where, o.ID)
		}

		if want := f.current[o.ID]; want != "" && o.CurrentValue != want {
			t.Errorf("%s: option %q currentValue = %q; want the surface's effective value %q",
				where, o.ID, o.CurrentValue, want)
		}
	}

	for _, id := range fakeOptionIDs() {
		if !seen[id] {
			t.Errorf("%s: menu missing option %q", where, id)
		}
	}
}

// driveConfigSession handshakes a surface-equipped harness through
// initialize + session/new and returns the session id.
func driveConfigSession(t *testing.T, h *pipeHarness) string {
	t.Helper()

	handshake(t, h)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readResultFrame(t)

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	_ = jsonUnmarshalStrict(snew.Result, &sres)

	if sres.SessionID == "" {
		t.Fatal("session/new returned an empty sessionId")
	}

	return sres.SessionID
}

// sendSetConfigOption sends one session/set_config_option request and returns
// its response frame.
func sendSetConfigOption(t *testing.T, h *pipeHarness, id int, sessionID, optionID string, value any) *Message {
	t.Helper()

	params := map[string]any{keySessionID: sessionID, "configId": optionID}
	if value != nil {
		params["value"] = value
	}

	h.send(t, newRequest(id, methodSetConfigOption, params))

	for {
		msg := h.readFrame(t)
		if msg.ID != nil && string(msg.ID) == itoa(id) {
			return msg
		}

		t.Fatalf("expected the set_config_option response (id=%d); got method=%q id=%v", id, msg.Method, msg.ID)
	}
}

// decodeSetResult unmarshals a successful set_config_option result.
func decodeSetResult(t *testing.T, msg *Message) []ConfigOptionFrame {
	t.Helper()

	var res struct {
		ConfigOptions []ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
	}

	uerr := jsonUnmarshalStrict(msg.Result, &res)
	if uerr != nil {
		t.Fatalf("unmarshal result: %v (raw=%s)", uerr, string(msg.Result))
	}

	return res.ConfigOptions
}

// findOption returns the advertised entry for the id (nil-safe helper).
func findOption(opts []ConfigOptionFrame, id string) *ConfigOptionFrame {
	for i := range opts {
		if opts[i].ID == id {
			return &opts[i]
		}
	}

	return nil
}

func TestSetConfigOptionValid(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))
	sid := driveConfigSession(t, h)

	msg := sendSetConfigOption(t, h, 2, sid, optModel, testModelFallback)

	if msg.Error != nil {
		t.Fatalf("valid set errored: %+v", msg.Error)
	}

	opts := decodeSetResult(t, msg)
	assertEightEntries(t, opts, f, "valid set")

	updated := findOption(opts, optModel)
	if updated == nil || updated.CurrentValue != testModelFallback {
		t.Fatalf("model currentValue = %+v; want the NEW effective value %q in the FULL set",
			updated, testModelFallback)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.setCalls) != 1 || f.setCalls[0].optionID != optModel ||
		f.setCalls[0].value != testModelFallback || f.setCalls[0].sessionID != sid {
		t.Errorf("Set calls = %+v; want exactly one with (sessionId=%q, id=%q, value=%q)",
			f.setCalls, sid, optModel, testModelFallback)
	}

	if len(f.applied) != 1 {
		t.Errorf("apply path recordings = %v; want exactly one (persist-then-apply)", f.applied)
	}
}

func TestSetConfigOptionUnknownID(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))
	sid := driveConfigSession(t, h)

	msg := sendSetConfigOption(t, h, 2, sid, "api_key", "sk-nope")

	if msg.Error == nil {
		t.Fatal("unknown option id returned a result; want a typed reject (D-09)")
	}

	if msg.Error.Code != CodeInvalidParams {
		t.Errorf("error code = %d; want %d (CodeInvalidParams)", msg.Error.Code, CodeInvalidParams)
	}

	assertViolationData(t, msg.Error.Data, "api_key")
}

func TestSetConfigOptionInvalidValue(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))
	sid := driveConfigSession(t, h)

	msg := sendSetConfigOption(t, h, 2, sid, optModel, "not-a-model")

	if msg.Error == nil {
		t.Fatal("invalid value returned a result; want a typed reject (D-09)")
	}

	if msg.Error.Code != CodeInvalidParams {
		t.Errorf("error code = %d; want %d (CodeInvalidParams)", msg.Error.Code, CodeInvalidParams)
	}

	assertViolationData(t, msg.Error.Data, optModel)

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.applied) != 0 {
		t.Errorf("apply recordings = %v; want none (rejected values never apply)", f.applied)
	}
}

func TestSetConfigOptionWriteFailure(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	f.failSet = errFakeDiskFull
	h := newPipeHarness(t, WithConfigSurface(f))
	sid := driveConfigSession(t, h)

	msg := sendSetConfigOption(t, h, 2, sid, optTier, testTierHeavy)

	if msg.Error == nil {
		t.Fatal("persist failure returned a result; want a typed error (D-07)")
	}

	if msg.Error.Code == CodeInvalidParams {
		t.Errorf("persist failure surfaced as a validation error (%d); want a DISTINCT class", msg.Error.Code)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.setCalls) != 1 {
		t.Errorf("Set calls = %d; want 1 (the handler reached the surface)", len(f.setCalls))
	}

	if len(f.applied) != 0 {
		t.Errorf("apply recordings = %v; want NONE (persist failed before apply — D-07 ordering)", f.applied)
	}
}

func TestSetConfigOptionPendingNoOp(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))
	sid := driveConfigSession(t, h)

	msg := sendSetConfigOption(t, h, 2, sid, optPermissionsMode, testPermGated)

	if msg.Error != nil {
		t.Fatalf("pending set errored: %+v (D-05: pending ids are accepted no-ops, never errors)", msg.Error)
	}

	opts := decodeSetResult(t, msg)
	assertEightEntries(t, opts, f, "pending no-op")

	perm := findOption(opts, optPermissionsMode)
	if perm != nil && perm.CurrentValue != testPermUngated {
		t.Errorf("permissions.mode currentValue = %q; want UNCHANGED %q (pending no-op)",
			perm.CurrentValue, testPermUngated)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if got := strings.Count(f.logBuf.String(), "\n"); got != 1 {
		t.Errorf("pending log lines = %d; want exactly 1 (logs=%q)", got, f.logBuf.String())
	}

	if len(f.applied) != 0 {
		t.Errorf("apply recordings = %v; want none (pending ids never apply)", f.applied)
	}
}

func TestSetConfigOptionPendingInvalid(t *testing.T) {
	t.Parallel()

	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))
	sid := driveConfigSession(t, h)

	msg := sendSetConfigOption(t, h, 2, sid, optPermissionsMode, "sideways")

	if msg.Error == nil {
		t.Fatal("pending id with invalid value returned a result; want a typed reject")
	}

	if msg.Error.Code != CodeInvalidParams {
		t.Errorf("error code = %d; want %d (CodeInvalidParams)", msg.Error.Code, CodeInvalidParams)
	}

	assertViolationData(t, msg.Error.Data, optPermissionsMode)
}

// TestSetConfigOptionNoSurface pins the degrade path: without a wired surface
// the advertisements omit configOptions entirely and set_config_option answers
// the typed not-available error — the acpserve-less setups keep working.
func TestSetConfigOptionNoSurface(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)
	handshake(t, h)

	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readResultFrame(t)

	if !strings.Contains(string(snew.Result), "sessionId") {
		t.Fatalf("session/new malformed: %s", string(snew.Result))
	}

	if strings.Contains(string(snew.Result), "configOptions") {
		t.Errorf("no-surface session/new advertises configOptions: %s", string(snew.Result))
	}

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	_ = jsonUnmarshalStrict(snew.Result, &sres)

	msg := sendSetConfigOption(t, h, 2, sres.SessionID, optModel, testModelFallback)

	if msg.Error == nil {
		t.Fatal("set_config_option without a surface returned a result; want the typed not-available error")
	}

	if msg.Error.Code == CodeInvalidParams {
		t.Errorf("not-available surfaced as a validation error; want a distinct degrade code")
	}

	if !strings.Contains(strings.ToLower(msg.Error.Message), "not available") {
		t.Errorf("not-available message = %q; want an explicit not-available signal", msg.Error.Message)
	}
}

// assertViolationData checks the D-09 error data shape: option key + violation
// detail (the client renders the rejection from these).
func assertViolationData(t *testing.T, data any, wantOptionID string) {
	t.Helper()

	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("error data = %#v; want an object carrying optionId + violation", data)
	}

	if m["optionId"] != wantOptionID {
		t.Errorf("error data optionId = %v; want %q", m["optionId"], wantOptionID)
	}

	v, _ := m["violation"].(string)
	if v == "" {
		t.Error("error data violation is empty; want a violation detail string")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 17-03 (D-13): the teardown-drain wiring. The ONE drain function (the
// session's ask-queue drain) is reachable from all three teardown paths, each
// pinned here and at the queue/session level: the session/cancel notification
// (before/with cancelTurn), the session-close path (logout, before the reap),
// and serve shutdown (the acpserve composition — behavior-tested beside the
// real registry in the acpserve drain battery).

// drainRecordingRunner is a TurnRunner implementing the AskDrainer capability
// and SessionCloser, recording the teardown event order so the tests can pin
// drain-before-cancel and drain-before-close.
type drainRecordingRunner struct {
	mu     sync.Mutex
	events []string
}

func (r *drainRecordingRunner) Run(
	ctx context.Context, _ string, emit ChunkEmitter, _ []ContentBlock,
) (string, error) {
	r.record("run")

	_ = emit.AgentMessageChunk("m1", "x")

	<-ctx.Done() // block until session/cancel aborts the turn

	r.record("run-cancelled")

	return stopCancelled, nil
}

// DrainAsks satisfies acp.AskDrainer (the session/cancel teardown seam).
func (r *drainRecordingRunner) DrainAsks(sessionID string) {
	r.record("drain:" + sessionID)
}

// DrainSessionAsks satisfies acp.AskDrainer (the session-close teardown seam).
func (r *drainRecordingRunner) DrainSessionAsks(sessionID string) {
	r.record("drain-session:" + sessionID)
}

// DrainAllAsks satisfies acp.AskDrainer (the serve-shutdown teardown seam).
func (r *drainRecordingRunner) DrainAllAsks() {
	r.record("drain-all")
}

// CloseSession satisfies acp.SessionCloser (logout's reap).
func (r *drainRecordingRunner) CloseSession(sessionID string) error {
	r.record("closed:" + sessionID)

	return nil
}

func (r *drainRecordingRunner) record(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, e)
}

func (r *drainRecordingRunner) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.events...)
}

// indexOf returns the first index of want in items, or -1.
func indexOf(items []string, want string) int {
	for i, v := range items {
		if v == want {
			return i
		}
	}

	return -1
}

// TestSessionCancelDrainOnCancel pins the cancel-path wiring: the
// session/cancel notification drains the session's ask queue BEFORE/with the
// turn cancel — the open dialog resolves and queued asks never survive the
// abort (D-13; the ACP cancel contract covers ALL pending permission
// requests).
func TestSessionCancelDrainOnCancel(t *testing.T) {
	t.Parallel()

	runner := &drainRecordingRunner{}
	h := newPipeHarness(t, WithTurnRunner(runner))
	sid := handshakeRecordingSession(t, h)

	done := make(chan struct{})

	go func() {
		defer close(done)

		h.send(t, newRequest(2, "session/prompt", promptRequest(sid, "hi")))
	}()

	gateWaitForEvent(t, func() bool {
		events := runner.recorded()

		return len(events) > 0 && events[0] == "run"
	})

	// The editor flow: escape mid-turn (a notification — dispatched inline).
	h.send(t, newNotification("session/cancel", map[string]any{keySessionID: sid}))

	<-done

	awaitResponseID(t, h, "2")

	events := runner.recorded()

	drainIdx := indexOf(events, "drain:"+sid)
	cancelIdx := indexOf(events, "run-cancelled")

	if drainIdx == -1 {
		t.Fatalf("session/cancel never drained the ask queue; events = %v", events)
	}

	if cancelIdx == -1 || drainIdx > cancelIdx {
		t.Errorf("drain (idx %d) must land before/with the turn cancel (idx %d); events = %v",
			drainIdx, cancelIdx, events)
	}
}

// TestSessionCancelDrainOnLogout pins the session-close wiring: logout drains
// the session's asks BEFORE the resource reap (CloseSession) — no dialog and
// no queued ask outlives the session (T-17-09).
func TestSessionCancelDrainOnLogout(t *testing.T) {
	t.Parallel()

	runner := &drainRecordingRunner{}
	h := newPipeHarness(t, WithTurnRunner(runner))
	sid := handshakeRecordingSession(t, h)

	h.send(t, newRequest(2, "logout", map[string]any{keySessionID: sid}))
	awaitResponseID(t, h, "2")

	events := runner.recorded()

	drainIdx := indexOf(events, "drain-session:"+sid)
	closeIdx := indexOf(events, "closed:"+sid)

	if drainIdx == -1 {
		t.Fatalf("logout never drained the session's asks; events = %v", events)
	}

	if closeIdx == -1 || drainIdx > closeIdx {
		t.Errorf("drain (idx %d) must land before the session close (idx %d); events = %v",
			drainIdx, closeIdx, events)
	}
}

// gateWaitForEvent polls cond until true or the deadline expires (the
// cancel/close notifications dispatch inline while the prompt runs on its own
// goroutine — the same async shape the session tests use).
func gateWaitForEvent(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	t.Fatal("condition never became true within 2s")
}
