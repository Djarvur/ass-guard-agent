// Package provider defines the common Provider interface and the protocol-shape
// adapters (Anthropic-shape primary, OpenAI-shape secondary) that talk to model
// providers and parse tool-call responses into the zcode-normalized
// []ToolCall{Name, Input} shape.
//
// Plan 01-01 T5 delivers the Anthropic adapter at tracer fidelity; Plan 01-04
// adds the OpenAI adapter and round-trip conformance.
//
// Phase-1 placeholder.
package provider
