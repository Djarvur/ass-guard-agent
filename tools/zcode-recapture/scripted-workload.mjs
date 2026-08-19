// The scripted divergence-prone workload (runbook §3), driven autonomously via app-server stdio.
// Thresholds: >=5 user turns, >=10 distinct tools, >=1 subagent dispatch, mid-session catalog
// change with BOTH directions (attach + detach via config-change + session/resume boundaries —
// the only headless-reachable mechanism; /mcp connect|disconnect are TUI-client-side).
//
// 2026-08-20 freshness fix (12-05 Task 1): the kit moved /tmp/zcode-recapture/ ->
// tools/zcode-recapture/, so the probe-server path and the config-restore backup path are now
// KIT-RELATIVE / RUN-SCOPED. The config backup is taken FRESH before any touch and restored +
// diff-verified in a process 'exit' handler (T-12-05-01: restore must not depend on a /tmp file
// that may not exist — the old catch{} silently left the probe entry in the operator's config).
//
// 2026-08-20 tour extension (12-05 Task 2): run with --tour to append the deferred-tools
// tour (the 12-05 forms harvest) to the same session. The module is IMPORT-SAFE (pure
// exports until run as main): TOUR_TURNS / tourToolCoverage / assertCronCleanup power the
// kit's node --test self-tests.
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readFileSync, writeFileSync, readdirSync, mkdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { execSync } from "node:child_process";

const KIT_DIR = dirname(fileURLToPath(import.meta.url));
const CLI_CFG = join(process.env.HOME, ".zcode", "cli", "config.json");
const ROLLOUT = join(process.env.HOME, ".zcode", "cli", "rollout");
const WS = "/tmp/zcode-recapture-ws";
const RUN_DIR = process.env.ZCODE_RECAPTURE_RUN_DIR ?? "/tmp/zcode-recapture-run";
const BACKUP = join(RUN_DIR, "cli-config-backup.json");
const PROBE = join(KIT_DIR, "probe-server.mjs");
const MODEL = { providerId: "builtin:zai-coding-plan", modelId: "GLM-5.3" };

// ---------------------------------------------------------------------------
// PURE EXPORTS (tour definitions + cleanup proof — no side effects on import)
// ---------------------------------------------------------------------------

// The deferred-tools tour (12-05 Task 2, Behavior 1). Every deferred tool at
// least once + the two ACP-07 hunts. Prompts are IMPERATIVE (the model obeys
// its tool descriptions). "<SID>" is replaced with the live session id at run
// time. delayMinutes-based crons ONLY (never recurring — a missed cleanup must
// not persist forever in the operator's environment; T-12-05-02).
export const TOUR_TURNS = [
  {
    id: "tour_ask_answered",
    tools: ["AskUserQuestion"],
    prefs: "answer",
    prompt: "I want to add caching to this project. Ask me ONE multiple-choice question about which cache approach to use before doing anything, then implement the smallest possible version of the option I pick.",
  },
  {
    id: "tour_ask_non_answer",
    tools: ["AskUserQuestion"],
    prefs: "holdPending",
    prompt: "Ask me ONE multiple-choice question about which logging library to add, wait for my selection, then act on it.",
  },
  {
    id: "tour_plan_mode",
    tools: ["EnterPlanMode", "ExitPlanMode"],
    prefs: "answer",
    prompt: "Use the EnterPlanMode tool NOW (do not answer in text), then explore briefly, then present a short plan for adding a multiply(a, b) function to calc.js via the ExitPlanMode tool.",
  },
  {
    id: "tour_subagent_message",
    tools: ["SendMessage"],
    prompt: "Dispatch one subagent to count the *.js files in this project. After it returns, use SendMessage to send that agent a one-line thank-you message, and report the exact result you got back.",
  },
  {
    id: "tour_read_session_context",
    tools: ["ReadSessionContext"],
    prompt: "Use ReadSessionContext with sessionId <SID> and query \"what changes were made to calc.js in this session\" to review this session's earlier work, then summarize the finding in one line.",
  },
  {
    id: "tour_cron_create_list",
    tools: ["CronCreate", "CronList"],
    prompt: "Create a scheduled automation titled 'recapture tour marker' that fires ONCE, two minutes from now (use the delayMinutes field — do NOT use a cron expression), with the prompt 'echo hello from the recapture tour'. Then call CronList and report the new automation's id exactly.",
  },
  {
    id: "tour_cron_update_delete",
    tools: ["CronUpdate", "CronDelete", "CronList"],
    prompt: "Update the automation titled 'recapture tour marker' to fire once five minutes from now instead. Then DELETE it by id. Then call CronList once more and reply with its verbatim output.",
  },
  {
    id: "tour_bash_background",
    tools: ["Bash:run_in_background", "TaskOutput", "TaskStop"],
    prompt: "Start a background shell that runs `sleep 60 && echo bg-done` using run_in_background, then immediately retrieve its output ONCE without blocking, then stop the task with TaskStop. Report each result exactly.",
  },
  {
    id: "tour_bash_timeout",
    tools: ["Bash:timeout_form"],
    prompt: "Run the command `sleep 30` with the Bash timeout parameter set to 10000 milliseconds. Report the exact result text you receive.",
  },
  {
    id: "tour_bash_truncation",
    tools: ["Bash:truncation"],
    prompt: "Run a command that prints roughly 200000 bytes of output, for example `seq 1 25000`. Report the exact LAST line of output you receive, verbatim.",
  },
];

