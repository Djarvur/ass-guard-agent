//go:build !linux

package coreexec

import "syscall"

// setPdeathsig is the non-Linux no-op: Pdeathsig exists only on Linux
// SysProcAttr (go doc syscall.SysProcAttr — verified RESEARCH Standard
// Stack). Darwin keeps the CGO-free static-binary claim and the same call
// site; orphan protection there rides the ReapAll + escalation ladder.
func setPdeathsig(_ *syscall.SysProcAttr) {}
