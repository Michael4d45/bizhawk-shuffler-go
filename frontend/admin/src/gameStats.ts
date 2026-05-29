import type { ServerState } from "./types.js";

export function countPlayersOnGame(state: ServerState | null, game: string): number {
  if (!state?.players) return 0;
  return Object.values(state.players).filter((p) => p.game === game).length;
}

export function countPlayersCompletedGame(state: ServerState | null, game: string): number {
  if (!state?.players) return 0;
  return Object.values(state.players).filter((p) => (p.completed_games ?? []).includes(game))
    .length;
}

export function countPlayersCompletedInstance(
  state: ServerState | null,
  instanceId: string
): number {
  if (!state?.players) return 0;
  return Object.values(state.players).filter((p) =>
    (p.completed_instances ?? []).includes(instanceId)
  ).length;
}

export function playerCompletionCount(p: {
  completed_games?: readonly string[];
  completed_instances?: readonly string[];
}): number {
  return (p.completed_games ?? []).length + (p.completed_instances ?? []).length;
}
