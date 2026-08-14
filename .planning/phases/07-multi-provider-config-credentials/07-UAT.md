---
phase: 07-multi-provider-config-credentials
status: complete
started: 2026-08-14
completed: 2026-08-14
total_tests: 1
passed: 1
failed: 0
---

# Phase 7 UAT — Multi-Provider Config & Credentials

Source: 07-VERIFICATION.md (status: human_needed — all automated gates pass; 1 operator-gated item)

## User-Flow Walk-Through

### Test 1: Live editor-spawned zero-env model turn (operator-gated, OPTIONAL per 07-02-PLAN surfaced assumption)
**Status:** PASS
**What happened:** Executed by the orchestrator emulating the editor role (same code path Zed uses — a parent process spawning `ass-guard acp serve` in the project working directory). `.ass-guard/scheduling.yaml` (perms 644) carried the operator's literal `api_key` under `providers.anthropic`; the child environment was scrubbed of every `*_API_KEY` var (zero-env condition). ACP v1 handshake over stdio: `initialize` → `protocolVersion=1`; `session/new` → non-empty `sessionId`; `session/prompt "Reply with OK"` → **real model turn succeeded** (`stopReason=end_turn`) against the configured base_url (`https://api.z.ai/api/anthropic`, merged from the embedded default). Since no env var existed, the credential resolved **from the config file** (`Source=config`) — proving an editor-spawned process authenticates with zero environment.
**Observed bonus (SC3):** the startup 0600-permission warning FIRED for the 644-perms config file (stderr only) — the credential-on-disk hygiene warning works live. The uncredentialed-provider warning correctly did NOT fire (provider is credentialed).
**No credential leakage:** stdout carried only 2 valid JSON-RPC frames; stderr contained no key material. (Note: `--audit-log` is not written on the `acp serve` path — LOG-01's redacted request log is tracer-wired; not a credential-hygiene failure.)

## Verdict

**1/1 PASS** — Phase 7 delivers its promised value. An operator-declared multi-provider config with inline credentials authenticates a real model turn from a zero-environment editor-spawned process, with credential precedence, 0600 hygiene warning, and no leakage — completing SC1 and complementing the 14 automated truths in 07-VERIFICATION.md.
