# opsx-e2e-matrix — the Phase-13 expanded-matrix capture corpus

D-03 pass-1 raw material (13-01) + the committed cross-plan contract
(13-02's harvest + 13-03's dead-end scan read these files from testdata —
/tmp evidence alone does not discharge it).

Contents:
- `<command>-happy-capture.txt` × 6 — each expanded command's stage closing
  output from a green happy leg (new/continue/ff/verify/bulk-archive/onboard).
- `<command>-fixable-capture.txt` × 6 — each command's failure closing +
  recovery closing from a green fixable leg (D-01's 2-path matrix; onboard =
  the D-08 idempotent re-run).

Every capture carries the provenance header: capture date (RFC3339), the
openspec binary version, and the model ("real binary + real model" — creds
from the repo's .ass-guard/config.yaml).

The legs are DOUBLE-gated (`ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1`); CI
never runs them. The operator's global openspec config is guarded
byte-exactly around every run (GuardOpenSpecGlobalConfig + the in-runner
pre/post byte-compare — restore discipline is measured, never assumed).
