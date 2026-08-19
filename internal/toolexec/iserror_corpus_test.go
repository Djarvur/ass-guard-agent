package toolexec_test

// 14-06 (EARLY-06) contract-inventory evidence tests.
//
// Two evidence bases live here:
//
//  1. The is_error corpus scan — mechanically scans zcode rollout sessions
//     (the capture authority) for tool-result messages carrying isError=true,
//     counting per tool name and classifying the distinct error FORM classes
//     (shapes only, payloads stripped). TestIsErrorCorpusFixture proves the
//     parse logic in CI against a committed corpus-shaped fixture;
//     TestIsErrorCorpusLive scans the newest rollouts on disk and SKIP-LOUDS
//     when no corpus is present (the scan is evidence collection, not a
//     pass/fail gate — zero isError findings is a legitimate corpus state).
//     The findings are quoted with provenance in docs/tool-contract-inventory.md.
//
//  2. The occ surface census (INPUT ONLY — the 2026-08-19 SEED-004
//     disposition): the committed fixture testdata/occ-census.json pins the
//     25 tool names, the CLAUDE_CODE_* env-var names, the 6 permission-mode
//     names, and the 4 MCP transports extracted from the pinned shallow clone
//     of ruvnet/open-claude-code. TestOccCensusFixture shape-pins the
//     fixture; TestOccCensusCrossCheck asserts every census tool name is
//     ACCOUNTED FOR against the shipped catalog (in-catalog or
//     documented-absent — the inventory doc carries the rationale table);
//     TestOccCensusDeterministic re-extracts from the clone at the pinned
//     commit and asserts the identical census (idempotency edge pinned).

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// --- 1. the is_error corpus scan ---

// rolloutToolMsg is the runtime-normalized tool-result message shape inside
// zcode rollout records: request.messages[].{role,content,toolCallId,
// toolName,isError} (camelCase on disk; the isError boolean is the corpus's
// is_error ground truth — the shaper renders it onto the wire's
// tool_result.is_error). Content is RawMessage because NON-tool messages in
// the same records (assistant) carry array-shaped content blocks — a strict
// string field would reject the whole line and lose its tool messages too.
type rolloutToolMsg struct {
	Role     string          `json:"role"`
	Content  json.RawMessage `json:"content"`
	ToolName string          `json:"toolName"` //nolint:tagliatelle // the rollout's on-disk camelCase format
	IsError  bool            `json:"isError"`  //nolint:tagliatelle // the rollout's on-disk camelCase format
}

// textContent best-effort decodes a message content as the tool-result plain
// string (the captured form); a non-string shape returns the raw JSON text.
func textContent(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	return string(raw)
}

type rolloutRecord struct {
	Request struct {
		Messages []rolloutToolMsg `json:"messages"`
	} `json:"request"`
}

// isErrorScan aggregates one session's isError evidence: total tool-result
// messages, the isError subset counted per tool, and the distinct error FORM
// classes (first line, digits normalized — payload-free shapes).
type isErrorScan struct {
	ToolMessages int
	IsError      int
	PerTool      map[string]int
	Forms        map[string]int
}

// formShape normalizes an error content into its shape class: first line,
// digits collapsed to N, truncated to 60 runes. Payloads (paths, globs,
// strings) beyond the prefix never enter the class — the redaction discipline.
func formShape(content string) string {
	first := content
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}

	var sb strings.Builder

	inDigits := false

	for _, r := range first {
		isDigit := r >= '0' && r <= '9'
		handled := handleFormRune(&sb, &inDigits, r, isDigit)

		if !handled {
			continue
		}

		if sb.Len() >= 60 {
			break
		}
	}

	return sb.String()
}

// handleFormRune folds one rune into the shape: digit runs collapse to a
// single N (handled=true); non-digits append verbatim and reset the run.
func handleFormRune(sb *strings.Builder, inDigits *bool, r rune, isDigit bool) bool {
	switch {
	case isDigit && *inDigits:
		return false // collapse the running digit run into the N already written
	case isDigit:
		sb.WriteByte('N')

		*inDigits = true
	default:
		sb.WriteRune(r)

		*inDigits = false
	}

	return true
}

