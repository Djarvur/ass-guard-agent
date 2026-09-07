// Package sandbox is the OS-sandbox machinery for spawned tool processes
// (22-05, SAND-01). See doc.go for the confinement-boundary honesty note.
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Policy is the ONE policy definition feeding BOTH backends (D-06): the
// same struct renders a landlock ruleset (Linux) and a seatbelt profile
// (macOS) — drift between OSes is impossible by construction. Paths come
// from the operator/session (workdir/tmp/.ass-guard per session), NEVER
// from the model.
type Policy struct {
	// RWPaths: full read-write (D-04's triple: workdir, session tmp,
	// workdir/.ass-guard).
	RWPaths []string `json:"rw_paths"`
	// ROSysPaths: read-only system paths (per-distro discretion, A8; the
	// landlock side restricts to these, the seatbelt side reads freely under
	// allow-default — symmetric OUTCOME, different mechanism).
	ROSysPaths []string `json:"ro_sys_paths"`
	// DenyNetwork: always true in v1 (D-04 — blocks curl-style exfil from
	// confined tools).
	DenyNetwork bool `json:"deny_network"`
}

// Rule is one landlock-shaped access row (the V4 description the linux
// backend consumes and the symmetry golden pins — portable so BOTH OSes
// assert against the same rows).
type Rule struct {
	Path   string // the target path (dir or file)
	Access string // "ro" | "rw"
}

// DefaultPolicy builds the D-04 policy: rw on workDir + tmpDir + guardDir,
// ro on the documented system set, network denied. The system set is the
// per-distro discretion item (RESEARCH A8): a common-denominator set both
// majors accept; the loud-degrade contract covers gaps.
func DefaultPolicy(workDir, tmpDir, guardDir string) Policy {
	return Policy{
		RWPaths: []string{workDir, tmpDir, guardDir},
		ROSysPaths: []string{
			"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc",
			"/System", "/Library",
		},
		DenyNetwork: true,
	}
}

// LandlockRules returns the policy's V4-shape rows: one rw row per RWPaths
// entry, one ro row per ROSysPaths entry (DenyNetwork is not a path rule —
// it maps to RestrictNet on the linux side and (deny network*) on darwin).
func (p Policy) LandlockRules() []Rule {
	rules := make([]Rule, 0, len(p.RWPaths)+len(p.ROSysPaths))

	for _, path := range p.RWPaths {
		rules = append(rules, Rule{Path: path, Access: "rw"})
	}

	for _, path := range p.ROSysPaths {
		rules = append(rules, Rule{Path: path, Access: "ro"})
	}

	return rules
}

// SeatbeltProfile renders the embedded .sb template IN MEMORY with the
// policy's paths substituted (D-06: no runtime profile artifacts on disk —
// the text rides `sandbox-exec -p` argv). Targeted denies over
// (allow default); never deny-default (SAND-01).
//
// The deny set (D-04): network denied entirely; the file-write family
// denied globally then re-allowed for exactly the rw triple. PR_SET_NO_NEW
// privileges is seatbelt-exec's own contract (documented in policy.go's
// threat register: T-22-21).
//
// PATH RESOLUTION (live-probed on this host): seatbelt subpath filters
// match the RESOLVED path — /tmp is a symlink to /private/tmp on macOS, and
// a subpath allow on the symlinked spelling never matches. Every rw path is
// symlink-resolved (best-effort EvalSymlinks; the original survives as the
// fallback when resolution fails, e.g. a not-yet-created dir).
func (p Policy) SeatbeltProfile() string {
	var sb strings.Builder

	sb.WriteString("(version 1)\n")
	sb.WriteString("(allow default)\n")

	if p.DenyNetwork {
		sb.WriteString("(deny network*)\n")
	}

	sb.WriteString("(deny file-write*)\n")

	if len(p.RWPaths) > 0 {
		sb.WriteString("(allow file-write*")

		for _, path := range p.RWPaths {
			sb.WriteString(` (subpath "` + resolveForSeatbelt(path) + `")`)
		}

		sb.WriteString(")\n")
	}

	return sb.String()
}

// resolveForSeatbelt symlink-resolves a path for the profile render
// (best-effort: the original survives when resolution fails).
func resolveForSeatbelt(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		return resolved
	}

	// The path itself may not exist yet — resolve its deepest existing
	// ancestor and reattach the tail.
	dir, tail := filepath.Split(path)

	if resolvedDir, derr := filepath.EvalSymlinks(filepath.Clean(dir)); derr == nil {
		return filepath.Join(resolvedDir, tail)
	}

	return path
}

