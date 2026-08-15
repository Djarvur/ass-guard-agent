package coreexec

import (
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// Config carries the per-session state the core executors close over
// (constructed ONCE at the sessionFor registration site — the 08-05 Skill
// pattern generalized): WorkDir is the session's working directory (Bash's
// cmd.Dir; absolute-path rendering for Write/Edit texts), Todos is the
// per-session todo store (D-16 isolation: every session gets its own).
type Config struct {
	WorkDir string
	Todos   *TodoStore
}

// RegisterCore sets Execute on the six core catalog entries (08-08's /opsx
// working set: Bash, Read, Write, Edit, TodoWrite, TodoRead) — one function,
// idempotent. ONLY the Execute field is overridden: Name, Description,
// InputSchema, and Mutability stay byte-identical to the captured catalog
// entry (the 08-05 Skill-override discipline — the captured schema is the
// mimicry target, never rewritten by an implementation). A missing catalog
// entry is skipped, not fatal (forward-compat with catalog revisions that
// rename tools).
func RegisterCore(catalog *toolcat.Catalog, cfg Config) {
	if catalog == nil {
		return
	}

	stubs := map[string]toolcat.Stub{
		"Bash":      BashExecute(cfg),
		"Read":      ReadExecute(cfg),
		"Write":     WriteExecute(cfg),
		"Edit":      EditExecute(cfg),
		"TodoWrite": TodoWriteExecute(cfg.Todos),
		"TodoRead":  TodoReadExecute(cfg.Todos),
	}

	for name, exec := range stubs {
		if tool, ok := catalog.Get(name); ok {
			tool.Execute = exec
			catalog.Register(tool)
		}
	}
}
