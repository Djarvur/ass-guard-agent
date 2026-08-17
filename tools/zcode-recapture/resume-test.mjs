// Resume test: does session/resume in a FRESH process re-read config (new tool-set) while
// continuing the SAME rollout session file? If yes: one session file, two tool-sets —
// the mid-session catalog change, achieved autonomously via a real-world resume pattern.
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readFileSync, writeFileSync, readdirSync, existsSync } from "node:fs";
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

const ASK = "List your available tools by name. Do you have a tool called recapture_probe? Answer one line: 'YES <count>' or 'NO <count>'.";
const before = new Set(readdirSync(ROLLOUT));

// Leg A: probe configured -> session with probe in catalog
setProbeServer(true);
let sid;
const a = new ZcodeDriver("/tmp/zcode-recapture-ws", { onLog: (m) => console.log(m) });
try {
  sid = await a.start("yolo");
  console.log("RESUME session:", sid);
  const t1 = await a.prompt(ASK + " Then invoke recapture_probe exactly once and include its raw output on a second line.", 300000);
  console.log("LEG-A T1 (probe on):", t1.done.payload.response?.trim().split("\n").slice(0, 2).join(" | "));
} finally {
  await a.stop();
}
console.log("[leg-a] process stopped");

// Leg B: probe REMOVED from config -> fresh process -> RESUME same session
setProbeServer(false);
console.log("[cfg] probe removed");
const b = new ZcodeDriver("/tmp/zcode-recapture-ws", { onLog: (m) => console.log(m) });
try {
  const resp = await b.rpc("session/resume", { sessionId: sid }, 60000);
  if (resp.error) throw new Error("resume failed: " + JSON.stringify(resp.error));
  // Pin the resumed session onto the default model (bridge's buildResumeRuntimeModel pattern).
  const pin = await b.rpc("session/setModel", {
    sessionId: sid,
    model: { providerId: "builtin:zai-coding-plan", modelId: "GLM-5.3" },
    runtimeModel: {
      revision: "recapture-resume-" + Date.now(),
      generatedAt: Date.now(),
      model: { providerId: "builtin:zai-coding-plan", modelId: "GLM-5.3" },
      provider: {
        providerId: "builtin:zai-coding-plan",
        kind: "anthropic",
        apiFormat: "anthropic-messages",
        baseURL: "https://api.z.ai/api/anthropic",
        models: [{ modelId: "GLM-5.3" }],
      },
    },
    persistAsWorkspaceLastUsed: false,
  }, 60000);
  if (pin.error) console.log("[setModel] error:", JSON.stringify(pin.error).slice(0, 200));
  b.sessionId = sid;
  await b.rpc("session/subscribe", { sessionId: sid, deliveryKind: "desktop-continuous", includeSnapshot: true, afterSeq: 0 }, 60000);
  const t2 = await b.prompt(ASK, 300000);
  console.log("LEG-B T2 (probe off, resumed):", t2.done.payload.response?.trim().split("\n")[0]);
} finally {
  try { const orig = readFileSync("/tmp/zcode-recapture/cli-config-backup.json"); writeFileSync(CLI_CFG, orig); console.log("[cfg] config restored"); } catch {}
  await b.stop();
}

await new Promise((r) => setTimeout(r, 3000));
const files = readdirSync(ROLLOUT).filter((f) => !before.has(f) && f.includes("sess"));
console.log("RESUME fresh rollout files:", JSON.stringify(files));
for (const f of files) {
  const lines = readFileSync(join(ROLLOUT, f), "utf8").split("\n").filter(Boolean);
  const sets = [];
  let turns = 0;
  for (const l of lines) {
    let rec; try { rec = JSON.parse(l); } catch { continue; }
    const tools = rec?.request?.body?.tools;
    if (!Array.isArray(tools)) continue;
    turns++;
    const sig = tools.map((t) => t.name).sort().join(",");
    if (!sets.some((s) => s.sig === sig)) sets.push({ n: tools.length, hasProbe: sig.includes("recapture_probe"), atTurn: turns });
  }
  console.log(`  ${f}: ${turns} request records, ${sets.length} distinct tool-sets: ${sets.map((s) => `turn${s.atTurn}(n=${s.n},probe=${s.hasProbe})`).join(" -> ")}`);
}