// scanRolloutIsError parses one model-io-sess_*.jsonl file for isError
// tool-result messages. Unparseable lines are skipped (the rollout writer can
// emit partial lines mid-session; the same tolerance profile.ScanRolloutDir
// applies).
func scanRolloutIsError(path string) (isErrorScan, error) {
	out := isErrorScan{PerTool: map[string]int{}, Forms: map[string]int{}}

	data, err := os.ReadFile(path)
	if err != nil {
		return out, err //nolint:wrapcheck // test evidence scan
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, `"role"`) {
			continue
		}

		var rec rolloutRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}

		for _, m := range rec.Request.Messages {
			if m.Role != "tool" {
				continue
			}

			out.ToolMessages++

			if !m.IsError {
				continue
			}

			out.IsError++
			out.PerTool[m.ToolName]++
			out.Forms[m.ToolName+" | "+formShape(textContent(m.Content))]++
		}
	}

	return out, nil
}

// TestIsErrorCorpusFixture proves the scan's parse logic in CI against the
// committed corpus-shaped fixture testdata/iserror-rollout-fixture.jsonl: the
// exact error FORM classes the live 2026-08-19 corpus scan observed (Bash
// `Exit code <N>`, Read missing-file, Edit wrapped modified-since-read +
// not-read-yet, Write bare modified-since-read), per-tool counts and form
// classes — including the mixed-content record (an assistant array-content
// block beside a tool message must not reject the line).
func TestIsErrorCorpusFixture(t *testing.T) {
	t.Parallel()

	scan, err := scanRolloutIsError(filepath.Join("testdata", "iserror-rollout-fixture.jsonl"))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if scan.ToolMessages != 7 {
		t.Errorf("ToolMessages = %d; want 7", scan.ToolMessages)
	}

	if scan.IsError != 6 {
		t.Errorf("IsError = %d; want 6", scan.IsError)
	}

	wantPerTool := map[string]int{toolBash: 2, toolRead: 1, toolEdit: 2, toolWrite: 1}
	for tool, n := range wantPerTool {
		if scan.PerTool[tool] != n {
			t.Errorf("PerTool[%s] = %d; want %d", tool, scan.PerTool[tool], n)
		}
	}

	// The Bash exit-code form class collapses digits (1 and 137 → one shape).
	if scan.Forms[toolBash+" | Exit code N"] != 2 {
		t.Errorf("Forms[Bash | Exit code N] = %d; want 2 (digit normalization)", scan.Forms[toolBash+" | Exit code N"])
	}

	if scan.Forms[toolRead+" | File does not exist. Note: your current working directory is"] != 1 {
		t.Errorf("Read missing-file form class missing: %v", scan.Forms)
	}

	// Every isError finding carries a tool name (the per-tool table's key).
	for form := range scan.Forms {
		if !strings.Contains(form, " | ") {
			t.Errorf("form class %q lacks the '<tool> | <shape>' key form", form)
		}
	}
}

// TestIsErrorCorpusLive scans the NEWEST zcode rollouts on disk (main first —
// profile.ScanRolloutDir's sort) and logs the isError evidence table. Skip-loud
// when no corpus is present; zero isError findings is a legitimate outcome
// (recorded, not failed) — the scan feeds the inventory doc's provenance.
func TestIsErrorCorpusLive(t *testing.T) { //nolint:paralleltest // reads the live shared rollout dir
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir (live is_error scan skipped loudly): %v", err)
	}

	dir := filepath.Join(home, ".zcode", "cli", "rollout")

	stats, err := profile.ScanRolloutDir(dir)
	if err != nil || len(stats) == 0 {
		t.Skipf("no zcode rollout corpus on disk (live is_error scan skipped loudly): %v", err)
	}

	agg, sessions := aggregateNewestScans(t, stats)

	t.Logf("aggregate over %d sessions: %d tool messages, %d isError", sessions, agg.ToolMessages, agg.IsError)
	logEvidenceTable(t, agg)

	// Structural invariant only: every isError finding has a tool name.
	for tool := range agg.PerTool {
		if tool == "" {
			t.Error("isError finding with empty toolName — per-tool table key broken")
		}
	}
}

