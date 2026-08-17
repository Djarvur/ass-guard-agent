// Minimal stdio MCP server exposing one trivial tool (recapture_probe).
// Used ONLY as the catalog-change probe for the zcode parity re-capture workload.
import { createInterface } from "node:readline";

const rl = createInterface({ input: process.stdin });
const out = (o) => process.stdout.write(JSON.stringify(o) + "\n");

rl.on("line", (l) => {
  l = l.trim();
  if (!l) return;
  let m;
  try { m = JSON.parse(l); } catch { return; }
  const { id, method, params } = m;
  if (id === undefined) return; // notification (e.g. notifications/initialized)
  if (method === "initialize") {
    out({ jsonrpc: "2.0", id, result: { protocolVersion: params?.protocolVersion || "2024-11-05", capabilities: { tools: {} }, serverInfo: { name: "recapture-probe", version: "1.0.0" } } });
  } else if (method === "tools/list") {
    out({ jsonrpc: "2.0", id, result: { tools: [{
      name: "recapture_probe",
      description: "Returns a fixed marker string; exists to make the tool catalog observably change.",
      inputSchema: { type: "object", properties: {}, additionalProperties: false },
    }] } });
  } else if (method === "tools/call") {
    out({ jsonrpc: "2.0", id, result: { content: [{ type: "text", text: "recapture-probe-ok (probe server alive)" }] } });
  } else if (method === "ping") {
    out({ jsonrpc: "2.0", id, result: {} });
  } else {
    out({ jsonrpc: "2.0", id, error: { code: -32601, message: `unknown method ${method}` } });
  }
});
