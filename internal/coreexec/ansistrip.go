package coreexec

// StripANSI removes terminal escape sequences from PTY-captured output
// (22-04, PAR-09).
//
// RED-PHASE SCAFFOLDING: the passthrough body exists only so the Task 1
// table battery compiles and fails on its assertions; the feat commit of
// this plan replaces it with the real hand-rolled CSI/SGR-class stripper
// (the deliberately-NOT-a-dependency decision, 22-RESEARCH Standard Stack).
func StripANSI(s string) string { return s }
