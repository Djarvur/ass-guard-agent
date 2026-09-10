package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
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

// TestSessionFor_NoLightTierStamp (REVERSED 20-03/D-13) pins the REVERSAL of
// 14-05's session-level stamp: sessionFor NEVER sets SubagentModel anymore —
// a tiers.light binding (same provider or not) and an absent binding BOTH
// leave it empty, because the tier arm moved into the dispatch-time planner
// and fires ONLY when a session has no model at all. Neither case warns at
// wiring (the decision is per-dispatch now).
func TestSessionFor_NoLightTierStamp(t *testing.T) { //nolint:paralleltest // HOME pinned via pinEmptyHome — serial
	// pinEmptyHome keeps the global config layer out (t.Setenv ⇒ non-parallel).
	pinEmptyHome(t)

	table := []struct {
		name    string
		fixture string
	}{
		{name: "same-provider binding does NOT stamp", fixture: lightTierSameProviderConfig},
		{name: "absent binding does NOT stamp", fixture: lightTierAbsentConfig},
		{name: "cross-provider binding does NOT stamp", fixture: lightTierCrossProviderConfig},
	}

	for _, tc := range table { //nolint:paralleltest // subtests share the pinned HOME — serial
		t.Run(tc.name, func(t *testing.T) {
			r, stderr := tierWiringRunner(t, tc.fixture)

			sess := r.sessionFor(context.Background(), "sess-light-tier")

			require.Empty(t, sess.SubagentModel,
				"SubagentModel must stay empty at construction (D-13 reversed the 14-05 default)")
			require.NotNil(t, sess.SubagentModelPlanner,
				"the dispatch-time planner seam must be wired")
			require.Empty(t, stderr.String(),
				"no wiring-time warning — the tier decision is per-dispatch now")
		})
	}
}

