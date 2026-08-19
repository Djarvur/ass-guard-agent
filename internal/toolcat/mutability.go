package toolcat

// This file implements the mutability-declaration interface (D-19, SESS-02/03):
//
//   - EffectiveMutability applies the SESS-03 "more-mutating wins" formula. A
//     tool is a boundary candidate if EITHER its declared catalog Mutability OR
//     the adapter-class argument is Mutating. The formula is the single source
//     of truth Phase 4's OpenSpec adapter plugs into (D-19 forward-design) — it
//     takes a tool + an adapter class, with no OpenSpec-specific branch.
//   - IsBoundary applies the SESS-02 structural floor: a mutating tool is
//     ALWAYS a boundary, and config may only ADD boundaries (never downgrade a
//     declared-mutating tool to read-only). There is no API path here that
//     flips a catalog-mutating tool off; the formula does not consult config for
//     downgrading.

import "slices"

// EffectiveMutability returns the effective mutability of a tool given an
// adapter-class hint (SESS-03). The "more-mutating wins" rule: if either the
// tool's declared Mutability or the adapter class is Mutating, the result is
// Mutating; otherwise ReadOnly. The adapter class lets Phase 4's OpenSpec
// adapter (or a per-command override) escalate a read-only catalog tool to a
// boundary without editing the catalog.
//
//nolint:gocritic // hugeParam: Tool is a value-semantic catalog entry (pre-existing signature)
func EffectiveMutability(tool Tool, adapterClass Mutability) Mutability {
	if tool.Mutability == MutabilityMutating || adapterClass == MutabilityMutating {
		return MutabilityMutating
	}

	return MutabilityReadOnly
}

// IsBoundary reports whether a tool invocation is a session boundary
// (SESS-02/03). It is true when the tool's EffectiveMutability (catalog field,
// with a read-only adapter class — the common case) is Mutating OR the tool
// name appears in configAdded. configAdded is the additive-only surface: it can
// declare ADDITIONAL boundaries (e.g. WebFetch for a project that wants it
// treated as mutating) but cannot remove a catalog-mutating boundary, because
// the catalog field is consulted independently (structural floor).
//
// An unknown tool (not in the catalog) is a boundary only if config-added.
func IsBoundary(toolName string, catalog *Catalog, configAdded []string) bool {
	if catalog != nil {
		if tool, ok := catalog.Get(toolName); ok {
			if EffectiveMutability(tool, MutabilityReadOnly) == MutabilityMutating {
				return true
			}
		}
	}

	return slices.Contains(configAdded, toolName)
}
