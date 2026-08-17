// Kill test: does killing the MCP server subprocess mid-session shrink the live tool catalog?
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { execSync } from "node:child_process";

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

const ASK = "List your available tools by name. Do you have a tool called recapture_probe? Answer one line: 'YES <count>' or 'NO <count>'.";

const before = new Set(readdirSync(ROLLOUT));
setProbeServer(true);
const d = new ZcodeDriver("/tmp/zcode-recapture-ws", { onLog: (m) => console.log(m) });
try {
  const sid = await d.start("yolo");
  console.log("KILL session:", sid);

  const t1 = await d.prompt(ASK + " Then invoke recapture_probe exactly once and include its raw output on a second line.", 300000);
  console.log("T1 (probe alive):", t1.done.payload.response?.trim().split("\n").slice(0, 2).join(" | "));

  const pids = execSync("pgrep -f probe-server.mjs || true").toString().trim();
  console.log("[kill] probe pids:", JSON.stringify(pids.split("\n").filter(Boolean)));
  if (pids) execSync("pkill -f probe-server.mjs");
  console.log("[kill] probe server killed");
  await new Promise((r) => setTimeout(r, 3000));

  const t2 = await d.prompt(ASK, 300000);
  console.log("T2 (probe killed):", t2.done.payload.response?.trim().split("\n")[0]);

  await new Promise((r) => setTimeout(r, 2000));
  const t3 = await d.prompt(ASK, 300000);
  console.log("T3 (still dead):", t3.done.payload.response?.trim().split("\n")[0]);

  await new Promise((r) => setTimeout(r, 3000));
  const fresh = readdirSync(ROLLOUT).filter((f) => !before.has(f) && f.includes(sid.replace("sess_", "").slice(0, 8)));
  console.log("KILL rollout file:", JSON.stringify(fresh));

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
    console.log(`KILL ground truth: ${turns} request records, ${sets.length} distinct tool-sets:`);
    for (const s of sets) console.log(`  - turn ${s.atTurn}: ${s.n} tools, probe=${s.hasProbe}`);
    console.log(sets.length >= 2 ? "KILL RESULT: PASS (kill shrank the live catalog)" : "KILL RESULT: FAIL (catalog frozen even across server death)");
  }
} catch (e) {
  console.error("KILL RESULT: FAIL —", e.message);
  process.exitCode = 1;
} finally {
  try { const orig = readFileSync("/tmp/zcode-recapture/cli-config-backup.json"); writeFileSync(CLI_CFG, orig); console.log("[cfg] config restored"); } catch {}
  await d.stop();
}
