// The Claude-Code hook runner (12-02 Task 4) lives here: plugin-bundled
// hooks/hooks.json entries fire at the mapped lifecycle seams with the
// documented stdin-JSON / stdout / exit-code semantics. Hooks are
// operator-installed config executing operator-chosen commands — the SAME
// trust tier as the hook-DAG (the standing safety model); the exit-2
// PreToolUse refusal is a policy channel, NOT a new confirmation tier. Every
// firing is bounded and never fatal (AUD-03: a hook failure is an audit-loud
// stderr skip, logged through the swappable seam). See the HookRunner doc and
// doc.go (the package godoc) for the seam map.

package ecosys

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// HookConfig is one parsed hook command from a plugin bundle's
// hooks/hooks.json (12-02) or a settings.json scope (21-01 PAR-03): the
// lifecycle Event it fires at, the Matcher (a tool-name regex for the tool
// events; empty/"*" matches all — CC's two-path exact/alternatives dialect
// applies to the settings scopes), the shell Command (run via `sh -c`), its
// per-hook timeout, provenance (the source file's Path + the bundle's
// PluginRoot, exported to the hook's env as CLAUDE_PLUGIN_ROOT — the
// documented expansion vehicle), and the discovery Scope (21-01).
type HookConfig struct {
	Event      string
	Matcher    string
	Command    string
	TimeoutSec int
	Path       string
	PluginRoot string

	// Scope tags where the hook was discovered (21-01 PAR-03/D-03). The zero
	// value is ScopePlugin so every pre-21-01 construction is unchanged.
	Scope HookScope
}

// HookScope is the discovery scope of a hook (21-01 PAR-03): the two
// settings.json scopes (project = repo-shipped, user = operator-owned) and
// the plugin bundles. The D-03 firing order (project → user → plugin) is
// resolved by scopeRank at runner construction — the enum's numeric order is
// an implementation detail, NOT the firing order.
type HookScope int

// The scope constants are exported at the 21-06 gate join: D-01's
// scope-split authority (only USER scope may allow) is a property of the
// verdict the gate consumes, so joined-level tests and consumers construct
// scoped hooks by name.
const (
	ScopePlugin  HookScope = iota // plugin-bundled hooks/hooks.json (zero value)
	ScopeProject                  // project .claude/settings.json (repo-shipped)
	ScopeUser                     // user ~/.claude/settings.json (operator-owned)
)

// HookOutcome is one Fire result: Proceed=false ONLY for an exit-2 PreToolUse
// refusal (the operator-configured policy channel — the hook-table safety
// model, NOT a new confirmation tier); Message carries the refusal text or
// the captured stdout (the context-bearing events' injection payload).
type HookOutcome struct {
	Proceed bool
	Message string
}

// Hook event names and stdin-payload keys (goconst; the documented wire
// vocabulary — never renamed).
const (
	hookEventPreToolUse       = "PreToolUse"
	hookEventPostToolUse      = "PostToolUse"
	hookEventUserPromptSubmit = "UserPromptSubmit"
	hookEventStop             = "Stop"
	hookEventSubagentStop     = "SubagentStop"
	hookEventSessionStart     = "SessionStart"
	hookEventSessionEnd       = "SessionEnd"

	keyToolName     = "tool_name"
	keyToolInput    = "tool_input"
	keyToolResponse = "tool_response"
)

// hookEventsMapped is the set of Claude-Code hook events ass-guard has a seam
// for (12-02). Every other event name (Notification, PreCompact, future
// additions) is UNMAPPED: it parses, warns, and degrades to observe-only —
// documented, never silently ignored.
var hookEventsMapped = map[string]bool{ //nolint:gochecknoglobals // immutable table
	hookEventPreToolUse:       true,
	hookEventPostToolUse:      true,
	hookEventUserPromptSubmit: true,
	hookEventStop:             true,
	hookEventSubagentStop:     true,
	hookEventSessionStart:     true,
	hookEventSessionEnd:       true,
}

// hookToolEvents are the events whose matcher is a tool-name regex.
var hookToolEvents = map[string]bool{ //nolint:gochecknoglobals // immutable table
	hookEventPreToolUse:  true,
	hookEventPostToolUse: true,
}

