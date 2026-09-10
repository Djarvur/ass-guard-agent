package main

// cli_contract_test.go is the Phase-15 Wave-0 CLI contract pin (plan 15-01 T2,
// RUNT-01 / threat T-15-01). It builds the real ass-guard binary, invokes every
// pinned command surface with --help, and compares the combined output
// byte-for-byte against the inline goldens at the bottom of this file. The
// goldens were transcribed verbatim FROM THE BINARY'S ACTUAL OUTPUT at plan
// time (never synthesized from source); cobra's default help/completion
// entries ride along exactly as rendered, and no hard-coded subcommand count
// is asserted anywhere — the goldens ARE the count. Any command-name, flag,
// default-value, or usage-string drift introduced by the carve fails loudly.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cliContractCase couples one pinned CLI surface (an argv path like
// "checkpoint list") with the golden text its --help must produce
// byte-for-byte.
type cliContractCase struct {
	path   string
	golden string
}

// buildCLIContractBinary builds the ass-guard binary with CGO_ENABLED=0 (the
// mise build task's recipe) into a fresh temp dir. The dir doubles as the
// child process cwd: the cwd-derived flag defaults (--profiles-dir, --suite,
// --cache-pin) embed it, and the capture helper normalizes exactly that path.
//
//nolint:nonamedreturns // the names document the (binary, cwd) pair
func buildCLIContractBinary(t *testing.T) (bin, dir string) {
	t.Helper()

	dir = t.TempDir()
	bin = filepath.Join(dir, "ass-guard")

	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin,
		"github.com/Djarvur/ass-guard-agent/cmd/ass-guard")

	build.Env = append(os.Environ(), "CGO_ENABLED=0")

	var buildErr bytes.Buffer

	build.Stderr = &buildErr

	err := build.Run()
	if err != nil {
		t.Fatalf("go build cmd/ass-guard: %v\n%s", err, buildErr.String())
	}

	return bin, dir
}

// captureCLIContractHelp runs `<bin> path --help` with cwd=dir, asserts
// exit 0, and returns the combined output. The ONLY normalization is the temp
// dir path (and its symlink-resolved form — macOS resolves /var to
// /private/var in the child's Getwd), replaced with <TMP>: it is the sole
// volatile substring, appearing exclusively inside cwd-derived flag defaults.
func captureCLIContractHelp(t *testing.T, bin, dir, path string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, append(strings.Fields(path), "--help")...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s --help: %v\n%s", path, err, out)
	}

	normalized := strings.ReplaceAll(string(out), dir, "<TMP>")

	resolved, rerr := filepath.EvalSymlinks(dir)
	if rerr == nil {
		normalized = strings.ReplaceAll(normalized, resolved, "<TMP>")
	}

	return normalized
}

// runCLIContractCases executes every case against a freshly built binary and
// fails with a golden-vs-actual dump on any drift. The bare group-name paths
// (acp, model-routing, profile) are derived from the cobra constructors' Use
// fields — the test invokes the exact command name the wiring registers.
func runCLIContractCases(t *testing.T, cases []cliContractCase) {
	t.Helper()

	bin, dir := buildCLIContractBinary(t)

	for _, tc := range cases {
		name := tc.path
		if name == "" {
			name = "root" // the bare `ass-guard --help` surface has no argv of its own
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := captureCLIContractHelp(t, bin, dir, tc.path)

			if got != tc.golden {
				t.Errorf("CLI surface drifted from golden:\n--- golden ---\n%s\n--- actual ---\n%s",
					tc.golden, got)
			}
		})
	}
}

// TestCLIBinaryContractRootAndACP pins the root help (the six AddCommand
// subcommand groups in wiring order plus cobra's default help/completion
// entries as rendered), the acp group, and acp serve — the entrypoint Zed
// spawns with --profile --max-concurrent --profiles-dir --work-dir
// --no-engine --ask-timeout (+ the persistent --audit-log).
func TestCLIBinaryContractRootAndACP(t *testing.T) {
	t.Parallel()

	runCLIContractCases(t, []cliContractCase{
		{path: "", golden: goldenRoot},
		{path: newACPCmd().Use, golden: goldenACP},
		{path: "acp serve", golden: goldenACPServe},
	})
}

