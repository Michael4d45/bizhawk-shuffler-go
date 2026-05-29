import type { ServerState } from "./types.js";

export function nextSwapDisplay(state: ServerState | null, nowMs = Date.now()): string {
  if (!state?.next_swap_at) return "—";
  const diff = Math.floor(state.next_swap_at - nowMs / 1000);
  if (diff <= 0) return "Due";
  const hrs = Math.floor(diff / 3600);
  const mins = Math.floor((diff % 3600) / 60);
  const secs = diff % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  if (hrs > 0) return `${hrs}:${pad(mins)}:${pad(secs)}`;
  return `${mins}:${pad(secs)}`;
}

export function swapProgress(state: ServerState | null, nowMs = Date.now()): number {
  if (!state?.next_swap_at || !state.min_interval_secs) return 0;
  const total = (state.max_interval_secs ?? state.min_interval_secs) || 300;
  const remaining = Math.max(0, state.next_swap_at - nowMs / 1000);
  return Math.min(100, Math.max(0, ((total - remaining) / total) * 100));
}

export function swapProgressUrgent(state: ServerState | null, nowMs = Date.now()): boolean {
  return swapProgress(state, nowMs) >= 95;
}

export function swapTimerActive(state: ServerState | null): boolean {
  return Boolean(state?.running && state.next_swap_at && state.min_interval_secs);
}

export function intervalDisplay(state: ServerState | null): string {
  if (!state?.min_interval_secs || !state?.max_interval_secs) return "—";
  return `${state.min_interval_secs} – ${state.max_interval_secs} sec`;
}
