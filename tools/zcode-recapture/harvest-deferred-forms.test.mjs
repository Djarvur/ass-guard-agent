// 12-05 Task 2 self-tests (Behaviors 3-4): harvester determinism + redaction.
// Run: node --test tools/zcode-recapture/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { harvest } from "./harvest-deferred-forms.mjs";

const KIT = dirname(fileURLToPath(import.meta.url));
const SYNTHETIC = join(KIT, "testdata", "synthetic-rollout.jsonl");

function syntheticRecords() {
  return readFileSync(SYNTHETIC, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l));
}

// Behavior 3 (determinism): the same input yields byte-identical fixture JSON.
test("harvester is deterministic over the synthetic rollout", () => {
  const recs = syntheticRecords();
  const a = JSON.stringify(harvest(recs, { date: "2026-08-20" }).fixture, null, 2);
  const b = JSON.stringify(harvest(recs, { date: "2026-08-20" }).fixture, null, 2);
  assert.equal(a, b);
});

// Behavior 4 (redaction): no harvested family retains a real path, uuid, or
// prompt-verbatim span — values scrubbed, structure preserved.
test("harvested families are redacted (no paths, no uuids)", () => {
  const { fixture } = harvest(syntheticRecords(), { date: "2026-08-20" });
  const blob = JSON.stringify(fixture);
  assert.ok(!blob.includes("/tmp/"), "fixture leaks an absolute /tmp path");
  assert.ok(!blob.includes("zcode-recapture-ws"), "fixture leaks the scratch ws name");
  const uuid = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;
  for (const [tool, entry] of Object.entries(fixture.tools)) {
    for (const [fam, famDef] of Object.entries(entry.results ?? {})) {
      const text = JSON.stringify(famDef);
      assert.match(tool, /^[A-Za-z]+$/); // family keys sane
      assert.ok(!uuid.test(text), `${tool}.${fam} retains a uuid: ${text.slice(0, 120)}`);
    }
  }
});

// The synthetic corpus's known families cluster under the right tool names.
test("deferred-tool families are collected by tool name", () => {
  const { fixture, report } = harvest(syntheticRecords(), { date: "2026-08-20" });
  const tools = Object.keys(fixture.tools);
  for (const want of ["AskUserQuestion", "CronCreate", "CronList", "CronUpdate", "CronDelete",
    "TaskOutput", "TaskStop", "SendMessage", "ReadSessionContext", "EnterPlanMode", "ExitPlanMode", "Bash"]) {
    assert.ok(tools.includes(want), `fixture missing ${want} (have ${tools.join(",")})`);
  }
  // The synthetic answered-ask result form is preserved verbatim (template text
  // is the mimicry target) — the variable span is masked, the fixed text is not.
  const ask = fixture.tools.AskUserQuestion.results;
  const answered = Object.values(ask).find((f) => (f.template ?? "").includes("User has answered your questions"));
  assert.ok(answered, "answered-ask family missing");
  assert.ok(answered.observed_count >= 1, "answered-ask family carries observed_count");
  // The report names hits and misses per family.
  assert.ok(String(report).length > 0, "report is empty");
});

// Bash timeout + truncation families from the synthetic corpus survive clustering.
test("bash timeout and truncation hunt families cluster", () => {
  const { fixture } = harvest(syntheticRecords(), { date: "2026-08-20" });
  const bashFams = Object.keys(fixture.tools.Bash.results);
  assert.ok(bashFams.some((f) => /timeout/i.test(f)), `no timeout family in ${bashFams}`);
  assert.ok(bashFams.some((f) => /truncat/i.test(f)), `no truncation family in ${bashFams}`);
});

// --selftest stays green (the kit's runnable-check contract).
test("harvester --selftest exits 0", async () => {
  const { execFileSync } = await import("node:child_process");
  execFileSync(process.execPath, [join(KIT, "harvest-deferred-forms.mjs"), "--selftest"], { stdio: "pipe" });
});
