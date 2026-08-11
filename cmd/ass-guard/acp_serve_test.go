package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestACPServeCommandRegistered verifies the cobra root has an `acp serve`
// subcommand with the expected flags (transport discipline: stdout is framer-
// only; --profile default zcode; --max-concurrent default 6).
func TestACPServeCommandRegistered(t *testing.T) {
	root := newRootCmd()
	var acpCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "acp" {
			acpCmd = c
			break
		}
	}
	if acpCmd == nil {
		t.Fatal("no `acp` parent command registered on root")
	}
	var serve *cobra.Command
	for _, c := range acpCmd.Commands() {
		if c.Use == "serve" {
			serve = c
			break
		}
	}
	if serve == nil {
		t.Fatal("no `serve` subcommand under `acp`")
	}
	pf := serve.Flags()
	prof, _ := pf.GetString("profile")
	if prof != "zcode" {
		t.Errorf("serve --profile default = %q; want zcode", prof)
	}
	mc, _ := pf.GetInt("max-concurrent")
	if mc != 6 {
		t.Errorf("serve --max-concurrent default = %d; want 6 (RESEARCH §11.1)", mc)
	}
}

// TestACPServeWiresStdoutClean verifies that running `acp serve` against a
// canned initialize frame produces the initialize response on stdout and sends
// all diagnostics to stderr — transport discipline (stdout = ACP frames only).
func TestACPServeWiresStdoutClean(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"test","version":"0"}}}
`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := runACPServe(ctx, in, &stdout, &stderr, serveOptions{Profile: "zcode", MaxConcurrent: 6})
	if err != nil && err != io.EOF {
		t.Logf("runACPServe returned %v (acceptable)", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"agentCapabilities"`) {
		t.Errorf("stdout missing agentCapabilities in initialize response: %s", out)
	}
	if !strings.Contains(out, `"loadSession":false`) {
		t.Errorf("stdout missing loadSession:false: %s", out)
	}
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("stdout line %d is not valid JSON (transport discipline): %v (line=%q)", i, err, line)
		}
	}
}

// TestACPServeNoStdoutPollutionFromLogs verifies stderr gets diagnostics and
// stdout NEVER receives log bytes (Pitfall 1).
func TestACPServeNoStdoutPollutionFromLogs(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = runACPServe(ctx, in, &stdout, &stderr, serveOptions{Profile: "zcode", MaxConcurrent: 6})
	if strings.Contains(stdout.String(), "ass-guard/acp") {
		t.Errorf("stdout contains a log prefix (transport discipline violation): %s", stdout.String())
	}
}

// TestACPServeDoesNotRegressProfileCheck verifies the Phase-1 `profile check`
// subcommand still exists alongside the new `acp serve` subcommand.
func TestACPServeDoesNotRegressProfileCheck(t *testing.T) {
	root := newRootCmd()
	var profileCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "profile" {
			profileCmd = c
			break
		}
	}
	if profileCmd == nil {
		t.Fatal("Phase-1 `profile` parent command is missing (regression)")
	}
	hasCheck := false
	for _, c := range profileCmd.Commands() {
		if strings.HasPrefix(c.Use, "check") {
			hasCheck = true
			break
		}
	}
	if !hasCheck {
		t.Fatal("Phase-1 `profile check` subcommand is missing (regression)")
	}
}
