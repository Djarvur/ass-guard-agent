# API Coverage — Phase 23 decision record

No external API integration: Phase 23 touches only the stdlib, the platform `git` subprocess (the pre-existing, documented dependency of `internal/checkpoint/store.go:53-56` — not a module or network API), and ass-guard's own internal Go surfaces (the new transport-neutral SteerQueue API consumed in-process, the checkpoint store, and the existing ACP wire machinery from Phase 16). No new modules are installed (RESEARCH.md `## Package Legitimacy Audit`: no new installs).

The detector's single signal ("the transport-neutral steering API's concrete interface shape") refers to ass-guard's own internal Go API produced BY this phase for the TG-02 consumer contract — not an external service being integrated, so no capability matrix is fabricated.
