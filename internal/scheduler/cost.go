package scheduler

import (
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/djarvur/ass-guard-agent/internal/event"
)

// CostCeilingTracker implements CostTracker (D-08): dollars-per-fixed-window
// accumulation from token counts × per-model pricing, with degrade-then-stop on
// breach. First breach → CostDegrade + a warn CostCeilingWarn event (the
// scheduler degrades to the cheaper tier). Second breach (the degraded tier
// also hits the ceiling) → CostHardStop (Dispatch returns KindExhausted). NOT
// hard-stop-on-first-breach — the graceful degradation the ROADMAP goal names.
//
// The fixed calendar-aligned window (MVP) is simpler than a sliding window and
// deterministic for tests via the injected clock; a sliding window is a
// documented v2 (RESEARCH §6.2). On rollover the prior window's spend is frozen
// (logged at Info for accounting) and the budget + degraded flag reset.
type CostCeilingTracker struct {
	cfg     CostCeilingConfig
	pricing map[string]Pricing
	bus     *event.Bus
	log     *slog.Logger

	mu             sync.Mutex
	spent          float64
	degradedSpent  float64
	windowStart    time.Time
	degraded       bool
	degradeWarned  bool
	hardStopWarned bool
}

// NewCostCeilingTracker constructs a tracker over the given ceiling config +
// pricing map. The pricing map (model slug → Pricing) is built by safety.go
// from Config.Models. A nil bus is tolerated (warnings are skipped).
func NewCostCeilingTracker(cfg CostCeilingConfig, pricing map[string]Pricing, bus *event.Bus, log *slog.Logger) *CostCeilingTracker {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultCost.Window
	}
	if pricing == nil {
		pricing = map[string]Pricing{}
	}
	return &CostCeilingTracker{cfg: cfg, pricing: pricing, bus: bus, log: log}
}

// Account records the estimated cost of one completed request (D-08):
// cost = (inTokens×InputPerMToken + outTokens×OutputPerMToken)/1e6 from the
// resolved model's Pricing. When degraded, cost accumulates against the
// degraded-tier budget. A (0,0) call (the Send path, which carries no token
// counts — pitfall 5) is a no-op logged at Warn.
func (c *CostCeilingTracker) Account(model string, inTokens, outTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if inTokens == 0 && outTokens == 0 {
		c.log.Warn("scheduler: cost tracking disabled — no token counts from provider", "model", model)
		return
	}
	p, ok := c.pricing[model]
	if !ok {
		// Unknown model — cannot price; skip silently (the resolver should have
		// rejected dangling slugs at load time, so this is defense-in-depth).
		return
	}
	cost := (float64(inTokens)*p.InputPerMToken + float64(outTokens)*p.OutputPerMToken) / 1e6
	if c.degraded {
		c.degradedSpent += cost
	} else {
		c.spent += cost
	}
}

// Check returns the action for the next request (D-08 degrade-then-stop). The
// ceiling is operator-set in dollars; a zero AmountUSD disables the ceiling
// (always CostAllow) so a config without cost_ceiling never blocks.
func (c *CostCeilingTracker) Check(now time.Time) CostAction {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize / rollover the window.
	if c.windowStart.IsZero() {
		c.windowStart = now.Truncate(c.cfg.Window)
	} else if now.Sub(c.windowStart) >= c.cfg.Window {
		c.log.Info("scheduler: cost window froze", "window_start", c.windowStart, "spent", c.spent, "degraded_spent", c.degradedSpent)
		c.windowStart = now.Truncate(c.cfg.Window)
		c.spent = 0
		c.degradedSpent = 0
		c.degraded = false
		c.degradeWarned = false
		c.hardStopWarned = false
	}

	if c.cfg.AmountUSD <= 0 {
		return CostAllow // ceiling disabled
	}

	if !c.degraded {
		if c.spent >= c.cfg.AmountUSD {
			c.degraded = true
			if !c.degradeWarned {
				c.publish(CostCeilingWarn{
					Window: c.windowLabel(), Spent: c.spent, Ceiling: c.cfg.AmountUSD,
					DegradedTo: c.cfg.DegradeTo, HardStop: false,
				})
				c.degradeWarned = true
			}
			c.log.Warn("scheduler: cost ceiling hit — degrading",
				"spent", c.spent, "ceiling", c.cfg.AmountUSD, "degrade_to", c.cfg.DegradeTo)
			return CostDegrade
		}
		return CostAllow
	}

	// Already degraded: idempotent CostDegrade until the degraded-tier budget
	// also hits the ceiling (then HardStop).
	if c.degradedSpent >= c.cfg.AmountUSD {
		if !c.hardStopWarned {
			c.publish(CostCeilingWarn{
				Window: c.windowLabel(), Spent: c.degradedSpent, Ceiling: c.cfg.AmountUSD,
				HardStop: true,
			})
			c.hardStopWarned = true
		}
		c.log.Warn("scheduler: cost ceiling exhausted (hard stop)",
			"degraded_spent", c.degradedSpent, "ceiling", c.cfg.AmountUSD)
		return CostHardStop
	}
	return CostDegrade
}

// publish emits a warn event to the bus if one is configured (nil bus = warnings
// skipped, for unit tests that don't need the event).
func (c *CostCeilingTracker) publish(e CostCeilingWarn) {
	if c.bus == nil {
		return
	}
	c.bus.Publish(e)
}

// windowLabel formats the current window's [start, end) range for the warn event.
func (c *CostCeilingTracker) windowLabel() string {
	end := c.windowStart.Add(c.cfg.Window)
	return c.cfg.Window.String() + " ending " + end.Format(time.RFC3339)
}

// Spent returns the current window's total spend (primary + degraded tiers) —
// a test/diagnostic accessor (the Account method returns nothing to satisfy the
// CostTracker interface).
func (c *CostCeilingTracker) Spent() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.spent + c.degradedSpent
}

// IsDegraded reports whether the ceiling has been breached (test/diagnostic).
func (c *CostCeilingTracker) IsDegraded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.degraded
}
