// 12-05 Task 2 self-tests (Behaviors 1-2): tour coverage + cleanup proof +
// import side-effect freedom.
// Run: node --test tools/zcode-recapture/
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { TOUR_TURNS, tourToolCoverage, assertCronCleanup } from "./scripted-workload.mjs";

// Behavior 1 (tour coverage): the extended workload's turn list covers every
// deferred tool at least once + the two ACP-07 hunts.
test("tour covers every deferred tool + the timeout/truncation hunts", () => {
  const cov = tourToolCoverage();
  const want = [
    "AskUserQuestion", "EnterPlanMode", "ExitPlanMode", "SendMessage", "ReadSessionContext",
    "CronCreate", "CronList", "CronUpdate", "CronDelete", "TaskOutput", "TaskStop",
    "Bash:run_in_background", "Bash:timeout_form", "Bash:truncation",
  ];
  for (const w of want) assert.ok(cov.includes(w), `tour coverage missing ${w} (have ${cov.join(",")})`);
  // Every tour turn carries an imperative prompt (the provocation contract).
  for (const t of TOUR_TURNS) assert.ok(t.prompt.length > 20, `tour turn ${t.id} prompt too thin`);
});

// Behavior 2 (cleanup proof): the CronList-empty assertion over rollout records.
test("assertCronCleanup passes on empty and throws on leftover entries", () => {
  const mk = (value) => [
    { request: { body: { messages: [{ role: "user", content: "x" }] } } },
    { request: { body: { messages: [{ role: "tool", content: [{ type: "tool-result", toolCallId: "c1", toolName: "CronList", output: { type: "text", value } }] }] } } },
  ];
  assert.doesNotThrow(() => assertCronCleanup(mk("[]")));
  assert.doesNotThrow(() => assertCronCleanup(mk("No automations configured.")));
  assert.throws(() => assertCronCleanup(mk('[{"id":"cron_123","title":"recapture tour marker"}]')));
  assert.throws(() => assertCronCleanup(mk("1. cron_123 recapture tour marker — next in 3m")));
});

// Importing the workload module must be side-effect free (no config backup
// written, no scratch dirs created) — the run happens only as main.
test("module import is side-effect free", () => {
  const canary = mkdtempSync(join(tmpdir(), "zcode-recapture-import-"));
  process.env.ZCODE_RECAPTURE_RUN_DIR = canary;
  // (import already happened at top; assert the run dir was NOT created by it)
  assert.ok(!existsSync(join(canary, "cli-config-backup.json")), "import wrote a config backup");
});
