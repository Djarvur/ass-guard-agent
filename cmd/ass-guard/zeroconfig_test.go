package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// wantGitignore is the canonical D-07 self-gitignore body (must match
// internal/session/transcript.go selfGitignoreContent byte-for-byte).
const wantGitignore = "*\n!.gitignore\n"

// TestZeroConfigFirstRun is the Phase-6 TRACER proof (plan 06-01 T5): it builds
// the real ass-guard binary, drops it into a fresh empty project directory as
// the registry would, sends one ACP initialize frame over stdio, and asserts:
// .ass-guard/ is seeded (D-01/D-04); the seeded zcode profile loaded (a valid
// initialize response with agentCapabilities is on stdout); stdout carries ONLY
// ACP frames (transport discipline, T-06-03); and a second run does not re-seed
// (idempotent). No ZAI_API_KEY is required — the handshake never calls the model.
func TestZeroConfigFirstRun(t *testing.T) {
	t.Parallel()

	bin := buildServeBinary(t)
	project := t.TempDir()

	firstOut, firstErr := driveServeInit(t, bin, project)

	assertACPHandshakeOnStdout(t, firstOut, firstErr)
	assertAssGuardSeeded(t, project)

	if !strings.Contains(firstErr, "ass-guard: initialized") {
		t.Errorf("stderr missing first-run init log; got:\n%s", firstErr)
	}

	assertSecondRunIsIdempotent(t, bin, project)
}

// buildServeBinary builds the ass-guard binary by import path (robust under
// -trimpath and from any cwd — no filesystem path lookup). Building the real
// binary also proves the go:embed survives compilation into a real artifact.
func buildServeBinary(t *testing.T) string {
	t.Helper()

	tmpBin := filepath.Join(t.TempDir(), "ass-guard")

	build := exec.CommandContext(t.Context(), "go", "build", "-o", tmpBin,
		"github.com/Djarvur/ass-guard-agent/cmd/ass-guard")

	var buildErr bytes.Buffer

	build.Stderr = &buildErr

	err := build.Run()
	if err != nil {
		t.Fatalf("go build cmd/ass-guard: %v\n%s", err, buildErr.String())
	}

	return tmpBin
}

// assertACPHandshakeOnStdout asserts every stdout line is a valid ACP frame and
// at least one carries agentCapabilities (proving the seeded profile loaded —
// runACPServe returns early on a failed load, yielding no response).
func assertACPHandshakeOnStdout(t *testing.T, stdout, stderr string) {
	t.Helper()

	var sawAgentCapabilities bool

	for i, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if line == "" {
			continue
		}

		var m map[string]any

		err := json.Unmarshal([]byte(line), &m)
		if err != nil {
			t.Errorf("stdout line %d not valid JSON (transport discipline): %v (%q)", i, err, line)
		}

		if strings.Contains(line, "agentCapabilities") {
			sawAgentCapabilities = true
		}
	}

	if !sawAgentCapabilities {
		t.Errorf("stdout missing initialize response (agentCapabilities) — profile load failed")
		t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

// assertAssGuardSeeded asserts .ass-guard/ was materialized with the embedded
// defaults and the canonical D-07 self-gitignore body.
func assertAssGuardSeeded(t *testing.T, project string) {
	t.Helper()

	assGuard := filepath.Join(project, ".ass-guard")

	for _, rel := range []string{
		".gitignore",
		filepath.Join("profiles", "zcode", "tools.json"),
		filepath.Join("profiles", "zcode", "profile.yaml"),
		"openspec.toml",
		"config.yaml",
	} {
		_, err := os.Stat(filepath.Join(assGuard, rel))
		if err != nil {
			t.Errorf("expected .ass-guard/%s seeded: %v", rel, err)
		}
	}

	gi, err := os.ReadFile(filepath.Join(assGuard, ".gitignore"))
	if err != nil {
		t.Fatalf("read .ass-guard/.gitignore: %v", err)
	}

	if string(gi) != wantGitignore {
		t.Errorf(".gitignore body = %q, want %q (D-07)", gi, wantGitignore)
	}
}

// assertSecondRunIsIdempotent asserts a second invocation in the same dir does
// not re-seed: the .gitignore mtime is unchanged and stderr omits "initialized".
func assertSecondRunIsIdempotent(t *testing.T, bin, project string) {
	t.Helper()

	giPath := filepath.Join(project, ".ass-guard", ".gitignore")

	beforeInfo, err := os.Stat(giPath)
	if err != nil {
		t.Fatalf("stat .gitignore before second run: %v", err)
	}

	_, secondErr := driveServeInit(t, bin, project)

	afterInfo, err := os.Stat(giPath)
	if err != nil {
		t.Fatalf("stat .gitignore after second run: %v", err)
	}

	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Errorf("second run rewrote .gitignore (non-idempotent): mtime %s → %s",
			beforeInfo.ModTime(), afterInfo.ModTime())
	}

	if strings.Contains(secondErr, "ass-guard: initialized") {
		t.Errorf("second run re-seeded an existing .ass-guard/ (non-idempotent):\n%s", secondErr)
	}
}

// driveServeInit runs `<bin> acp serve` in dir, sends one ACP initialize frame
// on stdin, drains stdout (BEFORE Wait — os/exec closes the pipe on Wait), then
// reaps the child. A bounded timeout guards against a hung server. Returns the
// captured stdout and stderr.
//
//nolint:nonamedreturns // the names document the (stdout, stderr) pair
func driveServeInit(t *testing.T, bin, dir string) (stdout, stderr string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, "acp", "serve")
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	var stderrBuf bytes.Buffer

	cmd.Stderr = &stderrBuf

	err = cmd.Start()
	if err != nil {
		t.Fatalf("start serve: %v", err)
	}

	// ACP v1 initialize (protocolVersion is integer 1).
	initFrame := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":` +
		`{"protocolVersion":1,"clientCapabilities":{},` +
		`"clientInfo":{"name":"zeroconfig-test","version":"0"}}}` + "\n"

	_, err = io.WriteString(stdin, initFrame)
	_ = stdin.Close()

	if err != nil {
		t.Fatalf("write init frame: %v", err)
	}

	stdoutBytes := drainServeStdout(t, cmd, stdoutPipe)

	return string(stdoutBytes), stderrBuf.String()
}

// drainServeStdout reads stdout to EOF (child exit), bounded by a timeout so a
// hung server cannot stall the suite. Must complete BEFORE cmd.Wait (Wait closes
// the pipe).
func drainServeStdout(t *testing.T, cmd *exec.Cmd, stdoutPipe io.ReadCloser) []byte {
	t.Helper()

	type drain struct{ b []byte }

	stdoutCh := make(chan drain, 1)

	go func() {
		b, _ := io.ReadAll(stdoutPipe)
		stdoutCh <- drain{b}
	}()

	var captured []byte

	select {
	case r := <-stdoutCh:
		captured = r.b
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		t.Fatalf("acp serve did not exit within 15s after stdin EOF")

		return nil
	}

	// stdout drained to EOF (child exited); reap it before returning.
	_ = cmd.Wait()

	return captured
}
