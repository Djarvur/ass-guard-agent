package mcp //nolint:testpackage // internal package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grandchildConfig builds a ServerConfig that re-execs the test binary as the
// grandchild-forking echo server. The grandchild writes its PID+PGID to pidFile.
func grandchildConfig(t *testing.T, pidFile, mode string, extraEnv ...string) Config {
	t.Helper()

	testBin := os.Args[0]

	envMap := map[string]string{"PATH": os.Getenv("PATH")}
	if pidFile != "" {
		envMap[pidFileEnv] = pidFile
	}

	for _, e := range extraEnv {
		if before, after, ok := strings.Cut(e, "="); ok {
			envMap[before] = after
		}
	}

	return Config{Servers: []ServerConfig{{
		Name:    echoServerName,
		Command: testBin,
		Args:    []string{mode},
		Env:     envMap,
	}}}
}

// readGrandchildPID reads "pid pgid" from the pidfile.
//
//nolint:nonamedreturns // gocritic unnamedResult prefers names for two-int clarity
func readGrandchildPID(t *testing.T, pidFile string) (pid, pgid int) {
	t.Helper()

	data, err := waitForFile(pidFile, 5*time.Second)
	require.NoError(t, err, "grandchild pidfile not written")

	parts := strings.Fields(string(data))
	require.Len(t, parts, 2, "pidfile format: %q", data)

	p, err := strconv.Atoi(parts[0])
	require.NoError(t, err)

	g, err := strconv.Atoi(parts[1])
	require.NoError(t, err)

	return p, g
}

// waitForFile polls for path to exist + be non-empty, up to the timeout.
func waitForFile(path string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data, nil
		}

		time.Sleep(50 * time.Millisecond)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pidfile %s: %w", path, err)
	}

	return data, nil
}

// TestProcessGroupIsolation proves Setpgid put the server in its own process
// group: the grandchild's PGID equals the SERVER's PID (the group leader), NOT
// ass-guard's PGID (T1).
func TestProcessGroupIsolation(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns subprocesses")
	}

	if runtime.GOOS == "windows" { //nolint:goconst // platform check
		t.Skip("process-group test is darwin/linux only")
	}

	pidFile := filepath.Join(t.TempDir(), "gc.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, _, err := Start(ctx, grandchildConfig(t, pidFile, grandchildServerArg))
	require.NoError(t, err)

	gcPID, gcPGID := readGrandchildPID(t, pidFile)

	serverPGID, ok := host.PGID(echoServerName)
	require.True(t, ok, "server PGID not recorded")

	// The grandchild PID is a real live process.
	require.True(t, processAlive(gcPID), "grandchild should be alive")

	// The grandchild's PGID must equal the server's PGID (group leader).
	assert.Equal(t, serverPGID, gcPGID,
		"grandchild PGID (%d) must equal server PGID (%d) — Setpgid isolation", gcPGID, serverPGID)

	// The grandchild's PGID must NOT be ass-guard's PGID.
	selfPGID := os.Getpid()
	// (ass-guard's PGID is its own; on a clean test run it differs from the
	// spawned group. We assert the server is in a DIFFERENT group.)
	assert.NotEqual(t, selfPGID, gcPGID, "server group must differ from ass-guard's group")

	require.NoError(t, host.Close())
}

// TestPGIDCapture verifies Host.PGID records each server's PGID after Start.
func TestPGIDCapture(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns subprocesses")
	}

	if runtime.GOOS == "windows" {
		t.Skip("process-group test is darwin/linux only")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, _, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	defer func() { _ = host.Close() }()

	pgid, ok := host.PGID(echoServerName)
	require.True(t, ok)
	assert.Positive(t, pgid, "PGID should be a live PID")
}

