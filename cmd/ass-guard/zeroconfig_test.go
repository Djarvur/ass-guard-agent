package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestZeroConfigFirstRun is the Phase-6 TRACER proof (plan 06-01 T5): it builds
// the real ass-guard binary, drops it into a fresh empty project directory as
// the registry would, sends one ACP `initialize` frame over stdio, and asserts:
//
//   (a) .ass-guard/ was created with .gitignore (body "*\n!.gitignore\n"),
//       profiles/zcode/tools.json, openspec.toml, scheduling.yaml (D-01/D-04);
//   (b) the seeded zcode profile loaded — a valid initialize response with
//       agentCapabilities appears on stdout (runACPServe loads the profile
//       before serving; a failed load yields no response);
//   (c) stdout carries ONLY ACP frames — the first-run log went to stderr
//       (transport discipline, T-06-03);
//   (d) a second invocation in the same dir does NOT re-seed (idempotent — the
//       .gitignore is unchanged and stderr omits the "initialized" log).
//
// No ZAI_API_KEY is required: the handshake + first-run + profile load never
// call the model (DIST-03 is about defaults being present, not a live turn).
func TestZeroConfigFirstRun(t *testing.T) {
	// Building the binary also proves the go:embed survives compilation into a
	// real artifact (the embed is compiled into the cmd/ass-guard main package).
	tmpBin := filepath.Join(t.TempDir(), "ass-guard")

	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled // build-dir resolution
	pkgDir := filepath.Dir(thisFile)

	build := exec.Command("go", "build", "-o", tmpBin, ".")
	build.Dir = pkgDir

	var buildErr bytes.Buffer
	build.Stderr = &buildErr
	if err := build.Run(); err != nil {
		t.Fatalf("go build . (in %s): %v\n%s", pkgDir, err, buildErr.String())
	}

	project := t.TempDir()

	// First run: drive the initialize handshake and capture stdout/stderr.
	firstStdout, firstStderr := driveServeInit(t, tmpBin, project)

	// (c) Transport discipline: every non-empty stdout line is a valid ACP frame.
	var sawAgentCapabilities bool

	for i, line := range strings.Split(strings.TrimRight(firstStdout, "\n"), "\n") {
		if line == "" {
			continue
		}

		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("stdout line %d is not valid JSON (transport discipline): %v (line=%q)", i, err, line)
		}

		if strings.Contains(line, "agentCapabilities") {
			sawAgentCapabilities = true
		}
	}

	// (b) The seeded profile loaded: the initialize response is on stdout.
	if !sawAgentCapabilities {
		t.Errorf("stdout missing initialize response (agentCapabilities) — profile load likely failed.\nstdout:\n%s\nstderr:\n%s",
			firstStdout, firstStderr)
	}

	// (a) .ass-guard/ was seeded with the embedded defaults.
	assGuard := filepath.Join(project, ".ass-guard")

	for _, rel := range []string{
		".gitignore",
		"profiles/zcode/tools.json",
		"profiles/zcode/profile.yaml",
		"openspec.toml",
		"scheduling.yaml",
	} {
		if _, err := os.Stat(filepath.Join(assGuard, rel)); err != nil {
			t.Errorf("expected .ass-guard/%s seeded: %v", rel, err)
		}
	}

	// The .gitignore body is the canonical D-07 self-gitignore.
	gi, err := os.ReadFile(filepath.Join(assGuard, ".gitignore"))
	if err != nil {
		t.Fatalf("read .ass-guard/.gitignore: %v", err)
	}

	const wantGI = "*\n!.gitignore\n"
	if string(gi) != wantGI {
		t.Errorf(".gitignore body = %q, want %q (D-07)", gi, wantGI)
	}

	// The first-run log went to stderr (transport discipline) and the seeded
	// profile carries 103 tools (catalog drift — not the plan's stale 77).
	if !strings.Contains(firstStderr, "ass-guard: initialized") {
		t.Errorf("stderr missing first-run init log; got:\n%s", firstStderr)
	}

	// (d) Idempotence: a second run in the same dir does NOT re-seed. Capture the
	// .gitignore mtime, run again, assert it is unchanged + no "initialized" log.
	giPath := filepath.Join(assGuard, ".gitignore")
	beforeInfo, err := os.Stat(giPath)
	if err != nil {
		t.Fatalf("stat .gitignore before second run: %v", err)
	}

	_, secondStderr := driveServeInit(t, tmpBin, project)

	afterInfo, err := os.Stat(giPath)
	if err != nil {
		t.Fatalf("stat .gitignore after second run: %v", err)
	}

	// mtime unchanged ⇒ the file was not rewritten (non-clobbering, idempotent).
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Errorf("second run rewrote .gitignore (non-idempotent): mtime %s → %s",
			beforeInfo.ModTime(), afterInfo.ModTime())
	}

	if strings.Contains(secondStderr, "ass-guard: initialized") {
		t.Errorf("second run re-seeded an existing .ass-guard/ (non-idempotent):\n%s", secondStderr)
	}
}

// driveServeInit runs `<bin> acp serve` in dir, sends one ACP initialize frame
// on stdin, reads the response(s) from stdout, then closes stdin to let the
// server exit. Returns (stdout, stderr). stdout MUST be drained before Wait
// (os/exec closes the pipe on Wait); a goroutine + timeout guards against a
// hung server.
func driveServeInit(t *testing.T, bin, dir string) (string, string) { //nolint:nonamedreturns // names document the pair
	t.Helper()

	cmd := exec.Command(bin, "acp", "serve")
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}

	// Send the initialize frame (ACP v1: protocolVersion integer 1).
	initFrame := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":` +
		`{"protocolVersion":1,"clientCapabilities":{},` +
		`"clientInfo":{"name":"zeroconfig-test","version":"0"}}}` + "\n"

	_, writeErr := io.WriteString(stdin, initFrame)
	_ = stdin.Close()

	if writeErr != nil {
		t.Fatalf("write init frame: %v", writeErr)
	}

	// Drain stdout BEFORE Wait (Wait closes the pipe). EOF arrives when the child
	// exits (stdin EOF ends Serve). Bounded so a hung server cannot stall the suite.
	type drain struct{ b []byte }

	stdoutCh := make(chan drain, 1)
	go func() {
		b, _ := io.ReadAll(stdoutPipe)
		stdoutCh <- drain{b}
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var stdoutBytes []byte

	select {
	case r := <-stdoutCh:
		stdoutBytes = r.b
		<-waitCh // reap the child
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
		t.Fatalf("acp serve did not exit within 15s after stdin EOF")
	}

	return string(stdoutBytes), stderr.String()
}
