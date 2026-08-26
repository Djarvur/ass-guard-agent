package main

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestACPServeCommandRegistered verifies the cobra root has an `acp serve`
// subcommand with the expected flags (transport discipline: stdout is framer-
// only; --profile default zcode; --max-concurrent default 6; --ask-timeout
// default 10m with the block-forever documentation, D-01).
func TestACPServeCommandRegistered(t *testing.T) {
	t.Parallel()

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
	if prof != profileZcode {
		t.Errorf("serve --profile default = %q; want zcode", prof)
	}

	mc, _ := pf.GetInt("max-concurrent")
	if mc != 6 {
		t.Errorf("serve --max-concurrent default = %d; want 6 (RESEARCH §11.1)", mc)
	}

	// The ask-timeout cobra knob (12-01 T2 Test 3, D-01 — the flag half of
	// TestAskWiring_ConfigKnob, which pins the runner threading in
	// internal/runtime): exists, default 10m, documents 0 = block forever.
	askFlag := pf.Lookup("ask-timeout")
	if askFlag == nil {
		t.Fatal("acp serve has no --ask-timeout flag")
	}

	if askFlag.DefValue != (10 * time.Minute).String() {
		t.Errorf("--ask-timeout default = %q; want the D-01 10m default", askFlag.DefValue)
	}

	if !strings.Contains(askFlag.Usage, "block forever") {
		t.Errorf("--ask-timeout usage = %q; want the 0 = block forever (interactive mode) documentation", askFlag.Usage)
	}
}

// TestACPServeDoesNotRegressProfileCheck verifies the Phase-1 `profile check`
// subcommand still exists alongside the new `acp serve` subcommand.
func TestACPServeDoesNotRegressProfileCheck(t *testing.T) {
	t.Parallel()

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
