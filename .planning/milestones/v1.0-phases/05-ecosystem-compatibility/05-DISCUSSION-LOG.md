# Phase 5: Ecosystem Compatibility - Discussion Log

**Date:** 2026-08-12
**Phase:** 5-Ecosystem Compatibility
**Areas discussed:** MCP hosting & tool bridging, Skills/commands/plugins + namespacing

---

## MCP hosting & tool bridging

### Tool catalog merge

| Option | Description | Selected |
|--------|-------------|----------|
| Built-in in profile + dynamic MCP at session start (rec.) | Profile declares stable core; MCP discovered dynamically per session | ✓ |
| All tools in profile (static) | Brittle — MCP reconfig = profile drift | |

**User's choice:** Built-in + dynamic MCP.

---

## Skills/commands/plugins + namespacing

### Namespace location

| Option | Description | Selected |
|--------|-------------|----------|
| .claude/ass-guard/ namespaced subdir (rec.) | Same dir, namespaced, no clobber | |
| Separate .ass-guard/ directory | Complete separation from .claude/ | ✓ |

**User's choice:** Separate `.ass-guard/` directory.

**Notes:** Consistent with Phase 2 D-06 (transcripts under .ass-guard/). All ass-guard state in one place. .claude/ is read-only from ass-guard's perspective.

---

## Claude's Discretion

D-02/D-03/D-04/D-07 follow directly from ECOS requirements + STACK.

---

*Phase: 5-Ecosystem Compatibility*
*Discussion date: 2026-08-12*
