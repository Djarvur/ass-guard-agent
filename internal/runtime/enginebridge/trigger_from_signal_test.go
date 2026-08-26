package enginebridge //nolint:testpackage // internal package test

import "testing"

// TestStageVocab_TriggerFromSignal (08-06 Test 5): stage-bearing pattern ids
// map to their hook stages; un-staged signals keep the post-implement default.
func TestStageVocab_TriggerFromSignal(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"text:post-explore-handoff": stagePostExplore,
		"text:post-propose-handoff": stagePostPropose,
		"text:post-apply-handoff":   "post-apply",
		"text:post-archive-handoff": "post-archive",
		"hook:post-propose-handoff": stagePostPropose,
		// Hybrid chaining (findings-6): the provenance rows carry the same
		// stage-bearing ids behind the "command:" signal prefix.
		"command:post-explore-handoff": stagePostExplore,
		"command:post-propose-handoff": stagePostPropose,
		"text:impl-complete":           "post-implement",
		"text:changes-proposed":        "post-phase",
		"text:something-unstaged":      "post-implement",
	}

	for signal, want := range cases {
		if got := triggerFromSignal(signal); got != want {
			t.Errorf("triggerFromSignal(%q) = %q; want %q", signal, got, want)
		}
	}
}
