package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeCaptureFixture writes a model_io JSON line to a temp file and returns
// its path. The capture carries the field counts the drift detector inspects.
func writeCaptureFixture(t *testing.T, sysCount, toolCount, headerCount int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.json")

	sys := make([]map[string]any, sysCount)
	for i := range sys {
		sys[i] = map[string]any{keyType: blockText, blockText: "x"}
	}

	tools := make([]map[string]any, toolCount)
	for i := range tools {
		tools[i] = map[string]any{"name": "t", "input_schema": map[string]any{keyType: "object"}}
	}

	headers := map[string]any{}
	for i := range headerCount {
		headers["h"+itoa(i)] = "x"
	}

	line := map[string]any{
		keyType:      "model_io",
		keySessionID: "s",
		"request": map[string]any{
			"body": map[string]any{
				"model":       "GLM-5.2",
				"system":      sys,
				"tools":       tools,
				"thinking":    map[string]any{keyType: "enabled"},
				"tool_choice": map[string]any{keyType: "auto"},
			},
			"headers": headers,
		},
	}

	raw, _ := json.Marshal(line)

	err := os.WriteFile(path, raw, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}

	return out
}

// writeCoverageFixture writes a coverage.yaml manifest with the given declared
// counts into a profiles/<name>/ directory and returns the profiles dir.
func writeCoverageFixture(t *testing.T, name string, sysCount, toolCount, headerCount int) string {
	t.Helper()
	root := t.TempDir()

	pdir := filepath.Join(root, name)

	err := os.MkdirAll(pdir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	manifest := []byte("profile: " + name + "\nfields:\n" +
		"  - path: request.body.system\n    tier: 1\n    observed_count: " + itoa(sysCount) + "\n" +
		"  - path: request.body.tools\n    tier: 1\n    observed_count: " + itoa(toolCount) + "\n" +
		"  - path: request.headers\n    tier: 2\n    observed_count: " + itoa(headerCount) + "\n")

	err = os.WriteFile(filepath.Join(pdir, "coverage.yaml"), manifest, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return root
}

// TestProfileCheck_NoDrift: capture matches manifest → no drift, nil error.
func TestProfileCheck_NoDrift(t *testing.T) {
	t.Parallel()
	profilesDir := writeCoverageFixture(t, profileZcode, 3, 103, 12)
	capture := writeCaptureFixture(t, 3, 103, 12)

	err := runProfileCheck(profileZcode, profilesDir, capture)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestProfileCheck_DriftDetected: capture's tool count differs → error + drift.
func TestProfileCheck_DriftDetected(t *testing.T) { //nolint:paralleltest // swaps process-global os.Stderr
	profilesDir := writeCoverageFixture(t, profileZcode, 3, 103, 12)
	capture := writeCaptureFixture(t, 3, 90, 12) // tools drifted

	var stderr bytes.Buffer

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	done := make(chan struct{})

	go func() { stderr.ReadFrom(r); close(done) }()

	err := runProfileCheck(profileZcode, profilesDir, capture)

	w.Close()

	os.Stderr = oldStderr

	<-done

	if err == nil {
		t.Fatal("expected a drift error, got nil")
	}

	if !bytes.Contains(stderr.Bytes(), []byte("DRIFT")) {
		t.Errorf("stderr missing DRIFT footer:\n%s", stderr.String())
	}

	if !bytes.Contains(stderr.Bytes(), []byte("request.body.tools")) {
		t.Errorf("stderr missing the drifted field:\n%s", stderr.String())
	}
}

// TestProfileCheck_ReportContainsStructuredFooter confirms the C6 footer shape.
func TestProfileCheck_ReportContainsStructuredFooter(t *testing.T) { //nolint:paralleltest // swaps process-global os.Stderr
	profilesDir := writeCoverageFixture(t, profileZcode, 3, 103, 12)
	capture := writeCaptureFixture(t, 3, 103, 12)

	var stderr bytes.Buffer

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	done := make(chan struct{})

	go func() { stderr.ReadFrom(r); close(done) }()

	_ = runProfileCheck(profileZcode, profilesDir, capture)

	w.Close()

	os.Stderr = oldStderr

	<-done

	for _, want := range []string{"=== PROFILE CHECK", "overall_status:", "drifts:"} {
		if !bytes.Contains(stderr.Bytes(), []byte(want)) {
			t.Errorf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}