// TestShutdownGrandchildReached proves the group signal reaches the grandchild:
// after Close, the grandchild PID is no longer alive (T2 Test 3).
func TestShutdownGrandchildReached(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns subprocesses + 3s grace")
	}

	if runtime.GOOS == "windows" {
		t.Skip("process-group test is darwin/linux only")
	}

	pidFile := filepath.Join(t.TempDir(), "gc.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	host, _, err := Start(ctx, grandchildConfig(t, pidFile, grandchildServerArg))
	require.NoError(t, err)

	gcPID, _ := readGrandchildPID(t, pidFile)
	require.True(t, processAlive(gcPID), "grandchild should be alive before Close")

	require.NoError(t, host.Close())

	// The grandchild must be dead (group signal reached the whole tree).
	require.False(t, processAlive(gcPID), "grandchild still alive after Close — group signal failed")
}

// TestShutdownForceSIGKILL proves a SIGTERM-ignoring server is killed by SIGKILL
// after the grace period (T2 Test 2).
func TestShutdownForceSIGKILL(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns subprocesses + 3s grace")
	}

	if runtime.GOOS == "windows" {
		t.Skip("process-group test is darwin/linux only")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host, _, err := Start(ctx, grandchildConfig(t, "", sigtermIgnoreArg))
	require.NoError(t, err)

	pgid, ok := host.PGID(echoServerName)
	require.True(t, ok)

	require.NoError(t, host.Close())

	// The whole group (including the SIGTERM-ignoring server) must be gone.
	assert.False(t, processAlive(pgid), "SIGTERM-ignoring server survived SIGKILL")
}

// TestCloseIdempotent proves double-Close is a no-op (T2 Test 4).
func TestCloseIdempotent(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns subprocesses")
	}

	if runtime.GOOS == "windows" {
		t.Skip("process-group test is darwin/linux only")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, _, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	require.NoError(t, host.Close())
	require.NoError(t, host.Close(), "second Close must be a no-op")
}

// TestReapNoZombie proves no process in the server's group is a zombie after
// Close (T3 Test 1): Wait4(-pgid, WNOHANG) returns ECHILD (nothing left).
func TestReapNoZombie(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns subprocesses")
	}

	if runtime.GOOS == "windows" {
		t.Skip("process-group test is darwin/linux only")
	}

	pidFile := filepath.Join(t.TempDir(), "gc.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	host, _, err := Start(ctx, grandchildConfig(t, pidFile, grandchildServerArg))
	require.NoError(t, err)

	pgid, ok := host.PGID(echoServerName)
	require.True(t, ok)

	require.NoError(t, host.Close())

	// After Close, reaping the group yields ECHILD (fully drained). This loop
	// best-effort confirms nothing is left to reap.
	reap(pgid)

	// processAlive on the server PID returns false (gone, reaped).
	assert.False(t, processAlive(pgid), "server PID still alive after Close+reap")
}

// TestReapNonBlocking proves reap on an empty group returns immediately.
func TestReapNonBlocking(t *testing.T) { //nolint:paralleltest // uses process table
	if runtime.GOOS == "windows" {
		t.Skip("process-group test is darwin/linux only")
	}

	done := make(chan struct{})

	go func() {
		reap(999999) // unlikely to be a real group with children
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reap on empty group did not return within 2s (should be non-blocking)")
	}
}

// TestPrepareCommandSetsEnv verifies prepareCommand applies the env overlay.
func TestPrepareCommandSetsEnv(t *testing.T) { //nolint:paralleltest // uses process table
	if runtime.GOOS == "windows" {
		t.Skip("unix-only lifecycle")
	}

	cmd := exec.Command("echo") //nolint:noctx // never started; tests SysProcAttr only
	cfg := ServerConfig{Env: map[string]string{"FOO_TEST": "bar"}}
	require.NoError(t, prepareCommand(cmd, &cfg))

	// Env should contain FOO_TEST=bar.
	found := false

	for _, e := range cmd.Env {
		if e == "FOO_TEST=bar" {
			found = true
		}
	}

	assert.True(t, found, "prepareCommand must apply the env overlay")
	assert.NotNil(t, cmd.SysProcAttr, "prepareCommand must set SysProcAttr (Setpgid)")
}
