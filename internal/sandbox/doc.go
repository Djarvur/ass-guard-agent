// Package sandbox provides the OS-sandbox machinery for spawned tool
// processes (22-05, SAND-01): one Policy struct feeds two backends —
// landlock (Linux ≥5.13, go-landlock v0.10.0, applied in a re-exec child)
// and sandbox-exec (macOS, in-memory profile via -p) — behind one portable
// entry, Wrap/WrapCmd.
//
// # Confinement boundary (the D-05 honesty note)
//
// What IS confined: SPAWNED tool processes only — the Bash-class
// subprocesses (foreground Bash exec, TaskRegistry.Start's background exec,
// the PTY persistent-shell spawn). The ruleset is applied to the child
// before exec (Linux: inside the re-exec sentinel child, since landlock is
// process-wide and irreversible; macOS: sandbox-exec IS the confined child).
//
// What is NOT confined: the ass-guard process itself (applying landlock
// here would sever provider/MCP networking — landlock has no per-scope
// carve-out), in-process tools, and MCP host connections. A confined child
// that spawns its own children IS confined transitively (kernel
// clone-inheritance; docs.kernel.org: "Every new thread resulting from a
// clone(2) inherits Landlock domain restrictions from its parent").
//
// # MPTCP caveat (Pitfall 2)
//
// Since Go 1.24, net.Listen defaults to Multipath TCP, which landlock's TCP
// rules do not cover (kernel bug landlock-lsm/linux#54): a confined Go
// ≥1.24 CHILD could bind via MPTCP outside the TCP rules. Non-Go children
// (curl, npm — the normal case) are unaffected on the connect side. Known
// boundary, documented rather than silently claimed.
//
// # Probe-and-degrade (SAND-01)
//
// Enforcement availability is probed STRICTLY (never BestEffort — a
// best-effort ruleset succeeds without error even with no Landlock at all,
// which is a silent fail-open): ENOSYS (kernel lacks Landlock), EOPNOTSUPP
// (disabled), ABI < 4 (network rules need v4), and missing sandbox-exec
// each map to a distinct Availability reason. When unavailable, tool
// processes run UNCONFINED with a loud stderr note — the sandbox is OFF by
// default and never refuses the run.
package sandbox