// TestCLIBinaryContractSupportCommands pins the four CLI-support command
// groups whose run-logic leaves cmd during the carve: checkpoint (list,
// restore), learning (list, revert), model-routing (validate, resolve), and
// parity. A failure here names the group whose cobra shell drifted.
func TestCLIBinaryContractSupportCommands(t *testing.T) {
	t.Parallel()

	runCLIContractCases(t, []cliContractCase{
		{path: "checkpoint", golden: goldenCheckpoint},
		{path: "checkpoint list", golden: goldenCheckpointList},
		{path: "checkpoint restore", golden: goldenCheckpointRestore},
		{path: "learning", golden: goldenLearning},
		{path: "learning list", golden: goldenLearningList},
		{path: "learning revert", golden: goldenLearningRevert},
		{path: newModelRoutingCmd().Use, golden: goldenModelRouting},
		{path: "model-routing validate", golden: goldenModelRoutingValidate},
		{path: "model-routing resolve", golden: goldenModelRoutingResolve},
		{path: "parity", golden: goldenParity},
	})
}

// TestCLIBinaryContractProfile pins the profile group and profile check — the
// drift-detector surface the operator runs after refreshing a capture.
func TestCLIBinaryContractProfile(t *testing.T) {
	t.Parallel()

	runCLIContractCases(t, []cliContractCase{
		{path: newProfileCmd().Use, golden: goldenProfile},
		{path: "profile check", golden: goldenProfileCheck},
	})
}

// The golden blocks below are byte-exact transcriptions of the built binary's
// --help output (see the file comment), regenerated in 18-06 for the
// root-persistent --resume/--continue/-c flags. <TMP> stands for the temp cwd
// that the cwd-derived flag defaults embed at runtime. The //nolint:dupword
// directives are load-bearing: cobra's own "Available Commands" rows repeat
// each command name in the name and Short columns, and that repetition is the
// pinned truth, not an authoring defect.

//nolint:dupword // cobra help rows repeat command names by construction
const goldenRoot = "Loads the named profile (default: zcode), shapes one outgoing request via the Profile Shaper," +
	" sends it to the Anthropic-protocol provider (Z.ai GLM by default), and prints the parsed" +
	" tool-calls as JSON to STDERR. stdout stays byte-clean (reserved for ACP frames). Needs" +
	" ZAI_API_KEY in the environment." +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard [flags]" +
	"\n" +
	"  ass-guard [command]" +
	"\n" +
	"" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  acp           ACP (IDE-native) interface" +
	"\n" +
	"  checkpoint    Shadow-git workspace checkpoints (EARLY-01): list and restore pre-turn snapshots" +
	"\n" +
	"  completion    Generate the autocompletion script for the specified shell" +
	"\n" +
	"  help          Help about any command" +
	"\n" +
	"  learning      Inspect and revert the learned-config store" +
	"\n" +
	"  model-routing Inspect and validate the model scheduling config" +
	"\n" +
	"  parity        run the behavioral mimicry A/B parity gate (MIMC-03, the north-star gate)" +
	"\n" +
	"  profile       profile operations (drift detection, inspection)" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"  -h, --help                        help for ass-guard" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n" +
	"      --version                     print the ass-guard version (build-time-injected) to stderr and exit" +
	"\n" +
	"" +
	"\n" +
	"Use \"ass-guard [command] --help\" for more information about a command." +
	"\n"

const goldenACP = "ACP (IDE-native) interface" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard acp [command]" +
	"\n" +
	"" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  serve       Run the ACP v1 server over stdio (the entrypoint Zed spawns)" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help   help for acp" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n" +
	"" +
	"\n" +
	"Use \"ass-guard acp [command] --help\" for more information about a command." +
	"\n"

