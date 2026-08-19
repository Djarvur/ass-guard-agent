// 12-05 Task 2: the deferred-tool forms harvester.
//
// Reads a fresh zcode rollout JSONL (model-io records) and extracts every
// tool-result observation: request.messages[] entries with role "tool"
// { toolName, content (string), toolCallId, isError } — the CURRENT zcode
// wire shape (messagesKind=full; verified on the 2026-08-20 fresh capture;
// the 08-08 fixture's nested {type:'tool-result',output:{...}} block form is
// handled as a fallback for older corpora). Values are REDACTED AT EXTRACTION
// (D-03): variable spans (uuids, absolute paths, generated ids, digit runs)
// are masked to placeholders — the surviving fixed text IS the template (the
// mimicry target, preserved verbatim by construction).
//
// Output: the zcode-core-results.json fixture SHAPE
//   { _provenance:{...}, tools:{<Name>:{ input_keys_observed, input_note,
//     results:{<family>:{ isError, template, observed_count }}}}}
// plus a stdout report of hits/misses per tool. DETERMINISTIC: the same input
// records produce byte-identical fixture JSON (no timestamps inside families;
// the single harvest date comes from --date / opts.date).
import { readFileSync, writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const KIT_DIR = dirname(fileURLToPath(import.meta.url));

// --- redaction / templating -------------------------------------------------

/** Mask a uuid (any version). */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;
/** Mask an absolute path (unix). */
const ABS_PATH = /(?:^|[\s"'(:])((?:\/[\w. @-]+)+)/g;
/** Mask generated ids: cron_x, bash_x, msg_12, agent_<uuid>, sess_..., call_..., shell_... */
const GEN_ID = /\b(?:cron|bash|msg|agent|sess|call|shell|task|turn|interaction|automation)_[a-z0-9-]{4,}\b/gi;
/** Mask digit runs (exit codes, byte counts, timings, line numbers). */
const DIGITS = /\d+/g;

/** normalize derives the redacted template from one observed value. */
export function normalizeTemplate(value) {
  let t = String(value);
  t = t.replace(UUID, "<uuid>");
  t = t.replace(GEN_ID, "<id>");
  t = t.replace(ABS_PATH, (m, p) => m.replace(p, "<path>"));
  t = t.replace(DIGITS, "<n>");
  return t;
}

/** Family name from the template's fixed text (deterministic, content-derived). */
export function familyName(template, isError) {
  const t = template;
  if (/timed? ?out/i.test(t)) return "timeout";
  if (/truncat/i.test(t)) return "truncation";
  if (/User has answered your questions/i.test(t)) return "answered";
  if (/did not provide answers|has not answered/i.test(t)) return "non_answer";
  if (/File does not exist/i.test(t)) return "error_file_missing";
  if (/String to replace not found/i.test(t)) return "error_not_found";
  if (/has not been read yet/i.test(t)) return "error_not_read_yet";
  if (/Exit code/i.test(t)) return "error_exit_code";
  if (/Bash completed with no output/i.test(t)) return "success_no_output";
  if (/was (queued|delivered|sent) for local agent/i.test(t)) return "ack_queued";
  if (/Plan mode entered/i.test(t)) return "entered";
  if (/approved your plan/i.test(t)) return "plan_approved";
  if (/Automation created/i.test(t)) return "created";
  if (/Automation .* updated/i.test(t)) return "updated";
  if (/Automation .* deleted|Automation deleted/i.test(t)) return "deleted";
  if (/^No automations|^0 automations/i.test(t)) return "empty";
  if (/background task (was )?start/i.test(t)) return "background_started";
  if (/Task .* stopped|stopped task/i.test(t)) return "task_stopped";
  return isError ? "error_form" : "form";
}

// --- extraction --------------------------------------------------------------

/** Extract tool-result observations from parsed rollout records. */
export function extractToolResults(records) {
  const out = [];
  for (const rec of records ?? []) {
    const req = rec?.request ?? {};
    // Current shape: request.messages[] role:"tool" entries.
    for (const m of req.messages ?? []) {
      if (m?.role !== "tool") continue;
      let value = m.content;
      // Fallback: nested tool-result blocks (older corpora).
      if (Array.isArray(value)) {
        value = value
          .filter((b) => b?.type === "tool-result" || b?.type === "text")
          .map((b) => b?.output?.value ?? b?.value ?? b?.text ?? "")
          .join("\n");
      }
      out.push({ tool: m.toolName ?? "?", value: String(value ?? ""), isError: Boolean(m.isError) });
    }
    // Old shape fallback: body.messages[].content[] tool-result blocks.
    for (const m of req.body?.messages ?? []) {
      const blocks = Array.isArray(m?.content) ? m.content : [];
      for (const b of blocks) {
        if (b?.type !== "tool-result") continue;
        out.push({ tool: b.toolName ?? "?", value: String(b.output?.value ?? ""), isError: Boolean(b.isError) });
      }
    }
  }
  return out;
}

/** Extract observed input key sets from response.toolCalls[]. */
export function extractInputKeys(records) {
  const keys = new Map(); // tool -> Set(keys)
  for (const rec of records ?? []) {
    for (const c of rec?.response?.toolCalls ?? []) {
      if (!c?.name) continue;
      if (!keys.has(c.name)) keys.set(c.name, new Set());
      for (const k of Object.keys(c.input ?? {})) keys.get(c.name).add(k);
    }
  }
  return keys;
}

// --- harvest ------------------------------------------------------------------

/** Harvest parsed records into { fixture, report }. Deterministic per input. */
export function harvest(records, opts = {}) {
  const date = opts.date ?? "1970-01-01";
  const obs = extractToolResults(records);
  const inputKeys = extractInputKeys(records);

  // tool -> { families: Map(template -> {template,isError,count,name}), unnamedCounter }
  const tools = new Map();
  let unnamed = 0;
  for (const o of obs) {
    if (!tools.has(o.tool)) tools.set(o.tool, { families: new Map(), inputKeys: new Set() });
    const t = tools.get(o.tool);
    const tpl = normalizeTemplate(o.value);
    if (!t.families.has(tpl)) {
      const base = familyName(tpl, o.isError);
      const name = base === "error_form" || base === "form" ? `${base}_${++unnamed}` : base;
      t.families.set(tpl, { name, isError: o.isError, template: tpl, count: 0 });
    }
    t.families.get(tpl).count++;
  }
  for (const [name, ks] of inputKeys) {
    if (!tools.has(name)) tools.set(name, { families: new Map(), inputKeys: new Set() });
    tools.get(name).inputKeys = ks;
  }

  // Deterministic assembly: tools in first-occurrence order; families in
  // first-occurrence order; observed_count per family.
  const fixtureTools = {};
  const report = [];
  for (const [toolName, t] of tools) {
    const results = {};
    for (const f of t.families.values()) {
      results[f.name] = {
        isError: f.isError,
        template: f.template,
        observed_count: f.count,
      };
    }
    fixtureTools[toolName] = {
      input_keys_observed: [...t.inputKeys],
      input_note: t.inputKeys.size ? "observed key set union from response.toolCalls" : "no model input observed for this tool in this capture",
      results,
    };
    report.push(`${toolName}: ${t.families.size} family(ies), ${[...t.families.values()].reduce((a, f) => a + f.count, 0)} observation(s)`);
  }

  const fixture = {
    _provenance: {
      source: "zcode live rollout capture — request.messages role:'tool' entries {toolName, content, toolCallId, isError} (messagesKind=full), harvested by tools/zcode-recapture/harvest-deferred-forms.mjs. Templates are REDACTED AT EXTRACTION (D-03): uuids/paths/generated-ids/digit-runs masked; the fixed text is the mimicry target.",
      harvestDate: date,
      zcode_version: opts.zcodeVersion ?? null,
      session_id: opts.sessionId ?? null,
      redaction: "D-03 discipline: JSON structure preserved; values scrubbed into <uuid>/<path>/<id>/<n> placeholders; template texts preserved verbatim.",
    },
    tools: fixtureTools,
  };
  return { fixture, report: report.join("\n") };
}

// --- CLI -----------------------------------------------------------------------

function parseArgs(argv) {
  const a = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--out") a.out = argv[++i];
    else if (argv[i] === "--date") a.date = argv[++i];
    else if (argv[i] === "--zcode-version") a.zcodeVersion = argv[++i];
    else if (argv[i] === "--session") a.sessionId = argv[++i];
    else a._.push(argv[i]);
  }
  return a;
}

export function main(argv) {
  if (argv[0] === "--selftest") {
    const records = readFileSync(join(KIT_DIR, "testdata", "synthetic-rollout.jsonl"), "utf8")
      .split("\n").filter(Boolean).map((l) => JSON.parse(l));
    const { fixture } = harvest(records, { date: "2026-08-20" });
    const names = Object.keys(fixture.tools);
    if (!names.includes("AskUserQuestion") || !names.includes("CronCreate")) throw new Error("selftest: families missing");
    const blob = JSON.stringify(fixture);
    if (blob.includes("/tmp/") || /[0-9a-f]{8}-[0-9a-f]{4}/i.test(blob)) throw new Error("selftest: redaction leak");
    console.log("SELFTEST OK —", names.length, "tools");
    return;
  }
  const args = parseArgs(argv);
  const file = args._[0];
  if (!file) {
    console.error("usage: node harvest-deferred-forms.mjs <rollout.jsonl> [--out fixture.json] [--date D] [--zcode-version V] [--session S] | --selftest");
    process.exit(2);
  }
  const records = readFileSync(file, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l));
  const { fixture, report } = harvest(records, args);
  const json = JSON.stringify(fixture, null, 2) + "\n";
  if (args.out) {
    writeFileSync(args.out, json);
    console.log(`wrote ${args.out} (${json.length} bytes)`);
  } else {
    console.log(json);
  }
  console.log(report);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  main(process.argv.slice(2));
}
