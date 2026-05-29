import type { Player, ServerState } from "./types.js";

export function playerCounts(state: ServerState | null) {
  const players = Object.values(state?.players ?? {});
  const online = players.filter((p) => p.connected).length;
  const ready = players.filter((p) => p.connected && p.bizhawk_ready).length;
  return { total: players.length, online, ready };
}

export function playerStatusBadge(p: Player): { label: string; variant: "ok" | "warn" | "err" } {
  if (!p.connected) return { label: "Offline", variant: "err" };
  if (p.bizhawk_ready) return { label: "Ready", variant: "ok" };
  return { label: "Connected", variant: "warn" };
}

export function formatUpdatedAt(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}