const goldenACPServe = "Speaks ACP v1 (newline-delimited JSON-RPC) over stdio. stdout carries ONLY valid ACP" +
	" frames; all diagnostics go to stderr (transport discipline). The lifecycle is initialize" +
	" → session/new → session/prompt with streamed session/update notifications (ACP-04)." +
	" session/load restores a past session and replays it through the same ordered frames" +
	" (ACP-06, 18-01). Needs ZAI_API_KEY for real model turns; the server skeleton works" +
	" without it for the ACP handshake." +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard acp serve [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"      --ask-timeout duration   how long an unanswered AskUserQuestion waits before the turn resumes with" +
	" the non-answer form (D-01); 0 = block forever (interactive mode) (default 10m0s)" +
	"\n" +
	"  -h, --help                   help for serve" +
	"\n" +
	"      --max-concurrent int     max concurrent outbound provider calls across parent + subagents (PARA-04)" +
	" (default 6)" +
	"\n" +
	"      --no-engine              disable the Phase-4 unified engine (fall back to manual continue — D-04)" +
	"\n" +
	"      --profile string         profile name to load (PROF-01) (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string    directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --sandbox string         confine spawned tool processes (Bash-class children) to the workdir rw" +
	" triple with network denied (SAND-01); off|on (default off) (default \"off\")" +
	"\n" +
	"      --work-dir string        working directory for .ass-guard/ transcripts (default: cwd)" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

//nolint:dupword // cobra help rows repeat command names by construction
const goldenCheckpoint = "Operates the per-workspace shadow-git checkpoint store under" +
	" .ass-guard/checkpoints/shadow.git. Every parent turn snapshots the workspace BEFORE its" +
	" mutations; `checkpoint list` shows the workspace's recovery history and `checkpoint" +
	" restore <sessionID-turn-NNN>` returns the workspace to that pre-turn state (the user's" +
	" repository git state is never touched). All output is written to stderr — stdout stays" +
	" reserved for ACP frames." +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard checkpoint [command]" +
	"\n" +
	"" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  list        list checkpoints, oldest first (empty store: \"no checkpoints\", exit 0)" +
	"\n" +
	"  restore     restore the workspace to a checkpoint's pre-turn state" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help   help for checkpoint" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n" +
	"" +
	"\n" +
	"Use \"ass-guard checkpoint [command] --help\" for more information about a command." +
	"\n"

const goldenCheckpointList = "list checkpoints, oldest first (empty store: \"no checkpoints\", exit 0)" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard checkpoint list [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help              help for list" +
	"\n" +
	"      --work-dir string   workspace whose checkpoints to operate on (default: cwd)" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

const goldenCheckpointRestore = "restore the workspace to a checkpoint's pre-turn state" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard checkpoint restore <sessionID-turn-NNN> [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help              help for restore" +
	"\n" +
	"      --work-dir string   workspace whose checkpoints to operate on (default: cwd)" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

//nolint:dupword // cobra help rows repeat command names by construction
const goldenLearning = "Inspect and revert the learned-config store" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard learning [command]" +
	"\n" +
	"" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  list        list every learned entry (id, situation, answer, confidence, status, expiry)" +
	"\n" +
	"  revert      revert one learned entry by id (LRN-04)" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help   help for learning" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n" +
	"" +
	"\n" +
	"Use \"ass-guard learning [command] --help\" for more information about a command." +
	"\n"

const goldenLearningList = "list every learned entry (id, situation, answer, confidence, status, expiry)" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard learning list [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help             help for list" +
	"\n" +
	"      --learned string   path to learned.yaml (default: .ass-guard/learned.yaml)" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

const goldenLearningRevert = "revert one learned entry by id (LRN-04)" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard learning revert <id> [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help             help for revert" +
	"\n" +
	"      --learned string   path to learned.yaml (default: .ass-guard/learned.yaml)" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

//nolint:dupword // cobra help rows repeat command names by construction
const goldenModelRouting = "Inspect and validate the model scheduling config" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard model-routing [command]" +
	"\n" +
	"" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  resolve     resolve a tier to a concrete (provider, model) at a given time" +
	"\n" +
	"  validate    load + validate the model-routing config (D-10 load-time guarantee)" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help   help for model-routing" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n" +
	"" +
	"\n" +
	"Use \"ass-guard model-routing [command] --help\" for more information about a command." +
	"\n"

const goldenModelRoutingValidate = "load + validate the model-routing config (D-10 load-time guarantee)" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard model-routing validate [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"      --config string   path to config.yaml (default: embedded zero-config floor)" +
	"\n" +
	"  -h, --help            help for validate" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

const goldenModelRoutingResolve = "resolve a tier to a concrete (provider, model) at a given time" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard model-routing resolve [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"      --at string        RFC3339 time to resolve at (default: now)" +
	"\n" +
	"      --config string    path to config.yaml (default: embedded zero-config floor)" +
	"\n" +
	"  -h, --help             help for resolve" +
	"\n" +
	"      --json             emit machine-readable JSON to stdout (default: human form to stderr)" +
	"\n" +
	"      --project string   per-project key (default: global)" +
	"\n" +
	"      --tier string      tier to resolve (heavy|good|light) — required" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"

const goldenParity = "run the behavioral mimicry A/B parity gate (MIMC-03, the north-star gate)" +
	"\n" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard parity [flags]" +
	"\n" +
	"  ass-guard parity [command]" +
	"\n" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  nightly-check nightly upstream-parity drift gate: zcode probe + bundle hash vs the pinned capture " +
	"(TAIL-02)" +
	"\n" +
	"\n" +
	"Flags:" +
	"\n" +
	"      --cache-pin string        corpus cache_control placement pin fixture (14-02 JSONL; empty = " +
	"skip the placement check) (default " +
	"\"<TMP>/internal/profile/testdata/context-behavior/cache-control.jsonl\")" +
	"\n" +
	"      --from-rollout string     generate the suite from a zcode rollout JSONL (same-session = " +
	"matching system prompt + tools)" +
	"\n" +
	"  -h, --help                    help for parity" +
	"\n" +
	"      --profile string          profile to load for the ass-guard arm (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string     directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --results string          results JSON output path (default \"parity-results.json\")" +
	"\n" +
	"      --suite string            curated divergence suite JSON (ignored if --from-rollout is set) " +
	"(default \"<TMP>/internal/parity/suite/curated_suite.json\")" +
	"\n" +
	"      --surprise-check string   optional second suite JSON to run after the curated suite passes" +
	"\n" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file " +
	"(LOG-01); empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the " +
	"picker (D-10)" +
	"\n" +
	"\n" +
	"Use \"ass-guard parity [command] --help\" for more information about a command." +
	"\n"

const goldenProfile = "profile operations (drift detection, inspection)" +
	"\n" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard profile [command]" +
	"\n" +
	"\n" +
	"Available Commands:" +
	"\n" +
	"  check         diff a fresh capture against the profile's tiered manifest (PROF-04 drift detector)" +
	"\n" +
	"  nightly-check nightly upstream-parity drift gate: zcode probe + bundle hash vs the pinned capture " +
	"(TAIL-02)" +
	"\n" +
	"\n" +
	"Flags:" +
	"\n" +
	"  -h, --help   help for profile" +
	"\n" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file " +
	"(LOG-01); empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --profiles-dir string         directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the " +
	"picker (D-10)" +
	"\n" +
	"\n" +
	"Use \"ass-guard profile [command] --help\" for more information about a command." +
	"\n"

const goldenProfileCheck = "diff a fresh capture against the profile's tiered manifest (PROF-04 drift detector)" +
	"\n" +
	"" +
	"\n" +
	"Usage:" +
	"\n" +
	"  ass-guard profile check <name> [flags]" +
	"\n" +
	"" +
	"\n" +
	"Flags:" +
	"\n" +
	"      --capture-file string   fixture JSON capture (a model_io line) — bypasses the live path" +
	"\n" +
	"  -h, --help                  help for check" +
	"\n" +
	"      --profiles-dir string   directory containing profile bundles (default \"<TMP>/profiles\")" +
	"\n" +
	"      --zcode-bin string      zcode binary (live path; operator-gated) (default \"zcode\")" +
	"\n" +
	"" +
	"\n" +
	"Global Flags:" +
	"\n" +
	"      --audit-log string            write the redacted verbatim shaped request to this file (LOG-01);" +
	" empty = stderr" +
	"\n" +
	"  -c, --continue                    resume the most recent session in the current directory (D-10)" +
	"\n" +
	"      --profile string              profile name to load (default \"zcode\")" +
	"\n" +
	"      --prompt string               prompt to send through the loop (required for the tracer)" +
	"\n" +
	"      --resume string[=\"@picker\"]   resume a past session by id or title prefix; bare opens the picker" +
	" (D-10)" +
	"\n"
