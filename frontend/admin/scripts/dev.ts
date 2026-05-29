import { watch } from "node:fs";
import { join, resolve } from "node:path";
import { buildAdminUI } from "./build-shared.js";

const pkgRoot = resolve(import.meta.dir, "..");
const outDir = resolve(pkgRoot, "../../serverhost/static");
const apiOrigin = process.env.BIZSHUFFLE_API ?? "http://127.0.0.1:8080";
const port = Number(process.env.ADMIN_DEV_PORT ?? 5173);
const wsBackend = apiOrigin.replace(/^http/i, "ws") + "/ws";

let building = false;
let buildQueued = false;

async function rebuild(): Promise<void> {
  if (building) {
    buildQueued = true;
    return;
  }
  building = true;
  try {
    console.log("[admin-ui] rebuilding…");
    await buildAdminUI({ outDir, minify: false, clean: true });
    console.log("[admin-ui] ready");
  } catch (err) {
    console.error("[admin-ui] build failed:", err);
  } finally {
    building = false;
    if (buildQueued) {
      buildQueued = false;
      void rebuild();
    }
  }
}

await rebuild();

let debounce: ReturnType<typeof setTimeout> | undefined;
watch(join(pkgRoot, "src"), { recursive: true }, () => {
  if (debounce) clearTimeout(debounce);
  debounce = setTimeout(() => void rebuild(), 200);
});

function proxyTarget(req: Request): string {
  const url = new URL(req.url);
  return `${apiOrigin}${url.pathname}${url.search}`;
}

function staticFile(pathname: string): Bun.BunFile {
  const rel = pathname === "/" ? "/index.html" : pathname;
  return Bun.file(join(outDir, rel));
}

type AdminWsData = {
  backend: WebSocket | null;
  pending: (string | Buffer<ArrayBuffer>)[];
};

console.log(`[admin-ui] http://127.0.0.1:${port}  →  API ${apiOrigin}`);

Bun.serve<AdminWsData>({
  port,
  hostname: "127.0.0.1",
  async fetch(req, server) {
    const url = new URL(req.url);
    if (url.pathname === "/ws") {
      if (server.upgrade(req, { data: { backend: null, pending: [] } })) return;
      return new Response("WebSocket upgrade failed", { status: 500 });
    }
    if (
      url.pathname.startsWith("/api/") ||
      url.pathname === "/api" ||
      url.pathname === "/state.json"
    ) {
      return fetch(proxyTarget(req), req);
    }
    const file = staticFile(url.pathname);
    if (await file.exists()) return new Response(file);
    return new Response("Not Found", { status: 404 });
  },
  websocket: {
    open(ws) {
      const backend = new WebSocket(wsBackend);
      ws.data.backend = backend;
      backend.addEventListener("open", () => {
        for (const msg of ws.data.pending) backend.send(msg);
        ws.data.pending.length = 0;
      });
      backend.addEventListener("message", (ev) => {
        ws.send(ev.data);
      });
      backend.addEventListener("close", () => ws.close());
      backend.addEventListener("error", () => ws.close());
    },
    message(ws, message) {
      const backend = ws.data.backend;
      if (backend?.readyState === WebSocket.OPEN) {
        backend.send(message);
        return;
      }
      ws.data.pending.push(message);
    },
    close(ws) {
      ws.data.backend?.close();
      ws.data.backend = null;
      ws.data.pending.length = 0;
    },
  },
});
