---
phase: 15
slug: 15-internal-runtime-carve-step-0
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: 2026-08-26
---

# Phase 15 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

Phase 15 is a mechanical verbatim relocation (sessionTurnRunner → internal/runtime
+ CLI-support package extraction). Zero behavior change by contract; the threat
register's mitigate entries are equivalence proofs, not new controls.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| editor→binary argv | Zed spawns `ass-guard acp serve`; the flag surface IS the trust contract | argv strings (flags, paths) |
| config-file→provider credentials | loadModelRoutingFactory stat-gates config paths carrying api_key material | file paths, api_key references |
| argv→restore operation | runCheckpointRestore mutates the user's working tree from a CLI id argument | checkpoint id, working tree |
| zcode binary exec | zcodeInstalledVersion execs an installed binary by fixed argv | fixed argv, version stdout |
| corpus fixture input | placementCheck reads operator-supplied cache-pin JSONL | JSONL fixtures |
| stdio frames | stdout carries only ACP frames — any diagnostic leak corrupts the editor channel | ACP JSON frames |
| schedule store | sched.Open reads operator workdir state | schedule JSON |
| editor→runner (acp.TurnRunner) | Zed-driven prompts enter Run; the interface contract must survive the rename untouched | prompt/session content |
| engine↔session seams | enginebridge adapters reach session/engine internals through func-value crossings | closures over runner behavior |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-15-01 | Tampering | cobra flag/command surface | medium | mitigate | cmd/ass-guard/cli_contract_test.go golden-pins every command + flag (3 test functions, green) | closed |
| T-15-02 | Information Disclosure | WarnLooseConfigPerm (0600 advice) | medium | mitigate | Verbatim move verified byte-identical; perm-warning test relocated to internal/providerfactory/provider_factory_test.go and green | closed |
| T-15-03 | Repudiation | setupModelRouting degradation logging | low | mitigate | Graceful-degradation log shape preserved byte-identical; embedded-default fallback covered by relocated suite | closed |
| T-15-04 | Tampering | RunCheckpointRestore arg handling | medium | mitigate | Verbatim move to internal/checkpointcmd; wrapping, stderr notes, store semantics byte-identical; failure paths covered | closed |
| T-15-05 | Repudiation | learning table output | low | accept | Sanctioned stdout exception (SP-4) preserved as-is; format pinned by existing tests | closed |
| T-15-06 | Tampering | zcodeInstalledVersion exec argv | low | mitigate | Fixed-argv exec preserved verbatim incl. T-14-11/T-14-12 comment block; seam pinned by relocated suite | closed |
| T-15-07 | Information Disclosure | parity footer stderr output | low | accept | Report-only output discipline unchanged; never feeds exit code | closed |
| T-15-08 | Information Disclosure | stdout transport discipline | high | mitigate | WiresStdoutClean + NoStdoutPollutionFromLogs relocated with acpserve (internal/acpserve/serve_test.go, 4 occurrences) and green | closed |
| T-15-09 | Repudiation | startAuditMirror + bodyStore ordering | medium | mitigate | Statement order pinned in acpserve.Run; TestServeAudit_*/TestServeMirror_Override cover from new home | closed |
| T-15-10 | DoS | schedule store open failure | low | mitigate | Degrade-loudly branch preserved byte-identical in acpserve.Run | closed |
| T-15-11 | Tampering | acp.TurnRunner/SessionCloser contract | high | mitigate | Signatures byte-identical post-rename; WithTurnRunner plugs back identically; full -race suite green | closed |
| T-15-12 | Elevation of Privilege | enginebridge mirror-config divergence | medium | mitigate | Mirror struct (BridgeConfig) filled from ONE source in acpserve.Run; acceptance greps proved no unexported member crosses; divergence accepted deliberately per D-14 | closed |
| T-15-13 | DoS | scheduler loop lifecycle | medium | mitigate | startScheduler ctx-only lifecycle moved verbatim incl. ticker cleanup; cron_wiring_test.go relocated with the family and green | closed |
| T-15-14 | Information Disclosure | redactor exclusion surfaces | medium | mitigate | redactorAdapter moved verbatim beside sessionFor; transcript/redaction discipline untouched; redaction-carry tests ride along | closed |
| T-15-15 | Repudiation | phase-review attestation completeness | low | mitigate | phase-review.md records every gate with actual numbers; duplicate census proven against baseline | closed |
| T-15-SC | Tampering | npm/pip/cargo installs | high | accept | Zero package installs this phase (RESEARCH Standard Stack); supply-chain legitimacy gate not applicable — verified: phase diff touches only existing stdlib/local imports | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on (high) count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-15-01 | T-15-05 | Learning-table stdout is a sanctioned SP-4 exception predating the phase; format pinned by tests | Phase planning (15-03 threat model) | 2026-08-26 |
| AR-15-02 | T-15-07 | Parity footer stderr is report-only and never feeds exit code (locked prohibition rides the comment) | Phase planning (15-04 threat model) | 2026-08-26 |
| AR-15-03 | T-15-SC | No package installs occurred; supply-chain gate vacuously satisfied | Phase planning (all 7 plans) | 2026-08-26 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-08-26 | 16 | 16 | 0 | inline executor (sequential execution mode; ASVS L1 grep-depth per short-circuit rule) |

Classification method: register authored at plan time (all 7 PLANs carried
parseable threat_model blocks); each mitigate entry verified by direct
artifact check (test presence + green mise ci); threats_open = 0, ASVS L1 →
workflow short-circuit applied (no auditor spawn required).

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-08-26
