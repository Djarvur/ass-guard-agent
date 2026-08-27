package acp //nolint:testpackage // internal package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
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

	for _, id := range f.ids() {
		bare := strings.TrimPrefix(id, optGlobalPrefix)
		f.current[id] = defaults[bare]
	}

	return f
}

func (f *fakeConfigSurface) ids() []string {
	return []string{
		optModel, optTier, optPermissionsMode, optCompactionThresh,
		optGlobalPrefix + optModel, optGlobalPrefix + optTier,
		optGlobalPrefix + optPermissionsMode, optGlobalPrefix + optCompactionThresh,
	}
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
			Type: "select", CurrentValue: f.current[id], Options: opts,
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

	var known ConfigOptionFrame

	found := false

	for _, o := range f.menu() {
		if o.ID == optionID {
			known, found = o, true

			break
		}
	}

	if !found {
		return nil, &ConfigViolationError{OptionID: optionID, Violation: "unknown option id"}
	}

	member := false

	for _, o := range known.Options {
		if o.Value == val {
			member = true

			break
		}
	}

	if !member {
		return nil, &ConfigViolationError{
			OptionID:  optionID,
			Violation: fmt.Sprintf("value %q is not one of the offered options", val),
		}
	}

	if f.failSet != nil {
		// D-07: the persist failed — the apply path is NEVER reached.
		return nil, &ConfigPersistError{OptionID: optionID, Err: f.failSet}
	}

	if strings.HasPrefix(optionID, optGlobalPrefix+optPermissionsMode) ||
		optionID == optPermissionsMode ||
		optionID == optCompactionThresh ||
		optionID == optGlobalPrefix+optCompactionThresh {
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

// jsonUnmarshalStrict decodes a response result (shared helper for the config
// tests — json.Unmarshal with error propagation).
func jsonUnmarshalStrict(raw []byte, v any) error {
	return json.Unmarshal(raw, v)
}

func TestConfigAdvertise(t *testing.T) {
	f := newFakeConfigSurface()
	h := newPipeHarness(t, WithConfigSurface(f))

	// initialize: the advertisement carries the full menu with the fake's
	// effective values, and loadSession stays false (truthful — Phase 18).
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

	if err := jsonUnmarshalStrict(initResp.Result, &ires); err != nil {
		t.Fatalf("unmarshal initialize result: %v (raw=%s)", err, string(initResp.Result))
	}

	if ls, ok := ires.AgentCapabilities["loadSession"].(bool); !ok || ls {
		t.Errorf("agentCapabilities.loadSession = %v; want present and false (truthful until Phase 18)", ires.AgentCapabilities["loadSession"])
	}

	assertEightEntries(t, ires.ConfigOptions, f, "initialize")

	// session/new: the SAME builder feeds the session advertisement.
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readFrame(t)

	if snew.Error != nil {
		t.Fatalf("session/new errored: %+v", snew.Error)
	}

	var sres struct {
		SessionID     string              `json:"sessionId"`     //nolint:tagliatelle // ACP wire field
		ConfigOptions []ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
	}

	if err := jsonUnmarshalStrict(snew.Result, &sres); err != nil {
		t.Fatalf("unmarshal session/new result: %v (raw=%s)", err, string(snew.Result))
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

		if o.Type != "select" {
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
			t.Errorf("%s: option %q currentValue = %q; want the surface's effective value %q", where, o.ID, o.CurrentValue, want)
		}
	}

	for _, id := range f.ids() {
		if !seen[id] {
			t.Errorf("%s: menu missing option %q", where, id)
		}
	}
}

// driveConfigSession handshakes a surface-equipped harness through
// initialize + session/new and returns it with the session id.
func driveConfigSession(t *testing.T, h *pipeHarness) string {
	t.Helper()

	handshake(t, h)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readFrame(t)

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

	h.send(t, newRequest(id, "session/set_config_option", params))

	for {
		msg := h.readFrame(t)
		if msg.ID != nil && string(msg.ID) == itoa(id) {
			return msg
		}

		t.Fatalf("expected the set_config_option response (id=%d); got method=%q id=%v", id, msg.Method, msg.ID)
	}
}

func TestSetConfigOption(t *testing.T) {
	t.Run("valid set: full set back, Set called with exact id/value, applied", func(t *testing.T) {
		f := newFakeConfigSurface()
		h := newPipeHarness(t, WithConfigSurface(f))
		sid := driveConfigSession(t, h)

		msg := sendSetConfigOption(t, h, 2, sid, optModel, testModelFallback)

		if msg.Error != nil {
			t.Fatalf("valid set errored: %+v", msg.Error)
		}

		var res struct {
			ConfigOptions []ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
		}

		if err := jsonUnmarshalStrict(msg.Result, &res); err != nil {
			t.Fatalf("unmarshal result: %v (raw=%s)", err, string(msg.Result))
		}

		assertEightEntries(t, res.ConfigOptions, f, "valid set")

		var updated *ConfigOptionFrame

		for i := range res.ConfigOptions {
			if res.ConfigOptions[i].ID == optModel {
				updated = &res.ConfigOptions[i]

				break
			}
		}

		if updated == nil || updated.CurrentValue != testModelFallback {
			t.Fatalf("model currentValue = %+v; want the NEW effective value %q in the FULL set", updated, testModelFallback)
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
	})

	t.Run("unknown id: typed -32602 with optionId + violation data", func(t *testing.T) {
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
	})

	t.Run("invalid value: typed -32602 with value violation", func(t *testing.T) {
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
	})

	t.Run("surface write failure: distinct typed error, apply never reached", func(t *testing.T) {
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
	})

	t.Run("pending id valid value: one log line, full set, currentValue unchanged", func(t *testing.T) {
		f := newFakeConfigSurface()
		h := newPipeHarness(t, WithConfigSurface(f))
		sid := driveConfigSession(t, h)

		msg := sendSetConfigOption(t, h, 2, sid, optPermissionsMode, testPermGated)

		if msg.Error != nil {
			t.Fatalf("pending set errored: %+v (D-05: pending ids are accepted no-ops, never errors)", msg.Error)
		}

		var res struct {
			ConfigOptions []ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
		}

		if err := jsonUnmarshalStrict(msg.Result, &res); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}

		assertEightEntries(t, res.ConfigOptions, f, "pending no-op")

		for _, o := range res.ConfigOptions {
			if o.ID == optPermissionsMode && o.CurrentValue != testPermUngated {
				t.Errorf("permissions.mode currentValue = %q; want UNCHANGED %q (pending no-op)", o.CurrentValue, testPermUngated)
			}
		}

		f.mu.Lock()
		defer f.mu.Unlock()

		if got := strings.Count(f.logBuf.String(), "\n"); got != 1 {
			t.Errorf("pending log lines = %d; want exactly 1 (logs=%q)", got, f.logBuf.String())
		}

		if len(f.applied) != 0 {
			t.Errorf("apply recordings = %v; want none (pending ids never apply)", f.applied)
		}
	})

	t.Run("pending id invalid value: typed reject (validation still applies)", func(t *testing.T) {
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
	})
}

// TestSetConfigOptionNoSurface pins the degrade path: without a wired surface
// the advertisements omit configOptions entirely and set_config_option answers
// the typed not-available error — the acpserve-less setups keep working.
func TestSetConfigOptionNoSurface(t *testing.T) {
	h := newPipeHarness(t)
	handshake(t, h)

	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readFrame(t)

	if !strings.Contains(string(snew.Result), "sessionId") {
		t.Fatalf("session/new malformed: %s", string(snew.Result))
	}

	if strings.Contains(string(snew.Result), "configOptions") {
		t.Errorf("no-surface session/new advertises configOptions: %s", string(snew.Result))
	}

	sid := ""

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	_ = jsonUnmarshalStrict(snew.Result, &sres)

	sid = sres.SessionID

	msg := sendSetConfigOption(t, h, 2, sid, optModel, testModelFallback)

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

var errFakeDiskFull = errors.New("fake: disk full")
