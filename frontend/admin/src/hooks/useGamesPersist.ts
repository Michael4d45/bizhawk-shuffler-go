import { postGames, type GamesPayload } from "../api.js";

export function useGamesPersist(onLog: (msg: string) => void) {
  return async function persistGames(payload: GamesPayload) {
    const res = await postGames(payload);
    if (!res.ok) {
      const detail = (await res.text()).trim();
      onLog(`save games failed: ${res.status}${detail ? ` — ${detail}` : ""}`);
      return false;
    }
    onLog("games saved");
    return true;
  };
}
