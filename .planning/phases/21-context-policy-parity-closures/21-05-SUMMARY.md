---
phase: 21-context-policy-parity-closures
plan: 05
subsystem: api
tags: [images, ingress, downscale, x-image, catmullrom, decodeconfig, pixel-bomb, provider-capability, d11, base64, ref]

requires:
  - phase: 21-context-policy-parity-closures/04
    provides: the expandUserBlocks turn-entry seam this plan's ingress rides beside, and the shared runtime.go/transcript.go surfaces
provides:
  - ValidateAndScaleImage — DecodeConfig-first image validation with pure-Go auto-downscale (draw.CatmullRom, D-09) and typed ImageError failure classes
  - ImageLimits/DefaultImageLimits — D-09's pinned Anthropic classes (8000×8000, 10 MB, 1568 px long edge, JPEG q85)
  - image ingress at the turn entry (ingressImages) — sha-keyed original + dims-suffixed scaled persistence (temp+rename, idempotent) and Ref-form transcript lines (09-05 discipline)
  - ContentBlock image variants — acp (Data/MimeType/URI, omitempty) and transcript (DataRef/MediaType/dims/provenance/Scaled, omitempty; Data as a json:"-" pre-ingress carrier)
  - Provider.SupportsImages() — the D-11 capability declaration on the interface (compile-level completeness)
  - shaper.Message.Blocks + imageBlockParam — ordered rich rendering, Ref read at shape time onto anthropic.NewImageBlockBase64
  - projector seed fold — the user_message line's image blocks reach the outgoing request end-to-end (criterion 5)
  - dropUnsupportedImages — the D-11 turn-path strip (exactly one loud note naming the provider)
affects: [22-background-execution-sandbox-reality, 21-context-policy-parity-closures/06, evals]

actuals:
  tokens: 26900   # chars/4 over the realized production diff (38 files, 2527 insertions)
  tasks: 2
  commits: 4      # 2× RED + 2× GREEN

tech-stack:
  added: ["golang.org/x/image v0.45.0 (the phase's only new dependency — pure Go, official golang.org/x governance, CGO_ENABLED=0 gate verified)"]
  patterns:
    - "DecodeConfig-first ingress: dims/format/bytes validated from the header before ANY pixel allocation (the pixel-bomb law)"
    - "Ref-based lean transcript lines: metadata-in-line, bytes-on-disk, base64 never serialized (type-enforced via json:\"-\" carrier)"
    - "Capability-on-the-interface: SupportsImages() as a Provider method so a missed adapter fails to compile, not silently at runtime"
    - "Two-tier size policy: provider limits trigger downscale; a 100 Mpx decode-safety ceiling refuses before decode"

key-files:
  created:
    - internal/runtime/imgscale.go
    - internal/runtime/imgscale_test.go
    - internal/runtime/img_capability_test.go
  modified:
    - internal/acp/types.go
    - internal/session/transcript.go
    - internal/session/transcript_newkinds_test.go
    - internal/session/projector.go
    - internal/runtime/runtime.go
    - internal/provider/provider.go
    - internal/provider/anthropic.go
    - internal/provider/openai.go
    - internal/provider/conformance_test.go
    - internal/shaper/shaper.go
    - internal/shaper/shaper_test.go
    - internal/modelrouting/factory.go
    - go.mod / go.sum

