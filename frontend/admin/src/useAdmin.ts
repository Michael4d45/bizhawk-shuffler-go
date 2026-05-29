import { useEffect, useState } from "react";
import { useToast } from "./components/Toast.js";
import type { Command, ServerState } from "./types.js";
import { fetchState, post } from "./api.js";

export function wsUrl(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/ws`;
}

export function useAdmin() {
  const { showToast } = useToast();
  const [state, setState] = useState<ServerState | null>(null);
  const [log, setLog] = useState<string[]>([]);
  const [wsConnected, setWsConnected] = useState(false);

  function pushLog(msg: string) {
    setLog((prev) => [`${new Date().toLocaleTimeString()} ${msg}`, ...prev].slice(0, 200));
  }

  async function refreshState() {
    const s = await fetchState();
    setState(s);
    return s;
  }

  async function trigger(path: string, body?: unknown) {
    const res = await post(path, body);
    if (!res.ok) {
      const detail = (await res.text()).trim();
      pushLog(`${path} failed: ${res.status}${detail ? ` — ${detail}` : ""}`);
      showToast("Action failed", "err");
    } else {
      pushLog(`${path} ok`);
      showToast("Action successful", "ok");
      await refreshState();
    }
    return res.ok;
  }

  useEffect(() => {
    void refreshState().catch((e) => pushLog(String(e)));
    const interval = setInterval(() => {
      void refreshState().catch(() => undefined);
    }, 5000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    const ws = new WebSocket(wsUrl());
    ws.onopen = () => {
      setWsConnected(true);
      ws.send(
        JSON.stringify({
          cmd: "hello_admin",
          id: String(Date.now()),
          payload: { name: "admin-ui" },
        } satisfies Command)
      );
      pushLog("admin WS connected");
    };
    ws.onmessage = (ev) => {
      try {
        const cmd = JSON.parse(ev.data as string) as Command;
        if (cmd.cmd === "state_update") void refreshState();
      } catch {
        /* ignore */
      }
    };
    ws.onclose = () => setWsConnected(false);
    return () => ws.close();
  }, []);

  return { state, setState, log, pushLog, wsConnected, refreshState, trigger };
}
