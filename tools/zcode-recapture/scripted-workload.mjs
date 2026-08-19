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
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readFileSync, writeFileSync, readdirSync, mkdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { execSync } from "node:child_process";

const KIT_DIR = dirname(fileURLToPath(import.meta.url));
const CLI_CFG = join(process.env.HOME, ".zcode", "cli", "config.json");
const ROLLOUT = join(process.env.HOME, ".zcode", "cli", "rollout");
const WS = "/tmp/zcode-recapture-ws";
const RUN_DIR = "/tmp/zcode-recapture-run";
const BACKUP = join(RUN_DIR, "cli-config-backup.json");
const PROBE = join(KIT_DIR, "probe-server.mjs");
const MODEL = { providerId: "builtin:zai-coding-plan", modelId: "GLM-5.3" };

// T-12-05-01: backup BEFORE any touch; restore + diff-verify on every exit path.
mkdirSync(RUN_DIR, { recursive: true });
writeFileSync(BACKUP, readFileSync(CLI_CFG));
let cfgDirty = false;
function restoreConfigAndVerify() {
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
}
process.on("exit", restoreConfigAndVerify);
for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => { restoreConfigAndVerify(); process.exit(130); });

function setProbeServer(add) {
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
seedWorkspace();

async function resumeWithPin(sid) {
  const d = new ZcodeDriver(WS, { onLog: () => {} });
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

const before = new Set(readdirSync(ROLLOUT));
execSync("pkill -f probe-server.mjs || true");
setProbeServer(false); // leg A: clean catalog
console.log("[cfg] clean start (no probe)");

const t0 = Date.now();
let sid;
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
setProbeServer(true);
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
setProbeServer(false);
console.log("[cfg] probe REMOVED -> resume for detach");
let c = await resumeWithPin(sid);
try {
  const t5 = await c.prompt(
    "Mixed finale: (1) list your tools and confirm in one line whether recapture_probe is still available; (2) Read package.json; (3) run the test suite once with Bash; (4) reply with a one-line summary of everything you did this turn.", 600000);
  console.log(`T5 mixed finale (${((Date.now()-t0)/1000).toFixed(0)}s):`, (t5.done.payload.response ?? "").trim().split("\n")[0].slice(0, 100));
} finally {
  await c.stop();
}

restoreConfigAndVerify();
execSync("pkill -f probe-server.mjs || true");

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
  for (const l of lines) {
    let rec; try { rec = JSON.parse(l); } catch { continue; }
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
  console.log(`\nWORKLOAD RESULT: ${requestRecords >= 5 && toolUnion.size >= 10 && seq.length >= 2 ? "SESSION QUALIFIES" : "DOES NOT QUALIFY"}`);
}
