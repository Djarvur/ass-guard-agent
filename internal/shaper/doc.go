// Package shaper implements the Profile Shaper — the mimicry chokepoint
// (MIMC-01). It populates anthropic-sdk-go native request types from a loaded
// profile and emits per-request identity headers via option.WithHeader (the
// D-09 escape hatch).
//
// The Shaper MUST NOT contain profile-specific code paths (PROF-02): the same
// code shapes any profile. Plan 01-01 T4 delivers tracer fidelity; Plan 01-04
// brings it to full fidelity and enforces PROF-02 structurally.
//
// Phase-1 placeholder.
package shaper
