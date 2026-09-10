// Package profile loads and represents an agent profile: a configurable bundle
// of system prompts, tool catalog, message shape, and identity that ass-guard
// shapes every outgoing model-provider request from.
//
// A profile is selected by name (PROF-01); zcode is the first profile, and the
// architecture supports N profiles from day one. Profile content is extracted
// from the target agent's on-disk logs (grounded, not guessed).
//
// This is a Phase-1 placeholder; the loader, types, coverage manifest, and
// stability validation land in Plans 01-01 T3 and 01-02.
package profile
