package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
)

// --- 14-05 (EARLY-05): sessionFor light-tier resolution (Tests 3-4) ---

// lightTierSameProviderConfig binds BOTH heavy and light on the zai provider
// (the same-provider case: the override applies).
const lightTierSameProviderConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
  oai:
    base_url: "https://api.openai.com/v1"
    shape: openai
    api_key: "sk-test-literal"
models:
  glm-5.2:
    provider: zai
  glm-5.2-air:
    provider: zai
  gpt-air:
    provider: oai
tiers:
  heavy:
    model: glm-5.2
    fallback: []
  light:
    model: glm-5.2-air
    fallback: []
`

// lightTierAbsentConfig carries NO tiers.light binding (heavy only): the
// documented default — the subagent keeps the parent model, silently.
const lightTierAbsentConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
models:
  glm-5.2:
    provider: zai
tiers:
  heavy:
    model: glm-5.2
    fallback: []
`

// lightTierCrossProviderConfig binds light on the oai provider while the
// session provider (heavy) is zai — the loud-degrade case: the override is
// skipped, the parent model kept, exactly ONE warning naming both providers.
const lightTierCrossProviderConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
  oai:
    base_url: "https://api.openai.com/v1"
    shape: openai
    api_key: "sk-test-literal"
models:
  glm-5.2:
    provider: zai
  gpt-air:
    provider: oai
tiers:
  heavy:
    model: glm-5.2
    fallback: []
  light:
    model: gpt-air
    fallback: []
`

// tierWiringRunner builds an expansion runner over a workDir carrying the
// fixture config, then loads the scheduling config through the REAL serve seam
// (setupModelRouting — load + heavy-tier provider resolution) and arms the
// runner exactly as runACPServe does, with an injected stderr buffer.
func tierWiringRunner(t *testing.T, fixture string) (*Runner, *bytes.Buffer) {
	t.Helper()

	r, _ := newExpansionRunner(t, false, scriptedResp{text: "ok", finish: stopEndTurn})
	writeTestModelRouting(t, r.workDir, fixture)

	cfg, _, providerName, err := providerfactory.SetupModelRouting(r.workDir, io.Discard)
	require.NoError(t, err, "fixture config must load")

	r.schedCfg = cfg
	r.providerName = providerName

	stderr := &bytes.Buffer{}
	r.stderr = stderr

	return r, stderr
}

// TestSessionFor_ResolvesLightTier (14-05, Test 3) proves the both-ways
// behavior at the wiring seam: a tiers.light binding on the SESSION's provider
// sets Session.SubagentModel to the light model slug; the same fixture WITHOUT
// the binding leaves it empty (parent model) — and neither case warns.
func TestSessionFor_ResolvesLightTier(t *testing.T) { //nolint:paralleltest // HOME pinned via pinEmptyHome — serial
	// pinEmptyHome keeps the global config layer out (t.Setenv ⇒ non-parallel).
	pinEmptyHome(t)

	table := []struct {
		name    string
		fixture string
		want    string
	}{
		{name: "binding routes the light model", fixture: lightTierSameProviderConfig, want: "glm-5.2-air"},
		{name: "absent binding keeps the parent model", fixture: lightTierAbsentConfig, want: ""},
	}

	for _, tc := range table { //nolint:paralleltest // subtests share the pinned HOME — serial
		t.Run(tc.name, func(t *testing.T) {
			r, stderr := tierWiringRunner(t, tc.fixture)

			sess := r.sessionFor(context.Background(), "sess-light-tier")

			require.Equal(t, tc.want, sess.SubagentModel,
				"SubagentModel must be the light-tier slug on a same-provider binding, empty on absence")
			require.Empty(t, stderr.String(),
				"neither a same-provider binding nor an absent one ever warns (absence is the documented default)")
		})
	}
}

// TestSessionFor_LightTierCrossProviderWarns (14-05, Test 4) proves the loud
// degrade: a tiers.light bound to a DIFFERENT provider than the session's
// keeps SubagentModel empty (parent model) and emits EXACTLY ONE warning
// naming both providers — never a silent wrong-wire.
//
//nolint:paralleltest // HOME pinned via pinEmptyHome — serial
func TestSessionFor_LightTierCrossProviderWarns(t *testing.T) {
	pinEmptyHome(t)

	r, stderr := tierWiringRunner(t, lightTierCrossProviderConfig)

	sess := r.sessionFor(context.Background(), "sess-light-cross")

	require.Empty(t, sess.SubagentModel,
		"a cross-provider light binding must NOT set the override (the session provider keeps the parent model)")

	warn := stderr.String()
	require.Equal(t, 1, strings.Count(warn, "tiers.light"),
		"exactly one degrade warning must fire at wiring")
	require.Contains(t, warn, "oai", "the warning names the light binding's provider")
	require.Contains(t, warn, "zai", "the warning names the session provider")
}