// Runner bounds (T-12-02-03/T-12-02-06): the per-hook default timeout
// (Claude Code's documented 60s) and the captured-output cap (the documented
// 30000-char discipline, applied to stdout AND stderr).
const (
	hookDefaultTimeoutSec = 60
	hookOutputCap         = 30000

	// hookRefusalExitCode is the documented blocking-refusal exit code.
	hookRefusalExitCode = 2
)

// HookRunner executes Registry.Hooks at the mapped lifecycle seams:
//
//   - UserPromptSubmit       — turn entry (post-expansion; session.Prompt)
//   - PreToolUse             — the session gate head's verdict consult (21-06:
//     PreToolUseVerdict, consumed ONLY by
//     internal/session's gateCall)
//   - PostToolUse            — the executor-side observation seam (coreexec)
//   - Stop/SubagentStop      — parent/subagent turn end
//   - SessionStart/SessionEnd — session create/close
//
// Unmapped events warn + observe-only. Nil-runner methods are safe no-ops.
type HookRunner struct {
	Hooks          []HookConfig
	SessionID      string
	WorkDir        string
	TranscriptPath string
}

// NewHookRunner builds a runner over the discovered hooks for one session,
// partitioning them ONCE into the deterministic D-03 firing order
// (project settings → user settings → plugin bundles; stable within scope).
// The scope order resolves HERE, at construction — never in the loader's
// merge (Pitfall 1: other consumers depend on overlay precedence) — so the
// runtime call site keeps its signature and all-plugin inputs are unchanged.
func NewHookRunner(hooks []HookConfig, sessionID, workDir, transcriptPath string) *HookRunner {
	ordered := append([]HookConfig(nil), hooks...) // never mutate the caller's slice

	sort.SliceStable(ordered, func(i, j int) bool {
		return scopeRank(ordered[i].Scope) < scopeRank(ordered[j].Scope)
	})

	return &HookRunner{
		Hooks: ordered, SessionID: sessionID, WorkDir: workDir, TranscriptPath: transcriptPath,
	}
}

// scopeRankPlugin is the lowest D-03 firing rank (the plugin tier; future
// scopes sort with it — fail-soft).
const scopeRankPlugin = 2

// scopeRank is the D-03 firing rank: project settings first, then user
// settings, then plugin bundles. Ordering ONLY — authority is deny-wins in
// ResolveVerdict, never this rank.
func scopeRank(s HookScope) int {
	switch s {
	case ScopeProject:
		return 0
	case ScopeUser:
		return 1
	case ScopePlugin:
		return scopeRankPlugin
	default:
		return scopeRankPlugin
	}
}

// parseHooksJSON reads one hooks/hooks.json (the live-measured shape: a
// top-level object with a `hooks` map keyed by event name → matcher groups →
// command entries) into HookConfigs tagged ScopePlugin.
func parseHooksJSON(path, pluginRoot string) []HookConfig {
	return parseHooksFile(path, pluginRoot, ScopePlugin)
}

// parseHooksFile is the shared hooks-file reader for plugin bundles
// (12-02) and settings.json scopes (21-01): a top-level object with a
// `hooks` map keyed by event name → matcher groups → command entries.
// Deterministic event order (sorted keys). A malformed file or non-command
// hook entry skips with a warning; an unmapped event parses + warns
// (observe-only). The scope tag is set at parse time (D-03).
func parseHooksFile(path, pluginRoot string, scope HookScope) []HookConfig {
	data, ok := readCapped(path, pluginArtifactMaxBytes)
	if !ok {
		return nil
	}

	var raw struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}

	if json.Unmarshal(data, &raw) != nil || len(raw.Hooks) == 0 {
		logPluginSkipf("malformed or empty hooks file %s (skipped)", path)

		return nil
	}

	events := make([]string, 0, len(raw.Hooks))
	for ev := range raw.Hooks {
		events = append(events, ev)
	}

	sort.Strings(events)

	var out []HookConfig

	for _, ev := range events {
		if !hookEventsMapped[ev] {
			logPluginSkipf("hooks %s: event %s has no ass-guard seam — observe-only", path, ev)
		}

		for _, group := range raw.Hooks[ev] {
			for _, h := range group.Hooks {
				if h.Type != "" && h.Type != "command" {
					logPluginSkipf("hooks %s: unsupported hook type %q (skipped)", path, h.Type)

					continue
				}

				if h.Command == "" {
					continue
				}

				out = append(out, HookConfig{
					Event: ev, Matcher: group.Matcher, Command: h.Command,
					TimeoutSec: h.Timeout, Path: path, PluginRoot: pluginRoot, Scope: scope,
				})
			}
		}
	}

	return out
}

