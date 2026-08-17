// Decisive test: pre-configure probe server in ~/.zcode/cli/config.json, then use
// in-session /mcp connect + /mcp disconnect slash commands to change the catalog mid-session.
// Ground truth = distinct tool-sets in the session's rollout request records.
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

const before = new Set(readdirSync(ROLLOUT));
setProbeServer(true);
console.log("[cfg] probe pre-configured BEFORE app-server start");

const d = new ZcodeDriver("/tmp/zcode-recapture-ws", { onLog: (m) => console.log(m) });
try {
  const sid = await d.start("yolo");
  console.log("TEST2 session:", sid);

  const t1 = await d.prompt("List your available tools by name. Do you have a tool called recapture_probe? Answer one line: 'YES <count>' or 'NO <count>' (<count> = total tools).", 300000);
  console.log("T1 (fresh session, server configured):", t1.done.payload.response?.trim().split("\n")[0]);

  const t2 = await d.prompt("/mcp connect recapture_probe", 120000);
  console.log("T2 (/mcp connect):", t2.done.payload.response?.trim().split("\n")[0]);

  const t3 = await d.prompt("List your available tools by name. Do you have a tool called recapture_probe now? Answer one line: 'YES <count>' or 'NO <count>'. If you have it, invoke it exactly once and add its raw output on a second line.", 300000);
  console.log("T3 (post-connect):", t3.done.payload.response?.trim().split("\n").slice(0, 2).join(" | "));
  console.log("T3 tools fired:", t3.tools.join(","));

  const t4 = await d.prompt("/mcp disconnect recapture_probe", 120000);
  console.log("T4 (/mcp disconnect):", t4.done.payload.response?.trim().split("\n")[0]);

  const t5 = await d.prompt("List your available tools by name. Is recapture_probe still available? Answer one line: 'YES <count>' or 'NO <count>'.", 300000);
  console.log("T5 (post-disconnect):", t5.done.payload.response?.trim().split("\n")[0]);

  await new Promise((r) => setTimeout(r, 3000));
  const fresh = readdirSync(ROLLOUT).filter((f) => !before.has(f) && f.includes(sid.replace("sess_", "").slice(0, 8)));
  console.log("TEST2 rollout file:", JSON.stringify(fresh));

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
    console.log(`TEST2 ground truth: ${turns} request records, ${sets.length} distinct tool-sets:`);
    for (const s of sets) console.log(`  - turn ${s.atTurn}: ${s.n} tools, probe=${s.hasProbe}`);
    console.log(sets.length >= 2 ? "TEST2 RESULT: PASS (catalog changed mid-session)" : "TEST2 RESULT: FAIL (catalog frozen)");
  }
} catch (e) {
  console.error("TEST2 RESULT: FAIL —", e.message);
  process.exitCode = 1;
} finally {
  try { const orig = readFileSync("/tmp/zcode-recapture/cli-config-backup.json"); writeFileSync(CLI_CFG, orig); console.log("[cfg] config restored from backup"); } catch {}
  await d.stop();
}
