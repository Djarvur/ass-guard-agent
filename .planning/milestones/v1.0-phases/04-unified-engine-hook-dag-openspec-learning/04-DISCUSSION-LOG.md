# Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.

**Date:** 2026-08-12
**Phase:** 4-Unified Engine + Hook-DAG + OpenSpec + Learning
**Areas discussed:** Unified engine decision logic, Hook-DAG structure & seeded set, OpenSpec adapter & pattern matching, Learning mode & memory
**External reference:** https://github.com/justxor/Claudecourse (user-provided; architectural patterns for hooks, auto-continue, concurrency)

---

## Area selection

All four areas selected. User also requested consulting the Claudecourse repo.

---

## Unified engine decision logic

### Engine design (user declined — Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Post-turn observer (rec.) | Engine decides after turn completes; never in critical path (ENG-04) | ✓ (Claude's discretion) |
| Interleaved with turn loop | Checks after each tool-call; more responsive but in critical path | |
| Hybrid: post-turn decide + mid-turn hooks | Continue decision post-turn; hooks at tool-call boundaries | |

**Notes:** POST-TURN OBSERVER directly follows from ENG-04.

### Dual-signal detection (user declined — Claude's discretion)

| Option | Description | Selected |
|--------|-------------|----------|
| Pattern table + handoff tools (rec.) | Two independent signals, either triggers continue; declarative tables | ✓ (Claude's discretion) |
| Text-pattern only | Simpler but misses tool-call handoffs | |
| Handoff tools only | Misses text handoffs | |

**Notes:** Directly follows from ENG-01 + OPEN-02.

---

## Hook-DAG structure & seeded set

### Hook config format

| Option | Description | Selected |
|--------|-------------|----------|
| Declarative YAML (rec.) | Matches Phase 3 scheduling.yaml; config-not-code convention | ✓ |
| Go plugin interface | Type-safe but violates config-not-code; requires rebuild | |

**User's choice:** Declarative YAML.

---

## OpenSpec adapter & pattern matching

### Hosting model

| Option | Description | Selected |
|--------|-------------|----------|
| External CLI subprocess (rec.) | exec.Command; unmodified toolkit; matches ROADMAP goal | ✓ |
| Go library link | Tighter integration but couples to internals; violates 'unmodified' | |

**User's choice:** External CLI subprocess.

---

## Learning mode & memory

### Learned settings store

| Option | Description | Selected |
|--------|-------------|----------|
| Versioned file under .ass-guard/ (rec.) | Simple, git-trackable, revertible | ✓ |
| SQLite database | Richer querying but heavier infrastructure | |

**User's choice:** Versioned file under .ass-guard/.

---

## Claude's Discretion

D-01 through D-05 (engine design: post-turn observer, dual-signal, structural safety, graceful degradation, provenance) — user declined follow-up questions on engine design. Each directly follows from the ENG requirement it implements.

D-07 through D-12, D-21, D-22 — follow directly from HOOK/TOOL requirements or STACK Focus 4. Not gray areas.

## Deferred Ideas

None raised. Additional toolkit adapters (GSD, spec-kit, BMad) explicitly v2.

---

*Phase: 4-Unified Engine + Hook-DAG + OpenSpec + Learning*
*Discussion date: 2026-08-12*
