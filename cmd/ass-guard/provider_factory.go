package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/scheduler"
)

// loadSchedulingFactory is the D-08 wiring seam shared by acp serve / tracer /
// parity: it loads the operator's .ass-guard/scheduling.yaml (when present)
// overlaid on the embedded default, builds the scheduler.ProviderFactory, and
// emits the D-07 startup uncredentialed-provider warnings to stderr (transport
// discipline — stdout stays ACP-only). A load error is returned for the caller
// to degrade gracefully (T-07-08); the factory never refuses to build.
func loadSchedulingFactory(workDir, apiKeyFlag string, stderr io.Writer) (*scheduler.Config, *scheduler.ProviderFactory, error) {
	overlayPath := filepath.Join(workDir, ".ass-guard", "scheduling.yaml")

	var (
		cfg *scheduler.Config
		err error
	)

	if _, statErr := os.Stat(overlayPath); statErr == nil {
		cfg, err = scheduler.Load(overlayPath)
	} else {
		cfg, err = scheduler.Load()
	}

	if err != nil {
		return nil, nil, err
	}

	factory := scheduler.NewProviderFactory(cfg, apiKeyFlag, slog.Default())
	factory.WarnUncredentialed(stderr)

	return cfg, factory, nil
}

// setupProviderFactory builds the runtime factory + the provider to construct
// for the heavy tier (D-08): it resolves the tier via the scheduler resolver
// and defaults to the first declared provider (sorted) on resolve error. A
// failed operator config degrades to the embedded default + a stderr log,
// mirroring the engine/learning-store degradation pattern (T-07-08); only a
// failure of the embedded default itself is returned.
func setupProviderFactory(workDir string, stderr io.Writer) (*scheduler.ProviderFactory, string, error) {
	cfg, factory, err := loadSchedulingFactory(workDir, "", stderr)
	if err != nil {
		log.Printf("ass-guard: scheduling config load failed (continuing with embedded default): %v", err)

		cfg, err = scheduler.Load()
		if err != nil {
			return nil, "", err
		}

		factory = scheduler.NewProviderFactory(cfg, "", nil)
		factory.WarnUncredentialed(stderr)
	}

	providerName := firstDeclaredProvider(cfg)
	if primary, _, rerr := scheduler.NewResolver(cfg).Resolve(
		tierHeavy, "", time.Now(), scheduler.CapabilityReq{},
	); rerr == nil {
		providerName = primary.Provider
	}

	return factory, providerName, nil
}

// firstDeclaredProvider returns the first provider name in sorted order — the
// resolve-error fallback so a factory-wired session always has a provider.
func firstDeclaredProvider(cfg *scheduler.Config) string {
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}

	sort.Strings(names)

	if len(names) == 0 {
		return ""
	}

	return names[0]
}
