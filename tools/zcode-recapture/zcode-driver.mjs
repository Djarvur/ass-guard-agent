// Minimal driver for `zcode app-server --stdio` (ZCode internal JSON-RPC, line-delimited, no jsonrpc field).
// Protocol per github.com/william0wang/zcode-acp docs/PROTOCOL.md.
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const ZCODE_BIN = "/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs";

/** Mirror zcode-acp credentials.ts: inject the first enabled provider from ~/.zcode/v2/config.json as env. */
function modelEnv() {
  try {
    const cfg = JSON.parse(readFileSync(join(process.env.HOME, ".zcode", "v2", "config.json"), "utf8"));
    for (const p of Object.values(cfg.provider ?? {})) {
      if (p?.enabled) {
        const env = {};
        const model = Object.keys(p.models ?? {})[0];
        if (model) env.ZCODE_MODEL = model;
        if (p.options?.baseURL) env.ZCODE_BASE_URL = p.options.baseURL;
        if (p.options?.apiKey) env.ANTHROPIC_API_KEY = p.options.apiKey;
        return env;
      }
    }
  } catch {}
  return {};
}

export class ZcodeDriver {
  constructor(wsPath, { onLog = () => {}, timeoutMs = 600000 } = {}) {
    this.wsPath = wsPath;
    this.onLog = onLog;
    this.defaultTimeout = timeoutMs;
    this.nextId = 1;
    this.pending = new Map(); // id -> {resolve, reject, timer}
    this.eventWaiters = []; // [{test, resolve}]
    this.events = [];
    this.sessionId = null;
    this.proc = spawn(process.execPath, [ZCODE_BIN, "app-server", "--stdio"], {
      cwd: wsPath,
      stdio: ["pipe", "pipe", "ignore"],
      detached: true, // own process group -> kill(-pid) reaps the tree
      env: { ...process.env, ...modelEnv() },
    });
    this.dead = false;
    this.proc.stdin.on("error", (err) => {
      this.dead = true;
      this.onLog(`[driver] stdin error: ${err.message}`);
    });
    const rl = createInterface({ input: this.proc.stdout });
    rl.on("line", (line) => this.handleLine(line.trim()));
    rl.on("close", () => {
      this.dead = true;
      this.onLog("[driver] stdout closed");
      for (const [, p] of this.pending) { clearTimeout(p.timer); p.reject(new Error("app-server stdout closed")); }
    });
  }

  handleLine(line) {
    if (!line) return;
    let msg;
    try { msg = JSON.parse(line); } catch { this.onLog(`[driver] unparseable: ${line.slice(0, 160)}`); return; }
    const { id, method } = msg;
    if (id !== undefined && method === undefined) {
      const p = this.pending.get(id);
      if (p) { clearTimeout(p.timer); this.pending.delete(id); p.resolve(msg); }
      return;
    }
    if (id !== undefined && method !== undefined) {
      // server -> client request
      if (method === "session/requestRuntimePreferences") {
        this.onLog(`[driver] replying ${method}`);
        this.send({ id, result: { nativeSearchEnhancementsEnabled: false, memoryEnabled: false, askUserQuestionAutoResolutionEnabled: false } });
      } else if (method.startsWith("interaction/")) {
        this.onLog(`[driver] auto-allowing ${method} ${JSON.stringify(msg.params).slice(0, 200)}`);
        this.send({ id, result: { outcome: { kind: "allow_once" } } });
      } else {
        this.onLog(`[driver] generic-acking server request ${method}`);
        this.send({ id, result: {} });
      }
      return;
    }
    if (method === "session/event") {
      const ev = msg.params ?? {};
      this.events.push(ev);
      this.eventWaiters = this.eventWaiters.filter((w) => {
        if (w.test(ev)) { w.resolve(ev); return false; }
        return true;
      });
    }
  }

  send(obj) {
    this.proc.stdin.write(JSON.stringify(obj) + "\n");
  }

  rpc(method, params, timeoutMs = this.defaultTimeout) {
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`rpc timeout: ${method}`)); }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      this.send({ id, method, params });
    });
  }

  async start(mode = "yolo") {
    const resp = await this.rpc("session/create", {
      workspace: { workspacePath: this.wsPath, workspaceKey: this.wsPath },
      mode,
    }, 60000);
    if (resp.error) throw new Error(`session/create failed: ${JSON.stringify(resp.error)}`);
    this.sessionId = resp.result.session.sessionId;
    this.onLog(`[driver] session ${this.sessionId}`);
    await this.rpc("session/subscribe", {
      sessionId: this.sessionId,
      deliveryKind: "desktop-continuous",
      includeSnapshot: true,
      afterSeq: 0,
    }, 60000);
    return this.sessionId;
  }

  waitForEvent(test, timeoutMs = this.defaultTimeout, label = "event") {
    const existing = this.events.find(test);
    if (existing) return Promise.resolve(existing);
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.eventWaiters = this.eventWaiters.filter((w) => w.w !== waiter);
        reject(new Error(`timeout waiting for ${label}`));
      }, timeoutMs);
      const waiter = { test, resolve: (ev) => { clearTimeout(timer); resolve(ev); }, w: null };
      waiter.w = waiter;
      this.eventWaiters.push(waiter);
    });
  }

  async prompt(content, timeoutMs = this.defaultTimeout) {
    this.events = []; // per-turn scoping: waiters registered after this see only this turn's events
    const resp = await this.rpc("session/send", { sessionId: this.sessionId, content }, 60000);
    if (resp.error) throw new Error(`session/send failed: ${JSON.stringify(resp.error)}`);
    const done = await this.waitForEvent(
      (ev) => ev.sessionId === this.sessionId && (ev.type === "turn.completed" || ev.type === "turn.failed"),
      timeoutMs,
      `turn completion for prompt: ${content.slice(0, 50)}`,
    );
    if (done.type === "turn.failed") throw new Error(`turn.failed: ${JSON.stringify(done.payload)}`);
    // collect tool names from this turn's events
    const tools = [...new Set(this.events.filter((e) => e.type === "tool.updated").map((e) => e.payload?.name).filter(Boolean))];
    return { done, tools, events: this.events };
  }

  async stop() {
    try { this.send({ method: "session/stop", params: { sessionId: this.sessionId } }); } catch {}
    try { process.kill(-this.proc.pid, "SIGKILL"); } catch {}
  }
}
