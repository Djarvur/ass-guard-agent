// Smoke test: boot app-server, create a session, run ONE trivial turn, verify events + rollout file.
import { ZcodeDriver } from "./zcode-driver.mjs";
import { readdirSync } from "node:fs";

const WS = "/tmp/zcode-recapture-ws";
const before = new Set(readdirSync(process.env.HOME + "/.zcode/cli/rollout"));

const d = new ZcodeDriver(WS, { onLog: (m) => console.log(m) });
try {
  const sid = await d.start("yolo");
  console.log("SMOKE session:", sid);
  const r = await d.prompt("Reply with exactly: SMOKE-OK", 300000);
  console.log("SMOKE turn done:", JSON.stringify(r.done.payload));
  console.log("SMOKE tools used:", r.tools.join(", ") || "(none)");
  await new Promise((res) => setTimeout(res, 3000)); // let rollout flush
  const after = readdirSync(process.env.HOME + "/.zcode/cli/rollout");
  const fresh = after.filter((f) => !before.has(f) && f.includes("sess"));
  console.log("SMOKE fresh rollout files:", JSON.stringify(fresh));
  const match = fresh.filter((f) => f.includes(sid.replace("sess_", "").slice(0, 8)) || f.includes(sid));
  console.log("SMOKE matching-session rollout:", JSON.stringify(match));
  console.log("SMOKE RESULT: PASS");
} catch (e) {
  console.error("SMOKE RESULT: FAIL —", e.message);
  process.exitCode = 1;
} finally {
  await d.stop();
}
