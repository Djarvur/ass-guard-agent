package coreexec

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// ToolHooks is the PostToolUse observation seam at the tool-exec chokepoint
// (12-02 Task 4; reduced at the 21-06 gate join): PostToolUse observes the
// completed result (never blocks). The PreToolUse consultation leg this
// seam once carried was DISPOSED when hook verdicts joined Phase 17's
// single gate pipeline at internal/session's gateCall head (21-06, 21-RESEARCH
// Pitfall 2: two consultation sites meant double hook firing and two result
// forms per denial). Policy is the gate's alone; observation stays
// executor-side. Implemented structurally by ecosys.HookRunner.
type ToolHooks interface {
	PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage)
}

// Config carries the per-session state the core executors close over
// (constructed ONCE at the sessionFor registration site — the 08-05 Skill
// pattern generalized): WorkDir is the session's working directory (Bash's
// cmd.Dir; absolute-path rendering for Write/Edit texts), Todos is the
// per-session todo store (D-16 isolation: every session gets its own), and
// Hooks is the optional PostToolUse observation seam (nil = no hook wrap).
type Config struct {
	WorkDir string
	Todos   *TodoStore
	Hooks   ToolHooks
	// Tasks owns Bash run_in_background executions (12-06, ACP-06): nil =
	// background starts degrade to the structured no-registry error (the
	// foreground path is unaffected).
	Tasks *TaskRegistry
}

// RegisterCore sets Execute on the six core catalog entries (08-08's /opsx
// working set: Bash, Read, Write, Edit, TodoWrite, TodoRead) — one function,
// idempotent. ONLY the Execute field is overridden: Name, Description,
// InputSchema, and Mutability stay byte-identical to the captured catalog
// entry (the 08-05 Skill-override discipline — the captured schema is the
// mimicry target, never rewritten by an implementation). A missing catalog
// entry is skipped, not fatal (forward-compat with catalog revisions that
// rename tools). When cfg.Hooks is set, each stub is wrapped with the
// PostToolUse observation seam (the PreToolUse leg was disposed at the
// 21-06 gate join — hook verdicts consume at the session gate head only).
func RegisterCore(catalog *toolcat.Catalog, cfg Config) {
	if catalog == nil {
		return
	}

	stubs := map[string]toolcat.Stub{
		"Bash":      withHooks("Bash", BashExecute(cfg), cfg.Hooks),
		"Read":      withHooks("Read", ReadExecute(cfg), cfg.Hooks),
		"Write":     withHooks("Write", WriteExecute(cfg), cfg.Hooks),
		"Edit":      withHooks("Edit", EditExecute(cfg), cfg.Hooks),
		"TodoWrite": withHooks("TodoWrite", TodoWriteExecute(cfg.Todos), cfg.Hooks),
		"TodoRead":  withHooks("TodoRead", TodoReadExecute(cfg.Todos), cfg.Hooks),
	}

	for name, exec := range stubs {
		if tool, ok := catalog.Get(name); ok {
			tool.Execute = exec
			catalog.Register(tool)
		}
	}
}

// withHooks wraps one core stub with the PostToolUse observation seam: the
// tool runs, then the hook observes the completed output. It never blocks
// and never refuses — the 12-02 PreToolUse consultation that lived here was
// deleted at the 21-06 gate join (a dead policy seam beside the gate head
// was a double-fire and bypass hazard, 21-RESEARCH Pitfall 2 / Open
// Question 1; Phase 25 re-homes the shape if the kit needs it).
func withHooks(name string, exec toolcat.Stub, hooks ToolHooks) toolcat.Stub {
	if hooks == nil {
		return exec
	}

	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		out, err := exec(ctx, args)

		hooks.PostToolUse(ctx, name, args, out)

		return out, err
	}
}
