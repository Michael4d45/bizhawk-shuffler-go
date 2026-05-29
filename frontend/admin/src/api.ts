import type { GameEntry, GameSwapInstance, Plugin, ServerState } from "./types.js";

export type ShareUrls = {
  lan: string[];
  wan: string | null;
  local_only: boolean;
};

export async function fetchShareUrls(): Promise<ShareUrls> {
  return fetchJson<ShareUrls>("/api/share_urls");
}

export async function fetchState(): Promise<ServerState> {
  const res = await fetch("/state.json");
  if (!res.ok) throw new Error(`state.json ${res.status}`);
  const body = (await res.json()) as { state: ServerState };
  return body.state;
}

export async function post(path: string, body?: unknown): Promise<Response> {
  return fetch(path, {
    method: "POST",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export async function postForm(path: string, form: FormData): Promise<Response> {
  return fetch(path, { method: "POST", body: form });
}

async function del(path: string): Promise<Response> {
  return fetch(path, { method: "DELETE" });
}

export async function fetchJson<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path} ${res.status}`);
  return (await res.json()) as T;
}

export type GamesPayload = {
  games?: string[];
  main_games?: GameEntry[];
  game_instances?: GameSwapInstance[];
};

export async function postGames(payload: GamesPayload): Promise<Response> {
  return post("/api/games", payload);
}

export async function fetchFilesList(): Promise<string[]> {
  const raw = await fetchJson<unknown>("/files/list.json");
  if (!Array.isArray(raw)) return [];
  return raw
    .map((f) => {
      if (!f) return "";
      if (typeof f === "string") return f;
      if (typeof f === "object" && f !== null && "name" in f) {
        return String((f as { name?: string }).name ?? "");
      }
      return "";
    })
    .filter(Boolean);
}

export async function getPluginDetails(name: string): Promise<Plugin> {
  return fetchJson(`/api/plugins/${encodeURIComponent(name)}`);
}

export async function getPluginSettings(name: string): Promise<Record<string, string>> {
  return fetchJson(`/api/plugins/${encodeURIComponent(name)}/settings`);
}

export async function postPluginSettings(
  name: string,
  settings: Record<string, string>
): Promise<Response> {
  return post(`/api/plugins/${encodeURIComponent(name)}/settings`, settings);
}

export async function addCompletedGame(player: string, game: string): Promise<Response> {
  return post(`/api/players/${encodeURIComponent(player)}/completed_games`, { game });
}

export async function removeCompletedGame(player: string, game: string): Promise<Response> {
  return del(
    `/api/players/${encodeURIComponent(player)}/completed_games?game=${encodeURIComponent(game)}`
  );
}

export async function addCompletedInstance(player: string, instance: string): Promise<Response> {
  return post(`/api/players/${encodeURIComponent(player)}/completed_instances`, { instance });
}

export async function removeCompletedInstance(player: string, instance: string): Promise<Response> {
  return del(
    `/api/players/${encodeURIComponent(player)}/completed_instances?instance=${encodeURIComponent(instance)}`
  );
}

export type MessagePayload = {
  message: string;
  duration?: number;
  x?: number;
  y?: number;
  fontsize?: number;
  fg?: string;
  bg?: string;
};

export const defaultMessageComposer = (): MessagePayload & { text: string } => ({
  text: "",
  message: "",
  duration: 3,
  x: 10,
  y: 10,
  fontsize: 12,
  fg: "#FFFFFF",
  bg: "#000000",
});