// Fire executes every hook bound to event. fields carry the event-specific
// payload entries (tool_name/tool_input/tool_response/prompt). Multiple
// hooks' stdout concatenates into Message (context events); an exit-2
// PreToolUse refusal flips Proceed to false with the hook's stderr as
// Message. Never returns on the failure paths — every degradation is a
// warned skip with Proceed intact.
func (r *HookRunner) Fire(ctx context.Context, event string, fields map[string]any) HookOutcome {
	if r == nil || len(r.Hooks) == 0 {
		return HookOutcome{Proceed: true}
	}

	if !hookEventsMapped[event] {
		logPluginSkipf("hook event %s is unmapped in ass-guard — observe-only (nothing fired)", event)

		return HookOutcome{Proceed: true}
	}

	toolName, _ := fields[keyToolName].(string)

	out := HookOutcome{Proceed: true}

	var sb strings.Builder

	matches := r.matchingHooks(event, toolName)

	for i := range matches {
		res := r.runOne(ctx, &matches[i], event, fields)

		switch {
		case res.refused && event == "PreToolUse":
			// The ONE blocking semantic (operator policy channel): the call
			// is refused with the hook's message as the tool result.
			return HookOutcome{Proceed: false, Message: res.message}
		case res.refused:
			// Exit 2 on a non-tool event has no blocking seam — warn + proceed.
			logPluginSkipf("hook [%s] exit 2 has no blocking seam here (ignored): %s", event, res.message)
		case res.stdout != "":
			sb.WriteString(res.stdout)
		}
	}

	out.Message = sb.String()

	return out
}

// PreToolUseVerdict runs every matching PreToolUse hook SEQUENTIALLY in the
// partitioned D-03 order (never concurrently — the flagged PAR-03
// concurrency guarantee) under the existing runOne bounds (per-hook timeout,
// output cap, sanitized env, stdin payload), parses each execution into a
// Verdict, and delegates the combined verdict to the pure ResolveVerdict.
// This is the structured seam the Phase-17 gate head consumes when 21-06
// joins hooks to gateCall — the RESOLUTION site; consumption stays the
// gate's alone. Nil runner is a safe no-op (no decision).
func (r *HookRunner) PreToolUseVerdict(
	ctx context.Context, toolName string, input json.RawMessage,
) (verdict Verdict, reason string) { //nolint:nonamedreturns // the D-02 verdict pair
	if r == nil {
		return VerdictNone, ""
	}

	matches := r.matchingHooks(hookEventPreToolUse, toolName)

	results := make([]ScopedResult, 0, len(matches))

	for i := range matches {
		res := r.runOne(ctx, &matches[i], hookEventPreToolUse, map[string]any{
			keyToolName:  toolName,
			keyToolInput: input,
		})

		v, resReason := parseHookVerdict(res.stdout, res.runErr)
		if res.refused {
			// Exit 2 already yields deny; upgrade the reason to
			// classifyHookRun's stderr-first extraction (D-02 — the only
			// place stderr exists).
			resReason = res.message
		}

		results = append(results, ScopedResult{Scope: matches[i].Scope, Verdict: v, Reason: resReason})
	}

	return ResolveVerdict(results)
}