// TestDispatchModel_TierArms pins the D-13 tier arms at the planner: a
// session WITH a model keeps the parent (the light binding does NOT apply —
// 14-05's default reversed); a session with NO model at all falls to
// tiers.light on the same provider; a cross-provider light binding on a
// model-less session skips with the one loud warning (the 14-05 Test-4
// coverage, moved to its new dispatch-time home).
//
//nolint:paralleltest,funlen // HOME pinned via pinEmptyHome — serial subtests
func TestDispatchModel_TierArms(t *testing.T) {
	pinEmptyHome(t)

	t.Run("session model present keeps parent despite light binding", func(t *testing.T) {
		r, stderr := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-tier-parent")

		plan := r.planSubagentDispatch(sess, nil, "")

		require.Empty(t, plan.Model, "unset agent + session model => PARENT (D-13 arm 3)")
		require.Nil(t, plan.Provider)
		require.Empty(t, stderr.String(), "no warning — the parent arm is silent")
	})

	t.Run("model-less session falls to tiers.light", func(t *testing.T) {
		r, _ := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-tier-less")
		sess.Profile.Model = "" // the degraded session shape

		plan := r.planSubagentDispatch(sess, nil, "")

		require.Equal(t, "glm-5.2-air", plan.Model,
			"no session model at all => tiers.light (the 14-05 machinery's remaining arm)")
	})

	t.Run("model-less session with cross-provider light binding warns once", func(t *testing.T) {
		r, stderr := tierWiringRunner(t, lightTierCrossProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-tier-cross")
		sess.Profile.Model = ""

		plan := r.planSubagentDispatch(sess, nil, "")

		require.Empty(t, plan.Model,
			"a cross-provider light binding must NOT route (parent-less session keeps NO model)")

		warn := stderr.String()
		require.Equal(t, 1, strings.Count(warn, "tiers.light"),
			"exactly one degrade warning names the skipped binding")
		require.Contains(t, warn, "oai", "the warning names the light binding's provider")
		require.Contains(t, warn, "zai", "the warning names the session provider")
	})
}

// crossRoutingConfig declares a SECOND provider (oai, credentialed) with a
// model the frontmatter can route to — the D-15 ROUTE case.
const crossRoutingConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
  oai:
    base_url: "https://api.openai.com/v1"
    shape: openai
    api_key: "sk-oai-literal"
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

// noCredCrossConfig declares the second provider WITHOUT a credential — the
// D-15 uncredentialed-degrade tripwire.
const noCredCrossConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
  oai:
    base_url: "https://api.openai.com/v1"
    shape: openai
models:
  glm-5.2:
    provider: zai
  gpt-air:
    provider: oai
tiers:
  heavy:
    model: glm-5.2
    fallback: []
`

// TestDispatchModel_Precedence pins D-13/D-14/D-15 at the planner seam: the
// strict frontmatter > dispatch-time > session/parent > tier order, inherit
// normalization, same-provider stamping with NO second provider, cross-
// provider ROUTING with the per-(provider, session) cache, and the
// unknown-slug + uncredentialed one-warning degrades.
//
//nolint:funlen,paralleltest // HOME pinned via pinEmptyHome — serial
//nolint:funlen,paralleltest // HOME pinned — serial subtests
func TestDispatchModel_Precedence(t *testing.T) {
	pinEmptyHome(t)

	frontAgent := func(model string) *ecosys.Agent {
		return &ecosys.Agent{Name: "af", Model: model, Prompt: "be brief"}
	}

	t.Run("frontmatter same-provider stamps, no second provider", func(t *testing.T) {
		r, stderr := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-p1")

		plan := r.planSubagentDispatch(sess, frontAgent("glm-5.2-air"), "")

		require.Equal(t, "glm-5.2-air", plan.Model, "frontmatter wins (D-13 arm 1)")
		require.Nil(t, plan.Provider, "same provider — NO second provider built")
		require.Empty(t, r.subagentProviders, "factory cache untouched (zero constructions)")
		require.Empty(t, stderr.String())
	})

	t.Run("inherit normalizes to unset => parent", func(t *testing.T) {
		r, _ := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-p2")

		plan := r.planSubagentDispatch(sess, frontAgent("inherit"), "")

		require.Empty(t, plan.Model, "model: inherit == unset (D-14) — parent keeps the request")
		require.Nil(t, plan.Provider)
	})

	t.Run("dispatch-time model fills an unset frontmatter", func(t *testing.T) {
		r, _ := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-p3")

		plan := r.planSubagentDispatch(sess, frontAgent(""), "glm-5.2-air")

		require.Equal(t, "glm-5.2-air", plan.Model, "dispatch-time slot (D-13 arm 2)")
	})

	t.Run("unknown slug degrades to parent with ONE warning", func(t *testing.T) {
		r, stderr := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-p4")

		plan := r.planSubagentDispatch(sess, frontAgent("not-a-declared-slug"), "")

		require.Empty(t, plan.Model, "unknown slug => parent model (the dispatch still succeeds)")
		require.Nil(t, plan.Provider)
		require.Contains(t, stderr.String(), "not-a-declared-slug",
			"the warning names the intended model")
		require.EqualValues(t, 1, r.subagentDegrades.Load(), "one counter bump")

		note1 := plan.Note
		require.NotEmpty(t, note1, "the first degrade carries the client-visible note")

		plan2 := r.planSubagentDispatch(sess, frontAgent("not-a-declared-slug"), "")
		require.Empty(t, plan2.Note, "repeated mis-routing: the note dedupes per session (one warning)")
	})

	t.Run("cross-provider ROUTES via the factory cache", func(t *testing.T) {
		r, _ := tierWiringRunner(t, crossRoutingConfig)

		sess := r.sessionFor(context.Background(), "sess-p5")

		plan := r.planSubagentDispatch(sess, frontAgent("gpt-air"), "")

		require.Equal(t, "gpt-air", plan.Model, "the frontmatter slug routes (D-15)")
		require.NotNil(t, plan.Provider, "a second provider instance carries the dispatch")

		first := plan.Provider

		plan2 := r.planSubagentDispatch(sess, frontAgent("gpt-air"), "")
		require.Same(t, first, plan2.Provider, "the per-(provider, session) cache returns ONE instance")
	})

	t.Run("uncredentialed cross-provider degrades to parent", func(t *testing.T) {
		r, stderr := tierWiringRunner(t, noCredCrossConfig)

		sess := r.sessionFor(context.Background(), "sess-p6")

		plan := r.planSubagentDispatch(sess, frontAgent("gpt-air"), "")

		require.Empty(t, plan.Model, "missing credentials => parent model (never a failed turn)")
		require.Nil(t, plan.Provider)
		require.Contains(t, stderr.String(), "gpt-air", "the warning names the intended model")
		require.Contains(t, stderr.String(), "credential", "the warning names the reason")
	})

	t.Run("live note dedupes per session", func(t *testing.T) {
		r, _ := tierWiringRunner(t, lightTierSameProviderConfig)

		sess := r.sessionFor(context.Background(), "sess-p7")

		plan1 := r.planSubagentDispatch(sess, frontAgent("glm-5.2-air"), "")
		plan2 := r.planSubagentDispatch(sess, frontAgent("glm-5.2-air"), "")

		require.NotEmpty(t, plan1.Note, "the first dispatch carries the D-16 live note")
		require.Empty(t, plan2.Note, "subsequent dispatches dedupe (advisoryNoteDue)")
	})
}

// TestDispatchModel_LiveAgentLookup pins the rescan-safe agent registry: an
// agent registered AFTER session construction dispatches through the live
// chain accessor (the SubagentTypes snapshot would have missed it).
//
//nolint:paralleltest // sequenced chain state
func TestDispatchModel_LiveAgentLookup(t *testing.T) {
	r, _ := tierWiringRunner(t, lightTierSameProviderConfig)

	sess := r.sessionFor(context.Background(), "sess-live-lookup")

	// The agent did NOT exist at construction time.
	if _, ok := sess.AgentLookup("late-agent"); ok {
		t.Fatal("late-agent already resolvable before planting")
	}

	reg := r.reg
	reg.Agents["late-agent"] = ecosys.Agent{
		Name: "late-agent", Description: "arrived late", Prompt: "do the late thing",
	}
	r.reg = reg
	r.rebuildCommandChain()

	def, ok := sess.AgentLookup("late-agent")
	require.True(t, ok, "the live chain resolves the post-construction agent")
	require.Equal(t, "do the late thing", def.Prompt, "the discovered Prompt applies")
}