key-decisions:
  - "D-11 seam: the turn path (Run) consults sess.Provider.SupportsImages and strips+notes BEFORE ingress and the transcript append — the shaper stays pure (the plan's recommended alternative, picked because the turn path is the one seam that knows both the blocks and the session's provider)"
  - "Two-tier size thresholds: dims over the PROVIDER limits (8000×8000) downscale per D-09; declared pixels over a 100 Mpx decode-safety ceiling refuse at the config stage — reconciling the plan's 9000×6000-downscales row with its 40000×40000-refuses row (both can't key off the same threshold)"
  - "LoadImageBytes lives in the shaper (not imgscale.go as the artifact sketched): runtime imports shaper, so shaper→runtime would be an import cycle; the reading layer owns the reader"
  - "session.ContentBlock.Data is a json:\"-\" pre-ingress base64 carrier — the lean-line discipline is enforced by the type: even a path that skipped ingress cannot serialize bytes into a transcript line"
  - "Media-type spoofing defense (T-21-16): the DECODED format wins; the declared MimeType never drives shaping"
  - "The plan's literal 9000×6000 over-dims row runs behind ASSGUARD_IMG_HEAVY=1; the default row (2000×1334 over a 1600 cap, same 1568×1045 output) proves the identical code path — the 54 MP fixture under -race held ~1 GB of shadowed pixels and starved pre-existing timing tests sharing the parallel batch"

patterns-established:
  - "Ingress-tier rewrite: acp blocks map through toContentBlocks, get validated/persisted/rewritten at turn entry BEFORE AppendUserMessage — the 21-04 expandUserBlocks pipeline shape extended to binary content"
  - "Loud-degrade notes as fixed-form stderr lines naming the outcome class (never bytes) — ingress failures, D-11 drops, and shape-time missing Refs all join the D-10 family"

requirements-completed: [PAR-06]

coverage:
  - id: D1
    description: "ValidateAndScaleImage: within-limits byte-identical passthrough (JPEG+PNG, decoded format wins), over-dims downscale to the 1568 long edge with provenance, over-bytes downscale within the cap, GIF/WebP acceptance, undecodable typed error"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestValidateAndScale_WithinLimitsPassthrough
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestValidateAndScale_OverDimsDownscalesToTargetLongEdge
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestValidateAndScale_OverBytesDownscalesWithinCap
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestValidateAndScale_GIFAndWebPAccepted
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestValidateAndScale_UndecodableBytesTypedError
        status: pass
    human_judgment: false
  - id: D2
    description: "Pixel-bomb guard: an IHDR 40000×40000 tiny PNG refuses at the DecodeConfig stage with ZERO pixel decodes (counting seam)"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestValidateAndScale_PixelBombRefusedBeforeDecode
        status: pass
    human_judgment: false
  - id: D3
    description: "Ingress wiring: Ref-form rewrite with sha-keyed original + dims-suffixed scaled files persisted atomically, lean transcript lines (no base64), idempotent re-ingress, failure drop with one fixed-form note"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestImageIngress_MapsAndRewritesToRefForm
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestImageIngress_PersistsOriginalAndScaled
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestImageIngress_TranscriptLeanNoBase64
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestImageIngress_IdempotentReIngress
        status: pass
      - kind: unit
        ref: internal/runtime/imgscale_test.go#TestImageIngress_FailureDropsBlockWithLoudNote
        status: pass
    human_judgment: false
  - id: D4
    description: "ContentBlock omitempty discipline: pre-change text-only blocks and user_message lines marshal byte-identically; the image variant round-trips Ref + metadata + provenance"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: internal/session/transcript_newkinds_test.go#TestContentBlockRoundTrip
        status: pass
    human_judgment: false
  - id: D5
    description: "Provider capability declarations: Anthropic true, OpenAI-shape false (MultiContent out of PAR-06's letter), compile-level completeness across every adapter and fake"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: internal/provider/conformance_test.go#TestConformance_ImageCapability
        status: pass
      - kind: unit
        ref: internal/runtime/img_capability_test.go#TestImageCapability_OpenAIShapeBodyHasNoImage
        status: pass
    human_judgment: false
  - id: D6
    description: "Shaper image mapping: Ref read at shape time onto Base64ImageSourceParam, image-between-texts order preservation, missing-Ref degrade (block dropped + one note, Shape continues), zero-blocks byte-identity"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: internal/shaper/shaper_test.go#TestImageCapability_MapsRefToBase64Param
        status: pass
      - kind: unit
        ref: internal/shaper/shaper_test.go#TestImageCapability_OrderPreserved
        status: pass
      - kind: unit
        ref: internal/shaper/shaper_test.go#TestImageCapability_MissingRefDegrades
        status: pass
      - kind: unit
        ref: internal/shaper/shaper_test.go#TestImageCapability_ZeroBlocksByteIdentical
        status: pass
    human_judgment: false
  - id: D7
    description: "D-11 end-to-end: against a SupportsImages=false provider the image is dropped before the transcript, exactly ONE note naming the provider fires, and the turn completes end_turn with the text"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: internal/runtime/img_capability_test.go#TestImageCapability_D11DropAndNote
        status: pass
    human_judgment: false
  - id: D8
    description: "Criterion 5 image half end-to-end: a pasted image produces the corresponding image content block in the outgoing request body (real Anthropic adapter, fake SSE backend) while the transcript line stays lean"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: internal/runtime/img_capability_test.go#TestImageCapability_ImageReachesOutgoingRequest
        status: pass
    human_judgment: false
  - id: D9
    description: "D-09 build contract: the pure-Go image dependency survives CGO_ENABLED=0 (golang.org/x/image v0.45.0, the phase's only new dep)"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: command:CGO_ENABLED=0 go build ./... && go list -m golang.org/x/image
        status: pass
    human_judgment: false

