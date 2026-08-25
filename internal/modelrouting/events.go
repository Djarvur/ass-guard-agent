package modelrouting

import (
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// ProviderFallback is emitted when a transient failure causes the scheduler to
// walk from one candidate to the next (D-06). Info-level → the Phase-2 ACP
// adapter forwards it as a session/update notification: "provider X failed
// (429), falling back to provider Y." The developer sees the degradation but
// is not blocked. The event is pure data — the ACP adapter (not this type)
// renders the session/update frame (C1 transport discipline).
type ProviderFallback struct {
	TurnID       string
	FromProvider string
	FromModel    string
	ToProvider   string
	ToModel      string
	Reason       string             // "Transient: HTTP 429", "circuit open", etc.
	ErrorKind    provider.ErrorKind // the kind that triggered the walk
	Attempt      int                // 1-based index into the fallback walk
}

// Kind returns the event discriminator (event.Event interface).
func (ProviderFallback) Kind() string { return "ProviderFallback" }

// CostCeilingWarn is emitted when the cost ceiling is breached (D-08). Warn-
// level → ACP session/update. HardStop is false on the first breach (the
// scheduler degrades to the cheaper tier) and true on the second (the degraded
// tier also hit its ceiling — the turn fails with KindExhausted).
type CostCeilingWarn struct {
	TurnID     string
	Window     string  // "24h ending 2026-08-09T00:00:00Z"
	Spent      float64 // USD spent this window
	Ceiling    float64 // the breached amount_usd
	DegradedTo string  // tier slug ("" on the hard-stop warning)
	HardStop   bool    // true when the degraded tier also hit its ceiling
}

// Kind returns the event discriminator (event.Event interface).
func (CostCeilingWarn) Kind() string { return "CostCeilingWarn" }

// compile-time interface checks.
var (
	_ event.Event = ProviderFallback{}
	_ event.Event = CostCeilingWarn{}
)
