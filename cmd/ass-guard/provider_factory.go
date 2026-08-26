package main

import (
	"io"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
)

// Transitional delegation bridge (Phase 15 plan 15-02, D-08/D-09): the
// provider-factory infrastructure now lives in internal/providerfactory; these
// same-name cmd-local wrappers keep the not-yet-relocated call sites
// (acp_serve.go's serve pipeline, parity.go's A/B arm, and the runner-adjacent
// tests) compiling untouched. Plan 15-04 re-points parity.go and deletes
// setupProviderFactory; plan 15-05 re-points acp_serve.go and deletes the rest.
func setupProviderFactory(workDir string, stderr io.Writer) (*modelrouting.ProviderFactory, string, error) {
	return providerfactory.SetupProviderFactory(workDir, stderr) //nolint:wrapcheck // thin delegation
}

// setupModelRouting delegates to providerfactory.SetupModelRouting — see the
// bridge note above.
func setupModelRouting(
	workDir string, stderr io.Writer,
) (*modelrouting.Config, *modelrouting.ProviderFactory, string, error) {
	return providerfactory.SetupModelRouting(workDir, stderr) //nolint:wrapcheck // thin delegation
}

// globalConfigPath delegates to providerfactory.GlobalConfigPath — see the
// bridge note above.
func globalConfigPath() (string, error) {
	return providerfactory.GlobalConfigPath() //nolint:wrapcheck // thin delegation
}

// projectConfigPath delegates to providerfactory.ProjectConfigPath — see the
// bridge note above.
func projectConfigPath(workDir string) string {
	return providerfactory.ProjectConfigPath(workDir)
}

// warnLooseConfigPerm delegates to providerfactory.WarnLooseConfigPerm — see
// the bridge note above.
func warnLooseConfigPerm(path string, stderr io.Writer) {
	providerfactory.WarnLooseConfigPerm(path, stderr)
}