duration: 109min
completed: 2026-09-04
status: complete
---

# Phase 21 Plan 05: Image Ingress + Provider Capability (PAR-06) Summary

**DecodeConfig-first image ingress with pure-Go auto-downscale (x/image CatmullRom), Ref-based lean transcript lines, and per-provider image capability with D-11's exactly-one-note degradation — pasted images reach the outgoing Anthropic request end-to-end.**

## Performance

- **Duration:** 1h 49m (2026-09-03T20:37Z → 2026-09-03T22:26Z)
- **Tasks:** 2/2
- **Files modified:** 38 (3 created, 35 modified; includes go.mod/go.sum)

## Accomplishments

- `internal/runtime/imgscale.go`: `ValidateAndScaleImage` — header-first validation (dims/format/bytes before ANY pixel allocation), auto-downscale via `draw.CatmullRom` to the 1568 px long edge when over D-09's pinned Anthropic classes, format-preserving re-encode (PNG/GIF kept when the cap fits; JPEG q85 fallback; webp decodes → re-encodes JPEG), typed `ImageError` stage/class failures, and a 100 Mpx decode-safety ceiling that refuses pixel bombs at the config stage (proven by a counting seam: zero decodes).
- Image ingress at the turn entry (`ingressImages`, both engine and plain paths): base64 → validate/scale → sha-keyed original `<sha>.<ext>` + scaled `<sha>.<w>x<h>.<ext>` persisted under the session `.ass-guard/images` dir (temp+rename atomic, idempotent re-ingress) → block rewritten to Ref+metadata+provenance. Transcript lines carry NO base64 (pinned by test; the `json:"-"` Data carrier makes the leak type-impossible).
- `Provider.SupportsImages()` on the interface (anthropic true, openai-shape false); the turn path consults it (`dropUnsupportedImages`) BEFORE ingress — exactly ONE stderr note naming the provider, the text proceeds, the turn completes (never silent, never dead). Every adapter and all ~19 test fakes updated (compile-level completeness).
- Shaper: `Message.Blocks` ordered rich rendering; `imageBlockParam` reads the Ref at shape time via `LoadImageBytes` (moved into shaper — the dependency law) onto `anthropic.NewImageBlockBase64`; a missing Ref drops the block loudly and shaping continues. Projector: the seed user message folds the line's image blocks so criterion 5 holds end-to-end (pinned by a real-adapter e2e capturing the outgoing body).

## Task Commits