// aggregateNewestScans scans up to three of the newest sessions and merges
// their isError evidence (newest three suffice as evidence).
//
//nolint:gocritic // unnamedResult conflicts w/ nonamedreturns
func aggregateNewestScans(t *testing.T, stats []profile.SessionStat) (isErrorScan, int) {
	t.Helper()

	agg := isErrorScan{PerTool: map[string]int{}, Forms: map[string]int{}}
	sessions := 0

	for _, st := range stats {
		if sessions >= 3 {
			break
		}

		scan, serr := scanRolloutIsError(st.Path)
		if serr != nil {
			continue
		}

		sessions++
		agg.ToolMessages += scan.ToolMessages
		agg.IsError += scan.IsError

		for k, v := range scan.PerTool {
			agg.PerTool[k] += v
		}

		for k, v := range scan.Forms {
			agg.Forms[k] += v
		}

		t.Logf("session %s (role=%s): %d tool messages, %d isError, per-tool %v",
			st.ID, st.Role, scan.ToolMessages, scan.IsError, scan.PerTool)
	}

	return agg, sessions
}

// logEvidenceTable prints the per-tool counts and form classes, sorted — the
// table docs/tool-contract-inventory.md quotes with provenance.
func logEvidenceTable(t *testing.T, agg isErrorScan) {
	t.Helper()

	tools := sortedKeysCounted(agg.PerTool)
	for _, tool := range tools {
		t.Logf("isError[%s] = %d", tool, agg.PerTool[tool])
	}

	forms := sortedKeysCounted(agg.Forms)
	for _, form := range forms {
		t.Logf("form %dx %q", agg.Forms[form], form)
	}
}

// sortedKeysCounted returns a counting map's keys, sorted.
func sortedKeysCounted(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// --- 2. the occ surface census (INPUT ONLY) ---

type occCensus struct {
	Provenance struct {
		Repo        string   `json:"repo"`
		Commit      string   `json:"commit"`
		ExtractedAt string   `json:"extracted_at"`
		InputFiles  []string `json:"input_files"`
	} `json:"_provenance"` //nolint:tagliatelle // the fixture's committed key
	Tools []string `json:"tools"`
	Env   struct {
		Names        []string `json:"names"`
		Total        int      `json:"total"`
		WithDefaults int      `json:"with_defaults"`
	} `json:"env"`
	PermissionModes []string `json:"permission_modes"`
	MCPTransports   []string `json:"mcp_transports"`
}

func loadOccCensus(t *testing.T) occCensus {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "occ-census.json"))
	if err != nil {
		t.Fatalf("read occ-census.json: %v", err)
	}

	var c occCensus

	err = json.Unmarshal(data, &c)
	if err != nil {
		t.Fatalf("parse occ-census.json: %v", err)
	}

	return c
}

// TestOccCensusFixture shape-pins the committed census: 25 tool names, the
// env-var table with its defaults count, 6 permission-mode names, 4 MCP
// transports, and the provenance fields (repo/commit/extracted_at).
func TestOccCensusFixture(t *testing.T) {
	t.Parallel()

	c := loadOccCensus(t)

	if len(c.Tools) != 25 {
		t.Errorf("census tools = %d; want 25 (%v)", len(c.Tools), c.Tools)
	}

	if c.Env.Total != len(c.Env.Names) {
		t.Errorf("env total = %d; want %d (== len(names))", c.Env.Total, len(c.Env.Names))
	}

	if len(c.Env.Names) < 60 {
		t.Errorf("env names = %d; want >= 60 (the audit's ~100-entry table's CLAUDE_CODE_* subset)", len(c.Env.Names))
	}

	if c.Env.WithDefaults <= 0 || c.Env.WithDefaults > c.Env.Total {
		t.Errorf("env with_defaults = %d; want in (0, total]", c.Env.WithDefaults)
	}

	if len(c.PermissionModes) != 6 {
		t.Errorf("permission modes = %d; want 6 (%v)", len(c.PermissionModes), c.PermissionModes)
	}

	if len(c.MCPTransports) != 4 {
		t.Errorf("mcp transports = %d; want 4 (%v)", len(c.MCPTransports), c.MCPTransports)
	}

	if c.Provenance.Repo == "" || c.Provenance.Commit == "" || c.Provenance.ExtractedAt == "" {
		t.Errorf("provenance incomplete: %+v", c.Provenance)
	}
}

// absentNotCaptured is the shared documented-absent rationale (goconst).
const absentNotCaptured = "not in the captured zcode catalog"

