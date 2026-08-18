// The Claude-Code hook runner (12-02 Task 4): plugin-bundled hooks/hooks.json
// entries fire at the mapped lifecycle seams with the documented stdin-JSON /
// stdout / exit-code semantics. Hooks are operator-installed config executing
// operator-chosen commands — the SAME trust tier as the hook-DAG (the standing
// safety model); the exit-2 PreToolUse refusal is a policy channel, NOT a new
// confirmation tier. Every firing is bounded and never fatal (AUD-03: a hook
// failure is an audit-loud stderr skip, logged through the swappable seam).
package ecosys

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// HookConfig is one parsed hook command from a plugin bundle's
// hooks/hooks.json (12-02): the lifecycle Event it fires at, the Matcher
// (a tool-name regex for the tool events; empty/"*" matches all), the shell
// Command (run via `sh -c`), its per-hook timeout, and provenance (the
// hooks.json Path + the bundle's PluginRoot, exported to the hook's env as
// CLAUDE_PLUGIN_ROOT — the documented expansion vehicle).
type HookConfig struct {
	Event      string
	Matcher    string
	Command    string
	TimeoutSec int
	Path       string
	PluginRoot string
}

// HookOutcome is one Fire result: Proceed=false ONLY for an exit-2 PreToolUse
// refusal (the operator-configured policy channel — the hook-table safety
// model, NOT a new confirmation tier); Message carries the refusal text or
// the captured stdout (the context-bearing events' injection payload).
type HookOutcome struct {
	Proceed bool
	Message string
}

// hookEventsMapped is the set of Claude-Code hook events ass-guard has a seam
// for (12-02). Every other event name (Notification, PreCompact, future
// additions) is UNMAPPED: it parses, warns, and degrades to observe-only —
// documented, never silently ignored.
var hookEventsMapped = map[string]bool{ //nolint:gochecknoglobals // immutable table
	"PreToolUse":       true,
	"PostToolUse":      true,
	"UserPromptSubmit": true,
	"Stop":             true,
	"SubagentStop":     true,
	"SessionStart":     true,
	"SessionEnd":       true,
}

// hookToolEvents are the events whose matcher is a tool-name regex.
var hookToolEvents = map[string]bool{ //nolint:gochecknoglobals // immutable table
	"PreToolUse":  true,
	"PostToolUse": true,
}

// Runner bounds (T-12-02-03/T-12-02-06): the per-hook default timeout
// (Claude Code's documented 60s) and the captured-output cap (the documented
// 30000-char discipline, applied to stdout AND stderr).
const (
	hookDefaultTimeoutSec = 60
	hookOutputCap         = 30000
)

// HookRunner executes Registry.Hooks at the mapped lifecycle seams:
//
//   - UserPromptSubmit       — turn entry (post-expansion; session.Prompt)
//   - PreToolUse/PostToolUse — the tool-exec chokepoint (coreexec)
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

// NewHookRunner builds a runner over the discovered hooks for one session.
func NewHookRunner(hooks []HookConfig, sessionID, workDir, transcriptPath string) *HookRunner {
	return &HookRunner{Hooks: hooks, SessionID: sessionID, WorkDir: workDir, TranscriptPath: transcriptPath}
}

// parseHooksJSON reads one hooks/hooks.json (the live-measured shape: a
// top-level object with a `hooks` map keyed by event name → matcher groups →
// command entries) into HookConfigs. Deterministic event order (sorted keys).
// A malformed file or non-command hook entry skips with a warning; an unmapped
// event parses + warns (observe-only).
func parseHooksJSON(path, pluginRoot string) []HookConfig {
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
		logPluginSkip("malformed or empty hooks file %s (skipped)", path)

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
			logPluginSkip("hooks %s: event %s has no ass-guard seam — observe-only", path, ev)
		}

		for _, group := range raw.Hooks[ev] {
			for _, h := range group.Hooks {
				if h.Type != "" && h.Type != "command" {
					logPluginSkip("hooks %s: unsupported hook type %q (skipped)", path, h.Type)

					continue
				}

				if h.Command == "" {
					continue
				}

				out = append(out, HookConfig{
					Event: ev, Matcher: group.Matcher, Command: h.Command,
					TimeoutSec: h.Timeout, Path: path, PluginRoot: pluginRoot,
				})
			}
		}
	}

	return out
}