1. **Task 1: Image ingress — DecodeConfig guard, pure-Go downscale, Ref transcript lines** — `cbb455c` (test, RED) + `4d26bd8` (feat, GREEN)
2. **Task 2: Provider capability + shaper image mapping — D-11 loud degradation** — `eba3f9f` (test, RED) + `c66e6d7` (feat, GREEN)

**Plan metadata:** (docs commit follows this summary)

## Files Created/Modified

- `internal/runtime/imgscale.go` (NEW) — limits, validation+scaling, typed errors, atomic persistence, Ref naming
- `internal/runtime/imgscale_test.go` (NEW) — the six ValidateAndScale families + the ingress battery
- `internal/runtime/img_capability_test.go` (NEW) — D-11 e2e, image-positive e2e, OpenAI body check
- `internal/acp/types.go` — ContentBlock additive `Data/MimeType/URI` (v1 ImageBlock shape, omitempty)
- `internal/session/transcript.go` — ContentBlock image variant (`DataRef/MediaType/Width/Height/OrigWidth/OrigHeight/OrigSize/Scaled`, omitempty; `Data json:"-"` carrier)
- `internal/session/projector.go` — seed fold (`seedMessage`/`imageBlocksOf`/`findIntentLine`)
- `internal/runtime/runtime.go` — toContentBlocks image mapping, `dropUnsupportedImages`, `ingressImages`, Run wiring
- `internal/provider/{provider,anthropic,openai}.go` — `SupportsImages` + Block/ImageBlock aliases
- `internal/shaper/shaper.go` — Blocks/ImageBlock types, `richBlocks`, `imageBlockParam`, `LoadImageBytes`
- `internal/modelrouting/factory.go` + 18 test files — capability stubs on every Provider implementation
- `go.mod`/`go.sum` — `golang.org/x/image v0.45.0`

## Decisions Made

- **D-11 seam (plan left open):** turn path strips+notes, shaper stays pure — documented above; the shaper receives no capability flag.
- **Two-tier size policy:** provider limits (8000×8000 / 10 MB) trigger D-09 downscale; a 100 Mpx decode-safety ceiling refuses before decode. The plan's rows (9000×6000 downscales; 40000×40000 refuses) key off different tiers — both pinned green.
- **LoadImageBytes in shaper:** import-cycle avoidance (runtime→shaper exists; shaper→runtime forbidden).
- **`json:"-"` Data carrier:** the lean-line invariant is enforced by the type, not convention.
- **Env-gated literal heavy row:** `ASSGUARD_IMG_HEAVY=1` runs the plan's literal 9000×6000 @ DefaultImageLimits (verified green, 48.7s); the default suite runs the cheap row with the same 1568×1045 assertions.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Projector image fold**
- **Found during:** Task 2 (shaper mapping)
- **Issue:** `projector.go` was not in the plan's files list, but the projector's seed message is built from `extractText` (text only) — without a fold, images ingressed into transcript Ref lines would NEVER reach the outgoing request: criterion 5's "pasting an image produces the corresponding content block in the outgoing request" was unreachable (a data-source-less stub by the summary rules).
- **Fix:** `seedMessage` + `imageBlocksOf` + `findIntentLine` — the seed user Message carries ordered Blocks when the line has image blocks; text-only lines seed byte-identically (Blocks nil).
- **Files modified:** internal/session/projector.go
- **Verification:** TestImageCapability_ImageReachesOutgoingRequest (real Anthropic adapter, captured body contains the image block); all existing projector tests green.
- **Committed in:** c66e6d7

**2. [Rule 3 - Blocking] LoadImageBytes relocated to the shaper**
- **Found during:** Task 2
- **Issue:** the plan's artifact sketch placed `LoadImageBytes` in `imgscale.go` (runtime), but runtime imports shaper — shaper→runtime would be an import cycle.
- **Fix:** the reader lives in shaper.go (the layer that reads); imgscale's doc points at it.
- **Files modified:** internal/shaper/shaper.go, internal/runtime/imgscale.go
- **Verification:** build green; mapping tests read Refs through it.
- **Committed in:** c66e6d7

