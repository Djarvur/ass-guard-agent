// Package provider defines the common Provider interface and the protocol-shape
// adapters (Anthropic-shape primary, OpenAI-shape secondary) that talk to model
// providers and parse tool-call responses into the zcode-normalized
// []ToolCall{Name, Input} shape.
//
// Plan 01-01 T5 delivers the Anthropic adapter at tracer fidelity; Plan 01-04
// adds the OpenAI adapter and round-trip conformance.
//
// Phase 3 (D-04) adds the typed ProviderError + ClassifyHTTP classifier to this
// package: every adapter implementation returns a *ProviderError from Send/
// Stream (the adapter owns HTTP-status classification; the scheduler pattern-
// matches on Kind). Phase 3 defines the type + classifier; the Phase-1 adapters
// call it when they ship.
package provider