// PostToolUse implements the coreexec.ToolHooks seam (reduced to observation
// at the 21-06 gate join): observes the completed result (never blocks).
// The boolean PreToolUse pair this runner once exposed for the executor was
// DELETED at the join — PreToolUseVerdict is the verdict surface, and the
// session gate head (internal/session/gate.go) is its ONE consumer.
func (r *HookRunner) PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage) {
	_ = r.Fire(ctx, hookEventPostToolUse, map[string]any{
		keyToolName:     toolName,
		keyToolInput:    input,
		keyToolResponse: output,
	})
}

// matchingHooks returns the hooks bound to event that match toolName (the
// matcher applies only to the tool events; empty/"*" matches every tool).
// Settings-scope hooks match by CC's two-path dialect (matchSettingsHook);
// plugin hooks keep the legacy compile-everything-as-regex behavior
// byte-for-byte (D-02: existing plugin hooks unchanged). An invalid matcher
// regex warns and matches nothing (never fatal).
func (r *HookRunner) matchingHooks(event, toolName string) []HookConfig {
	var out []HookConfig

	for _, h := range r.Hooks {
		if h.Event != event {
			continue
		}

		if !hookToolEvents[event] {
			out = append(out, h)

			continue
		}

		if h.Matcher == "" || h.Matcher == "*" {
			out = append(out, h)

			continue
		}

		if isSettingsScope(h.Scope) {
			if matchSettingsHook(h.Matcher, toolName) {
				out = append(out, h)
			}

			continue
		}

		re, err := regexp.Compile(h.Matcher)
		if err != nil {
			logPluginSkipf("hooks %s: invalid matcher %q (%v) — hook never fires", h.Path, h.Matcher, err)

			continue
		}

		if re.MatchString(toolName) {
			out = append(out, h)
		}
	}

	return out
}

// isSettingsScope reports whether the scope uses CC's settings matcher
// dialect (the two settings.json scopes; plugin bundles stay legacy).
func isSettingsScope(s HookScope) bool {
	return s == ScopeProject || s == ScopeUser
}

// matchSettingsHook implements CC's two-path matcher dialect for
// settings-scope hooks (21-01, Pitfall 3): a matcher whose every rune is
// alphanumeric, underscore, hyphen, space, comma, or pipe splits on pipes,
// commas, and spaces into EXACT alternatives compared with == against the
// tool name; anything else compiles as an unanchored regex. KNOWN DIVERGENCE
// (resolved open question 2): plugin hooks deliberately keep the legacy
// compile-everything-as-regex behavior, so plugin matcher "Edit" still
// matches "NotebookEdit" while settings matcher "Edit" does not.
func matchSettingsHook(matcher, toolName string) bool {
	if settingsMatcherExactSet(matcher) {
		alts := strings.FieldsFunc(matcher, func(r rune) bool {
			return r == '|' || r == ',' || r == ' '
		})

		return slices.Contains(alts, toolName)
	}

	re, err := regexp.Compile(matcher)
	if err != nil {
		logPluginSkipf("settings hook: invalid matcher %q (%v) — hook never fires", matcher, err)

		return false
	}

	return re.MatchString(toolName)
}

// settingsMatcherExactSet reports whether every rune of matcher is in CC's
// exact-set: alphanumeric, underscore, hyphen, space, comma, or pipe. Such
// matchers take the exact/alternatives path; any other rune (dot, star,
// paren, slash, ...) forces the regex path. The matcher is non-empty here
// (the caller handled the empty/"*" catch-alls first).
func settingsMatcherExactSet(matcher string) bool {
	for _, r := range matcher {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == ' ' || r == ',' || r == '|':
		default:
			return false
		}
	}

	return true
}

// hookExecResult is one hook execution's classification.
type hookExecResult struct {
	refused bool   // exit code 2
	message string // refusal text (stderr, stdout fallback)
	stdout  string // captured stdout (context payload)

	// runErr is the raw exec error (nil on success; the ctx/timeout error on
	// a killed run) — parseHookVerdict's exit-2-vs-execution-failure channel
	// (21-01).
	runErr error
}

