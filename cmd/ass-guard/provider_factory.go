package main

import (
	"fmt"
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
//
//nolint:unparam // apiKeyFlag is the D-05 flag-precedence seam (07-CONTEXT D-05), kept per the plan contract
func loadSchedulingFactory(
	workDir, apiKeyFlag string, stderr io.Writer,
) (*scheduler.Config, *scheduler.ProviderFactory, error) {
	overlayPath := filepath.Join(workDir, ".ass-guard", "scheduling.yaml")

	var (
		cfg *scheduler.Config
		err error
	)

	_, statErr := os.Stat(overlayPath)
	if statErr == nil {
		cfg, err = scheduler.Load(overlayPath)
	} else {
		cfg, err = scheduler.Load()
	}

	if err != nil {
		return nil, nil, fmt.Errorf("load scheduling config: %w", err)
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
			return nil, "", fmt.Errorf("load embedded scheduling default: %w", err)
		}

		factory = scheduler.NewProviderFactory(cfg, "", nil)
		factory.WarnUncredentialed(stderr)
	}

	providerName := firstDeclaredProvider(cfg)

	primary, _, rerr := scheduler.NewResolver(cfg).Resolve(tierHeavy, "", time.Now(), scheduler.CapabilityReq{})
	if rerr == nil {
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

// warnLooseConfigPerm implements the SC3 credential-on-disk hygiene warning: a
// scheduling.yaml that is group/world-readable (mode not 0600-tight) may carry
// a literal api_key, so warn once at startup recommending chmod 0600. Advisory
// only — never refuses to start. Transport discipline: stderr only (T-07-06:
// names only the path + the recommended mode, never a key).
func warnLooseConfigPerm(path string, stderr io.Writer) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		_, _ = fmt.Fprintf(stderr,
			"ass-guard: %s is %04o (group/world-accessible) and may carry an api_key — chmod 0600 to protect it\n",
			path, perm)
	}
}
