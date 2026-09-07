# Backlog 999.1 — Study ai-agent-book, extract useful patterns

**Captured:** 2026-09-07 (operator request: "изучить и извлечь полезное для нашего проекта")
**Source:** https://github.com/bojieli/ai-agent-book/
**What it is:** Open-source repo for Li Bojie's (李博杰) book 《深入理解 AI Agent：设计原理与工程实践》 ("Deep Understanding of AI Agents: Design Principles and Engineering Practice"). ~45k stars, Apache-2.0. Book text in `book/` (10 chapters), code in `chapter1/`–`chapter10/` (109 companion experiments), pinned external repos (SWE-bench, verl, GUI/robotics), slides + PDF/EPUB builds. **English translation available in `book-en/`** (original is Chinese — use the EN tree). Python 3.11–3.13, uv/pip with per-chapter extras.

## Chapter-to-surface mapping (what to mine, for ass-guard-agent)

| Book topic | Our surface | What to look for |
|---|---|---|
| Context engineering | Phase 19 compaction, Phase 21 context closures, Projector reset-points | Alternative context-window management strategies; how they treat summarization boundaries vs our typed-marker reset-point class |
| Memory / RAG | Transcript/session persistence, `.ass-guard/` store | Durable-memory patterns beyond JSONL transcripts |
| Tools & MCP | Tool catalog, MCP hosting (`modelcontextprotocol/go-sdk`) | Tool-design principles; anything on tool-catalog shaping for model behavior |
| Coding agents (chapter on them) | **We are one** (zcode parity runtime) | Directly applicable agent-loop patterns; compare with our carved runner seams |
| Multimodal / async interaction | ACP stdio + Telegram peer, Phase 23 steering/input queue | Transport-neutral steering patterns; async turn interleaving |
| Evaluation | Parity probes, `internal/parity/cacheprobe.go`, /gsd eval tooling | Eval design for "agent behaves identically to target" — closest thing to our mimicry bar, even post-pivot |
| Multi-agent collaboration | Phase 22 subagents, per-agent model routing (Phase 20) | Dispatch/coordination patterns; task-notification discipline |
| Post-training (SFT vs RL) | Out of scope for the agent itself | Skim only; maybe relevant if we ever tune a model for the runtime |

## How to run the study

1. Clone shallow (`--depth 1`), read `book-en/` chapter list first; the 109 experiments are the highest-value part — each is a small runnable pattern.
2. Filter by the mapping table above; skip robotics/GUI/benchmarks pins.
3. Output: a distilled `EXTRACT.md` here — patterns with citations (chapter + experiment path) and a verdict per pattern (adopt / adapt / reject-with-reason) against our architecture docs (`.planning/ARCHITECTURE.md`, `docs/compaction-decision.md`).
4. Promising items become backlog sub-items or feed the next milestone's discuss-phase.

**Promotion:** `/gsd-review-backlog` when ready to turn this into real work.
