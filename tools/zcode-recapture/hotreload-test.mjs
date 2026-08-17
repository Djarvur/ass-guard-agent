// Hot-reload test: does a RUNNING zcode app-server session pick up ~/.zcode/cli/config.json
// mcp.servers changes between turns? Ground truth = the rollout file's per-request tool-sets.
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const CLI_CFG = join(process.env.HOME, ".zcode", "cli", "config.json");
const ROLLOUT = join(process.env.HOME, ".zcode", "cli", "rollout");

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

const ASK = "List your available tools by name. Do you have a tool called recapture_probe? Answer with one line: 'YES <count>' or 'NO <count>' where <count> is your total number of tools.";
const ASK_CALL = ASK + " If you have recapture_probe, invoke it exactly once and include its raw output on a second line.";

const before = new Set(readdirSync(ROLLOUT));
const d = new ZcodeDriver("/tmp/zcode-recapture-ws", { onLog: (m) => console.log(m) });
let added = false;
try {
  const sid = await d.start("yolo");
  console.log("HOTRELOAD session:", sid);

  const t1 = await d.prompt(ASK, 300000);
  console.log("T1 (no server):", t1.done.payload.response?.trim().split("\n")[0]);

  setProbeServer(true); added = true;
  console.log("[cfg] recapture_probe ADDED");
  await new Promise((r) => setTimeout(r, 2000));

  const t2 = await d.prompt(ASK_CALL, 300000);
  console.log("T2 (server added):", t2.done.payload.response?.trim().split("\n").slice(0, 2).join(" | "));
  console.log("T2 tools fired:", t2.tools.join(","));

  setProbeServer(false); added = false;
  console.log("[cfg] recapture_probe REMOVED");
  await new Promise((r) => setTimeout(r, 2000));

  const t3 = await d.prompt(ASK, 300000);
  console.log("T3 (server removed):", t3.done.payload.response?.trim().split("\n")[0]);

  await new Promise((r) => setTimeout(r, 3000));
  const fresh = readdirSync(ROLLOUT).filter((f) => !before.has(f) && f.includes(sid.replace("sess_", "").slice(0, 8)));
  console.log("HOTRELOAD rollout file:", JSON.stringify(fresh));

  // Ground truth: distinct tool-sets across this session's request records
  for (const f of fresh) {
    const lines = readFileSync(join(ROLLOUT, f), "utf8").split("\n").filter(Boolean);
    const sets = [];
    let turns = 0;
    for (const l of lines) {
      let rec; try { rec = JSON.parse(l); } catch { continue; }
      const tools = rec?.request?.body?.tools;
      if (!Array.isArray(tools)) continue;
      turns++;
      const sig = tools.map((t) => t.name).sort().join(",");
      if (!sets.some((s) => s.sig === sig)) sets.push({ sig, n: tools.length, hasProbe: sig.includes("recapture_probe"), atTurn: turns });
    }
    console.log(`HOTRELOAD ground truth: ${turns} request records, ${sets.length} distinct tool-sets:`);
    for (const s of sets) console.log(`  - turn ${s.atTurn}: ${s.n} tools, probe=${s.hasProbe}`);
  }
  console.log("HOTRELOAD RESULT: see ground truth above (>=2 distinct sets with probe flipping = PASS)");
} catch (e) {
  console.error("HOTRELOAD RESULT: FAIL —", e.message);
  process.exitCode = 1;
} finally {
  if (added) { try { setProbeServer(false); } catch {} }
  try { const orig = readFileSync("/tmp/zcode-recapture/cli-config-backup.json"); writeFileSync(CLI_CFG, orig); } catch {}
  await d.stop();
}