// runOne executes ONE hook command with the documented bounds: `sh -c`,
// cwd = the session workdir, SANITIZED env (PATH/HOME only, plus
// CLAUDE_PLUGIN_ROOT when the hook carries plugin provenance — the documented
// expansion vehicle for bundled scripts), the documented stdin JSON, a
// per-hook timeout (default 60s), and capped output capture. Every failure
// path warns through the stderr seam and proceeds (never fatal).
func (r *HookRunner) runOne(ctx context.Context, h *HookConfig, event string, fields map[string]any) hookExecResult {
	timeoutSec := h.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = hookDefaultTimeoutSec
	}

	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	payload, err := r.buildPayload(event, fields)
	if err != nil {
		logPluginSkipf("hook [%s] payload build failed (skipped): %v", event, err)

		return hookExecResult{}
	}

	//nolint:gosec // hook commands are operator-installed config (the hook-DAG trust tier)
	cmd := exec.CommandContext(tctx, "sh", "-c", h.Command)

	if r.WorkDir != "" {
		cmd.Dir = r.WorkDir
	}

	cmd.Env = sanitizedHookEnv(h.PluginRoot)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// A timed-out hook is killed by tctx; the group-kill straggler race is
	// bounded the same way as coreexec's Bash (a wrapper shell's orphan is
	// parented to init and never blocks this path — the Wait already
	// returned). The raw runErr rides along so parseHookVerdict classifies
	// the kill as execution failure → fail-open (21-01).
	if tctx.Err() != nil {
		logPluginSkipf("hook [%s] timeout after %ds (skipped): %s", event, timeoutSec, h.Command)

		return hookExecResult{runErr: runErr}
	}

	return classifyHookRun(event, runErr, stdout.String(), stderr.String())
}

// classifyHookRun maps a finished hook command's outcome to the documented
// semantics: exit 2 refuses (stderr as the message, stdout fallback); any
// other failure warns and skips; success captures (capped) stdout. The raw
// runErr rides on every result for parseHookVerdict (21-01).
func classifyHookRun(event string, runErr error, stdout, stderr string) hookExecResult {
	if runErr == nil {
		return hookExecResult{stdout: capHookOutput(stdout)}
	}

	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		logPluginSkipf("hook [%s] failed to start (skipped): %v", event, runErr)

		return hookExecResult{runErr: runErr}
	}

	if exitErr.ExitCode() == hookRefusalExitCode {
		msg := capHookOutput(stderr)
		if msg == "" {
			msg = capHookOutput(stdout)
		}

		return hookExecResult{refused: true, message: msg, runErr: runErr}
	}

	logPluginSkipf("hook [%s] exited %d (skipped): %s", event, exitErr.ExitCode(), capHookOutput(stderr))

	return hookExecResult{runErr: runErr}
}

// buildPayload renders the documented stdin JSON: the base fields
// (session_id, transcript_path, cwd, hook_event_name) plus the event-specific
// entries from fields (json.RawMessage values embed verbatim).
func (r *HookRunner) buildPayload(event string, fields map[string]any) ([]byte, error) {
	payload := map[string]any{
		"session_id":      r.SessionID,
		"transcript_path": r.TranscriptPath,
		"cwd":             r.WorkDir,
		"hook_event_name": event,
	}

	maps.Copy(payload, fields)

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ecosys: marshal hook payload: %w", err)
	}

	return out, nil
}

// sanitizedHookEnv is the minimal hook environment: PATH + HOME only, plus
// CLAUDE_PLUGIN_ROOT when the hook carries plugin provenance. No other parent
// variable reaches the hook (T-12-02-06 information-disclosure bound).
func sanitizedHookEnv(pluginRoot string) []string {
	env := []string{
		"PATH=" + envOrEmpty("PATH"),
		"HOME=" + envOrEmpty("HOME"),
	}

	if pluginRoot != "" {
		env = append(env, "CLAUDE_PLUGIN_ROOT="+pluginRoot)
	}

	return env
}

// capHookOutput applies the captured-output cap.
func capHookOutput(s string) string {
	if len(s) <= hookOutputCap {
		return s
	}

	return s[:hookOutputCap]
}

// envOrEmpty reads an env var ("" when unset).
func envOrEmpty(key string) string { return os.Getenv(key) }
