import { useState } from "react";
import type { AdminTrigger } from "../adminActions.js";
import { post } from "../api.js";
import { useGamesPersist } from "../hooks/useGamesPersist.js";
import { usePlayerDrag } from "../PlayerDragContext.js";
import {
  autofillInstanceId,
  instanceFileStateBadgeVariant,
  instanceFileStateLabel,
  instancePlayerMap,
  unassignedPlayers,
} from "../saveModeUtils.js";
import { countPlayersCompletedInstance } from "../gameStats.js";
import type { GameSwapInstance, ServerState } from "../types.js";
import { DraggablePlayerChip } from "./DraggablePlayerChip.js";
import { ActionRow, Badge, Button, EmptyState, FieldLabel, Input, Select, cn } from "./ui.js";

type Props = {
  state: ServerState | null;
  expanded?: boolean;
  trigger: AdminTrigger;
  pushLog: (msg: string) => void;
  refreshState: () => Promise<ServerState | null>;
};

export function SaveInstancesCard({
  state,
  expanded = false,
  trigger,
  pushLog,
  refreshState,
}: Props) {
  const instances = state?.game_instances ?? [];
  const mainFiles = (state?.main_games ?? []).map((g) => g.file);
  const playersByInstance = instancePlayerMap(state);
  const unassigned = unassignedPlayers(state);
  const persistGames = useGamesPersist(pushLog);
  const dnd = usePlayerDrag();

  const [newId, setNewId] = useState("");
  const [newGame, setNewGame] = useState("");

  const swapToInstance = async (player: string, instanceId: string) => {
    await post("/api/swap_player", { player, instance_id: instanceId });
    await refreshState();
  };

  const handleDrop = async (instanceId: string | null) => {
    if (!dnd.draggedPlayer) return;
    const player = dnd.draggedPlayer;
    if (instanceId) {
      pushLog(`Assigning ${player} → ${instanceId}`);
      await swapToInstance(player, instanceId);
    } else {
      pushLog(`Unassigning ${player}`);
      await post("/api/swap_player", { player, instance_id: "" });
      await refreshState();
    }
  };

  const removeInstance = async (id: string) => {
    const next = instances.filter((i) => i.id !== id);
    if (await persistGames({ game_instances: next, main_games: [...(state?.main_games ?? [])] })) {
      await refreshState();
    }
  };

  const addInstance = async () => {
    if (!newId.trim() || !newGame) return;
    const inst: GameSwapInstance = {
      id: newId.trim(),
      game: newGame,
      file_state: "none",
    };
    if (
      await persistGames({
        game_instances: [...instances, inst],
        main_games: [...(state?.main_games ?? [])],
      })
    ) {
      setNewId("");
      setNewGame("");
      await refreshState();
    }
  };

  return (
    <div className="space-y-3">
      <div className={cn("space-y-2", !expanded && "max-h-72 overflow-y-auto scrollbar-thin pr-1")}>
        {instances.length === 0 ? (
          <EmptyState>No save instances. Add one below.</EmptyState>
        ) : (
          instances.map((inst) => {
            const assigned = playersByInstance.get(inst.id);
            const dropActive = dnd.dropTarget === inst.id;
            const completed = countPlayersCompletedInstance(state, inst.id);
            return (
              <div
                key={inst.id}
                className={cn(
                  "rounded-lg border-2 bg-slate-950/40 px-3 py-2 transition",
                  dropActive
                    ? "scale-[1.01] border-sky-500 bg-sky-950/30"
                    : "border-transparent hover:bg-slate-900/60"
                )}
                onDragOver={(e) => dnd.onDragOver(inst.id, e)}
                onDragLeave={(e) => dnd.onDragLeave(e)}
                onDrop={(e) => {
                  e.preventDefault();
                  void handleDrop(inst.id);
                }}
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    <p className="font-mono text-xs text-slate-200">{inst.id}</p>
                    <p className="text-[11px] text-slate-500">{inst.game || "(no game)"}</p>
                    <div className="mt-1 flex flex-wrap items-center gap-2">
                      <Badge variant={instanceFileStateBadgeVariant(inst)}>
                        {instanceFileStateLabel(inst)}
                      </Badge>
                      <span className="text-[11px] text-slate-500">
                        {assigned ? "assigned" : "unassigned"}
                      </span>
                      {completed > 0 ? <Badge variant="warn">{completed} completed</Badge> : null}
                    </div>
                    {assigned ? (
                      <div className="mt-2">
                        <DraggablePlayerChip name={assigned} dnd={dnd} variant="assigned" />
                      </div>
                    ) : null}
                  </div>
                  <ActionRow>
                    <Button
                      variant="ghost"
                      onClick={() =>
                        void trigger(
                          `/api/instances/${encodeURIComponent(inst.id)}/mark_completed_all`
                        )
                      }
                    >
                      Mark done all
                    </Button>
                    <Button variant="danger" onClick={() => void removeInstance(inst.id)}>
                      Remove
                    </Button>
                  </ActionRow>
                </div>
              </div>
            );
          })
        )}
      </div>

      <div
        className={cn(
          "rounded-lg border-2 border-dashed p-4 text-center transition",
          dnd.dropTarget === "unassigned"
            ? "border-sky-400 bg-sky-950/30"
            : "border-slate-700 bg-slate-950/30"
        )}
        onDragOver={(e) => dnd.onDragOver("unassigned", e)}
        onDragLeave={(e) => dnd.onDragLeave(e)}
        onDrop={(e) => {
          e.preventDefault();
          void handleDrop(null);
        }}
      >
        <p className="text-xs text-slate-400">Unassigned players</p>
        <p className="text-[11px] text-slate-600">Drop here to unassign</p>
        {unassigned.length > 0 ? (
          <ActionRow className="mt-2 justify-center">
            {unassigned.map((name) => (
              <DraggablePlayerChip key={name} name={name} dnd={dnd} />
            ))}
          </ActionRow>
        ) : (
          <p className="mt-2 text-[11px] text-slate-600">No unassigned players</p>
        )}
      </div>

      <div className="rounded-lg border border-slate-800 bg-slate-950/50 p-3">
        <p className="mb-2 text-[11px] font-medium uppercase text-slate-500">Add instance</p>
        <div className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
          <div>
            <FieldLabel>Instance ID</FieldLabel>
            <Input value={newId} onChange={(e) => setNewId(e.target.value)} />
          </div>
          <div>
            <FieldLabel>Game file</FieldLabel>
            <Select
              value={newGame}
              onChange={(e) => {
                setNewGame(e.target.value);
                if (!newId) setNewId(autofillInstanceId(e.target.value, [...instances]));
              }}
            >
              <option value="">— select —</option>
              {mainFiles.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </Select>
          </div>
          <div className="flex items-end">
            <Button
              variant="primary"
              disabled={!newId.trim() || !newGame}
              onClick={() => void addInstance()}
            >
              Add
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