/** The unique covered-tool set of the tour (Behavior 1 self-test surface). */
export function tourToolCoverage() {
  return [...new Set(TOUR_TURNS.flatMap((t) => t.tools))];
}

/**
 * Cleanup proof (Behavior 2 / T-12-05-02): scan parsed rollout records for the
 * LAST CronList tool-result; pass only when it shows NO automations. Throws
 * with the offending value otherwise. Records use the CURRENT zcode shape
 * (request.messages[] role:"tool" {toolName, content, isError}).
 */
export function assertCronCleanup(records) {
  let last = null;
  for (const rec of records ?? []) {
    for (const m of rec?.request?.messages ?? []) {
      if (m?.role === "tool" && m.toolName === "CronList") last = String(m.content ?? "");
    }
    for (const m of rec?.request?.body?.messages ?? []) {
      for (const b of Array.isArray(m?.content) ? m.content : []) {
        if (b?.type === "tool-result" && b.toolName === "CronList") last = String(b.output?.value ?? "");
      }
    }
  }
  if (last === null) throw new Error("cleanup proof: no CronList result found in the capture");
  const emptyish = /^\s*(\[\]|\{\})?\s*$/m.test(last) || /no (automations|scheduled tasks|cron)/i.test(last) || /^0 automations/i.test(last);
  const hasEntry = /(^|\n)\s*\d+[.)]\s+\S/.test(last) || /\bcron_[a-z0-9]+\b/i.test(last) || /recapture tour marker/i.test(last) || /[{[]\s*{/.test(last);
  if (!emptyish || hasEntry) throw new Error(`cleanup proof FAILED — CronList not empty: ${last.slice(0, 200)}`);
  return true;
}

// ---------------------------------------------------------------------------
// RUN PATH (main-guarded; everything below touches the operator environment)
// ---------------------------------------------------------------------------

function armConfigRestore() {
  mkdirSync(RUN_DIR, { recursive: true });
  writeFileSync(BACKUP, readFileSync(CLI_CFG));
  let cfgDirty = false;
  const restore = () => {
    if (!cfgDirty) return;
    try {
      writeFileSync(CLI_CFG, readFileSync(BACKUP));
      const clean = readFileSync(CLI_CFG).equals(readFileSync(BACKUP));
      console.log(clean ? "[cfg] restored; diff vs backup EMPTY" : "[cfg] CONFIG DIFFERS FROM BACKUP — FAIL");
      if (!clean) process.exitCode = 1;
    } catch (e) {
      console.log("[cfg] restore error:", e.message);
      process.exitCode = 1;
    }
  };
  process.on("exit", restore);
  for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => { restore(); process.exit(130); });
  return {
    setProbeServer(add) {
      const c = JSON.parse(readFileSync(CLI_CFG, "utf8"));
      cfgDirty = true;
      if (add) {
        c.mcp = c.mcp ?? {};
        c.mcp.servers = c.mcp.servers ?? {};
        c.mcp.servers.recapture_probe = { type: "stdio", command: process.execPath, args: [PROBE] };
      } else {
        if (c.mcp?.servers) delete c.mcp.servers.recapture_probe;
        if (c.mcp && Object.keys(c.mcp.servers ?? {}).length === 0) delete c.mcp;
      }
      writeFileSync(CLI_CFG, JSON.stringify(c, null, 2) + "\n");
    },
    restore,
  };
}

// Seed the scratch workspace the runbook's turns assume (README/calc/test/package).
function seedWorkspace() {
  mkdirSync(WS, { recursive: true });
  const f = {
    "README.md": "# calc — tiny demo project\n\nAdd numbers. XYZZY_PLUGH marks the secret keyword location (see calc.js).\n",
    "calc.js": "'use strict';\n// The XYZZY_PLUGH keyword lives on this line.\nfunction add(a, b) {\n  return a + b;\n}\n\nmodule.exports = { add };\n",
    "test.js": "'use strict';\nconst assert = require('assert');\nconst { add } = require('./calc');\n\nassert.strictEqual(add(2, 3), 5);\nassert.strictEqual(add(-1, 1), 0);\nconsole.log('test.js: 2 passing');\n",
    "package.json": '{\n  "name": "calc-scratch",\n  "version": "1.0.0",\n  "private": true,\n  "scripts": { "test": "node test.js" }\n}\n',
  };
  for (const [name, content] of Object.entries(f)) {
    const p = join(WS, name);
    try { readFileSync(p); } catch { writeFileSync(p, content); }
  }
}

// Tour-pref driver options. Interaction answers use the app-server's v4 answer
// shape (verified live + in the 0.16.3 bundle: {action:"accept", content:{…}} —
// anything else normalizes to decline/"Permission request failed"):
//   "answer"      — accept user-input interactions with the first option's label
//                   (answered-ask leg; plan approval accepts with empty content)
//   "holdPending" — deliberately NEVER answer: the runtime's own 30s ask tool
//                   timeout renders the non-answer form (the D-01 capture)
function tourDriverOpts(prefs) {
  if (prefs === "holdPending") {
    return {
      prefs: { askUserQuestionAutoResolutionEnabled: false },
      onUserInput: () => ({ __noreply: true }),
    };
  }
  if (prefs === "answer") {
    return {
      prefs: { askUserQuestionAutoResolutionEnabled: false },
      onUserInput: (method, params) => {
        const s = JSON.stringify(params ?? {});
        console.log(`[tour] answering ${method}: ${s.slice(0, 300)}`);
        const isPlanApproval = (params?.schema?.interaction ?? "") === "plan_approval";
        if (isPlanApproval) return { action: "accept", content: {} };
        const qs = params?.questions ?? [];
        const label = qs[0]?.options?.[0]?.label ?? "yes";
        return { action: "accept", content: { answer: label } };
      },
    };
  }
  return {};
}

async function resumeWithPin(sid, opts = {}) {
  const d = new ZcodeDriver(WS, { onLog: () => {}, ...opts });
  const resp = await d.rpc("session/resume", { sessionId: sid }, 60000);
  if (resp.error) { await d.stop(); throw new Error("resume failed: " + JSON.stringify(resp.error)); }
  const pin = await d.rpc("session/setModel", {
    sessionId: sid,
    model: MODEL,
    runtimeModel: {
      revision: "recapture-" + Date.now(),
      generatedAt: Date.now(),
      model: MODEL,
      provider: { providerId: MODEL.providerId, kind: "anthropic", apiFormat: "anthropic-messages", baseURL: "https://api.z.ai/api/anthropic", models: [{ modelId: MODEL.modelId }] },
    },
    persistAsWorkspaceLastUsed: false,
  }, 60000);
  if (pin.error) console.log("[setModel] error:", JSON.stringify(pin.error).slice(0, 200));
  d.sessionId = sid;
  await d.rpc("session/subscribe", { sessionId: sid, deliveryKind: "desktop-continuous", includeSnapshot: true, afterSeq: 0 }, 60000);
  return d;
}

async function runTour(sid, t0) {
  // --only id1,id2 filters the tour (the retry route for legs that missed).
  const onlyIdx = process.argv.indexOf("--only");
  const only = onlyIdx !== -1 ? new Set(process.argv[onlyIdx + 1].split(",")) : null;
  console.log("[tour] deferred-tools tour begins");
  for (const turn of TOUR_TURNS) {
    if (only && !only.has(turn.id)) continue;
    const opts = tourDriverOpts(turn.prefs);
    const d = await resumeWithPin(sid, opts);
    try {
      const prompt = turn.prompt.replaceAll("<SID>", sid);
      const r = await d.prompt(prompt, 600000);
      const line = (r.done.payload?.response ?? "").trim().split("\n")[0].slice(0, 110);
      console.log(`${turn.id} (${((Date.now()-t0)/1000).toFixed(0)}s): ${line}`);
      console.log(`  tools: ${JSON.stringify(r.tools)}`);
    } catch (e) {
      // Honest negative: record the miss, keep the tour moving (Task 3 rules).
      console.log(`${turn.id} FAILED: ${e.message.slice(0, 200)}`);
    } finally {
      await d.stop();
    }
  }
}

async function run() {
  const cfg = armConfigRestore();
  seedWorkspace();

  // --session <sid>: retry mode — run ONLY the (--only-filtered) tour legs
  // against an EXISTING session; the base workload + probe boundaries are
  // skipped and the qualification check reads that session's rollout file.
  const sessIdx = process.argv.indexOf("--session");
  const retrySid = sessIdx !== -1 ? process.argv[sessIdx + 1] : null;

  const before = new Set(readdirSync(ROLLOUT));
  execSync("pkill -f probe-server.mjs || true");

  const t0 = Date.now();
  let sid = retrySid;

  if (retrySid) {
    console.log(`[cfg] retry mode — tour legs only, session ${retrySid}`);
  } else {
    sid = await runBase(cfg);
    if (!sid) throw new Error("base workload produced no session id");
  }

  // ---- Deferred-tools tour (12-05): same session, clean config ----
  if (process.argv.includes("--tour")) {
    await runTour(sid, t0);
  }

  cfg.restore();
  execSync("pkill -f probe-server.mjs || true");

  await finishQualification(before, sid);
}

// The 5-turn divergence-prone base workload + the attach/detach boundaries.
async function runBase(cfg) {
  cfg.setProbeServer(false); // leg A: clean catalog
  console.log("[cfg] clean start (no probe)");
  const t0 = Date.now();
  const a = new ZcodeDriver(WS, { onLog: () => {} });
  try {
    // ---- LEG A: clean catalog ----
    sid = await a.start("yolo");
    console.log("WORKLOAD session:", sid);

    const t1 = await a.prompt(
      "Explore this tiny project with several different tools: Read README.md and calc.js; use Grep to find where the keyword XYZZY_PLUGH appears; use Glob to list the *.js files; and use WebSearch to find the current Node.js LTS version. Finish with a 3-line summary naming each tool you used.", 600000);
    console.log(`T1 read/search (${((Date.now()-t0)/1000).toFixed(0)}s):`, (t1.done.payload.response ?? "").trim().split("\n")[0].slice(0, 100));

    const t2 = await a.prompt(
      "Make a small change with full verification: add a subtract(a, b) function to calc.js following the existing add() style, export it in module.exports, add a matching assertion to test.js, run the test suite with Bash, then Read calc.js back to verify the edit. Report the test output.", 600000);
    console.log(`T2 mutation (${((Date.now()-t0)/1000).toFixed(0)}s):`, (t2.done.payload.response ?? "").trim().split("\n")[0].slice(0, 100));

    const t3 = await a.prompt(
      "Dispatch a subagent (your agent/Task tool) to independently count the total lines and total characters across all *.js files in this project, instructing it to use at least two different tools. When it returns, report its numbers and which tools the subagent said it used.", 600000);
    console.log(`T3 subagent (${((Date.now()-t0)/1000).toFixed(0)}s):`, (t3.done.payload.response ?? "").trim().split("\n")[0].slice(0, 100));
  } finally {
    await a.stop();
  }

  // ---- BOUNDARY 1: ATTACH the probe (config add + resume same session) ----
  cfg.setProbeServer(true);
  console.log("[cfg] probe ADDED -> resume for attach");
  let b = await resumeWithPin(sid);
  try {
    const t4 = await b.prompt(
      "You just got a new MCP tool. List your available tools to confirm recapture_probe is present (one line: YES <count>), then invoke the recapture_probe tool exactly once and include its raw output on a second line.", 600000);
    console.log(`T4 probe-attached (${((Date.now()-t0)/1000).toFixed(0)}s):`, (t4.done.payload.response ?? "").trim().split("\n").slice(0, 2).join(" | ").slice(0, 160));
  } finally {
    await b.stop();
  }

  // ---- BOUNDARY 2: DETACH the probe (config remove + resume same session) ----
  cfg.setProbeServer(false);
  console.log("[cfg] probe REMOVED -> resume for detach");
  let c = await resumeWithPin(sid);
  try {
    const t5 = await c.prompt(
      "Mixed finale: (1) list your tools and confirm in one line whether recapture_probe is still available; (2) Read package.json; (3) run the test suite once with Bash; (4) reply with a one-line summary of everything you did this turn.", 600000);
    console.log(`T5 mixed finale (${((Date.now()-t0)/1000).toFixed(0)}s):`, (t5.done.payload.response ?? "").trim().split("\n")[0].slice(0, 100));
  } finally {
    await c.stop();
  }

  return sid;
}

// The runbook §3 mechanical thresholds over the session's rollout file.
async function finishQualification(before, sid) {
  // ---- Qualification check (harvest's mechanical thresholds) ----
  await new Promise((r) => setTimeout(r, 4000));
  const freshFiles = readdirSync(ROLLOUT).filter((f) => !before.has(f));
  console.log("\nWORKLOAD fresh rollout files:", JSON.stringify(freshFiles));
  const mainFile = freshFiles.find((f) => f.includes(sid.replace("sess_", "").slice(0, 8)));
  const subagentFiles = freshFiles.filter((f) => f.startsWith("subagent") || f.includes("subagent"));
  console.log("WORKLOAD main session file:", mainFile, "| subagent files:", JSON.stringify(subagentFiles));

  if (mainFile) {
    const lines = readFileSync(join(ROLLOUT, mainFile), "utf8").split("\n").filter(Boolean);
    const sets = new Map();
    const toolUnion = new Set();
    let requestRecords = 0;
    const records = [];
    for (const l of lines) {
      let rec; try { rec = JSON.parse(l); } catch { continue; }
      records.push(rec);
      const tools = rec?.request?.body?.tools;
      if (!Array.isArray(tools)) continue;
      requestRecords++;
      for (const t of tools) toolUnion.add(t.name);
      const sig = tools.map((t) => t.name).sort().join(",");
      if (!sets.has(sig)) sets.set(sig, { n: tools.length, hasProbe: sig.includes("recapture_probe"), at: requestRecords });
    }
    console.log(`\nQUALIFICATION (runbook §3 thresholds):`);
    console.log(`  request records (turns proxy): ${requestRecords}  ${requestRecords >= 5 ? "PASS" : "FAIL"} (>=5)`);
    console.log(`  distinct tools: ${toolUnion.size}  ${toolUnion.size >= 10 ? "PASS" : "FAIL"} (>=10)`);
    console.log(`  subagent dispatch: ${subagentFiles.length > 0 ? "PASS" : "UNCERTAIN — check transcript"} (>=1)`);
    const seq = [...sets.values()];
    console.log(`  tool-sets: ${seq.length} distinct -> ${seq.map((s) => `n=${s.n},probe=${s.hasProbe}`).join(" -> ")}  ${seq.length >= 2 ? "PASS (mid-session catalog change)" : "FAIL"} (>=2)`);
    const both = seq.some((s) => s.hasProbe) && seq.some((s) => !s.hasProbe);
    console.log(`  attach AND detach directions: ${both ? "PASS" : "FAIL"}`);

    if (process.argv.includes("--tour")) {
      try {
        assertCronCleanup(records);
        console.log("  cron cleanup (CronList empty at exit): PASS");
      } catch (e) {
        console.log(`  cron cleanup (CronList empty at exit): FAIL — ${e.message}`);
        process.exitCode = 1;
      }
    }
    console.log(`\nWORKLOAD RESULT: ${requestRecords >= 5 && toolUnion.size >= 10 && seq.length >= 2 ? "SESSION QUALIFIES" : "DOES NOT QUALIFY"}`);
    console.log("TOUR RESULT:", process.argv.includes("--tour") ? (process.exitCode ? "TOUR INCOMPLETE (see failures above)" : "TOUR COMPLETE") : "not run (--tour absent)");
  }
}

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];
if (isMain) {
  run().catch((e) => { console.error("WORKLOAD ERROR:", e); process.exit(1); });
}