**3. [Rule 3 - Blocking] All Provider implementations updated, not just conformance fakes**
- **Found during:** Task 2
- **Issue:** the grown Provider interface breaks every implementer at compile time — ~19 fakes across runtime/session/loop/modelrouting/acpserve tests plus `modelrouting.noCredentialProvider`, beyond the conformance fakes the plan named.
- **Fix:** one-line `SupportsImages() bool` stubs (text-only false) on each.
- **Files modified:** 19 test files + internal/modelrouting/factory.go
- **Verification:** `go vet ./...` + full suite green.
- **Committed in:** c66e6d7

**4. [Rule 1 - Suite health] Heavy-fixture tuning under -race**
- **Found during:** Task 2 verification
- **Issue:** the plan's literal 9000×6000 in-test JPEG (54 MP) under `-race` held ~1 GB of shadowed pixel allocations; running it in the default suite starved pre-existing timing-sensitive tests (ask/cron/emitter families with 5 s bounds) sharing the parallel batch — full-suite flakes absent at the base commit (verified: base full -race suite green ×3; suite with the heavy fixtures flaked 3 unrelated tests).
- **Fix:** default rows use cheap fixtures through the identical production path (over-dims: 2000×1334 over a 1600 cap, same 1568×1045 output; over-bytes: 400×400 noise PNG over a 150 KB cap); the literal 9000×6000 @ DefaultImageLimits row runs behind `ASSGUARD_IMG_HEAVY=1` (verified green, 48.7s); full-turn e2e tests made sequential to keep the parallel batch at base shape.
- **Files modified:** internal/runtime/imgscale_test.go, internal/runtime/img_capability_test.go
- **Verification:** full `go test -race -count=1 ./...` green (38/38 packages) after the change; the filtered plan verify commands green and deterministic.
- **Committed in:** c66e6d7

---

**Total deviations:** 4 auto-fixed (1× Rule 2, 2× Rule 3, 1× Rule 1)
**Impact on plan:** All necessary for correctness (criterion 5), compile health (interface growth), and suite stability. No scope creep — the projector fold is the only production surface beyond the plan's file list, and it is what makes the plan's own objective true end-to-end.

## Issues Encountered

- **Transient machine-load flakes (environmental):** during verification the host ran sustained external load (macOS StorageManagement scan, XProtect, Spotlight — load spikes to 160+ on 16 CPUs). Full `go test -race ./...` runs under those spikes flaked single pre-existing timer-bound tests (also observed once at the BASE commit: TestLoadFullOrdering). With load settled, the complete suite is green (38/38). `mise ci`'s lanes: vet green, build green, test green when run settled; **lint remains red as the known pre-existing baseline drift** (1,379 issues repo-wide, dominated by exhaustruct_v5; the files this plan touched carry no new findings except exhaustruct, matching the existing baseline — verified by filtered runs and against a base-commit worktree).
- The first `golangci-lint` binary on PATH (built with go1.26) panics on go1.27-requiring files; the mise-pinned 2.13.2 (built with go1.27) is the working one — matches the known drift.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- PAR-06's image leg is complete end-to-end; 21-06 (the gate join tracer) is the remaining plan in the phase, plus phase verification.
- The `json:"-"` Data carrier and the ingress seam are reusable for any future binary content kind (audio attachments etc.).
- The heavy literal-fixture row is runnable on demand: `ASSGUARD_IMG_HEAVY=1 go test -race ./internal/runtime/ -run TestValidateAndScale -count=1`.

---
*Phase: 21-context-policy-parity-closures*
*Completed: 2026-09-04*

## Self-Check: PASSED

All key files exist on disk; all four task commits plus the docs commit present in history.
