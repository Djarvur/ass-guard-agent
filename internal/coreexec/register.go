package coreexec

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// ToolHooks is the PreToolUse/PostToolUse seam at the tool-exec chokepoint
// (12-02 Task 4): PreToolUse consults the operator-configured hook table
// BEFORE the tool runs (proceed=false REFUSES the call — the hook's message
// becomes the tool result; the hook-table safety model, NOT a new
// confirmation tier); PostToolUse observes the completed result (never
// blocks). Implemented structurally by ecosys.HookRunner.
type ToolHooks interface {
	PreToolUse(ctx context.Context, toolName string, input json.RawMessage) (proceed bool, message string)
	PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage)
}

// Config carries the per-session state the core executors close over
// (constructed ONCE at the sessionFor registration site — the 08-05 Skill
// pattern generalized): WorkDir is the session's working directory (Bash's
// cmd.Dir; absolute-path rendering for Write/Edit texts), Todos is the
// per-session todo store (D-16 isolation: every session gets its own), and
// Hooks is the optional PreToolUse/PostToolUse seam (nil = no hook wrap).
type Config struct {
	WorkDir string
	Todos   *TodoStore
	Hooks   ToolHooks
}

// RegisterCore sets Execute on the six core catalog entries (08-08's /opsx
// working set: Bash, Read, Write, Edit, TodoWrite, TodoRead) — one function,
// idempotent. ONLY the Execute field is overridden: Name, Description,
// InputSchema, and Mutability stay byte-identical to the captured catalog
// entry (the 08-05 Skill-override discipline — the captured schema is the
// mimicry target, never rewritten by an implementation). A missing catalog
// entry is skipped, not fatal (forward-compat with catalog revisions that
// rename tools). When cfg.Hooks is set, each stub is wrapped with the
// PreToolUse/PostToolUse seam (an exit-2 refusal returns the structured
// {"error":"hook: …"} result — the captured failure convention).
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

// withHooks wraps one core stub with the PreToolUse/PostToolUse seam. A
// PreToolUse refusal short-circuits execution and returns the structured
// error form (IsError) carrying the hook's message as the tool result.
func withHooks(name string, exec toolcat.Stub, hooks ToolHooks) toolcat.Stub {
	if hooks == nil {
		return exec
	}

	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		if proceed, message := hooks.PreToolUse(ctx, name, args); !proceed {
			return marshalStructured("hook: "+message, errHookRefused)
		}

		out, err := exec(ctx, args)

		hooks.PostToolUse(ctx, name, args, out)

		return out, err
	}
}
