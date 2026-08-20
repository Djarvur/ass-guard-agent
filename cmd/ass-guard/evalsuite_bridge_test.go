package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/evalharness"
	"github.com/Djarvur/ass-guard-agent/internal/evalsuite"
)

// TestEvalSuite_Flagship_Gated (12-08 T1, the gate's first green): the
// evalsuite runner driving the flagship scenario ONCE at k=1 over a fresh
// scratch with the REAL binary + REAL model, through the SAME extracted
// harness the runtime E2E uses. The driver lives here because the runner
// construction is package-main machinery; the suite package owns the
// scenario schema, scoring, and artifacts.
//
// Gates (D-03): ASSGUARD_EVAL_GATE=1 + the standing ASSGUARD_OPENSPEC_BIN=1
// ASSGUARD_E2E_LLM=1 — the mise `eval-gate` task sets all three.
func TestEvalSuite_Flagship_Gated(t *testing.T) { //nolint:paralleltest // gated: real scratch + live model
	evalsuite.GatesEvalGate(t)

	scenarios, err := evalsuite.DefaultScenarios()
	if err != nil {
		t.Fatalf("DefaultScenarios: %v", err)
	}

	// 13-04: the embedded dir now carries the flagship + the expanded-matrix
	// suites; THIS gate stays the flagship's (the 15m mise eval-gate budget);
	// the matrix suites gate via TestEvalSuite_Matrix_Gated.
	flagship := make([]evalsuite.Scenario, 0, 1)

	for _, sc := range scenarios {
		if sc.ID == "opsx-flagship" {
			flagship = append(flagship, sc)
		}
	}

	if len(flagship) != 1 {
		t.Fatalf("embedded suite = %d scenarios; want opsx-flagship present", len(scenarios))
	}

	scenarios = flagship

	res, err := evalsuite.RunSuite(context.Background(), t, scenarios, 1,
		func(_ evalsuite.GateTB, scratchDir string) (evalharness.RunnerSeam, func() error) {
			// The driver runs synchronously inside RunSuite — the captured t
			// is the live test context (construction failures FAIL the run).
			return opsxRunnerSeam{r: newOpsxRunnerAt(t, scratchDir)}, func() error { return nil }
		},
		evalArtifactDir())
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}

	if !res.PassAtK {
		for _, sc := range res.Scenarios {
			for _, rec := range sc.Records {
				for _, f := range rec.Failures {
					t.Errorf("scenario %s: %s", sc.ID, f)
				}
			}
		}

		t.Fatalf("eval gate RED — artifact: %s", res.Artifact)
	}

	t.Logf("eval gate GREEN (k=1) — artifact: %s", res.Artifact)
}

// TestEvalSuite_Flagship_Deep is the manual/local k=3 leg (D-03: NOT part of
// the gate — mise eval-deep).
func TestEvalSuite_Flagship_Deep(t *testing.T) { //nolint:paralleltest // gated deep leg
	evalsuite.GatesEvalGate(t)

	scenarios, err := evalsuite.DefaultScenarios()
	if err != nil {
		t.Fatalf("DefaultScenarios: %v", err)
	}

	res, err := evalsuite.RunSuite(context.Background(), t, scenarios, 3,
		func(_ evalsuite.GateTB, scratchDir string) (evalharness.RunnerSeam, func() error) {
			return opsxRunnerSeam{r: newOpsxRunnerAt(t, scratchDir)}, func() error { return nil }
		},
		evalArtifactDir())
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}

	t.Logf("eval deep (k=3) pass_at_k=%v — artifact: %s", res.PassAtK, res.Artifact)
}

// evalArtifactDir is the run-artifact family under the repo's
// .ass-guard/eval/ (gitignored with the whole .ass-guard family).
func evalArtifactDir() string {
	repo, err := os.Getwd()
	if err != nil {
		return filepath.Join(os.TempDir(), "ass-guard-eval")
	}

	for range 16 {
		_, serr := os.Stat(filepath.Join(repo, "profiles"))
		if serr == nil {
			return filepath.Join(repo, ".ass-guard", "eval")
		}

		repo = filepath.Dir(repo)
	}

	return filepath.Join(os.TempDir(), "ass-guard-eval")
}

// TestEvalSuite_Matrix_Gated (13-04): the expanded-matrix per-command suites
// (happy + fixable, D-04) behind the same gate surface — each scenario runs
// at k=1 over a fresh expanded-profile scratch with its fixture seeded;
// failure attribution is the scenario id. Run on demand (the mise eval-gate's
// 15m budget stays the flagship's).
func TestEvalSuite_Matrix_Gated(t *testing.T) { //nolint:paralleltest // gated: real scratches + live model
	evalsuite.GatesEvalGate(t)

	all, err := evalsuite.DefaultScenarios()
	if err != nil {
		t.Fatalf("DefaultScenarios: %v", err)
	}

	var pick string

	if id := os.Getenv("MATRIX_SUITE_ID"); id != "" {
		pick = id
	}

	scenarios := make([]evalsuite.Scenario, 0, len(all))

	for _, sc := range all {
		if sc.ID == "opsx-flagship" {
			continue
		}

		if pick != "" && sc.ID != pick {
			continue
		}

		scenarios = append(scenarios, sc)
	}

	if len(scenarios) == 0 {
		t.Fatalf("no matrix scenarios selected (pick=%q, all=%d)", pick, len(all))
	}

	res, err := evalsuite.RunSuite(context.Background(), t, scenarios, 1,
		func(_ evalsuite.GateTB, scratchDir string) (evalharness.RunnerSeam, func() error) {
			return opsxRunnerSeam{r: newOpsxRunnerAt(t, scratchDir)}, func() error { return nil }
		},
		evalArtifactDir())
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}

	if !res.PassAtK {
		for _, sc := range res.Scenarios {
			for _, rec := range sc.Records {
				if len(rec.Failures) > 0 {
					t.Errorf("scenario %s FAILED: %v", sc.ID, rec.Failures)
				}
			}
		}

		t.Errorf("matrix eval gate RED (k=1) — artifact: %s", res.Artifact)

		return
	}

	t.Logf("matrix eval gate GREEN (k=1, %d scenarios) — artifact: %s", len(scenarios), res.Artifact)
}
