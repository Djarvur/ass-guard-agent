package acp

import "sync/atomic"

// Metrics is the D-16 in-process counter family: structured stderr logs carry
// the narrative; these atomic counters give /status (Phase 20) a cheap
// snapshot. No OTel, no new dependencies (D-16 verbatim).
//
// The writer-stall counter is ADOPTED from 16-01: the TurnEmitter keeps its
// fast-path increment (its legacy StallCount accessor still counts), and
// NewServer points the emitter at this family so ONE snapshot reports every
// wire-side episode.
type Metrics struct {
	probeTotal          atomic.Uint64 // elicitation probes issued (one per Call — retries are the same probe)
	probeTimeoutTotal   atomic.Uint64 // D-14 windows exhausted by probes (2 per ladder fallback)
	probeFallbackTotal  atomic.Uint64 // probes that ended degraded (ladder fallback, error response, cancellation)
	registryCancelTotal atomic.Uint64 // cancelled outbound requests (-32800, synthetic cancel, ctx cancel, shutdown)
	writerStallTotal    atomic.Uint64 // D-03 sustained writer-stall episodes (mirrors TurnEmitter.StallCount)
}

// MetricsSnapshot is a point-in-time copy of the counter family (each counter
// read once — presentation-grade consistency for /status).
type MetricsSnapshot struct {
	ProbeTotal          uint64
	ProbeTimeoutTotal   uint64
	ProbeFallbackTotal  uint64
	RegistryCancelTotal uint64
	WriterStallTotal    uint64
}

// Snapshot copies the counter family.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ProbeTotal:          m.probeTotal.Load(),
		ProbeTimeoutTotal:   m.probeTimeoutTotal.Load(),
		ProbeFallbackTotal:  m.probeFallbackTotal.Load(),
		RegistryCancelTotal: m.registryCancelTotal.Load(),
		WriterStallTotal:    m.writerStallTotal.Load(),
	}
}

// noteProbe records one elicitation probe issued. Nil-receiver safe so wiring
// (NewServer) stays the only place that must care whether the family exists.
func (m *Metrics) noteProbe() {
	if m == nil {
		return
	}

	m.probeTotal.Add(1)
}

// noteProbeTimeout records one D-14 window exhausted by a probe (the ladder
// burns exactly two before fallback).
func (m *Metrics) noteProbeTimeout() {
	if m == nil {
		return
	}

	m.probeTimeoutTotal.Add(1)
}

// noteProbeFallback records one probe that ended degraded.
func (m *Metrics) noteProbeFallback() {
	if m == nil {
		return
	}

	m.probeFallbackTotal.Add(1)
}

// noteRegistryCancel records one cancelled outbound request (the Registry's
// onCancel hook target).
func (m *Metrics) noteRegistryCancel() {
	if m == nil {
		return
	}

	m.registryCancelTotal.Add(1)
}

// noteWriterStall records one sustained writer-stall episode (the emitter's
// stall sampler is the only caller).
func (m *Metrics) noteWriterStall() {
	if m == nil {
		return
	}

	m.writerStallTotal.Add(1)
}
