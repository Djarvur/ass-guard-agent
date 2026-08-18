// Package ecosys — the Claude-Code hook runner (12-02 Task 4).
//
// RED SCAFFOLD: types + seams compile; behavior lands in the GREEN commit.
package ecosys

import (
	"context"
	"encoding/json"
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

// HookRunner executes Registry.Hooks at the mapped lifecycle seams with
// Claude-Code semantics where ass-guard has a seam (12-02):
//
//   - UserPromptSubmit    — turn entry (post-expansion; session.Prompt)
//   - PreToolUse/PostToolUse — the tool-exec chokepoint (coreexec)
//   - Stop/SubagentStop   — parent/subagent turn end
//   - SessionStart/SessionEnd — session create/close
//
// Unmapped events (Notification, PreCompact) parse + warn + degrade to
// observe-only. Every firing is bounded (per-hook timeout, output caps,
// sanitized env, session-workdir cwd) and NEVER fatal to the turn — a hook
// failure is an audit-loud skip (the AUD-03 pattern).
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

// Fire executes every hook bound to event (RED STUB — always proceeds).
func (r *HookRunner) Fire(_ context.Context, _ string, _ map[string]any) HookOutcome {
	return HookOutcome{Proceed: true}
}

// PreToolUse implements the coreexec.ToolHooks seam (RED STUB).
func (r *HookRunner) PreToolUse(_ context.Context, _ string, _ json.RawMessage) (bool, string) {
	return true, ""
}

// PostToolUse implements the coreexec.ToolHooks seam (RED STUB).
func (r *HookRunner) PostToolUse(_ context.Context, _ string, _, _ json.RawMessage) {}

// hookOutputCap bounds captured hook output (RED STUB constant).
const hookOutputCap = 30000

// matchingHooks returns the hooks bound to event that match toolName (RED
// STUB — matches nothing).
func (r *HookRunner) matchingHooks(_ string, _ string) []HookConfig {
	return nil
}
