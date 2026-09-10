package toolcat

import (
	"context"
	"encoding/json"
	"fmt"
)

// Mutability classifies a tool's execution side-effect (a Phase-2/4 concern).
// Recorded in Phase 1 even though execution is stubbed.
// String stays value-receiver (fmt.Stringer via %s on struct fields);
// UnmarshalJSON must be pointer-receiver to mutate.
type Mutability int //nolint:recvcheck // mixed receivers are intentional

const (
	// MutabilityReadOnly is the default for tools with no side effects.
	MutabilityReadOnly Mutability = iota
	// MutabilityMutating marks tools that change state (Bash, Write, Edit).
	MutabilityMutating
)

// String returns the catalog's canonical mutability label.
func (m Mutability) String() string {
	switch m {
	case MutabilityMutating:
		return classMutating
	case MutabilityReadOnly:
		return classReadOnly
	default:
		return classReadOnly
	}
}

// UnmarshalJSON accepts the catalog's string form ("mutating"/"read-only") so
// the embedded coretools.json (which stores mutability as a human-readable
// string) deserializes into the typed Mutability enum.
func (m *Mutability) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"mutating"`:
		*m = MutabilityMutating
	case `"read-only"`, `""`, `null`:
		*m = MutabilityReadOnly
	default:
		return fmt.Errorf("toolcat: unknown mutability %s", b) //nolint:err113 // dynamic error message
	}

	return nil
}

// Decl is a model-facing tool declaration (mirrors profile.Decl). The adapter
// returns these to the Shaper — the profile-declared schema is authoritative
// (TOOL-02), not the catalog's.
type Decl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// DefaultToolTimeoutMS is the catalog-wide per-call execution BACKSTOP in
// milliseconds (14-06 / EARLY-06): every tool call dispatched through
// toolexec.DispatchBatch is wrapped in its own context deadline derived from
// the tool's timeout_ms annotation, defaulting to this value when the tool
// carries none (MCP/plugin tools, dynamically-registered commands). It is the
// OUTER bound — a tool's own tighter deadline (Bash's model-provided ms
// timeout, openspec's per-command timeout) always fires first. The value
// matches the Bash schema's declared default ("timeout is in milliseconds:
// default 120000") and the occ census's CLAUDE_CODE_TOOL_TIMEOUT default —
// two independent sources agreeing on 120s as the ecosystem's tool bound.
const DefaultToolTimeoutMS int64 = 120000

// Stub is the tool execution signature (forward-compatible to Phase 2/4).
// Phase-1 catalog entries leave Execute nil; the dispatch skeleton returns a
// canned "stubbed" result when called.
type Stub func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// Tool is one catalog entry: the model-facing schema + the execution behavior.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Mutability  Mutability      `json:"mutability"`
	// TimeoutMS is the ass-guard-side per-call execution backstop in
	// milliseconds (14-06). 0/negative = unset → EffectiveTimeoutMS() returns
	// DefaultToolTimeoutMS. These annotations live ONLY in ass-guard-side
	// metadata (coretools.json) — the captured catalog schema
	// (profiles/zcode/tools.json) is never rewritten (the 08-08 discipline).
	TimeoutMS int64 `json:"timeout_ms"`
	// ConcurrencySafeOpt declares whether the tool may run inside
	// DispatchBatch's parallel read-only pool. nil = undeclared →
	// IsConcurrencySafe() derives from Mutability (read-only ⇒ pool-eligible).
	// A declared false SERIALIZES a read-only tool (alone-in-slot) without
	// touching the D-21 floor — the flag can only REMOVE a tool from the
	// pool, never add a mutating one.
	ConcurrencySafeOpt *bool `json:"concurrency_safe,omitempty"`
	// Destructive marks mutation that is irreversible-by-nature (Bash: an
	// arbitrary shell command can do anything). It is an engine/audit
	// OBSERVATION flag — it does NOT gate engine chaining (the locked
	// unmatched⇒nothing safety model is unchanged; 14-06 routed that
	// decision to the operator).
	Destructive bool `json:"destructive"`
	// Execute is the tool's runtime behavior (nil/stubbed in Phase 1 — D-15).
	Execute Stub `json:"-"`
}

// IsMutating reports whether the tool has side effects.
//
//nolint:gocritic // hugeParam: Tool is a value-semantic catalog entry (the mutability precedent)
func (t Tool) IsMutating() bool { return t.Mutability == MutabilityMutating }

// EffectiveTimeoutMS returns the resolved per-call timeout backstop: the
// declared timeout_ms when set (> 0), else DefaultToolTimeoutMS. (Named
// "Effective" after the EffectiveMutability precedent — a Tool cannot carry
// both a TimeoutMS field and a TimeoutMS method.)
//
//nolint:gocritic // hugeParam: Tool is a value-semantic catalog entry (the mutability precedent)
func (t Tool) EffectiveTimeoutMS() int64 {
	if t.TimeoutMS > 0 {
		return t.TimeoutMS
	}

	return DefaultToolTimeoutMS
}

// IsConcurrencySafe reports whether the tool may join DispatchBatch's parallel
// pool: the declared concurrency_safe value when set, else the mutability
// default (read-only ⇒ safe). Mutating tools stay alone-in-slot regardless of
// any declaration (D-21 floor — T-14-19).
//
//nolint:gocritic // hugeParam: Tool is a value-semantic catalog entry (the mutability precedent)
func (t Tool) IsConcurrencySafe() bool {
	if t.ConcurrencySafeOpt != nil {
		return *t.ConcurrencySafeOpt
	}

	return t.Mutability == MutabilityReadOnly
}

// IsDestructive reports the irreversible-by-nature mutation flag (see the
// Destructive field comment — observation only, no engine gating).
//
//nolint:gocritic // hugeParam: Tool is a value-semantic catalog entry (the mutability precedent)
func (t Tool) IsDestructive() bool { return t.Destructive }
