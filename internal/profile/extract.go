package profile

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ModelIO mirrors the per-line schema of a zcode rollout JSONL file
// (VERIFIED-FACTS.md item #1). Only the fields ass-guard extracts are modeled;
// the rest are ignored. The rollout captures the FULL wire-level request body
// (system, tools, thinking, tool_choice) plus the 12 identity headers — no MITM
// proxy is needed for the request body.
type ModelIO struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"` //nolint:tagliatelle // external model_io rollout format (camelCase source) — cannot rename
	Request   struct {
		Body struct {
			Model      json.RawMessage `json:"model"`
			MaxTokens  int             `json:"max_tokens"`
			System     []SystemBlock   `json:"system"`
			Tools      []RawTool       `json:"tools"`
			Thinking   json.RawMessage `json:"thinking"`
			ToolChoice json.RawMessage `json:"tool_choice"`
		} `json:"body"`
		Headers map[string]json.RawMessage `json:"headers"`
	} `json:"request"`
}

// SystemBlock is one entry of request.body.system (Anthropic system-block shape).
type SystemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// RawTool is one entry of request.body.tools before null-name filtering.
type RawTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ExtractResult is the profile shape extracted from one rollout session, plus
// the metadata needed for the coverage manifest and target_capture_ref.
type ExtractResult struct {
	System      []TextBlock
	Tools       []Decl
	Headers     []Header
	Thinking    json.RawMessage
	ToolChoice  json.RawMessage
	Model       string
	MaxTokens   int
	SessionID   string
	SessionRole string // "main" or "subagent" (inferred from the file name)
	SourcePath  string
	ToolCount   int // post-null-filter count
}

// SessionStat summarizes one rollout file for picking the richest source.
type SessionStat struct {
	ID               string
	Path             string
	Role             string
	FullRequestLines int
	FirstToolCount   int
}