// occDocumentedAbsent returns the census tool-name set NOT in ass-guard's
// shipped 19-coretool catalog, each with its rationale row in
// docs/tool-contract-inventory.md's cross-check table.
func occDocumentedAbsent() map[string]string {
	return map[string]string{
		"AskUser":         "occ name variant — the captured zcode catalog ships AskUserQuestion",
		"EnterWorktree":   absentNotCaptured + " (worktree tools absent from the pin)",
		"ExitWorktree":    absentNotCaptured,
		"Glob":            "not in the captured zcode 79-tool catalog (STATE 08-08 finding)",
		"Grep":            "not in the captured zcode 79-tool catalog (STATE 08-08 finding)",
		"LS":              absentNotCaptured,
		"LSP":             "occ explicit stub; " + absentNotCaptured,
		"MultiEdit":       absentNotCaptured,
		"NotebookEdit":    absentNotCaptured,
		"ReadMcpResource": "MCP-adjacent builtin; ass-guard surfaces MCP via the dynamic mcp__ namespace instead",
		"RemoteTrigger":   "occ-specific; " + absentNotCaptured,
		"ToolSearch":      "occ-specific; " + absentNotCaptured,
	}
}

// TestOccCensusCrossCheck asserts the three-way accounting: EVERY census tool
// name is either in the shipped catalog, mcp__-shaped (none are — the dynamic
// namespace note), or documented-absent with a rationale. The full rationale
// table lives in docs/tool-contract-inventory.md (INPUT ONLY — no census
// content becomes catalog/profile/config behavior).
func TestOccCensusCrossCheck(t *testing.T) {
	t.Parallel()

	c := loadOccCensus(t)
	catalog := toolcat.NewCatalog()

	if len(c.Tools) == 0 {
		t.Fatal("census fixture empty — cross-check vacuous")
	}

	absent := occDocumentedAbsent()

	for _, name := range c.Tools {
		if strings.HasPrefix(name, "mcp__") {
			continue // the MCP-shaped bucket (empty at this pin — dynamic namespace)
		}

		if _, ok := catalog.Get(name); ok {
			continue // in shipped catalog
		}

		if _, ok := absent[name]; !ok {
			t.Errorf("census tool %q is neither in the shipped catalog nor documented-absent — accounting hole", name)
		}
	}

	// And the reverse direction: every documented-absent entry actually appears
	// in the census (the map stays honest).
	inCensus := map[string]bool{}
	for _, name := range c.Tools {
		inCensus[name] = true
	}

	for name := range absent {
		if !inCensus[name] {
			t.Errorf("documented-absent entry %q is not in the census fixture — stale map row", name)
		}
	}
}

// occ extraction patterns (the same regexes the fixture was produced with;
// RE2-compatible by construction). Compiled once — immutable by construction.
var (
	occImportRE  = regexp.MustCompile(`import \{ \w+ \} from '\./([a-z-]+\.mjs)';`)
	occNameRE    = regexp.MustCompile("(?m)^ {4}name: '([^']+)'")
	occEnvRE     = regexp.MustCompile(`(?m)^ {4}([A-Z0-9_]+): \{([^}]*)\}`)
	occCaseRE    = regexp.MustCompile(`case '([a-zA-Z]+)':`)
	occClientRE  = regexp.MustCompile(`case '([a-z-]+)': *return this\._connect`)
	occDefaultRE = regexp.MustCompile(`default:`)
)

// extractOccCensus re-derives the census from a clone tree at the pinned
// commit — the determinism input. Same order of operations as the original
// extraction: registry imports → per-file tool names; env table entries;
// checker.mjs mode cases; client.mjs transport cases.
func extractOccCensus(t *testing.T, root string) occCensus {
	t.Helper()

	var c occCensus

	c.Tools = extractOccTools(t, root)
	c.Env.Names, c.Env.WithDefaults = extractOccEnv(t, root)
	c.Env.Total = len(c.Env.Names)
	c.PermissionModes = extractOccModes(t, root)
	c.MCPTransports = extractOccTransports(t, root)

	return c
}

// occSrc reads one clone source file under v2/src/<dirs...>/<file>.
func occSrc(t *testing.T, root string, elems ...string) string {
	t.Helper()

	all := append([]string{root, "v2", "src"}, elems...)

	data, err := os.ReadFile(filepath.Join(all...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(elems...), err)
	}

	return string(data)
}

// extractOccTools walks registry.mjs's imports and takes each tool file's
// declared name.
func extractOccTools(t *testing.T, root string) []string {
	t.Helper()

	reg := occSrc(t, root, "tools", "registry.mjs")

	seen := map[string]bool{}

	for _, m := range occImportRE.FindAllStringSubmatch(reg, -1) {
		src := occSrc(t, root, "tools", m[1])

		if nm := occNameRE.FindStringSubmatch(src); nm != nil {
			seen[nm[1]] = true
		}
	}

	return sortedKeys(seen)
}