// Availability is the probe taxonomy's result (SAND-01): each failure class
// — ENOSYS (kernel lacks Landlock), EOPNOTSUPP (disabled), ABI < 4, missing
// sandbox-exec — maps to a DISTINCT reason; best-effort mode is never the
// enforcement path (Pitfall 1).
type Availability struct {
	Mode      string // "landlock" | "seatbelt" | ""
	Available bool
	Reason    string // empty when Available
}

// Handle is the resolved immutable pair executors read (22-06): the policy
// plus the probe outcome for this host.
type Handle struct {
	Policy       Policy
	Availability Availability
}

// Probe resolves the host's enforcement availability for the policy's mode
// (darwin: seatbelt first-use probe; linux: strict ProbeABI). Cheap after
// the first call (cached per process).
func Probe() Availability {
	return probePlatform()
}

// Resolve pairs a policy with the host's probe outcome.
func Resolve(p Policy) Handle {
	return Handle{Policy: p, Availability: Probe()}
}

// Wrap confines cmd's child with the policy: the ONE portable enforcement
// entry (D-06) — every exec site (22-06: foreground Bash, background Start,
// PTY spawn) goes through here; no call site re-implements argv
// substitution. Substitution-ONLY contract: Path/Args are replaced, Dir and
// SysProcAttr are never touched (the lifecycle stays the caller's); Env is
// appended to, never replaced.
//
// When the host's enforcement is unavailable (probe degraded), the command
// runs UNCONFINED — loudly noted once (SAND-01's degrade contract; 22-06
// surfaces the startup warning and per-run notes from the Availability).
func Wrap(cmd *exec.Cmd, p Policy) error {
	return wrapPlatform(cmd, p)
}

// WrapCmd is Wrap under the plan's artifact name (kept as the documented
// single entry; Wrap is the short alias the exec sites use).
func WrapCmd(cmd *exec.Cmd, p Policy) error {
	return wrapPlatform(cmd, p)
}

// The sandbox-child sentinel protocol (linux leg, Pattern 4): ass-guard's
// own binary re-execs as the sandbox loader. os/exec has no pre-exec child
// hook, so WrapCmd(linux) rewrites the child to `[self] --sandbox-child
// <original argv...>` with the JSON policy in the environment; the child
// main path intercepts via ApplySandboxChildHook BEFORE normal dispatch.
const (
	// ChildEnvSentinel marks the process as a sandbox loader child.
	ChildEnvSentinel = "__ASS_GUARD_SANDBOX_CHILD"
	// ChildEnvPolicy carries the JSON Policy for the loader child.
	ChildEnvPolicy = "__ASS_GUARD_SANDBOX_POLICY"
)

// ChildTargetArgv is the argv marker the loader child carries the original
// target behind (kept beside the sentinel constants so both legs share them).
const ChildTargetArgv = "--sandbox-child"

// IsSandboxChild reports whether THIS process was spawned as a sandbox
// loader child (the sentinel env fires exclusively on WrapCmd-spawned
// children — never on a normal serve start; prohibition 2).
func IsSandboxChild() bool {
	return os.Getenv(ChildEnvSentinel) == "1"
}

// ApplySandboxChildHook is the portable main-path hook: main() (22-06 wires
// the call) invokes it unconditionally before root-command dispatch. On the
// linux leg with the sentinel set it applies the child ruleset and
// syscall.Execs the real target — NEVER returning. On every other GOOS (or
// without the sentinel) it returns false immediately with zero side
// effects.
func ApplySandboxChildHook() bool {
	return applyChildHookPlatform()
}

// marshalPolicy serializes the policy for the child env (operator/session
// sourced; the model never supplies it — the env is set by ass-guard at
// spawn, and the child is already confined when it could read it, T-22-23).
func marshalPolicy(p Policy) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("sandbox: marshal policy: %w", err)
	}

	return string(b), nil
}

// unmarshalPolicy is the loader child's parse half.
func unmarshalPolicy(s string) (Policy, error) {
	var p Policy

	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return Policy{}, fmt.Errorf("sandbox: unmarshal policy: %w", err)
	}

	return p, nil
}

// probeOnce guards the per-process availability cache (the probe runs real
// children — never per-exec).
var probeOnce struct {
	sync.Once
	av Availability
}