// matchingHooks returns the hooks bound to event that match toolName (the
// matcher applies only to the tool events; empty/"*" matches every tool).
// An invalid matcher regex warns and matches nothing (never fatal).
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

		re, err := regexp.Compile(h.Matcher)
		if err != nil {
			logPluginSkip("hooks %s: invalid matcher %q (%v) — hook never fires", h.Path, h.Matcher, err)

			continue
		}

		if re.MatchString(toolName) {
			out = append(out, h)
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
		logPluginSkip("hook event %s is unmapped in ass-guard — observe-only (nothing fired)", event)

		return HookOutcome{Proceed: true}
	}

	toolName, _ := fields["tool_name"].(string)

	out := HookOutcome{Proceed: true}

	var sb strings.Builder

	for _, h := range r.matchingHooks(event, toolName) {
		res := r.runOne(ctx, h, event, fields)

		switch {
		case res.refused && event == "PreToolUse":
			// The ONE blocking semantic (operator policy channel): the call
			// is refused with the hook's message as the tool result.
			return HookOutcome{Proceed: false, Message: res.message}
		case res.refused:
			// Exit 2 on a non-tool event has no blocking seam — warn + proceed.
			logPluginSkip("hook [%s] exit 2 has no blocking seam here (ignored): %s", event, res.message)
		case res.stdout != "":
			sb.WriteString(res.stdout)
		}
	}

	out.Message = sb.String()

	return out
}

// PreToolUse implements the coreexec.ToolHooks seam: consults the hook table
// BEFORE the tool runs; (false, message) refuses the call.
func (r *HookRunner) PreToolUse(ctx context.Context, toolName string, input json.RawMessage) (bool, string) {
	out := r.Fire(ctx, "PreToolUse", map[string]any{
		"tool_name":  toolName,
		"tool_input": input,
	})

	return out.Proceed, out.Message
}

// PostToolUse implements the coreexec.ToolHooks seam: observes the completed
// result (never blocks).
func (r *HookRunner) PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage) {
	_ = r.Fire(ctx, "PostToolUse", map[string]any{
		"tool_name":     toolName,
		"tool_input":    input,
		"tool_response": output,
	})
}

// hookExecResult is one hook execution's classification.
type hookExecResult struct {
	refused bool   // exit code 2
	message string // refusal text (stderr, stdout fallback)
	stdout  string // captured stdout (context payload)
}

// runOne executes ONE hook command with the documented bounds: `sh -c`,
// cwd = the session workdir, SANITIZED env (PATH/HOME only, plus
// CLAUDE_PLUGIN_ROOT when the hook carries plugin provenance — the documented
// expansion vehicle for bundled scripts), the documented stdin JSON, a
// per-hook timeout (default 60s), and capped output capture. Every failure
// path warns through the stderr seam and proceeds (never fatal).
func (r *HookRunner) runOne(ctx context.Context, h HookConfig, event string, fields map[string]any) hookExecResult {
	timeoutSec := h.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = hookDefaultTimeoutSec
	}

	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	payload, err := r.buildPayload(event, fields)
	if err != nil {
		logPluginSkip("hook [%s] payload build failed (skipped): %v", event, err)

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
	// returned).
	if tctx.Err() != nil {
		logPluginSkip("hook [%s] timeout after %ds (skipped): %s", event, timeoutSec, h.Command)

		return hookExecResult{}
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			switch exitErr.ExitCode() {
			case 2:
				msg := capHookOutput(stderr.String())
				if msg == "" {
					msg = capHookOutput(stdout.String())
				}

				return hookExecResult{refused: true, message: msg}
			default:
				logPluginSkip("hook [%s] exited %d (skipped): %s", event, exitErr.ExitCode(),
					capHookOutput(stderr.String()))

				return hookExecResult{}
			}
		}

		logPluginSkip("hook [%s] failed to start (skipped): %v", event, runErr)

		return hookExecResult{}
	}

	return hookExecResult{stdout: capHookOutput(stdout.String())}
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

	for k, v := range fields {
		payload[k] = v
	}

	return json.Marshal(payload)
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