// extractOccEnv returns the CLAUDE_CODE_* env names + how many carry defaults.
//
//nolint:gocritic // unnamedResult conflicts w/ nonamedreturns
func extractOccEnv(t *testing.T, root string) ([]string, int) {
	t.Helper()

	envSeen := map[string]bool{}
	withDefaults := 0

	for _, m := range occEnvRE.FindAllStringSubmatch(occSrc(t, root, "config", "env.mjs"), -1) {
		if !strings.HasPrefix(m[1], "CLAUDE_CODE_") {
			continue
		}

		envSeen[m[1]] = true

		if occDefaultRE.MatchString(m[2]) {
			withDefaults++
		}
	}

	return sortedKeys(envSeen), withDefaults
}

// extractOccModes returns the permission-mode case labels.
func extractOccModes(t *testing.T, root string) []string {
	t.Helper()

	modeSeen := map[string]bool{}
	for _, m := range occCaseRE.FindAllStringSubmatch(occSrc(t, root, "permissions", "checker.mjs"), -1) {
		modeSeen[m[1]] = true
	}

	return sortedKeys(modeSeen)
}

// extractOccTransports returns the MCP transport case labels.
func extractOccTransports(t *testing.T, root string) []string {
	t.Helper()

	trSeen := map[string]bool{}
	for _, m := range occClientRE.FindAllStringSubmatch(occSrc(t, root, "mcp", "client.mjs"), -1) {
		trSeen[m[1]] = true
	}

	return sortedKeys(trSeen)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// TestOccCensusDeterministic: when the pinned clone is present (gitignored —
// re-clone per the fixture's provenance), a re-extraction at the SAME pinned
// commit reproduces the committed census exactly, and a second extraction is
// byte-stable — the idempotency edge pinned. Skip-loud when the clone is
// absent (CI): the fixture shape + accounting tests still run.
func TestOccCensusDeterministic(t *testing.T) { //nolint:paralleltest // execs git in the shared clone
	root, err := filepath.Abs(filepath.Join("..", "..", "tools", "occ-census-clone"))
	if err != nil || !isOccClonePinned(root) {
		t.Skipf("occ clone absent at %s — re-run `git clone --depth 1 "+
			"https://github.com/ruvnet/open-claude-code tools/occ-census-clone` to verify determinism", root)
	}

	gitCtx, gitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer gitCancel()

	out, err := exec.CommandContext(gitCtx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Skipf("clone commit unreadable: %v", err)
	}

	commit := strings.TrimSpace(string(out))

	want := loadOccCensus(t)
	if want.Provenance.Commit != commit {
		t.Fatalf("clone HEAD %s != fixture pin %s — re-pin required", commit, want.Provenance.Commit)
	}

	got := extractOccCensus(t, root)
	got2 := extractOccCensus(t, root)

	if !equalStrings(got.Tools, want.Tools) {
		t.Errorf("re-extracted tools differ:\n got %v\nwant %v", got.Tools, want.Tools)
	}

	if !equalStrings(got.Env.Names, want.Env.Names) {
		t.Errorf("re-extracted env names differ (got %d, want %d)", len(got.Env.Names), len(want.Env.Names))
	}

	if got.Env.WithDefaults != want.Env.WithDefaults {
		t.Errorf("re-extracted env with_defaults = %d; want %d", got.Env.WithDefaults, want.Env.WithDefaults)
	}

	if !equalStrings(got.PermissionModes, want.PermissionModes) {
		t.Errorf("re-extracted permission modes differ: %v vs %v", got.PermissionModes, want.PermissionModes)
	}

	if !equalStrings(got.MCPTransports, want.MCPTransports) {
		t.Errorf("re-extracted mcp transports differ: %v vs %v", got.MCPTransports, want.MCPTransports)
	}

	// Idempotency: two consecutive extractions are identical.
	if !equalStrings(got.Tools, got2.Tools) || !equalStrings(got.Env.Names, got2.Env.Names) {
		t.Error("extraction is not deterministic across consecutive runs")
	}
}

func isOccClonePinned(root string) bool {
	st, err := os.Stat(filepath.Join(root, "v2", "src", "tools", "registry.mjs"))

	return err == nil && !st.IsDir()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