// ExtractFromRollout reads a single rollout JSONL file, extracts the profile
// shape from the first full-request line (system present AND tools present),
// and asserts the TIER-1 shape (system block count, tool NAME set, header NAME
// set) is stable across every full-request line in the session. Null/empty-named
// tools are filtered (the observed edge case).
func ExtractFromRollout(path string) (ExtractResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("open rollout: %w", err)
	}
	defer func() { _ = f.Close() }()

	var (
		firstFull *ModelIO
		firstFile string
	)
	// Stability signatures built from the first full line; later lines must agree.
	firstSysCount := -1
	firstToolNames := map[string]struct{}{}
	firstHeaderNames := []string{}

	scanner := bufio.NewScanner(f)
	// Rollout lines can be large (system prompts + tool schemas per line); raise the buffer.
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	lineIdx := 0
	for scanner.Scan() {
		lineIdx++

		raw := scanner.Bytes()
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}

		var mio ModelIO

		err := json.Unmarshal(raw, &mio)
		if err != nil {
			return ExtractResult{}, fmt.Errorf("line %d: parse: %w", lineIdx, err)
		}

		if !isFullRequest(mio) {
			continue
		}
		// Stability check against the first full line.
		if firstFull == nil {
			firstFull = &mio
			firstFile = path

			firstSysCount = len(mio.Request.Body.System)
			for _, t := range mio.Request.Body.Tools {
				if t.Name != "" {
					firstToolNames[t.Name] = struct{}{}
				}
			}

			for k := range mio.Request.Headers {
				firstHeaderNames = append(firstHeaderNames, k)
			}

			sort.Strings(firstHeaderNames)
		} else {
			err := assertStable(firstSysCount, firstToolNames, firstHeaderNames, mio)
			if err != nil {
				return ExtractResult{}, fmt.Errorf("line %d: within-session stability: %w", lineIdx, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return ExtractResult{}, fmt.Errorf("scan rollout: %w", err)
	}

	if firstFull == nil {
		return ExtractResult{}, fmt.Errorf("no full-request lines (system+tools) found in %s", firstFile)
	}

	return buildResult(*firstFull, path), nil
}

func isFullRequest(m ModelIO) bool {
	return m.Type == "model_io" && len(m.Request.Body.System) > 0 && len(m.Request.Body.Tools) > 0
}

func assertStable(sysCount int, toolNames map[string]struct{}, headerNames []string, m ModelIO) error {
	if len(m.Request.Body.System) != sysCount {
		return fmt.Errorf("system block count drift: %d -> %d", sysCount, len(m.Request.Body.System))
	}

	seen := map[string]struct{}{}

	for _, t := range m.Request.Body.Tools {
		if t.Name == "" {
			continue
		}

		seen[t.Name] = struct{}{}
	}

	if len(seen) != len(toolNames) {
		return fmt.Errorf("tool-name set drift: %d -> %d", len(toolNames), len(seen))
	}

	for n := range seen {
		if _, ok := toolNames[n]; !ok {
			return fmt.Errorf("tool-name set drift: %q not in first-line set", n)
		}
	}

	curHeaders := make([]string, 0, len(m.Request.Headers))
	for k := range m.Request.Headers {
		curHeaders = append(curHeaders, k)
	}

	sort.Strings(curHeaders)

	if strings.Join(curHeaders, ",") != strings.Join(headerNames, ",") {
		return fmt.Errorf("header-name set drift: %v -> %v", headerNames, curHeaders)
	}

	return nil
}

func buildResult(m ModelIO, path string) ExtractResult {
	res := ExtractResult{
		SessionID:  m.SessionID,
		Thinking:   m.Request.Body.Thinking,
		ToolChoice: m.Request.Body.ToolChoice,
		MaxTokens:  m.Request.Body.MaxTokens,
		SourcePath: path,
	}
	res.SessionRole = inferRole(path)
	// Model may be a JSON string or an object; normalize to a bare string.
	res.Model = string(m.Request.Body.Model)

	res.Model = strings.Trim(res.Model, `"`)
	for _, b := range m.Request.Body.System {
		res.System = append(res.System, TextBlock(b))
	}

	for _, t := range m.Request.Body.Tools {
		if t.Name == "" {
			continue // filter the null/empty-named edge case
		}

		res.Tools = append(res.Tools, Decl(t))
	}

	res.ToolCount = len(res.Tools)
	// Headers: byte-faithful NAMES, templated VALUES (never stored verbatim — T-01-02).
	names := make([]string, 0, len(m.Request.Headers))
	for k := range m.Request.Headers {
		names = append(names, k)
	}

	sort.Strings(names)

	for _, n := range names {
		res.Headers = append(res.Headers, Header{Name: n, ValueTemplate: "<" + strings.ToLower(strings.ReplaceAll(n, "-", "_")) + ">"})
	}

	return res
}

// inferRole classifies a rollout file as "main" or "subagent" from its name.
// D-16 finding: main and subagent sessions have DIFFERENT built-in tool sets +
// system block counts; the stability test treats cross-role comparison as
// informational, not a hard failure.
func inferRole(path string) string {
	base := filepath.Base(path)
	if strings.Contains(base, "subagent") {
		return "subagent"
	}

	return sessionKindMain
}

// ScanRolloutDir lists model-io-sess_*.jsonl files under dir and summarizes each
// for picking the richest extraction source (D-16: scan at execution time).
func ScanRolloutDir(dir string) ([]SessionStat, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read rollout dir: %w", err)
	}

	var stats []SessionStat

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "model-io-sess_") || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		full := filepath.Join(dir, e.Name())
		st := SessionStat{Path: full, ID: sessionIDFromName(e.Name()), Role: inferRole(full)}
		st.FullRequestLines, st.FirstToolCount = countFullRequests(full)
		stats = append(stats, st)
	}

	sort.Slice(stats, func(i, j int) bool {
		// Prefer main sessions with the most full-request lines.
		if stats[i].Role != stats[j].Role {
			return stats[i].Role == sessionKindMain
		}

		return stats[i].FullRequestLines > stats[j].FullRequestLines
	})

	return stats, nil
}

func sessionIDFromName(name string) string {
	n := strings.TrimPrefix(name, "model-io-sess_")

	return strings.TrimSuffix(n, ".jsonl")
}

// countFullRequests returns (full-request line count, first line's tool count).
func countFullRequests(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	full, firstTools := 0, 0

	for scanner.Scan() {
		var m ModelIO
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			continue
		}

		if !isFullRequest(m) {
			continue
		}

		full++
		if full == 1 {
			firstTools = len(m.Request.Body.Tools)
		}
	}

	return full, firstTools
}

// PickRichestMain returns the richest main-role session from stats, or the
// richest overall if no main session exists. Returns an error if stats is empty.
func PickRichestMain(stats []SessionStat) (SessionStat, error) {
	if len(stats) == 0 {
		return SessionStat{}, errors.New("no rollout sessions found")
	}

	for _, s := range stats {
		if s.Role == sessionKindMain && s.FullRequestLines > 0 {
			return s, nil
		}
	}
	// Fall back to whatever has the most full-request lines.
	for _, s := range stats {
		if s.FullRequestLines > 0 {
			return s, nil
		}
	}

	return SessionStat{}, errors.New("no session with full-request lines found")
}

// ExtractedAtNow is a tiny helper for timestamping manifests in tests/prod.
func ExtractedAtNow() time.Time { return time.Now().UTC() }
