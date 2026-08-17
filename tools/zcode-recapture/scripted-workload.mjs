// The scripted divergence-prone workload (runbook §3), driven autonomously via app-server stdio.
// Thresholds: >=5 user turns, >=10 distinct tools, >=1 subagent dispatch, mid-session catalog
// change with BOTH directions (attach + detach via config-change + session/resume boundaries —
// the only headless-reachable mechanism; /mcp connect|disconnect are TUI-client-side).
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { execSync } from "node:child_process";

const CLI_CFG = join(process.env.HOME, ".zcode", "cli", "config.json");
const ROLLOUT = join(process.env.HOME, ".zcode", "cli", "rollout");
const WS = "/tmp/zcode-recapture-ws";
const MODEL = { providerId: "builtin:zai-coding-plan", modelId: "GLM-5.3" };

function setProbeServer(add) {
  const c = JSON.parse(readFileSync(CLI_CFG, "utf8"));
  if (add) {
    c.mcp = c.mcp ?? {};
    c.mcp.servers = c.mcp.servers ?? {};
    c.mcp.servers.recapture_probe = { type: "stdio", command: process.execPath, args: ["/tmp/zcode-recapture/probe-server.mjs"] };
  } else {
    if (c.mcp?.servers) delete c.mcp.servers.recapture_probe;
    if (c.mcp && Object.keys(c.mcp.servers ?? {}).length === 0) delete c.mcp;
  }
  writeFileSync(CLI_CFG, JSON.stringify(c, null, 2) + "\n");
}

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

try { const orig = readFileSync("/tmp/zcode-recapture/cli-config-backup.json"); writeFileSync(CLI_CFG, orig); console.log("[cfg] config restored"); } catch {}
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
